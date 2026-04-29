package cmd

import (
	"fmt"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
)

func TestLifecycleForModeDefaultsToBinary(t *testing.T) {
	strategy := lifecycleForMode("")
	fetchOpts := providers.FetchOpts{}
	b := &config.Binary{
		Path:        "/tmp/bin/tool",
		RemoteName:  "tool",
		PackagePath: "tool",
	}

	if strategy.install == nil {
		t.Fatal("expected binary strategy install handler")
	}
	if strategy.uninstall != nil {
		t.Fatal("expected binary strategy to omit uninstall handler")
	}
	if err := strategy.applyStoredFetch(b, &fetchOpts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strategy.resolvePath(b) {
		t.Fatal("expected binary strategy to resolve install path")
	}
	if fetchOpts.PackagePath != "tool" {
		t.Fatalf("expected package path to be preserved, got %q", fetchOpts.PackagePath)
	}
	if fetchOpts.PackageName != "tool" {
		t.Fatalf("expected package name to be preserved, got %q", fetchOpts.PackageName)
	}
}

func TestLifecycleForModeSystemPackageAppliesStoredMetadata(t *testing.T) {
	strategy := lifecycleForMode(installModeSystemPackage)
	fetchOpts := providers.FetchOpts{}
	b := &config.Binary{
		Path:        "/Applications/Paseo.app/Contents/MacOS/Paseo",
		RemoteName:  "Paseo",
		PackageType: "dmg",
		PackagePath: "Paseo.app",
	}

	if strategy.install == nil || strategy.uninstall == nil {
		t.Fatal("expected system-package strategy handlers")
	}
	if err := strategy.applyStoredFetch(b, &fetchOpts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy.resolvePath(b) {
		t.Fatal("expected system-package strategy to skip path resolution")
	}
	if !fetchOpts.SystemPackage {
		t.Fatal("expected system package flag to be set")
	}
	if fetchOpts.PackageType != "dmg" {
		t.Fatalf("expected normalized package type, got %q", fetchOpts.PackageType)
	}
	if fetchOpts.PackageName != "Paseo" {
		t.Fatalf("expected package name to be preserved, got %q", fetchOpts.PackageName)
	}
	if fetchOpts.PackagePath != "Paseo.app" {
		t.Fatalf("expected package path to be preserved, got %q", fetchOpts.PackagePath)
	}
}

func TestLifecycleForModeSystemPackageRequiresPackageType(t *testing.T) {
	strategy := lifecycleForMode(installModeSystemPackage)
	err := strategy.applyStoredFetch(&config.Binary{Path: "/tmp/tool"}, &providers.FetchOpts{})
	if err == nil {
		t.Fatal("expected missing package type error")
	}
}

func TestRequestedInstallModesDefaultToBinaryThenSystemPackage(t *testing.T) {
	modes := requestedInstallModes(false, false, "")
	if len(modes) != 2 || modes[0] != installModeBinary || modes[1] != installModeSystemPackage {
		t.Fatalf("unexpected mode order: %v", modes)
	}
}

func TestRequestedInstallModesPreferSystemPackage(t *testing.T) {
	modes := requestedInstallModes(false, true, "tool")
	if len(modes) != 2 || modes[0] != installModeSystemPackage || modes[1] != installModeBinary {
		t.Fatalf("unexpected mode order: %v", modes)
	}
}

func TestRequestedInstallModesKeepExplicitPathBinaryOnly(t *testing.T) {
	modes := requestedInstallModes(false, true, "./bin/tool")
	if len(modes) != 1 || modes[0] != installModeBinary {
		t.Fatalf("unexpected mode order: %v", modes)
	}
}

func TestShouldFallbackInstallMode(t *testing.T) {
	if !shouldFallbackInstallMode(fmt.Errorf("%w: nope", assets.ErrNoCompatibleFiles)) {
		t.Fatal("expected no-compatible-files error to trigger fallback")
	}
	if !shouldFallbackInstallMode(systempackage.NewCompatibilityError("wrong type")) {
		t.Fatal("expected system package compatibility error to trigger fallback")
	}
	if shouldFallbackInstallMode(fmt.Errorf("boom")) {
		t.Fatal("did not expect generic error to trigger fallback")
	}
}
