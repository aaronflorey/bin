package providers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

func TestAuditSlashReleaseTag(t *testing.T) {
	for _, suffix := range []string{"tag/kustomize/v5.7.1", "tag/kustomize%2Fv5.7.1", "download/kustomize/v5.7.1/kustomize.tar.gz", "download/kustomize%2Fv5.7.1/kustomize.tar.gz"} {
		t.Run(suffix, func(t *testing.T) {
			raw := "https://github.com/kubernetes-sigs/kustomize/releases/" + suffix
			normalized, version, explicit, err := NormalizeGitHubURL(raw, "")
			if err != nil || !explicit || version != "kustomize/v5.7.1" || normalized != "github.com/kubernetes-sigs/kustomize" {
				t.Fatalf("normalize = %q, %q, %t, %v", normalized, version, explicit, err)
			}
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := releaseTagFromSegments(providerPathSegments(u)); got != version {
				t.Fatalf("direct provider tag = %q", got)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/kubernetes-sigs/kustomize/releases/tags/kustomize/v5.7.1" {
					t.Errorf("API path = %q", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": version})
			}))
			defer server.Close()
			p := newTestGitHubProvider(t, server.URL, "kubernetes-sigs", "kustomize", "")
			release, err := p.releaseByTag(version)
			if err != nil || release.GetTagName() != version {
				t.Fatalf("release = %#v, %v", release, err)
			}
		})
	}
}

func TestAuditSHA256sumManifest(t *testing.T) {
	payload := genericRunnablePayload(t)
	name := "jq" + genericScriptExtension()
	sum := sha256.Sum256(payload)
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/jqlang/jq/releases/latest":
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "jq-1.7.1", "assets": []map[string]string{{"name": name, "url": server.URL + "/asset"}, {"name": "sha256sum.txt", "url": server.URL + "/checksum"}, {"name": "sha256sum.txt.gpgsig", "url": server.URL + "/signature"}}})
				case "/asset":
					_, _ = w.Write(payload)
				case "/checksum":
					hash := fmtDigest(sum)
					if corrupt {
						hash = strings.Repeat("0", 64)
					}
					_, _ = fmt.Fprintf(w, "%s  %s\n", hash, name)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			file, err := newTestGitHubProvider(t, server.URL, "jqlang", "jq", "").Fetch(&FetchOpts{NonInteractive: true})
			if corrupt {
				if file != nil {
					_ = closeFileData(file.Data)
				}
				if !errors.Is(err, assets.ErrChecksumMismatch) {
					t.Fatalf("expected checksum mismatch, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := closeFileData(file.Data); err != nil {
					t.Error(err)
				}
			}()
			if file.DownloadIntegrity == nil || file.DownloadIntegrity.Observed != fmtDigest(sum) || file.DownloadIntegrity.Source != "sha256sum.txt" {
				t.Fatalf("integrity = %#v", file.DownloadIntegrity)
			}
		})
	}
}

func TestAuditGitHubVersionedArchiveLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 audit fixture")
	}
	for _, tc := range []struct{ repo, command, old, next string }{
		{"ripgrep", "rg", "ripgrep-14.1.1-x86_64-unknown-linux-musl/rg", "ripgrep-15.1.0-x86_64-unknown-linux-musl/rg"},
		{"neovim", "nvim", "nvim-linux64/bin/nvim", "nvim-linux-x86_64/bin/nvim"},
		{"prometheus", "prometheus", "prometheus-2.47.0.linux-amd64/prometheus", "prometheus-3.5.0.linux-amd64/prometheus"},
	} {
		t.Run(tc.repo, func(t *testing.T) {
			members := map[string]string{"v1.0.0": tc.old, "v2.0.0": tc.next}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tag, ok := strings.CutPrefix(r.URL.Path, "/repos/acme/"+tc.repo+"/releases/tags/"); ok {
					member, found := members[tag]
					if !found {
						http.NotFound(w, r)
						return
					}
					name := strings.SplitN(member, "/", 2)[0] + ".zip"
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "assets": []map[string]string{{"name": name, "url": server.URL + "/assets/" + tag}}})
					return
				}
				if tag, ok := strings.CutPrefix(r.URL.Path, "/assets/"); ok {
					_, _ = w.Write(buildProviderZip(t, members[tag], "#!/bin/sh\nexit 0\n"))
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			fetch := func(opts *FetchOpts) *File {
				t.Helper()
				file, err := newTestGitHubProvider(t, server.URL, "acme", tc.repo, "").Fetch(opts)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.ReadAll(file.Data); err != nil {
					t.Fatal(err)
				}
				if err := closeFileData(file.Data); err != nil {
					t.Fatal(err)
				}
				return file
			}
			installed := fetch(&FetchOpts{Version: "v1.0.0", NonInteractive: true})
			intent := assets.StoredSelectionDescriptor(&config.Binary{SourceAsset: installed.SourceAsset, PackagePath: installed.PackagePath, SelectionIntent: installed.SelectionIntent})
			updated := fetch(&FetchOpts{Version: "v2.0.0", NonInteractive: true, PackageName: tc.command, PackagePath: installed.PackagePath, SelectionIntent: intent})
			restored := fetch(&FetchOpts{Version: "v2.0.0", NonInteractive: true, PackageName: tc.command, PackagePath: updated.PackagePath, SelectionIntent: updated.SelectionIntent})
			if updated.PackagePath != tc.next || restored.PackagePath != tc.next || restored.Name != tc.command {
				t.Fatalf("updated = %#v, restored = %#v", updated, restored)
			}
		})
	}
}
