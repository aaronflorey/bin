package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
)

type inspectErrorWriter struct{}

func (inspectErrorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func runInspectCommand(t *testing.T, root *inspectCmd, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stderr)
	root.cmd.SetArgs(args)
	err := root.cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func decodeInspectReport(t *testing.T, out string) inspectReport {
	t.Helper()
	var report inspectReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode inspect report: %v\n%s", err, out)
	}
	return report
}

func TestInspectHasJSONAndProviderFlags(t *testing.T) {
	cmd := newInspectCmd().cmd
	if cmd.Flags().Lookup("json") == nil {
		t.Fatal("expected --json flag to be registered")
	}
	if cmd.Flags().Lookup("provider") == nil {
		t.Fatal("expected --provider flag to be registered")
	}
}

func TestInspectJSONReportsResolvedSelection(t *testing.T) {
	data := &trackingReadCloser{reader: strings.NewReader("#!/bin/sh\nexit 0\n")}
	evidence := &assets.ArtifactEvidence{
		Release: assets.ReleaseEvidence{
			Selected: "tool_1.2.3_linux_amd64.zip",
			Candidates: []assets.ReleaseCandidateDecision{
				{
					Candidate: assets.ReleaseCandidate{
						ID:      "tool_1.2.3_linux_amd64.zip",
						Product: "tool",
						Format:  "zip",
						Target:  assets.ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}},
					},
					Eligible: true,
					Selected: true,
				},
				{
					Candidate: assets.ReleaseCandidate{
						ID:      "helper_1.2.3_linux_amd64.zip",
						Product: "helper",
						Format:  "zip",
						Target:  assets.ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}},
					},
				},
			},
		},
		Archive: assets.ArchiveEvidence{
			Selected: "bin/tool",
			Entries: []assets.ArchiveMemberDecision{
				{Identity: "bin/tool", Class: "executable", TargetCompatible: true, Runnable: true, Eligible: true, Selected: true},
				{Identity: "README.md", Class: "ignored"},
			},
		},
		Transformations: []string{"zip"},
		Integrity:       assets.ArtifactIntegrityEvidence{DownloadSHA256: "download", InstalledSHA256: "installed"},
	}

	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if !opts.NonInteractive {
				t.Fatal("inspect must fetch non-interactively")
			}
			return &providers.File{
				Data:        data,
				Name:        "tool",
				Version:     "1.2.3",
				SourceAsset: "tool_1.2.3_linux_amd64.zip",
				PackagePath: "bin/tool",
				Evidence:    evidence,
				DownloadIntegrity: &providers.IntegrityRecord{
					Algorithm: "sha256", Expected: "download-expected", Observed: "download-observed",
					Source: "tool_1.2.3_linux_amd64.zip.sha256", Scope: "download", Result: "verified",
				},
				InstalledIntegrity: &providers.IntegrityRecord{
					Algorithm: "sha256", Expected: "installed-expected", Observed: "installed-observed",
					Source: "tool_1.2.3_linux_amd64.zip.sha256", Scope: "installed", Result: "verified",
				},
			}, nil
		}}, nil
	}

	out, stderr, err := runInspectCommand(t, cmd, "--json", "https://example.test/tool")
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("inspect wrote to stderr: %q", stderr)
	}

	report := decodeInspectReport(t, out)
	if report.SchemaVersion != inspectSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", report.SchemaVersion, inspectSchemaVersion)
	}
	if report.Status != inspectStatusResolved || report.Reason != "" {
		t.Fatalf("status/reason = %q/%q, want resolved with no reason", report.Status, report.Reason)
	}
	if report.Source.Provider != "generic" || report.Source.Version != "1.2.3" {
		t.Fatalf("source = %#v", report.Source)
	}
	if report.Release == nil || report.Release.Selected != "tool_1.2.3_linux_amd64.zip" {
		t.Fatalf("release report = %#v", report.Release)
	}
	if len(report.Release.Candidates) != 2 {
		t.Fatalf("candidate decisions = %d, want 2", len(report.Release.Candidates))
	}
	if !report.Release.Candidates[0].Eligible || !report.Release.Candidates[0].Selected || report.Release.Candidates[1].Eligible {
		t.Fatalf("candidate outcomes = %#v", report.Release.Candidates)
	}
	if report.Archive == nil || report.Archive.Selected != "bin/tool" || len(report.Archive.Entries) != 2 {
		t.Fatalf("archive report = %#v", report.Archive)
	}
	if len(report.Transformations) != 1 || report.Transformations[0] != "zip" {
		t.Fatalf("transformations = %#v", report.Transformations)
	}
	if report.Integrity == nil || report.Integrity.DownloadSHA256 != "download" || report.Integrity.InstalledSHA256 != "installed" || report.Integrity.UnchangedBytes {
		t.Fatalf("integrity report = %#v", report.Integrity)
	}
	assertInspectIntegrityDecision(t, report.Integrity.Download, "sha256", "download-expected", "download-observed", "tool_1.2.3_linux_amd64.zip.sha256", "download", "verified")
	assertInspectIntegrityDecision(t, report.Integrity.Installed, "sha256", "installed-expected", "installed-observed", "tool_1.2.3_linux_amd64.zip.sha256", "installed", "verified")
	if report.Artifact == nil || report.Artifact.Name != "tool" || report.Artifact.PackagePath != "bin/tool" {
		t.Fatalf("artifact report = %#v", report.Artifact)
	}
	if data.closeCount != 1 {
		t.Fatalf("fetched data closed %d times, want 1", data.closeCount)
	}
}

func TestInspectJSONReportsStableAmbiguityWithoutPrompting(t *testing.T) {
	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "github", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if !opts.NonInteractive {
				t.Fatal("inspect must not allow interactive ambiguity resolution")
			}
			return nil, &assets.ReleaseCandidateResolutionError{
				Reason:     assets.ReleaseCandidateAmbiguousProduct,
				Candidates: []string{"tool-cli", "beta-tool"},
			}
		}}, nil
	}

	out, _, err := runInspectCommand(t, cmd, "--json", "github.com/acme/tools")
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}

	report := decodeInspectReport(t, out)
	if report.Status != inspectStatusUnresolved {
		t.Fatalf("status = %q, want unresolved", report.Status)
	}
	if report.Reason != string(assets.ReleaseCandidateAmbiguousProduct) {
		t.Fatalf("reason = %q, want stable ambiguity reason", report.Reason)
	}
	if report.Release == nil || report.Release.Selected != "" {
		t.Fatalf("ambiguous report selected a candidate: %#v", report.Release)
	}
	want := []string{"beta-tool", "tool-cli"}
	if len(report.Release.Ambiguous) != len(want) {
		t.Fatalf("ambiguous = %#v, want %#v", report.Release.Ambiguous, want)
	}
	for index := range want {
		if report.Release.Ambiguous[index] != want[index] {
			t.Fatalf("ambiguous = %#v, want %#v", report.Release.Ambiguous, want)
		}
	}
	if report.Error != "" {
		t.Fatalf("typed ambiguity reported as generic error: %q", report.Error)
	}
}

func TestInspectReportsArchiveAmbiguityFromSharedEvidence(t *testing.T) {
	extension := inspectScriptExtension()
	archive := inspectZipArchive(t, map[string]string{
		"a/tool" + extension: string(inspectRunnablePayload()),
		"b/tool" + extension: string(inspectRunnablePayload()),
	})
	filename := fmt.Sprintf("tool_0.16.0_%s_%s", runtime.GOOS, runtime.GOARCH)
	server := inspectArtifactServer(t, filename, archive)

	// Isolate os.TempDir so concurrently running packages cannot change the
	// artifact count between the before and after samples.
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	previous := allowInsecureHTTP
	allowInsecureHTTP = true
	t.Cleanup(func() { allowInsecureHTTP = previous })

	cmd := newInspectCmd()
	before := inspectTempArtifacts(t)
	out, _, err := runInspectCommand(t, cmd, "--json", server.URL)
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}
	if after := inspectTempArtifacts(t); after != before {
		t.Fatalf("rejected inspection leaked temporary artifacts: before %d after %d", before, after)
	}

	report := decodeInspectReport(t, out)
	if report.Status != inspectStatusUnresolved {
		t.Fatalf("status = %q, want unresolved", report.Status)
	}
	if report.Archive == nil || report.Archive.Reason != string(assets.ArchiveMemberAmbiguous) {
		t.Fatalf("archive report = %#v", report.Archive)
	}
	if report.Archive.Selected != "" {
		t.Fatalf("ambiguous archive selected %q", report.Archive.Selected)
	}
	if len(report.Archive.Entries) != 2 {
		t.Fatalf("archive entries = %d, want shared inventory", len(report.Archive.Entries))
	}
}

func TestInspectKeepsStdoutPureJSON(t *testing.T) {
	previousLogger := log.Log
	var logs bytes.Buffer
	log.Log = log.New(&logs)
	t.Cleanup(func() { log.Log = previousLogger })

	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			log.Warn("diagnostic from provider")
			return nil, errors.New("download failed")
		}}, nil
	}

	out, stderr, err := runInspectCommand(t, cmd, "--json", "https://example.test/tool")
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("diagnostics leaked to command stderr: %q", stderr)
	}
	if !strings.Contains(logs.String(), "diagnostic from provider") {
		t.Fatalf("diagnostic was not routed to the logger: %q", logs.String())
	}

	report := decodeInspectReport(t, out)
	if report.Status != inspectStatusFailed || !strings.Contains(report.Error, "download failed") {
		t.Fatalf("failed report = %#v", report)
	}
}

func TestInspectDoesNotCreateOrMutatePersistentState(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	exeDir := filepath.Join(dir, "exe-dir")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", exeDir)
	t.Setenv("HOME", t.TempDir())

	filename := fmt.Sprintf("tool_0.16.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, inspectScriptExtension())
	server := inspectArtifactServer(t, filename, inspectRunnablePayload())

	var exitCode int
	root := newRootCmd("test", func(code int) { exitCode = code })
	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)

	root.Execute([]string{"inspect", "--json", "--allow-insecure-http", server.URL})

	if exitCode != 0 {
		t.Fatalf("inspect exited with code %d", exitCode)
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect created config at %s (stat error %v)", configPath, err)
	}
	if _, err := os.Stat(exeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect created default path %s (stat error %v)", exeDir, err)
	}

	report := decodeInspectReport(t, stdout.String())
	if report.Status != inspectStatusResolved {
		t.Fatalf("status = %q, want resolved for a readable artifact", report.Status)
	}
	if report.Release == nil || report.Release.Selected != filename {
		t.Fatalf("release report = %#v", report.Release)
	}
}

func TestInspectStartupDoesNotCreateLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "inspect.log")
	t.Setenv("BIN_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("BIN_EXE_DIR", filepath.Join(dir, "exe-dir"))
	t.Setenv("HOME", t.TempDir())

	var exitCode int
	root := newRootCmd("test", func(code int) { exitCode = code })
	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)

	root.Execute([]string{"inspect", "--json", "--log-file", logPath, "https://example.test/tool"})

	if exitCode == 0 {
		t.Fatal("inspect accepted a durable --log-file request")
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect created log file %s (stat error %v)", logPath, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("rejected inspect wrote to stdout: %q", stdout.String())
	}
}

func TestInspectSurfacesCloseErrorsWithoutCorruptingStdout(t *testing.T) {
	data := &trackingReadCloser{reader: strings.NewReader("#!/bin/sh\nexit 0\n"), closeErr: errors.New("close failed")}
	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Data: data, Name: "tool", Version: "1.2.3"}, nil
		}}, nil
	}

	out, stderr, err := runInspectCommand(t, cmd, "--json", "https://example.test/tool")
	if err == nil || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("inspect error = %v, want surfaced close failure", err)
	}
	if stderr != "" {
		t.Fatalf("command writer captured the close error: %q", stderr)
	}
	if data.closeCount != 1 {
		t.Fatalf("fetched data closed %d times, want 1", data.closeCount)
	}
	if report := decodeInspectReport(t, out); report.Status != inspectStatusResolved {
		t.Fatalf("status = %q, want a resolved JSON document before the close failure", report.Status)
	}
}

func TestInspectRedactsIntegritySourceURL(t *testing.T) {
	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{
				Data: strings.NewReader("#!/bin/sh\nexit 0\n"),
				Name: "tool",
				DownloadIntegrity: &providers.IntegrityRecord{
					Algorithm: "sha256", Expected: "expected", Observed: "observed",
					Source: "https://user:secret@example.test/tool.sha256?token=abc123#frag",
					Scope:  "download", Result: "verified",
				},
			}, nil
		}}, nil
	}

	out, _, err := runInspectCommand(t, cmd, "--json", "https://example.test/tool")
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}

	report := decodeInspectReport(t, out)
	if report.Integrity == nil || report.Integrity.Download == nil {
		t.Fatalf("integrity report = %#v", report.Integrity)
	}
	if report.Integrity.Download.Source != "https://example.test/tool.sha256" {
		t.Fatalf("integrity source = %q, want redacted url", report.Integrity.Download.Source)
	}
	for _, secret := range []string{"secret", "abc123", "token=", "frag"} {
		if strings.Contains(out, secret) {
			t.Fatalf("inspect output leaked %q: %s", secret, out)
		}
	}
}

func TestInspectIgnoresConfiguredHooksAndDefaultPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	defaultPath := filepath.Join(dir, "default-path")
	marker := filepath.Join(dir, "hook-ran")
	contents := fmt.Sprintf(`{"default_path":%q,"bins":{},"hooks":[{"type":"pre-install","command":"touch","args":[%q]}]}`, defaultPath, marker)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("HOME", t.TempDir())

	filename := fmt.Sprintf("tool_0.16.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, inspectScriptExtension())
	server := inspectArtifactServer(t, filename, inspectRunnablePayload())

	var exitCode int
	root := newRootCmd("test", func(code int) { exitCode = code })
	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.Execute([]string{"inspect", "--json", "--allow-insecure-http", server.URL})

	if exitCode != 0 {
		t.Fatalf("inspect exited with code %d", exitCode)
	}
	if report := decodeInspectReport(t, stdout.String()); report.Status != inspectStatusResolved {
		t.Fatalf("status = %q, want resolved", report.Status)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(after) != contents {
		t.Fatalf("inspect mutated config:\n%s", after)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect ran a configured hook (marker stat error %v)", err)
	}
	if _, err := os.Stat(defaultPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect created the configured default path (stat error %v)", err)
	}
}

func TestInspectRejectsEffectfulSourcesBeforeProviderWork(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"docker scheme", []string{"docker://alpine:latest"}},
		{"goinstall scheme", []string{"goinstall://github.com/acme/tool"}},
		{"docker provider", []string{"--provider", "docker", "github.com/acme/tool"}},
		{"goinstall provider", []string{"--provider", "goinstall", "github.com/acme/tool"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			constructed := false
			cmd := newInspectCmd()
			cmd.newProvider = func(string, string) (providers.Provider, error) {
				constructed = true
				t.Fatal("provider constructed before effectful source rejection")
				return nil, nil
			}

			out, _, err := runInspectCommand(t, cmd, tc.args...)
			if err == nil {
				t.Fatalf("inspect accepted %v", tc.args)
			}
			if constructed {
				t.Fatal("provider was constructed")
			}
			if out != "" {
				t.Fatalf("rejected source wrote to stdout: %q", out)
			}
		})
	}
}

func TestInspectPreservesTransportPolicy(t *testing.T) {
	previous := allowInsecureHTTP
	allowInsecureHTTP = false
	t.Cleanup(func() { allowInsecureHTTP = previous })

	cmd := newInspectCmd()
	cmd.newProvider = newProviderWithPolicy

	out, _, err := runInspectCommand(t, cmd, "--json", "http://user:secret@127.0.0.1:9/tool_1.0.0_linux_amd64?token=abc123#frag")
	if !errors.Is(err, providers.ErrInsecureHTTP) {
		t.Fatalf("inspect error = %v, want insecure http rejection", err)
	}
	if out != "" {
		t.Fatalf("rejected transport wrote to stdout: %q", out)
	}
	for _, secret := range []string{"secret", "abc123", "token=", "frag"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("transport rejection leaked %q: %v", secret, err)
		}
	}
}

func TestInspectRedactsURLSecrets(t *testing.T) {
	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return nil, errors.New("downloading https://user:secret@example.test/tool?token=abc123&sig=xyz#frag failed")
		}}, nil
	}

	out, _, err := runInspectCommand(t, cmd, "--json", "https://user:secret@example.test/tool?token=abc123&sig=xyz#frag")
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}

	report := decodeInspectReport(t, out)
	if report.Source.URL != "https://example.test/tool" {
		t.Fatalf("source url = %q, want credentials, query, and fragment removed", report.Source.URL)
	}
	if report.Error != "downloading https://example.test/tool failed" {
		t.Fatalf("error = %q, want redacted url", report.Error)
	}
	for _, secret := range []string{"secret", "abc123", "sig=xyz", "frag", "token="} {
		if strings.Contains(out, secret) {
			t.Fatalf("inspect output leaked %q: %s", secret, out)
		}
	}
}

func TestInspectRedactionHelpers(t *testing.T) {
	cases := map[string]string{
		"https://user:pass@example.test/path?token=abc#frag": "https://example.test/path",
		"user:pass@example.test/path?token=abc":              "example.test/path",
		"github.com/acme/tool":                               "github.com/acme/tool",
		"https://example.test/path.":                         "https://example.test/path.",
	}
	for input, want := range cases {
		if got := redactURL(input); got != want {
			t.Fatalf("redactURL(%q) = %q, want %q", input, got, want)
		}
	}

	if got := redactText("failed https://user:pass@example.test/path?token=abc#frag now"); got != "failed https://example.test/path now" {
		t.Fatalf("redactText() = %q", got)
	}
}

func TestInspectClosesFetchedDataOnWriteFailure(t *testing.T) {
	data := &trackingReadCloser{reader: strings.NewReader("#!/bin/sh\nexit 0\n")}
	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Data: data, Name: "tool", Version: "1.2.3"}, nil
		}}, nil
	}
	cmd.cmd.SetOut(inspectErrorWriter{})
	cmd.cmd.SetArgs([]string{"--json", "https://example.test/tool"})

	if err := cmd.cmd.Execute(); err == nil {
		t.Fatal("expected inspect to report the stdout write failure")
	}
	if data.closeCount != 1 {
		t.Fatalf("fetched data closed %d times, want 1 on output failure", data.closeCount)
	}
}

func TestInspectRequiresJSONFlag(t *testing.T) {
	fetched := false
	cmd := newInspectCmd()
	cmd.newProvider = func(string, string) (providers.Provider, error) {
		fetched = true
		return fetchBinaryTestProvider{id: "generic", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Data: strings.NewReader("data"), Name: "tool"}, nil
		}}, nil
	}

	out, _, err := runInspectCommand(t, cmd, "https://example.test/tool")
	if err == nil {
		t.Fatal("inspect without --json succeeded, want an explicit error")
	}
	if fetched {
		t.Fatal("inspect performed provider work without --json")
	}
	if out != "" {
		t.Fatalf("inspect without --json wrote to stdout: %q", out)
	}
}

func assertInspectIntegrityDecision(t *testing.T, got *config.IntegrityRecord, algorithm, expected, observed, source, scope, result string) {
	t.Helper()
	if got == nil {
		t.Fatal("missing structured integrity decision")
	}
	if got.Algorithm != algorithm || got.Expected != expected || got.Observed != observed ||
		got.Source != source || got.Scope != scope || got.Result != result {
		t.Fatalf("integrity decision = %#v, want %s/%s/%s/%s/%s/%s", got, algorithm, expected, observed, source, scope, result)
	}
}

func inspectTempArtifacts(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "bin-artifact-") || strings.HasPrefix(name, "bin-download-") {
			count++
		}
	}
	return count
}

func inspectArtifactServer(t *testing.T, filename string, payload []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	return server
}

func inspectZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create archive entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write archive entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buf.Bytes()
}

func inspectScriptExtension() string {
	if runtime.GOOS == "windows" {
		return ".cmd"
	}
	return ""
}

func inspectRunnablePayload() []byte {
	if runtime.GOOS == "windows" {
		return []byte("@echo off\r\n")
	}
	return []byte("#!/bin/sh\nexit 0\n")
}
