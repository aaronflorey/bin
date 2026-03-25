package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcosnils/bin/pkg/config"
	"github.com/marcosnils/bin/pkg/providers"
)

func TestOutdatedTextOutput(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-outdated-tool")
	upToDatePath := filepath.Join(t.TempDir(), "generic-up-to-date-tool")
	pinnedPath := filepath.Join(t.TempDir(), "generic-pinned-tool")
	writeTestBinary(t, outdatedPath)
	writeTestBinary(t, upToDatePath)
	writeTestBinary(t, pinnedPath)

	if err := config.UpsertBinaries([]*config.Binary{
		{
			Path:     outdatedPath,
			Version:  "1.0.0",
			URL:      "https://example.com/generic-outdated-tool",
			Provider: "github",
		},
		{
			Path:     upToDatePath,
			Version:  "1.1.0",
			URL:      "https://example.com/generic-up-to-date-tool",
			Provider: "github",
		},
		{
			Path:     pinnedPath,
			Version:  "1.0.0",
			URL:      "https://example.com/generic-pinned-tool",
			Provider: "github",
			Pinned:   true,
		},
	}); err != nil {
		t.Fatalf("failed to seed binaries: %v", err)
	}

	cmd := newOutdatedCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/generic-outdated-tool": {
			latestVersion:    "1.2.0",
			latestVersionURL: "https://example.com/generic-outdated-tool/releases/tag/v1.2.0",
		},
		"https://example.com/generic-up-to-date-tool": {
			latestVersion:    "1.1.0",
			latestVersionURL: "https://example.com/generic-up-to-date-tool/releases/tag/v1.1.0",
		},
	})

	var out bytes.Buffer
	cmd.cmd.SetOut(&out)
	cmd.cmd.SetArgs([]string{outdatedPath, upToDatePath, pinnedPath})

	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected outdated command error: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, fmt.Sprintf("%s 1.0.0 -> 1.2.0", outdatedPath)) {
		t.Fatalf("expected outdated binary in output, got: %s", text)
	}
	if strings.Contains(text, upToDatePath) {
		t.Fatalf("did not expect up-to-date binary in output, got: %s", text)
	}
	if strings.Contains(text, pinnedPath) {
		t.Fatalf("did not expect pinned binary in output, got: %s", text)
	}
}

func TestOutdatedJSONOutput(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-json-tool")
	writeTestBinary(t, outdatedPath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     outdatedPath,
		Version:  "0.9.0",
		URL:      "https://example.com/generic-json-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newOutdatedCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/generic-json-tool": {
			latestVersion:    "1.0.0",
			latestVersionURL: "https://example.com/generic-json-tool/releases/tag/v1.0.0",
		},
	})

	var out bytes.Buffer
	cmd.cmd.SetOut(&out)
	cmd.cmd.SetArgs([]string{"--format=json", outdatedPath})

	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected outdated command error: %v", err)
	}

	var payload []outdatedBin
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode outdated json payload: %v", err)
	}

	if len(payload) != 1 {
		t.Fatalf("expected 1 outdated binary, got %d", len(payload))
	}
	if payload[0].Path != outdatedPath {
		t.Fatalf("unexpected path: got %q want %q", payload[0].Path, outdatedPath)
	}
	if payload[0].CurrentVersion != "0.9.0" || payload[0].LatestVersion != "1.0.0" {
		t.Fatalf("unexpected versions: got %+v", payload[0])
	}
}

func TestOutdatedInvalidFormat(t *testing.T) {
	setupTestConfig(t)

	cmd := newOutdatedCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{})
	cmd.cmd.SetArgs([]string{"--format=xml"})

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutdatedTextOutputWhenNoUpdates(t *testing.T) {
	setupTestConfig(t)

	upToDatePath := filepath.Join(t.TempDir(), "generic-latest-tool")
	writeTestBinary(t, upToDatePath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     upToDatePath,
		Version:  "1.0.0",
		URL:      "https://example.com/generic-latest-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newOutdatedCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/generic-latest-tool": {
			latestVersion:    "1.0.0",
			latestVersionURL: "https://example.com/generic-latest-tool/releases/tag/v1.0.0",
		},
	})

	var out bytes.Buffer
	cmd.cmd.SetOut(&out)
	cmd.cmd.SetArgs([]string{upToDatePath})

	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected outdated command error: %v", err)
	}

	if strings.TrimSpace(out.String()) != "All binaries are up to date" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func newMockOutdatedProviderFactory(t *testing.T, providersByURL map[string]mockProvider) providerFactory {
	t.Helper()
	return func(u, _ string) (providers.Provider, error) {
		p, ok := providersByURL[u]
		if !ok {
			return nil, fmt.Errorf("unexpected provider request for %s", u)
		}
		return p, nil
	}
}

func writeTestBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("generic-tool"), 0o755); err != nil {
		t.Fatalf("failed to write test binary: %v", err)
	}
}
