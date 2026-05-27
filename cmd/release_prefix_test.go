package cmd

import (
	"reflect"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func TestDiscoverInstallableReleasePrefixes(t *testing.T) {
	newProvider := func(_, _ string) (providers.Provider, error) {
		return &staticProvider{
			history: []*providers.ReleaseInfo{
				{Version: "v2.0.0", Assets: []string{"tool-linux-amd64.tar.gz"}},
				{Version: "pi-v1.1.0", Assets: []string{"tool-pi-linux-arm64.tar.gz"}},
				{Version: "pi-v1.0.0", Assets: []string{"tool-pi-linux-arm64.tar.gz"}},
			},
		}, nil
	}

	options, err := discoverInstallableReleasePrefixes(newProvider, "github.com/acme/tool", "github", providers.FetchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := []string{}
	for _, option := range options {
		got = append(got, option.Prefix)
	}
	want := []string{"v", "pi-v"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected prefixes: got %v want %v", got, want)
	}
}

func TestExistingBinaryForInstallMatchesReleaseTagPrefix(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool":    {Path: "/tmp/tool", URL: "github.com/acme/tool", ReleaseTagPrefix: "v"},
		"/tmp/tool-pi": {Path: "/tmp/tool-pi", URL: "github.com/acme/tool", ReleaseTagPrefix: "pi-v"},
	}

	matched := existingBinaryForInstall(bins, "github.com/acme/tool", "", "", "pi-v")
	if matched == nil || matched.Path != "/tmp/tool-pi" {
		t.Fatalf("unexpected match: %+v", matched)
	}
}
