//go:build windows

package cmd

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func TestWindowsFilesystemPathsAreExplicit(t *testing.T) {
	for _, path := range []string{
		`C:\tools\bin\tool.exe`,
		`C:tools\bin\tool.exe`,
		`C:tool.exe`,
		`tools\bin\tool.exe`,
		`tools/bin/tool.exe`,
	} {
		if !isExplicitTargetPath(path) || !isExplicitInstallDestination(path) {
			t.Errorf("Windows path %q was not explicit for targeting and installation", path)
		}
	}
}

func TestWindowsGetBinPathFindsManagedBareNamesOutsidePATH(t *testing.T) {
	for _, test := range []struct {
		name      string
		stored    string
		requested string
	}{
		{name: "stored executable suffix", stored: "rg.exe", requested: "rg"},
		{name: "requested executable suffix", stored: "rg", requested: "rg.exe"},
	} {
		t.Run(test.name, func(t *testing.T) {
			installDir := setupTestConfig(t)
			path := filepath.Join(installDir, test.stored)
			if err := config.UpsertBinary(&config.Binary{Path: path}); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", t.TempDir())

			resolved, err := getBinPath(test.requested)
			if err != nil {
				t.Fatalf("getBinPath(%q) error = %v", test.requested, err)
			}
			if resolved != path {
				t.Fatalf("getBinPath(%q) = %q, want %q", test.requested, resolved, path)
			}
		})
	}
}

func TestWindowsInstallReusesSystemPackageOutsideDefaultPath(t *testing.T) {
	defaultPath := setupTestConfig(t)
	destination := filepath.Join(t.TempDir(), "outside", "tool.exe")
	if pathWithin(defaultPath, destination) {
		t.Fatalf("test destination %q is within default path %q", destination, defaultPath)
	}

	config.Get().Bins[destination] = &config.Binary{
		Path:             destination,
		RemoteName:       "tool",
		URL:              "github.com/acme/tool",
		Version:          "v1.0.0",
		InstallMode:      installModeSystemPackage,
		PackageType:      "dmg",
		PackagePath:      "tool.dmg",
		ReleaseTagPrefix: "v",
	}

	originalRegistry := lifecycleRegistry
	originalProviderFactory := installProviderFactory
	t.Cleanup(func() {
		lifecycleRegistry = originalRegistry
		installProviderFactory = originalProviderFactory
	})

	installProviderFactory = func(string, string) (providers.Provider, error) {
		return &staticProvider{history: []*providers.ReleaseInfo{{
			Version: "1.1.0",
			Assets:  []string{fmt.Sprintf("tool_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)},
		}}}, nil
	}

	registry := make(map[string]lifecycleStrategy, len(originalRegistry))
	for mode, strategy := range originalRegistry {
		registry[mode] = strategy
	}
	binaryStrategy := registry[installModeBinary]
	binaryStrategy.install = func(InstallOpts) (*InstallResult, error) {
		t.Fatal("existing system-package target entered direct-binary installation")
		return nil, nil
	}
	registry[installModeBinary] = binaryStrategy

	var got InstallOpts
	systemPackageStrategy := registry[installModeSystemPackage]
	systemPackageStrategy.install = func(opts InstallOpts) (*InstallResult, error) {
		got = opts
		return &InstallResult{Name: "tool", Version: "1.1.0", Path: destination}, nil
	}
	registry[installModeSystemPackage] = systemPackageStrategy
	lifecycleRegistry = registry

	root := newInstallCmd()
	if err := root.installTarget(root.cmd, installTarget{url: "github.com/acme/tool", path: destination}); err != nil {
		t.Fatalf("installTarget() error = %v", err)
	}
	if got.Path != destination || got.ConfigPath != destination {
		t.Fatalf("system-package destination = path %q config path %q, want %q", got.Path, got.ConfigPath, destination)
	}
	if got.ResolvePath {
		t.Fatal("system-package reinstall unexpectedly resolved a direct-binary path")
	}
	if !got.FetchOpts.SystemPackage || got.FetchOpts.PackageType != "dmg" {
		t.Fatalf("system-package lifecycle metadata = %+v", got.FetchOpts)
	}
	if got.FetchOpts.ReleaseTagPrefix != providers.BareReleaseTagPrefix {
		t.Fatalf("release lane = %q, want discovered bare lane %q", got.FetchOpts.ReleaseTagPrefix, providers.BareReleaseTagPrefix)
	}
}
