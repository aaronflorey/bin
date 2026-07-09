package cmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
)

type mockProvider struct {
	providers.Provider
	id               string
	latestVersion    string
	latestVersionURL string
	publishedAt      *time.Time
	release          *providers.ReleaseInfo
	history          []*providers.ReleaseInfo
	historyErr       error
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

func (m mockProvider) ListReleases(limit int) ([]*providers.ReleaseInfo, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	if limit > 0 && len(m.history) > limit {
		return m.history[:limit], nil
	}
	return m.history, nil
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

func TestGetLatestVersionUsesStoredReleaseTagPrefix(t *testing.T) {
	b := &config.Binary{
		Path:             "/home/user/bin/tool",
		Version:          "v1.0.0",
		URL:              "github.com/acme/tool",
		Provider:         "github",
		ReleaseTagPrefix: "pi-v",
	}

	p := mockProvider{
		history: []*providers.ReleaseInfo{
			{Version: "v2.0.0", URL: "https://example.test/core"},
			{Version: "pi-v1.1.0", URL: "https://example.test/pi"},
		},
	}

	got, err := getLatestVersion(b, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected update info")
	}
	if got.version != "pi-v1.1.0" {
		t.Fatalf("unexpected version: %s", got.version)
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

func TestUpdateRequiresYesInNonInteractiveMode(t *testing.T) {
	setupTestConfig(t)

	outdatedPath := filepath.Join(t.TempDir(), "generic-update-noninteractive-tool")
	writeTestBinary(t, outdatedPath)
	if err := config.UpsertBinary(&config.Binary{
		Path:     outdatedPath,
		Version:  "1.0.0",
		URL:      "https://example.com/generic-update-noninteractive-tool",
		Provider: "github",
	}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		if u != "https://example.com/generic-update-noninteractive-tool" {
			return nil, fmt.Errorf("unexpected provider request for %s", u)
		}
		return mockProvider{latestVersion: "1.1.0", latestVersionURL: "https://example.com/generic-update-noninteractive-tool/releases/tag/v1.1.0"}, nil
	}

	confirmCalled := false
	cmd.confirm = func(string) error {
		confirmCalled = true
		return nil
	}
	cmd.isInteractive = func() bool {
		return false
	}

	cmd.cmd.SetArgs([]string{outdatedPath})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected update to require --yes in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "requires --yes or --dry-run") {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmCalled {
		t.Fatal("did not expect confirmation prompt in non-interactive mode")
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

func TestUpdateContinuesAfterInstallFailureWithYesFlag(t *testing.T) {
	setupTestConfig(t)

	paths := seedOutdatedUpdateBinaries(t, []string{
		"alpha-update-install-failure-tool",
		"beta-update-install-failure-tool",
		"gamma-update-install-failure-tool",
	})

	cmd := newUpdateCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/alpha-update-install-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/alpha-update-install-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/beta-update-install-failure-tool":  {latestVersion: "1.1.0", latestVersionURL: "https://example.com/beta-update-install-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/gamma-update-install-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/gamma-update-install-failure-tool/releases/tag/v1.1.0"},
	})

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	var attemptedPaths []string
	var succeededPaths []string
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			applyStoredFetch: func(_ *config.Binary, _ *providers.FetchOpts) error {
				return nil
			},
			install: func(opts InstallOpts) (*InstallResult, error) {
				attemptedPaths = append(attemptedPaths, opts.Path)
				if opts.Path == paths[0] {
					return nil, fmt.Errorf("mock install failure for %s", opts.Path)
				}
				succeededPaths = append(succeededPaths, opts.Path)
				return &InstallResult{Name: filepath.Base(opts.Path), Version: opts.FetchOpts.Version, Path: opts.Path}, nil
			},
			resolvePath: originalRegistry[installModeBinary].resolvePath,
		},
	}

	cmd.cmd.SetArgs([]string{"--yes", "--parallelism=1"})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected some updates failed error")
	}
	if !strings.Contains(err.Error(), "some updates failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(attemptedPaths, paths) {
		t.Fatalf("unexpected install attempt order: got %v want %v", attemptedPaths, paths)
	}
	if len(succeededPaths) < 1 {
		t.Fatalf("expected at least one later successful update, got %v", succeededPaths)
	}
	if succeededPaths[0] != paths[1] {
		t.Fatalf("expected first success after failure to be %s, got %v", paths[1], succeededPaths)
	}
}

func TestUpdateContinuesAfterApplyStoredFetchFailureWithYesFlag(t *testing.T) {
	setupTestConfig(t)

	paths := seedOutdatedUpdateBinaries(t, []string{
		"alpha-update-fetch-failure-tool",
		"beta-update-fetch-failure-tool",
		"gamma-update-fetch-failure-tool",
	})

	cmd := newUpdateCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/alpha-update-fetch-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/alpha-update-fetch-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/beta-update-fetch-failure-tool":  {latestVersion: "1.1.0", latestVersionURL: "https://example.com/beta-update-fetch-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/gamma-update-fetch-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/gamma-update-fetch-failure-tool/releases/tag/v1.1.0"},
	})

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	var attemptedPaths []string
	var installedPaths []string
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			applyStoredFetch: func(b *config.Binary, _ *providers.FetchOpts) error {
				attemptedPaths = append(attemptedPaths, b.Path)
				if b.Path == paths[0] {
					return fmt.Errorf("mock stored fetch failure for %s", b.Path)
				}
				return nil
			},
			install: func(opts InstallOpts) (*InstallResult, error) {
				installedPaths = append(installedPaths, opts.Path)
				return &InstallResult{Name: filepath.Base(opts.Path), Version: opts.FetchOpts.Version, Path: opts.Path}, nil
			},
			resolvePath: originalRegistry[installModeBinary].resolvePath,
		},
	}

	cmd.cmd.SetArgs([]string{"--yes", "--parallelism=1"})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected some updates failed error")
	}
	if !strings.Contains(err.Error(), "some updates failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(attemptedPaths, paths) {
		t.Fatalf("unexpected update attempt order: got %v want %v", attemptedPaths, paths)
	}
	if !reflect.DeepEqual(installedPaths, paths[1:]) {
		t.Fatalf("expected later installs to continue after applyStoredFetch failure, got %v want %v", installedPaths, paths[1:])
	}
}

func TestUpdateRecordsPerBinaryFailuresAndContinuesWithYesFlag(t *testing.T) {
	setupTestConfig(t)

	paths := seedOutdatedUpdateBinaries(t, []string{
		"alpha-update-stored-metadata-failure-tool",
		"beta-update-compatibility-failure-tool",
		"gamma-update-fetch-failure-tool",
		"delta-update-save-failure-tool",
		"epsilon-update-install-failure-tool",
		"zeta-update-success-tool",
	})

	if err := config.UpsertBinary(&config.Binary{
		Path:        paths[1],
		Version:     "1.0.0",
		URL:         "https://example.com/beta-update-compatibility-failure-tool",
		Provider:    "github",
		InstallMode: installModeSystemPackage,
		PackageType: "deb",
	}); err != nil {
		t.Fatalf("failed to seed system package binary: %v", err)
	}

	cmd := newUpdateCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/alpha-update-stored-metadata-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/alpha-update-stored-metadata-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/beta-update-compatibility-failure-tool":    {latestVersion: "1.1.0", latestVersionURL: "https://example.com/beta-update-compatibility-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/gamma-update-fetch-failure-tool":           {latestVersion: "1.1.0", latestVersionURL: "https://example.com/gamma-update-fetch-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/delta-update-save-failure-tool":            {latestVersion: "1.1.0", latestVersionURL: "https://example.com/delta-update-save-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/epsilon-update-install-failure-tool":       {latestVersion: "1.1.0", latestVersionURL: "https://example.com/epsilon-update-install-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/zeta-update-success-tool":                  {latestVersion: "1.1.0", latestVersionURL: "https://example.com/zeta-update-success-tool/releases/tag/v1.1.0"},
	})

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	originalLog := log.Log
	defer func() {
		log.Log = originalLog
	}()
	var logOutput bytes.Buffer
	log.Log = log.New(&logOutput)

	var applyStoredFetchPaths []string
	var installAttemptPaths []string
	var installedPaths []string
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			applyStoredFetch: func(b *config.Binary, _ *providers.FetchOpts) error {
				applyStoredFetchPaths = append(applyStoredFetchPaths, b.Path)
				if b.Path == paths[0] {
					return fmt.Errorf("mock stored metadata failure")
				}
				return nil
			},
			install: func(opts InstallOpts) (*InstallResult, error) {
				installAttemptPaths = append(installAttemptPaths, opts.Path)
				switch opts.Path {
				case paths[2]:
					return nil, fmt.Errorf("mock fetch failure")
				case paths[3]:
					return nil, fmt.Errorf("error installing binary: mock save failure")
				case paths[4]:
					return nil, fmt.Errorf("mock install failure")
				default:
					installedPaths = append(installedPaths, opts.Path)
					return &InstallResult{Name: filepath.Base(opts.Path), Version: opts.FetchOpts.Version, Path: opts.Path}, nil
				}
			},
			resolvePath: originalRegistry[installModeBinary].resolvePath,
		},
		installModeSystemPackage: {
			applyStoredFetch: func(b *config.Binary, _ *providers.FetchOpts) error {
				applyStoredFetchPaths = append(applyStoredFetchPaths, b.Path)
				return nil
			},
			install: func(opts InstallOpts) (*InstallResult, error) {
				installAttemptPaths = append(installAttemptPaths, opts.Path)
				return nil, assets.ErrNoCompatibleFiles
			},
			resolvePath: originalRegistry[installModeSystemPackage].resolvePath,
		},
	}

	cmd.cmd.SetArgs([]string{"--yes", "--parallelism=1"})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected some updates failed error")
	}
	if !strings.Contains(err.Error(), "some updates failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(applyStoredFetchPaths, paths) {
		t.Fatalf("unexpected applyStoredFetch order: got %v want %v", applyStoredFetchPaths, paths)
	}
	if !reflect.DeepEqual(installAttemptPaths, paths[1:]) {
		t.Fatalf("unexpected install attempt order: got %v want %v", installAttemptPaths, paths[1:])
	}
	if !reflect.DeepEqual(installedPaths, []string{paths[5]}) {
		t.Fatalf("expected only final binary to install successfully, got %v", installedPaths)
	}

	logs := logOutput.String()
	for _, path := range paths[:5] {
		if !strings.Contains(logs, path) {
			t.Fatalf("expected warning logs to mention failed binary %s, logs: %s", path, logs)
		}
	}
	if !strings.Contains(logs, "latest release no longer exposes a compatible deb package") {
		t.Fatalf("expected compatibility failure details in logs, got: %s", logs)
	}
	if got := strings.Count(logs, "latest release no longer exposes a compatible deb package"); got != 1 {
		t.Fatalf("expected compatibility failure warning exactly once, got %d occurrences in logs: %s", got, logs)
	}
}

func TestUpdateStopsAfterInstallFailureWhenContinueOnErrorDisabled(t *testing.T) {
	setupTestConfig(t)

	paths := seedOutdatedUpdateBinaries(t, []string{
		"alpha-update-stop-on-install-failure-tool",
		"beta-update-stop-on-install-failure-tool",
		"gamma-update-stop-on-install-failure-tool",
	})

	cmd := newUpdateCmd()
	cmd.newProvider = newMockOutdatedProviderFactory(t, map[string]mockProvider{
		"https://example.com/alpha-update-stop-on-install-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/alpha-update-stop-on-install-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/beta-update-stop-on-install-failure-tool":  {latestVersion: "1.1.0", latestVersionURL: "https://example.com/beta-update-stop-on-install-failure-tool/releases/tag/v1.1.0"},
		"https://example.com/gamma-update-stop-on-install-failure-tool": {latestVersion: "1.1.0", latestVersionURL: "https://example.com/gamma-update-stop-on-install-failure-tool/releases/tag/v1.1.0"},
	})

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	var attemptedPaths []string
	expectedErr := fmt.Errorf("mock install failure for %s", paths[0])
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			applyStoredFetch: func(_ *config.Binary, _ *providers.FetchOpts) error {
				return nil
			},
			install: func(opts InstallOpts) (*InstallResult, error) {
				attemptedPaths = append(attemptedPaths, opts.Path)
				if opts.Path == paths[0] {
					return nil, expectedErr
				}
				return &InstallResult{Name: filepath.Base(opts.Path), Version: opts.FetchOpts.Version, Path: opts.Path}, nil
			},
			resolvePath: originalRegistry[installModeBinary].resolvePath,
		},
	}

	cmd.cmd.SetArgs([]string{"--yes", "--parallelism=1", "--continue-on-error=false"})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected first install failure")
	}
	if err != expectedErr {
		t.Fatalf("expected original install failure, got %v", err)
	}
	if !reflect.DeepEqual(attemptedPaths, paths[:1]) {
		t.Fatalf("expected only first install attempt, got %v want %v", attemptedPaths, paths[:1])
	}
	if strings.Contains(err.Error(), "some updates failed") {
		t.Fatalf("expected original failure instead of aggregate error, got %v", err)
	}
}

func TestUpdateInteractivePromptPreservesDiscoveryFailuresForFinalExit(t *testing.T) {
	setupTestConfig(t)

	paths := seedOutdatedUpdateBinaries(t, []string{
		"alpha-update-discovery-failure-tool",
		"beta-update-selected-tool",
		"gamma-update-unselected-tool",
	})

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		switch u {
		case "https://example.com/alpha-update-discovery-failure-tool":
			return nil, fmt.Errorf("mock discovery failure for %s", u)
		case "https://example.com/beta-update-selected-tool":
			return mockProvider{latestVersion: "1.1.0", latestVersionURL: u + "/releases/tag/v1.1.0"}, nil
		case "https://example.com/gamma-update-unselected-tool":
			return mockProvider{latestVersion: "1.1.0", latestVersionURL: u + "/releases/tag/v1.1.0"}, nil
		default:
			return nil, fmt.Errorf("unexpected provider request for %s", u)
		}
	}

	selectorCalled := false
	cmd.selectItems = func(updates []availableUpdate) ([]availableUpdate, error) {
		selectorCalled = true
		if len(updates) != 2 {
			t.Fatalf("expected 2 selectable updates after discovery failure, got %d", len(updates))
		}
		return updates[:1], nil
	}

	confirmCalled := false
	cmd.confirm = func(string) error {
		confirmCalled = true
		return nil
	}
	cmd.isInteractive = func() bool {
		return true
	}

	originalRegistry := lifecycleRegistry
	defer func() {
		lifecycleRegistry = originalRegistry
	}()

	var installedPaths []string
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeBinary: {
			applyStoredFetch: func(_ *config.Binary, _ *providers.FetchOpts) error {
				return nil
			},
			install: func(opts InstallOpts) (*InstallResult, error) {
				installedPaths = append(installedPaths, opts.Path)
				return &InstallResult{Name: filepath.Base(opts.Path), Version: opts.FetchOpts.Version, Path: opts.Path}, nil
			},
			resolvePath: originalRegistry[installModeBinary].resolvePath,
		},
	}

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected discovery failure to produce a final aggregate error")
	}
	exitErr, ok := err.(*exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T: %v", err, err)
	}
	if exitErr.code != 4 {
		t.Fatalf("expected exit code 4, got %d", exitErr.code)
	}
	if !strings.Contains(err.Error(), "some updates failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !selectorCalled {
		t.Fatal("expected interactive selector to be called")
	}
	if !confirmCalled {
		t.Fatal("expected confirmation prompt to be called")
	}
	if !reflect.DeepEqual(installedPaths, []string{paths[1]}) {
		t.Fatalf("expected selected update to install despite discovery failure, got %v want %v", installedPaths, []string{paths[1]})
	}
}

func TestUpdateInteractivePromptNoSelectionPreservesDiscoveryFailuresForFinalExit(t *testing.T) {
	setupTestConfig(t)

	seedOutdatedUpdateBinaries(t, []string{
		"alpha-update-discovery-failure-tool",
		"beta-update-unselected-tool",
	})

	cmd := newUpdateCmd()
	cmd.newProvider = func(u, _ string) (providers.Provider, error) {
		switch u {
		case "https://example.com/alpha-update-discovery-failure-tool":
			return nil, fmt.Errorf("mock discovery failure for %s", u)
		case "https://example.com/beta-update-unselected-tool":
			return mockProvider{latestVersion: "1.1.0", latestVersionURL: u + "/releases/tag/v1.1.0"}, nil
		default:
			return nil, fmt.Errorf("unexpected provider request for %s", u)
		}
	}

	selectorCalled := false
	cmd.selectItems = func(updates []availableUpdate) ([]availableUpdate, error) {
		selectorCalled = true
		if len(updates) != 1 {
			t.Fatalf("expected 1 selectable update after discovery failure, got %d", len(updates))
		}
		return nil, nil
	}
	cmd.confirm = func(string) error {
		t.Fatal("did not expect confirmation prompt when nothing was selected")
		return nil
	}
	cmd.isInteractive = func() bool {
		return true
	}

	originalLog := log.Log
	defer func() {
		log.Log = originalLog
	}()
	var logOutput bytes.Buffer
	log.Log = log.New(&logOutput)

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected discovery failure to produce a final aggregate error")
	}
	exitErr, ok := err.(*exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T: %v", err, err)
	}
	if exitErr.code != 4 {
		t.Fatalf("expected exit code 4, got %d", exitErr.code)
	}
	if !strings.Contains(err.Error(), "some updates failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !selectorCalled {
		t.Fatal("expected interactive selector to be called")
	}

	logs := logOutput.String()
	if !strings.Contains(logs, "mock discovery failure for https://example.com/alpha-update-discovery-failure-tool") {
		t.Fatalf("expected discovery failure warning in logs, got: %s", logs)
	}
	if !strings.Contains(logs, "No binaries selected for update") {
		t.Fatalf("expected no-selection message in logs, got: %s", logs)
	}
}

func seedOutdatedUpdateBinaries(t *testing.T, names []string) []string {
	t.Helper()

	paths := make([]string, 0, len(names))
	for _, name := range names {
		path := filepath.Join(t.TempDir(), name)
		writeTestBinary(t, path)
		if err := config.UpsertBinary(&config.Binary{
			Path:     path,
			Version:  "1.0.0",
			URL:      fmt.Sprintf("https://example.com/%s", name),
			Provider: "github",
		}); err != nil {
			t.Fatalf("failed to seed binary %s: %v", name, err)
		}
		paths = append(paths, path)
	}
	return paths
}
