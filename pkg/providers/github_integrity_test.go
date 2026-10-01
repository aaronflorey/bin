package providers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
)

func TestGitHubFetchDigestSourcePriorityAndFailures(t *testing.T) {
	assetName := "tool" + genericScriptExtension()
	payload := genericRunnablePayload(t)
	payloadDigest := sha256.Sum256(payload)
	correctDigest := "sha256:" + fmtDigest(payloadDigest)
	manifestDigest := strings.Repeat("a", 64)

	tests := []struct {
		name              string
		digest            string
		includeSidecar    bool
		sidecarDigest     string
		wantFailureReason checksumFailureReason
		wantSidecarCalls  int
	}{
		{name: "missing digest uses applicable manifest", includeSidecar: true, sidecarDigest: fmtDigest(payloadDigest), wantSidecarCalls: 1},
		{name: "supported digest takes priority", digest: correctDigest, includeSidecar: true, sidecarDigest: manifestDigest},
		{name: "malformed digest fails", digest: "sha256:not-hex", includeSidecar: true, sidecarDigest: fmtDigest(payloadDigest), wantFailureReason: checksumParsingFailure},
		{name: "unsupported digest fails", digest: "sha512:" + strings.Repeat("b", 128), includeSidecar: true, sidecarDigest: fmtDigest(payloadDigest), wantFailureReason: checksumUnsupportedAlgorithmFailure},
		{name: "authoritative mismatch does not fall back", digest: "sha256:" + strings.Repeat("c", 64), includeSidecar: true, sidecarDigest: fmtDigest(payloadDigest), wantFailureReason: checksumMismatchFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGitHubReleaseCache(t)
			var sidecarCalls int
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/tool/releases/latest":
					assets := []map[string]string{{"name": assetName, "url": server.URL + "/asset", "digest": tt.digest}}
					if tt.includeSidecar {
						assets = append(assets, map[string]string{"name": assetName + ".sha256", "url": server.URL + "/sidecar"})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.3", "assets": assets})
				case "/asset":
					_, _ = w.Write(payload)
				case "/sidecar":
					sidecarCalls++
					_, _ = w.Write([]byte(tt.sidecarDigest))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			file, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "").Fetch(&FetchOpts{AutoSelect: assetName})
			if tt.wantFailureReason == "" {
				if err != nil {
					t.Fatalf("Fetch returned error: %v", err)
				}
				if file == nil {
					t.Fatal("Fetch returned no file")
				}
				if file.DownloadIntegrity == nil || file.DownloadIntegrity.Algorithm != "sha256" || file.DownloadIntegrity.Expected == "" || file.DownloadIntegrity.Observed == "" || file.DownloadIntegrity.Source == "" || file.DownloadIntegrity.Scope != "download" || file.DownloadIntegrity.Result != "verified" {
					t.Fatalf("missing verified download integrity record: %#v", file.DownloadIntegrity)
				}
				if file.InstalledIntegrity == nil || file.InstalledIntegrity.Algorithm != "sha256" || file.InstalledIntegrity.Expected == "" || file.InstalledIntegrity.Observed == "" || file.InstalledIntegrity.Source == "" || file.InstalledIntegrity.Scope != "installed" || file.InstalledIntegrity.Result != "verified" {
					t.Fatalf("missing verified installed integrity record: %#v", file.InstalledIntegrity)
				}
				if !file.ProcessingUnchanged {
					t.Fatal("plain payload was not recorded as unchanged")
				}
				if closer, ok := file.Data.(interface{ Close() error }); ok {
					_ = closer.Close()
				}
			} else {
				var integrityErr *checksumIntegrityError
				if !errors.As(err, &integrityErr) || integrityErr.State != checksumFailed || integrityErr.Reason != tt.wantFailureReason {
					t.Fatalf("unexpected integrity error: %v", err)
				}
			}
			if sidecarCalls != tt.wantSidecarCalls {
				t.Fatalf("sidecar requests = %d, want %d", sidecarCalls, tt.wantSidecarCalls)
			}
		})
	}
}

func TestGitHubFetchArchiveDoesNotInheritDownloadIntegrity(t *testing.T) {
	filename := platformFixtureName("tool-linux-amd64.zip", "tool-windows-amd64.zip")
	fixture := "tool" + genericScriptExtension()
	archive := buildProviderZip(t, fixture, string(genericRunnablePayload(t)))
	sum := sha256.Sum256(archive)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/tool/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.3", "assets": []map[string]string{
				{"name": filename, "url": server.URL + "/asset"},
				{"name": filename + ".sha256", "url": server.URL + "/sidecar"},
			}})
		case "/asset":
			_, _ = w.Write(archive)
		case "/sidecar":
			_, _ = fmt.Fprint(w, fmtDigest(sum))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resetGitHubReleaseCache(t)
	file, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "").Fetch(&FetchOpts{AutoSelect: filename})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	defer func() {
		if closer, ok := file.Data.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	if file.DownloadIntegrity == nil || file.DownloadIntegrity.Expected != fmtDigest(sum) || file.DownloadIntegrity.Source != filename+".sha256" {
		t.Fatalf("unexpected download integrity record: %#v", file.DownloadIntegrity)
	}
	if file.InstalledIntegrity != nil || file.ExpectedSHA != "" || file.ProcessingUnchanged {
		t.Fatalf("archive inherited download verification: installed=%#v expected=%q unchanged=%v", file.InstalledIntegrity, file.ExpectedSHA, file.ProcessingUnchanged)
	}
}

func TestGitHubFetchExplicitSelectionValidatesFinalPayload(t *testing.T) {
	resetGitHubReleaseCache(t)
	filename := platformFixtureName("tool-linux-amd64", "tool-windows-amd64"+genericScriptExtension())
	invalidPayload := "not executable"
	if genericScriptExtension() != "" {
		invalidPayload = "#!/bin/sh\nexit 0\n"
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/tool/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.3", "assets": []map[string]string{
				{"name": filename, "url": server.URL + "/asset"},
			}})
		case "/asset":
			_, _ = fmt.Fprint(w, invalidPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "").Fetch(&FetchOpts{AutoSelect: filename})
	if !errors.Is(err, assets.ErrNoCompatibleFiles) {
		t.Fatalf("Fetch() error = %v, want final runnable-payload validation failure", err)
	}
}

func TestGitHubFetchApplicableAndUnrelatedSidecarFailures(t *testing.T) {
	assetName := "tool" + genericScriptExtension()
	payload := genericRunnablePayload(t)

	tests := []struct {
		name              string
		assets            func(serverURL string) []map[string]string
		sidecarStatus     int
		sidecarContents   string
		wantFailureReason checksumFailureReason
		wantFile          bool
	}{
		{
			name: "applicable sidecar HTTP failure",
			assets: func(serverURL string) []map[string]string {
				return []map[string]string{{"name": assetName, "url": serverURL + "/asset"}, {"name": assetName + ".sha256", "url": serverURL + "/sidecar"}}
			},
			sidecarStatus: http.StatusServiceUnavailable, wantFailureReason: checksumRetrievalFailure,
		},
		{
			name: "applicable sidecar parse failure",
			assets: func(serverURL string) []map[string]string {
				return []map[string]string{{"name": assetName, "url": serverURL + "/asset"}, {"name": assetName + ".sha256", "url": serverURL + "/sidecar"}}
			},
			sidecarContents: "not a checksum", wantFailureReason: checksumParsingFailure,
		},
		{
			name: "unrelated failing sidecar is ignored",
			assets: func(serverURL string) []map[string]string {
				return []map[string]string{{"name": assetName, "url": serverURL + "/asset"}, {"name": "other.sha256", "url": serverURL + "/sidecar"}}
			},
			sidecarStatus: http.StatusServiceUnavailable, wantFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGitHubReleaseCache(t)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/tool/releases/latest":
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.3", "assets": tt.assets(server.URL)})
				case "/asset":
					_, _ = w.Write(payload)
				case "/sidecar":
					if tt.sidecarStatus != 0 {
						http.Error(w, "unavailable", tt.sidecarStatus)
						return
					}
					_, _ = fmt.Fprint(w, tt.sidecarContents)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			file, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "").Fetch(&FetchOpts{AutoSelect: assetName})
			if tt.wantFile {
				if err != nil || file == nil {
					t.Fatalf("Fetch = %#v, %v; want file without checksum evidence", file, err)
				}
				if file.DownloadIntegrity != nil || file.InstalledIntegrity != nil {
					t.Fatalf("unrelated sidecar became integrity evidence: %#v", file)
				}
				if closer, ok := file.Data.(interface{ Close() error }); ok {
					_ = closer.Close()
				}
				return
			}
			var integrityErr *checksumIntegrityError
			if !errors.As(err, &integrityErr) || integrityErr.Reason != tt.wantFailureReason {
				t.Fatalf("unexpected integrity error: %v", err)
			}
		})
	}

}

func fmtDigest(sum [sha256.Size]byte) string { return fmt.Sprintf("%x", sum[:]) }

func buildProviderZip(t *testing.T, name, contents string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}
