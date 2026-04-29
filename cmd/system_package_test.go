package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestFindSingleAppBundleRejectsMultipleApps(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"One.app", "Two.app"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir bundle: %v", err)
		}
	}

	_, err := findSingleAppBundle(root)
	if err == nil {
		t.Fatal("expected multiple app bundles error")
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

	resolved := findManagedBinByAlias(bins, "Paseo")
	if resolved == "" {
		t.Fatal("expected alias lookup to resolve app bundle name")
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

	installProviderFactory = func(string, string) (providers.Provider, error) {
		return testFetchProvider{file: &providers.File{
			Data:        bytes.NewReader([]byte("fake dmg")),
			Name:        "Paseo-0.1.64-arm64.dmg",
			Version:     "0.1.64",
			PackagePath: "Paseo.app",
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
			if err := os.WriteFile(filepath.Join(execDir, "Paseo"), []byte("app"), 0o755); err != nil {
				t.Fatalf("write app executable: %v", err)
			}
		case name == "ditto" && len(args) == 2:
			if err := copyDir(args[0], args[1]); err != nil {
				t.Fatalf("copy bundle: %v", err)
			}
		}
	})

	res, err := installSystemPackage(InstallOpts{
		URL: "https://github.com/getpaseo/paseo/releases/tag/v0.1.64",
		FetchOpts: providers.FetchOpts{
			SystemPackage: true,
			PackageType:   "dmg",
			PackageName:   "Paseo",
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
	if !strings.HasSuffix(binCfg.Path, "/Paseo.app/Contents/MacOS/Paseo") {
		t.Fatalf("unexpected tracked path: %s", binCfg.Path)
	}
	if _, err := os.Stat(filepath.Join(applicationsDir, "Paseo.app", "Contents", "MacOS", "Paseo")); err != nil {
		t.Fatalf("expected installed app executable: %v", err)
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
