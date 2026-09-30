package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/providers"
)

func TestSearchJoinsQueryAndRendersRepositoryMetadata(t *testing.T) {
	search := newSearchCmd()
	var gotQuery string
	search.searchRepositories = func(query string) ([]providers.GitHubRepository, error) {
		gotQuery = query
		return []providers.GitHubRepository{{
			FullName:    "acme/termfm",
			Stars:       123,
			Description: "A terminal file manager",
		}}, nil
	}
	var output bytes.Buffer
	search.cmd.SetOut(&output)
	search.cmd.SetArgs([]string{"terminal", "file", "manager", "language:go", "stars:>100"})

	if err := search.cmd.Execute(); err != nil {
		t.Fatalf("search command error = %v", err)
	}
	if gotQuery != "terminal file manager language:go stars:>100" {
		t.Fatalf("search query = %q", gotQuery)
	}
	for _, expected := range []string{
		"acme/termfm",
		"Stars: 123",
		"Description: A terminal file manager",
		"bin browse https://github.com/acme/termfm",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("search output missing %q: %s", expected, output.String())
		}
	}
}

func TestSearchPrintsFriendlyEmptyResult(t *testing.T) {
	search := newSearchCmd()
	search.searchRepositories = func(string) ([]providers.GitHubRepository, error) { return nil, nil }
	var output bytes.Buffer
	search.cmd.SetOut(&output)
	search.cmd.SetArgs([]string{"no", "matches"})

	if err := search.cmd.Execute(); err != nil {
		t.Fatalf("search command error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "No GitHub repositories found") || !strings.Contains(got, `"no matches"`) {
		t.Fatalf("unexpected empty-result output: %q", got)
	}
}

func TestSearchPropagatesSearchErrors(t *testing.T) {
	wantErr := errors.New("GitHub unavailable")
	search := newSearchCmd()
	search.searchRepositories = func(string) ([]providers.GitHubRepository, error) { return nil, wantErr }
	search.cmd.SetArgs([]string{"terminal"})

	if err := search.cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("search error = %v, want %v", err, wantErr)
	}
}

func TestSearchRootDoesNotLoadOrWritePersistentState(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	t.Setenv("BIN_CONFIG", configPath)

	previousSearch := searchGitHubRepositories
	var gotQuery string
	searchGitHubRepositories = func(query string) ([]providers.GitHubRepository, error) {
		gotQuery = query
		return []providers.GitHubRepository{{FullName: "acme/termfm", Stars: 3}}, nil
	}
	t.Cleanup(func() { searchGitHubRepositories = previousSearch })

	var output bytes.Buffer
	exitCode := -1
	root := newRootCmd("test", func(code int) { exitCode = code })
	root.cmd.SetOut(&output)
	root.Execute([]string{"search", "terminal", "language:go"})
	if exitCode != -1 {
		t.Fatalf("search exit code = %d, want success", exitCode)
	}
	if gotQuery != "terminal language:go" {
		t.Fatalf("search query = %q", gotQuery)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("search created or accessed config file %s (stat error %v)", configPath, err)
	}
	if !strings.Contains(output.String(), "bin browse https://github.com/acme/termfm") {
		t.Fatalf("unexpected root search output: %q", output.String())
	}

	logPath := filepath.Join(t.TempDir(), "search.log")
	exitCode = -1
	root = newRootCmd("test", func(code int) { exitCode = code })
	root.cmd.SetOut(&output)
	root.Execute([]string{"search", "terminal", "--log-file", logPath})
	if exitCode != 1 {
		t.Fatalf("search with --log-file exit code = %d, want 1", exitCode)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("read-only search created log file %s (stat error %v)", logPath, err)
	}
}
