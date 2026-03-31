package cmd

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

type staticProvider struct {
	id      string
	release *providers.ReleaseInfo
	err     error
}

func (s *staticProvider) Fetch(*providers.FetchOpts) (*providers.File, error) {
	return &providers.File{Data: strings.NewReader(""), Name: "tool", Version: "1.0.0"}, nil
}

func (s *staticProvider) GetLatestVersion() (*providers.ReleaseInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.release, nil
}

func (s *staticProvider) Cleanup(*providers.CleanupOpts) error {
	return nil
}

func (s *staticProvider) GetID() string {
	if s.id != "" {
		return s.id
	}
	return "github"
}

func TestCollectAvailableUpdatesContinueOnError(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-a": {Path: "/tmp/tool-a", URL: "success", Version: "1.0.0", Provider: "github"},
		"/tmp/tool-b": {Path: "/tmp/tool-b", URL: "fail", Version: "1.0.0", Provider: "github"},
		"/tmp/tool-c": {Path: "/tmp/tool-c", URL: "pinned", Version: "1.0.0", Provider: "github", Pinned: true},
	}

	newProvider := func(u, _ string) (providers.Provider, error) {
		switch u {
		case "success":
			return &staticProvider{release: &providers.ReleaseInfo{Version: "1.1.0", URL: "https://example.test/tool-a"}}, nil
		case "fail":
			return &staticProvider{err: fmt.Errorf("boom")}, nil
		default:
			return nil, io.EOF
		}
	}

	updates, failures, err := collectAvailableUpdates(bins, newProvider, true, 2)
	if err != nil {
		t.Fatalf("collectAvailableUpdates returned unexpected error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].binary.Path != "/tmp/tool-a" {
		t.Fatalf("unexpected update path: %s", updates[0].binary.Path)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
}

func TestCollectAvailableUpdatesReturnsErrorWithoutContinue(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-a": {Path: "/tmp/tool-a", URL: "fail", Version: "1.0.0", Provider: "github"},
	}

	newProvider := func(_, _ string) (providers.Provider, error) {
		return &staticProvider{err: fmt.Errorf("boom")}, nil
	}

	_, _, err := collectAvailableUpdates(bins, newProvider, false, 0)
	if err == nil {
		t.Fatal("expected collectAvailableUpdates to fail")
	}
}
