package providers

import (
	"net/url"
	"os"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestNewGitHubUsesGHTokenWhenEnabled(t *testing.T) {
	t.Setenv("GITHUB_AUTH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GHES_BASE_URL", "")
	t.Setenv("GHES_UPLOAD_URL", "")
	t.Setenv("GHES_AUTH_TOKEN", "")

	prevUseGHAuth := config.Get().UseGHAuth
	config.Get().UseGHAuth = true
	t.Cleanup(func() {
		config.Get().UseGHAuth = prevUseGHAuth
	})

	prevRunGHAuthToken := runGHAuthToken
	runGHAuthToken = func() ([]byte, error) {
		return []byte("gh-token\n"), nil
	}
	t.Cleanup(func() {
		runGHAuthToken = prevRunGHAuthToken
	})

	u, err := url.Parse("https://github.com/cli/cli")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	p, err := newGitHub(u)
	if err != nil {
		t.Fatalf("newGitHub returned error: %v", err)
	}

	gh, ok := p.(*gitHub)
	if !ok {
		t.Fatalf("expected *gitHub provider, got %T", p)
	}
	if gh.token != "gh-token" {
		t.Fatalf("unexpected token: got %q, want %q", gh.token, "gh-token")
	}
}

func TestNewGitHubPrefersEnvTokenOverGH(t *testing.T) {
	t.Setenv("GITHUB_AUTH_TOKEN", "env-token")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GHES_BASE_URL", "")
	t.Setenv("GHES_UPLOAD_URL", "")
	t.Setenv("GHES_AUTH_TOKEN", "")

	prevUseGHAuth := config.Get().UseGHAuth
	config.Get().UseGHAuth = true
	t.Cleanup(func() {
		config.Get().UseGHAuth = prevUseGHAuth
	})

	prevRunGHAuthToken := runGHAuthToken
	called := false
	runGHAuthToken = func() ([]byte, error) {
		called = true
		return []byte("gh-token\n"), nil
	}
	t.Cleanup(func() {
		runGHAuthToken = prevRunGHAuthToken
	})

	u, err := url.Parse("https://github.com/cli/cli")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	p, err := newGitHub(u)
	if err != nil {
		t.Fatalf("newGitHub returned error: %v", err)
	}

	gh, ok := p.(*gitHub)
	if !ok {
		t.Fatalf("expected *gitHub provider, got %T", p)
	}
	if called {
		t.Fatal("expected gh auth token command not to be called when env token is set")
	}
	if gh.token != "env-token" {
		t.Fatalf("unexpected token: got %q, want %q", gh.token, "env-token")
	}
}

func TestNewGitHubIgnoresGHTokenFailure(t *testing.T) {
	t.Setenv("GITHUB_AUTH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GHES_BASE_URL", "")
	t.Setenv("GHES_UPLOAD_URL", "")
	t.Setenv("GHES_AUTH_TOKEN", "")

	prevUseGHAuth := config.Get().UseGHAuth
	config.Get().UseGHAuth = true
	t.Cleanup(func() {
		config.Get().UseGHAuth = prevUseGHAuth
	})

	prevRunGHAuthToken := runGHAuthToken
	runGHAuthToken = func() ([]byte, error) {
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() {
		runGHAuthToken = prevRunGHAuthToken
	})

	u, err := url.Parse("https://github.com/cli/cli")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	p, err := newGitHub(u)
	if err != nil {
		t.Fatalf("newGitHub returned error: %v", err)
	}

	gh, ok := p.(*gitHub)
	if !ok {
		t.Fatalf("expected *gitHub provider, got %T", p)
	}
	if gh.token != "" {
		t.Fatalf("expected empty token when gh auth token lookup fails, got %q", gh.token)
	}
}
