package providers

import "testing"

func TestReleaseTagPrefix_ClassifiesStableAndPrereleaseLanes(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{name: "stable with v prefix", tag: "v1.2.3", want: "v"},
		{name: "stable without prefix", tag: "1.2.3", want: ""},
		{name: "stable with stored compatibility prefix", tag: "pi-v1.1.0", want: "pi-v"},
		{name: "alpha prerelease lane", tag: "v2.22.0-alpha.1", want: "alpha"},
		{name: "rc prerelease lane", tag: "v2.22.0-rc.1", want: "rc"},
		{name: "prefixed rc prerelease keeps namespace", tag: "pi-v1.2.3-rc.1", want: "pi-v-rc"},
		{name: "nightly prerelease lane", tag: "v2.18.0-eafb0ec6-nightly", want: "nightly"},
		{name: "rolling nightly tag joins nightly lane", tag: "nightly", want: "nightly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReleaseTagPrefix(tt.tag); got != tt.want {
				t.Fatalf("ReleaseTagPrefix(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestReleaseTagPrefix_PersistsProviderMetadataForPrereleases(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "alpha release persists alpha lane", version: "v2.22.0-alpha.1", want: "alpha"},
		{name: "rc release persists rc lane", version: "v2.22.0-rc.1", want: "rc"},
		{name: "prefixed rc release persists namespaced lane", version: "pi-v1.2.3-rc.1", want: "pi-v-rc"},
		{name: "nightly release persists nightly lane", version: "v2.18.0-eafb0ec6-nightly", want: "nightly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &File{
				Version:          tt.version,
				ReleaseTagPrefix: ReleaseTagPrefix(tt.version),
			}

			if file.ReleaseTagPrefix != tt.want {
				t.Fatalf("persisted ReleaseTagPrefix for %q = %q, want %q", tt.version, file.ReleaseTagPrefix, tt.want)
			}
		})
	}
}

func TestFetchedReleaseTagPrefixPreservesExplicitBareLane(t *testing.T) {
	if got := fetchedReleaseTagPrefix("1.2.3", BareReleaseTagPrefix); got != BareReleaseTagPrefix {
		t.Fatalf("unexpected fetched bare prefix: %q", got)
	}
	if got := fetchedReleaseTagPrefix("v1.2.3", "v"); got != "v" {
		t.Fatalf("unexpected fetched v prefix: %q", got)
	}
}

func TestMatchesReleaseTagPrefix_LegacyAndLaneAwareCompatibility(t *testing.T) {
	tests := []struct {
		name   string
		tag    string
		prefix string
		want   bool
	}{
		{name: "legacy stable v prefix still matches stable", tag: "v1.2.3", prefix: "v", want: true},
		{name: "legacy empty prefix still matches bare stable", tag: "1.2.3", prefix: "", want: true},
		{name: "explicit bare lane matches bare stable", tag: "1.2.3", prefix: BareReleaseTagPrefix, want: true},
		{name: "explicit bare lane excludes v stable", tag: "v1.2.3", prefix: BareReleaseTagPrefix, want: false},
		{name: "legacy pi-v prefix still matches stable", tag: "pi-v1.1.0", prefix: "pi-v", want: true},
		{name: "stable prefix does not match alpha lane", tag: "v2.22.0-alpha.1", prefix: "v", want: false},
		{name: "alpha lane matches alpha prerelease", tag: "v2.22.0-alpha.1", prefix: "alpha", want: true},
		{name: "legacy stored alpha prefix still matches alpha prerelease", tag: "v2.22.0-alpha.1", prefix: "v-alpha", want: true},
		{name: "rc lane matches rc prerelease", tag: "v2.22.0-rc.1", prefix: "rc", want: true},
		{name: "legacy stored rc prefix still matches rc prerelease", tag: "v2.22.0-rc.1", prefix: "v-rc", want: true},
		{name: "prefixed prerelease matches namespaced lane", tag: "pi-v1.2.3-rc.1", prefix: "pi-v-rc", want: true},
		{name: "prefixed prerelease does not match bare rc lane", tag: "pi-v1.2.3-rc.1", prefix: "rc", want: false},
		{name: "prefixed prerelease does not match v-rc compat lane", tag: "pi-v1.2.3-rc.1", prefix: "v-rc", want: false},
		{name: "nightly lane matches nightly prerelease", tag: "v2.18.0-eafb0ec6-nightly", prefix: "nightly", want: true},
		{name: "legacy stored nightly prefix still matches nightly prerelease", tag: "v2.18.0-eafb0ec6-nightly", prefix: "v-nightly", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesReleaseTagPrefix(tt.tag, tt.prefix); got != tt.want {
				t.Fatalf("MatchesReleaseTagPrefix(%q, %q) = %v, want %v", tt.tag, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestEffectiveReleaseTagPrefix_PreservesStableAndConvertsLegacyPrereleasePrefixes(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		storedPrefix string
		want         string
	}{
		{name: "legacy prerelease v prefix becomes nightly lane", version: "v2.18.0-a734383d-nightly", storedPrefix: "v", want: "nightly"},
		{name: "legacy prerelease empty prefix becomes rc lane", version: "1.2.3-rc.1", storedPrefix: "", want: "rc"},
		{name: "legacy prerelease pi-v prefix becomes namespaced rc lane", version: "pi-v1.2.3-rc.1", storedPrefix: "pi-v", want: "pi-v-rc"},
		{name: "legacy stable empty prefix stays stable", version: "1.2.3", storedPrefix: "", want: ""},
		{name: "legacy stable prefixed release stays unchanged", version: "pi-v1.1.0", storedPrefix: "pi-v", want: "pi-v"},
		{name: "unrelated stored prefix is preserved", version: "v2.18.0-a734383d-nightly", storedPrefix: "custom", want: "custom"},
		{name: "legacy stored prerelease lane prefix is preserved", version: "v2.18.0-a734383d-nightly", storedPrefix: "v-nightly", want: "v-nightly"},
		{name: "namespaced prerelease lane prefix is preserved", version: "pi-v1.2.3-rc.1", storedPrefix: "pi-v-rc", want: "pi-v-rc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveReleaseTagPrefix(tt.version, tt.storedPrefix); got != tt.want {
				t.Fatalf("EffectiveReleaseTagPrefix(%q, %q) = %q, want %q", tt.version, tt.storedPrefix, got, tt.want)
			}
		})
	}
}

func TestSelectReleaseByPrefix_SeparatesStableFromPrereleaseLanes(t *testing.T) {
	releases := []*ReleaseInfo{
		{Version: "1.3.0"},
		{Version: "v2.22.0-alpha.1"},
		{Version: "v2.22.0-rc.1"},
		{Version: "pi-v1.2.3-rc.1"},
		{Version: "v2.18.0-eafb0ec6-nightly"},
		{Version: "v2.21.0"},
		{Version: "pi-v1.1.0"},
	}

	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "legacy stable v prefix selects stable release", prefix: "v", want: "v2.21.0"},
		{name: "explicit bare lane selects bare stable release", prefix: BareReleaseTagPrefix, want: "1.3.0"},
		{name: "alpha lane selects alpha release", prefix: "alpha", want: "v2.22.0-alpha.1"},
		{name: "legacy stored alpha prefix still selects alpha release", prefix: "v-alpha", want: "v2.22.0-alpha.1"},
		{name: "rc lane selects rc release", prefix: "rc", want: "v2.22.0-rc.1"},
		{name: "legacy stored rc prefix still selects rc release", prefix: "v-rc", want: "v2.22.0-rc.1"},
		{name: "namespaced rc lane selects prefixed prerelease", prefix: "pi-v-rc", want: "pi-v1.2.3-rc.1"},
		{name: "nightly lane selects nightly release", prefix: "nightly", want: "v2.18.0-eafb0ec6-nightly"},
		{name: "legacy stored nightly prefix still selects nightly release", prefix: "v-nightly", want: "v2.18.0-eafb0ec6-nightly"},
		{name: "legacy pi-v prefix selects prefixed stable release", prefix: "pi-v", want: "pi-v1.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectReleaseByPrefix(releases, tt.prefix)
			if got == nil {
				t.Fatalf("SelectReleaseByPrefix(..., %q) = nil, want version %q", tt.prefix, tt.want)
			}
			if got.Version != tt.want {
				t.Fatalf("SelectReleaseByPrefix(..., %q) = %q, want %q", tt.prefix, got.Version, tt.want)
			}
		})
	}
}
