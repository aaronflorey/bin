package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	t.Setenv("BIN_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))

	root := newRootCmd("1.2.3", func(int) {})

	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stdout)
	root.cmd.SetArgs([]string{"version"})

	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	if got := stdout.String(); got != "1.2.3\n" {
		t.Fatalf("unexpected version output: got %q, want %q", got, "1.2.3\n")
	}
}
