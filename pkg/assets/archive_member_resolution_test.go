package assets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveArchiveMemberDeterministicIdentityOrder(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	for _, names := range [][]string{
		{"helpers/tool", "tool", "tool-v2/bin/tool", "tool-v2/tool"},
		{"tool-v2/tool", "tool-v2/bin/tool", "tool", "helpers/tool"},
	} {
		inventory := resolverInventory(t, names...)
		entry, err := resolveArchiveMember(inventory, archiveMemberResolutionRequest{
			packagePath: "tool-v1/bin/tool",
			logicalName: "tool",
		})
		if err != nil {
			t.Fatal(err)
		}
		if entry.identity != "tool" {
			t.Fatalf("resolved %q, want exact logical identity tool", entry.identity)
		}

		entry, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{
			packagePath: "tool-v1/bin/tool",
			logicalName: "missing",
		})
		if err != nil {
			t.Fatal(err)
		}
		if entry.identity != "tool-v2/bin/tool" {
			t.Fatalf("resolved %q, want narrowly normalized wrapper identity", entry.identity)
		}

		entry, err = resolveArchiveMember(resolverInventory(t, "helpers-2/tool", "tool-v2/bin/tool"), archiveMemberResolutionRequest{
			packagePath: "tool-v1/bin/tool",
			logicalName: "missing",
		})
		if err != nil || entry.identity != "tool-v2/bin/tool" {
			t.Fatalf("resolve with helper wrapper = %v, %v; want tool-v2/bin/tool", entry, err)
		}
	}
}

func TestResolveArchiveMemberBasenameAndDepthSafety(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	t.Run("unique basename fallback", func(t *testing.T) {
		entry, err := resolveArchiveMember(resolverInventory(t, "bin/tool", "helpers/install"), archiveMemberResolutionRequest{logicalName: "tool"})
		if err != nil || entry.identity != "bin/tool" {
			t.Fatalf("resolve = %v, %v; want bin/tool", entry, err)
		}
	})

	t.Run("duplicate basename is ambiguous", func(t *testing.T) {
		_, err := resolveArchiveMember(resolverInventory(t, "helpers/tool", "bin/tool"), archiveMemberResolutionRequest{logicalName: "tool"})
		assertArchiveResolutionReason(t, err, ErrAmbiguousArchiveMember, ArchiveMemberAmbiguous)
		var resolutionErr *ArchiveMemberResolutionError
		if !errors.As(err, &resolutionErr) || strings.Join(resolutionErr.Candidates, ",") != "bin/tool,helpers/tool" {
			t.Fatalf("candidates = %#v, want sorted archive members", resolutionErr)
		}
	})

	t.Run("depth cannot displace stored identity", func(t *testing.T) {
		entry, err := resolveArchiveMember(resolverInventory(t, "tool", "deep/helpers/tool"), archiveMemberResolutionRequest{packagePath: "deep/helpers/tool", logicalName: "tool"})
		if err != nil || entry.identity != "deep/helpers/tool" {
			t.Fatalf("resolve = %v, %v; want stored deep identity", entry, err)
		}
	})

	t.Run("nested archive scope remains identity-bearing", func(t *testing.T) {
		_, err := resolveArchiveMember(resolverInventory(t, "release-v2.zip!/tool", "other-v2.zip!/tool"), archiveMemberResolutionRequest{logicalName: "tool"})
		assertArchiveResolutionReason(t, err, ErrAmbiguousArchiveMember, ArchiveMemberAmbiguous)
	})
}

func TestResolveArchiveMemberExplicitSelectionValidation(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	inventory := resolverInventory(t, "tool", "tool-windows", "not-runnable")
	inventory.entries[0].class = artifactEntryCompletion
	inventory.entries[1].targetCompatible = false
	inventory.entries[2].runnable = false

	entry, err := resolveArchiveMember(resolverInventory(t, "bin/tool"), archiveMemberResolutionRequest{explicit: `bin\tool`})
	if err != nil || entry.identity != "bin/tool" {
		t.Fatalf("explicit normalized selection = %v, %v", entry, err)
	}

	_, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{explicit: "../tool"})
	assertArchiveResolutionReason(t, err, ErrInvalidArchiveMemberSelection, ArchiveMemberInvalidSelection)

	_, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{explicit: "missing"})
	assertArchiveResolutionReason(t, err, ErrInvalidArchiveMemberSelection, ArchiveMemberInvalidSelection)

	_, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{explicit: "tool"})
	assertArchiveResolutionReason(t, err, ErrIncompatibleArchiveMemberSelection, ArchiveMemberIncompatibleSelection)

	_, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{explicit: "tool-windows"})
	assertArchiveResolutionReason(t, err, ErrIncompatibleArchiveMemberSelection, ArchiveMemberIncompatibleSelection)

	_, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{explicit: "not-runnable"})
	assertArchiveResolutionReason(t, err, ErrIncompatibleArchiveMemberSelection, ArchiveMemberIncompatibleSelection)

	_, err = resolveArchiveMember(inventory, archiveMemberResolutionRequest{explicitSelection: true})
	assertArchiveResolutionReason(t, err, ErrInvalidArchiveMemberSelection, ArchiveMemberInvalidSelection)
}

func TestResolveArchiveMemberNoEligible(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	inventory := resolverInventory(t, "tool")
	inventory.entries[0].class = artifactEntryIgnored
	_, err := resolveArchiveMember(inventory, archiveMemberResolutionRequest{logicalName: "tool"})
	assertArchiveResolutionReason(t, err, ErrNoEligibleArchiveMember, ArchiveMemberNoEligible)
}

func TestResolveArchiveMemberNilInventory(t *testing.T) {
	_, err := resolveArchiveMember(nil, archiveMemberResolutionRequest{logicalName: "tool"})
	assertArchiveResolutionReason(t, err, ErrNoEligibleArchiveMember, ArchiveMemberNoEligible)
}

func TestResolveArchiveMemberUsesRecordedEligibility(t *testing.T) {
	originalResolver := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = originalResolver })

	inventory := resolverInventory(t, "tool")
	if err := os.Remove(inventory.entries[0].stagedPath); err != nil {
		t.Fatal(err)
	}
	entry, err := resolveArchiveMember(inventory, archiveMemberResolutionRequest{logicalName: "tool"})
	if err != nil || entry.identity != "tool" {
		t.Fatalf("resolve after staged file removal = %v, %v", entry, err)
	}
}

func resolverInventory(t *testing.T, names ...string) *artifactInventory {
	t.Helper()
	inventory := newArtifactInventory()
	for _, name := range names {
		if _, err := inventory.add(name, artifactEntryExecutable); err != nil {
			t.Fatal(err)
		}
		stagedPath := filepath.Join(t.TempDir(), filepath.Base(name))
		if err := os.WriteFile(stagedPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		inventory.entries[len(inventory.entries)-1].stagedPath = stagedPath
		inventory.entries[len(inventory.entries)-1].targetCompatible = true
		inventory.entries[len(inventory.entries)-1].runnable = true
	}
	return inventory
}

func assertArchiveResolutionReason(t *testing.T, err, want error, reason ArchiveMemberResolutionReason) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	var resolutionErr *ArchiveMemberResolutionError
	if !errors.As(err, &resolutionErr) || resolutionErr.Reason != reason {
		t.Fatalf("resolution error = %#v, want reason %q", resolutionErr, reason)
	}
}
