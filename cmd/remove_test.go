package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
)

func TestRemoveAliases(t *testing.T) {
	cmd := newRemoveCmd().cmd
	for _, alias := range []string{"rm", "r", "uninstall"} {
		if !slices.Contains(cmd.Aliases, alias) {
			t.Fatalf("expected remove alias %q", alias)
		}
	}

	if slices.Contains(cmd.Aliases, "u") {
		t.Fatalf("did not expect remove alias %q because update already uses it", "u")
	}
}

func TestRemoveWithoutArgsRequiresInteractive(t *testing.T) {
	defaultPath := setupTestConfig(t)

	trackedPath := filepath.Join(defaultPath, "tool-a")
	if err := config.UpsertBinary(&config.Binary{
		Path:       trackedPath,
		RemoteName: "tool-a",
		Version:    "1.0.0",
		Hash:       "hash",
		URL:        "https://example.test/acme/tool-a",
		Provider:   "github",
	}); err != nil {
		t.Fatalf("failed to seed test config: %v", err)
	}

	root := newRemoveCmd()
	root.isInteractive = func() bool { return false }

	root.cmd.SetArgs([]string{})
	err := root.cmd.Execute()
	if err == nil {
		t.Fatal("expected interactive terminal error")
	}
	if !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveWithoutArgsUsesSelector(t *testing.T) {
	defaultPath := setupTestConfig(t)

	trackedPathA := filepath.Join(defaultPath, "tool-a")
	trackedPathB := filepath.Join(defaultPath, "tool-b")
	for _, trackedPath := range []string{trackedPathA, trackedPathB} {
		if err := os.WriteFile(trackedPath, []byte("binary"), 0o755); err != nil {
			t.Fatalf("failed creating binary file %s: %v", trackedPath, err)
		}
		if err := config.UpsertBinary(&config.Binary{
			Path:       trackedPath,
			RemoteName: filepath.Base(trackedPath),
			Version:    "1.0.0",
			Hash:       "hash",
			URL:        "https://example.test/acme/" + filepath.Base(trackedPath),
			Provider:   "unknown-provider",
		}); err != nil {
			t.Fatalf("failed to seed test config: %v", err)
		}
	}

	root := newRemoveCmd()
	root.isInteractive = func() bool { return true }
	root.selectTargets = func(_ string, options []prompt.MultiSelectOption) ([]string, error) {
		if len(options) != 2 {
			t.Fatalf("expected 2 options, got %d", len(options))
		}
		return []string{trackedPathA, trackedPathB}, nil
	}

	root.cmd.SetArgs([]string{})
	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	if _, ok := config.Get().Bins[trackedPathA]; ok {
		t.Fatalf("expected config entry %s to be removed", trackedPathA)
	}
	if _, ok := config.Get().Bins[trackedPathB]; ok {
		t.Fatalf("expected config entry %s to be removed", trackedPathB)
	}
}

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
