package cmd

import (
	"path/filepath"
	"strings"
	"testing"

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
	targets, err := parseInstallTargets([]string{"github.com/cli/cli"})
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
	targets, err := parseInstallTargets([]string{"github.com/cli/cli", "./bin/gh"})
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
	targets, err := parseInstallTargets([]string{"github.com/cli/cli", "github.com/sharkdp/fd"})
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
	targets, err := parseInstallTargets([]string{"github.com/cli/cli", "github.com/sharkdp/fd", "docker://hashicorp/terraform:light"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
}

func TestParseInstallTargetsRejectsMixedMultiArgs(t *testing.T) {
	_, err := parseInstallTargets([]string{"github.com/cli/cli", "github.com/sharkdp/fd", "./bin/custom"})
	if err == nil {
		t.Fatal("expected parseInstallTargets to reject non-url argument in multi-url mode")
	}
	if !strings.Contains(err.Error(), "all arguments must be URLs") {
		t.Fatalf("unexpected error: %v", err)
	}
}
