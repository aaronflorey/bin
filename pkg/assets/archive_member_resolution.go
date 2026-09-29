package assets

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrNoEligibleArchiveMember            = errors.New("no eligible archive member")
	ErrAmbiguousArchiveMember             = errors.New("ambiguous archive member")
	ErrInvalidArchiveMemberSelection      = errors.New("invalid archive member selection")
	ErrIncompatibleArchiveMemberSelection = errors.New("incompatible archive member selection")
)

// ArchiveMemberResolutionReason identifies a member-resolution outcome for
// callers that need to present or serialize selection failures.
type ArchiveMemberResolutionReason string

const (
	ArchiveMemberNoEligible            ArchiveMemberResolutionReason = "no_eligible_member"
	ArchiveMemberAmbiguous             ArchiveMemberResolutionReason = "ambiguous_member"
	ArchiveMemberInvalidSelection      ArchiveMemberResolutionReason = "invalid_selection"
	ArchiveMemberIncompatibleSelection ArchiveMemberResolutionReason = "incompatible_selection"
)

// ArchiveMemberResolutionError is returned when an archive member cannot be
// resolved safely. It supports both errors.Is and reason-based branching.
type ArchiveMemberResolutionError struct {
	Reason     ArchiveMemberResolutionReason
	Member     string
	Candidates []string
}

func (e *ArchiveMemberResolutionError) Error() string {
	if e.Member == "" {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.Member)
}

func (e *ArchiveMemberResolutionError) Unwrap() error {
	switch e.Reason {
	case ArchiveMemberNoEligible:
		return ErrNoEligibleArchiveMember
	case ArchiveMemberAmbiguous:
		return ErrAmbiguousArchiveMember
	case ArchiveMemberInvalidSelection:
		return ErrInvalidArchiveMemberSelection
	case ArchiveMemberIncompatibleSelection:
		return ErrIncompatibleArchiveMemberSelection
	default:
		return nil
	}
}

type archiveMemberResolutionRequest struct {
	packagePath       string
	logicalName       string
	explicit          string
	explicitSelection bool
}

// resolveArchiveMember deterministically resolves an executable owned by an
// artifact inventory. It does not mutate the inventory or depend on traversal
// order. Runtime and payload eligibility are recorded before resolution.
func resolveArchiveMember(inventory *artifactInventory, request archiveMemberResolutionRequest) (*artifactInventoryEntry, error) {
	if inventory == nil {
		return nil, archiveMemberResolutionError(ArchiveMemberNoEligible, "")
	}
	if request.explicitSelection || request.explicit != "" {
		return resolveExplicitArchiveMember(inventory, request.explicit)
	}

	candidates := eligibleArchiveMembers(inventory)
	if len(candidates) == 0 {
		return nil, archiveMemberResolutionError(ArchiveMemberNoEligible, "")
	}
	if entry, err := resolveArchiveMemberIdentity(candidates, request.packagePath, false); entry != nil || err != nil {
		return entry, err
	}
	if entry, err := resolveArchiveMemberIdentity(candidates, request.logicalName, false); entry != nil || err != nil {
		return entry, err
	}
	if entry, err := resolveArchiveMemberIdentity(candidates, request.packagePath, true); entry != nil || err != nil {
		return entry, err
	}
	if entry, err := resolveArchiveMemberIdentity(candidates, request.logicalName, true); entry != nil || err != nil {
		return entry, err
	}
	// A release with one eligible member is safe to install even when its
	// archive-derived name does not repeat the repository name. A stored member
	// path, however, is an identity assertion and must not be bypassed.
	if len(candidates) == 1 && request.packagePath == "" {
		return candidates[0], nil
	}
	entry, err := resolveArchiveMemberBasename(candidates, request.logicalName)
	if errors.Is(err, ErrNoEligibleArchiveMember) && request.packagePath == "" {
		return nil, archiveMemberResolutionErrorWithCandidates(ArchiveMemberAmbiguous, request.logicalName, candidates)
	}
	return entry, err
}

func resolveExplicitArchiveMember(inventory *artifactInventory, explicit string) (*artifactInventoryEntry, error) {
	identity, err := normalizeArtifactMemberIdentity(explicit)
	if err != nil {
		return nil, archiveMemberResolutionError(ArchiveMemberInvalidSelection, explicit)
	}
	if inventory == nil {
		return nil, archiveMemberResolutionError(ArchiveMemberInvalidSelection, explicit)
	}
	for index := range inventory.entries {
		entry := &inventory.entries[index]
		if entry.identity != identity {
			continue
		}
		if !isEligibleArchiveMember(entry) {
			return nil, archiveMemberResolutionError(ArchiveMemberIncompatibleSelection, explicit)
		}
		return entry, nil
	}
	return nil, archiveMemberResolutionError(ArchiveMemberInvalidSelection, explicit)
}

func eligibleArchiveMembers(inventory *artifactInventory) []*artifactInventoryEntry {
	if inventory == nil {
		return nil
	}
	candidates := make([]*artifactInventoryEntry, 0, len(inventory.entries))
	for index := range inventory.entries {
		entry := &inventory.entries[index]
		if isEligibleArchiveMember(entry) {
			candidates = append(candidates, entry)
		}
	}
	return candidates
}

func isEligibleArchiveMember(entry *artifactInventoryEntry) bool {
	return entry.class == artifactEntryExecutable && entry.stagedPath != "" && entry.targetCompatible && entry.runnable
}

func resolveArchiveMemberIdentity(candidates []*artifactInventoryEntry, requested string, normalizeWrapper bool) (*artifactInventoryEntry, error) {
	if requested == "" {
		return nil, nil
	}
	requestedIdentity, err := normalizeArtifactMemberIdentity(requested)
	if err != nil {
		return nil, nil
	}
	if normalizeWrapper {
		requestedIdentity = normalizeArchiveMemberVersionWrapper(requestedIdentity)
	}
	matches := make([]*artifactInventoryEntry, 0, len(candidates))
	for _, entry := range candidates {
		identity := entry.identity
		if normalizeWrapper {
			identity = normalizeArchiveMemberVersionWrapper(identity)
		}
		if identity == requestedIdentity {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return matches[0], nil
	default:
		return nil, archiveMemberResolutionErrorWithCandidates(ArchiveMemberAmbiguous, requested, matches)
	}
}

func resolveArchiveMemberBasename(candidates []*artifactInventoryEntry, requested string) (*artifactInventoryEntry, error) {
	if requested == "" {
		return nil, archiveMemberResolutionErrorWithCandidates(ArchiveMemberAmbiguous, "", candidates)
	}
	requestedLeaf, err := archiveMemberLeaf(requested)
	if err != nil {
		return nil, archiveMemberResolutionError(ArchiveMemberNoEligible, "")
	}
	matches := make([]*artifactInventoryEntry, 0, len(candidates))
	for _, entry := range candidates {
		leaf, err := archiveMemberLeaf(entry.identity)
		if err == nil && leaf == requestedLeaf {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return nil, archiveMemberResolutionError(ArchiveMemberNoEligible, requested)
	case 1:
		return matches[0], nil
	default:
		return nil, archiveMemberResolutionErrorWithCandidates(ArchiveMemberAmbiguous, requested, matches)
	}
}

func normalizeArchiveMemberVersionWrapper(identity string) string {
	leaf, _ := archiveMemberLeaf(identity)
	return normalizeArchiveMemberForProduct(identity, leaf)
}

func normalizeArchiveMemberForProduct(identity, product string) string {
	if scopeEnd := strings.LastIndex(identity, "!/"); scopeEnd >= 0 {
		return identity[:scopeEnd+2] + normalizeArchiveMemberForProduct(identity[scopeEnd+2:], product)
	}
	parts := strings.Split(identity, "/")
	if len(parts) < 2 {
		return identity
	}
	leaf := parts[len(parts)-1]
	if len(parts) >= 3 && (parts[0] == leaf || parts[0] == product) && isVersionToken(parts[1]) {
		return strings.Join(parts[2:], "/")
	}
	if isVersionArchiveWrapper(parts[0], leaf) || isVersionArchiveWrapper(parts[0], product) ||
		(strings.HasPrefix(parts[0], product) && product != "" && parts[0] != product && isReleaseTargetSuffix(strings.TrimPrefix(parts[0], product))) {
		return strings.Join(parts[1:], "/")
	}
	return identity
}

// isVersionArchiveWrapper reports whether component is a versioned top-level
// wrapper for leaf, such as "tool-v1", "gum_2.0.1_Darwin_arm64", or
// "mago-1.49.0-aarch64-apple-darwin". The wrapper must start with the leaf
// followed by a separator, a version token, and only recognized target tokens.
func isVersionArchiveWrapper(component, leaf string) bool {
	if leaf == "" || len(component) <= len(leaf) || component[:len(leaf)] != leaf {
		return false
	}
	remainder := component[len(leaf):]
	if remainder[0] != '-' && remainder[0] != '_' {
		return false
	}
	remainder = remainder[1:]
	if isVersionToken(remainder) {
		return true
	}
	for separator := len(remainder) - 1; separator > 0; separator-- {
		if strings.ContainsRune("-_.", rune(remainder[separator])) && isVersionToken(remainder[:separator]) && isReleaseTargetSuffix(remainder[separator:]) {
			return true
		}
	}
	return false
}

func isReleaseTargetSuffix(suffix string) bool {
	for suffix != "" {
		if suffix[0] != '-' && suffix[0] != '_' && suffix[0] != '.' {
			return false
		}
		suffix = suffix[1:]
		matched := 0
		for end := len(suffix); end > 0; end-- {
			if end < len(suffix) && suffix[end] != '-' && suffix[end] != '_' && suffix[end] != '.' {
				continue
			}
			token := strings.ToLower(suffix[:end])
			if isReleaseTargetToken(token) || token == "unknown" || token == "pc" || token == "msvc" {
				matched = end
				break
			}
		}
		if matched == 0 {
			return false
		}
		suffix = suffix[matched:]
	}
	return true
}

func isVersionToken(token string) bool {
	version := strings.TrimPrefix(token, "v")
	if version == "" {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func archiveMemberResolutionError(reason ArchiveMemberResolutionReason, member string) error {
	return &ArchiveMemberResolutionError{Reason: reason, Member: member}
}

func archiveMemberResolutionErrorWithCandidates(reason ArchiveMemberResolutionReason, member string, entries []*artifactInventoryEntry) error {
	candidates := make([]string, 0, len(entries))
	for _, entry := range entries {
		candidates = append(candidates, entry.identity)
	}
	sort.Strings(candidates)
	return &ArchiveMemberResolutionError{Reason: reason, Member: member, Candidates: candidates}
}
