package assets

import (
	"errors"
	"fmt"

	"github.com/aaronflorey/bin/pkg/config"
)

// ErrUnavailablePersistedSelection indicates that a previously selected
// product, target, variant, or archive member is absent from this release.
var ErrUnavailablePersistedSelection = errors.New("unavailable persisted selection")

// PersistedSelectionReason identifies which persisted constraint is absent.
type PersistedSelectionReason string

const (
	PersistedSelectionProduct PersistedSelectionReason = "product_unavailable"
	PersistedSelectionTarget  PersistedSelectionReason = "target_unavailable"
	PersistedSelectionVariant PersistedSelectionReason = "variant_unavailable"
	PersistedSelectionMember  PersistedSelectionReason = "member_unavailable"
)

// PersistedSelectionError is a stable failure for an unavailable descriptor
// constraint. It intentionally does not permit ranking a different build.
type PersistedSelectionError struct {
	Reason    PersistedSelectionReason
	Selection string
}

func (e *PersistedSelectionError) Error() string {
	if e.Selection == "" {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.Selection)
}

func (e *PersistedSelectionError) Unwrap() error { return ErrUnavailablePersistedSelection }

// ResolvePersistedSelection applies descriptor constraints before ordinary
// release resolution. Unlike platform compatibility preferences, a stored
// target fact must be stated by a candidate; an omitted fact cannot satisfy it.
func ResolvePersistedSelection(candidates []ReleaseCandidate, request ReleaseCandidateResolutionRequest, descriptor *config.SelectionDescriptor) (*ReleaseCandidateResolution, error) {
	if descriptor == nil {
		return ResolveReleaseCandidate(candidates, request)
	}
	if descriptor.LogicalProduct != "" {
		request.Product = descriptor.LogicalProduct
		if !hasReleaseProduct(candidates, descriptor.LogicalProduct) {
			return nil, persistedSelectionError(PersistedSelectionProduct, descriptor.LogicalProduct)
		}
	}
	if descriptor.Target != nil {
		constraints := selectionTarget(descriptor.Target)
		candidates = candidatesWithPersistedTarget(candidates, constraints)
		if len(candidates) == 0 {
			reason := PersistedSelectionTarget
			if descriptor.Target.CPUVariant != "" {
				reason = PersistedSelectionVariant
			}
			return nil, persistedSelectionError(reason, descriptorTargetSelection(descriptor.Target))
		}
	}
	resolution, err := ResolveReleaseCandidate(candidates, request)
	if err != nil && (errors.Is(err, ErrNoEligibleReleaseCandidate) || errors.Is(err, ErrIncompatibleReleaseTarget)) {
		reason := PersistedSelectionTarget
		selection := ""
		if descriptor.Target != nil {
			selection = descriptorTargetSelection(descriptor.Target)
			if descriptor.Target.CPUVariant != "" {
				reason = PersistedSelectionVariant
			}
		}
		return nil, persistedSelectionError(reason, selection)
	}
	return resolution, err
}

// DeriveSelectionDescriptor derives portable intent only from parsed release
// facts and a validated archive-member identity. It never uses a remote name
// or raw source filename itself as a descriptor value.
func DeriveSelectionDescriptor(binary *config.Binary) *config.SelectionDescriptor {
	if binary == nil {
		return nil
	}
	descriptor := selectionDescriptorForCandidate(describeReleaseCandidate(&Asset{Name: binary.SourceAsset}, ""))
	if isArchiveAsset(binary.SourceAsset) && binary.PackagePath != "" {
		if member, err := normalizeArtifactMemberIdentity(binary.PackagePath); err == nil {
			if descriptor == nil {
				descriptor = &config.SelectionDescriptor{}
			}
			descriptor.ArchiveMember = normalizeArchiveMemberVersionWrapper(member)
		}
	}
	return descriptor
}

func selectionDescriptorForCandidate(candidate ReleaseCandidate) *config.SelectionDescriptor {
	target, valid := mergedCandidateTarget(candidate)
	if !valid {
		return nil
	}
	descriptor := &config.SelectionDescriptor{LogicalProduct: canonicalProduct(candidate.Product)}
	descriptor.Target = descriptorTarget(target)
	if descriptor.LogicalProduct == "" && descriptor.Target == nil {
		return nil
	}
	return descriptor
}

func descriptorTarget(target ReleaseTarget) *config.SelectionTarget {
	result := &config.SelectionTarget{}
	if value := singleCanonicalValue(target.OS, canonicalOS); value != "" {
		result.OS = value
	}
	if value := singleCanonicalValue(target.Architecture, canonicalArchitecture); value != "" {
		result.Architecture = value
	}
	if value := singleCanonicalValue(target.ABI, canonicalABI); value != "" {
		result.ABI = value
	}
	if value := singleCanonicalValue(target.CPUVariant, canonicalCPUVariant); value != "" {
		result.CPUVariant = value
	}
	if *result == (config.SelectionTarget{}) {
		return nil
	}
	return result
}

func selectionTarget(target *config.SelectionTarget) ReleaseTarget {
	if target == nil {
		return ReleaseTarget{}
	}
	return ReleaseTarget{OS: []string{target.OS}, Architecture: []string{target.Architecture}, ABI: []string{target.ABI}, CPUVariant: []string{target.CPUVariant}}
}

func candidatesWithPersistedTarget(candidates []ReleaseCandidate, target ReleaseTarget) []ReleaseCandidate {
	matched := make([]ReleaseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		actual, valid := mergedCandidateTarget(candidate)
		if valid && persistedTargetMatches(actual, target) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

func persistedTargetMatches(actual, required ReleaseTarget) bool {
	return persistedTargetFieldMatches(actual.OS, required.OS, canonicalOS) &&
		persistedTargetFieldMatches(actual.Architecture, required.Architecture, canonicalArchitecture) &&
		persistedTargetFieldMatches(actual.ABI, required.ABI, canonicalABI) &&
		persistedTargetFieldMatches(actual.CPUVariant, required.CPUVariant, canonicalCPUVariant)
}

func persistedTargetFieldMatches(actual, required []string, canonical func(string) string) bool {
	requested := canonicalValues(required, canonical)
	if len(requested) == 0 {
		return true
	}
	available := canonicalValues(actual, canonical)
	if len(available) == 0 {
		return false
	}
	for value := range requested {
		if _, ok := available[value]; ok {
			return true
		}
	}
	return false
}

func singleCanonicalValue(values []string, canonical func(string) string) string {
	values = mapKeys(canonicalValues(values, canonical))
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func descriptorTargetSelection(target *config.SelectionTarget) string {
	if target == nil {
		return ""
	}
	return target.OS + "/" + target.Architecture + "/" + target.ABI + "/" + target.CPUVariant
}

func persistedSelectionError(reason PersistedSelectionReason, selection string) error {
	return &PersistedSelectionError{Reason: reason, Selection: selection}
}

func isArchiveAsset(name string) bool {
	format := releaseAssetFormat(normalizedAssetBasename(name))
	return format != "" && format != "standalone"
}
