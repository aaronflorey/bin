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

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/systempackage"
	"github.com/caarlos0/log"
)

var execCommand = exec.Command
var lookPathCommand = exec.LookPath
var applicationsDir = "/Applications"

func installSystemPackage(opts InstallOpts) (*InstallResult, error) {
	// System packages never participate in managed completion fetching or
	// generation, even if an internal caller populated these direct-binary
	// fields.
	opts.FetchOpts.BundledCompletionShell = ""
	opts.FetchOpts.BundledCompletionCommand = ""
	p, pResult, err := fetchBinary(installProviderFactory, opts.URL, opts.Provider, opts.FetchOpts, opts.AllowProviderFallback)
	if err != nil {
		return nil, err
	}
	fetchedStreamOpen := true
	defer func() {
		if fetchedStreamOpen {
			_ = closeFetchedFile(pResult)
		}
	}()

	existing, minAgeDays, pinned := resolveInstallState(opts)
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

	fetchedStreamOpen = false // writePackageArtifactToTemp owns the stream from this point.
	artifactPath, err := writePackageArtifactToTemp(pResult.Name, pResult.Data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(artifactPath)

	var installedApp dmgInstalledApp
	if pkgType == "dmg" {
		installedApp, err = installDMGApp(artifactPath, opts.FetchOpts.NonInteractive, opts.RequestedAppBundle, opts.AppBundle, opts.LogicalName)
		if err != nil {
			return nil, err
		}
	} else if err := installPackageArtifact(pkgType, artifactPath); err != nil {
		return nil, err
	}

	resolvedPath, trackedName, appBundle, err := resolveTrackedSystemInstall(pkgType, opts.FetchOpts.PackageName, installedApp, before)
	if err != nil {
		return nil, err
	}
	if opts.LogicalName != "" {
		trackedName = opts.LogicalName
	}

	hashString, err := hashExecutableFile(resolvedPath)
	if err != nil {
		return nil, err
	}

	configPath, err := resolveTrackedConfigPath(opts, resolvedPath)
	if err != nil {
		return nil, err
	}

	if err := persistInstalledBinary(&config.Binary{
		RemoteName:        trackedName,
		Path:              configPath,
		Version:           pResult.Version,
		Hash:              hashString,
		URL:               opts.URL,
		Provider:          p.GetID(),
		InstallMode:       installModeSystemPackage,
		PackageType:       pkgType,
		AppBundle:         appBundle,
		PackagePath:       pResult.PackagePath,
		SourceAsset:       pResult.SourceAsset,
		SelectionIntent:   installedSelectionIntent(pResult, opts.FetchOpts, existing),
		ReleaseTagPrefix:  pResult.ReleaseTagPrefix,
		DownloadIntegrity: configIntegrityRecord(pResult.DownloadIntegrity),
		Pinned:            pinned,
		MinAgeDays:        minAgeDays,
	}); err != nil {
		return nil, err
	}

	return &InstallResult{Name: trackedName, Version: pResult.Version, Path: configPath}, nil
}

func resolveTrackedSystemInstall(packageType, packageName string, installedApp dmgInstalledApp, before map[string]string) (string, string, string, error) {
	if packageType == "dmg" {
		if installedApp.bundleName == "" || installedApp.executablePath == "" {
			return "", "", "", fmt.Errorf("missing installed app bundle metadata")
		}
		return installedApp.executablePath, strings.TrimSuffix(installedApp.bundleName, ".app"), installedApp.bundleName, nil
	}

	resolvedPath, trackedName, err := resolveTrackedSystemCommand(packageName, before)
	if err != nil {
		return "", "", "", err
	}
	return resolvedPath, trackedName, "", nil
}

func writePackageArtifactToTemp(name string, src io.Reader) (path string, err error) {
	ext := filepath.Ext(strings.ToLower(name))
	if strings.HasSuffix(strings.ToLower(name), ".pkg.tar.zst") {
		ext = ".pkg.tar.zst"
	}
	if strings.HasSuffix(strings.ToLower(name), ".flatpack") {
		ext = ".flatpak"
	}

	var source io.Closer
	if closer, ok := src.(io.Closer); ok {
		source = closer
	}

	var f *os.File
	defer func() {
		if f != nil {
			_ = f.Close()
		}
		if source != nil {
			if closeErr := source.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		if err != nil && path != "" {
			_ = os.Remove(path)
			path = ""
		}
	}()

	f, err = os.CreateTemp("", "bin-system-package-*"+ext)
	if err != nil {
		return "", err
	}
	path = f.Name()

	if _, err = io.Copy(f, src); err != nil {
		return
	}
	closeErr := f.Close()
	f = nil
	if closeErr != nil {
		err = closeErr
		return
	}

	return path, nil
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

type dmgInstalledApp struct {
	bundleName     string
	executablePath string
}

func installDMGApp(packagePath string, nonInteractive bool, requestedIdentity, storedIdentity, legacyName string) (dmgInstalledApp, error) {
	mountPoint, err := os.MkdirTemp("", "bin-dmg-mount-*")
	if err != nil {
		return dmgInstalledApp{}, err
	}
	defer os.RemoveAll(mountPoint)

	out, err := execCommand("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountPoint, packagePath).CombinedOutput()
	if err != nil {
		return dmgInstalledApp{}, fmt.Errorf("failed to mount dmg: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	defer detachDMG(mountPoint)

	bundlePath, err := resolveDMGAppBundle(mountPoint, requestedIdentity, storedIdentity, legacyName)
	if err != nil {
		return dmgInstalledApp{}, err
	}
	sourceExecutable, err := resolveDMGSourceExecutable(mountPoint, bundlePath)
	if err != nil {
		return dmgInstalledApp{}, err
	}

	bundleName := filepath.Base(bundlePath)
	targetPath := filepath.Join(applicationsDir, bundleName)
	relativeExecutable, err := filepath.Rel(bundlePath, sourceExecutable)
	if err != nil {
		return dmgInstalledApp{}, fmt.Errorf("resolve app executable path: %w", err)
	}
	targetExecutable := filepath.Join(targetPath, relativeExecutable)
	out, err = execCommand("ditto", bundlePath, targetPath).CombinedOutput()
	if err != nil {
		return dmgInstalledApp{}, fmt.Errorf("failed to install app bundle %s: %v (%s)", bundleName, err, strings.TrimSpace(string(out)))
	}

	if _, err := os.Stat(targetPath); err != nil {
		return dmgInstalledApp{}, fmt.Errorf("app bundle %s was not installed to %s", bundleName, applicationsDir)
	}
	if err := offerToSignUnsignedApp(targetPath, nonInteractive); err != nil {
		return dmgInstalledApp{}, err
	}

	return dmgInstalledApp{bundleName: bundleName, executablePath: targetExecutable}, nil
}

// resolveDMGSourceExecutable validates the executable before any copy or
// signing side effect. Both the bundle and executable must remain inside the
// mounted image and selected bundle after symlink resolution.
func resolveDMGSourceExecutable(mountPoint, bundlePath string) (string, error) {
	canonicalMount, err := filepath.EvalSymlinks(mountPoint)
	if err != nil {
		return "", fmt.Errorf("resolve dmg mount root: %w", err)
	}
	canonicalBundle, err := filepath.EvalSymlinks(bundlePath)
	if err != nil {
		return "", fmt.Errorf("resolve dmg app bundle: %w", err)
	}
	if !pathWithin(canonicalMount, canonicalBundle) {
		return "", fmt.Errorf("dmg app bundle %q escapes mount root", filepath.Base(bundlePath))
	}
	executable, err := resolveAppBundleExecutable(canonicalBundle)
	if err != nil {
		return "", err
	}
	canonicalExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve app executable %q: %w", filepath.Base(executable), err)
	}
	if !pathWithin(canonicalBundle, canonicalExecutable) {
		return "", fmt.Errorf("app bundle %q executable escapes bundle", filepath.Base(bundlePath))
	}
	if err := assets.ValidateRunnablePayload(canonicalExecutable, filepath.Base(canonicalExecutable)); err != nil {
		return "", fmt.Errorf("app bundle %q contains an invalid executable: %w", filepath.Base(bundlePath), err)
	}
	relativeExecutable, err := filepath.Rel(canonicalBundle, canonicalExecutable)
	if err != nil {
		return "", fmt.Errorf("resolve app executable path: %w", err)
	}
	return filepath.Join(bundlePath, relativeExecutable), nil
}

func offerToSignUnsignedApp(appPath string, nonInteractive bool) error {
	if err := execCommand("codesign", "--verify", "--deep", "--strict", appPath).Run(); err == nil {
		return nil
	}

	if nonInteractive || !isPromptInteractive() {
		log.Warnf("Installed app %s is unsigned; leaving it unsigned because prompts are disabled", filepath.Base(appPath))
		return nil
	}
	if err := confirmDefaultNoPrompt(fmt.Sprintf("%s is unsigned. Trust it by applying an ad-hoc signature?", filepath.Base(appPath))); err != nil {
		if err.Error() == "command aborted" {
			log.Warnf("Installed app %s remains unsigned", filepath.Base(appPath))
			return nil
		}
		return err
	}

	out, err := execCommand("codesign", "--force", "--sign", "-", "--deep", appPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to sign app bundle %s: %v (%s)", filepath.Base(appPath), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func detachDMG(mountPoint string) {
	out, err := execCommand("hdiutil", "detach", mountPoint).CombinedOutput()
	if err != nil {
		log.Warnf("failed to detach dmg at %s: %v (%s)", mountPoint, err, strings.TrimSpace(string(out)))
	}
}

type dmgAppBundleCandidate struct {
	path     string
	name     string
	identity string
}

// resolveDMGAppBundle selects a top-level application bundle from a mounted image.
// A managed identity takes precedence over the legacy name and an unnamed image may
// only be used when it has one eligible application bundle.
func resolveDMGAppBundle(root, requestedIdentity, storedIdentity, legacyName string) (string, error) {
	candidates, err := eligibleDMGAppBundles(root)
	if err != nil {
		return "", err
	}

	identity, source, err := firstDMGAppIdentity(requestedIdentity, storedIdentity, legacyName)
	if err != nil {
		return "", err
	}
	if identity == "" {
		if len(candidates) == 0 {
			return "", fmt.Errorf("dmg did not contain an eligible top-level app bundle")
		}
		if len(candidates) > 1 {
			return "", fmt.Errorf("dmg contained multiple eligible app bundles (%s)", dmgAppBundleNames(candidates))
		}
		return candidates[0].path, nil
	}

	for _, candidate := range candidates {
		if candidate.identity == identity {
			return candidate.path, nil
		}
	}
	return "", fmt.Errorf("dmg did not contain %s app bundle %q", source, identity)
}

func eligibleDMGAppBundles(root string) ([]dmgAppBundleCandidate, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve dmg mount root: %w", err)
	}
	entries, err := os.ReadDir(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("read dmg mount root: %w", err)
	}

	candidates := make([]dmgAppBundleCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
			continue
		}
		sourcePath := filepath.Join(root, entry.Name())
		candidatePath, err := filepath.EvalSymlinks(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("resolve dmg app bundle %q: %w", entry.Name(), err)
		}
		if !pathWithin(canonicalRoot, candidatePath) {
			return nil, fmt.Errorf("dmg app bundle %q escapes mount root", entry.Name())
		}
		identity, err := normalizedDMGAppIdentity(entry.Name())
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, dmgAppBundleCandidate{path: sourcePath, name: entry.Name(), identity: identity})
	}

	if err := validateUniqueDMGAppBundleIdentities(candidates); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].name < candidates[j].name })
	return candidates, nil
}

func validateUniqueDMGAppBundleIdentities(candidates []dmgAppBundleCandidate) error {
	byIdentity := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		if existing, ok := byIdentity[candidate.identity]; ok {
			names := []string{existing, candidate.name}
			sort.Strings(names)
			return fmt.Errorf("dmg contained colliding app bundle identities (%s)", strings.Join(names, ", "))
		}
		byIdentity[candidate.identity] = candidate.name
	}
	return nil
}

func firstDMGAppIdentity(requestedIdentity, storedIdentity, legacyName string) (string, string, error) {
	for _, selection := range []struct {
		value  string
		source string
	}{
		{requestedIdentity, "requested"},
		{storedIdentity, "stored"},
		{legacyName, "legacy"},
	} {
		if strings.TrimSpace(selection.value) == "" {
			continue
		}
		identity, err := normalizedDMGAppIdentity(selection.value)
		if err != nil {
			return "", "", fmt.Errorf("invalid %s app bundle identity: %w", selection.source, err)
		}
		return identity, selection.source, nil
	}
	return "", "", nil
}

func normalizedDMGAppIdentity(name string) (string, error) {
	identity := strings.TrimSpace(name)
	if len(identity) >= len(".app") && strings.EqualFold(identity[len(identity)-len(".app"):], ".app") {
		identity = identity[:len(identity)-len(".app")]
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return "", fmt.Errorf("app bundle name is empty")
	}
	return strings.ToLower(identity), nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func dmgAppBundleNames(candidates []dmgAppBundleCandidate) string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.name)
	}
	return strings.Join(names, ", ")
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
	path := expandTrackedBinaryPath(b.Path)

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
		bundlePath, err := managedDMGBundlePath(b.AppBundle)
		if err != nil {
			return "", fmt.Errorf("%w for %s", err, b.Path)
		}
		if !pathWithin(bundlePath, path) {
			return "", fmt.Errorf("tracked executable %q does not belong to app bundle %q", b.Path, b.AppBundle)
		}
		return bundlePath, nil
	default:
		return "", fmt.Errorf("unsupported package type %q", packageType)
	}
}

func managedDMGBundlePath(bundleName string) (string, error) {
	trimmed := strings.TrimSpace(bundleName)
	if trimmed == "" {
		return "", fmt.Errorf("missing app bundle metadata")
	}
	if trimmed != bundleName || trimmed == "." || trimmed == ".." ||
		filepath.IsAbs(trimmed) || filepath.Base(trimmed) != trimmed ||
		strings.ContainsAny(trimmed, `/\\`) || !strings.HasSuffix(strings.ToLower(trimmed), ".app") {
		return "", fmt.Errorf("invalid app bundle metadata %q", bundleName)
	}
	return filepath.Join(applicationsDir, trimmed), nil
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
	if isExplicitTargetPath(trimmed) {
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
