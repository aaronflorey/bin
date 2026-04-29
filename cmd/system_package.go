package cmd

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/systempackage"
	"github.com/caarlos0/log"
)

var execCommand = exec.Command
var lookPathCommand = exec.LookPath
var applicationsDir = "/Applications"

func installSystemPackage(opts InstallOpts) (*InstallResult, error) {
	p, pResult, err := fetchBinary(installProviderFactory, opts.URL, opts.Provider, opts.FetchOpts, opts.AllowProviderFallback)
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

	pkgType, ok := systempackage.DetectType(pResult.Name)
	if !ok {
		return nil, systempackage.NewCompatibilityError("selected artifact %q is not a supported system package", pResult.Name)
	}
	requiredType := systempackage.NormalizeType(opts.FetchOpts.PackageType)
	if requiredType != "" && pkgType != requiredType {
		return nil, systempackage.NewCompatibilityError("selected package type %q does not match required type %q", pkgType, requiredType)
	}

	var before map[string]string
	if pkgType != "dmg" {
		before, err = snapshotPathCommands()
		if err != nil {
			return nil, err
		}
	}

	artifactPath, err := writePackageArtifactToTemp(pResult.Name, pResult.Data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(artifactPath)

	installedAppBundle := ""
	if pkgType == "dmg" {
		installedAppBundle, err = installDMGApp(artifactPath)
		if err != nil {
			return nil, err
		}
	} else if err := installPackageArtifact(pkgType, artifactPath); err != nil {
		return nil, err
	}

	resolvedPath, trackedName, appBundle, err := resolveTrackedSystemInstall(pkgType, opts.FetchOpts.PackageName, installedAppBundle, before)
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
		AppBundle:   appBundle,
		PackagePath: pResult.PackagePath,
		Pinned:      pinned,
		MinAgeDays:  minAgeDays,
	}); err != nil {
		return nil, err
	}

	warnDuplicateManagedHash(configPath, hashString)

	return &InstallResult{Name: trackedName, Version: pResult.Version, Path: configPath}, nil
}

func resolveTrackedSystemInstall(packageType, packageName, installedAppBundle string, before map[string]string) (string, string, string, error) {
	if packageType == "dmg" {
		bundleName, err := findInstalledAppBundleName(packageName, installedAppBundle)
		if err != nil {
			return "", "", "", err
		}
		bundlePath := filepath.Join(applicationsDir, bundleName)
		resolvedPath, err := resolveAppBundleExecutable(bundlePath)
		if err != nil {
			return "", "", "", err
		}
		return resolvedPath, strings.TrimSuffix(bundleName, ".app"), bundleName, nil
	}

	resolvedPath, trackedName, err := resolveTrackedSystemCommand(packageName, before)
	if err != nil {
		return "", "", "", err
	}
	return resolvedPath, trackedName, "", nil
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

func installDMGApp(packagePath string) (string, error) {
	mountPoint, err := os.MkdirTemp("", "bin-dmg-mount-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(mountPoint)

	out, err := execCommand("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountPoint, packagePath).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to mount dmg: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	defer detachDMG(mountPoint)

	bundlePath, err := findSingleAppBundle(mountPoint)
	if err != nil {
		return "", err
	}

	bundleName := filepath.Base(bundlePath)
	targetPath := filepath.Join(applicationsDir, bundleName)
	out, err = execCommand("ditto", bundlePath, targetPath).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to install app bundle %s: %v (%s)", bundleName, err, strings.TrimSpace(string(out)))
	}

	if _, err := os.Stat(targetPath); err != nil {
		return "", fmt.Errorf("app bundle %s was not installed to %s", bundleName, applicationsDir)
	}

	return bundleName, nil
}

func detachDMG(mountPoint string) {
	out, err := execCommand("hdiutil", "detach", mountPoint).CombinedOutput()
	if err != nil {
		log.Warnf("failed to detach dmg at %s: %v (%s)", mountPoint, err, strings.TrimSpace(string(out)))
	}
}

func findSingleAppBundle(root string) (string, error) {
	var bundles []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".app") {
			bundles = append(bundles, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(bundles) == 0 {
		return "", fmt.Errorf("dmg did not contain an app bundle")
	}
	if len(bundles) > 1 {
		return "", fmt.Errorf("dmg contained multiple app bundles (%s)", strings.Join(appBundleBaseNames(bundles), ", "))
	}
	return bundles[0], nil
}

func appBundleBaseNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)
	return names
}

func findInstalledAppBundleName(expectedName, installedAppBundle string) (string, error) {
	if installedAppBundle != "" {
		if expectedName != "" && !strings.EqualFold(strings.TrimSuffix(installedAppBundle, ".app"), expectedName) {
			return "", fmt.Errorf("installed app bundle %q did not match requested app name %q", strings.TrimSuffix(installedAppBundle, ".app"), expectedName)
		}
		return installedAppBundle, nil
	}

	if expectedName != "" {
		bundleName := expectedName + ".app"
		if _, err := os.Stat(filepath.Join(applicationsDir, bundleName)); err == nil {
			return bundleName, nil
		}
		return "", fmt.Errorf("app bundle %q was not found in %s", expectedName+".app", applicationsDir)
	}

	return "", fmt.Errorf("missing installed app bundle metadata")
}

func resolveAppBundleExecutable(appPath string) (string, error) {
	execDir := filepath.Join(appPath, "Contents", "MacOS")
	entries, err := os.ReadDir(execDir)
	if err != nil {
		return "", fmt.Errorf("failed to inspect app executable directory %s: %w", execDir, err)
	}

	appName := strings.TrimSuffix(filepath.Base(appPath), ".app")
	var executables []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(execDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		if strings.EqualFold(entry.Name(), appName) {
			return fullPath, nil
		}
		executables = append(executables, fullPath)
	}

	if len(executables) == 1 {
		return executables[0], nil
	}
	if len(executables) == 0 {
		return "", fmt.Errorf("app bundle %s does not contain an executable in Contents/MacOS", filepath.Base(appPath))
	}

	return "", fmt.Errorf("app bundle %s contains multiple executables in Contents/MacOS", filepath.Base(appPath))
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
	packageType := systempackage.NormalizeType(b.PackageType)
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
	if packageType == "dmg" {
		if err := os.RemoveAll(packageID); err != nil {
			return fmt.Errorf("failed to remove app bundle %q: %w", packageID, err)
		}
		return nil
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
	case "dmg":
		if b.AppBundle == "" {
			return "", fmt.Errorf("missing app bundle metadata for %s", b.Path)
		}
		return filepath.Join(applicationsDir, b.AppBundle), nil
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
