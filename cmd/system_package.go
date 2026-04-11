package cmd

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
)

var execCommand = exec.Command
var lookPathCommand = exec.LookPath

func installSystemPackage(opts InstallOpts) (*InstallResult, error) {
	p, pResult, err := fetchBinary(providers.New, opts.URL, opts.Provider, opts.FetchOpts)
	if err != nil {
		return nil, err
	}

	existing, _ := existingConfigBinary(opts)
	minAgeDays := 0
	if existing != nil {
		minAgeDays = existing.MinAgeDays
	}
	if opts.MinAgeDays != nil {
		minAgeDays = *opts.MinAgeDays
	}
	if err := ensureReleaseAge(p.GetID(), pResult.Version, pResult.PublishedAt, minAgeDays); err != nil {
		return nil, err
	}

	pkgType, ok := detectSystemPackageType(pResult.Name)
	if !ok {
		return nil, fmt.Errorf("selected artifact %q is not a supported system package", pResult.Name)
	}
	requiredType := normalizePackageType(opts.FetchOpts.PackageType)
	if requiredType != "" && pkgType != requiredType {
		return nil, fmt.Errorf("selected package type %q does not match required type %q", pkgType, requiredType)
	}

	before, err := snapshotPathCommands()
	if err != nil {
		return nil, err
	}

	artifactPath, err := writePackageArtifactToTemp(pResult.Name, pResult.Data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(artifactPath)

	if err := installPackageArtifact(pkgType, artifactPath); err != nil {
		return nil, err
	}

	resolvedPath, trackedName, err := resolveTrackedSystemCommand(opts.FetchOpts.PackageName, before)
	if err != nil {
		return nil, err
	}

	hashString, err := hashExecutableFile(resolvedPath)
	if err != nil {
		return nil, err
	}

	pinned := opts.Pinned
	if existing != nil {
		pinned = pinned || existing.Pinned
	}

	configPath, err := absExpandedPath(resolvedPath)
	if err != nil {
		return nil, err
	}

	if err := config.UpsertBinary(&config.Binary{
		RemoteName:  trackedName,
		Path:        configPath,
		Version:     pResult.Version,
		Hash:        hashString,
		URL:         opts.URL,
		Provider:    p.GetID(),
		InstallMode: installModeSystemPackage,
		PackageType: pkgType,
		PackagePath: pResult.PackagePath,
		Pinned:      pinned,
		MinAgeDays:  minAgeDays,
	}); err != nil {
		return nil, err
	}

	warnDuplicateManagedHash(configPath, hashString)

	return &InstallResult{Name: trackedName, Version: pResult.Version, Path: configPath}, nil
}

func writePackageArtifactToTemp(name string, src io.Reader) (string, error) {
	if closer, ok := src.(io.Closer); ok {
		defer closer.Close()
	}

	ext := filepath.Ext(strings.ToLower(name))
	if strings.HasSuffix(strings.ToLower(name), ".pkg.tar.zst") {
		ext = ".pkg.tar.zst"
	}
	if strings.HasSuffix(strings.ToLower(name), ".flatpack") {
		ext = ".flatpak"
	}

	f, err := os.CreateTemp("", "bin-system-package-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, src); err != nil {
		return "", err
	}

	return f.Name(), nil
}

func installPackageArtifact(packageType, packagePath string) error {
	var cmd *exec.Cmd
	switch packageType {
	case "deb":
		cmd = execCommand("dpkg", "-i", packagePath)
	case "rpm":
		cmd = execCommand("rpm", "-Uvh", packagePath)
	case "apk":
		cmd = execCommand("apk", "add", "--allow-untrusted", packagePath)
	case "flatpak":
		cmd = execCommand("flatpak", "install", "--noninteractive", "-y", packagePath)
	default:
		return fmt.Errorf("unsupported package type %q", packageType)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install %s package: %v (%s)", packageType, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resolveTrackedSystemCommand(commandName string, before map[string]string) (string, string, error) {
	if strings.TrimSpace(commandName) != "" {
		resolved, err := lookPathCommand(commandName)
		if err != nil {
			return "", "", fmt.Errorf("system-package install succeeded but command %q was not found on PATH", commandName)
		}
		return resolved, filepath.Base(commandName), nil
	}

	after, err := snapshotPathCommands()
	if err != nil {
		return "", "", err
	}

	newCommands := make([]string, 0)
	for name, resolvedPath := range after {
		if _, exists := before[name]; !exists {
			if resolvedPath == "" {
				continue
			}
			newCommands = append(newCommands, name)
		}
	}
	sort.Strings(newCommands)

	if len(newCommands) == 0 {
		return "", "", fmt.Errorf("system package did not expose a new command on PATH; pass an explicit command name as the second argument")
	}
	if len(newCommands) > 1 {
		return "", "", fmt.Errorf("system package exposed multiple commands (%s); pass the command name as the second argument", strings.Join(newCommands, ", "))
	}

	command := newCommands[0]
	resolvedPath, err := lookPathCommand(command)
	if err != nil {
		return "", "", fmt.Errorf("resolved command %q is not available on PATH", command)
	}

	return resolvedPath, command, nil
}

func snapshotPathCommands() (map[string]string, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return map[string]string{}, nil
	}

	commands := map[string]string{}
	for _, dir := range filepath.SplitList(pathEnv) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, exists := commands[name]; exists {
				continue
			}
			fullPath := filepath.Join(dir, name)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0o111 == 0 {
				continue
			}
			commands[name] = fullPath
		}
	}
	return commands, nil
}

func hashExecutableFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func uninstallSystemPackage(b *config.Binary) error {
	packageType := normalizePackageType(b.PackageType)
	if packageType == "" {
		return fmt.Errorf("missing package type metadata for %s", b.Path)
	}

	packageID, err := resolveInstalledPackageID(b, packageType)
	if err != nil {
		return err
	}

	if packageID == "" {
		return fmt.Errorf("could not determine installed package identifier for %s", b.Path)
	}

	var cmd *exec.Cmd
	switch packageType {
	case "deb":
		cmd = execCommand("dpkg", "-r", packageID)
	case "rpm":
		cmd = execCommand("rpm", "-e", packageID)
	case "apk":
		cmd = execCommand("apk", "del", packageID)
	case "flatpak":
		cmd = execCommand("flatpak", "uninstall", "--noninteractive", "-y", packageID)
	default:
		return fmt.Errorf("unsupported package type %q", packageType)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to uninstall %s package %q: %v (%s)", packageType, packageID, err, strings.TrimSpace(string(out)))
	}

	return nil
}

func resolveInstalledPackageID(b *config.Binary, packageType string) (string, error) {
	path := os.ExpandEnv(b.Path)

	switch packageType {
	case "deb":
		out, err := execCommand("dpkg", "-S", path).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to resolve deb owner for %s: %v", b.Path, err)
		}
		line := firstLine(string(out))
		owner := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
		owner = strings.Split(owner, ",")[0]
		return owner, nil
	case "rpm":
		out, err := execCommand("rpm", "-qf", path).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to resolve rpm owner for %s: %v", b.Path, err)
		}
		return strings.TrimSpace(firstLine(string(out))), nil
	case "apk":
		out, err := execCommand("apk", "info", "--who-owns", path).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to resolve apk owner for %s: %v", b.Path, err)
		}
		line := strings.TrimSpace(firstLine(string(out)))
		if idx := strings.LastIndex(line, " "); idx > 0 {
			return strings.TrimSpace(line[idx+1:]), nil
		}
		return line, nil
	case "flatpak":
		base := filepath.Base(path)
		if strings.Count(base, ".") >= 2 {
			return base, nil
		}

		out, err := execCommand("flatpak", "list", "--columns=application,command").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to resolve flatpak app for %s: %v", b.Path, err)
		}
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			appID := fields[0]
			cmdName := fields[1]
			if cmdName == filepath.Base(path) || cmdName == b.RemoteName {
				return appID, nil
			}
		}
		return "", fmt.Errorf("failed to resolve flatpak app id for %s", b.Path)
	default:
		return "", fmt.Errorf("unsupported package type %q", packageType)
	}
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func detectSystemPackageType(name string) (string, bool) {
	lower := strings.ToLower(name)

	switch {
	case strings.HasSuffix(lower, ".flatpak"), strings.HasSuffix(lower, ".flatpack"):
		return "flatpak", true
	case strings.HasSuffix(lower, ".deb"):
		return "deb", true
	case strings.HasSuffix(lower, ".rpm"):
		return "rpm", true
	case strings.HasSuffix(lower, ".apk"):
		return "apk", true
	default:
		return "", false
	}
}

func normalizePackageType(packageType string) string {
	switch strings.ToLower(strings.TrimSpace(packageType)) {
	case "flatpack":
		return "flatpak"
	default:
		return strings.ToLower(strings.TrimSpace(packageType))
	}
}

func systemPackagePathLooksExplicit(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, `\\`) {
		return true
	}
	if strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "~") {
		return true
	}
	return false
}

func logSystemPackageSelected(packageType, commandName string) {
	if commandName == "" {
		log.Infof("Installing using %s system package mode", packageType)
		return
	}
	log.Infof("Installing using %s system package mode (tracking command %s)", packageType, commandName)
}
