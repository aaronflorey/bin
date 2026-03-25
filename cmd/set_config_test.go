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
}
