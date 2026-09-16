package assets

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReleaseEvidenceMatchesResolution(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	gf, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-linux-amd64.tar.gz"},
		{Name: "tool-darwin-arm64.tar.gz"},
		{Name: "helper-linux-amd64.zip"},
	}, "")
	if err != nil {
		t.Fatalf("FilterAssets() error = %v", err)
	}

	evidence := f.Evidence()
	if evidence == nil {
		t.Fatal("Evidence() = nil, want recorded decisions")
	}
	if evidence.Release.Selected != gf.Name || evidence.Release.Selected != "tool-linux-amd64.tar.gz" {
		t.Fatalf("selected = %q, want %q", evidence.Release.Selected, gf.Name)
	}
	if evidence.Release.Reason != "" || evidence.Release.PersistedReason != "" {
		t.Fatalf("unresolved reasons on successful resolution: %#v", evidence.Release)
	}
	if evidence.Integrity.DownloadSHA256 != "" || evidence.Integrity.InstalledSHA256 != "" {
		t.Fatalf("resolution populated processing integrity: %#v", evidence.Integrity)
	}

	byID := map[string]ReleaseCandidateDecision{}
	for _, decision := range evidence.Release.Candidates {
		byID[decision.Candidate.ID] = decision
	}
	if len(byID) != 3 {
		t.Fatalf("candidate decisions = %d, want 3", len(byID))
	}
	assertCandidateOutcome(t, byID["tool-linux-amd64.tar.gz"], true, true)
	assertCandidateOutcome(t, byID["tool-darwin-arm64.tar.gz"], false, false)
	assertCandidateOutcome(t, byID["helper-linux-amd64.zip"], false, false)

	selected := byID["tool-linux-amd64.tar.gz"].Candidate
	if selected.Format != "tar.gz" {
		t.Fatalf("selected format = %q, want tar.gz", selected.Format)
	}
	if len(selected.Target.Architecture) != 1 || selected.Target.Architecture[0] != "amd64" {
		t.Fatalf("selected target = %#v, want amd64", selected.Target)
	}
}

func TestReleaseEvidenceReportsTypedAmbiguity(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.FilterAssets("unknown", []*Asset{
		{Name: "alpha-linux-amd64"},
		{Name: "beta-linux-amd64"},
	}, "")
	if !errors.Is(err, ErrAmbiguousReleaseProduct) {
		t.Fatalf("FilterAssets() error = %v, want ambiguous product", err)
	}

	evidence := f.Evidence()
	if evidence == nil {
		t.Fatal("Evidence() = nil, want recorded failure")
	}
	if evidence.Release.Reason != ReleaseCandidateAmbiguousProduct {
		t.Fatalf("release reason = %q, want %q", evidence.Release.Reason, ReleaseCandidateAmbiguousProduct)
	}
	if evidence.Release.Selected != "" {
		t.Fatalf("ambiguous resolution selected %q", evidence.Release.Selected)
	}
	assertStringSlice(t, evidence.Release.Ambiguous, []string{"alpha", "beta"})
	for _, decision := range evidence.Release.Candidates {
		if decision.Eligible || decision.Selected {
			t.Fatalf("ambiguous candidate marked resolved: %#v", decision)
		}
	}
}

func TestArchiveEvidenceRecordsInventoryAndSelection(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	archive := buildTestZipArchive(t, map[string]string{
		"bin/tool":               "#!/bin/sh\nexit 0\n",
		"share/completions/tool": "complete -W tool tool\n",
		"README.md":              "docs\n",
	})

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	result, err := f.ProcessReader("tool-linux-amd64.zip", int64(len(archive)), bytes.NewReader(archive), "", false)
	if err != nil {
		t.Fatalf("ProcessReader() error = %v", err)
	}
	defer func() {
		if closer, ok := result.Source.(io.Closer); ok {
			_ = closer.Close()
		}
	}()

	evidence := f.Evidence()
	if evidence == nil {
		t.Fatal("Evidence() = nil, want recorded decisions")
	}
	if evidence.Archive.Selected != "bin/tool" {
		t.Fatalf("archive selected = %q, want bin/tool", evidence.Archive.Selected)
	}
	if evidence.Archive.Selected != result.PackagePath {
		t.Fatalf("evidence selected %q does not match processed package path %q", evidence.Archive.Selected, result.PackagePath)
	}
	if evidence.Archive.Reason != "" || len(evidence.Archive.Candidates) != 0 {
		t.Fatalf("unresolved archive reasons on success: %#v", evidence.Archive)
	}

	entries := map[string]ArchiveMemberDecision{}
	for _, entry := range evidence.Archive.Entries {
		entries[entry.Identity] = entry
	}
	assertArchiveEntry(t, entries["bin/tool"], "executable", true, true)
	assertArchiveEntry(t, entries["share/completions/tool"], "completion", false, false)
	assertArchiveEntry(t, entries["README.md"], "ignored", false, false)

	assertStringSlice(t, evidence.Transformations, []string{"zip"})
	if evidence.Integrity.UnchangedBytes {
		t.Fatal("archive processing reported unchanged bytes")
	}
	if evidence.Integrity.DownloadSHA256 == "" || evidence.Integrity.InstalledSHA256 == "" {
		t.Fatalf("missing integrity digests: %#v", evidence.Integrity)
	}
	if evidence.Integrity.DownloadSHA256 == evidence.Integrity.InstalledSHA256 {
		t.Fatalf("transformed artifact reported identical digests: %#v", evidence.Integrity)
	}
}

func TestArchiveEvidenceReportsTypedAmbiguity(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	archive := buildTestZipArchive(t, map[string]string{
		"a/tool": "#!/bin/sh\nexit 0\n",
		"b/tool": "#!/bin/sh\nexit 0\n",
	})

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessReader("tool-linux-amd64.zip", int64(len(archive)), bytes.NewReader(archive), "", false)
	if !errors.Is(err, ErrAmbiguousArchiveMember) {
		t.Fatalf("ProcessReader() error = %v, want ambiguous archive member", err)
	}

	evidence := f.Evidence()
	if evidence == nil {
		t.Fatal("Evidence() = nil, want recorded failure")
	}
	if evidence.Archive.Reason != ArchiveMemberAmbiguous {
		t.Fatalf("archive reason = %q, want %q", evidence.Archive.Reason, ArchiveMemberAmbiguous)
	}
	if evidence.Archive.Selected != "" {
		t.Fatalf("ambiguous archive resolution selected %q", evidence.Archive.Selected)
	}
	assertStringSlice(t, evidence.Archive.Candidates, []string{"a/tool", "b/tool"})
	if len(evidence.Archive.Entries) != 2 {
		t.Fatalf("archive entries = %d, want 2", len(evidence.Archive.Entries))
	}
}

func TestEvidenceIsDetachedFromFilter(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	if _, err := f.FilterAssets("tool", []*Asset{{Name: "tool-linux-amd64"}}, ""); err != nil {
		t.Fatalf("FilterAssets() error = %v", err)
	}

	first := f.Evidence()
	first.Release.Selected = "mutated"
	first.Release.Candidates[0].Candidate.ID = "mutated"
	first.Transformations = append(first.Transformations, "mutated")

	second := f.Evidence()
	if second.Release.Selected != "tool-linux-amd64" {
		t.Fatalf("selected leaked mutation: %q", second.Release.Selected)
	}
	if second.Release.Candidates[0].Candidate.ID != "tool-linux-amd64" {
		t.Fatalf("candidate leaked mutation: %q", second.Release.Candidates[0].Candidate.ID)
	}
	if len(second.Transformations) != 0 {
		t.Fatalf("transformations leaked mutation: %#v", second.Transformations)
	}
}

func TestEvidenceFromErrorReportsFilterFailure(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.FilterAssets("unknown", []*Asset{
		{Name: "beta-linux-amd64"},
		{Name: "alpha-linux-amd64"},
	}, "")
	if err == nil {
		t.Fatal("FilterAssets() error = nil, want ambiguity")
	}

	evidence := EvidenceFromError(err)
	if evidence == nil {
		t.Fatal("EvidenceFromError() = nil, want recorded failure")
	}
	if evidence.Release.Reason != ReleaseCandidateAmbiguousProduct {
		t.Fatalf("release reason = %q, want %q", evidence.Release.Reason, ReleaseCandidateAmbiguousProduct)
	}
	assertStringSlice(t, evidence.Release.Ambiguous, []string{"alpha", "beta"})
	if len(evidence.Release.Candidates) != 2 {
		t.Fatalf("candidate decisions = %d, want described candidates preserved", len(evidence.Release.Candidates))
	}
}

func TestEvidenceFromErrorReportsArchiveInventory(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	archive := buildTestZipArchive(t, map[string]string{
		"a/tool": "#!/bin/sh\nexit 0\n",
		"b/tool": "#!/bin/sh\nexit 0\n",
	})
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessReader("tool-linux-amd64.zip", int64(len(archive)), bytes.NewReader(archive), "", false)
	if err == nil {
		t.Fatal("ProcessReader() error = nil, want ambiguity")
	}

	evidence := EvidenceFromError(err)
	if evidence == nil {
		t.Fatal("EvidenceFromError() = nil, want recorded failure")
	}
	if evidence.Archive.Reason != ArchiveMemberAmbiguous {
		t.Fatalf("archive reason = %q, want %q", evidence.Archive.Reason, ArchiveMemberAmbiguous)
	}
	if len(evidence.Archive.Entries) != 2 {
		t.Fatalf("archive entries = %d, want recorded inventory", len(evidence.Archive.Entries))
	}
	assertStringSlice(t, evidence.Archive.Candidates, []string{"a/tool", "b/tool"})
}

func TestEvidenceFromErrorFallsBackToTypedReasons(t *testing.T) {
	t.Run("release", func(t *testing.T) {
		evidence := EvidenceFromError(&ReleaseCandidateResolutionError{
			Reason:     ReleaseCandidateAmbiguousVariant,
			Candidates: []string{"beta", "alpha"},
		})
		if evidence == nil || evidence.Release.Reason != ReleaseCandidateAmbiguousVariant {
			t.Fatalf("release evidence = %#v", evidence)
		}
		assertStringSlice(t, evidence.Release.Ambiguous, []string{"alpha", "beta"})
	})

	t.Run("persisted", func(t *testing.T) {
		evidence := EvidenceFromError(&PersistedSelectionError{Reason: PersistedSelectionMember, Selection: "bin/tool"})
		if evidence == nil || evidence.Release.PersistedReason != PersistedSelectionMember {
			t.Fatalf("persisted evidence = %#v", evidence)
		}
		assertStringSlice(t, evidence.Release.Ambiguous, []string{"bin/tool"})
	})

	t.Run("archive", func(t *testing.T) {
		evidence := EvidenceFromError(&ArchiveMemberResolutionError{
			Reason:     ArchiveMemberNoEligible,
			Candidates: []string{"bin/tool"},
		})
		if evidence == nil || evidence.Archive.Reason != ArchiveMemberNoEligible {
			t.Fatalf("archive evidence = %#v", evidence)
		}
	})

	t.Run("untyped", func(t *testing.T) {
		if evidence := EvidenceFromError(errors.New("plain failure")); evidence != nil {
			t.Fatalf("EvidenceFromError() = %#v, want nil", evidence)
		}
		if evidence := EvidenceFromError(nil); evidence != nil {
			t.Fatalf("EvidenceFromError(nil) = %#v, want nil", evidence)
		}
	})
}

func assertCandidateOutcome(t *testing.T, decision ReleaseCandidateDecision, eligible, selected bool) {
	t.Helper()
	if decision.Eligible != eligible || decision.Selected != selected {
		t.Fatalf("%s outcome = eligible:%t selected:%t, want eligible:%t selected:%t", decision.Candidate.ID, decision.Eligible, decision.Selected, eligible, selected)
	}
}

func assertArchiveEntry(t *testing.T, entry ArchiveMemberDecision, class string, eligible, selected bool) {
	t.Helper()
	if entry.Class != class || entry.Eligible != eligible || entry.Selected != selected {
		t.Fatalf("archive entry outcome = class:%q eligible:%t selected:%t, want class:%q eligible:%t selected:%t", entry.Class, entry.Eligible, entry.Selected, class, eligible, selected)
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("string slice = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("string slice = %#v, want %#v", got, want)
		}
	}
}
