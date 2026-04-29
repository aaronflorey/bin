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

func TestWriteListIncludesInstallMode(t *testing.T) {
	defaultPath := setupTestConfig(t)
	bins := map[string]*config.Binary{
		filepath.Join(defaultPath, "tool"): {
			Path:        filepath.Join(defaultPath, "tool"),
			Version:     "1.0.0",
			URL:         "https://example.test/tool",
			InstallMode: installModeSystemPackage,
			PackageType: "dmg",
		},
	}

	var out bytes.Buffer
	if err := writeList(&out, bins); err != nil {
		t.Fatalf("writeList returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Mode") {
		t.Fatalf("expected mode header in output: %s", out.String())
	}
	if !strings.Contains(out.String(), "system-package:dmg") {
		t.Fatalf("expected mode value in output: %s", out.String())
	}
}

func TestWriteListJSONIncludesInstallMetadata(t *testing.T) {
	defaultPath := setupTestConfig(t)
	toolPath := filepath.Join(defaultPath, "tool")
	if err := os.WriteFile(toolPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}
	bins := map[string]*config.Binary{
		toolPath: {
			Path:        toolPath,
			Version:     "1.0.0",
			URL:         "https://example.test/tool",
			InstallMode: installModeSystemPackage,
			PackageType: "dmg",
			AppBundle:   "Tool.app",
			Provider:    "github",
			RemoteName:  "Tool",
			Pinned:      true,
		},
	}

	var out bytes.Buffer
	if err := writeListJSON(&out, bins); err != nil {
		t.Fatalf("writeListJSON returned error: %v", err)
	}

	var entries []listedBinary
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("failed to parse json output: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one list entry, got %d", len(entries))
	}
	if entries[0].InstallMode != installModeSystemPackage {
		t.Fatalf("expected install mode %q, got %q", installModeSystemPackage, entries[0].InstallMode)
	}
	if entries[0].PackageType != "dmg" {
		t.Fatalf("expected package type dmg, got %q", entries[0].PackageType)
	}
	if entries[0].AppBundle != "Tool.app" {
		t.Fatalf("expected app bundle Tool.app, got %q", entries[0].AppBundle)
	}
	if entries[0].Status != "ok" {
		t.Fatalf("expected status ok, got %q", entries[0].Status)
	}
	if !entries[0].Pinned {
		t.Fatal("expected pinned entry")
	}
}

func TestListRejectsUnknownFormat(t *testing.T) {
	cmd := newListCmd()
	cmd.cmd.SetArgs([]string{"--format=xml"})

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected list command to reject unknown format")
	}
	if !strings.Contains(err.Error(), `unsupported --format "xml"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
