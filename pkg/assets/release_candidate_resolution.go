package assets

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrNoEligibleReleaseCandidate       = errors.New("no eligible release candidate")
	ErrAmbiguousReleaseProduct          = errors.New("ambiguous release product")
	ErrAmbiguousReleaseVariant          = errors.New("ambiguous release variant")
	ErrIncompatibleReleaseTarget        = errors.New("incompatible release target")
	ErrInvalidReleaseCandidateSelection = errors.New("invalid release candidate selection")
)

// ReleaseCandidateResolutionReason identifies a release-candidate resolution
// failure for callers that need stable presentation or serialized diagnostics.
type ReleaseCandidateResolutionReason string

const (
	ReleaseCandidateNoEligible       ReleaseCandidateResolutionReason = "no_eligible_candidate"
	ReleaseCandidateAmbiguousProduct ReleaseCandidateResolutionReason = "ambiguous_product"
	ReleaseCandidateAmbiguousVariant ReleaseCandidateResolutionReason = "ambiguous_variant"
	ReleaseCandidateIncompatible     ReleaseCandidateResolutionReason = "incompatible_target"
	ReleaseCandidateInvalidSelection ReleaseCandidateResolutionReason = "invalid_selection"
)

// ReleaseCandidateResolutionError supports errors.Is as well as reason-based
// branching. Candidates are sorted candidate IDs where applicable.
type ReleaseCandidateResolutionError struct {
	Reason     ReleaseCandidateResolutionReason
	Selection  string
	Candidates []string
}

func (e *ReleaseCandidateResolutionError) Error() string {
	if e.Selection == "" {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.Selection)
}

func (e *ReleaseCandidateResolutionError) Unwrap() error {
	switch e.Reason {
	case ReleaseCandidateNoEligible:
		return ErrNoEligibleReleaseCandidate
	case ReleaseCandidateAmbiguousProduct:
		return ErrAmbiguousReleaseProduct
	case ReleaseCandidateAmbiguousVariant:
		return ErrAmbiguousReleaseVariant
	case ReleaseCandidateIncompatible:
		return ErrIncompatibleReleaseTarget
	case ReleaseCandidateInvalidSelection:
		return ErrInvalidReleaseCandidateSelection
	default:
		return nil
	}
}

// ReleaseTarget describes target facts independently. Values within a field
// are aliases for the same fact; repeated values never add preference.
type ReleaseTarget struct {
	OS           []string
	Architecture []string
	ABI          []string
	CPUVariant   []string
}

// ReleasePackageFormat identifies packaging independently from target facts.
// AppImage has an implied Linux target; other formats are target-neutral here.
type ReleasePackageFormat string

const ReleasePackageFormatAppImage ReleasePackageFormat = "appimage"

// ReleaseCandidate is a provider-independent description of one artifact.
// Product, target, ABI, CPU variant, and packaging intentionally remain
// separate so packaging can only decide between eligible equivalents.
type ReleaseCandidate struct {
	ID            string
	Product       string
	Target        ReleaseTarget
	Format        ReleasePackageFormat
	ImpliedTarget ReleaseTarget
}

// CPUVariantRule explicitly permits a candidate CPU variant for a requested
// CPU variant. Without a matching rule, CPU-specific candidates only match the
// same requested variant and never replace a baseline target.
type CPUVariantRule struct {
	Requested string
	Candidate string
}

// ReleaseCandidateResolutionRequest describes an intended product and target.
// PackagePreferences are considered only after all compatibility checks.
type ReleaseCandidateResolutionRequest struct {
	Product             string
	Target              ReleaseTarget
	PackagePreferences  []ReleasePackageFormat
	ExplicitID          string
	ExplicitSelection   bool
	SafeCPUVariantRules []CPUVariantRule
}

// ReleaseCandidateResolution is the deterministic result of resolving one
// release artifact.
type ReleaseCandidateResolution struct {
	Candidate          ReleaseCandidate
	EligibleCandidates []ReleaseCandidate
}

// ResolveReleaseCandidate resolves a release artifact without provider,
// filesystem, or prompt state. An explicit selection identifies an artifact,
// but cannot bypass target or CPU-variant compatibility.
func ResolveReleaseCandidate(candidates []ReleaseCandidate, request ReleaseCandidateResolutionRequest) (*ReleaseCandidateResolution, error) {
	if request.ExplicitSelection || request.ExplicitID != "" {
		return resolveExplicitReleaseCandidate(candidates, request)
	}
	if len(candidates) == 0 {
		return nil, releaseCandidateResolutionError(ReleaseCandidateNoEligible, "", nil)
	}

	products := groupReleaseCandidatesByProduct(candidates)
	product := canonicalProduct(request.Product)
	if product == "" {
		if len(products) != 1 {
			return nil, releaseCandidateResolutionError(ReleaseCandidateAmbiguousProduct, "", productIDs(products))
		}
		for product = range products {
		}
	}
	productCandidates := products[product]
	if len(productCandidates) == 0 {
		return nil, releaseCandidateResolutionError(ReleaseCandidateNoEligible, request.Product, nil)
	}
	return selectEligibleReleaseCandidate(productCandidates, request)
}

func resolveExplicitReleaseCandidate(candidates []ReleaseCandidate, request ReleaseCandidateResolutionRequest) (*ReleaseCandidateResolution, error) {
	if request.ExplicitID == "" {
		return nil, releaseCandidateResolutionError(ReleaseCandidateInvalidSelection, "", nil)
	}
	matched := make([]ReleaseCandidate, 0, 1)
	for _, candidate := range candidates {
		if candidate.ID == request.ExplicitID {
			matched = append(matched, candidate)
		}
	}
	if len(matched) != 1 {
		return nil, releaseCandidateResolutionError(ReleaseCandidateInvalidSelection, request.ExplicitID, candidateIDs(matched))
	}
	if request.Product != "" && canonicalProduct(matched[0].Product) != canonicalProduct(request.Product) {
		return nil, releaseCandidateResolutionError(ReleaseCandidateInvalidSelection, request.ExplicitID, nil)
	}
	return selectEligibleReleaseCandidate(matched, request)
}

func selectEligibleReleaseCandidate(candidates []ReleaseCandidate, request ReleaseCandidateResolutionRequest) (*ReleaseCandidateResolution, error) {
	eligible := make([]ReleaseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateMatchesReleaseTarget(candidate, request.Target, request.SafeCPUVariantRules) {
			eligible = append(eligible, candidate)
		}
	}
	if len(eligible) == 0 {
		return nil, releaseCandidateResolutionError(ReleaseCandidateIncompatible, request.ExplicitID, candidateIDs(candidates))
	}
	eligible = preferExactABI(eligible, request.Target.ABI)
	eligible = mostSpecificReleaseCandidates(eligible)
	if hasAmbiguousReleaseVariant(eligible) {
		return nil, releaseCandidateResolutionError(ReleaseCandidateAmbiguousVariant, "", candidateIDs(eligible))
	}
	sort.Slice(eligible, func(i, j int) bool {
		left, right := packagePreference(eligible[i].Format, request.PackagePreferences), packagePreference(eligible[j].Format, request.PackagePreferences)
		if left != right {
			return left < right
		}
		if eligible[i].Format != eligible[j].Format {
			return eligible[i].Format < eligible[j].Format
		}
		return eligible[i].ID < eligible[j].ID
	})
	return &ReleaseCandidateResolution{Candidate: eligible[0], EligibleCandidates: eligible}, nil
}

func preferExactABI(candidates []ReleaseCandidate, requested []string) []ReleaseCandidate {
	exact := make([]ReleaseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		target, valid := mergedCandidateTarget(candidate)
		if valid && targetFieldMatches(target.ABI, requested, canonicalABI) {
			exact = append(exact, candidate)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return candidates
}

func mostSpecificReleaseCandidates(candidates []ReleaseCandidate) []ReleaseCandidate {
	maximum := -1
	filtered := make([]ReleaseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		target, _ := mergedCandidateTarget(candidate)
		specificity := len(canonicalValues(target.OS, canonicalOS)) +
			len(canonicalValues(target.Architecture, canonicalArchitecture)) +
			len(canonicalValues(target.ABI, canonicalABI)) +
			len(canonicalValues(target.CPUVariant, canonicalCPUVariant))
		if specificity > maximum {
			maximum, filtered = specificity, filtered[:0]
		}
		if specificity == maximum {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func candidateMatchesReleaseTarget(candidate ReleaseCandidate, requested ReleaseTarget, rules []CPUVariantRule) bool {
	target, valid := mergedCandidateTarget(candidate)
	if !valid {
		return false
	}
	return targetFieldMatches(target.OS, requested.OS, canonicalOS) &&
		targetFieldMatches(target.Architecture, requested.Architecture, canonicalArchitecture) &&
		abiMatches(target.ABI, requested.ABI) &&
		cpuVariantMatches(target.CPUVariant, requested.CPUVariant, rules)
}

func abiMatches(candidate, requested []string) bool {
	if targetFieldMatches(candidate, requested, canonicalABI) {
		return true
	}
	candidateValues, requestedValues := canonicalValues(candidate, canonicalABI), canonicalValues(requested, canonicalABI)
	_, candidateIsMusl := candidateValues["musl"]
	_, requestedIsGlibc := requestedValues["glibc"]
	return len(candidateValues) == 1 && len(requestedValues) == 1 && candidateIsMusl && requestedIsGlibc
}

func mergedCandidateTarget(candidate ReleaseCandidate) (ReleaseTarget, bool) {
	implied := candidate.ImpliedTarget
	if strings.EqualFold(string(candidate.Format), string(ReleasePackageFormatAppImage)) {
		implied.OS = append([]string(nil), implied.OS...)
		implied.OS = append(implied.OS, "linux")
	}
	if !singleTargetFact(candidate.Target.OS, canonicalOS) ||
		!singleTargetFact(candidate.Target.Architecture, canonicalArchitecture) ||
		!singleTargetFact(candidate.Target.ABI, canonicalABI) ||
		!singleTargetFact(candidate.Target.CPUVariant, canonicalCPUVariant) ||
		!singleTargetFact(implied.OS, canonicalOS) ||
		!singleTargetFact(implied.Architecture, canonicalArchitecture) ||
		!singleTargetFact(implied.ABI, canonicalABI) ||
		!singleTargetFact(implied.CPUVariant, canonicalCPUVariant) {
		return ReleaseTarget{}, false
	}
	return ReleaseTarget{
			OS:           mergeTargetValues(candidate.Target.OS, implied.OS, canonicalOS),
			Architecture: mergeTargetValues(candidate.Target.Architecture, implied.Architecture, canonicalArchitecture),
			ABI:          mergeTargetValues(candidate.Target.ABI, implied.ABI, canonicalABI),
			CPUVariant:   mergeTargetValues(candidate.Target.CPUVariant, implied.CPUVariant, canonicalCPUVariant),
		}, targetValuesCompatible(candidate.Target.OS, implied.OS, canonicalOS) &&
			targetValuesCompatible(candidate.Target.Architecture, implied.Architecture, canonicalArchitecture) &&
			targetValuesCompatible(candidate.Target.ABI, implied.ABI, canonicalABI) &&
			targetValuesCompatible(candidate.Target.CPUVariant, implied.CPUVariant, canonicalCPUVariant)
}

func targetFieldMatches(candidate, requested []string, canonical func(string) string) bool {
	candidateValues, requestedValues := canonicalValues(candidate, canonical), canonicalValues(requested, canonical)
	if len(candidateValues) == 0 || len(requestedValues) == 0 {
		return true
	}
	for value := range candidateValues {
		if _, ok := requestedValues[value]; ok {
			return true
		}
	}
	return false
}

func cpuVariantMatches(candidate, requested []string, rules []CPUVariantRule) bool {
	candidateValues, requestedValues := canonicalValues(candidate, canonicalCPUVariant), canonicalValues(requested, canonicalCPUVariant)
	if len(candidateValues) == 0 {
		return true
	}
	if len(requestedValues) == 0 { // A baseline target cannot safely use a CPU-specific artifact.
		return false
	}
	for candidateValue := range candidateValues {
		if _, ok := requestedValues[candidateValue]; ok || hasSafeCPUVariantRule(requestedValues, candidateValue, rules) {
			return true
		}
	}
	return false
}

func hasAmbiguousReleaseVariant(candidates []ReleaseCandidate) bool {
	variants := make(map[string]struct{})
	for _, candidate := range candidates {
		target, _ := mergedCandidateTarget(candidate)
		for variant := range canonicalValues(target.CPUVariant, canonicalCPUVariant) {
			variants[variant] = struct{}{}
		}
	}
	return len(variants) > 1
}

func mergeTargetValues(explicit, implied []string, canonical func(string) string) []string {
	values := canonicalValues(explicit, canonical)
	for value := range canonicalValues(implied, canonical) {
		values[value] = struct{}{}
	}
	return mapKeys(values)
}

func targetValuesCompatible(explicit, implied []string, canonical func(string) string) bool {
	if len(explicit) == 0 || len(implied) == 0 {
		return true
	}
	return targetFieldMatches(explicit, implied, canonical)
}

func singleTargetFact(values []string, canonical func(string) string) bool {
	return len(canonicalValues(values, canonical)) <= 1
}

func hasSafeCPUVariantRule(requested map[string]struct{}, candidate string, rules []CPUVariantRule) bool {
	for _, rule := range rules {
		if canonicalCPUVariant(rule.Candidate) != candidate {
			continue
		}
		if _, ok := requested[canonicalCPUVariant(rule.Requested)]; ok {
			return true
		}
	}
	return false
}

func packagePreference(format ReleasePackageFormat, preferences []ReleasePackageFormat) int {
	seen := make(map[string]struct{}, len(preferences))
	index := 0
	for _, preferred := range preferences {
		value := strings.ToLower(strings.TrimSpace(string(preferred)))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		if strings.EqualFold(string(format), value) {
			return index
		}
		index++
	}
	return index
}

func groupReleaseCandidatesByProduct(candidates []ReleaseCandidate) map[string][]ReleaseCandidate {
	products := make(map[string][]ReleaseCandidate)
	for _, candidate := range candidates {
		product := canonicalProduct(candidate.Product)
		products[product] = append(products[product], candidate)
	}
	return products
}

func productIDs(products map[string][]ReleaseCandidate) []string {
	ids := make([]string, 0, len(products))
	for product := range products {
		ids = append(ids, product)
	}
	sort.Strings(ids)
	return ids
}

func candidateIDs(candidates []ReleaseCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	sort.Strings(ids)
	return ids
}

func releaseCandidateResolutionError(reason ReleaseCandidateResolutionReason, selection string, candidates []string) error {
	return &ReleaseCandidateResolutionError{Reason: reason, Selection: selection, Candidates: candidates}
}

func canonicalValues(values []string, canonical func(string) string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = canonical(value); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func canonicalProduct(value string) string    { return strings.ToLower(strings.TrimSpace(value)) }
func canonicalCPUVariant(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func canonicalOS(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "macos", "macosx", "osx", "apple", "darwin":
		return "darwin"
	case "win", "win32", "win64", "windows":
		return "windows"
	case "manylinux", "linux":
		return "linux"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func canonicalArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "x64", "64bit", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "i386", "i686", "x86", "386", "32bit":
		return "386"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func canonicalABI(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gnu", "glibc":
		return "glibc"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// describeReleaseCandidate derives the facts that affect release selection
// from an asset name. Transport metadata deliberately does not participate.
func describeReleaseCandidate(asset *Asset, intendedProduct string) ReleaseCandidate {
	name := normalizedAssetBasename(asset.Name)
	lower := strings.ToLower(name)
	candidate := ReleaseCandidate{ID: asset.Name, Product: releaseProduct(name)}
	if candidate.Product == "" && containsDelimitedToken(lower, canonicalProduct(intendedProduct)) {
		candidate.Product = canonicalProduct(intendedProduct)
	}

	for _, token := range knownOSTokens {
		if containsDelimitedToken(lower, token) {
			candidate.Target.OS = append(candidate.Target.OS, canonicalOS(token))
		}
	}
	for _, token := range knownArchTokens {
		if containsReleaseArchitectureToken(lower, token) {
			candidate.Target.Architecture = append(candidate.Target.Architecture, canonicalArchitecture(token))
		}
	}
	for _, token := range knownLibCTokens {
		if containsDelimitedToken(lower, token) {
			candidate.Target.ABI = append(candidate.Target.ABI, canonicalABI(token))
		}
	}
	for _, token := range knownCPUVariantTokens {
		if containsDelimitedToken(lower, token) {
			candidate.Target.CPUVariant = append(candidate.Target.CPUVariant, canonicalCPUVariant(token))
		}
	}

	switch {
	case strings.HasSuffix(lower, ".appimage"):
		candidate.Format = ReleasePackageFormatAppImage
	case strings.HasSuffix(lower, ".exe"), strings.HasSuffix(lower, ".msi"):
		candidate.Format = ReleasePackageFormat("windows")
		candidate.ImpliedTarget.OS = []string{"windows"}
	case strings.HasSuffix(lower, ".dmg"):
		candidate.Format = ReleasePackageFormat("dmg")
		candidate.ImpliedTarget.OS = []string{"darwin"}
	case strings.HasSuffix(lower, ".deb"), strings.HasSuffix(lower, ".rpm"), strings.HasSuffix(lower, ".apk"), strings.HasSuffix(lower, ".flatpak"), strings.Contains(lower, ".pkg.tar"):
		candidate.Format = ReleasePackageFormat("system-package")
		candidate.ImpliedTarget.OS = []string{"linux"}
	default:
		candidate.Format = ReleasePackageFormat(releaseAssetFormat(lower))
	}
	return candidate
}

func containsReleaseArchitectureToken(name, token string) bool {
	// Short aliases are valid targets in their own right, but must not turn a
	// longer alias into contradictory architecture facts.
	if token == "x86" && strings.Contains(name, "x86_64") {
		return false
	}
	if token == "arm" && (strings.Contains(name, "arm64") || strings.Contains(name, "armv6") || strings.Contains(name, "armv7")) {
		return false
	}
	return containsDelimitedToken(name, token)
}

func releaseProduct(name string) string {
	base := strings.TrimSuffix(strings.ToLower(normalizedAssetBasename(name)), ".appimage")
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	product := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || isReleaseTargetToken(part) || isReleaseVersionToken(part) {
			break
		}
		product = append(product, part)
	}
	result := strings.Join(product, "-")
	for _, alias := range []string{"-cli", "-client", "-mcp"} {
		result = strings.TrimSuffix(result, alias)
	}
	return result
}

func isReleaseTargetToken(token string) bool {
	for _, prefix := range []string{"linux", "win"} {
		if strings.HasPrefix(token, prefix) && len(token) > len(prefix) {
			allDigits := true
			for _, ch := range token[len(prefix):] {
				allDigits = allDigits && ch >= '0' && ch <= '9'
			}
			if allDigits {
				return true
			}
		}
	}
	for _, known := range append(append([]string{}, knownOSTokens...), knownArchTokens...) {
		if canonicalOS(token) == canonicalOS(known) || canonicalArchitecture(token) == canonicalArchitecture(known) {
			return true
		}
	}
	for _, known := range knownLibCTokens {
		if token == known {
			return true
		}
	}
	for _, known := range knownCPUVariantTokens {
		if token == known {
			return true
		}
	}
	return false
}

var knownCPUVariantTokens = []string{"avx512", "avx2", "avx", "neon"}

func isReleaseVersionToken(token string) bool {
	token = strings.TrimPrefix(token, "v")
	return len(token) > 0 && token[0] >= '0' && token[0] <= '9'
}

func releaseAssetFormat(name string) string {
	for _, suffix := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst", ".tgz", ".zip", ".gz", ".xz", ".bz2", ".zst", ".tar"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimPrefix(suffix, ".")
		}
	}
	return "standalone"
}
