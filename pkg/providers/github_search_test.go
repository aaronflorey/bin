package providers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v80/github"
)

func TestSearchGitHubRepositoriesSendsQueryAndMapsMetadata(t *testing.T) {
	const query = "terminal file manager language:go stars:>100"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Errorf("request path = %q, want /search/repositories", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != query {
			t.Errorf("query = %q, want %q", got, query)
		}
		if got := r.URL.Query().Get("per_page"); got != "20" {
			t.Errorf("per_page = %q, want 20", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":1,"items":[{"full_name":"acme/termfm","stargazers_count":123,"description":"A terminal file manager"}]}`))
	}))
	defer server.Close()

	repositories, err := searchGitHubRepositories(searchTestClient(t, server.URL), query)
	if err != nil {
		t.Fatalf("searchGitHubRepositories() error = %v", err)
	}
	want := []GitHubRepository{{FullName: "acme/termfm", Stars: 123, Description: "A terminal file manager"}}
	if len(repositories) != len(want) || repositories[0] != want[0] {
		t.Fatalf("repositories = %+v, want %+v", repositories, want)
	}
}

func TestSearchGitHubRepositoriesReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer server.Close()

	_, err := searchGitHubRepositories(searchTestClient(t, server.URL), "invalid query")
	if err == nil {
		t.Fatal("expected GitHub API error")
	}
	var apiError *github.ErrorResponse
	if !errors.As(err, &apiError) || !strings.Contains(err.Error(), "Validation Failed") {
		t.Fatalf("search error = %v, want wrapped GitHub API error", err)
	}
}

func searchTestClient(t *testing.T, baseURL string) *github.Client {
	t.Helper()

	parsedBaseURL, err := url.Parse(baseURL + "/")
	if err != nil {
		t.Fatalf("parse GitHub API base URL: %v", err)
	}
	client := github.NewClient(nil)
	client.BaseURL = parsedBaseURL
	return client
}

func TestSearchGitHubRepositoriesReturnsEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "items": []any{}}); err != nil {
			t.Errorf("encode empty search result: %v", err)
		}
	}))
	defer server.Close()

	repositories, err := searchGitHubRepositories(searchTestClient(t, server.URL), "no matches")
	if err != nil {
		t.Fatalf("searchGitHubRepositories() error = %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("repositories = %+v, want empty result", repositories)
	}
}
