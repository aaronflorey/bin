//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureUserLocalBinDirCreatesMissingDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	got, err := ensureUserLocalBinDir()
	if err != nil {
		t.Fatalf("ensureUserLocalBinDir: %v", err)
	}

	want := filepath.Join(homeDir, ".local", "bin")
	if got != want {
		t.Fatalf("unexpected path: got %q, want %q", got, want)
	}

	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat %q: %v", want, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", want)
	}
}
