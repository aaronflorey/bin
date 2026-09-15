package cmd

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
	"github.com/caarlos0/log"
)

type fetchBinaryTestProvider struct {
	id      string
	fetchFn func(*providers.FetchOpts) (*providers.File, error)
	fetches *int
}

type trackingReadCloser struct {
	reader     io.Reader
	closeCount int
	closeErr   error
}

func (r *trackingReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }
func (r *trackingReadCloser) Close() error {
	r.closeCount++
	return r.closeErr
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func (p fetchBinaryTestProvider) Fetch(opts *providers.FetchOpts) (*providers.File, error) {
	if p.fetches != nil {
		(*p.fetches)++
	}
	if p.fetchFn != nil {
		return p.fetchFn(opts)
	}
	return nil, nil
}

func (p fetchBinaryTestProvider) GetLatestVersion() (*providers.ReleaseInfo, error) { return nil, nil }
func (p fetchBinaryTestProvider) Cleanup(*providers.CleanupOpts) error              { return nil }
func (p fetchBinaryTestProvider) GetID() string                                     { return p.id }

func TestAbsExpandedPath(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(prevWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := absExpandedPath("$HOME/.local/bin/tool")
	if err != nil {
		t.Fatalf("absExpandedPath: %v", err)
	}

	want := filepath.Join(homeDir, ".local", "bin", "tool")
	if got != want {
		t.Fatalf("unexpected expanded path: got %q, want %q", got, want)
	}
}

func TestExpandTrackedBinaryPathKeepsFilenameLiteral(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("remote_name", "outside/tool")

	got := expandTrackedBinaryPath("$HOME/bin/$remote_name")
	want := filepath.Join(homeDir, "bin", "$remote_name")
	if got != want {
		t.Fatalf("unexpected tracked path: got %q, want %q", got, want)
	}
}

func TestExistingConfigBinaryMatchesExpandedPath(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(prevWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	expandedPath := filepath.Join(homeDir, ".local", "bin", "tool")
	prevBins := config.Get().Bins
	config.Get().Bins = map[string]*config.Binary{
		expandedPath: {Path: expandedPath, RemoteName: "tool"},
	}
	defer func() {
		config.Get().Bins = prevBins
	}()

	got, ok := existingConfigBinary(InstallOpts{Path: "$HOME/.local/bin/tool"})
	if !ok {
		t.Fatal("expected existingConfigBinary to match expanded path")
	}
	if got.Path != expandedPath {
		t.Fatalf("unexpected binary path: got %q, want %q", got.Path, expandedPath)
	}
}

func TestSaveToDiskValidatesExpectedSHA(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	payload := "#!/bin/sh\necho hello\n"
	sum := sha256.Sum256([]byte(payload))

	_, err := saveToDisk(&providers.File{
		Data:        strings.NewReader(payload),
		Name:        "tool",
		ExpectedSHA: fmt.Sprintf("%x", sum),
	}, target, false)
	if err != nil {
		t.Fatalf("saveToDisk returned error: %v", err)
	}

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(target)
		if statErr != nil {
			t.Fatalf("stat installed file: %v", statErr)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("unexpected installed mode: got %o, want 755", info.Mode().Perm())
		}
	}

	_, err = saveToDisk(&providers.File{
		Data:        strings.NewReader("world"),
		Name:        "tool2",
		ExpectedSHA: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	}, filepath.Join(dir, "tool2"), false)
	if err == nil {
		t.Fatal("expected saveToDisk to fail on sha mismatch")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "tool2")); !os.IsNotExist(statErr) {
		t.Fatalf("expected failed install target to be absent, got err=%v", statErr)
	}
}

func TestSaveToDiskChecksumMismatchPreservesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("existing"), 0o755); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := saveToDisk(&providers.File{
		Data:        strings.NewReader("world"),
		Name:        "tool",
		ExpectedSHA: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	}, target, true)
	if err == nil {
		t.Fatal("expected saveToDisk to fail on sha mismatch")
	}

	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read file: %v", readErr)
	}
	if string(raw) != "existing" {
		t.Fatalf("expected original file to remain untouched, got %q", string(raw))
	}
}

func TestSaveToDiskRejectsInvalidPayloadWithoutReplacingDestination(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := saveToDisk(&providers.File{Data: strings.NewReader("BSDIFF40"), Name: "tool"}, target, true)
	if !errors.Is(err, assets.ErrNotRunnablePayload) {
		t.Fatalf("saveToDisk() error = %v, want ErrNotRunnablePayload", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing" {
		t.Fatalf("destination was replaced with %q", got)
	}
}

func TestCheckFinalPathErrorsWhenExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, _, err := checkFinalPath(target, "ignored", false)
	if err == nil {
		t.Fatal("expected checkFinalPath to fail when file already exists")
	}
}

func TestCheckFinalPathAllowsOverwriteWhenForced(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	finalPath, overwrite, err := checkFinalPath(target, "ignored", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalPath != target {
		t.Fatalf("unexpected final path: %s", finalPath)
	}
	if !overwrite {
		t.Fatal("expected overwrite to remain enabled")
	}
}

func TestCheckFinalPathKeepsOverwriteDisabledForNewPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")

	finalPath, overwrite, err := checkFinalPath(target, "ignored", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalPath != target {
		t.Fatalf("unexpected final path: %s", finalPath)
	}
	if overwrite {
		t.Fatal("expected overwrite to stay disabled")
	}
}

func TestCheckFinalPathExpandsOnlyExplicitDestination(t *testing.T) {
	homeDir := t.TempDir()
	installDir := filepath.Join(homeDir, "bin")
	if err := os.Mkdir(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("REMOTE_NAME", "outside/tool")

	path, _, err := checkFinalPath(os.ExpandEnv("$HOME/bin"), "$REMOTE_NAME", false)
	if err != nil {
		t.Fatalf("checkFinalPath returned error: %v", err)
	}
	want := filepath.Join(installDir, "$REMOTE_NAME")
	if path != want {
		t.Fatalf("unexpected path: got %q, want %q", path, want)
	}
}

func TestInstallBinaryRejectsUnsafeProviderNameWithAlias(t *testing.T) {
	installDir := setupTestConfig(t)
	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{
			id: "test",
			fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
				return &providers.File{Name: "tool_1.2.3...", Version: "1.2.3", Data: strings.NewReader("payload")}, nil
			},
		}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	_, err := installBinary(InstallOpts{
		URL:         "https://example.test/tool",
		Path:        installDir,
		LogicalName: "user-alias",
		ResolvePath: true,
	})
	if err == nil {
		t.Fatal("installBinary accepted an unsafe provider name")
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "user-alias")); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected destination after rejected provider name: %v", statErr)
	}
}

func TestInstallBinaryKeepsProviderEnvironmentSyntaxLiteral(t *testing.T) {
	setupTestConfig(t)
	homeDir := t.TempDir()
	installDir := filepath.Join(homeDir, "bin")
	if err := os.Mkdir(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)
	// SanitizeName lowercases provider names, so this is the environment name
	// present in the final provider-derived component.
	t.Setenv("remote_name", "outside/tool")

	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{
			id: "test",
			fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
				return &providers.File{Name: "$REMOTE_NAME", Version: "1.2.3", Data: strings.NewReader("#!/bin/sh\nexit 0\n")}, nil
			},
		}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	result, err := installBinary(InstallOpts{
		URL:         "https://example.test/tool",
		Path:        "$HOME/bin",
		ResolvePath: true,
	})
	if err != nil {
		t.Fatalf("installBinary returned error: %v", err)
	}
	want := filepath.Join(installDir, "$remote_name")
	if result.Path != want {
		t.Fatalf("unexpected installed path: got %q, want %q", result.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("literal provider name was not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "outside", "tool")); !os.IsNotExist(err) {
		t.Fatalf("provider environment syntax expanded into a path: %v", err)
	}
}

func TestInstallBinaryExpandsDestinationWithoutPathResolution(t *testing.T) {
	setupTestConfig(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{
			id: "test",
			fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
				return &providers.File{Name: "tool", Version: "1.2.3", Data: strings.NewReader("#!/bin/sh\nexit 0\n")}, nil
			},
		}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	path := "$HOME/bin/tool"
	if _, err := installBinary(InstallOpts{URL: "https://example.test/tool", Path: path, ResolvePath: false}); err != nil {
		t.Fatalf("installBinary returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "bin", "tool")); err != nil {
		t.Fatalf("expanded destination was not installed: %v", err)
	}
}

func TestInstallBinaryClosesFetchedStreamAfterMinimumAgeFailure(t *testing.T) {
	setupTestConfig(t)
	stream := &trackingReadCloser{reader: strings.NewReader(runnableRunScript)}
	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		now := time.Now()
		return fetchBinaryTestProvider{id: "test", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Name: "tool", Version: "1.0.0", PublishedAt: &now, Data: stream}, nil
		}}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	minAgeDays := 1
	_, err := installBinary(InstallOpts{URL: "https://example.test/tool", Path: t.TempDir(), ResolvePath: true, MinAgeDays: &minAgeDays})
	if err == nil {
		t.Fatal("expected minimum-age failure")
	}
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream closed %d times, want 1", stream.closeCount)
	}
}

func TestInstallBinaryClosesFetchedStreamAfterDestinationResolutionFailure(t *testing.T) {
	setupTestConfig(t)
	stream := &trackingReadCloser{reader: strings.NewReader(runnableRunScript)}
	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "test", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Name: "tool", Version: "1.0.0", Data: stream}, nil
		}}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	_, err := installBinary(InstallOpts{URL: "https://example.test/tool", Path: "\x00", ResolvePath: true})
	if err == nil {
		t.Fatal("expected destination-resolution failure")
	}
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream closed %d times, want 1", stream.closeCount)
	}
}

func TestInstallBinaryClosesFetchedStreamAfterCopyFailure(t *testing.T) {
	setupTestConfig(t)
	stream := &trackingReadCloser{reader: failingReader{err: errors.New("read failed")}}
	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "test", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Name: "tool", Version: "1.0.0", Data: stream}, nil
		}}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	_, err := installBinary(InstallOpts{URL: "https://example.test/tool", Path: t.TempDir(), ResolvePath: true})
	if err == nil {
		t.Fatal("expected copy failure")
	}
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream closed %d times, want 1", stream.closeCount)
	}
}

func TestInstallBinaryClosesFetchedStreamAfterSuccess(t *testing.T) {
	installDir := setupTestConfig(t)
	stream := &trackingReadCloser{reader: strings.NewReader(runnableRunScript)}
	previousFactory := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "test", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Name: "tool", Version: "1.0.0", Data: stream}, nil
		}}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousFactory })

	_, err := installBinary(InstallOpts{URL: "https://example.test/tool", Path: installDir, ResolvePath: true})
	if err != nil {
		t.Fatalf("installBinary returned error: %v", err)
	}
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream closed %d times, want 1", stream.closeCount)
	}
}

func TestFindManagedDuplicateByHash(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-a": {Path: "/tmp/tool-a", Hash: "abc"},
		"/tmp/tool-b": {Path: "/tmp/tool-b", Hash: "def"},
		"/tmp/tool-c": {Path: "/tmp/tool-c", Hash: "abc"},
	}

	duplicatePath, ok := findManagedDuplicateByHash(bins, "/tmp/tool-a", "abc")
	if !ok {
		t.Fatal("expected duplicate hash to be found")
	}
	if duplicatePath != "/tmp/tool-c" {
		t.Fatalf("unexpected duplicate path: %s", duplicatePath)
	}
}

func TestResolveManagedBinSuggestionNonInteractive(t *testing.T) {
	previousInteractive := isPromptInteractive
	previousConfirm := confirmPrompt
	isPromptInteractive = func() bool { return false }
	confirmPrompt = func(_ string) error { return nil }
	defer func() {
		isPromptInteractive = previousInteractive
		confirmPrompt = previousConfirm
	}()

	_, err := resolveManagedBinSuggestion(map[string]*config.Binary{
		"/tmp/unison": {Path: "/tmp/unison"},
	}, "uni")
	if err == nil {
		t.Fatal("expected suggestion error")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("expected not found error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `did you mean "unison"`) {
		t.Fatalf("unexpected suggestion error: %v", err)
	}
}

func TestResolveManagedBinSuggestionInteractiveAcceptsMatch(t *testing.T) {
	previousInteractive := isPromptInteractive
	previousConfirm := confirmPrompt
	isPromptInteractive = func() bool { return true }
	prompted := ""
	confirmPrompt = func(message string) error {
		prompted = message
		return nil
	}
	defer func() {
		isPromptInteractive = previousInteractive
		confirmPrompt = previousConfirm
	}()

	path, err := resolveManagedBinSuggestion(map[string]*config.Binary{
		"/tmp/unison": {Path: "/tmp/unison"},
	}, "uni")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/tmp/unison" {
		t.Fatalf("unexpected suggested path: %s", path)
	}
	if prompted != `Did you mean "unison"?` {
		t.Fatalf("unexpected prompt message: %s", prompted)
	}
}

func TestResolveManagedBinSuggestionAmbiguous(t *testing.T) {
	previousInteractive := isPromptInteractive
	previousConfirm := confirmPrompt
	isPromptInteractive = func() bool { return false }
	confirmPrompt = func(_ string) error { return nil }
	defer func() {
		isPromptInteractive = previousInteractive
		confirmPrompt = previousConfirm
	}()

	_, err := resolveManagedBinSuggestion(map[string]*config.Binary{
		"/tmp/unicode": {Path: "/tmp/unicode"},
		"/tmp/unison":  {Path: "/tmp/unison"},
	}, "uni")
	if err == nil {
		t.Fatal("expected ambiguous suggestion error")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("expected not found error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "multiple matches: unicode, unison") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestResolveBinsToProcessUsesAcceptedSuggestion(t *testing.T) {
	setupTestConfig(t)
	path := filepath.Join(t.TempDir(), "unison")
	if err := config.UpsertBinary(&config.Binary{Path: path, URL: "https://example.test/acme/unison", Version: "1.0.0"}); err != nil {
		t.Fatalf("failed to seed binary: %v", err)
	}

	previousInteractive := isPromptInteractive
	previousConfirm := confirmPrompt
	isPromptInteractive = func() bool { return true }
	confirmPrompt = func(_ string) error { return nil }
	defer func() {
		isPromptInteractive = previousInteractive
		confirmPrompt = previousConfirm
	}()

	bins, err := resolveBinsToProcess(config.Get().Bins, []string{"uni"})
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if len(bins) != 1 {
		t.Fatalf("expected one resolved binary, got %d", len(bins))
	}
	if _, ok := bins[path]; !ok {
		t.Fatalf("expected resolved path %q", path)
	}
}

func TestShouldFallbackProviderFetch(t *testing.T) {
	if !shouldFallbackProviderFetch(fmt.Errorf("%w: nope", assets.ErrNoCompatibleFiles)) {
		t.Fatal("expected no-compatible-files error to trigger provider fallback")
	}
	if !shouldFallbackProviderFetch(systempackage.NewCompatibilityError("wrong type")) {
		t.Fatal("expected system package compatibility error to trigger provider fallback")
	}
	if shouldFallbackProviderFetch(errors.New("boom")) {
		t.Fatal("did not expect generic error to trigger provider fallback")
	}
}

func TestFetchBinarySkipsNoOpProviderFallbackRetry(t *testing.T) {
	previousLogger := log.Log
	var logs bytes.Buffer
	logger := log.New(&logs)
	logger.Level = log.DebugLevel
	log.Log = logger
	defer func() {
		log.Log = previousLogger
	}()

	fetchCount := 0
	newProviderCalls := 0
	newProvider := func(_ string, forcedProvider string) (providers.Provider, error) {
		newProviderCalls++
		return fetchBinaryTestProvider{
			id:      "github",
			fetches: &fetchCount,
			fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
				return nil, fmt.Errorf("%w: no linux asset", assets.ErrNoCompatibleFiles)
			},
		}, nil
	}

	provider, file, err := fetchBinary(newProvider, "https://example.test/owner/repo", "github", providers.FetchOpts{}, true)
	if !errors.Is(err, assets.ErrNoCompatibleFiles) {
		t.Fatalf("expected original compatibility error, got provider=%v file=%v err=%v", provider, file, err)
	}
	if provider != nil || file != nil {
		t.Fatalf("expected no provider or file on no-op fallback, got provider=%v file=%v", provider, file)
	}
	if fetchCount != 1 {
		t.Fatalf("expected exactly one fetch attempt, got %d", fetchCount)
	}
	if newProviderCalls != 2 {
		t.Fatalf("expected provider auto-detection lookup after forced provider failure, got %d calls", newProviderCalls)
	}
	if strings.Contains(logs.String(), "retrying with auto-detection") {
		t.Fatalf("expected no fallback retry warning when auto-detection resolves same provider, got %q", logs.String())
	}
}
