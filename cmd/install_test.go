package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
)

func TestInstallRejectsNonPositiveMinAgeDays(t *testing.T) {
	cmd := newInstallCmd()
	cmd.cmd.SetArgs([]string{"--min-age-days=0", "https://example.test/acme/tool"})

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected install command to reject min-age-days=0")
	}
	if !strings.Contains(err.Error(), "--min-age-days must be a positive integer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallHasPinFlag(t *testing.T) {
	cmd := newInstallCmd()

	if cmd.cmd.Flags().Lookup("pin") == nil {
		t.Fatal("expected --pin flag to be registered")
	}
}

func TestInstallHasPreferSystemPackageFlag(t *testing.T) {
	cmd := newInstallCmd()

	if cmd.cmd.Flags().Lookup("prefer-system-package") == nil {
		t.Fatal("expected --prefer-system-package flag to be registered")
	}
}

func TestInstallHasPackageTypeFlag(t *testing.T) {
	cmd := newInstallCmd()

	if cmd.cmd.Flags().Lookup("package-type") == nil {
		t.Fatal("expected --package-type flag to be registered")
	}
}

func TestInstallRejectsUnknownPackageType(t *testing.T) {
	cmd := newInstallCmd()
	cmd.cmd.SetArgs([]string{"--package-type=msi", "https://example.test/acme/tool"})

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected install command to reject unknown package type")
	}
	if !strings.Contains(err.Error(), `unsupported --package-type "msi"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallFallsBackFromBinaryToSystemPackageOnCompatibilityError(t *testing.T) {
	setupTestConfig(t)
	root := newInstallCmd()

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	var attempts []string
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			install: func(opts InstallOpts) (*InstallResult, error) {
				attempts = append(attempts, installModeBinary)
				return nil, fmt.Errorf("%w: binary mismatch", assets.ErrNoCompatibleFiles)
			},
			applyStoredFetch:  originalRegistry[installModeBinary].applyStoredFetch,
			applyRequestFetch: originalRegistry[installModeBinary].applyRequestFetch,
			resolvePath:       originalRegistry[installModeBinary].resolvePath,
		},
		installModeSystemPackage: {
			install: func(opts InstallOpts) (*InstallResult, error) {
				attempts = append(attempts, installModeSystemPackage)
				if !opts.FetchOpts.SystemPackage {
					t.Fatal("expected system-package fetch flag")
				}
				return &InstallResult{Name: "tool", Version: "1.0.0", Path: "/Applications/Tool.app/Contents/MacOS/Tool"}, nil
			},
			uninstall:         originalRegistry[installModeSystemPackage].uninstall,
			applyStoredFetch:  originalRegistry[installModeSystemPackage].applyStoredFetch,
			applyRequestFetch: originalRegistry[installModeSystemPackage].applyRequestFetch,
			resolvePath:       originalRegistry[installModeSystemPackage].resolvePath,
		},
	}

	if err := root.installTarget(root.cmd, installTarget{url: "github.com/acme/tool", path: "Tool"}); err != nil {
		t.Fatalf("unexpected install error: %v", err)
	}
	if !slices.Equal(attempts, []string{installModeBinary, installModeSystemPackage}) {
		t.Fatalf("unexpected install attempts: %v", attempts)
	}
}

func TestInstallPrefersSystemPackageWhenRequested(t *testing.T) {
	setupTestConfig(t)
	root := newInstallCmd()
	root.opts.preferSystemPackage = true
	root.opts.packageType = "DMG"

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	var attempts []string
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			install: func(opts InstallOpts) (*InstallResult, error) {
				attempts = append(attempts, installModeBinary)
				return &InstallResult{Name: "tool", Version: "1.0.0", Path: opts.Path}, nil
			},
			applyStoredFetch:  originalRegistry[installModeBinary].applyStoredFetch,
			applyRequestFetch: originalRegistry[installModeBinary].applyRequestFetch,
			resolvePath:       originalRegistry[installModeBinary].resolvePath,
		},
		installModeSystemPackage: {
			install: func(opts InstallOpts) (*InstallResult, error) {
				attempts = append(attempts, installModeSystemPackage)
				if opts.FetchOpts.PackageType != "dmg" {
					t.Fatalf("expected normalized package type, got %q", opts.FetchOpts.PackageType)
				}
				return &InstallResult{Name: "Tool", Version: "1.0.0", Path: "/Applications/Tool.app/Contents/MacOS/Tool"}, nil
			},
			uninstall:         originalRegistry[installModeSystemPackage].uninstall,
			applyStoredFetch:  originalRegistry[installModeSystemPackage].applyStoredFetch,
			applyRequestFetch: originalRegistry[installModeSystemPackage].applyRequestFetch,
			resolvePath:       originalRegistry[installModeSystemPackage].resolvePath,
		},
	}

	if err := root.installTarget(root.cmd, installTarget{url: "github.com/acme/tool", path: "Tool"}); err != nil {
		t.Fatalf("unexpected install error: %v", err)
	}
	if !slices.Equal(attempts, []string{installModeSystemPackage}) {
		t.Fatalf("unexpected install attempts: %v", attempts)
	}
}

func TestInstallDoesNotFallbackOnNonCompatibilityError(t *testing.T) {
	setupTestConfig(t)
	root := newInstallCmd()

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	attempts := 0
	fatalErr := errors.New("boom")
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			install: func(opts InstallOpts) (*InstallResult, error) {
				attempts++
				return nil, fatalErr
			},
			applyStoredFetch:  originalRegistry[installModeBinary].applyStoredFetch,
			applyRequestFetch: originalRegistry[installModeBinary].applyRequestFetch,
			resolvePath:       originalRegistry[installModeBinary].resolvePath,
		},
		installModeSystemPackage: originalRegistry[installModeSystemPackage],
	}

	err := root.installTarget(root.cmd, installTarget{url: "github.com/acme/tool", path: "Tool"})
	if !errors.Is(err, fatalErr) {
		t.Fatalf("expected fatal error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected one install attempt, got %d", attempts)
	}
}

func TestExistingBinaryForInstallByURL(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool": {
			Path:     "/tmp/tool",
			URL:      "https://example.test/acme/tool",
			Provider: "github",
		},
	}

	existing := existingBinaryForInstall(bins, "https://example.test/acme/tool", "", "")
	if existing == nil {
		t.Fatal("expected existing binary to be found by URL")
	}
}

func TestExistingBinaryForInstallMatchesRequestedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	prevBins := config.Get().Bins
	config.Get().Bins = map[string]*config.Binary{
		path: {
			Path:     path,
			URL:      "https://example.test/acme/tool",
			Provider: "github",
		},
	}
	defer func() {
		config.Get().Bins = prevBins
	}()

	existing := existingBinaryForInstall(config.Get().Bins, "https://example.test/other", "", path)
	if existing == nil {
		t.Fatal("expected existing binary to be found by requested path")
	}
}

func TestParseInstallTargetsSingleURL(t *testing.T) {
	targets, err := parseInstallTargets([]string{"github.com/cli/cli"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].url != "github.com/cli/cli" || targets[0].path != "" {
		t.Fatalf("unexpected target: %+v", targets[0])
	}
}

func TestParseInstallTargetsSingleURLWithPath(t *testing.T) {
	targets, err := parseInstallTargets([]string{"github.com/cli/cli", "./bin/gh"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].url != "github.com/cli/cli" || targets[0].path != "./bin/gh" {
		t.Fatalf("unexpected target: %+v", targets[0])
	}
}

func TestParseInstallTargetsTwoURLs(t *testing.T) {
	targets, err := parseInstallTargets([]string{"github.com/cli/cli", "github.com/sharkdp/fd"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].path != "" || targets[1].path != "" {
		t.Fatalf("expected no explicit paths: %+v", targets)
	}
}

func TestParseInstallTargetsThreeURLs(t *testing.T) {
	targets, err := parseInstallTargets([]string{"github.com/cli/cli", "github.com/sharkdp/fd", "docker://hashicorp/terraform:light"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
}

func TestParseInstallTargetsRejectsMixedMultiArgs(t *testing.T) {
	_, err := parseInstallTargets([]string{"github.com/cli/cli", "github.com/sharkdp/fd", "./bin/custom"}, false)
	if err == nil {
		t.Fatal("expected parseInstallTargets to reject non-url argument in multi-url mode")
	}
	if !strings.Contains(err.Error(), "all arguments must be URLs") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseInstallTargetsSystemPackageRejectsFilesystemPath(t *testing.T) {
	_, err := parseInstallTargets([]string{"github.com/cli/cli", "./bin/gh"}, true)
	if err == nil {
		t.Fatal("expected parseInstallTargets to reject filesystem path in --system-package mode")
	}
	if !strings.Contains(err.Error(), "does not accept filesystem paths") {
		t.Fatalf("unexpected error: %v", err)
	}
}
