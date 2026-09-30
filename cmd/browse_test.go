package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/providers"
)

type browseHistoryTestProvider struct {
	id       string
	fetches  int
	limit    int
	releases []*providers.ReleaseInfo
	err      error
}

func (p *browseHistoryTestProvider) Fetch(*providers.FetchOpts) (*providers.File, error) {
	p.fetches++
	return nil, nil
}

func (*browseHistoryTestProvider) GetLatestVersion() (*providers.ReleaseInfo, error) { return nil, nil }
func (*browseHistoryTestProvider) Cleanup(*providers.CleanupOpts) error              { return nil }
func (p *browseHistoryTestProvider) GetID() string {
	if p.id == "" {
		return "github"
	}
	return p.id
}

func (p *browseHistoryTestProvider) ListReleases(limit int) ([]*providers.ReleaseInfo, error) {
	p.limit = limit
	return p.releases, p.err
}

func runBrowseCommand(t *testing.T, browse *browseCmd, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	browse.cmd.SetOut(&output)
	browse.cmd.SetArgs(args)
	err := browse.cmd.Execute()
	return output.String(), err
}

func browseRelease(version, url string, names ...string) *providers.ReleaseInfo {
	return &providers.ReleaseInfo{Version: version, URL: url, Assets: names}
}

func browsePlatformAsset(version string) string {
	return fmt.Sprintf("tool_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

func browseOtherPlatformAsset(version string) string {
	otherOS := "linux"
	if runtime.GOOS == "linux" {
		otherOS = "darwin"
	}
	return fmt.Sprintf("tool_%s_%s_%s.tar.gz", version, otherOS, runtime.GOARCH)
}

func TestBrowseListsOnlyMetadataCompatibleAssetsWithoutFetching(t *testing.T) {
	provider := &browseHistoryTestProvider{releases: []*providers.ReleaseInfo{
		browseRelease("v1.2.3", "https://github.com/acme/tool/releases/tag/v1.2.3",
			browsePlatformAsset("1.2.3"), "tool_1.2.3.sha256", browseOtherPlatformAsset("1.2.3")),
	}}
	browse := newBrowseCmd()
	browse.newProvider = func(gotURL, gotProvider string) (providers.Provider, error) {
		if gotURL != "github.com/acme/tool" || gotProvider != "" {
			t.Fatalf("provider factory got %q, %q", gotURL, gotProvider)
		}
		return provider, nil
	}
	browse.isInteractive = func(io.Writer) bool { return false }
	browse.selectRelease = func(string, []fmt.Stringer) (interface{}, error) {
		t.Fatal("browse prompted without an interactive terminal")
		return nil, nil
	}

	output, err := runBrowseCommand(t, browse, "https://github.com/acme/tool")
	if err != nil {
		t.Fatalf("browse error = %v", err)
	}
	if provider.limit != browseReleaseHistoryLimit {
		t.Fatalf("history limit = %d, want %d", provider.limit, browseReleaseHistoryLimit)
	}
	if provider.fetches != 0 {
		t.Fatalf("browse fetched %d release payloads, want none", provider.fetches)
	}
	if !strings.Contains(output, browsePlatformAsset("1.2.3")) || strings.Contains(output, "tool_1.2.3.sha256") || strings.Contains(output, browseOtherPlatformAsset("1.2.3")) {
		t.Fatalf("unexpected compatible-asset listing: %s", output)
	}
	if !strings.Contains(output, "asset compatibility is metadata-only") || !strings.Contains(output, "payloads are not validated") {
		t.Fatalf("browse did not explain metadata-only compatibility: %s", output)
	}
	if !strings.Contains(output, "bin install 'https://github.com/acme/tool/releases/tag/v1.2.3' --select '") {
		t.Fatalf("browse omitted install guidance: %s", output)
	}
	if strings.Contains(output, "--provider") {
		t.Fatalf("browse added a provider flag without an explicit --provider: %s", output)
	}
}

func TestBrowseInteractiveSelectionAndNonInteractiveListing(t *testing.T) {
	releases := []*providers.ReleaseInfo{
		browseRelease("v1.0.0", "https://github.com/acme/tool/releases/tag/v1.0.0", browsePlatformAsset("1.0.0")),
		browseRelease("v2.0.0", "https://github.com/acme/tool/releases/tag/v2.0.0", browsePlatformAsset("2.0.0")),
	}

	t.Run("interactive choice shows selected release", func(t *testing.T) {
		browse := newBrowseCmd()
		browse.newProvider = func(string, string) (providers.Provider, error) {
			return &browseHistoryTestProvider{releases: releases}, nil
		}
		browse.isInteractive = func(io.Writer) bool { return true }
		prompted := false
		browse.selectRelease = func(message string, choices []fmt.Stringer) (interface{}, error) {
			prompted = true
			if message != "Select a release to browse:" || len(choices) != 2 || choices[1].String() != "v2.0.0" {
				t.Fatalf("release prompt = %q, choices = %v", message, choices)
			}
			return choices[1], nil
		}
		output, err := runBrowseCommand(t, browse, "github.com/acme/tool")
		if err != nil {
			t.Fatalf("browse error = %v", err)
		}
		if !prompted || !strings.Contains(output, `Release "v2.0.0"`) || strings.Contains(output, `Release "v1.0.0"`) {
			t.Fatalf("interactive browse output or prompt mismatch: prompted=%t output=%s", prompted, output)
		}
	})

	t.Run("explicit non-interactive flag lists all recent releases", func(t *testing.T) {
		browse := newBrowseCmd()
		browse.newProvider = func(string, string) (providers.Provider, error) {
			return &browseHistoryTestProvider{releases: releases}, nil
		}
		browse.isInteractive = func(io.Writer) bool { return true }
		browse.selectRelease = func(string, []fmt.Stringer) (interface{}, error) {
			t.Fatal("--non-interactive invoked the selector")
			return nil, nil
		}
		output, err := runBrowseCommand(t, browse, "--non-interactive", "github.com/acme/tool")
		if err != nil {
			t.Fatalf("browse error = %v", err)
		}
		if !strings.Contains(output, `Release "v1.0.0"`) || !strings.Contains(output, `Release "v2.0.0"`) {
			t.Fatalf("non-interactive browse did not list both releases: %s", output)
		}
	})
}

func TestBrowseRedirectedOutputDoesNotPrompt(t *testing.T) {
	browse := newBrowseCmd()
	browse.newProvider = func(string, string) (providers.Provider, error) {
		return &browseHistoryTestProvider{releases: []*providers.ReleaseInfo{
			browseRelease("v1.0", "https://github.com/acme/tool/releases/tag/v1.0"),
		}}, nil
	}
	browse.isInteractive = browseOutputIsInteractive
	browse.selectRelease = func(string, []fmt.Stringer) (interface{}, error) {
		t.Fatal("redirected output invoked the selector")
		return nil, nil
	}

	if _, err := runBrowseCommand(t, browse, "github.com/acme/tool"); err != nil {
		t.Fatalf("browse error = %v", err)
	}
}

func TestBrowseSelectsRequestedReleaseFromBoundedHistory(t *testing.T) {
	provider := &browseHistoryTestProvider{}
	for i := 0; i < browseReleaseHistoryLimit+1; i++ {
		version := fmt.Sprintf("v%02d", i)
		provider.releases = append(provider.releases, browseRelease(version, "https://github.com/acme/tool/releases/tag/"+version))
	}
	browse := newBrowseCmd()
	browse.newProvider = func(gotURL, _ string) (providers.Provider, error) {
		if gotURL != "github.com/acme/tool" {
			t.Fatalf("normalized provider URL = %q", gotURL)
		}
		return provider, nil
	}
	browse.isInteractive = func(io.Writer) bool { return true }
	browse.selectRelease = func(string, []fmt.Stringer) (interface{}, error) {
		t.Fatal("explicit tag prompted for a release")
		return nil, nil
	}

	output, err := runBrowseCommand(t, browse, "--version", "v03", "github.com/acme/tool")
	if err != nil {
		t.Fatalf("browse error = %v", err)
	}
	if !strings.Contains(output, `Release "v03"`) || strings.Contains(output, `Release "v20"`) {
		t.Fatalf("requested version or history bound not respected: %s", output)
	}
	if provider.limit != browseReleaseHistoryLimit || provider.fetches != 0 {
		t.Fatalf("history limit/fetches = %d/%d", provider.limit, provider.fetches)
	}
}

func TestBrowseRejectsRequestedTagOutsideRecentHistory(t *testing.T) {
	provider := &browseHistoryTestProvider{}
	for i := 0; i < browseReleaseHistoryLimit+1; i++ {
		version := fmt.Sprintf("v%02d", i)
		provider.releases = append(provider.releases, browseRelease(version, "https://github.com/acme/tool/releases/tag/"+version))
	}
	browse := newBrowseCmd()
	browse.newProvider = func(string, string) (providers.Provider, error) { return provider, nil }

	_, err := runBrowseCommand(t, browse, "--version", "v20", "github.com/acme/tool")
	if err == nil || !strings.Contains(err.Error(), "most recent 20 releases") {
		t.Fatalf("out-of-window version error = %v", err)
	}
}

func TestBrowseUsesExplicitReleaseURLTagAndErrorsWhenAbsent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		versions []string
		wantErr  string
	}{
		{name: "present", versions: []string{"v1.0", "v2.0"}},
		{name: "missing", versions: []string{"v1.0"}, wantErr: `release tag "v2.0" was not found`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &browseHistoryTestProvider{}
			for _, version := range tc.versions {
				provider.releases = append(provider.releases, browseRelease(version,
					"https://github.com/acme/tool/releases/tag/"+version, browsePlatformAsset(strings.TrimPrefix(version, "v"))))
			}
			browse := newBrowseCmd()
			browse.newProvider = func(gotURL, _ string) (providers.Provider, error) {
				if gotURL != "github.com/acme/tool" {
					t.Fatalf("normalized provider URL = %q", gotURL)
				}
				return provider, nil
			}
			browse.isInteractive = func(io.Writer) bool { return true }
			browse.selectRelease = func(string, []fmt.Stringer) (interface{}, error) {
				t.Fatal("release URL tag prompted for a selection")
				return nil, nil
			}
			output, err := runBrowseCommand(t, browse, "https://github.com/acme/tool/releases/tag/v2.0")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("browse error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || !strings.Contains(output, `Release "v2.0"`) || strings.Contains(output, `Release "v1.0"`) {
				t.Fatalf("browse output/error = %s / %v", output, err)
			}
		})
	}
}

func TestBrowseUsesExplicitGitLabAndCodebergReleaseTags(t *testing.T) {
	for _, tc := range []struct {
		providerID string
		source     string
	}{
		{providerID: "gitlab", source: "https://gitlab.com/acme/tool/-/releases/v2.0?channel=stable"},
		{providerID: "codeberg", source: "https://codeberg.org/acme/tool/releases/tag/v2.0"},
	} {
		t.Run(tc.providerID, func(t *testing.T) {
			provider := &browseHistoryTestProvider{
				id: tc.providerID,
				releases: []*providers.ReleaseInfo{
					browseRelease("v1.0", "https://example.test/releases/tag/v1.0"),
					browseRelease("v2.0", "https://example.test/releases/tag/v2.0"),
				},
			}
			browse := newBrowseCmd()
			browse.newProvider = func(gotURL, _ string) (providers.Provider, error) {
				if gotURL != tc.source {
					t.Fatalf("provider URL = %q, want original URL %q", gotURL, tc.source)
				}
				return provider, nil
			}
			browse.isInteractive = func(io.Writer) bool { return true }
			browse.selectRelease = func(string, []fmt.Stringer) (interface{}, error) {
				t.Fatal("explicit provider release URL prompted for a selection")
				return nil, nil
			}

			output, err := runBrowseCommand(t, browse, tc.source)
			if err != nil || !strings.Contains(output, `Release "v2.0"`) || strings.Contains(output, `Release "v1.0"`) {
				t.Fatalf("browse output/error = %s / %v", output, err)
			}
		})
	}
}

func TestBrowseForcedGitLabGuidancePreservesProviderAndUsesInstallableReleaseURL(t *testing.T) {
	source := "https://example.com/acme/tool?source=private"
	asset := browsePlatformAsset("1.2.3")
	for _, metadataURL := range []string{
		"/-/tags/v1.2.3",
		"https://example.com/api/v4/projects/42/releases/v1.2.3",
	} {
		t.Run(metadataURL, func(t *testing.T) {
			provider := &browseHistoryTestProvider{
				id: "gitlab",
				releases: []*providers.ReleaseInfo{
					browseRelease("v1.2.3", metadataURL, asset),
				},
			}
			browse := newBrowseCmd()
			browse.newProvider = func(gotURL, gotProvider string) (providers.Provider, error) {
				if gotURL != source || gotProvider != "gitlab" {
					t.Fatalf("provider factory got %q, %q; want %q, gitlab", gotURL, gotProvider, source)
				}
				return provider, nil
			}
			browse.isInteractive = func(io.Writer) bool { return false }

			output, err := runBrowseCommand(t, browse, "--provider", "gitlab", source)
			if err != nil {
				t.Fatalf("browse error = %v", err)
			}
			installURL := "https://example.com/acme/tool/-/releases/v1.2.3?source=private"
			command := "bin install '" + installURL + "' --select '" + asset + "' --provider 'gitlab'"
			if !strings.Contains(output, command) {
				t.Fatalf("browse guidance did not retain provider and exact release URL: %s", output)
			}

			resolved, err := resolveFetchRequest(installURL, "gitlab", providers.FetchOpts{})
			if err != nil {
				t.Fatalf("resolve generated install URL: %v", err)
			}
			if resolved.url != installURL {
				t.Fatalf("generated install URL normalized to %q, want %q", resolved.url, installURL)
			}
			installProvider, err := providers.New(resolved.url, "gitlab")
			if err != nil {
				t.Fatalf("construct forced GitLab provider from guidance: %v", err)
			}
			if installProvider.GetID() != "gitlab" {
				t.Fatalf("generated install provider ID = %q, want gitlab", installProvider.GetID())
			}
		})
	}
}

func TestBrowseQuotesRemoteMetadataInInstallGuidance(t *testing.T) {
	asset := "tool'safe_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	releaseURL := "https://github.com/acme/tool/releases/tag/v1.2.3'$(touch nope)?token=abc%2Fdef&variant=nightly"
	browse := newBrowseCmd()
	browse.newProvider = func(string, string) (providers.Provider, error) {
		return &browseHistoryTestProvider{releases: []*providers.ReleaseInfo{
			browseRelease("v1.2.3'$(echo unsafe)", releaseURL, asset),
		}}, nil
	}
	browse.isInteractive = func(io.Writer) bool { return false }

	output, err := runBrowseCommand(t, browse, "github.com/acme/tool")
	if err != nil {
		t.Fatalf("browse error = %v", err)
	}
	if !strings.Contains(output, "'https://github.com/acme/tool/releases/tag/v1.2.3'\"'\"'$(touch nope)?token=abc%2Fdef&variant=nightly' --select 'tool'\"'\"'safe_") {
		t.Fatalf("install guidance did not shell-quote remote values: %s", output)
	}
	if !strings.Contains(output, `Release "v1.2.3'$(echo unsafe)"`) {
		t.Fatalf("version metadata was not safely rendered: %s", output)
	}
}

func TestBrowseRejectsEffectfulAndUnsupportedSources(t *testing.T) {
	for _, tc := range []struct {
		name, source, provider, wantError string
	}{
		{name: "docker source", source: "docker://alpine", wantError: "effectful"},
		{name: "go install source", source: "goinstall://example.com/tool", wantError: "execute processes"},
		{name: "docker provider", source: "github.com/acme/tool", provider: "docker", wantError: "effectful"},
		{name: "unsupported history", source: "https://example.com/tool", provider: "generic", wantError: "requires release history"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			browse := newBrowseCmd()
			called := false
			browse.newProvider = func(string, string) (providers.Provider, error) {
				called = true
				if tc.provider == "generic" {
					return fetchBinaryTestProvider{id: "generic"}, nil
				}
				t.Fatal("effectful source reached provider construction")
				return nil, nil
			}
			args := []string{}
			if tc.provider != "" {
				args = append(args, "--provider", tc.provider)
			}
			args = append(args, tc.source)
			_, err := runBrowseCommand(t, browse, args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("browse error = %v, want %q", err, tc.wantError)
			}
			if tc.provider == "generic" && !called {
				t.Fatal("unsupported release history was not checked")
			}
			if tc.provider != "generic" && called {
				t.Fatal("effectful source was constructed before validation")
			}
		})
	}
}

func TestBrowseSurfacesProviderHistoryErrors(t *testing.T) {
	want := errors.New("provider API failed")
	browse := newBrowseCmd()
	browse.newProvider = func(string, string) (providers.Provider, error) {
		return &browseHistoryTestProvider{err: want}, nil
	}
	_, err := runBrowseCommand(t, browse, "github.com/acme/tool")
	if !errors.Is(err, want) {
		t.Fatalf("browse error = %v, want wrapped %v", err, want)
	}
}

type browseFailWriter struct{ err error }

func (w browseFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestBrowseOutputErrorsPropagate(t *testing.T) {
	want := errors.New("writer failed")
	err := writeBrowseReleases(browseFailWriter{err: want}, "source", []*providers.ReleaseInfo{
		browseRelease("v1", "https://example.com/releases/tag/v1"),
	}, providers.FetchOpts{}, "github", "")
	if !errors.Is(err, want) {
		t.Fatalf("browse output error = %v, want wrapped %v", err, want)
	}
}

func TestBrowseRootDoesNotLoadConfigOrCreateLogFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "missing-config.json")
	logPath := filepath.Join(dir, "browse.log")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", filepath.Join(dir, "exe-dir"))
	t.Setenv("HOME", t.TempDir())

	var exitCode int
	root := newRootCmd("test", func(code int) { exitCode = code })
	var output bytes.Buffer
	root.cmd.SetOut(&output)
	root.Execute([]string{"browse", "--provider", "docker", "github.com/acme/tool"})
	if exitCode != 1 {
		t.Fatalf("effectful browse exit code = %d, want 1", exitCode)
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("browse created or loaded config %s (stat error %v)", configPath, err)
	}

	exitCode = 0
	root = newRootCmd("test", func(code int) { exitCode = code })
	root.cmd.SetOut(&output)
	root.Execute([]string{"browse", "--log-file", logPath, "github.com/acme/tool"})
	if exitCode != 1 {
		t.Fatalf("browse with --log-file exit code = %d, want 1", exitCode)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only browse created log file %s (stat error %v)", logPath, err)
	}
}
