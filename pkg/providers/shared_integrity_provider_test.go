package providers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.gitea.io/sdk/gitea"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

func TestGitLabFetchPreservesApplicableSidecarFailure(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/v1"):
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1", "assets": map[string]any{"links": []map[string]string{
				{"name": "tool", "url": server.URL + "/asset"},
				{"name": "tool.sha256", "url": server.URL + "/sidecar"},
			}}})
		case strings.HasSuffix(r.URL.Path, "/projects/acme/tool"):
			_ = json.NewEncoder(w).Encode(map[string]string{"visibility": "public"})
		case strings.HasSuffix(r.URL.Path, "/packages"):
			_ = json.NewEncoder(w).Encode([]any{})
		case r.URL.Path == "/sidecar":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case r.URL.Path == "/asset":
			_, _ = fmt.Fprint(w, "payload")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(server.URL+"/api/v4/"), gitlab.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&gitLab{client: client, owner: "acme", repo: "tool", tag: "v1"}).Fetch(&FetchOpts{AutoSelect: "tool"})
	assertChecksumFailure(t, err, checksumRetrievalFailure)
}

func TestCodebergFetchRetainsScopedIntegrityRecords(t *testing.T) {
	payload := []byte("payload")
	digest := sha256.Sum256(payload)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.20.0"})
		case strings.Contains(r.URL.Path, "/releases/tags/v1"):
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1", "assets": []map[string]string{
				{"name": "tool", "browser_download_url": server.URL + "/asset"},
				{"name": "tool.sha256", "browser_download_url": server.URL + "/sidecar"},
			}})
		case r.URL.Path == "/asset":
			_, _ = w.Write(payload)
		case r.URL.Path == "/sidecar":
			_, _ = fmt.Fprint(w, fmtDigest(digest))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := gitea.NewClient(server.URL+"/", gitea.SetHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	file, err := (&codeberg{client: client, owner: "acme", repo: "tool", tag: "v1"}).Fetch(&FetchOpts{AutoSelect: "tool"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer file.Data.(interface{ Close() error }).Close()
	if file.DownloadIntegrity == nil || file.DownloadIntegrity.Scope != "download" || file.InstalledIntegrity == nil || file.InstalledIntegrity.Scope != "installed" {
		t.Fatalf("scoped integrity records = download=%#v installed=%#v", file.DownloadIntegrity, file.InstalledIntegrity)
	}
}

func TestHashiCorpFetchPreservesApplicableSidecarParseFailure(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tool/1.0/index.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "tool", "version": "1.0", "builds": []map[string]string{
				{"filename": "tool_linux_amd64", "url": server.URL + "/asset"},
				{"filename": "tool_linux_amd64.sha256", "url": server.URL + "/sidecar"},
			}})
		case "/sidecar":
			_, _ = fmt.Fprint(w, "not a checksum")
		case "/asset":
			_, _ = fmt.Fprint(w, "payload")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&hashiCorp{client: server.Client(), repo: "tool", tag: "1.0", baseURL: baseURL}).Fetch(&FetchOpts{AutoSelect: "tool_linux_amd64"})
	assertChecksumFailure(t, err, checksumParsingFailure)
}

func assertChecksumFailure(t *testing.T, err error, want checksumFailureReason) {
	t.Helper()
	var integrityErr *checksumIntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.State != checksumFailed || integrityErr.Reason != want {
		t.Fatalf("integrity error = %v, want failed %s", err, want)
	}
}
