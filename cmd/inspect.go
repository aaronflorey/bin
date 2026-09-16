package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/spf13/cobra"
)

const inspectSchemaVersion = 1

const (
	inspectStatusResolved   = "resolved"
	inspectStatusUnresolved = "unresolved"
	inspectStatusFailed     = "failed"
)

type inspectCmd struct {
	cmd         *cobra.Command
	opts        inspectOpts
	newProvider providerFactory
}

type inspectOpts struct {
	json     bool
	provider string
}

func newInspectCmd() *inspectCmd {
	root := &inspectCmd{newProvider: newProviderWithPolicy}
	cmd := &cobra.Command{
		Use:   "inspect <source>",
		Short: "Report artifact resolution without installing",
		Long: "Resolve a supported release or generic-URL artifact read-only and report the " +
			"selection, transformations, and integrity decisions that installation would make.\n\n" +
			"--json is required and is the only supported output. Inspection never creates or mutates " +
			"configuration, cache, log, or install state and never runs hooks, downloaded programs, " +
			"package managers, or completion work. Effectful Docker and Go-install sources and forced " +
			"non-release providers are rejected before any fetch.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          root.run,
	}

	root.cmd = cmd
	cmd.Flags().BoolVar(&root.opts.json, "json", false, "Emit the stable JSON report on stdout (required)")
	cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Forces a specific read-only provider")
	return root
}

func (root *inspectCmd) run(cmd *cobra.Command, args []string) (err error) {
	if !root.opts.json {
		return fmt.Errorf("inspect requires --json to emit its report; diagnostics are not a supported output mode")
	}

	source := strings.TrimSpace(args[0])
	if err := validateInspectSource(source, root.opts.provider); err != nil {
		return err
	}

	resolved, err := resolveFetchRequest(source, root.opts.provider, providers.FetchOpts{NonInteractive: true})
	if err != nil {
		return redactInspectError(err)
	}

	provider, err := root.newProvider(resolved.url, root.opts.provider)
	if err != nil {
		return redactInspectError(err)
	}

	fetchOpts := resolved.fetchOpts

	file, fetchErr := provider.Fetch(&fetchOpts)
	if file != nil {
		// The fetched stream owns any bounded temporary download and processing
		// resources; close it on success and on report encoding or write failure.
		// Surface close failures on the diagnostic channel instead of stdout.
		defer func() {
			if closeErr := closeFetchedFile(file); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
	}

	report := buildInspectReport(resolved.url, provider.GetID(), file, fetchErr)
	return writeInspectReport(cmd, report)
}

var inspectReadOnlyProviders = map[string]struct{}{
	"github":    {},
	"gitlab":    {},
	"codeberg":  {},
	"hashicorp": {},
	"generic":   {},
}

// validateInspectSource rejects effectful sources and forced providers before
// any provider construction, version discovery, fetch, or process execution.
func validateInspectSource(source, provider string) error {
	lower := strings.ToLower(source)
	switch {
	case strings.HasPrefix(lower, "docker://"):
		return fmt.Errorf("inspect does not support docker:// sources: image pulls are effectful")
	case strings.HasPrefix(lower, "goinstall://"):
		return fmt.Errorf("inspect does not support goinstall:// sources: builds execute processes")
	}

	switch provider {
	case "":
		return nil
	case "docker", "goinstall":
		return fmt.Errorf("inspect does not support the %q provider: it is effectful", provider)
	}
	if _, ok := inspectReadOnlyProviders[provider]; !ok {
		return fmt.Errorf("unsupported inspect provider %q", provider)
	}
	return nil
}

type inspectReport struct {
	SchemaVersion   int                     `json:"schema_version"`
	Source          inspectSourceReport     `json:"source"`
	Status          string                  `json:"status"`
	Reason          string                  `json:"reason,omitempty"`
	Artifact        *inspectArtifactReport  `json:"artifact,omitempty"`
	Release         *inspectReleaseReport   `json:"release,omitempty"`
	Archive         *inspectArchiveReport   `json:"archive,omitempty"`
	Transformations []string                `json:"transformations,omitempty"`
	Integrity       *inspectIntegrityReport `json:"integrity,omitempty"`
	Error           string                  `json:"error,omitempty"`
}

type inspectSourceReport struct {
	URL      string `json:"url"`
	Provider string `json:"provider,omitempty"`
	Version  string `json:"version,omitempty"`
}

type inspectArtifactReport struct {
	Name        string `json:"name,omitempty"`
	SourceAsset string `json:"source_asset,omitempty"`
	PackagePath string `json:"package_path,omitempty"`
}

type inspectReleaseReport struct {
	Selected        string                   `json:"selected,omitempty"`
	Reason          string                   `json:"reason,omitempty"`
	PersistedReason string                   `json:"persisted_reason,omitempty"`
	Ambiguous       []string                 `json:"ambiguous,omitempty"`
	Candidates      []inspectCandidateReport `json:"candidates,omitempty"`
}

type inspectCandidateReport struct {
	ID         string   `json:"id"`
	Product    string   `json:"product,omitempty"`
	Format     string   `json:"format,omitempty"`
	OS         []string `json:"os,omitempty"`
	Arch       []string `json:"arch,omitempty"`
	ABI        []string `json:"abi,omitempty"`
	CPUVariant []string `json:"cpu_variant,omitempty"`
	Eligible   bool     `json:"eligible"`
	Selected   bool     `json:"selected"`
}

type inspectArchiveReport struct {
	Selected   string               `json:"selected,omitempty"`
	Reason     string               `json:"reason,omitempty"`
	Candidates []string             `json:"candidates,omitempty"`
	Entries    []inspectEntryReport `json:"entries,omitempty"`
}

type inspectEntryReport struct {
	Identity         string `json:"identity"`
	Class            string `json:"class"`
	TargetCompatible bool   `json:"target_compatible"`
	Runnable         bool   `json:"runnable"`
	Eligible         bool   `json:"eligible"`
	Selected         bool   `json:"selected"`
}

type inspectIntegrityReport struct {
	// DownloadSHA256 and InstalledSHA256 are the processing digests the shared
	// resolver computed for the downloaded and installed bytes.
	DownloadSHA256  string `json:"download_sha256,omitempty"`
	InstalledSHA256 string `json:"installed_sha256,omitempty"`
	UnchangedBytes  bool   `json:"unchanged_bytes"`
	// Download and Installed are the provider's structured integrity
	// decisions, matching the records installation would persist.
	Download  *config.IntegrityRecord `json:"download,omitempty"`
	Installed *config.IntegrityRecord `json:"installed,omitempty"`
}

func buildInspectReport(source, providerID string, file *providers.File, fetchErr error) inspectReport {
	report := inspectReport{
		SchemaVersion: inspectSchemaVersion,
		Source:        inspectSourceReport{URL: redactURL(source), Provider: providerID},
		Status:        inspectStatusFailed,
	}

	var evidence *assets.ArtifactEvidence
	if file != nil {
		evidence = file.Evidence
		report.Source.Version = file.Version
		report.Artifact = &inspectArtifactReport{
			Name:        file.Name,
			SourceAsset: file.SourceAsset,
			PackagePath: file.PackagePath,
		}
	}
	if evidence == nil {
		evidence = assets.EvidenceFromError(fetchErr)
	}
	report.applyEvidence(evidence)
	report.applyProviderIntegrity(file)

	switch {
	case fetchErr == nil:
		report.Status = inspectStatusResolved
	case report.Reason != "":
		report.Status = inspectStatusUnresolved
	default:
		report.Error = redactText(fetchErr.Error())
	}
	return report
}

func (r *inspectReport) applyEvidence(evidence *assets.ArtifactEvidence) {
	if evidence == nil {
		return
	}

	if len(evidence.Release.Candidates) > 0 || evidence.Release.Selected != "" ||
		evidence.Release.Reason != "" || evidence.Release.PersistedReason != "" {
		release := &inspectReleaseReport{
			Selected:        evidence.Release.Selected,
			Reason:          string(evidence.Release.Reason),
			PersistedReason: string(evidence.Release.PersistedReason),
			Ambiguous:       evidence.Release.Ambiguous,
		}
		for _, decision := range evidence.Release.Candidates {
			release.Candidates = append(release.Candidates, inspectCandidateReport{
				ID:         decision.Candidate.ID,
				Product:    decision.Candidate.Product,
				Format:     string(decision.Candidate.Format),
				OS:         decision.Candidate.Target.OS,
				Arch:       decision.Candidate.Target.Architecture,
				ABI:        decision.Candidate.Target.ABI,
				CPUVariant: decision.Candidate.Target.CPUVariant,
				Eligible:   decision.Eligible,
				Selected:   decision.Selected,
			})
		}
		r.Release = release
	}

	if len(evidence.Archive.Entries) > 0 || evidence.Archive.Selected != "" ||
		evidence.Archive.Reason != "" || len(evidence.Archive.Candidates) > 0 {
		archive := &inspectArchiveReport{
			Selected:   evidence.Archive.Selected,
			Reason:     string(evidence.Archive.Reason),
			Candidates: evidence.Archive.Candidates,
		}
		for _, entry := range evidence.Archive.Entries {
			archive.Entries = append(archive.Entries, inspectEntryReport{
				Identity:         entry.Identity,
				Class:            entry.Class,
				TargetCompatible: entry.TargetCompatible,
				Runnable:         entry.Runnable,
				Eligible:         entry.Eligible,
				Selected:         entry.Selected,
			})
		}
		r.Archive = archive
	}

	r.Transformations = append(r.Transformations, evidence.Transformations...)

	if evidence.Integrity != (assets.ArtifactIntegrityEvidence{}) {
		r.Integrity = &inspectIntegrityReport{
			DownloadSHA256:  evidence.Integrity.DownloadSHA256,
			InstalledSHA256: evidence.Integrity.InstalledSHA256,
			UnchangedBytes:  evidence.Integrity.UnchangedBytes,
		}
	}

	r.Reason = inspectReason(evidence)
}

// applyProviderIntegrity reports the provider's structured download and
// installed integrity decisions, the same records installation persists.
func (r *inspectReport) applyProviderIntegrity(file *providers.File) {
	if file == nil {
		return
	}
	download := inspectIntegrityRecord(file.DownloadIntegrity)
	installed := inspectIntegrityRecord(file.InstalledIntegrity)
	if download == nil && installed == nil {
		return
	}
	if r.Integrity == nil {
		r.Integrity = &inspectIntegrityReport{}
	}
	r.Integrity.Download = download
	r.Integrity.Installed = installed
}

// inspectIntegrityRecord copies a provider integrity decision into its
// serializable form, redacting any URL material a source field might carry.
func inspectIntegrityRecord(record *providers.IntegrityRecord) *config.IntegrityRecord {
	decision := configIntegrityRecord(record)
	if decision == nil {
		return nil
	}
	decision.Source = redactText(decision.Source)
	return decision
}

func inspectReason(evidence *assets.ArtifactEvidence) string {
	if evidence == nil {
		return ""
	}
	switch {
	case evidence.Release.Reason != "":
		return string(evidence.Release.Reason)
	case evidence.Release.PersistedReason != "":
		return string(evidence.Release.PersistedReason)
	default:
		return string(evidence.Archive.Reason)
	}
}

func writeInspectReport(cmd *cobra.Command, report inspectReport) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode inspect report: %w", err)
	}
	payload := append(encoded, '\n')

	if _, err := cmd.OutOrStdout().Write(payload); err != nil {
		return fmt.Errorf("write inspect report: %w", err)
	}
	return nil
}

var inspectURLPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://[^\s"'<>]+`)

// inspectRedactedError preserves a returned error's identity while exposing
// only a redacted message, so diagnostics cannot leak URL secrets.
type inspectRedactedError struct{ err error }

func (e *inspectRedactedError) Error() string { return redactText(e.err.Error()) }
func (e *inspectRedactedError) Unwrap() error { return e.err }

// redactInspectError wraps err only when redaction changes its message.
func redactInspectError(err error) error {
	if err == nil {
		return nil
	}
	if redacted := redactText(err.Error()); redacted == err.Error() {
		return err
	}
	return &inspectRedactedError{err: err}
}

// redactText removes credentials and signed query/fragment data from any URL
// embedded in a diagnostic string.
func redactText(text string) string {
	if text == "" {
		return ""
	}
	return inspectURLPattern.ReplaceAllStringFunc(text, redactURL)
}

// redactURL strips userinfo, query, and fragment material from a single URL or
// URL-like source. Invalid inputs fall back to cutting at the first query or
// fragment marker.
func redactURL(raw string) string {
	core, suffix := splitTrailingPunctuation(raw)
	// A bare "user:pass@host/path" parses as scheme "user" with an opaque body,
	// so strip that userinfo before parsing.
	if !strings.Contains(core, "://") {
		if at := strings.IndexByte(core, '@'); at >= 0 {
			if slash := strings.IndexByte(core, '/'); slash == -1 || at < slash {
				core = core[at+1:]
			}
		}
	}
	parsed, err := url.Parse(core)
	if err != nil {
		return stripURLSecrets(core) + suffix
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String() + suffix
}

func splitTrailingPunctuation(raw string) (string, string) {
	trimmed := strings.TrimRight(raw, ".,;:!?)]}")
	return trimmed, raw[len(trimmed):]
}

func stripURLSecrets(raw string) string {
	if index := strings.IndexAny(raw, "?#"); index >= 0 {
		return raw[:index]
	}
	return raw
}
