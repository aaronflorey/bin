package providers

import (
	"strings"
	"testing"
	"time"
)

func TestParseImage(t *testing.T) {
	cases := []struct {
		name                      string
		imageURL                  string
		expectedRepo, expectedTag string
		withErr                   bool
	}{
		{name: "no host, no version", imageURL: "postgres", expectedRepo: "library/postgres", expectedTag: "latest"},
		{name: "no host, with version", imageURL: "postgres:1.2.3", expectedRepo: "library/postgres", expectedTag: "1.2.3"},
		{name: "with host, no version", imageURL: "quay.io/calico/node", expectedRepo: "quay.io/calico/node", expectedTag: "latest"},
		{name: "with host, with version", imageURL: "quay.io/calico/node:1.2.3", expectedRepo: "quay.io/calico/node", expectedTag: "1.2.3"},
		{name: "no host, with version and owner", imageURL: "hashicorp/terraform:1.2.3", expectedRepo: "hashicorp/terraform", expectedTag: "1.2.3"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			repo, tag := parseImage(test.imageURL)
			switch {
			case test.expectedRepo != repo:
				t.Errorf("expected repo was %s, got %s", test.expectedRepo, repo)
			case test.expectedTag != tag:
				t.Errorf("expected tag was %s, got %s", test.expectedTag, tag)
			}
		})
	}
}

func TestDockerHubRepoPath(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		expected string
		wantErr  bool
	}{
		{name: "docker hub shorthand", repo: "library/postgres", expected: "library/postgres"},
		{name: "docker hub explicit host", repo: "docker.io/library/postgres", expected: "library/postgres"},
		{name: "unsupported host", repo: "quay.io/calico/node", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := dockerHubRepoPath(tc.repo)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.repo)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if repo != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, repo)
			}
		})
	}
}

func TestLatestDockerTag(t *testing.T) {
	now := time.Now()
	tags := []dockerHubTag{
		{Name: "1.0.0", LastUpdated: now.Add(-48 * time.Hour)},
		{Name: "1.2.0", LastUpdated: now.Add(-24 * time.Hour)},
		{Name: "latest", LastUpdated: now.Add(-12 * time.Hour)},
	}

	version, publishedAt := latestDockerTag("1.1.0", tags)
	if version != "1.2.0" {
		t.Fatalf("expected 1.2.0, got %q", version)
	}
	if publishedAt == nil {
		t.Fatal("expected publishedAt to be set")
	}
}

func TestLatestDockerTagFallsBackToLatest(t *testing.T) {
	now := time.Now()
	tags := []dockerHubTag{
		{Name: "latest", LastUpdated: now.Add(-2 * time.Hour)},
		{Name: "edge", LastUpdated: now.Add(-1 * time.Hour)},
	}

	version, _ := latestDockerTag("dev", tags)
	if version != "latest" {
		t.Fatalf("expected latest, got %q", version)
	}
}

func TestDockerWrapperScriptTemplateOverride(t *testing.T) {
	t.Setenv("BIN_DOCKER_RUN_TEMPLATE", "docker run --network host %s:%s")
	script := dockerWrapperScript("acme/tool", "1.2.3")
	if script != "docker run --network host acme/tool:1.2.3" {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestDockerWrapperScriptDefaultTemplate(t *testing.T) {
	t.Setenv("BIN_DOCKER_RUN_TEMPLATE", "")
	script := dockerWrapperScript("acme/tool", "1.2.3")
	if !strings.Contains(script, "acme/tool:1.2.3") {
		t.Fatalf("expected default template to include image tag, got %q", script)
	}
}
