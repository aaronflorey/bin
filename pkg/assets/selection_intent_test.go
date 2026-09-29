package assets

import (
	"errors"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestResolvePersistedSelectionSurvivesVersionedAssetNames(t *testing.T) {
	descriptor := &config.SelectionDescriptor{
		LogicalProduct: "tool",
		Target:         &config.SelectionTarget{OS: "linux", Architecture: "amd64", ABI: "musl"},
	}
	request := ReleaseCandidateResolutionRequest{Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}, ABI: []string{"musl"}}}
	for _, name := range []string{"tool-v1.2.3-linux-amd64-musl.tar.gz", "tool-v2.0.0-linux-amd64-musl.tar.gz"} {
		candidate := describeReleaseCandidate(&Asset{Name: name}, "tool")
		resolved, err := ResolvePersistedSelection([]ReleaseCandidate{candidate}, request, descriptor)
		if err != nil || resolved.Candidate.ID != name {
			t.Fatalf("ResolvePersistedSelection(%q) = %#v, %v", name, resolved, err)
		}
	}
}

func TestResolvePersistedSelectionAllowsWorktrunkMuslBuildOnGlibc(t *testing.T) {
	name := "worktrunk-x86_64-unknown-linux-musl.tar.xz"
	candidate := describeReleaseCandidate(&Asset{Name: name}, "worktrunk")
	descriptor := &config.SelectionDescriptor{
		LogicalProduct: "worktrunk",
		Target:         &config.SelectionTarget{OS: "linux", Architecture: "amd64", ABI: "musl"},
		ArchiveMember:  "worktrunk-x86_64-unknown-linux-musl/wt",
	}
	request := ReleaseCandidateResolutionRequest{Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}, ABI: []string{"glibc"}}}
	resolved, err := ResolvePersistedSelection([]ReleaseCandidate{candidate}, request, descriptor)
	if err != nil || resolved.Candidate.ID != name {
		t.Fatalf("Worktrunk persisted selection = %#v, %v", resolved, err)
	}
}

func TestResolvePersistedSelectionRejectsMissingTargetAndVariant(t *testing.T) {
	candidates := []ReleaseCandidate{
		{ID: "tool-linux-arm64", Product: "tool", Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"arm64"}}},
		{ID: "tool-linux-amd64-avx2", Product: "tool", Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}, CPUVariant: []string{"avx2"}}},
	}
	for _, test := range []struct {
		name       string
		descriptor *config.SelectionDescriptor
		want       PersistedSelectionReason
	}{
		{"target", &config.SelectionDescriptor{LogicalProduct: "tool", Target: &config.SelectionTarget{OS: "linux", Architecture: "386"}}, PersistedSelectionTarget},
		{"variant", &config.SelectionDescriptor{LogicalProduct: "tool", Target: &config.SelectionTarget{OS: "linux", Architecture: "amd64", CPUVariant: "avx512"}}, PersistedSelectionVariant},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolvePersistedSelection(candidates, ReleaseCandidateResolutionRequest{}, test.descriptor)
			assertPersistedSelectionReason(t, err, test.want)
		})
	}
}

func TestResolveReleaseArchiveMemberMatchesVersionedWrapperForPersistedMember(t *testing.T) {
	f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{ArchiveMember: "tool-v1/bin/tool"}})
	entry, err := f.resolveReleaseArchiveMember(resolverInventory(t, "tool-v2/bin/tool"))
	if err != nil || entry.identity != "tool-v2/bin/tool" {
		t.Fatalf("resolve persisted version wrapper = %v, %v", entry, err)
	}

	_, err = f.resolveReleaseArchiveMember(resolverInventory(t, "tool-v2/bin/other"))
	assertPersistedSelectionReason(t, err, PersistedSelectionMember)
}

func TestResolveReleaseArchiveMemberKeepsInternalDirectoriesDistinct(t *testing.T) {
	f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{ArchiveMember: "tool-v1/commands/tool"}})
	_, err := f.resolveReleaseArchiveMember(resolverInventory(t, "tool-v2/bin/tool"))
	assertPersistedSelectionReason(t, err, PersistedSelectionMember)
}

func TestDeriveSelectionDescriptorUsesOnlySupportedFacts(t *testing.T) {
	descriptor := DeriveSelectionDescriptor(&config.Binary{
		RemoteName:  "unrelated-local-name",
		SourceAsset: "tool-v2.0.0-linux-amd64-musl.tar.gz",
		PackagePath: "tool-v2/bin/tool",
	})
	if descriptor == nil || descriptor.LogicalProduct != "tool" || descriptor.Target == nil ||
		descriptor.Target.OS != "linux" || descriptor.Target.Architecture != "amd64" || descriptor.Target.ABI != "musl" ||
		descriptor.ArchiveMember != "bin/tool" {
		t.Fatalf("derived descriptor = %#v", descriptor)
	}
	if descriptor := DeriveSelectionDescriptor(&config.Binary{RemoteName: "tool"}); descriptor != nil {
		t.Fatalf("derived unsupported descriptor = %#v", descriptor)
	}
}

func TestResolveReleaseArchiveMemberMatchesRealWorldVersionedWrapper(t *testing.T) {
	for _, test := range []struct {
		persisted string
		entry     string
	}{
		{"gum_2.0.1_Darwin_arm64/gum", "gum_2.0.2_Darwin_arm64/gum"},
		{"gum", "gum_2.0.2_Darwin_arm64/gum"},
		{"jsonschema-16.10.0-darwin-arm64/bin/jsonschema", "jsonschema-17.0.0-darwin-arm64/bin/jsonschema"},
		{"bin/jsonschema", "jsonschema-17.0.0-darwin-arm64/bin/jsonschema"},
		{"llmfit-v1.1.15-aarch64-apple-darwin/llmfit", "llmfit-v1.1.16-aarch64-apple-darwin/llmfit"},
		{"mago-1.49.0-aarch64-apple-darwin/mago", "mago-1.50.0-aarch64-apple-darwin/mago"},
		{"druk/1.31.0/bin/druk", "druk/1.36.0/bin/druk"},
	} {
		f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{ArchiveMember: test.persisted}})
		entry, err := f.resolveReleaseArchiveMember(resolverInventory(t, test.entry))
		if err != nil || entry.identity != test.entry {
			t.Fatalf("resolve persisted %q against %q = %v, %v", test.persisted, test.entry, entry, err)
		}
	}
}

func TestResolvePersistedSelectionPrefersArchiveForPersistedMember(t *testing.T) {
	originalResolver := resolver
	resolver = testDarwinARMResolver
	t.Cleanup(func() { resolver = originalResolver })
	descriptor := &config.SelectionDescriptor{
		LogicalProduct: "mise",
		Target:         &config.SelectionTarget{OS: "darwin", Architecture: "arm64"},
		ArchiveMember:  "mise/bin/mise",
	}
	request := ReleaseCandidateResolutionRequest{Target: ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"arm64"}}}

	candidates := []ReleaseCandidate{
		describeReleaseCandidate(&Asset{Name: "mise-v2026.9.16-macos-arm64"}, "mise"),
		describeReleaseCandidate(&Asset{Name: "mise-v2026.9.16-macos-arm64.tar.gz"}, "mise"),
	}
	resolved, err := ResolvePersistedSelection(candidates, request, descriptor)
	if err != nil {
		t.Fatalf("ResolvePersistedSelection = %v", err)
	}
	if resolved.Candidate.ID != "mise-v2026.9.16-macos-arm64.tar.gz" {
		t.Fatalf("selected %q, want the tar.gz archive", resolved.Candidate.ID)
	}
	filter := NewFilter(&FilterOpts{NonInteractive: true, SelectionIntent: descriptor})
	selected, err := filter.FilterAssets("mise", []*Asset{
		{Name: "mise-v2026.9.16-macos-arm64"},
		{Name: "mise-v2026.9.16-macos-arm64.tar.gz"},
	}, "")
	if err != nil || selected.Name != "mise-v2026.9.16-macos-arm64.tar.gz" {
		t.Fatalf("FilterAssets selected %#v, %v", selected, err)
	}

	resolved, err = ResolvePersistedSelection([]ReleaseCandidate{
		describeReleaseCandidate(&Asset{Name: "mise-v2026.9.16-macos-arm64"}, "mise"),
	}, request, descriptor)
	if err != nil || resolved.Candidate.ID != "mise-v2026.9.16-macos-arm64" {
		t.Fatalf("opaque/raw fallback = %#v, %v", resolved, err)
	}

	extensionlessDescriptor := &config.SelectionDescriptor{LogicalProduct: "mise", ArchiveMember: "mise/bin/mise"}
	resolved, err = ResolvePersistedSelection([]ReleaseCandidate{
		describeReleaseCandidate(&Asset{Name: "mise"}, "mise"),
	}, request, extensionlessDescriptor)
	if err != nil || resolved.Candidate.ID != "mise" {
		t.Fatalf("extensionless candidate = %#v, %v", resolved, err)
	}
}

func TestPersistedMemberPreferenceDoesNotTreatDMGOrZstandardAsMemberArchives(t *testing.T) {
	dmg := describeReleaseCandidate(&Asset{Name: "tool-macos-arm64.dmg"}, "tool")
	resolved, err := ResolvePersistedSelection([]ReleaseCandidate{dmg}, ReleaseCandidateResolutionRequest{Target: ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"arm64"}}}, &config.SelectionDescriptor{LogicalProduct: "tool", Target: &config.SelectionTarget{OS: "darwin", Architecture: "arm64"}})
	if err != nil || resolved.Candidate.ID != dmg.ID {
		t.Fatalf("DMG resolution = %#v, %v", resolved, err)
	}

	candidates := []ReleaseCandidate{
		describeReleaseCandidate(&Asset{Name: "tool-linux-amd64"}, "tool"),
		describeReleaseCandidate(&Asset{Name: "tool-linux-amd64.tar.zst"}, "tool"),
	}
	resolved, err = ResolvePersistedSelection(candidates, ReleaseCandidateResolutionRequest{Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}}, PackagePreferences: []ReleasePackageFormat{"standalone"}}, &config.SelectionDescriptor{LogicalProduct: "tool", ArchiveMember: "bin/tool"})
	if err != nil || resolved.Candidate.ID != "tool-linux-amd64" {
		t.Fatalf("zstandard preference = %#v, %v", resolved, err)
	}
}

func TestStoredSelectionDescriptorRepairsOnlyPoisonedRawMember(t *testing.T) {
	original := &config.SelectionDescriptor{LogicalProduct: "tool", Target: &config.SelectionTarget{OS: "linux"}, ArchiveMember: "tool-linux-amd64"}
	repaired := StoredSelectionDescriptor(&config.Binary{InstallMode: "binary", SourceAsset: "tool-linux-amd64", SelectionIntent: original})
	if repaired == original || repaired.ArchiveMember != "" || repaired.LogicalProduct != "tool" || repaired.Target == nil || repaired.Target.OS != "linux" {
		t.Fatalf("repaired descriptor = %#v", repaired)
	}
	if original.ArchiveMember != "tool-linux-amd64" {
		t.Fatalf("input descriptor was mutated: %#v", original)
	}

	for _, binary := range []*config.Binary{
		{InstallMode: "binary", SourceAsset: "tool-linux-amd64", PackagePath: "bin/tool", SelectionIntent: original},
		{InstallMode: "binary", SourceAsset: "tool.zip", SelectionIntent: original},
		{InstallMode: "system-package", SourceAsset: "tool-linux-amd64", SelectionIntent: original},
		{InstallMode: "system-package", PackageType: "dmg", SourceAsset: "tool.dmg", SelectionIntent: &config.SelectionDescriptor{LogicalProduct: "tool"}},
	} {
		got := StoredSelectionDescriptor(binary)
		if got.ArchiveMember != binary.SelectionIntent.ArchiveMember || got.LogicalProduct != binary.SelectionIntent.LogicalProduct {
			t.Fatalf("unrelated descriptor changed: got %#v, want %#v", got, binary.SelectionIntent)
		}
	}
}

func TestResolvePersistedArchiveMemberRejectsNormalizedCollisions(t *testing.T) {
	orders := [][]string{
		{"tool-v2.0-linux-amd64/bin/tool", "tool-v2.1-linux-amd64/bin/tool"},
		{"tool-v2.1-linux-amd64/bin/tool", "tool-v2.0-linux-amd64/bin/tool"},
	}
	for _, stored := range []string{"tool-v1.0-linux-amd64/bin/tool", "bin/tool"} {
		for _, names := range orders {
			f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{ArchiveMember: stored}})
			_, err := f.resolveReleaseArchiveMember(resolverInventory(t, names...))
			if !errors.Is(err, ErrUnavailablePersistedSelection) || !errors.Is(err, ErrAmbiguousArchiveMember) {
				t.Fatalf("resolve %q against %v error = %v", stored, names, err)
			}
			var resolutionErr *ArchiveMemberResolutionError
			if !errors.As(err, &resolutionErr) || len(resolutionErr.Candidates) != 2 || resolutionErr.Candidates[0] != "tool-v2.0-linux-amd64/bin/tool" {
				t.Fatalf("ambiguity = %#v", resolutionErr)
			}
		}
	}

	f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{ArchiveMember: "tool-v2.1-linux-amd64/bin/tool"}})
	entry, err := f.resolveReleaseArchiveMember(resolverInventory(t, orders[0]...))
	if err != nil || entry.identity != "tool-v2.1-linux-amd64/bin/tool" {
		t.Fatalf("exact match = %#v, %v", entry, err)
	}
}

func TestResolveReleaseArchiveMemberRejectsExplicitPersistedConflict(t *testing.T) {
	f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{ArchiveMember: "bin/tool"}})
	f.containedFile, f.containedFileSelected = "bin/other", true
	_, err := f.resolveReleaseArchiveMember(resolverInventory(t, "bin/tool", "bin/other"))
	assertArchiveResolutionReason(t, err, ErrInvalidArchiveMemberSelection, ArchiveMemberInvalidSelection)
}

func assertPersistedSelectionReason(t *testing.T, err error, want PersistedSelectionReason) {
	t.Helper()
	if !errors.Is(err, ErrUnavailablePersistedSelection) {
		t.Fatalf("error = %v, want unavailable persisted selection", err)
	}
	var selectionErr *PersistedSelectionError
	if !errors.As(err, &selectionErr) || selectionErr.Reason != want {
		t.Fatalf("selection error = %#v, want reason %q", selectionErr, want)
	}
}
