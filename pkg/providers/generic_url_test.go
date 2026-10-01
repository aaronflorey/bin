package providers

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
)

func TestGenericURLGetLatestVersionFromRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("expected HEAD request, got %s", r.Method)
		}
		http.Redirect(w, r, "/artifacts/tool_1.2.3_linux_amd64.tar.gz", http.StatusFound)
	})
	mux.HandleFunc("/artifacts/tool_1.2.3_linux_amd64.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	u, err := url.Parse(server.URL + "/download")
	if err != nil {
		t.Fatalf("url parse failed: %v", err)
	}
	p, err := newGenericURL(u)
	if err != nil {
		t.Fatalf("newGenericURL failed: %v", err)
	}

	info, err := p.GetLatestVersion()
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected release info")
	}
	if info.Version != "1.2.3" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if !strings.HasSuffix(info.URL, "/artifacts/tool_1.2.3_linux_amd64.tar.gz") {
		t.Fatalf("unexpected release URL: %s", info.URL)
	}
}

func TestGenericURLGetLatestVersionFromContentDisposition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="tool_v2.4.1_darwin_arm64.zip"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL + "/download")
	if err != nil {
		t.Fatalf("url parse failed: %v", err)
	}
	p, err := newGenericURL(u)
	if err != nil {
		t.Fatalf("newGenericURL failed: %v", err)
	}

	info, err := p.GetLatestVersion()
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected release info")
	}
	if info.Version != "2.4.1" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
}

func TestGenericURLGetLatestVersionHeadFallbackToGet(t *testing.T) {
	var sawRange bool
	var headCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			headCalls++
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			if r.Header.Get("Range") == "bytes=0-0" {
				sawRange = true
			}
			w.Header().Set("Content-Disposition", `attachment; filename="tool_3.0.0_linux_amd64"`)
			_, _ = w.Write([]byte("payload"))
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	u, err := url.Parse(server.URL + "/download")
	if err != nil {
		t.Fatalf("url parse failed: %v", err)
	}
	p, err := newGenericURL(u)
	if err != nil {
		t.Fatalf("newGenericURL failed: %v", err)
	}

	info, err := p.GetLatestVersion()
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected release info")
	}
	if info.Version != "3.0.0" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if headCalls == 0 {
		t.Fatal("expected HEAD probe")
	}
	if !sawRange {
		t.Fatal("expected Range header in GET fallback")
	}
}

func TestGenericURLGetLatestVersionNoVersionReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="tool-linux-amd64"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL + "/download")
	if err != nil {
		t.Fatalf("url parse failed: %v", err)
	}
	p, err := newGenericURL(u)
	if err != nil {
		t.Fatalf("newGenericURL failed: %v", err)
	}

	info, err := p.GetLatestVersion()
	if err == nil {
		t.Fatal("expected error when version cannot be inferred")
	}
	if info != nil {
		t.Fatalf("expected nil release info on error, got %+v", info)
	}
}

func TestGenericURLFetchReturnsFileNameVersionAndData(t *testing.T) {
	payload := genericRunnablePayload(t)
	filename := genericArtifactName("", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL + "/download")
	if err != nil {
		t.Fatalf("url parse failed: %v", err)
	}
	p, err := newGenericURL(u)
	if err != nil {
		t.Fatalf("newGenericURL failed: %v", err)
	}

	file, err := p.Fetch(&FetchOpts{})
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	defer func() {
		if closer, ok := file.Data.(io.Closer); ok {
			_ = closer.Close()
		}
	}()

	if file.Name != "tool"+genericScriptExtension() {
		t.Fatalf("unexpected name: %s", file.Name)
	}
	if file.Version != "0.16.0" {
		t.Fatalf("unexpected version: %s", file.Version)
	}

	content, err := io.ReadAll(file.Data)
	if err != nil {
		t.Fatalf("read file content failed: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("unexpected payload: %q", string(content))
	}
	if file.SourceAsset != filename || !file.ProcessingUnchanged || file.DownloadIntegrity != nil || file.InstalledIntegrity != nil || file.ExpectedSHA != "" {
		t.Fatalf("unexpected generic provenance/integrity: %#v", file)
	}
}

func TestGenericURLFetchRejectsUnsafeFilenameBeforeSanitizing(t *testing.T) {
	for _, filename := range []string{"tool_1.2.3...", "tool_1.2.3.......", `tool_1.2.3\\child`} {
		t.Run(filename, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			u, err := url.Parse(server.URL + "/download")
			if err != nil {
				t.Fatal(err)
			}
			p, err := newGenericURL(u)
			if err != nil {
				t.Fatal(err)
			}
			file, err := p.Fetch(&FetchOpts{})
			if err == nil {
				if closer, ok := file.Data.(io.Closer); ok {
					_ = closer.Close()
				}
				t.Fatalf("Fetch accepted unsafe filename %q", filename)
			}
		})
	}
}

func TestGenericURLFetchProcessesArchivesAndResolvesMembers(t *testing.T) {
	filename := genericArtifactName("", ".zip")
	archive := genericZip(t, map[string][]byte{
		"bin/alpha" + genericScriptExtension(): genericRunnablePayload(t),
		"bin/beta" + genericScriptExtension():  genericRunnablePayload(t),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	p := newGenericProvider(t, server.URL)
	file, err := p.Fetch(&FetchOpts{AutoSelect: filename + ":bin/beta" + genericScriptExtension(), NonInteractive: true})
	if err != nil {
		t.Fatalf("Fetch explicit member: %v", err)
	}
	explicitData := file.Data
	t.Cleanup(func() {
		if err := closeFileData(explicitData); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if file.Name != "beta"+genericScriptExtension() || file.PackagePath != "bin/beta"+genericScriptExtension() || file.SourceAsset != filename {
		t.Fatalf("explicit archive resolution = %#v", file)
	}

	file, err = p.Fetch(&FetchOpts{NonInteractive: true, SelectionIntent: &config.SelectionDescriptor{ArchiveMember: "bin/alpha" + genericScriptExtension()}})
	if err != nil {
		t.Fatalf("Fetch persisted member: %v", err)
	}
	persistedData := file.Data
	t.Cleanup(func() {
		if err := closeFileData(persistedData); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if file.Name != "alpha"+genericScriptExtension() || file.SelectionIntent == nil || file.SelectionIntent.ArchiveMember != "bin/alpha"+genericScriptExtension() {
		t.Fatalf("persisted archive resolution = %#v", file)
	}
}

func TestGenericURLFetchRecordsArtifactEvidence(t *testing.T) {
	filename := genericArtifactName("", ".zip")
	archive := genericZip(t, map[string][]byte{
		"bin/tool" + genericScriptExtension(): genericRunnablePayload(t),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	file, err := newGenericProvider(t, server.URL).Fetch(&FetchOpts{NonInteractive: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	data := file.Data
	t.Cleanup(func() {
		if err := closeFileData(data); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})

	if file.Evidence == nil {
		t.Fatal("File.Evidence = nil, want recorded decisions")
	}
	if file.Evidence.Release.Selected != filename {
		t.Fatalf("release selected = %q, want %q", file.Evidence.Release.Selected, filename)
	}
	if file.Evidence.Archive.Selected != file.PackagePath {
		t.Fatalf("archive selected = %q, want package path %q", file.Evidence.Archive.Selected, file.PackagePath)
	}
	if file.Evidence.Archive.Selected == "" || file.Evidence.Archive.Reason != "" {
		t.Fatalf("archive evidence = %#v", file.Evidence.Archive)
	}
	if len(file.Evidence.Transformations) != 1 || file.Evidence.Transformations[0] != "zip" {
		t.Fatalf("transformations = %#v, want [zip]", file.Evidence.Transformations)
	}
	if file.Evidence.Integrity.UnchangedBytes {
		t.Fatal("archive fetch reported unchanged bytes")
	}
	if file.Evidence.Integrity.DownloadSHA256 == "" || file.Evidence.Integrity.InstalledSHA256 == "" {
		t.Fatalf("missing integrity digests: %#v", file.Evidence.Integrity)
	}
}

func TestGenericURLFetchCarriesRequestedBundledCompletion(t *testing.T) {
	filename := genericArtifactName("", ".zip")
	command := "tool" + genericScriptExtension()
	completionName := "share/completions/" + command + ".bash"
	archive := genericZip(t, map[string][]byte{
		"bin/" + command: genericRunnablePayload(t),
		completionName:   []byte("complete bundled\n"),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	file, err := newGenericProvider(t, server.URL).Fetch(&FetchOpts{
		NonInteractive:           true,
		BundledCompletionShell:   "bash",
		BundledCompletionCommand: command,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	data := file.Data
	t.Cleanup(func() {
		if err := closeFileData(data); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if got := string(file.BundledCompletion); got != "complete bundled\n" {
		t.Fatalf("BundledCompletion = %q", got)
	}
	if file.BundledCompletionName != completionName {
		t.Fatalf("BundledCompletionName = %q, want %q", file.BundledCompletionName, completionName)
	}
}

func TestGenericURLFetchProcessesTarAndRejectsNonRunnablePayload(t *testing.T) {
	t.Run("tar", func(t *testing.T) {
		filename := genericArtifactName("", ".tar")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			_, _ = w.Write(genericTar(t, "tool"+genericScriptExtension(), genericRunnablePayload(t)))
		}))
		defer server.Close()

		file, err := newGenericProvider(t, server.URL).Fetch(&FetchOpts{NonInteractive: true})
		if err != nil {
			t.Fatalf("Fetch tar: %v", err)
		}
		tarData := file.Data
		t.Cleanup(func() {
			if err := closeFileData(tarData); err != nil {
				t.Errorf("close fetched file: %v", err)
			}
		})
		if file.Name != "tool"+genericScriptExtension() || file.PackagePath != "tool"+genericScriptExtension() {
			t.Fatalf("tar resolution = %#v", file)
		}
	})

	t.Run("non runnable", func(t *testing.T) {
		filename := genericArtifactName("", "")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			_, _ = io.WriteString(w, "not executable")
		}))
		defer server.Close()

		_, err := newGenericProvider(t, server.URL).Fetch(&FetchOpts{NonInteractive: true})
		if !errors.Is(err, assets.ErrNoCompatibleFiles) {
			t.Fatalf("Fetch non-runnable error = %v", err)
		}
	})
}

func TestGenericURLFetchUsesResponseMetadataAndOneDownload(t *testing.T) {
	filename := genericArtifactName("content", "")
	var getCalls, sidecarCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %s, want GET", r.Method)
		}
		getCalls++
		http.Redirect(w, r, "/redirected/"+genericArtifactName("redirect", ""), http.StatusFound)
	})
	mux.HandleFunc("/redirected/", func(w http.ResponseWriter, r *http.Request) {
		getCalls++
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(genericRunnablePayload(t))
	})
	mux.HandleFunc("/download.sha256", func(w http.ResponseWriter, r *http.Request) { sidecarCalls++ })
	server := httptest.NewServer(mux)
	defer server.Close()

	file, err := newGenericProvider(t, server.URL+"/download").Fetch(&FetchOpts{NonInteractive: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	fileData := file.Data
	t.Cleanup(func() {
		if err := closeFileData(fileData); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if getCalls != 2 || sidecarCalls != 0 || file.SourceAsset != filename || file.Name != "content"+genericScriptExtension() {
		t.Fatalf("GETs=%d sidecars=%d file=%#v", getCalls, sidecarCalls, file)
	}
}

func TestGenericURLFetchDoesNotForwardBasicAuthAcrossRedirect(t *testing.T) {
	filename := genericArtifactName("redirected", "")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("redirected request Authorization = %q, want empty", authorization)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(genericRunnablePayload(t))
	}))
	defer target.Close()

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	targetURL.Host = "localhost:" + targetURL.Port()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "user" || password != "pass" {
			t.Fatalf("source request basic auth = %q:%q present=%t", user, password, ok)
		}
		http.Redirect(w, r, targetURL.String(), http.StatusFound)
	}))
	defer source.Close()

	sourceURL, err := url.Parse(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	sourceURL.User = url.UserPassword("user", "pass")
	file, err := newGenericProvider(t, sourceURL.String()).Fetch(&FetchOpts{NonInteractive: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	fileData := file.Data
	t.Cleanup(func() {
		if err := closeFileData(fileData); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if file.SourceAsset != filename {
		t.Fatalf("SourceAsset = %q, want %q", file.SourceAsset, filename)
	}
}

func TestGenericURLFetchFilenamePrecedence(t *testing.T) {
	contentName := genericArtifactName("content", "")
	redirectName := genericArtifactName("redirect", "")
	originalName := genericArtifactName("original", "")

	for _, test := range []struct {
		name       string
		redirect   bool
		content    string
		wantSource string
	}{
		{name: "content disposition", redirect: true, content: contentName, wantSource: contentName},
		{name: "redirect URL", redirect: true, wantSource: redirectName},
		{name: "original URL", wantSource: originalName},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/"+originalName && test.redirect {
					http.Redirect(w, r, "/"+redirectName, http.StatusFound)
					return
				}
				if test.content != "" {
					w.Header().Set("Content-Disposition", `attachment; filename="`+test.content+`"`)
				}
				_, _ = w.Write(genericRunnablePayload(t))
			}))
			defer server.Close()

			file, err := newGenericProvider(t, server.URL+"/"+originalName).Fetch(&FetchOpts{NonInteractive: true})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			fileData := file.Data
			t.Cleanup(func() {
				if err := closeFileData(fileData); err != nil {
					t.Errorf("close fetched file: %v", err)
				}
			})
			if file.SourceAsset != test.wantSource {
				t.Fatalf("SourceAsset = %q, want %q", file.SourceAsset, test.wantSource)
			}
		})
	}
}

func TestGenericURLFetchExplicitVersionAndSelectionIntent(t *testing.T) {
	filename := "tool_" + genericOS() + "_" + runtime.GOARCH + genericScriptExtension()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write(genericRunnablePayload(t))
	}))
	defer server.Close()

	file, err := newGenericProvider(t, server.URL).Fetch(&FetchOpts{Version: "9.8.7", PackageName: "tool", NonInteractive: true, SelectionIntent: &config.SelectionDescriptor{LogicalProduct: "tool"}})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	fileData := file.Data
	t.Cleanup(func() {
		if err := closeFileData(fileData); err != nil {
			t.Errorf("close fetched file: %v", err)
		}
	})
	if file.Version != "9.8.7" || file.SelectionIntent == nil || file.SelectionIntent.LogicalProduct != "tool" || file.SelectionIntent.Target == nil {
		t.Fatalf("version or selection intent not propagated: %#v", file)
	}
}

func newGenericProvider(t *testing.T, rawURL string) Provider {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newGenericURL(u)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func genericArtifactName(prefix, extension string) string {
	if prefix == "" {
		prefix = "tool"
	}
	return prefix + "_0.16.0_" + genericOS() + "_" + runtime.GOARCH + extension
}

func genericOS() string {
	if runtime.GOOS == "darwin" {
		return "darwin"
	}
	return runtime.GOOS
}

func genericScriptExtension() string {
	if runtime.GOOS == "windows" {
		return ".cmd"
	}
	return ""
}

func platformFixtureName(unixName, windowsName string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(windowsName, "amd64", runtime.GOARCH)
	}
	name := strings.ReplaceAll(unixName, "linux", runtime.GOOS)
	return strings.ReplaceAll(name, "amd64", runtime.GOARCH)
}

func genericRunnablePayload(t *testing.T) []byte {
	t.Helper()
	if runtime.GOOS == "windows" {
		return []byte("@echo off\r\n")
	}
	return []byte("#!/bin/sh\nexit 0\n")
}

func genericZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, contents := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func genericTar(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractVersionFromFilenamePicksHighest(t *testing.T) {
	got := extractVersionFromFilename("tool_1.2.0_to_1.3.4_darwin_amd64")
	if got != "1.3.4" {
		t.Fatalf("unexpected highest version: %s", got)
	}
}

func TestFilenameFromContentDisposition(t *testing.T) {
	got := filenameFromContentDisposition(`attachment; filename*=UTF-8''tool_1.2.3_linux_amd64.tar.gz`)
	if got != "tool_1.2.3_linux_amd64.tar.gz" {
		t.Fatalf("unexpected filename: %s", got)
	}
}
