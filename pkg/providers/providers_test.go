package providers

import (
	"net/url"
	"testing"
)

func TestNewFallsBackToGenericForUnknownHTTPSHost(t *testing.T) {
	p, err := New("https://downloads.example.test/tool", "")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p.GetID() != "generic" {
		t.Fatalf("expected generic provider, got %q", p.GetID())
	}
}

func TestNewForcedGenericProvider(t *testing.T) {
	p, err := New("downloads.example.test/tool", "generic")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p.GetID() != "generic" {
		t.Fatalf("expected generic provider, got %q", p.GetID())
	}
}

func TestNewDoesNotMisclassifySimilarHostnames(t *testing.T) {
	p, err := New("https://notgithub.example.test/acme/tool", "")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p.GetID() != "generic" {
		t.Fatalf("expected generic provider, got %q", p.GetID())
	}
}

func TestNewHashiCorpRejectsMissingRepo(t *testing.T) {
	u, err := url.Parse("https://releases.hashicorp.com/")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	if _, err := newHashiCorp(u); err == nil {
		t.Fatal("expected missing repo error")
	}
}

func TestNewGitHubDownloadURLExtractsTagOnly(t *testing.T) {
	u, err := url.Parse("https://github.com/cli/cli/releases/download/v2.64.0/gh_2.64.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	p, err := newGitHub(u)
	if err != nil {
		t.Fatalf("newGitHub returned error: %v", err)
	}

	gh := p.(*gitHub)
	if gh.tag != "v2.64.0" {
		t.Fatalf("expected tag v2.64.0, got %q", gh.tag)
	}
}

func TestNewCodebergDownloadURLExtractsTagOnly(t *testing.T) {
	u, err := url.Parse("https://codeberg.org/foo/bar/releases/download/v1.2.3/bar_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	p, err := newCodeberg(u)
	if err != nil {
		t.Fatalf("newCodeberg returned error: %v", err)
	}

	cb := p.(*codeberg)
	if cb.tag != "v1.2.3" {
		t.Fatalf("expected tag v1.2.3, got %q", cb.tag)
	}
}
