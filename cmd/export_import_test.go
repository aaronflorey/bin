package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcosnils/bin/pkg/config"
)

func TestExportWritesInstalledBinsToStdout(t *testing.T) {
	setupTestConfig(t)

	installedPath := filepath.Join(t.TempDir(), "generic-tool")
	if err := os.WriteFile(installedPath, []byte("generic-tool-content"), 0o755); err != nil {
		t.Fatalf("failed to write installed test binary: %v", err)
	}

	if err := config.UpsertBinary(&config.Binary{
		Path:       installedPath,
		RemoteName: "generic-tool",
		Version:    "1.2.3",
		Hash:       "stale-hash",
		URL:        "https://example.com/tools/generic-tool/releases/tag/v1.2.3",
		Provider:   "github",
	}); err != nil {
		t.Fatalf("failed to upsert installed binary: %v", err)
	}

	missingPath := filepath.Join(t.TempDir(), "missing-tool")
	if err := config.UpsertBinary(&config.Binary{
		Path:       missingPath,
		RemoteName: "missing-tool",
		Version:    "0.1.0",
		Hash:       "unused-hash",
		URL:        "https://example.com/tools/missing-tool/releases/tag/v0.1.0",
		Provider:   "github",
	}); err != nil {
		t.Fatalf("failed to upsert missing binary: %v", err)
	}

	cmd := newExportCmd().cmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected export command error: %v", err)
	}

	var exported []map[string]any
	if err := json.Unmarshal(out.Bytes(), &exported); err != nil {
		t.Fatalf("failed to decode export payload: %v", err)
	}

	if len(exported) != 1 {
		t.Fatalf("expected 1 exported binary, got %d", len(exported))
	}

	expectedHash, err := hashFile(installedPath)
	if err != nil {
		t.Fatalf("failed to hash installed binary: %v", err)
	}

	got := exported[0]
	if _, ok := got["path"]; ok {
		t.Fatalf("did not expect exported payload to include path")
	}
	if got["name"] != "generic-tool" {
		t.Fatalf("unexpected exported name: got %#v, want %q", got["name"], "generic-tool")
	}
	if got["version"] != "1.2.3" {
		t.Fatalf("unexpected exported version: got %#v, want %q", got["version"], "1.2.3")
	}
	if got["hash"] != expectedHash {
		t.Fatalf("unexpected exported hash: got %#v, want %q", got["hash"], expectedHash)
	}
}

func TestExportWritesToFileWhenPathIsProvided(t *testing.T) {
	setupTestConfig(t)

	installedPath := filepath.Join(t.TempDir(), "another-generic-tool")
	if err := os.WriteFile(installedPath, []byte("another-generic-tool-content"), 0o755); err != nil {
		t.Fatalf("failed to write installed test binary: %v", err)
	}

	if err := config.UpsertBinary(&config.Binary{
		Path:       installedPath,
		RemoteName: "another-generic-tool",
		Version:    "2.3.4",
		Hash:       "stale-hash",
		URL:        "https://example.com/tools/another-generic-tool/releases/tag/v2.3.4",
		Provider:   "gitlab",
	}); err != nil {
		t.Fatalf("failed to upsert binary: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "export.json")

	cmd := newExportCmd().cmd
	cmd.SetArgs([]string{outPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected export command error: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}

	var exported []map[string]any
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatalf("failed to decode export payload: %v", err)
	}

	if len(exported) != 1 {
		t.Fatalf("expected 1 exported binary, got %d", len(exported))
	}
	if exported[0]["name"] != "another-generic-tool" {
		t.Fatalf("unexpected exported name: got %#v, want %q", exported[0]["name"], "another-generic-tool")
	}
}

func TestImportReadsFromStdinWhenNoPathIsProvided(t *testing.T) {
	defaultPath := setupTestConfig(t)

	name := "stdin-imported-tool"
	path := filepath.Join(defaultPath, name)
	imported := []map[string]any{
		{
			"name":        name,
			"remote_name": "stdin-imported-tool",
			"version":     "3.0.0",
			"hash":        "stdin-hash",
			"url":         "https://example.com/tools/stdin-imported-tool/releases/tag/v3.0.0",
			"provider":    "codeberg",
		},
	}
	payload, err := json.Marshal(imported)
	if err != nil {
		t.Fatalf("failed to marshal import payload: %v", err)
	}

	cmd := newImportCmd().cmd
	cmd.SetIn(bytes.NewReader(payload))
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected import command error: %v", err)
	}

	got, ok := config.Get().Bins[path]
	if !ok {
		t.Fatalf("expected imported binary at path %q", path)
	}
	if got.Version != "3.0.0" {
		t.Fatalf("unexpected imported version: got %q, want %q", got.Version, "3.0.0")
	}
	if got.Hash != "stdin-hash" {
		t.Fatalf("unexpected imported hash: got %q, want %q", got.Hash, "stdin-hash")
	}
}

func TestImportReadsFromFileWhenPathIsProvided(t *testing.T) {
	defaultPath := setupTestConfig(t)

	name := "file-imported-tool"
	path := filepath.Join(defaultPath, name)
	imported := []map[string]any{
		{
			"name":        name,
			"remote_name": "file-imported-tool",
			"version":     "4.5.6",
			"hash":        "file-hash",
			"url":         "https://example.com/tools/file-imported-tool/releases/tag/v4.5.6",
			"provider":    "hashicorp",
		},
	}
	payload, err := json.Marshal(imported)
	if err != nil {
		t.Fatalf("failed to marshal import payload: %v", err)
	}

	inPath := filepath.Join(t.TempDir(), "import.json")
	if err := os.WriteFile(inPath, payload, 0o644); err != nil {
		t.Fatalf("failed to write import payload: %v", err)
	}

	cmd := newImportCmd().cmd
	cmd.SetArgs([]string{inPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected import command error: %v", err)
	}

	got, ok := config.Get().Bins[path]
	if !ok {
		t.Fatalf("expected imported binary at path %q", path)
	}
	if got.Provider != "hashicorp" {
		t.Fatalf("unexpected imported provider: got %q, want %q", got.Provider, "hashicorp")
	}
}

func TestImportOutputsInstalledUpdatedSkipped(t *testing.T) {
	defaultPath := setupTestConfig(t)

	skippedPath := filepath.Join(defaultPath, "skipped-tool")
	updatedPath := filepath.Join(defaultPath, "updated-tool")
	if err := config.UpsertBinaries([]*config.Binary{
		{
			Path:       skippedPath,
			RemoteName: "skipped-tool",
			Version:    "1.0.0",
			Hash:       "same-hash",
			URL:        "https://example.com/tools/skipped-tool/releases/tag/v1.0.0",
			Provider:   "github",
		},
		{
			Path:       updatedPath,
			RemoteName: "updated-tool",
			Version:    "0.9.0",
			Hash:       "old-hash",
			URL:        "https://example.com/tools/updated-tool/releases/tag/v0.9.0",
			Provider:   "github",
		},
	}); err != nil {
		t.Fatalf("failed to seed binaries: %v", err)
	}

	imported := []map[string]any{
		{
			"name":        "installed-tool",
			"remote_name": "installed-tool",
			"version":     "2.0.0",
			"hash":        "new-hash",
			"url":         "https://example.com/tools/installed-tool/releases/tag/v2.0.0",
			"provider":    "gitlab",
		},
		{
			"name":        "updated-tool",
			"remote_name": "updated-tool",
			"version":     "1.0.0",
			"hash":        "updated-hash",
			"url":         "https://example.com/tools/updated-tool/releases/tag/v1.0.0",
			"provider":    "github",
		},
		{
			"name":        "skipped-tool",
			"remote_name": "skipped-tool",
			"version":     "1.0.0",
			"hash":        "same-hash",
			"url":         "https://example.com/tools/skipped-tool/releases/tag/v1.0.0",
			"provider":    "github",
		},
	}
	payload, err := json.Marshal(imported)
	if err != nil {
		t.Fatalf("failed to marshal import payload: %v", err)
	}

	cmd := newImportCmd().cmd
	cmd.SetIn(bytes.NewReader(payload))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected import command error: %v", err)
	}

	outText := out.String()
	if !strings.Contains(outText, "installed: "+filepath.Join(defaultPath, "installed-tool")) {
		t.Fatalf("expected installed status output, got: %s", outText)
	}
	if !strings.Contains(outText, "updated: "+updatedPath) {
		t.Fatalf("expected updated status output, got: %s", outText)
	}
	if !strings.Contains(outText, "skipped: "+skippedPath) {
		t.Fatalf("expected skipped status output, got: %s", outText)
	}
	if !strings.Contains(outText, "import complete: installed=1 updated=1 skipped=1") {
		t.Fatalf("expected summary output, got: %s", outText)
	}
}

func setupTestConfig(t *testing.T) string {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := t.TempDir()
	initial := struct {
		DefaultPath string                    `json:"default_path"`
		Bins        map[string]*config.Binary `json:"bins"`
	}{
		DefaultPath: defaultPath,
		Bins:        map[string]*config.Binary{},
	}
	raw, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	t.Setenv("BIN_CONFIG", cfgPath)
	if err := config.CheckAndLoad(); err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}
	return defaultPath
}
