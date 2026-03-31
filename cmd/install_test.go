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
