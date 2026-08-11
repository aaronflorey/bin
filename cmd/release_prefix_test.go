package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func TestDiscoverInstallableReleasePrefixes(t *testing.T) {
	newProvider := func(_, _ string) (providers.Provider, error) {
		return &staticProvider{
			history: []*providers.ReleaseInfo{
				{Version: "v2.0.0", Assets: []string{"tool.tar.gz"}},
				{Version: "v2.18.0-eafb0ec6-nightly", Assets: []string{"tool.tar.gz"}},
				{Version: "nightly", Assets: []string{"tool.tar.gz"}},
				{Version: "pi-v1.1.0", Assets: []string{"tool-pi.tar.gz"}},
				{Version: "pi-v1.0.0", Assets: []string{"tool-pi.tar.gz"}},
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
	want := []string{"v", "nightly", "pi-v"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected prefixes: got %v want %v", got, want)
	}
}

func TestSelectReleaseTagPrefixesFailsNonInteractiveAmbiguity(t *testing.T) {
	_, err := selectReleaseTagPrefixesInteractively([]releasePrefixOption{
		{Prefix: "v", LatestTag: "v1.0.0"},
		{Prefix: "nightly", LatestTag: "v1.1.0-abcd123-nightly"},
	}, true)
	if err == nil {
		t.Fatal("expected ambiguous release lanes to fail")
	}
	if !strings.Contains(err.Error(), "multiple release lanes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseFetchPrefixPreservesBareStableLane(t *testing.T) {
	if got := releaseFetchPrefix(""); got != providers.BareReleaseTagPrefix {
		t.Fatalf("unexpected bare release prefix: %q", got)
	}
	if got := releaseFetchPrefix("v"); got != "v" {
		t.Fatalf("unexpected v release prefix: %q", got)
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

func TestExistingBinaryForInstallMatchesEffectiveStoredReleaseLane(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-nightly": {
			Path:             "/tmp/tool-nightly",
			URL:              "github.com/acme/tool",
			Version:          "v2.18.0-a734383d-nightly",
			ReleaseTagPrefix: "v",
		},
	}

	matched := existingBinaryForInstall(bins, "github.com/acme/tool", "", "", "nightly")
	if matched == nil || matched.Path != "/tmp/tool-nightly" {
		t.Fatalf("unexpected match: %+v", matched)
	}
}

func TestExistingBinaryForInstallMatchesEffectiveStoredReleaseLaneWithEmptyLegacyPrefix(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-rc": {
			Path:             "/tmp/tool-rc",
			URL:              "github.com/acme/tool",
			Version:          "1.2.3-rc.1",
			ReleaseTagPrefix: "",
		},
	}

	matched := existingBinaryForInstall(bins, "github.com/acme/tool", "", "", "rc")
	if matched == nil || matched.Path != "/tmp/tool-rc" {
		t.Fatalf("unexpected match: %+v", matched)
	}
}

func TestExistingBinaryForInstallKeepsPrefixedPrereleaseOnOwnLane(t *testing.T) {
	bins := map[string]*config.Binary{
		"/tmp/tool-bare-rc": {
			Path:             "/tmp/tool-bare-rc",
			URL:              "github.com/acme/tool",
			Version:          "1.2.3-rc.1",
			ReleaseTagPrefix: "",
		},
		"/tmp/tool-v-rc": {
			Path:             "/tmp/tool-v-rc",
			URL:              "github.com/acme/tool",
			Version:          "v1.2.3-rc.1",
			ReleaseTagPrefix: "v-rc",
		},
		"/tmp/tool-pi-v-rc": {
			Path:             "/tmp/tool-pi-v-rc",
			URL:              "github.com/acme/tool",
			Version:          "pi-v1.2.3-rc.1",
			ReleaseTagPrefix: "pi-v",
		},
	}

	matched := existingBinaryForInstall(bins, "github.com/acme/tool", "", "", "pi-v-rc")
	if matched == nil || matched.Path != "/tmp/tool-pi-v-rc" {
		t.Fatalf("unexpected match: %+v", matched)
	}

	if got := existingBinaryForInstall(bins, "github.com/acme/tool", "", "", "v-rc"); got == nil || got.Path != "/tmp/tool-v-rc" {
		t.Fatalf("unexpected v-rc match: %+v", got)
	}
	if got := existingBinaryForInstall(bins, "github.com/acme/tool", "", "", "rc"); got == nil || got.Path != "/tmp/tool-bare-rc" {
		t.Fatalf("unexpected rc match: %+v", got)
	}
}
