//go:build freebsd

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishStagedBinaryWithoutOverwriteUsesAtomicLink(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "tool")
	stagePath := destination + ".tmp-stage"
	if err := os.WriteFile(destination, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagePath, []byte("candidate"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := publishStagedBinary(stagePath, destination, false); err == nil {
		t.Fatal("expected existing destination to reject non-overwrite publication")
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "existing" {
		t.Fatalf("destination bytes = %q, want existing bytes", contents)
	}
	if _, err := os.Stat(stagePath); err != nil {
		t.Fatalf("stage owned by failed publication was removed: %v", err)
	}
}

func TestPublishStagedBinaryWithoutOverwriteCleansPublishedStage(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "tool")
	stagePath := destination + ".tmp-stage"
	if err := os.WriteFile(stagePath, []byte("candidate"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := publishStagedBinary(stagePath, destination, false); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "candidate" {
		t.Fatalf("destination bytes = %q, want candidate bytes", contents)
	}
	if _, err := os.Lstat(stagePath); !os.IsNotExist(err) {
		t.Fatalf("published stage still exists, err = %v", err)
	}
}
