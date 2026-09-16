package assets

import (
	"errors"
	"sort"
)

// ArtifactEvidence is a read-only record of the identity and processing
// decisions the shared artifact resolver made for one artifact. It is produced
// by the same resolution and processing calls used for installation, so it
// reports those decisions without ranking, validating, or downloading anything
// a second time.
type ArtifactEvidence struct {
	Release         ReleaseEvidence
	Archive         ArchiveEvidence
	Transformations []string
	Integrity       ArtifactIntegrityEvidence
}

// ReleaseEvidence records how described release candidates were accepted or
// rejected. Reason and PersistedReason are populated only when no candidate
// could be selected, using the existing typed resolver reasons.
type ReleaseEvidence struct {
	Candidates      []ReleaseCandidateDecision
	Selected        string
	Reason          ReleaseCandidateResolutionReason
	PersistedReason PersistedSelectionReason
	// Ambiguous lists the sorted candidate identities involved in an
	// unresolved product, variant, or persisted-selection failure.
	Ambiguous []string
}

// ReleaseCandidateDecision pairs one described release candidate with the
// resolver's outcome. The embedded candidate carries the product, target, CPU
// variant, and package-format facts that led to the decision.
type ReleaseCandidateDecision struct {
	Candidate ReleaseCandidate
	Eligible  bool
	Selected  bool
}

// ArchiveEvidence records every member of the owned artifact inventory and the
// member the resolver selected. Reason and Candidates are populated only when
// member selection failed.
type ArchiveEvidence struct {
	Entries    []ArchiveMemberDecision
	Selected   string
	Reason     ArchiveMemberResolutionReason
	Candidates []string
}

// ArchiveMemberDecision records the classification and eligibility facts the
// resolver computed for one inventory member.
type ArchiveMemberDecision struct {
	Identity         string
	Class            string
	TargetCompatible bool
	Runnable         bool
	Eligible         bool
	Selected         bool
}

// ArtifactIntegrityEvidence records the derived digests for the downloaded
// bytes and the bytes handed to the installer. UnchangedBytes reports whether
// processing passed the download through without transforming it.
type ArtifactIntegrityEvidence struct {
	DownloadSHA256  string
	InstalledSHA256 string
	UnchangedBytes  bool
}

// Evidence returns a detached copy of the resolver and processing decisions
// recorded for this filter. It returns nil when nothing has been recorded.
func (f *Filter) Evidence() *ArtifactEvidence {
	if f == nil {
		return nil
	}
	return f.evidence.clone()
}

// EvidenceFromError returns the typed artifact evidence associated with a
// resolver failure. Failures surfaced by the shared filter carry the full
// recorded evidence; bare typed resolution errors fall back to the release,
// persisted-selection, or archive reason they identify.
func EvidenceFromError(err error) *ArtifactEvidence {
	if err == nil {
		return nil
	}
	var carrier interface {
		artifactEvidence() *ArtifactEvidence
	}
	if errors.As(err, &carrier) {
		if evidence := carrier.artifactEvidence(); evidence != nil {
			return evidence.clone()
		}
	}
	return failureEvidence(err)
}

// evidenceError attaches the filter's recorded decisions to a resolver failure
// without changing the wrapped error's message or identity.
type evidenceError struct {
	err      error
	evidence *ArtifactEvidence
}

func (e *evidenceError) Error() string { return e.err.Error() }
func (e *evidenceError) Unwrap() error { return e.err }

func (e *evidenceError) artifactEvidence() *ArtifactEvidence { return e.evidence }

// attachEvidence wraps err with the decisions recorded so far. It returns err
// unchanged when nothing has been recorded, so unsupported failures keep their
// original identity.
func (f *Filter) attachEvidence(err error) error {
	if err == nil {
		return nil
	}
	evidence := f.Evidence()
	if evidence == nil {
		return err
	}
	return &evidenceError{err: err, evidence: evidence}
}

// failureEvidence maps the bare typed resolution errors to their stable
// reasons. It is the fallback for failures that do not carry filter evidence.
func failureEvidence(err error) *ArtifactEvidence {
	if reason, persisted, ambiguous, ok := releaseFailureFacts(err); ok {
		return &ArtifactEvidence{Release: ReleaseEvidence{
			Reason:          reason,
			PersistedReason: persisted,
			Ambiguous:       ambiguous,
		}}
	}
	if reason, candidates, ok := archiveFailureFacts(err); ok {
		return &ArtifactEvidence{Archive: ArchiveEvidence{Reason: reason, Candidates: candidates}}
	}
	return nil
}

// releaseFailureFacts extracts the stable reason and ambiguous identities from a
// release failure. It reports false when err carries no release reason.
func releaseFailureFacts(err error) (reason ReleaseCandidateResolutionReason, persisted PersistedSelectionReason, ambiguous []string, ok bool) {
	var resolutionErr *ReleaseCandidateResolutionError
	if errors.As(err, &resolutionErr) {
		return resolutionErr.Reason, "", sortedCopy(resolutionErr.Candidates), true
	}
	var persistedErr *PersistedSelectionError
	if errors.As(err, &persistedErr) {
		if persistedErr.Selection != "" {
			ambiguous = []string{persistedErr.Selection}
		}
		return "", persistedErr.Reason, ambiguous, true
	}
	return "", "", nil, false
}

// archiveFailureFacts extracts the stable reason and candidate identities from an
// archive-member failure. It reports false when err carries no archive reason.
func archiveFailureFacts(err error) (ArchiveMemberResolutionReason, []string, bool) {
	var archiveErr *ArchiveMemberResolutionError
	if errors.As(err, &archiveErr) {
		return archiveErr.Reason, sortedCopy(archiveErr.Candidates), true
	}
	return "", nil, false
}

func (f *Filter) evidenceRecord() *ArtifactEvidence {
	if f.evidence == nil {
		f.evidence = &ArtifactEvidence{}
	}
	return f.evidence
}

// recordReleaseResolution annotates every described candidate with the
// eligibility already computed by resolution. It never reranks; the selected
// candidate and eligible set come directly from the resolution result.
func (f *Filter) recordReleaseResolution(candidates []ReleaseCandidate, resolution *ReleaseCandidateResolution) {
	if resolution == nil {
		return
	}
	eligible := make(map[string]struct{}, len(resolution.EligibleCandidates))
	for _, candidate := range resolution.EligibleCandidates {
		eligible[candidate.ID] = struct{}{}
	}
	decisions := make([]ReleaseCandidateDecision, 0, len(candidates))
	for _, candidate := range candidates {
		_, ok := eligible[candidate.ID]
		decisions = append(decisions, ReleaseCandidateDecision{
			Candidate: cloneReleaseCandidate(candidate),
			Eligible:  ok,
			Selected:  candidate.ID == resolution.Candidate.ID,
		})
	}
	f.evidenceRecord().Release = ReleaseEvidence{Candidates: decisions, Selected: resolution.Candidate.ID}
}

func (f *Filter) recordReleaseFailure(candidates []ReleaseCandidate, err error) {
	release := ReleaseEvidence{Candidates: make([]ReleaseCandidateDecision, 0, len(candidates))}
	for _, candidate := range candidates {
		release.Candidates = append(release.Candidates, ReleaseCandidateDecision{Candidate: cloneReleaseCandidate(candidate)})
	}
	release.Reason, release.PersistedReason, release.Ambiguous, _ = releaseFailureFacts(err)
	f.evidenceRecord().Release = release
}

// completeReleaseSelection reconciles the recorded release decision with the
// asset the command will actually use, including interactive fallback paths
// that recovered after an initial ambiguity.
func (f *Filter) completeReleaseSelection(identity string) {
	if f == nil || identity == "" {
		return
	}
	release := &f.evidenceRecord().Release
	release.Selected = identity
	release.Reason = ""
	release.PersistedReason = ""
	release.Ambiguous = nil
	for index := range release.Candidates {
		selected := release.Candidates[index].Candidate.ID == identity
		release.Candidates[index].Selected = selected
		if selected {
			release.Candidates[index].Eligible = true
		}
	}
}

// recordArchiveInventory snapshots the owned inventory after eligibility facts
// have been recorded, before any member is selected or prompted for.
func (f *Filter) recordArchiveInventory(inventory *artifactInventory) {
	if inventory == nil {
		return
	}
	entries := make([]ArchiveMemberDecision, 0, len(inventory.entries))
	for index := range inventory.entries {
		entry := &inventory.entries[index]
		entries = append(entries, ArchiveMemberDecision{
			Identity:         entry.identity,
			Class:            entry.class.String(),
			TargetCompatible: entry.targetCompatible,
			Runnable:         entry.runnable,
			Eligible:         isEligibleArchiveMember(entry),
		})
	}
	f.evidenceRecord().Archive = ArchiveEvidence{Entries: entries}
}

func (f *Filter) recordArchiveFailure(err error) {
	reason, candidates, ok := archiveFailureFacts(err)
	if !ok {
		return
	}
	archive := &f.evidenceRecord().Archive
	archive.Reason = reason
	archive.Candidates = candidates
}

func (f *Filter) setArchiveSelection(identity string) {
	if f == nil || identity == "" {
		return
	}
	archive := &f.evidenceRecord().Archive
	archive.Selected = identity
	archive.Reason = ""
	archive.Candidates = nil
	for index := range archive.Entries {
		archive.Entries[index].Selected = archive.Entries[index].Identity == identity
	}
}

func (f *Filter) recordTransformation(format artifactFormat) {
	if format == artifactFormatPlain {
		return
	}
	evidence := f.evidenceRecord()
	evidence.Transformations = append(evidence.Transformations, format.String())
}

func (f *Filter) recordProcessingIntegrity(final *finalFile) {
	if f == nil || final == nil {
		return
	}
	f.evidenceRecord().Integrity = ArtifactIntegrityEvidence{
		DownloadSHA256:  final.DownloadSHA256,
		InstalledSHA256: final.InstalledSHA256,
		UnchangedBytes:  final.UnchangedBytes,
	}
}

func (e *ArtifactEvidence) clone() *ArtifactEvidence {
	if e == nil {
		return nil
	}
	cloned := &ArtifactEvidence{
		Transformations: append([]string(nil), e.Transformations...),
		Integrity:       e.Integrity,
		Release: ReleaseEvidence{
			Selected:        e.Release.Selected,
			Reason:          e.Release.Reason,
			PersistedReason: e.Release.PersistedReason,
			Ambiguous:       append([]string(nil), e.Release.Ambiguous...),
		},
		Archive: ArchiveEvidence{
			Selected:   e.Archive.Selected,
			Reason:     e.Archive.Reason,
			Candidates: append([]string(nil), e.Archive.Candidates...),
		},
	}
	for _, decision := range e.Release.Candidates {
		cloned.Release.Candidates = append(cloned.Release.Candidates, ReleaseCandidateDecision{
			Candidate: cloneReleaseCandidate(decision.Candidate),
			Eligible:  decision.Eligible,
			Selected:  decision.Selected,
		})
	}
	cloned.Archive.Entries = append(cloned.Archive.Entries, e.Archive.Entries...)
	return cloned
}

func cloneReleaseCandidate(candidate ReleaseCandidate) ReleaseCandidate {
	candidate.Target = cloneReleaseTarget(candidate.Target)
	candidate.ImpliedTarget = cloneReleaseTarget(candidate.ImpliedTarget)
	return candidate
}

func cloneReleaseTarget(target ReleaseTarget) ReleaseTarget {
	return ReleaseTarget{
		OS:           append([]string(nil), target.OS...),
		Architecture: append([]string(nil), target.Architecture...),
		ABI:          append([]string(nil), target.ABI...),
		CPUVariant:   append([]string(nil), target.CPUVariant...),
	}
}

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted
}

func (c artifactEntryClass) String() string {
	switch c {
	case artifactEntryExecutable:
		return "executable"
	case artifactEntryCompletion:
		return "completion"
	case artifactEntryIgnored:
		return "ignored"
	default:
		return "unknown"
	}
}
