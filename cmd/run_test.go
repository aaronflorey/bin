package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

type runTestProvider struct {
	id         string
	name       string
	version    string
	content    string
	fetchCount int
	lastFetch  providers.FetchOpts
	err        error
}

func (p *runTestProvider) Fetch(opts *providers.FetchOpts) (*providers.File, error) {
	p.fetchCount++
	if opts != nil {
		p.lastFetch = *opts
	}
	if p.err != nil {
		return nil, p.err
	}
	return &providers.File{
		Data:    strings.NewReader(p.content),
		Name:    p.name,
		Version: p.version,
	}, nil
}

func (p *runTestProvider) GetLatestVersion() (*providers.ReleaseInfo, error) {
	return &providers.ReleaseInfo{Version: p.version, URL: "https://example.test/releases/tag/" + p.version}, nil
}

func (p *runTestProvider) Cleanup(*providers.CleanupOpts) error {
	return nil
}

func (p *runTestProvider) GetID() string {
	if p.id != "" {
		return p.id
	}
	return "github"
}

func TestParseRunTargetSingleURL(t *testing.T) {
	target, err := parseRunTarget([]string{"github.com/cli/cli"}, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.url != "github.com/cli/cli" {
		t.Fatalf("unexpected url: %s", target.url)
	}
	if len(target.args) != 0 {
		t.Fatalf("expected no passthrough args, got %v", target.args)
	}
}

func TestRunForwardsArgsAfterDash(t *testing.T) {
	setupTestConfig(t)
	cacheDir := t.TempDir()
	provider := &runTestProvider{name: "tool", version: "1.2.3", content: "binary"}
	cmd := newRunCmd()
	cmd.newProvider = func(_, _ string) (providers.Provider, error) { return provider, nil }
	cmd.userCacheDir = func() (string, error) { return cacheDir, nil }

	var gotPath string
	var gotArgs []string
	cmd.execCommand = helperExecCommand(t, 0, func(name string, args []string) {
		gotPath = name
		gotArgs = args
	})

	cmd.cmd.SetArgs([]string{"github.com/cli/cli", "--", "version", "--json"})
	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected run command error: %v", err)
	}

	wantPath := filepath.Join(cacheDir, "bin", "tool-1.2.3")
	if gotPath != wantPath {
		t.Fatalf("unexpected executable path: got %q want %q", gotPath, wantPath)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "version" || gotArgs[1] != "--json" {
		t.Fatalf("unexpected passthrough args: %v", gotArgs)
	}
	if provider.fetchCount != 1 {
		t.Fatalf("expected one fetch, got %d", provider.fetchCount)
	}
	if provider.lastFetch.Version != "" {
		t.Fatalf("expected empty requested version, got %q", provider.lastFetch.Version)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected cached binary to exist: %v", err)
	}
	if len(config.Get().Bins) != 0 {
		t.Fatalf("expected config to remain unchanged, got %d entries", len(config.Get().Bins))
	}
}

func TestRunReusesCachedExecutableWhenVersionAlreadyExists(t *testing.T) {
	setupTestConfig(t)
	cacheDir := t.TempDir()
	provider := &runTestProvider{name: "tool", version: "1.0.0", content: "new-binary"}
	cmd := newRunCmd()
	cmd.newProvider = func(_, _ string) (providers.Provider, error) { return provider, nil }
	cmd.userCacheDir = func() (string, error) { return cacheDir, nil }
	cmd.execCommand = helperExecCommand(t, 0, nil)

	cachePath := filepath.Join(cacheDir, "bin", "tool-1.0.0")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cached-binary"), 0o755); err != nil {
		t.Fatalf("seed cache file: %v", err)
	}

	cmd.cmd.SetArgs([]string{"github.com/cli/cli"})
	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected run command error: %v", err)
	}

	raw, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if string(raw) != "cached-binary" {
		t.Fatalf("expected cached binary to be reused, got %q", string(raw))
	}
}

func TestRunDoesNotUpdateConfig(t *testing.T) {
	setupTestConfig(t)
	cacheDir := t.TempDir()
	provider := &runTestProvider{name: "tool", version: "1.0.0", content: "binary"}
	cmd := newRunCmd()
	cmd.newProvider = func(_, _ string) (providers.Provider, error) { return provider, nil }
	cmd.userCacheDir = func() (string, error) { return cacheDir, nil }
	cmd.execCommand = helperExecCommand(t, 0, nil)

	configPath := os.Getenv("BIN_CONFIG")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config before run: %v", err)
	}

	cmd.cmd.SetArgs([]string{"github.com/cli/cli"})
	if err := cmd.cmd.Execute(); err != nil {
		t.Fatalf("unexpected run command error: %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after run: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("expected run command to leave config.json unchanged")
	}
	if len(config.Get().Bins) != 0 {
		t.Fatalf("expected config bins to remain empty, got %d entries", len(config.Get().Bins))
	}
}

func TestRunReturnsChildExitCode(t *testing.T) {
	setupTestConfig(t)
	cacheDir := t.TempDir()
	provider := &runTestProvider{name: "tool", version: "1.0.0", content: "binary"}
	cmd := newRunCmd()
	cmd.newProvider = func(_, _ string) (providers.Provider, error) { return provider, nil }
	cmd.userCacheDir = func() (string, error) { return cacheDir, nil }
	cmd.execCommand = helperExecCommand(t, 17, nil)

	cmd.cmd.SetArgs([]string{"github.com/cli/cli"})
	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected run command to return an exit error")
	}

	exitErr, ok := err.(*exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T", err)
	}
	if exitErr.code != 17 {
		t.Fatalf("unexpected exit code: got %d want 17", exitErr.code)
	}
}

func helperExecCommand(t *testing.T, exitCode int, capture func(string, []string)) func(string, ...string) *exec.Cmd {
	t.Helper()

	return func(name string, args ...string) *exec.Cmd {
		if capture != nil {
			capture(name, append([]string(nil), args...))
		}

		cmd := exec.Command(os.Args[0], "-test.run=TestRunHelperProcess", "--", name)
		cmd.Args = append(cmd.Args, args...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_RUN_HELPER_PROCESS=1",
			fmt.Sprintf("RUN_HELPER_EXIT_CODE=%d", exitCode),
		)
		return cmd
	}
}

func TestRunHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RUN_HELPER_PROCESS") != "1" {
		return
	}

	code, err := strconv.Atoi(os.Getenv("RUN_HELPER_EXIT_CODE"))
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(code)
}
