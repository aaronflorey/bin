//go:build windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/providers"
	"golang.org/x/sys/windows"
)

func TestSaveToDiskLockedDestinationPreservesExistingBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool.cmd")
	const original = "@echo off\r\necho old\r\n"
	if err := os.WriteFile(target, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)

	_, err = saveToDisk(&providers.File{Data: strings.NewReader("@echo off\r\necho new\r\n"), Name: "tool.cmd"}, target, true)
	if err == nil {
		t.Fatal("expected locked destination publication failure")
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != original {
		t.Fatalf("locked destination bytes = %q, want %q", contents, original)
	}
	assertNoStagedBinary(t, target)
}
