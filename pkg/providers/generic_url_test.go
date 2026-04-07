package providers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
	payload := "binary-bytes"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="tool_0.16.0_linux_amd64"`)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(payload))
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

	if file.Name != "tool" {
		t.Fatalf("unexpected name: %s", file.Name)
	}
	if file.Version != "0.16.0" {
		t.Fatalf("unexpected version: %s", file.Version)
	}

	content, err := io.ReadAll(file.Data)
	if err != nil {
		t.Fatalf("read file content failed: %v", err)
	}
	if string(content) != payload {
		t.Fatalf("unexpected payload: %q", string(content))
	}
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
