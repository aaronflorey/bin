package cmd

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

type mockProvider struct {
	providers.Provider
	id               string
	latestVersion    string
	latestVersionURL string
	publishedAt      *time.Time
	release          *providers.ReleaseInfo
	returnNilRelease bool
	err              error
}

func (m mockProvider) GetLatestVersion() (*providers.ReleaseInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.returnNilRelease {
		return nil, nil
	}
	if m.release != nil {
		return m.release, nil
	}
	return &providers.ReleaseInfo{
		Version:     m.latestVersion,
		URL:         m.latestVersionURL,
		PublishedAt: m.publishedAt,
	}, nil
}

func (m mockProvider) GetID() string {
	if m.id != "" {
		return m.id
	}
	return "github"
}

func TestGetLatestVersion(t *testing.T) {
	type mockValues struct {
		latestVersion    string
		latestVersionURL string
		publishedAt      *time.Time
		err              error
	}
	oldRelease := time.Now().AddDate(0, 0, -10)
	newRelease := time.Now().AddDate(0, 0, -2)
	cases := []struct {
		in  *config.Binary
		m   mockValues
		out *updateInfo
		err string
	}{
		{
			&config.Binary{
				Path:       "/home/user/bin/tool",
				Version:    "1.1.0",
				URL:        "https://example.test/acme/tool/releases/download/1.1.0/tool-linux-x64",
				RemoteName: "tool-linux-x64",
				Provider:   "github",
			},
			mockValues{"1.1.1", "https://example.test/acme/tool/releases/download/1.1.1/tool-linux-x64", &oldRelease, nil},
			&updateInfo{
				version: "1.1.1",
				url:     "https://example.test/acme/tool/releases/download/1.1.1/tool-linux-x64",
			},
			"",
		},
		{
			&config.Binary{
				Path:       "/home/user/bin/tool",
				Version:    "1.2.0-rc.1",
				URL:        "https://example.test/acme/tool/releases/download/1.2.0-rc.1/tool-linux-x64",
				RemoteName: "tool-linux-x64",
				Provider:   "github",
			},
			mockValues{"1.1.1", "https://example.test/acme/tool/releases/download/1.1.1/tool-linux-x64", &oldRelease, nil},
			nil,
			"",
		},
		{
			&config.Binary{
				Path:       "/home/user/bin/tool",
				Version:    "1.1.0",
				URL:        "https://example.test/acme/tool/releases/download/1.1.0/tool-linux-x64",
				RemoteName: "tool-linux-x64",
				Provider:   "github",
				MinAgeDays: 7,
			},
			mockValues{"1.1.1", "https://example.test/acme/tool/releases/download/1.1.1/tool-linux-x64", &newRelease, nil},
			nil,
			"",
		},
		{
			&config.Binary{
				Path:       "/home/user/bin/tool",
				Version:    "1.1.0",
				URL:        "https://example.test/acme/tool/releases/download/1.1.0/tool-linux-x64",
				RemoteName: "tool-linux-x64",
				Provider:   "docker",
				MinAgeDays: 7,
			},
			mockValues{"1.1.1", "https://example.test/acme/tool/releases/download/1.1.1/tool-linux-x64", nil, nil},
			nil,
			`provider "docker" does not expose release publication time`,
		},
	}

	for _, c := range cases {
		p := mockProvider{id: c.in.Provider, latestVersion: c.m.latestVersion, latestVersionURL: c.m.latestVersionURL, publishedAt: c.m.publishedAt, err: c.m.err}
		if v, err := getLatestVersion(c.in, p); c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("expected error %q, got %v", c.err, err)
			}
		} else if err != nil {
			t.Fatalf("Error during getLatestVersion(%#v, %#v): %v", c.in, p, err)
		} else if !reflect.DeepEqual(v, c.out) {
			t.Fatalf("For case %#v: %#v does not match %#v", c.in, v, c.out)
		}
	}
}

func TestResolveUpdateTargetsWithURL(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool": {
			Path:    "/tmp/tool",
			URL:     "github.com/acme/tool",
			Version: "1.0.0",
		},
	}

	resolved, explicitVersion, hasExplicitVersion, err := resolveUpdateTargets(bins, []string{"github.com/acme/tool/releases/tag/v1.2.0"})
	if err != nil {
		t.Fatalf("resolveUpdateTargets returned error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved binary, got %d", len(resolved))
	}
	if explicitVersion != "v1.2.0" {
		t.Fatalf("unexpected explicit version: %s", explicitVersion)
	}
	if !hasExplicitVersion {
		t.Fatal("expected explicit version to be detected")
	}
}

func TestShouldUpdateToExplicitVersion(t *testing.T) {
	if shouldUpdateToExplicitVersion("1.2.0", "1.1.0") {
		t.Fatal("should not update when explicit version is older")
	}
	if !shouldUpdateToExplicitVersion("1.1.0", "1.2.0") {
		t.Fatal("should update when explicit version is newer")
	}
}

func TestGetLatestVersionSkipsWhenProviderCannotInferVersion(t *testing.T) {
	b := &config.Binary{
		Path:       "/home/user/bin/tool",
		Version:    "1.1.0",
		URL:        "https://downloads.example.test/tool",
		RemoteName: "tool",
		Provider:   "generic",
	}

	p := mockProvider{id: "generic", returnNilRelease: true}
	got, err := getLatestVersion(b, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil update info, got %+v", got)
	}
}

func TestGetLatestVersionDetectsGenericSemverUpdate(t *testing.T) {
	b := &config.Binary{
		Path:       "/home/user/bin/tool",
		Version:    "0.15.0",
		URL:        "https://downloads.example.test/tool",
		RemoteName: "tool",
		Provider:   "generic",
	}

	p := mockProvider{
		id: "generic",
		release: &providers.ReleaseInfo{
			Version: "0.16.0",
			URL:     "https://cdn.example.test/tool_0.16.0_darwin_arm64",
		},
	}

	got, err := getLatestVersion(b, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected update info")
	}
	if got.version != "0.16.0" {
		t.Fatalf("unexpected version: %s", got.version)
	}
}

func TestUpdateWithoutArgsUsesInteractiveSelector(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-update-selector-tool")
	writeTestBinary(t, outdatedPath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     outdatedPath,
		Version:  "1.0.0",
		URL:      "https://example.com/generic-update-selector-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		if u != "https://example.com/generic-update-selector-tool" {
			return nil, fmt.Errorf("unexpected provider request for %s", u)
		}
		return mockProvider{latestVersion: "1.1.0", latestVersionURL: "https://example.com/generic-update-selector-tool/releases/tag/v1.1.0"}, nil
	}

	selectorCalled := false
	cmd.selectItems = func(updates []availableUpdate) ([]availableUpdate, error) {
		selectorCalled = true
		if len(updates) != 1 {
			t.Fatalf("expected 1 update candidate, got %d", len(updates))
		}
		return nil, nil
	}

	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected update command error: %v", err)
	}
	if !selectorCalled {
		t.Fatal("expected interactive selector to be called")
	}
}

func TestUpdateWithArgsSkipsInteractiveSelector(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-update-no-selector-tool")
	writeTestBinary(t, outdatedPath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     outdatedPath,
		Version:  "1.0.0",
		URL:      "https://example.com/generic-update-no-selector-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		if u != "https://example.com/generic-update-no-selector-tool" {
			return nil, fmt.Errorf("unexpected provider request for %s", u)
		}
		return mockProvider{latestVersion: "1.2.0", latestVersionURL: "https://example.com/generic-update-no-selector-tool/releases/tag/v1.2.0"}, nil
	}

	selectorCalled := false
	cmd.selectItems = func(updates []availableUpdate) ([]availableUpdate, error) {
		selectorCalled = true
		return updates, nil
	}

	cmd.cmd.SetArgs([]string{"--dry-run", outdatedPath})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected dry-run command to return an error")
	}
	if !strings.Contains(err.Error(), "dry-run mode") {
		t.Fatalf("unexpected error: %v", err)
	}
	if selectorCalled {
		t.Fatal("did not expect interactive selector to be called when args are provided")
	}
}

func TestUpdateDryRunNoArgsSkipsInteractiveSelector(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-update-dryrun-tool")
	writeTestBinary(t, outdatedPath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     outdatedPath,
		Version:  "1.0.0",
		URL:      "https://example.com/generic-update-dryrun-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		return mockProvider{latestVersion: "1.1.0", latestVersionURL: "https://example.com/generic-update-dryrun-tool/releases/tag/v1.1.0"}, nil
	}

	selectorCalled := false
	cmd.selectItems = func(updates []availableUpdate) ([]availableUpdate, error) {
		selectorCalled = true
		return updates, nil
	}

	cmd.cmd.SetArgs([]string{"--dry-run"})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected dry-run command to return an error")
	}
	if !strings.Contains(err.Error(), "dry-run mode") {
		t.Fatalf("unexpected error: %v", err)
	}
	if selectorCalled {
		t.Fatal("did not expect interactive selector to be called with --dry-run")
	}
}

func TestUpdateYesFlagNoArgsSkipsInteractiveSelector(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-update-yes-tool")
	writeTestBinary(t, outdatedPath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     outdatedPath,
		Version:  "1.0.0",
		URL:      "https://example.com/generic-update-yes-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		return mockProvider{latestVersion: "1.1.0", latestVersionURL: "https://example.com/generic-update-yes-tool/releases/tag/v1.1.0"}, nil
	}

	selectorCalled := false
	cmd.selectItems = func(updates []availableUpdate) ([]availableUpdate, error) {
		selectorCalled = true
		return updates, nil
	}

	// Inject a dry-run to avoid actually downloading in case --yes bypass fails.
	cmd.cmd.SetArgs([]string{"--yes", "--dry-run"})
	cmd.cmd.Execute() //nolint:errcheck

	if selectorCalled {
		t.Fatal("did not expect interactive selector to be called with --yes")
	}
}
