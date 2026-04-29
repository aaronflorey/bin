package cmd

import (
	"bytes"
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
