package providers

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v73/github"
)

func TestGitHubReusesLatestReleaseForFetch(t *testing.T) {
	resetGitHubReleaseCache(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeTestGitHubRelease(w, "v1.2.3")
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL, "acme", "tool", "token")
	info, err := provider.GetLatestVersion()
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if info.Version != "v1.2.3" {
		t.Fatalf("unexpected latest version: %q", info.Version)
	}

	if _, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "token").Fetch(&FetchOpts{}); err == nil {
		t.Fatal("expected fetch with no release assets to fail")
	}
	if _, err := newTestGitHubProvider(t, server.URL, "acme", "tool", "token").Fetch(&FetchOpts{Version: "v1.2.3"}); err == nil {
		t.Fatal("expected fetch with no release assets to fail")
	}

	if got := requests.Load(); got != 1 {
		t.Fatalf("expected one GitHub metadata request, got %d", got)
	}
}

func TestGitHubReusesListedReleaseForPrefixFetch(t *testing.T) {
	resetGitHubReleaseCache(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/repos/acme/tool/releases" || r.URL.Query().Get("per_page") != "10" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"tag_name":"v1.2.3","html_url":"https://github.com/acme/tool/releases/tag/v1.2.3","assets":[]}]`)
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL, "acme", "tool", "token")
	if _, err := provider.ListReleases(10); err != nil {
		t.Fatalf("ListReleases failed: %v", err)
	}
	if _, err := provider.Fetch(&FetchOpts{ReleaseTagPrefix: "v"}); err == nil {
		t.Fatal("expected fetch with no release assets to fail")
	}

	if got := requests.Load(); got != 1 {
		t.Fatalf("expected one GitHub metadata request, got %d", got)
	}
}

func TestGitHubCoalescesConcurrentLatestReleaseRequests(t *testing.T) {
	resetGitHubReleaseCache(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeTestGitHubRelease(w, "v1.2.3")
	}))
	defer server.Close()

	const callers = 20
	providers := make([]*gitHub, callers)
	for i := range providers {
		providers[i] = newTestGitHubProvider(t, server.URL, "acme", "tool", "token")
	}

	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := provider.GetLatestVersion()
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("GetLatestVersion failed: %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected one coalesced GitHub request, got %d", got)
	}
}

func TestGitHubReleaseCacheSeparatesRepositoriesCredentialsAndAPIs(t *testing.T) {
	var firstRequests, secondRequests atomic.Int32
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstRequests.Add(1)
		writeTestGitHubRelease(w, "v1.2.3")
	}))
	defer firstServer.Close()
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondRequests.Add(1)
		writeTestGitHubRelease(w, "v1.2.3")
	}))
	defer secondServer.Close()

	tests := []struct {
		name   string
		second func(*testing.T) *gitHub
	}{
		{name: "repository", second: func(t *testing.T) *gitHub {
			return newTestGitHubProvider(t, firstServer.URL, "acme", "other", "token-a")
		}},
		{name: "credential", second: func(t *testing.T) *gitHub {
			return newTestGitHubProvider(t, firstServer.URL, "acme", "tool", "token-b")
		}},
		{name: "API base", second: func(t *testing.T) *gitHub {
			return newTestGitHubProvider(t, secondServer.URL, "acme", "tool", "token-a")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGitHubReleaseCache(t)
			firstRequests.Store(0)
			secondRequests.Store(0)
			providers := []*gitHub{
				newTestGitHubProvider(t, firstServer.URL, "acme", "tool", "token-a"),
				tt.second(t),
			}
			for _, provider := range providers {
				if _, err := provider.GetLatestVersion(); err != nil {
					t.Fatalf("GetLatestVersion failed: %v", err)
				}
			}
			if got := firstRequests.Load() + secondRequests.Load(); got != 2 {
				t.Fatalf("expected separate requests, got %d", got)
			}
		})
	}
}

func TestGitHubReleaseCacheRetriesErrors(t *testing.T) {
	resetGitHubReleaseCache(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"message":"temporary failure"}`)
			return
		}
		writeTestGitHubRelease(w, "v1.2.3")
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL, "acme", "tool", "token")
	if _, err := provider.GetLatestVersion(); err == nil {
		t.Fatal("expected first request to fail")
	}
	if _, err := provider.GetLatestVersion(); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}

	if got := requests.Load(); got != 2 {
		t.Fatalf("expected failed response to be retried, got %d requests", got)
	}
}

func resetGitHubReleaseCache(t *testing.T) {
	t.Helper()
	previous := githubReleaseCache
	githubReleaseCache = &sync.Map{}
	t.Cleanup(func() {
		githubReleaseCache = previous
	})
}

func newTestGitHubProvider(t *testing.T, baseURL, owner, repo, token string) *gitHub {
	t.Helper()
	parsedBaseURL, err := url.Parse(baseURL + "/")
	if err != nil {
		t.Fatalf("parse GitHub base URL: %v", err)
	}
	client := github.NewClient(nil)
	client.BaseURL = parsedBaseURL
	return &gitHub{
		client:          client,
		owner:           owner,
		repo:            repo,
		authFingerprint: sha256.Sum256([]byte(token)),
	}
}

func writeTestGitHubRelease(w http.ResponseWriter, tag string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://github.com/acme/tool/releases/tag/%s","assets":[]}`, tag, tag)
}
