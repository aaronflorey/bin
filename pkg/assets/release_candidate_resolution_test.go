package assets

import (
	"errors"
	"testing"
)

func TestResolveReleaseCandidateIsIndependentOfCandidateOrder(t *testing.T) {
	request := ReleaseCandidateResolutionRequest{
		Product:            "agentsview",
		Target:             ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"arm64"}},
		PackagePreferences: []ReleasePackageFormat{"zip", "tar.gz"},
	}
	candidates := []ReleaseCandidate{
		{ID: "helper.zip", Product: "helper", Target: ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"arm64"}}, Format: "zip"},
		{ID: "agentsview.tar.gz", Product: "agentsview", Target: ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"arm64"}}, Format: "tar.gz"},
		{ID: "agentsview.zip", Product: "agentsview", Target: ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"arm64"}}, Format: "zip"},
	}
	for _, ordered := range [][]ReleaseCandidate{candidates, {candidates[2], candidates[0], candidates[1]}} {
		result, err := ResolveReleaseCandidate(ordered, request)
		if err != nil || result.Candidate.ID != "agentsview.zip" {
			t.Fatalf("ResolveReleaseCandidate() = %#v, %v; want agentsview.zip", result, err)
		}
	}
}

func TestResolveReleaseCandidateTargetAndVariantSafety(t *testing.T) {
	request := ReleaseCandidateResolutionRequest{Product: "tool", Target: ReleaseTarget{
		OS: []string{"linux"}, Architecture: []string{"amd64"}, ABI: []string{"glibc"},
	}}
	candidates := []ReleaseCandidate{
		{ID: "darwin", Product: "tool", Target: ReleaseTarget{OS: []string{"darwin"}, Architecture: []string{"amd64"}}},
		{ID: "musl", Product: "tool", Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"x64"}, ABI: []string{"musl"}}},
		{ID: "avx2", Product: "tool", Target: ReleaseTarget{OS: []string{"linux"}, Architecture: []string{"amd64"}, ABI: []string{"gnu"}, CPUVariant: []string{"avx2"}}},
	}
	_, err := ResolveReleaseCandidate(candidates, request)
	assertReleaseCandidateReason(t, err, ErrIncompatibleReleaseTarget, ReleaseCandidateIncompatible)

	result, err := ResolveReleaseCandidate(append(candidates, ReleaseCandidate{ID: "baseline", Product: "tool", Target: ReleaseTarget{OS: []string{"linux", "linux"}, Architecture: []string{"amd64", "x86_64"}, ABI: []string{"glibc", "gnu"}}}), request)
	if err != nil || result.Candidate.ID != "baseline" {
		t.Fatalf("baseline resolution = %#v, %v", result, err)
	}
}

func TestResolveReleaseCandidateVariantAmbiguityRequiresExplicitRule(t *testing.T) {
	candidates := []ReleaseCandidate{
		{ID: "avx2", Product: "tool", Target: ReleaseTarget{CPUVariant: []string{"avx2"}}},
		{ID: "avx512", Product: "tool", Target: ReleaseTarget{CPUVariant: []string{"avx512"}}},
	}
	request := ReleaseCandidateResolutionRequest{Product: "tool", Target: ReleaseTarget{CPUVariant: []string{"avx512"}}, SafeCPUVariantRules: []CPUVariantRule{{Requested: "avx512", Candidate: "avx2"}}}
	_, err := ResolveReleaseCandidate(candidates, request)
	assertReleaseCandidateReason(t, err, ErrAmbiguousReleaseVariant, ReleaseCandidateAmbiguousVariant)
}

func TestResolveReleaseCandidateAppImageImpliedLinuxTarget(t *testing.T) {
	candidate := ReleaseCandidate{ID: "tool.AppImage", Product: "tool", Format: ReleasePackageFormatAppImage}
	for _, os := range []string{"darwin", "windows"} {
		_, err := ResolveReleaseCandidate([]ReleaseCandidate{candidate}, ReleaseCandidateResolutionRequest{Product: "tool", Target: ReleaseTarget{OS: []string{os}}})
		assertReleaseCandidateReason(t, err, ErrIncompatibleReleaseTarget, ReleaseCandidateIncompatible)
	}
	_, err := ResolveReleaseCandidate([]ReleaseCandidate{{ID: "bad.AppImage", Product: "tool", Format: ReleasePackageFormatAppImage, Target: ReleaseTarget{OS: []string{"darwin"}}}}, ReleaseCandidateResolutionRequest{Product: "tool", Target: ReleaseTarget{OS: []string{"linux"}}})
	assertReleaseCandidateReason(t, err, ErrIncompatibleReleaseTarget, ReleaseCandidateIncompatible)
}

func TestResolveReleaseCandidateDoesNotMutateImpliedTarget(t *testing.T) {
	impliedOS := make([]string, 1, 2)
	impliedOS[0] = "darwin"
	impliedOS = impliedOS[:2]
	impliedOS[1] = "unchanged"
	impliedOS = impliedOS[:1]
	candidate := ReleaseCandidate{
		ID:            "tool.AppImage",
		Product:       "tool",
		Format:        ReleasePackageFormatAppImage,
		ImpliedTarget: ReleaseTarget{OS: impliedOS},
	}

	_, err := ResolveReleaseCandidate([]ReleaseCandidate{candidate}, ReleaseCandidateResolutionRequest{Product: "tool", Target: ReleaseTarget{OS: []string{"linux"}}})
	assertReleaseCandidateReason(t, err, ErrIncompatibleReleaseTarget, ReleaseCandidateIncompatible)
	if impliedOS[:2][1] != "unchanged" {
		t.Fatalf("ResolveReleaseCandidate mutated caller target backing storage: %q", impliedOS[:2])
	}
}

func TestResolveReleaseCandidateDeduplicatesFormatPreferences(t *testing.T) {
	candidates := []ReleaseCandidate{
		{ID: "archive", Product: "tool", Format: "tar.gz"},
		{ID: "zip", Product: "tool", Format: "zip"},
	}
	result, err := ResolveReleaseCandidate(candidates, ReleaseCandidateResolutionRequest{
		Product:            "tool",
		PackagePreferences: []ReleasePackageFormat{"zip", "ZIP", "tar.gz"},
	})
	if err != nil || result.Candidate.ID != "zip" {
		t.Fatalf("deduplicated format preference = %#v, %v", result, err)
	}
}

func TestResolveReleaseCandidateStableFailureReasons(t *testing.T) {
	_, err := ResolveReleaseCandidate(nil, ReleaseCandidateResolutionRequest{})
	assertReleaseCandidateReason(t, err, ErrNoEligibleReleaseCandidate, ReleaseCandidateNoEligible)

	_, err = ResolveReleaseCandidate([]ReleaseCandidate{{ID: "tool", Product: "tool"}, {ID: "helper", Product: "helper"}}, ReleaseCandidateResolutionRequest{})
	assertReleaseCandidateReason(t, err, ErrAmbiguousReleaseProduct, ReleaseCandidateAmbiguousProduct)

	_, err = ResolveReleaseCandidate([]ReleaseCandidate{{ID: "tool", Product: "tool"}}, ReleaseCandidateResolutionRequest{ExplicitSelection: true})
	assertReleaseCandidateReason(t, err, ErrInvalidReleaseCandidateSelection, ReleaseCandidateInvalidSelection)

	_, err = ResolveReleaseCandidate([]ReleaseCandidate{{ID: "tool", Product: "tool"}}, ReleaseCandidateResolutionRequest{Product: "other", ExplicitID: "tool"})
	assertReleaseCandidateReason(t, err, ErrInvalidReleaseCandidateSelection, ReleaseCandidateInvalidSelection)
}

func assertReleaseCandidateReason(t *testing.T, err, want error, reason ReleaseCandidateResolutionReason) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	var resolutionErr *ReleaseCandidateResolutionError
	if !errors.As(err, &resolutionErr) || resolutionErr.Reason != reason {
		t.Fatalf("resolution error = %#v, want reason %q", resolutionErr, reason)
	}
}
