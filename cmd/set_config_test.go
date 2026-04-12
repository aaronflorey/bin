package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestSetConfigUpdatesDefaultPath(t *testing.T) {
	setupTestConfig(t)

	newDefaultPath := filepath.Join(t.TempDir(), "generic-bin-dir")

	root := newRootCmd("test", func(int) {})
	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stdout)
	root.cmd.SetArgs([]string{"set-config", "default_path", newDefaultPath})

	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("set-config command failed: %v", err)
	}

	if got := config.Get().DefaultPath; got != newDefaultPath {
		t.Fatalf("unexpected default path in memory: got %q, want %q", got, newDefaultPath)
	}

	cfgPath := os.Getenv("BIN_CONFIG")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var stored struct {
		DefaultPath string `json:"default_path"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("failed to decode config file: %v", err)
	}
	if stored.DefaultPath != newDefaultPath {
		t.Fatalf("unexpected default path on disk: got %q, want %q", stored.DefaultPath, newDefaultPath)
	}

	if got := stdout.String(); got != "Set default_path to "+newDefaultPath+"\n" {
		t.Fatalf("unexpected stdout: got %q", got)
	}
}

func TestSetConfigUpdatesUseGHAuth(t *testing.T) {
	setupTestConfig(t)

	root := newRootCmd("test", func(int) {})
	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stdout)
	root.cmd.SetArgs([]string{"set-config", "use_gh_for_github_token", "true"})

	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("set-config command failed: %v", err)
	}

	if !config.Get().UseGHAuth {
		t.Fatalf("expected use_gh_for_github_token to be true in memory")
	}

	cfgPath := os.Getenv("BIN_CONFIG")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var stored struct {
		UseGHAuth bool `json:"use_gh_for_github_token"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("failed to decode config file: %v", err)
	}
	if !stored.UseGHAuth {
		t.Fatalf("expected use_gh_for_github_token to be true on disk")
	}

	if got := stdout.String(); got != "Set use_gh_for_github_token to true\n" {
		t.Fatalf("unexpected stdout: got %q", got)
	}
}

func TestSetConfigRejectsUnsupportedKey(t *testing.T) {
	setupTestConfig(t)

	root := newRootCmd("test", func(int) {})
	root.cmd.SetArgs([]string{"set-config", "unknown_key", "value"})

	err := root.cmd.Execute()
	if err == nil {
		t.Fatal("expected set-config to reject unsupported key")
	}
	if !strings.Contains(err.Error(), `unsupported config key "unknown_key"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "Valid keys: default_path, use_gh_for_github_token") {
		t.Fatalf("expected valid keys in error, got: %v", err)
	}
}

func TestSetConfigHelpShowsValidKeys(t *testing.T) {
	setupTestConfig(t)

	root := newRootCmd("test", func(int) {})
	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stdout)
	root.cmd.SetArgs([]string{"set-config", "-h"})

	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("set-config help failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Valid keys: default_path, use_gh_for_github_token") {
		t.Fatalf("expected valid keys in help output, got: %s", out)
	}
}
