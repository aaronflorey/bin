package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestRemoveSystemPackageRequiresYesInNonInteractiveMode(t *testing.T) {
	defaultPath := setupTestConfig(t)

	trackedPath := filepath.Join(defaultPath, "com.example.Tool")
	if err := config.UpsertBinary(&config.Binary{
		Path:        trackedPath,
		RemoteName:  "tool",
		Version:     "1.0.0",
		Hash:        "hash",
		URL:         "https://example.test/acme/tool",
		Provider:    "github",
		InstallMode: installModeSystemPackage,
		PackageType: "flatpak",
	}); err != nil {
		t.Fatalf("failed to seed test config: %v", err)
	}

	cmd := newRemoveCmd().cmd
	cmd.SetArgs([]string{filepath.Base(trackedPath)})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected remove to require --yes in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveSystemPackageWithYesRemovesConfigEntry(t *testing.T) {
	defaultPath := setupTestConfig(t)

	trackedPath := filepath.Join(defaultPath, "com.example.Tool")
	if err := config.UpsertBinary(&config.Binary{
		Path:        trackedPath,
		RemoteName:  "tool",
		Version:     "1.0.0",
		Hash:        "hash",
		URL:         "https://example.test/acme/tool",
		Provider:    "github",
		InstallMode: installModeSystemPackage,
		PackageType: "flatpak",
	}); err != nil {
		t.Fatalf("failed to seed test config: %v", err)
	}

	originalExec := execCommand
	execCommand = helperExecCommand(t, 0, nil)
	defer func() {
		execCommand = originalExec
	}()

	cmd := newRemoveCmd().cmd
	cmd.SetArgs([]string{"--yes", filepath.Base(trackedPath)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	if _, ok := config.Get().Bins[trackedPath]; ok {
		t.Fatalf("expected config entry %s to be removed", trackedPath)
	}
}
