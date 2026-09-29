package assets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestArtifactFormatsRecognizeOnlySupportedDecoders(t *testing.T) {
	for _, test := range []struct {
		name        string
		header      []byte
		format      artifactFormat
		unsupported bool
	}{
		{name: "tool", format: artifactFormatPlain},
		{name: "tool.tar", format: artifactFormatPlain},
		{name: "tool.zip", format: artifactFormatPlain},
		{name: "tool.gz", header: []byte{0x1f, 0x8b}, format: artifactFormatGzip},
		{name: "tool.xz", header: []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, format: artifactFormatXz},
		{name: "tool.bz2", header: []byte("BZh9"), format: artifactFormatBzip2},
		{name: "tool.zip", header: []byte{0x1f, 0x8b}, format: artifactFormatGzip},
		{name: "tool", header: []byte{0x28, 0xb5, 0x2f, 0xfd}, format: artifactFormatZstandard, unsupported: true},
		{name: "tool.zst", format: artifactFormatZstandard, unsupported: true},
	} {
		t.Run(test.name+test.format.String(), func(t *testing.T) {
			format := artifactFormatFor(test.name, test.header)
			if format != test.format {
				t.Fatalf("artifactFormatFor() = %s, want %s", format, test.format)
			}
			err := validateArtifactFormat(format)
			if test.unsupported && !errors.Is(err, ErrUnsupportedArtifactFormat) {
				t.Fatalf("validateArtifactFormat() = %v, want unsupported format", err)
			}
			if !test.unsupported && err != nil {
				t.Fatalf("validateArtifactFormat() = %v", err)
			}
		})
	}
}

func TestArtifactBudgetsAreFiniteAndTracked(t *testing.T) {
	budgets := artifactBudgets{maxDownloadBytes: 2, maxArchiveEntries: 2, maxEntryBytes: 2, maxExpandedBytes: 3, maxNesting: 1}
	tracker, err := newArtifactBudgetTracker(budgets)
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.addDownloadBytes(2); err != nil {
		t.Fatal(err)
	}
	if err := tracker.addDownloadBytes(1); !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("download limit error = %v", err)
	}
	if err := tracker.visitArchiveEntry(); err != nil {
		t.Fatal(err)
	}
	if err := tracker.addEntryBytes(2); err != nil {
		t.Fatal(err)
	}
	if err := tracker.addEntryBytes(1); !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("per-entry limit error = %v", err)
	}
	if err := tracker.visitArchiveEntry(); err != nil {
		t.Fatal(err)
	}
	if err := tracker.addEntryBytes(2); !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("expanded limit error = %v", err)
	}
	if err := tracker.visitArchiveEntry(); !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("entry limit error = %v", err)
	}
	if err := tracker.enterArchive(); err != nil {
		t.Fatal(err)
	}
	tracker.leaveArchive()
	if err := tracker.enterArchive(); err != nil {
		t.Fatalf("archive nesting did not reset: %v", err)
	}
	if err := tracker.enterArchive(); !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("nesting limit error = %v", err)
	}
	if _, err := newArtifactBudgetTracker(artifactBudgets{}); err == nil {
		t.Fatal("zero budgets were accepted")
	}
}

func TestDefaultArtifactBudgetAllowsGooseSizedEntryButRemainsBounded(t *testing.T) {
	tracker, err := newArtifactBudgetTracker(defaultArtifactBudgets())
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.visitArchiveEntry(); err != nil {
		t.Fatal(err)
	}
	if err := tracker.addEntryBytes(300841352); err != nil {
		t.Fatalf("Goose-sized entry rejected: %v", err)
	}
	if err := tracker.addEntryBytes(defaultArtifactBudgets().maxEntryBytes); !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("oversized entry error = %v, want bounded rejection", err)
	}
}

func TestArtifactProcessingResultOwnsCleanup(t *testing.T) {
	closed := 0
	cleaned := 0
	result := &artifactProcessingResult{
		final: &finalFile{Source: closeTracker{closed: &closed}},
		cleanup: func() error {
			cleaned++
			return nil
		},
	}
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	if closed != 1 || cleaned != 1 {
		t.Fatalf("Close() calls: source=%d cleanup=%d, want one each", closed, cleaned)
	}
}

func TestArtifactProcessingResultReportsAllCleanupErrorsOnce(t *testing.T) {
	fixture := testRunnableFixture("tool")
	download, err := os.CreateTemp(t.TempDir(), "download-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(download, fixture.contents); err != nil {
		t.Fatal(err)
	}
	if err := download.Close(); err != nil {
		t.Fatal(err)
	}

	downloadErr := errors.New("download cleanup")
	rootErr := errors.New("root cleanup")
	originalRemoveDownload, originalRemoveRoot := removeArtifactDownload, removeArtifactRoot
	removeCalls, rootCalls := 0, 0
	removeArtifactDownload = func(path string) error {
		removeCalls++
		if err := os.Remove(path); err != nil {
			return err
		}
		return downloadErr
	}
	removeArtifactRoot = func(path string) error {
		rootCalls++
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return rootErr
	}
	t.Cleanup(func() {
		removeArtifactDownload, removeArtifactRoot = originalRemoveDownload, originalRemoveRoot
	})

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName, f.name = "tool", fixture.name
	result, err := f.processReleaseArtifact(download.Name(), "download")
	if err != nil {
		t.Fatal(err)
	}
	err = result.Close()
	if !errors.Is(err, downloadErr) || !errors.Is(err, rootErr) {
		t.Fatalf("Close() error = %v, want both cleanup errors", err)
	}
	if err := result.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if removeCalls != 1 || rootCalls != 1 {
		t.Fatalf("cleanup calls = download:%d root:%d, want one each", removeCalls, rootCalls)
	}
}

type closeTracker struct {
	closed *int
}

func (c closeTracker) Read([]byte) (int, error) { return 0, io.EOF }

func (c closeTracker) Close() error {
	*c.closed++
	return nil
}

type interruptedReadCloser struct {
	closed *bool
}

func (r *interruptedReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("interrupted download")
}
func (r *interruptedReadCloser) Close() error {
	*r.closed = true
	return nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestArtifactInventoryClassifiesAndRejectsAliasedMembers(t *testing.T) {
	inventory := newArtifactInventory()
	for _, test := range []struct {
		name  string
		class artifactEntryClass
	}{
		{name: "bin/tool", class: artifactEntryExecutable},
		{name: "share/completions/tool", class: artifactEntryCompletion},
		{name: "tool.notarization.json", class: artifactEntryIgnored},
	} {
		entry, err := inventory.add(test.name, classifyArtifactEntry(test.name))
		if err != nil {
			t.Fatalf("add(%q) = %v", test.name, err)
		}
		if entry.class != test.class {
			t.Fatalf("add(%q) class = %d, want %d", test.name, entry.class, test.class)
		}
	}
	if _, err := inventory.add(`bin\tool`, artifactEntryExecutable); !errors.Is(err, ErrDuplicateArtifactMember) {
		t.Fatalf("aliased member error = %v, want duplicate", err)
	}
	for _, name := range []string{"../tool", "bin/../tool", "/tool", "bin//tool", "bin/C:/tool"} {
		if _, err := normalizeArtifactMemberIdentity(name); err == nil {
			t.Errorf("normalizeArtifactMemberIdentity(%q) succeeded", name)
		}
	}

	caseInventory := newArtifactInventory()
	if _, err := caseInventory.add("bin/tool", artifactEntryExecutable); err != nil {
		t.Fatal(err)
	}
	_, err := caseInventory.add("bin/Tool", artifactEntryExecutable)
	if err != nil {
		t.Fatalf("case-sensitive names should remain distinct: %v", err)
	}
}

func TestProcessURLBoundsStreamingDownloadWithoutContentLength(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 4, maxArchiveEntries: 10, maxEntryBytes: 10, maxExpandedBytes: 10, maxNesting: 2}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want download limit", err)
	}
}

func TestProcessReaderProcessesPlainAndArchiveArtifacts(t *testing.T) {
	fixture := testRunnableFixture("tool")
	archive := buildTestZipArchive(t, map[string]string{fixture.name: fixture.contents})

	for _, test := range []struct {
		name          string
		artifactName  string
		payload       []byte
		wantUnchanged bool
	}{
		{name: "plain", artifactName: fixture.name, payload: []byte(fixture.contents), wantUnchanged: true},
		{name: "archive", artifactName: "tool.zip", payload: archive, wantUnchanged: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := NewFilter(&FilterOpts{NonInteractive: true})
			f.repoName = "tool"
			expected := sha256.Sum256(test.payload)
			result, err := f.ProcessReader(test.artifactName, int64(len(test.payload)), bytes.NewReader(test.payload), fmt.Sprintf("%x", expected), true)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Source.(io.Closer).Close()

			contents, err := io.ReadAll(result.Source)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != fixture.contents {
				t.Fatalf("processed contents = %q", contents)
			}
			if result.Name != fixture.name || result.UnchangedBytes != test.wantUnchanged {
				t.Fatalf("result = name:%q unchanged:%v", result.Name, result.UnchangedBytes)
			}
			if result.DownloadSHA256 != fmt.Sprintf("%x", expected) || result.InstalledSHA256 == "" {
				t.Fatalf("result digests = download:%q installed:%q", result.DownloadSHA256, result.InstalledSHA256)
			}
		})
	}
}

func TestProcessReaderBoundsStreamingDownload(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 4, maxArchiveEntries: 10, maxEntryBytes: 10, maxExpandedBytes: 10, maxNesting: 2}
	t.Cleanup(func() { artifactProcessingBudgets = original })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessReader("tool", -1, strings.NewReader("12345"), "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessReader() error = %v, want download limit", err)
	}
}

func TestProcessURLDelegatesAfterSingleRequest(t *testing.T) {
	fixture := testRunnableFixture("tool")
	originalClient := httpClient
	requests := 0
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if got := req.Header.Get("Accept"); got != "application/octet-stream" {
			t.Fatalf("Accept header = %q", got)
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: int64(len(fixture.contents)),
			Body:          io.NopCloser(strings.NewReader(fixture.contents)),
			Request:       req,
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalClient })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	result, err := f.ProcessURL(&FilteredAsset{Name: fixture.name, URL: "https://example.test/tool", ExtraHeaders: map[string]string{"Accept": "application/octet-stream"}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Source.(io.Closer).Close()
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestProcessURLBoundsPlainArtifactExpandedBytes(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 64, maxArchiveEntries: 10, maxEntryBytes: 64, maxExpandedBytes: 4, maxNesting: 2}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "#!/bin/sh\nexit 0\n")
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want expanded byte limit", err)
	}
}

func TestProcessURLWithNilOptionsProcessesPlainArtifact(t *testing.T) {
	fixture := testRunnableFixture("tool")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture.contents)
	}))
	defer server.Close()

	result, err := NewFilter(nil).ProcessURL(&FilteredAsset{Name: fixture.name, URL: server.URL}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Source.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessURLBoundsPlainArtifactEntryBytes(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 64, maxArchiveEntries: 10, maxEntryBytes: 4, maxExpandedBytes: 64, maxNesting: 2}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "#!/bin/sh\nexit 0\n")
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want entry byte limit", err)
	}
}

func TestProcessURLBoundsDecodedPayloadEntryBytes(t *testing.T) {
	payload := gzipPayload(t, "#!/bin/sh\nexit 0\n")
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 64, maxArchiveEntries: 10, maxEntryBytes: 4, maxExpandedBytes: 64, maxNesting: 2}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.gz", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want decoded entry byte limit", err)
	}
}

func TestProcessURLCountsDecodedPlainPayloadOnce(t *testing.T) {
	fixture := testRunnableFixture("tool")
	payload := gzipPayload(t, fixture.contents)
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 64, maxArchiveEntries: 10, maxEntryBytes: int64(len(fixture.contents)), maxExpandedBytes: int64(len(fixture.contents)), maxNesting: 2}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	result, err := f.ProcessURL(&FilteredAsset{Name: fixture.name + ".gz", URL: server.URL}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Source.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessURLUsesEmbeddedGzipMemberIdentity(t *testing.T) {
	fixture := testRunnableFixture("tool")
	payload := gzipPayloadNamed(t, fixture.name, fixture.contents)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	result, err := f.ProcessURL(&FilteredAsset{Name: fixture.name + ".gz", URL: server.URL}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Source.(io.Closer).Close()
	if result.Name != fixture.name || result.PackagePath != fixture.name {
		t.Fatalf("gzip result = name:%q package:%q, want %q", result.Name, result.PackagePath, fixture.name)
	}
}

func TestProcessURLDoesNotSelectExecutablesInsideCompletionArchives(t *testing.T) {
	inner := buildTestZipArchive(t, map[string]string{"tool": "#!/bin/sh\nexit 0\n"})
	for _, test := range []struct {
		name    string
		archive []byte
		asset   string
	}{
		{name: "zip", archive: buildTestZipArchive(t, map[string]string{"share/completions/tool.zip": string(inner)}), asset: "tool.zip"},
		{name: "tar", archive: tarPayload(t, map[string]string{"share/completions/tool.zip": string(inner)}), asset: "tool.tar"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(test.archive) }))
			defer server.Close()
			f := NewFilter(&FilterOpts{NonInteractive: true})
			f.repoName = "tool"
			_, err := f.ProcessURL(&FilteredAsset{Name: test.asset, URL: server.URL}, "", false)
			if !errors.Is(err, ErrNoCompatibleFiles) {
				t.Fatalf("ProcessURL() error = %v, want no compatible files", err)
			}
		})
	}
}

func TestExecutableGzipWithMetadataMemberIsIgnoredWithoutStaging(t *testing.T) {
	payload := gzipPayloadNamed(t, "tool.notarization.json", "#!/bin/sh\nexit 0\n")
	download, err := os.CreateTemp(t.TempDir(), "artifact-download-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := download.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := download.Close(); err != nil {
		t.Fatal(err)
	}
	tracker, err := newArtifactBudgetTracker(defaultArtifactBudgets())
	if err != nil {
		t.Fatal(err)
	}
	inventory := newArtifactInventory()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	if err := f.collectArtifact(download.Name(), "tool.gz", "", t.TempDir(), tracker, inventory, new(bool), false); err != nil {
		t.Fatal(err)
	}
	if len(inventory.entries) != 1 || inventory.entries[0].class != artifactEntryIgnored || inventory.entries[0].stagedPath != "" {
		t.Fatalf("inventory = %+v, want one unstaged ignored gzip member", inventory.entries)
	}
}

func TestProcessURLRejectsUnsafeEmbeddedGzipMemberIdentity(t *testing.T) {
	payload := gzipPayloadNamed(t, "../tool", "#!/bin/sh\nexit 0\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.gz", URL: server.URL}, "", false)
	if err == nil {
		t.Fatal("ProcessURL() accepted unsafe embedded gzip member name")
	}
}

func TestProcessURLRejectsConcatenatedGzipMembers(t *testing.T) {
	payload := concatGzipPayloads(
		gzipPayloadNamed(t, "tool", "#!/bin/sh\nexit 0\n"),
		gzipPayloadNamed(t, "other", "#!/bin/sh\nexit 0\n"),
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.gz", URL: server.URL}, "", false)
	if !errors.Is(err, ErrUnsupportedArtifactFormat) {
		t.Fatalf("ProcessURL() error = %v, want concatenated gzip rejection", err)
	}
}

func TestProcessReaderAcceptsUnnamedConcatenatedGzipTar(t *testing.T) {
	fixture := testRunnableFixture("tool")
	tarArchive := tarPayload(t, map[string]string{fixture.name: fixture.contents})
	cut := len(tarArchive) / 2
	payload := concatGzipPayloads(
		gzipPayload(t, string(tarArchive[:cut])),
		gzipPayload(t, string(tarArchive[cut:])),
	)

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	result, err := f.ProcessReader("tool.tar.gz", int64(len(payload)), bytes.NewReader(payload), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Source.(io.Closer).Close()
	if result.Name != fixture.name || result.PackagePath != fixture.name {
		t.Fatalf("result = name:%q package:%q, want %q", result.Name, result.PackagePath, fixture.name)
	}
}

func TestProcessReaderBoundsUnnamedConcatenatedGzipTarCumulatively(t *testing.T) {
	tarArchive := tarPayload(t, map[string]string{"tool": "#!/bin/sh\nexit 0\n"})
	cut := len(tarArchive) / 2
	payload := concatGzipPayloads(
		gzipPayload(t, string(tarArchive[:cut])),
		gzipPayload(t, string(tarArchive[cut:])),
	)
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: int64(len(payload)), maxArchiveEntries: 10, maxEntryBytes: int64(len(tarArchive) - 1), maxExpandedBytes: int64(len(tarArchive) * 2), maxNesting: 4}
	t.Cleanup(func() { artifactProcessingBudgets = original })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessReader("tool.tar.gz", int64(len(payload)), bytes.NewReader(payload), "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessReader() error = %v, want cumulative entry limit", err)
	}
}

func TestProcessURLRejectsUnsafeLaterGzipMemberIdentity(t *testing.T) {
	payload := concatGzipPayloads(
		gzipPayloadNamed(t, "tool", "#!/bin/sh\nexit 0\n"),
		gzipPayloadNamed(t, "../tool", "#!/bin/sh\nexit 0\n"),
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.gz", URL: server.URL}, "", false)
	if err == nil {
		t.Fatal("ProcessURL() accepted unsafe later gzip member identity")
	}
}

func TestProcessURLRejectsAliasedLaterGzipMemberIdentity(t *testing.T) {
	payload := concatGzipPayloads(
		gzipPayloadNamed(t, "bin/tool", "#!/bin/sh\nexit 0\n"),
		gzipPayloadNamed(t, `bin\tool`, "#!/bin/sh\nexit 0\n"),
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.gz", URL: server.URL}, "", false)
	if !errors.Is(err, ErrDuplicateArtifactMember) {
		t.Fatalf("ProcessURL() error = %v, want duplicate gzip identity", err)
	}
}

func TestProcessURLClosesInterruptedDownload(t *testing.T) {
	originalClient := httpClient
	closed := false
	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: &interruptedReadCloser{closed: &closed}, Request: req}, nil
	})}
	t.Cleanup(func() { httpClient = originalClient })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool", URL: "https://example.test/tool"}, "", false)
	if err == nil {
		t.Fatal("ProcessURL() succeeded after interrupted download")
	}
	if !closed {
		t.Fatal("ProcessURL() did not close interrupted response body")
	}
}

func TestReleaseArtifactInventoryStagesCompletionsAndCleansUp(t *testing.T) {
	fixture := testRunnableFixture("tool")
	archive := buildTestZipArchive(t, map[string]string{
		fixture.name:             fixture.contents,
		"docs/readme.txt":        "ignored",
		"share/completions/tool": "complete -c tool\n",
	})
	download, err := os.CreateTemp("", "artifact-download-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := download.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := download.Close(); err != nil {
		t.Fatal(err)
	}
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName, f.name = "tool", "tool.zip"
	result, err := f.processReleaseArtifact(download.Name(), "download")
	if err != nil {
		t.Fatal(err)
	}
	var completionPath, ignoredPath string
	for _, entry := range result.inventory.entries {
		switch entry.class {
		case artifactEntryCompletion:
			completionPath = entry.stagedPath
		case artifactEntryIgnored:
			ignoredPath = entry.stagedPath
		}
	}
	if completionPath == "" {
		t.Fatal("completion entry was not staged")
	}
	if ignoredPath != "" {
		t.Fatalf("ignored entry was staged at %q", ignoredPath)
	}
	if _, err := os.Stat(completionPath); err != nil {
		t.Fatalf("completion staging path is unavailable: %v", err)
	}
	root := filepath.Dir(completionPath)
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging root remains after Close: %v", err)
	}
}

func TestProcessReleaseArtifactSelectsBundledCompletionInExecutableScope(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	for _, test := range []struct {
		name      string
		archive   string
		shell     string
		candidate string
	}{
		{name: "zip bash filename", archive: "tool.zip", shell: "bash", candidate: "share/completions/tool.bash"},
		{name: "tar bash directory", archive: "tool.tar", shell: "bash", candidate: "share/autocomplete/bash/tool"},
		{name: "zip zsh", archive: "tool.zip", shell: "zsh", candidate: "share/complete/_tool"},
		{name: "tar fish", archive: "tool.tar", shell: "fish", candidate: "share/completions/tool.fish"},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected := buildTestZipArchive(t, map[string]string{
				"bin/tool":     "#!/bin/sh\nexit 0\n",
				test.candidate: "selected completion\n",
			})
			other := buildTestZipArchive(t, map[string]string{
				"bin/other":    "#!/bin/sh\nexit 0\n",
				test.candidate: "other completion\n",
			})
			files := map[string]string{"selected.zip": string(selected), "other.zip": string(other)}
			var archive []byte
			if test.archive == "tool.tar" {
				archive = tarPayload(t, files)
			} else {
				archive = buildTestZipArchive(t, files)
			}

			download := writeArtifactDownload(t, archive)
			filter := NewFilter(&FilterOpts{
				NonInteractive:           true,
				SelectionIntent:          &config.SelectionDescriptor{ArchiveMember: "selected.zip!/bin/tool"},
				BundledCompletionShell:   test.shell,
				BundledCompletionCommand: "tool",
			})
			filter.repoName, filter.name = "tool", test.archive
			result, err := filter.processReleaseArtifact(download, "download")
			if err != nil {
				t.Fatal(err)
			}
			if result.final.PackagePath != "selected.zip!/bin/tool" {
				t.Fatalf("selected PackagePath = %q", result.final.PackagePath)
			}
			if got := string(result.final.BundledCompletion); got != "selected completion\n" {
				t.Fatalf("BundledCompletion = %q", got)
			}
			if got := result.final.BundledCompletionName; got != "selected.zip!/"+test.candidate {
				t.Fatalf("BundledCompletionName = %q", got)
			}
			if err := result.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProcessReleaseArtifactSkipsUnusableBundledCompletions(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	for _, test := range []struct {
		name        string
		command     string
		completions map[string]string
		warn        bool
	}{
		{name: "default operation", completions: map[string]string{"share/completions/tool.bash": "complete tool\n"}},
		{name: "duplicate exact match", command: "tool", completions: map[string]string{"share/completions/tool.bash": "one", "share/complete/tool.bash": "two"}, warn: true},
		{name: "wrong command", command: "tool", completions: map[string]string{"share/completions/other.bash": "complete other\n"}},
		{name: "renamed command", command: "alias", completions: map[string]string{"share/completions/tool.bash": "complete tool\n"}},
		{name: "empty", command: "tool", completions: map[string]string{"share/completions/tool.bash": ""}, warn: true},
		{name: "non utf8", command: "tool", completions: map[string]string{"share/completions/tool.bash": "\xff"}, warn: true},
		{name: "nul", command: "tool", completions: map[string]string{"share/completions/tool.bash": "complete\x00tool"}, warn: true},
		{name: "oversized", command: "tool", completions: map[string]string{"share/completions/tool.bash": strings.Repeat("x", bundledCompletionMaxBytes+1)}, warn: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			logs := captureDebugLogs(t)
			files := map[string]string{"bin/tool": "#!/bin/sh\nexit 0\n"}
			for name, content := range test.completions {
				files[name] = content
			}
			download := writeArtifactDownload(t, buildTestZipArchive(t, files))
			filter := NewFilter(&FilterOpts{NonInteractive: true, BundledCompletionShell: "bash", BundledCompletionCommand: test.command})
			filter.repoName, filter.name = "tool", "tool.zip"
			result, err := filter.processReleaseArtifact(download, "download")
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if result.final.PackagePath != "bin/tool" {
				t.Fatalf("PackagePath = %q, want selected executable", result.final.PackagePath)
			}
			if result.final.BundledCompletion != nil || result.final.BundledCompletionName != "" {
				t.Fatalf("unexpected bundled completion = %q (%q)", result.final.BundledCompletion, result.final.BundledCompletionName)
			}
			warned := strings.Contains(logs.String(), "Skipping bundled completion") || strings.Contains(logs.String(), "Skipping ambiguous bundled")
			if warned != test.warn {
				t.Fatalf("bundled completion warning = %t, want %t: %q", warned, test.warn, logs.String())
			}
		})
	}
}

func writeArtifactDownload(t *testing.T, content []byte) string {
	t.Helper()
	download, err := os.CreateTemp(t.TempDir(), "artifact-download-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := download.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := download.Close(); err != nil {
		t.Fatal(err)
	}
	return download.Name()
}

func TestProcessReleaseArtifactResolvesArchiveMembersSafely(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	process := func(t *testing.T, files []archiveTestFile, filter *Filter) (*artifactProcessingResult, error) {
		t.Helper()
		download, err := os.CreateTemp(t.TempDir(), "artifact-download-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := download.Write(buildOrderedTestZipArchive(t, files)); err != nil {
			t.Fatal(err)
		}
		if err := download.Close(); err != nil {
			t.Fatal(err)
		}
		filter.repoName, filter.name = "tool", "tool.zip"
		return filter.processReleaseArtifact(download.Name(), "download")
	}

	t.Run("stored member identity outranks helper", func(t *testing.T) {
		result, err := process(t, []archiveTestFile{
			{name: "helpers/tool", body: "#!/bin/sh\nexit 0\n"},
			{name: "release/bin/tool", body: "#!/bin/sh\nexit 0\n"},
		}, NewFilter(&FilterOpts{NonInteractive: true, PackagePath: "release/bin/tool"}))
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		if result.final.PackagePath != "release/bin/tool" {
			t.Fatalf("PackagePath = %q", result.final.PackagePath)
		}
	})

	t.Run("persisted version wrapper resolves and stores portable member", func(t *testing.T) {
		filter := NewFilter(&FilterOpts{NonInteractive: true, SelectionIntent: &config.SelectionDescriptor{ArchiveMember: "tool-v1/bin/tool"}})
		result, err := process(t, []archiveTestFile{{name: "tool-v2/bin/tool", body: "#!/bin/sh\nexit 0\n"}}, filter)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		if result.final.PackagePath != "tool-v2/bin/tool" {
			t.Fatalf("PackagePath = %q", result.final.PackagePath)
		}
		if got := filter.SelectionIntent(); got == nil || got.ArchiveMember != "bin/tool" {
			t.Fatalf("selection intent = %#v, want portable member bin/tool", got)
		}
	})

	t.Run("logical package name outranks repository identity", func(t *testing.T) {
		result, err := process(t, []archiveTestFile{
			{name: "tool", body: "#!/bin/sh\nexit 0\n"},
			{name: "alternate", body: "#!/bin/sh\nexit 0\n"},
		}, NewFilter(&FilterOpts{NonInteractive: true, PackageName: "alternate"}))
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		if result.final.PackagePath != "alternate" {
			t.Fatalf("PackagePath = %q, want alternate", result.final.PackagePath)
		}
	})

	t.Run("explicit member validates identity target and payload", func(t *testing.T) {
		files := []archiveTestFile{
			{name: "bin/tool", body: "#!/bin/sh\nexit 0\n"},
			{name: "bin/tool-windows.exe", body: "#!/bin/sh\nexit 0\n"},
			{name: "bin/not-runnable", body: "data"},
		}
		result, err := process(t, files, &Filter{opts: &FilterOpts{NonInteractive: true}, containedFile: `bin\tool`})
		if err != nil {
			t.Fatal(err)
		}
		if err := result.Close(); err != nil {
			t.Fatal(err)
		}
		for _, containedFile := range []string{"../tool", "bin/tool-windows.exe", "bin/not-runnable"} {
			_, err := process(t, files, &Filter{opts: &FilterOpts{NonInteractive: true}, containedFile: containedFile})
			if containedFile == "../tool" {
				assertArchiveResolutionReason(t, err, ErrInvalidArchiveMemberSelection, ArchiveMemberInvalidSelection)
			} else {
				assertArchiveResolutionReason(t, err, ErrIncompatibleArchiveMemberSelection, ArchiveMemberIncompatibleSelection)
			}
		}
	})

	t.Run("empty explicit member is invalid", func(t *testing.T) {
		filter := NewFilter(&FilterOpts{NonInteractive: true})
		if outer := filter.ParseAutoSelection("tool.zip:"); outer != "tool.zip" {
			t.Fatalf("outer selection = %q, want tool.zip", outer)
		}
		_, err := process(t, []archiveTestFile{{name: "tool", body: "#!/bin/sh\nexit 0\n"}}, filter)
		assertArchiveResolutionReason(t, err, ErrInvalidArchiveMemberSelection, ArchiveMemberInvalidSelection)
	})

	t.Run("duplicate basenames remain ambiguous in either archive order", func(t *testing.T) {
		for _, files := range [][]archiveTestFile{
			{{name: "one/tool", body: "#!/bin/sh\nexit 0\n"}, {name: "two/tool", body: "#!/bin/sh\nexit 0\n"}},
			{{name: "two/tool", body: "#!/bin/sh\nexit 0\n"}, {name: "one/tool", body: "#!/bin/sh\nexit 0\n"}},
		} {
			_, err := process(t, files, NewFilter(&FilterOpts{NonInteractive: true}))
			assertArchiveResolutionReason(t, err, ErrAmbiguousArchiveMember, ArchiveMemberAmbiguous)
		}
	})

	t.Run("interactive ambiguity prompts with stable candidates", func(t *testing.T) {
		originalInteractive, originalSelect := isInteractive, selectOption
		isInteractive = func() bool { return true }
		selectOption = func(_ string, options []fmt.Stringer) (interface{}, error) {
			if options[0].String() != "one/tool" || options[1].String() != "two/tool" {
				t.Fatalf("options = %v, %v", options[0], options[1])
			}
			return options[1], nil
		}
		t.Cleanup(func() { isInteractive, selectOption = originalInteractive, originalSelect })
		result, err := process(t, []archiveTestFile{{name: "two/tool", body: "#!/bin/sh\nexit 0\n"}, {name: "one/tool", body: "#!/bin/sh\nexit 0\n"}}, NewFilter(&FilterOpts{}))
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		if result.final.PackagePath != "two/tool" {
			t.Fatalf("PackagePath = %q, want two/tool", result.final.PackagePath)
		}
	})
}

func TestProcessReleaseArtifactPersistsOnlyRealArchiveMembers(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })
	fixture := testRunnableFixture("tool")

	process := func(t *testing.T, name string, payload []byte, intent *config.SelectionDescriptor) (*Filter, *artifactProcessingResult, error) {
		t.Helper()
		download, err := os.CreateTemp(t.TempDir(), "artifact-download-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := download.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := download.Close(); err != nil {
			t.Fatal(err)
		}
		filter := NewFilter(&FilterOpts{NonInteractive: true, SelectionIntent: intent})
		filter.repoName, filter.name = "tool", name
		result, err := filter.processReleaseArtifact(download.Name(), "download")
		return filter, result, err
	}

	t.Run("raw payload records no member and remains selectable", func(t *testing.T) {
		filter, result, err := process(t, fixture.name, []byte(fixture.contents), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		intent := filter.SelectionIntent()
		if intent == nil || intent.ArchiveMember != "" || result.final.PackagePath != "" {
			t.Fatalf("raw selection = intent:%#v package:%q", intent, result.final.PackagePath)
		}
		candidate := describeReleaseCandidate(&Asset{Name: fixture.name}, "tool")
		if _, err := ResolvePersistedSelection([]ReleaseCandidate{candidate}, ReleaseCandidateResolutionRequest{}, intent); err != nil {
			t.Fatalf("next raw release selection failed: %v", err)
		}
		_, next, err := process(t, fixture.name, []byte(fixture.contents), intent)
		if err != nil {
			t.Fatalf("next raw release processing failed: %v", err)
		}
		defer next.Close()
	})

	t.Run("extensionless zip resolves persisted member", func(t *testing.T) {
		intent := &config.SelectionDescriptor{ArchiveMember: fixture.name}
		filter, result, err := process(t, fixture.name, buildTestZipArchive(t, map[string]string{fixture.name: fixture.contents}), intent)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		if got := filter.SelectionIntent(); got == nil || got.ArchiveMember != fixture.name || result.final.PackagePath != fixture.name {
			t.Fatalf("extensionless archive selection = intent:%#v package:%q", got, result.final.PackagePath)
		}
	})

	t.Run("same-name raw payload cannot satisfy member assertion", func(t *testing.T) {
		_, result, err := process(t, fixture.name, []byte(fixture.contents), &config.SelectionDescriptor{ArchiveMember: fixture.name})
		if result != nil {
			_ = result.Close()
		}
		if !errors.Is(err, ErrUnavailablePersistedSelection) || !errors.Is(err, ErrNoEligibleArchiveMember) {
			t.Fatalf("raw member assertion error = %v", err)
		}
	})

	t.Run("single-file compression is transformed but not member-bearing", func(t *testing.T) {
		filter, result, err := process(t, fixture.name+".gz", gzipPayloadNamed(t, fixture.name, fixture.contents), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()
		if !result.transformed || filter.SelectionIntent().ArchiveMember != "" {
			t.Fatalf("gzip selection = transformed:%t intent:%#v", result.transformed, filter.SelectionIntent())
		}
	})
}

type archiveTestFile struct {
	name string
	body string
}

func buildOrderedTestZipArchive(t *testing.T, files []archiveTestFile) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, file := range files {
		entry, err := writer.Create(file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, file.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func TestProcessURLRejectsMetadataSidecarsBeforeSelection(t *testing.T) {
	for _, name := range []string{"tool.notarization.json", "tool.notarization.json.gz", "tool.notarization.json.xz", "tool.notarization.json.bz2", "tool.notarization.json.zst"} {
		t.Run(name, func(t *testing.T) {
			f := NewFilter(&FilterOpts{NonInteractive: true})
			_, err := f.FilterAssets("tool", []*Asset{{Name: name, URL: "https://example.test/" + name}}, "")
			if !errors.Is(err, ErrNoCompatibleFiles) {
				t.Fatalf("FilterAssets() error = %v, want no compatible files", err)
			}
			if class := classifyArtifactEntry(name); class != artifactEntryIgnored {
				t.Fatalf("classifyArtifactEntry(%q) = %d, want ignored", name, class)
			}
		})
	}
}

func TestProcessURLRejectsCompressedMetadataSidecarBeforePayloadSelection(t *testing.T) {
	payload := gzipPayload(t, "#!/bin/sh\nexit 0\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.notarization.json.gz", URL: server.URL}, "", false)
	if !errors.Is(err, ErrNoCompatibleFiles) {
		t.Fatalf("ProcessURL() error = %v, want no compatible files", err)
	}
}

func TestProcessURLOpaquePayloadUsesRunnableGate(t *testing.T) {
	fixture := testRunnableFixture("tool")
	for _, test := range []struct {
		name      string
		assetName string
		body      string
		want      error
	}{
		{name: "runnable", assetName: fixture.name, body: fixture.contents},
		{name: "opaque data", assetName: "tool", body: "not executable", want: ErrNoCompatibleFiles},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, test.body) }))
			defer server.Close()
			f := NewFilter(&FilterOpts{NonInteractive: true})
			f.repoName = "tool"
			result, err := f.ProcessURL(&FilteredAsset{Name: test.assetName, URL: server.URL}, "", false)
			if !errors.Is(err, test.want) {
				t.Fatalf("ProcessURL() error = %v, want %v", err, test.want)
			}
			if result != nil {
				if err := result.Source.(io.Closer).Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestProcessURLOpaqueArchiveNamedPayloadUsesRunnableGate(t *testing.T) {
	fixture := testRunnableFixture("tool")
	for _, test := range []struct {
		name      string
		assetName string
		body      string
		want      error
	}{
		{name: "runnable zip name", assetName: fixture.name, body: fixture.contents},
		{name: "runnable tar name", assetName: fixture.name, body: fixture.contents},
		{name: "invalid zip name", body: "not executable", want: ErrNoCompatibleFiles},
		{name: "invalid tar name", body: "not executable", want: ErrNoCompatibleFiles},
	} {
		t.Run(test.name, func(t *testing.T) {
			assetName := test.assetName
			if assetName == "" {
				assetName = "tool.zip"
				if strings.Contains(test.name, "tar") {
					assetName = "tool.tar"
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, test.body) }))
			defer server.Close()

			f := NewFilter(&FilterOpts{NonInteractive: true})
			f.repoName = "tool"
			result, err := f.ProcessURL(&FilteredAsset{Name: assetName, URL: server.URL}, "", false)
			if !errors.Is(err, test.want) {
				t.Fatalf("ProcessURL() error = %v, want %v", err, test.want)
			}
			if result != nil {
				if err := result.Source.(io.Closer).Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestArchiveObservesDirectoriesAndNestedMembersWithoutIdentityCollisions(t *testing.T) {
	fixture := testRunnableFixture("tool")
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 1 << 20, maxArchiveEntries: 1, maxEntryBytes: 1 << 20, maxExpandedBytes: 1 << 20, maxNesting: 4}
	t.Cleanup(func() { artifactProcessingBudgets = original })

	archive := buildTestZipArchive(t, map[string]string{"dir/": "", fixture.name: fixture.contents})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want entry limit", err)
	}

	artifactProcessingBudgets.maxArchiveEntries = 10
	inner := buildTestZipArchive(t, map[string]string{fixture.name: fixture.contents})
	outer := buildTestZipArchive(t, map[string]string{"nested.zip": string(inner)})
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(outer) }))
	defer server.Close()
	result, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Source.(io.Closer).Close()
	if result.PackagePath != "nested.zip!/"+fixture.name {
		t.Fatalf("PackagePath = %q, want scoped nested identity", result.PackagePath)
	}
}

func TestTarCountsAndValidatesNonRegularEntries(t *testing.T) {
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err := writer.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "tool"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteHeader(&tar.Header{Name: "tool", Mode: 0o755, Size: int64(len("#!/bin/sh\nexit 0\n"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	tracker, err := newArtifactBudgetTracker(artifactBudgets{maxDownloadBytes: 1, maxArchiveEntries: 1, maxEntryBytes: 1 << 20, maxExpandedBytes: 1 << 20, maxNesting: 2})
	if err != nil {
		t.Fatal(err)
	}
	f := NewFilter(&FilterOpts{NonInteractive: true})
	err = f.collectTar(bytes.NewReader(archive.Bytes()), "", t.TempDir(), tracker, newArtifactInventory(), new(bool))
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("collectTar() error = %v, want non-regular entry to consume count budget", err)
	}
}

func TestArchiveRootMarkersRemainBoundedAndDoNotHideDuplicates(t *testing.T) {
	t.Run("zip root and normalized duplicate", func(t *testing.T) {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		for _, name := range []string{"./", "./tool", "tool"} {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if name != "./" {
				_, _ = io.WriteString(entry, "#!/bin/sh\nexit 0\n")
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		tracker, _ := newArtifactBudgetTracker(defaultArtifactBudgets())
		err := new(Filter).collectZip(bytes.NewReader(archive.Bytes()), "", t.TempDir(), tracker, newArtifactInventory(), new(bool))
		if !errors.Is(err, ErrDuplicateArtifactMember) {
			t.Fatalf("collectZip() error = %v, want duplicate normalized member", err)
		}
	})

	t.Run("repeated roots consume entry budget", func(t *testing.T) {
		var archive bytes.Buffer
		writer := tar.NewWriter(&archive)
		for range 2 {
			if err := writer.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		tracker, _ := newArtifactBudgetTracker(artifactBudgets{maxDownloadBytes: 1, maxArchiveEntries: 1, maxEntryBytes: 1, maxExpandedBytes: 1, maxNesting: 2})
		err := new(Filter).collectTar(bytes.NewReader(archive.Bytes()), "", t.TempDir(), tracker, newArtifactInventory(), new(bool))
		if !errors.Is(err, ErrArtifactLimitExceeded) {
			t.Fatalf("collectTar() error = %v, want entry limit", err)
		}
	})

	t.Run("root-named regular bytes consume byte budget", func(t *testing.T) {
		var archive bytes.Buffer
		writer := tar.NewWriter(&archive)
		if err := writer.WriteHeader(&tar.Header{Name: ".", Mode: 0o644, Size: 4}); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(writer, "data")
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		tracker, _ := newArtifactBudgetTracker(artifactBudgets{maxDownloadBytes: 1, maxArchiveEntries: 2, maxEntryBytes: 3, maxExpandedBytes: 10, maxNesting: 2})
		err := new(Filter).collectTar(bytes.NewReader(archive.Bytes()), "", t.TempDir(), tracker, newArtifactInventory(), new(bool))
		if !errors.Is(err, ErrArtifactLimitExceeded) {
			t.Fatalf("collectTar() error = %v, want byte limit", err)
		}
	})
}

func TestIgnoredNestedArchiveRootMarkersAreBenign(t *testing.T) {
	var tarArchive bytes.Buffer
	tarWriter := tar.NewWriter(&tarArchive)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var zipArchive bytes.Buffer
	zipWriter := zip.NewWriter(&zipArchive)
	if _, err := zipWriter.Create("./"); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name    string
		payload []byte
	}{
		{name: "metadata.tar", payload: tarArchive.Bytes()},
		{name: "metadata.zip", payload: zipArchive.Bytes()},
	} {
		tracker, _ := newArtifactBudgetTracker(defaultArtifactBudgets())
		if err := discardIgnoredArtifact(bytes.NewReader(test.payload), test.name, "ignored", tracker, newArtifactInventory()); err != nil {
			t.Fatalf("discardIgnoredArtifact(%s) = %v", test.name, err)
		}
	}
}

func TestGzipRootNameIsNotASkippableArchiveEntry(t *testing.T) {
	payload := gzipPayloadNamed(t, "./", "data")
	download := filepath.Join(t.TempDir(), "tool.gz")
	if err := os.WriteFile(download, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	tracker, _ := newArtifactBudgetTracker(defaultArtifactBudgets())
	err := new(Filter).collectArtifact(download, "tool.gz", "", t.TempDir(), tracker, newArtifactInventory(), new(bool), false)
	if !errors.Is(err, errArchiveRootMember) {
		t.Fatalf("collectArtifact() error = %v, want root-name rejection", err)
	}
}

func TestTarStagesCompletionButNotIgnoredContent(t *testing.T) {
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for name, content := range map[string]string{
		"share/completions/tool": "complete -c tool\n",
		"docs/readme.txt":        "ignored",
	} {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	tracker, err := newArtifactBudgetTracker(defaultArtifactBudgets())
	if err != nil {
		t.Fatal(err)
	}
	inventory := newArtifactInventory()
	root := t.TempDir()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	if err := f.collectTar(bytes.NewReader(archive.Bytes()), "", root, tracker, inventory, new(bool)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range inventory.entries {
		switch entry.class {
		case artifactEntryCompletion:
			if entry.stagedPath == "" {
				t.Fatal("tar completion was not staged")
			}
		case artifactEntryIgnored:
			if entry.stagedPath != "" {
				t.Fatalf("tar ignored content was staged at %q", entry.stagedPath)
			}
		}
	}
}

func TestProcessURLRejectsDuplicateArchiveIdentitiesAndZstandard(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"bin/tool", `bin\tool`} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("#!/bin/sh\nexit 0\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		body []byte
		want error
	}{
		{name: "duplicate", body: archive.Bytes(), want: ErrDuplicateArtifactMember},
		{name: "zstandard", body: []byte{0x28, 0xb5, 0x2f, 0xfd}, want: ErrUnsupportedArtifactFormat},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(test.body) }))
			defer server.Close()
			f := NewFilter(&FilterOpts{NonInteractive: true})
			f.repoName = "tool"
			_, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
			if !errors.Is(err, test.want) {
				t.Fatalf("ProcessURL() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestProcessURLBoundsCompressedArchiveNesting(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 1 << 20, maxArchiveEntries: 10, maxEntryBytes: 1 << 20, maxExpandedBytes: 1 << 20, maxNesting: 1}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	archive := buildTestTarGzArchive(t, map[string]string{"tool": "#!/bin/sh\nexit 0\n"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.tar.gz", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want nesting limit", err)
	}
}

func TestProcessURLCountsIgnoredArchiveEntryBytes(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 1 << 20, maxArchiveEntries: 10, maxEntryBytes: 3, maxExpandedBytes: 1 << 20, maxNesting: 4}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	archive := buildTestZipArchive(t, map[string]string{
		"tool":            "ok",
		"docs/readme.txt": "this ignored entry is too large",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want ignored entry limit", err)
	}
}

func TestIgnoredCompressedArchiveMemberBoundsDecodedBytes(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 1 << 20, maxArchiveEntries: 10, maxEntryBytes: 1 << 20, maxExpandedBytes: 4, maxNesting: 4}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	archive := buildTestZipArchive(t, map[string]string{
		"tool":                      "#!/bin/sh\nexit 0\n",
		"tool.notarization.json.gz": string(gzipPayload(t, "decoded ignored sidecar")),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want decoded ignored expansion limit", err)
	}
}

func TestIgnoredCompressedArchiveMemberBoundsNesting(t *testing.T) {
	original := artifactProcessingBudgets
	artifactProcessingBudgets = artifactBudgets{maxDownloadBytes: 1 << 20, maxArchiveEntries: 10, maxEntryBytes: 1 << 20, maxExpandedBytes: 1 << 20, maxNesting: 1}
	t.Cleanup(func() { artifactProcessingBudgets = original })
	archive := buildTestZipArchive(t, map[string]string{
		"tool":                      "#!/bin/sh\nexit 0\n",
		"tool.notarization.json.gz": string(gzipPayload(t, "decoded ignored sidecar")),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
	if !errors.Is(err, ErrArtifactLimitExceeded) {
		t.Fatalf("ProcessURL() error = %v, want ignored nesting limit", err)
	}
}

func TestIgnoredCompressedArchiveMemberIsNotStaged(t *testing.T) {
	fixture := testRunnableFixture("tool")
	archive := buildTestZipArchive(t, map[string]string{
		fixture.name:                fixture.contents,
		"tool.notarization.json.gz": string(gzipPayload(t, "decoded ignored sidecar")),
	})
	download, err := os.CreateTemp(t.TempDir(), "artifact-download-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := download.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := download.Close(); err != nil {
		t.Fatal(err)
	}
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName, f.name = "tool", "tool.zip"
	result, err := f.processReleaseArtifact(download.Name(), "download")
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	for _, entry := range result.inventory.entries {
		if entry.identity == "tool.notarization.json.gz" {
			if entry.class != artifactEntryIgnored || entry.stagedPath != "" {
				t.Fatalf("ignored compressed entry = %+v, want unstaged ignored entry", entry)
			}
			return
		}
	}
	t.Fatal("ignored compressed entry not inventoried")
}

func TestIgnoredCompressedArchiveMemberRejectsUnsafeEmbeddedIdentity(t *testing.T) {
	payload := gzipPayloadNamed(t, "../tool", "ignored")
	for _, test := range []struct {
		name    string
		archive []byte
		asset   string
	}{
		{name: "zip", archive: buildTestZipArchive(t, map[string]string{"tool.notarization.json.gz": string(payload)}), asset: "tool.zip"},
		{name: "tar", archive: tarPayload(t, map[string]string{"tool.notarization.json.gz": string(payload)}), asset: "tool.tar"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(test.archive) }))
			defer server.Close()
			f := NewFilter(&FilterOpts{NonInteractive: true})
			f.repoName = "tool"
			_, err := f.ProcessURL(&FilteredAsset{Name: test.asset, URL: server.URL}, "", false)
			if err == nil {
				t.Fatal("ProcessURL() accepted unsafe ignored gzip member identity")
			}
		})
	}
}

func TestIgnoredCompressedArchiveMemberRejectsConcatenatedGzipMembers(t *testing.T) {
	payload := concatGzipPayloads(
		gzipPayloadNamed(t, "safe", "ignored"),
		gzipPayloadNamed(t, "../tool", "ignored"),
	)
	archive := buildTestZipArchive(t, map[string]string{"tool.notarization.json.gz": string(payload)})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool.zip", URL: server.URL}, "", false)
	if err == nil {
		t.Fatal("ProcessURL() accepted unsafe later ignored gzip member identity")
	}
}

func gzipPayload(t *testing.T, payload string) []byte {
	return gzipPayloadNamed(t, "", payload)
}

func gzipPayloadNamed(t *testing.T, name, payload string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	writer.Name = name
	if _, err := io.WriteString(writer, payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func concatGzipPayloads(payloads ...[]byte) []byte {
	return bytes.Join(payloads, nil)
}

func tarPayload(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
