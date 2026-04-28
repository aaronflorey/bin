package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type noHistoryProvider struct {
	id string
}

func (p noHistoryProvider) Fetch(*providers.FetchOpts) (*providers.File, error) {
	return nil, nil
}

func (p noHistoryProvider) GetLatestVersion() (*providers.ReleaseInfo, error) {
	return nil, nil
}

func (p noHistoryProvider) Cleanup(*providers.CleanupOpts) error {
	return nil
}

func (p noHistoryProvider) GetID() string {
	return p.id
}

func TestRootExecuteLaunchesTUIForZeroArgs(t *testing.T) {
	root := newRootCmd("test", func(int) {})

	loadCalled := false
	launchCalled := false
	exitCode := -1
	root.exit = func(code int) {
		exitCode = code
	}
	root.shouldLaunchTUI = func(args []string) bool {
		return len(args) == 0
	}
	root.loadConfig = func() error {
		loadCalled = true
		return nil
	}
	root.launchTUI = func() error {
		launchCalled = true
		return nil
	}

	root.Execute(nil)

	if !loadCalled {
		t.Fatal("expected zero-arg execution to load config before launching TUI")
	}
	if !launchCalled {
		t.Fatal("expected zero-arg execution to launch TUI")
	}
	if exitCode != -1 {
		t.Fatalf("expected no exit call, got %d", exitCode)
	}
}

func TestRootExecuteFallsBackToListWhenTUIIsNotLaunched(t *testing.T) {
	root := newRootCmd("test", func(int) {})

	called := false
	root.shouldLaunchTUI = func([]string) bool { return false }
	root.cmd = &cobra.Command{Use: "bin", SilenceUsage: true, SilenceErrors: true}
	root.cmd.AddCommand(&cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			called = true
			return nil
		},
	})

	root.Execute(nil)

	if !called {
		t.Fatal("expected zero-arg execution to fall back to list")
	}
}

func TestLoadChangelogDetailFiltersAndOrdersReleases(t *testing.T) {
	target := changelogTarget{
		Binary: &config.Binary{
			Path:     "/tmp/tool",
			Version:  "1.2.3",
			URL:      "https://example.test/tool",
			Provider: "github",
		},
		Name:           "tool",
		CurrentVersion: "1.2.3",
		LatestVersion:  "1.5.0",
		ProviderID:     "github",
	}

	newProvider := func(u, provider string) (providers.Provider, error) {
		return mockProvider{
			id: "github",
			history: []*providers.ReleaseInfo{
				{Version: "1.5.0", Body: "Improved startup performance."},
				{Version: "1.4.0", Body: "Fixed proxy regressions."},
				{Version: "1.3.0", Body: "Added config support."},
				{Version: "1.2.3", Body: "Current install."},
			},
		}, nil
	}

	detail, err := loadChangelogDetail(target, newProvider)
	if err != nil {
		t.Fatalf("loadChangelogDetail returned error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected changelog detail")
	}
	if len(detail.Releases) != 3 {
		t.Fatalf("expected 3 releases, got %d", len(detail.Releases))
	}
	got := []string{detail.Releases[0].Version, detail.Releases[1].Version, detail.Releases[2].Version}
	want := []string{"1.3.0", "1.4.0", "1.5.0"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected release order: got %v want %v", got, want)
	}
	if detail.SelectedIndex != 0 {
		t.Fatalf("expected first update selected by default, got %d", detail.SelectedIndex)
	}
	if !strings.Contains(detail.Releases[0].Summary, "Added config support") {
		t.Fatalf("unexpected summary: %q", detail.Releases[0].Summary)
	}
}

func TestLoadChangelogDetailHandlesUnsupportedProvider(t *testing.T) {
	target := changelogTarget{
		Binary: &config.Binary{
			Path:     "/tmp/tool",
			Version:  "1.0.0",
			URL:      "https://example.test/tool",
			Provider: "generic",
		},
		Name:           "tool",
		CurrentVersion: "1.0.0",
		LatestVersion:  "1.1.0",
		ProviderID:     "generic",
	}

	newProvider := func(u, provider string) (providers.Provider, error) {
		return noHistoryProvider{id: "generic"}, nil
	}

	detail, err := loadChangelogDetail(target, newProvider)
	if err != nil {
		t.Fatalf("loadChangelogDetail returned error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected fallback detail")
	}
	if detail.Notice == "" {
		t.Fatal("expected unsupported-provider notice")
	}
	if !strings.Contains(detail.Notice, "provider \"generic\"") {
		t.Fatalf("unexpected notice: %q", detail.Notice)
	}
}

func TestOutdatedLoadedMsgOpensDetailView(t *testing.T) {
	model := newTUIModel(nil)
	target := changelogTarget{
		Binary: &config.Binary{Path: "/tmp/tool", Version: "1.0.0"},
		Name:   "tool",
	}

	updatedModel, cmd := model.Update(outdatedLoadedMsg{targets: []changelogTarget{target}})
	updated := updatedModel.(tuiModel)

	if updated.screen != tuiScreenDetail {
		t.Fatalf("expected detail screen, got %v", updated.screen)
	}
	if updated.targetCursor != 0 {
		t.Fatalf("expected first target selected, got %d", updated.targetCursor)
	}
	if len(updated.targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(updated.targets))
	}
	if !updated.detailLoading {
		t.Fatal("expected detail pane to remain in loading state")
	}
	if updated.detail == nil {
		t.Fatal("expected placeholder detail while loading")
	}
	if updated.detail.Target.Name != "tool" {
		t.Fatalf("expected placeholder detail target to be tool, got %q", updated.detail.Target.Name)
	}
	if cmd == nil {
		t.Fatal("expected detail loading command")
	}
}

func TestRenderMarkdownBodyRendersStructuredMarkdown(t *testing.T) {
	body := strings.Join([]string{
		"# Added",
		"",
		"- **Fast** startup for `bin install`",
		"- Docs at [site](https://example.test/docs)",
		"",
		"```sh",
		"bin update",
		"```",
	}, "\n")

	lines := renderMarkdownBody(body, 80)
	joined := stripANSI(strings.Join(lines, "\n"))

	checks := []string{
		"Added",
		"Fast",
		"bin install",
		"site (https://example.test/docs)",
		"bin update",
	}
	for _, want := range checks {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected rendered markdown to contain %q, got %q", want, joined)
		}
	}

	for _, unwanted := range []string{"```", "**", "[site](https://example.test/docs)"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("expected rendered markdown to omit %q, got %q", unwanted, joined)
		}
	}
}

func stripANSI(value string) string {
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiPattern.ReplaceAllString(value, "")
}
