package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

type testFetchProvider struct {
	file *providers.File
}

func (p testFetchProvider) Fetch(*providers.FetchOpts) (*providers.File, error) { return p.file, nil }
func (p testFetchProvider) GetLatestVersion() (*providers.ReleaseInfo, error)   { return nil, nil }
func (p testFetchProvider) Cleanup(*providers.CleanupOpts) error                { return nil }
func (p testFetchProvider) GetID() string                                       { return "github" }

func TestSystemPackagePathLooksExplicitForEitherSeparator(t *testing.T) {
	for _, path := range []string{"bin/tool", `bin\tool`} {
		if !systemPackagePathLooksExplicit(path) {
			t.Errorf("systemPackagePathLooksExplicit(%q) = false, want true", path)
		}
	}
}

func TestResolveAppBundleExecutablePrefersBundleName(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "Paseo.app")
	execDir := filepath.Join(appPath, "Contents", "MacOS")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("mkdir exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "helper"), []byte("helper"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	mainExec := filepath.Join(execDir, "Paseo")
	if err := os.WriteFile(mainExec, []byte("main"), 0o755); err != nil {
		t.Fatalf("write main executable: %v", err)
	}

	resolved, err := resolveAppBundleExecutable(appPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != mainExec {
		t.Fatalf("unexpected executable path: got %s want %s", resolved, mainExec)
	}
}

func TestResolveDMGAppBundle(t *testing.T) {
	tests := []struct {
		name              string
		bundles           []string
		requestedIdentity string
		storedIdentity    string
		legacyName        string
		wantBundle        string
		wantError         string
	}{
		{
			name:              "requested identity takes precedence",
			bundles:           []string{"Fastpotify.app", "Spotify.app"},
			requestedIdentity: "spotify",
			storedIdentity:    "Fastpotify.app",
			legacyName:        "Fastpotify",
			wantBundle:        "Spotify.app",
		},
		{
			name:           "stored identity takes precedence over legacy name",
			bundles:        []string{"Fastpotify.app", "Spotify.app"},
			storedIdentity: "Spotify.app",
			legacyName:     "Fastpotify",
			wantBundle:     "Spotify.app",
		},
		{
			name:       "legacy name selects matching bundle",
			bundles:    []string{"Fastpotify.app", "Spotify.app"},
			legacyName: "Fastpotify",
			wantBundle: "Fastpotify.app",
		},
		{
			name:       "unique fallback without identity",
			bundles:    []string{"Fastpotify.app"},
			wantBundle: "Fastpotify.app",
		},
		{
			name:              "identity suffix and case are normalized",
			bundles:           []string{"Fastpotify.APP"},
			requestedIdentity: "  fAsTpOtIfY.aPp ",
			wantBundle:        "Fastpotify.APP",
		},
		{
			name:              "missing named identity does not fall back",
			bundles:           []string{"Fastpotify.app"},
			requestedIdentity: "Spotify.app",
			wantError:         "did not contain requested app bundle",
		},
		{
			name:           "missing stored identity does not fall back to legacy name",
			bundles:        []string{"Fastpotify.app"},
			storedIdentity: "Spotify.app",
			legacyName:     "Fastpotify",
			wantError:      "did not contain stored app bundle",
		},
		{
			name:      "multiple candidates without identity are ambiguous",
			bundles:   []string{"Fastpotify.app", "Spotify.app"},
			wantError: "multiple eligible app bundles",
		},
		{
			name:       "embedded helper apps are not candidates",
			bundles:    []string{"Fastpotify.app", "Fastpotify.app/Contents/Library/LoginItems/Helper.app"},
			wantBundle: "Fastpotify.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, bundle := range tt.bundles {
				if err := os.MkdirAll(filepath.Join(root, bundle), 0o755); err != nil {
					t.Fatalf("mkdir bundle %s: %v", bundle, err)
				}
			}

			got, err := resolveDMGAppBundle(root, tt.requestedIdentity, tt.storedIdentity, tt.legacyName)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("resolveDMGAppBundle() error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDMGAppBundle() error = %v", err)
			}
			want := filepath.Join(root, tt.wantBundle)
			if got != want {
				t.Fatalf("resolveDMGAppBundle() = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveDMGAppBundlePreservesCallerRootPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink path spelling regression is Unix-specific")
	}
	canonicalRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(canonicalRoot, "Fastpotify.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	aliasRoot := filepath.Join(t.TempDir(), "mount-alias")
	if err := os.Symlink(canonicalRoot, aliasRoot); err != nil {
		t.Fatal(err)
	}

	got, err := resolveDMGAppBundle(aliasRoot, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(aliasRoot, "Fastpotify.app"); got != want {
		t.Fatalf("resolveDMGAppBundle() = %q, want caller-rooted path %q", got, want)
	}
}

func TestValidateUniqueDMGAppBundleIdentitiesRejectsCaseCollisions(t *testing.T) {
	candidates := []dmgAppBundleCandidate{
		{name: "Fastpotify.app", identity: "fastpotify"},
		{name: "fastpotify.APP", identity: "fastpotify"},
	}
	if err := validateUniqueDMGAppBundleIdentities(candidates); err == nil || !strings.Contains(err.Error(), "colliding app bundle identities") {
		t.Fatalf("validateUniqueDMGAppBundleIdentities() error = %v, want collision", err)
	}
}

func TestResolveDMGAppBundleIgnoresSymlinkedBundleOutsideMount(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "Fastpotify.app")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside bundle: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "Fastpotify.app")); err != nil {
		t.Fatalf("symlink outside bundle: %v", err)
	}

	_, err := resolveDMGAppBundle(root, "", "", "")
	if err == nil || !strings.Contains(err.Error(), "did not contain an eligible top-level app bundle") {
		t.Fatalf("resolveDMGAppBundle() error = %v, want rejected outside symlink", err)
	}
}

func TestOfferToSignUnsignedAppWithConsent(t *testing.T) {
	originalExec := execCommand
	originalInteractive := isPromptInteractive
	originalConfirm := confirmDefaultNoPrompt
	defer func() {
		execCommand = originalExec
		isPromptInteractive = originalInteractive
		confirmDefaultNoPrompt = originalConfirm
	}()

	isPromptInteractive = func() bool { return true }
	confirmDefaultNoPrompt = func(string) error { return nil }
	var signed bool
	execCommand = func(name string, args ...string) *exec.Cmd {
		exitCode := 0
		if len(args) > 0 && args[0] == "--verify" {
			exitCode = 1
		} else if name == "codesign" {
			signed = true
		}
		return helperExecCommand(t, exitCode, nil)(name, args...)
	}

	if err := offerToSignUnsignedApp("/Applications/Test.app", false); err != nil {
		t.Fatal(err)
	}
	if !signed {
		t.Fatal("expected unsigned app to be signed")
	}
}

func TestUninstallSystemPackageRemovesDMGAppBundle(t *testing.T) {
	originalApplicationsDir := applicationsDir
	applicationsDir = t.TempDir()
	defer func() {
		applicationsDir = originalApplicationsDir
	}()

	bundlePath := filepath.Join(applicationsDir, "Paseo.app")
	if err := os.MkdirAll(bundlePath, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}

	err := uninstallSystemPackage(&config.Binary{
		Path:        filepath.Join(bundlePath, "Contents", "MacOS", "Paseo"),
		InstallMode: installModeSystemPackage,
		PackageType: "dmg",
		AppBundle:   "Paseo.app",
	})
	if err != nil {
		t.Fatalf("unexpected uninstall error: %v", err)
	}
	if _, err := os.Stat(bundlePath); !os.IsNotExist(err) {
		t.Fatalf("expected app bundle to be removed, stat err=%v", err)
	}
}

func TestUninstallSystemPackageDoesNotRemoveMismatchedDMGBundle(t *testing.T) {
	originalApplicationsDir := applicationsDir
	applicationsDir = t.TempDir()
	t.Cleanup(func() { applicationsDir = originalApplicationsDir })

	fastpotify := filepath.Join(applicationsDir, "Fastpotify.app")
	spotifast := filepath.Join(applicationsDir, "Spotifast.app")
	for _, bundle := range []string{fastpotify, spotifast} {
		if err := os.MkdirAll(bundle, 0o755); err != nil {
			t.Fatalf("mkdir bundle: %v", err)
		}
	}

	err := uninstallSystemPackage(&config.Binary{
		Path:        filepath.Join(fastpotify, "Contents", "MacOS", "Fastpotify-bin"),
		InstallMode: installModeSystemPackage,
		PackageType: "dmg",
		AppBundle:   "Spotifast.app",
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("uninstallSystemPackage() error = %v, want path/bundle mismatch", err)
	}
	for _, bundle := range []string{fastpotify, spotifast} {
		if _, err := os.Stat(bundle); err != nil {
			t.Fatalf("unexpected bundle removal for %s: %v", bundle, err)
		}
	}
}

func TestUninstallSystemPackageRejectsMalformedDMGBundleMetadata(t *testing.T) {
	tests := []struct {
		name       string
		bundleName string
		outside    bool
	}{
		{name: "parent component", bundleName: "..", outside: true},
		{name: "separator", bundleName: "../outside.app", outside: true},
		{name: "backslash separator", bundleName: `..\outside.app`, outside: true},
		{name: "absolute", bundleName: "/outside.app", outside: true},
		{name: "invalid suffix", bundleName: "outside", outside: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			originalApplicationsDir := applicationsDir
			applicationsDir = filepath.Join(root, "Applications")
			t.Cleanup(func() { applicationsDir = originalApplicationsDir })

			protectedPath := filepath.Join(applicationsDir, "outside")
			if tt.outside {
				protectedPath = filepath.Join(root, "outside.app")
			}
			if err := os.MkdirAll(filepath.Join(protectedPath, "Contents", "MacOS"), 0o755); err != nil {
				t.Fatalf("create protected bundle: %v", err)
			}

			err := uninstallSystemPackage(&config.Binary{
				Path:        filepath.Join(protectedPath, "Contents", "MacOS", "tool"),
				InstallMode: installModeSystemPackage,
				PackageType: "dmg",
				AppBundle:   tt.bundleName,
			})
			if err == nil || !strings.Contains(err.Error(), "invalid app bundle metadata") {
				t.Fatalf("uninstallSystemPackage() error = %v, want invalid metadata", err)
			}
			if _, err := os.Stat(protectedPath); err != nil {
				t.Fatalf("malformed metadata removed protected path %s: %v", protectedPath, err)
			}
		})
	}
}

func TestFindManagedBinByAliasMatchesAppBundleName(t *testing.T) {
	bins := map[string]*config.Binary{
		"/Applications/Paseo.app/Contents/MacOS/Paseo": {
			Path:        "/Applications/Paseo.app/Contents/MacOS/Paseo",
			RemoteName:  "Paseo",
			InstallMode: installModeSystemPackage,
			PackageType: "dmg",
			AppBundle:   "Paseo.app",
		},
	}

	resolved, err := findManagedBinByAlias(bins, "Paseo")
	if err != nil {
		t.Fatalf("alias lookup error: %v", err)
	}
	if resolved != "/Applications/Paseo.app/Contents/MacOS/Paseo" {
		t.Fatalf("alias lookup = %q, want persisted executable path", resolved)
	}
}

func TestInstallSystemPackageDMGTracksInstalledAppBundle(t *testing.T) {
	setupTestConfig(t)

	originalApplicationsDir := applicationsDir
	originalExec := execCommand
	originalProviderFactory := installProviderFactory
	applicationsDir = t.TempDir()
	defer func() {
		applicationsDir = originalApplicationsDir
		execCommand = originalExec
		installProviderFactory = originalProviderFactory
	}()

	stream := &trackingReadCloser{reader: bytes.NewReader([]byte("fake dmg"))}
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "test", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if opts.BundledCompletionShell != "" || opts.BundledCompletionCommand != "" {
				t.Fatalf("system package requested managed completion: (%q, %q)", opts.BundledCompletionShell, opts.BundledCompletionCommand)
			}
			return &providers.File{
				Data:        stream,
				Name:        "Paseo-0.1.64-arm64.dmg",
				Version:     "0.1.64",
				PackagePath: "Paseo.app",
				DownloadIntegrity: &providers.IntegrityRecord{
					Algorithm: "sha256", Expected: "release-digest", Observed: "release-digest",
					Source: "Paseo-0.1.64-arm64.dmg.sha256", Scope: "download", Result: "verified",
				},
			}, nil
		}}, nil
	}

	execCommand = helperExecCommand(t, 0, func(name string, args []string) {
		switch {
		case name == "hdiutil" && len(args) >= 6 && args[0] == "attach":
			mountPoint := args[4]
			execDir := filepath.Join(mountPoint, "Paseo.app", "Contents", "MacOS")
			if err := os.MkdirAll(execDir, 0o755); err != nil {
				t.Fatalf("mkdir exec dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(execDir, "Paseo"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatalf("write app executable: %v", err)
			}
			siblingExecDir := filepath.Join(mountPoint, "Paseo Helper.app", "Contents", "MacOS")
			if err := os.MkdirAll(siblingExecDir, 0o755); err != nil {
				t.Fatalf("mkdir sibling exec dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(siblingExecDir, "Paseo Helper"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatalf("write sibling executable: %v", err)
			}
		case name == "ditto" && len(args) == 2:
			if err := copyDir(args[0], args[1]); err != nil {
				t.Fatalf("copy bundle: %v", err)
			}
		}
	})

	shell := "bash"
	res, err := installSystemPackage(InstallOpts{
		URL:                "https://github.com/getpaseo/paseo/releases/tag/v0.1.64",
		RequestedAppBundle: "Paseo",
		CompletionShell:    &shell,
		FetchOpts: providers.FetchOpts{
			SystemPackage:            true,
			PackageType:              "dmg",
			PackageName:              "Paseo",
			BundledCompletionShell:   "fish",
			BundledCompletionCommand: "Paseo",
		},
	})
	if err != nil {
		t.Fatalf("unexpected install error: %v", err)
	}
	if res.Name != "Paseo" {
		t.Fatalf("unexpected install result name: %s", res.Name)
	}
	if res.Path == "" {
		t.Fatal("expected tracked path")
	}

	binCfg, ok := config.Get().Bins[res.Path]
	if !ok {
		t.Fatalf("expected config entry for %s", res.Path)
	}
	if binCfg.AppBundle != "Paseo.app" {
		t.Fatalf("unexpected app bundle: %s", binCfg.AppBundle)
	}
	if binCfg.PackageType != "dmg" {
		t.Fatalf("unexpected package type: %s", binCfg.PackageType)
	}
	if binCfg.DownloadIntegrity == nil || binCfg.DownloadIntegrity.Algorithm != "sha256" || binCfg.DownloadIntegrity.Expected != "release-digest" || binCfg.DownloadIntegrity.Observed != "release-digest" || binCfg.DownloadIntegrity.Source != "Paseo-0.1.64-arm64.dmg.sha256" || binCfg.DownloadIntegrity.Scope != "download" || binCfg.DownloadIntegrity.Result != "verified" {
		t.Fatalf("download integrity was not persisted: %#v", binCfg.DownloadIntegrity)
	}
	if binCfg.InstalledIntegrity != nil {
		t.Fatalf("package-manager install asserted installed-byte integrity: %#v", binCfg.InstalledIntegrity)
	}
	if binCfg.CompletionShell != nil {
		t.Fatalf("system package persisted completion policy: %#v", binCfg)
	}
	if !strings.HasSuffix(binCfg.Path, "/Paseo.app/Contents/MacOS/Paseo") {
		t.Fatalf("unexpected tracked path: %s", binCfg.Path)
	}
	if _, err := os.Stat(filepath.Join(applicationsDir, "Paseo.app", "Contents", "MacOS", "Paseo")); err != nil {
		t.Fatalf("expected installed app executable: %v", err)
	}
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream closed %d times, want 1", stream.closeCount)
	}
}

func TestInstallSystemPackageDMGReinstallUsesStoredBundleOverSibling(t *testing.T) {
	setupTestConfig(t)
	originalApplicationsDir := applicationsDir
	originalExec := execCommand
	originalProviderFactory := installProviderFactory
	applicationsDir = t.TempDir()
	t.Cleanup(func() {
		applicationsDir = originalApplicationsDir
		execCommand = originalExec
		installProviderFactory = originalProviderFactory
	})

	trackedPath := filepath.Join(applicationsDir, "Fastpotify.app", "Contents", "MacOS", "Fastpotify-bin")
	shell := "bash"
	if err := config.UpsertBinary(&config.Binary{
		Path: trackedPath, RemoteName: "spotify", Version: "1.0.0", Hash: "old", URL: "https://example.test/spotify", Provider: "github",
		InstallMode: installModeSystemPackage, PackageType: "dmg", AppBundle: "Fastpotify.app", PackagePath: "spotify.dmg", CompletionShell: &shell,
	}); err != nil {
		t.Fatal(err)
	}

	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "github", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if opts.PackageName != "spotify" {
				t.Fatalf("provider product = %q, want spotify", opts.PackageName)
			}
			return &providers.File{Data: bytes.NewReader([]byte("fake dmg")), Name: "spotify.dmg", Version: "2.0.0", PackagePath: "spotify.dmg"}, nil
		}}, nil
	}

	siblingPath := filepath.Join(applicationsDir, "Spotifast.app")
	sentinel := filepath.Join(siblingPath, "sentinel")
	if err := os.MkdirAll(siblingPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	var copiedBundle string
	execCommand = helperExecCommand(t, 0, func(name string, args []string) {
		switch {
		case name == "hdiutil" && len(args) >= 6 && args[0] == "attach":
			mountPoint := args[4]
			for _, app := range []struct{ name, executable string }{{"Fastpotify.app", "Fastpotify-bin"}, {"Spotifast.app", "Spotifast"}} {
				execDir := filepath.Join(mountPoint, app.name, "Contents", "MacOS")
				if err := os.MkdirAll(execDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(execDir, app.executable), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
		case name == "ditto" && len(args) == 2:
			copiedBundle = filepath.Base(args[0])
			if err := copyDir(args[0], args[1]); err != nil {
				t.Fatal(err)
			}
		}
	})

	res, err := installSystemPackage(InstallOpts{
		URL: "https://example.test/spotify", Path: trackedPath, ConfigPath: trackedPath, Force: true,
		LogicalName: "spotify", AppBundle: "Fastpotify.app",
		FetchOpts: providers.FetchOpts{SystemPackage: true, PackageType: "dmg", PackageName: "spotify"},
	})
	if err != nil {
		t.Fatalf("reinstall error: %v", err)
	}
	if copiedBundle != "Fastpotify.app" || res.Path != trackedPath {
		t.Fatalf("reinstall copied %q to %q, want Fastpotify at persisted path", copiedBundle, res.Path)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("reinstall touched sibling app: %v", err)
	}
	if persisted := config.Get().Bins[trackedPath]; persisted.CompletionShell == nil || *persisted.CompletionShell != shell {
		t.Fatalf("system package reinstall did not preserve local completion state: %#v", persisted)
	}
}

func TestInstallDMGAppRejectsInvalidSourceBeforeCopyOrSigning(t *testing.T) {
	originalApplicationsDir := applicationsDir
	originalExec := execCommand
	applicationsDir = t.TempDir()
	t.Cleanup(func() {
		applicationsDir = originalApplicationsDir
		execCommand = originalExec
	})

	existingBundle := filepath.Join(applicationsDir, "Fastpotify.app")
	if err := os.MkdirAll(existingBundle, 0o755); err != nil {
		t.Fatalf("create existing bundle: %v", err)
	}
	sentinel := filepath.Join(existingBundle, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	var commands []string
	execCommand = helperExecCommand(t, 0, func(name string, args []string) {
		commands = append(commands, name)
		if name != "hdiutil" || len(args) < 6 || args[0] != "attach" {
			return
		}
		mountPoint := args[4]
		execDir := filepath.Join(mountPoint, "Fastpotify.app", "Contents", "MacOS")
		if err := os.MkdirAll(execDir, 0o755); err != nil {
			t.Fatalf("create source bundle: %v", err)
		}
		if err := os.WriteFile(filepath.Join(execDir, "Fastpotify"), []byte("not runnable"), 0o755); err != nil {
			t.Fatalf("create invalid source executable: %v", err)
		}
	})

	_, err := installDMGApp("fixture.dmg", true, "Fastpotify", "", "")
	if err == nil || !strings.Contains(err.Error(), "invalid executable") {
		t.Fatalf("installDMGApp() error = %v, want invalid source executable error", err)
	}
	if strings.Contains(strings.Join(commands, ","), "ditto") || strings.Contains(strings.Join(commands, ","), "codesign") {
		t.Fatalf("invalid source invoked copy or signing commands: %v", commands)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("existing target was changed: %v", err)
	}
}

func TestInstallSystemPackageClosesFetchedStreamBeforeStaging(t *testing.T) {
	setupTestConfig(t)
	stream := &trackingReadCloser{reader: strings.NewReader("package")}
	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return testFetchProvider{file: &providers.File{Name: "unsupported.txt", Version: "1.0.0", Data: stream}}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	_, err := installSystemPackage(InstallOpts{URL: "https://example.test/package"})
	if err == nil {
		t.Fatal("expected unsupported package failure")
	}
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream closed %d times, want 1", stream.closeCount)
	}
}

func TestWritePackageArtifactToTempRemovesArtifactAfterCopyFailure(t *testing.T) {
	before, err := filepath.Glob(filepath.Join(os.TempDir(), "bin-system-package-*"))
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]bool, len(before))
	for _, path := range before {
		known[path] = true
	}

	stream := &trackingReadCloser{reader: failingReader{err: os.ErrClosed}}
	_, err = writePackageArtifactToTemp("tool.deb", stream)
	if err == nil {
		t.Fatal("expected copy failure")
	}
	if stream.closeCount != 1 {
		t.Fatalf("source stream closed %d times, want 1", stream.closeCount)
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), "bin-system-package-*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range after {
		if !known[path] {
			t.Fatalf("temporary package artifact was not removed: %s", path)
		}
	}
}

func TestWritePackageArtifactToTempRemovesArtifactAfterSourceCloseFailure(t *testing.T) {
	before, err := filepath.Glob(filepath.Join(os.TempDir(), "bin-system-package-*"))
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]bool, len(before))
	for _, path := range before {
		known[path] = true
	}

	closeErr := os.ErrClosed
	stream := &trackingReadCloser{reader: strings.NewReader("package"), closeErr: closeErr}
	path, err := writePackageArtifactToTemp("tool.deb", stream)
	if !errors.Is(err, closeErr) {
		t.Fatalf("writePackageArtifactToTemp() error = %v, want %v", err, closeErr)
	}
	if path != "" {
		t.Fatalf("writePackageArtifactToTemp() path = %q, want empty", path)
	}
	if stream.closeCount != 1 {
		t.Fatalf("source stream closed %d times, want 1", stream.closeCount)
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), "bin-system-package-*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tempPath := range after {
		if !known[tempPath] {
			t.Fatalf("temporary package artifact was not removed: %s", tempPath)
		}
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
}
