package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func TestAbsExpandedPath(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(prevWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := absExpandedPath("$HOME/.local/bin/tool")
	if err != nil {
		t.Fatalf("absExpandedPath: %v", err)
	}

	want := filepath.Join(homeDir, ".local", "bin", "tool")
	if got != want {
		t.Fatalf("unexpected expanded path: got %q, want %q", got, want)
	}
}

func TestExistingConfigBinaryMatchesExpandedPath(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(prevWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	expandedPath := filepath.Join(homeDir, ".local", "bin", "tool")
	prevBins := config.Get().Bins
	config.Get().Bins = map[string]*config.Binary{
		expandedPath: {Path: expandedPath, RemoteName: "tool"},
	}
	defer func() {
		config.Get().Bins = prevBins
	}()

	got, ok := existingConfigBinary(InstallOpts{Path: "$HOME/.local/bin/tool"})
	if !ok {
		t.Fatal("expected existingConfigBinary to match expanded path")
	}
	if got.Path != expandedPath {
		t.Fatalf("unexpected binary path: got %q, want %q", got.Path, expandedPath)
	}
}

func TestSaveToDiskValidatesExpectedSHA(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")

	_, err := saveToDisk(&providers.File{
		Data:        strings.NewReader("hello"),
		Name:        "tool",
		ExpectedSHA: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	}, target, false)
	if err != nil {
		t.Fatalf("saveToDisk returned error: %v", err)
	}

	_, err = saveToDisk(&providers.File{
		Data:        strings.NewReader("world"),
		Name:        "tool2",
		ExpectedSHA: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	}, filepath.Join(dir, "tool2"), false)
	if err == nil {
		t.Fatal("expected saveToDisk to fail on sha mismatch")
	}
}

func TestCheckFinalPathErrorsWhenExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, _, err := checkFinalPath(target, "ignored", false)
	if err == nil {
		t.Fatal("expected checkFinalPath to fail when file already exists")
	}
}

func TestCheckFinalPathAllowsOverwriteWhenForced(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	finalPath, overwrite, err := checkFinalPath(target, "ignored", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalPath != target {
		t.Fatalf("unexpected final path: %s", finalPath)
	}
	if !overwrite {
		t.Fatal("expected overwrite to remain enabled")
	}
}

func TestCheckFinalPathKeepsOverwriteDisabledForNewPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")

	finalPath, overwrite, err := checkFinalPath(target, "ignored", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalPath != target {
		t.Fatalf("unexpected final path: %s", finalPath)
	}
	if overwrite {
		t.Fatal("expected overwrite to stay disabled")
	}
}

func TestFindManagedDuplicateByHash(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-a": {Path: "/tmp/tool-a", Hash: "abc"},
		"/tmp/tool-b": {Path: "/tmp/tool-b", Hash: "def"},
		"/tmp/tool-c": {Path: "/tmp/tool-c", Hash: "abc"},
	}

	duplicatePath, ok := findManagedDuplicateByHash(bins, "/tmp/tool-a", "abc")
	if !ok {
		t.Fatal("expected duplicate hash to be found")
	}
	if duplicatePath != "/tmp/tool-c" {
		t.Fatalf("unexpected duplicate path: %s", duplicatePath)
	}
}
