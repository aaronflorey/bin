package assets

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/caarlos0/log"
	"github.com/krolaw/zipstream"
	"github.com/xi2/xz"
)

var (
	ErrUnsupportedArtifactFormat = errors.New("unsupported artifact format")
	ErrArtifactLimitExceeded     = errors.New("artifact processing limit exceeded")
	ErrDuplicateArtifactMember   = errors.New("duplicate artifact member")
)

var artifactProcessingBudgets = defaultArtifactBudgets()

const bundledCompletionMaxBytes = 1 << 20

var errInvalidBundledCompletion = errors.New("invalid bundled completion")

var (
	removeArtifactDownload = os.Remove
	removeArtifactRoot     = os.RemoveAll
)

// artifactFormat is the complete set of formats recognized by artifact
// processing. New decoders must be added here before they can be used.
type artifactFormat uint8

const (
	artifactFormatPlain artifactFormat = iota
	artifactFormatTar
	artifactFormatZip
	artifactFormatGzip
	artifactFormatXz
	artifactFormatBzip2
	artifactFormatZstandard
)

type artifactFormatDefinition struct {
	format    artifactFormat
	supported bool
}

var artifactFormats = []artifactFormatDefinition{
	{format: artifactFormatPlain, supported: true},
	{format: artifactFormatTar, supported: true},
	{format: artifactFormatZip, supported: true},
	{format: artifactFormatGzip, supported: true},
	{format: artifactFormatXz, supported: true},
	{format: artifactFormatBzip2, supported: true},
	{format: artifactFormatZstandard},
}

func (f artifactFormat) supported() bool {
	for _, definition := range artifactFormats {
		if definition.format == f {
			return definition.supported
		}
	}
	return false
}

func artifactFormatFor(name string, header []byte) artifactFormat {
	if bytes.HasPrefix(header, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		return artifactFormatZstandard
	}
	if bytes.HasPrefix(header, []byte("PK\x03\x04")) {
		return artifactFormatZip
	}
	if len(header) >= 262 && string(header[257:262]) == "ustar" {
		return artifactFormatTar
	}
	if bytes.HasPrefix(header, []byte{0x1f, 0x8b}) {
		return artifactFormatGzip
	}
	if bytes.HasPrefix(header, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}) {
		return artifactFormatXz
	}
	if bytes.HasPrefix(header, []byte("BZh")) {
		return artifactFormatBzip2
	}
	if strings.HasSuffix(strings.ToLower(name), ".zst") {
		return artifactFormatZstandard
	}
	return artifactFormatPlain
}

func validateArtifactFormat(format artifactFormat) error {
	if format.supported() {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedArtifactFormat, format)
}

func (f artifactFormat) String() string {
	switch f {
	case artifactFormatPlain:
		return "plain"
	case artifactFormatTar:
		return "tar"
	case artifactFormatZip:
		return "zip"
	case artifactFormatGzip:
		return "gzip"
	case artifactFormatXz:
		return "xz"
	case artifactFormatBzip2:
		return "bzip2"
	case artifactFormatZstandard:
		return "zstandard"
	default:
		return "unknown"
	}
}

// artifactBudgets bounds every untrusted byte stream and archive layer. The
// defaults are intentionally finite; callers may supply smaller values in
// tests or future configuration.
type artifactBudgets struct {
	maxDownloadBytes  int64
	maxArchiveEntries int64
	maxEntryBytes     int64
	maxExpandedBytes  int64
	maxNesting        int
}

func defaultArtifactBudgets() artifactBudgets {
	return artifactBudgets{
		maxDownloadBytes:  1 << 30,
		maxArchiveEntries: 10000,
		maxEntryBytes:     256 << 20,
		maxExpandedBytes:  1 << 30,
		maxNesting:        8,
	}
}

func (b artifactBudgets) validate() error {
	if b.maxDownloadBytes <= 0 || b.maxArchiveEntries <= 0 || b.maxEntryBytes <= 0 || b.maxExpandedBytes <= 0 || b.maxNesting <= 0 {
		return fmt.Errorf("artifact budgets must all be finite positive values")
	}
	return nil
}

type artifactBudgetTracker struct {
	budgets    artifactBudgets
	downloaded int64
	entries    int64
	entryBytes int64
	expanded   int64
	nesting    int
}

func newArtifactBudgetTracker(budgets artifactBudgets) (*artifactBudgetTracker, error) {
	if err := budgets.validate(); err != nil {
		return nil, err
	}
	return &artifactBudgetTracker{budgets: budgets}, nil
}

func (t *artifactBudgetTracker) addDownloadBytes(bytes int64) error {
	if bytes < 0 {
		return fmt.Errorf("invalid download byte count %d", bytes)
	}
	t.downloaded += bytes
	return t.withinLimit("download bytes", t.downloaded, t.budgets.maxDownloadBytes)
}

func (t *artifactBudgetTracker) visitArchiveEntry() error {
	t.entries++
	t.entryBytes = 0
	return t.withinLimit("archive entries", t.entries, t.budgets.maxArchiveEntries)
}

func (t *artifactBudgetTracker) addEntryBytes(entryBytes int64) error {
	if entryBytes < 0 {
		return fmt.Errorf("invalid entry byte count %d", entryBytes)
	}
	t.entryBytes += entryBytes
	if err := t.withinLimit("entry bytes", t.entryBytes, t.budgets.maxEntryBytes); err != nil {
		return err
	}
	t.expanded += entryBytes
	return t.withinLimit("expanded bytes", t.expanded, t.budgets.maxExpandedBytes)
}

func (t *artifactBudgetTracker) addExpandedBytes(bytes int64) error {
	if bytes < 0 {
		return fmt.Errorf("invalid expanded byte count %d", bytes)
	}
	t.expanded += bytes
	return t.withinLimit("expanded bytes", t.expanded, t.budgets.maxExpandedBytes)
}

func (t *artifactBudgetTracker) addPayloadLayerBytes(layerBytes, bytes int64) error {
	if bytes < 0 || layerBytes < 0 {
		return fmt.Errorf("invalid payload byte count %d", bytes)
	}
	if err := t.withinLimit("entry bytes", layerBytes, t.budgets.maxEntryBytes); err != nil {
		return err
	}
	return t.addExpandedBytes(bytes)
}

func (t *artifactBudgetTracker) enterArchive() error {
	t.nesting++
	return t.withinLimit("archive nesting", int64(t.nesting), int64(t.budgets.maxNesting))
}

func (t *artifactBudgetTracker) leaveArchive() {
	if t.nesting > 0 {
		t.nesting--
	}
}

func (t *artifactBudgetTracker) withinLimit(name string, value, limit int64) error {
	if value <= limit {
		return nil
	}
	return fmt.Errorf("%w: %s exceeds %d", ErrArtifactLimitExceeded, name, limit)
}

type artifactEntryClass uint8

const (
	artifactEntryExecutable artifactEntryClass = iota
	artifactEntryCompletion
	artifactEntryIgnored
)

type artifactInventoryEntry struct {
	identity         string
	class            artifactEntryClass
	stagedPath       string
	targetCompatible bool
	runnable         bool
}

type artifactInventory struct {
	entries    []artifactInventoryEntry
	identities map[string]struct{}
}

func newArtifactInventory() *artifactInventory {
	return &artifactInventory{identities: make(map[string]struct{})}
}

func (i *artifactInventory) add(name string, class artifactEntryClass) (artifactInventoryEntry, error) {
	identity, err := normalizeArtifactMemberIdentity(name)
	if err != nil {
		return artifactInventoryEntry{}, err
	}
	if _, exists := i.identities[identity]; exists {
		return artifactInventoryEntry{}, fmt.Errorf("%w: %q", ErrDuplicateArtifactMember, identity)
	}
	i.identities[identity] = struct{}{}
	entry := artifactInventoryEntry{identity: identity, class: class}
	i.entries = append(i.entries, entry)
	return entry, nil
}

func (i *artifactInventory) addScoped(scope, name string, class artifactEntryClass) (*artifactInventoryEntry, error) {
	identity, err := normalizeArtifactMemberIdentity(name)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		identity = scope + "!/" + identity
	}
	if _, exists := i.identities[identity]; exists {
		return nil, fmt.Errorf("%w: %q", ErrDuplicateArtifactMember, identity)
	}
	i.identities[identity] = struct{}{}
	i.entries = append(i.entries, artifactInventoryEntry{identity: identity, class: class})
	return &i.entries[len(i.entries)-1], nil
}

func normalizeArtifactMemberIdentity(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") || hasWindowsDrivePrefix(name) {
		return "", fmt.Errorf("invalid archive member %q", name)
	}
	// Archive writers conventionally spell directory entries with one trailing
	// separator. It is not an empty path component or a distinct identity.
	if strings.HasSuffix(name, "/") || strings.HasSuffix(name, "\\") {
		name = name[:len(name)-1]
	}
	components := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	if len(components) == 0 || len(components) != strings.Count(name, "/")+strings.Count(name, "\\")+1 {
		return "", fmt.Errorf("invalid archive member %q", name)
	}
	for _, component := range components {
		if component == "." || component == ".." || hasWindowsDrivePrefix(component) || strings.IndexByte(component, 0) >= 0 {
			return "", fmt.Errorf("invalid archive member %q", name)
		}
	}
	if _, err := archiveMemberLeaf(name); err != nil {
		return "", err
	}
	return strings.Join(components, "/"), nil
}

func classifyArtifactEntry(name string) artifactEntryClass {
	if isCompletionArtifact(name) {
		return artifactEntryCompletion
	}
	if IsKnownNonRunnableName(name) || looksLikeLibrary(name) || looksLikeArchiveJunk(name) || looksLikePackageArtifact(name) {
		return artifactEntryIgnored
	}
	return artifactEntryExecutable
}

func isCompletionArtifact(name string) bool {
	normalized := "/" + strings.Trim(strings.ToLower(strings.ReplaceAll(name, "\\", "/")), "/") + "/"
	return strings.Contains(normalized, "/autocomplete/") || strings.Contains(normalized, "/completions/") || strings.Contains(normalized, "/complete/")
}

// artifactProcessingResult owns the staged inventory and final candidate. Its
// Close method is the single cleanup boundary for future processors.
type artifactProcessingResult struct {
	inventory   *artifactInventory
	final       *finalFile
	cleanup     func() error
	transformed bool
}

type artifactResultReader struct {
	result *artifactProcessingResult
	reader io.Reader
}

func (r *artifactResultReader) Read(p []byte) (int, error) { return r.reader.Read(p) }
func (r *artifactResultReader) Close() error               { return r.result.Close() }

func (r *artifactProcessingResult) Close() error {
	if r == nil {
		return nil
	}
	var err error
	if r.final != nil {
		err = closeReader(r.final.Source)
		r.final = nil
	}
	if r.cleanup != nil {
		err = errors.Join(err, r.cleanup())
		r.cleanup = nil
	}
	return err
}

var _ io.Closer = (*artifactProcessingResult)(nil)

// processReleaseArtifact turns an owned download into a bounded, staged
// inventory. The returned result is its only cleanup owner.
func (f *Filter) processReleaseArtifact(downloadPath, downloadSHA string) (*artifactProcessingResult, error) {
	tracker, err := newArtifactBudgetTracker(artifactProcessingBudgets)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "bin-artifact-*")
	if err != nil {
		return nil, err
	}
	result := &artifactProcessingResult{inventory: newArtifactInventory(), cleanup: func() error {
		return errors.Join(removeArtifactDownload(downloadPath), removeArtifactRoot(root))
	}}
	if err := f.collectArtifact(downloadPath, f.name, "", root, tracker, result.inventory, &result.transformed, false); err != nil {
		_ = result.Close()
		return nil, err
	}
	if len(result.inventory.entries) == 0 {
		_ = result.Close()
		return nil, fmt.Errorf("%w: no executable files found", ErrNoCompatibleFiles)
	}
	if bundleName, ok := inventoryAppBundle(result.inventory); ok {
		_ = result.Close()
		return nil, fmt.Errorf("archive contains a macOS app bundle (%s) instead of a standalone binary: %w", bundleName, ErrNoCompatibleFiles)
	}
	recordArchiveMemberEligibility(result.inventory)
	f.recordArchiveInventory(result.inventory)
	selectedEntry, err := f.resolveReleaseArchiveMember(result.inventory)
	if err != nil {
		f.recordArchiveFailure(err)
		_ = result.Close()
		if errors.Is(err, ErrNoEligibleArchiveMember) {
			return nil, fmt.Errorf("%w: %w", ErrNoCompatibleFiles, err)
		}
		return nil, err
	}
	if f.selectionIntent == nil {
		f.selectionIntent = &config.SelectionDescriptor{}
	}
	f.setArchiveSelection(selectedEntry.identity)
	// Versioned top-level wrappers are packaging details, not part of the
	// portable member intent. Keep all other directories identity-bearing.
	f.selectionIntent.ArchiveMember = normalizeArchiveMemberVersionWrapper(selectedEntry.identity)
	bundledCompletion, bundledCompletionName := f.selectBundledCompletion(result.inventory, selectedEntry)
	file, err := os.Open(selectedEntry.stagedPath)
	if err != nil {
		_ = result.Close()
		return nil, err
	}
	leaf, err := archiveMemberLeaf(selectedEntry.identity)
	if err != nil {
		_ = file.Close()
		_ = result.Close()
		return nil, err
	}
	installedSHA, err := fileSHA256(selectedEntry.stagedPath)
	if err != nil {
		_ = file.Close()
		_ = result.Close()
		return nil, err
	}
	packagePath := selectedEntry.identity
	if !result.transformed {
		packagePath = f.packagePath
	}
	result.final = &finalFile{
		Source:                file,
		Name:                  leaf,
		PackagePath:           packagePath,
		DownloadSHA256:        downloadSHA,
		InstalledSHA256:       installedSHA,
		UnchangedBytes:        !result.transformed,
		BundledCompletion:     bundledCompletion,
		BundledCompletionName: bundledCompletionName,
	}
	return result, nil
}

func (f *Filter) selectBundledCompletion(inventory *artifactInventory, executable *artifactInventoryEntry) ([]byte, string) {
	entry, ambiguous := f.bundledCompletionEntry(inventory, executable)
	if ambiguous {
		log.Warnf("Skipping ambiguous bundled %s completion for %q", f.opts.BundledCompletionShell, f.opts.BundledCompletionCommand)
		return nil, ""
	}
	if entry == nil {
		return nil, ""
	}
	content, err := readBundledCompletion(entry.stagedPath)
	if err != nil {
		if errors.Is(err, errInvalidBundledCompletion) {
			log.Warnf("Skipping bundled completion %q: %v", entry.identity, err)
		}
		return nil, ""
	}
	return content, entry.identity
}

func (f *Filter) bundledCompletionEntry(inventory *artifactInventory, executable *artifactInventoryEntry) (*artifactInventoryEntry, bool) {
	if f.opts == nil || f.opts.BundledCompletionShell == "" || f.opts.BundledCompletionCommand == "" || executable == nil {
		return nil, false
	}

	scope, _ := artifactEntryScope(executable.identity)
	var match *artifactInventoryEntry
	for index := range inventory.entries {
		entry := &inventory.entries[index]
		if !isBundledCompletionCandidate(entry, scope, f.opts.BundledCompletionShell, f.opts.BundledCompletionCommand) {
			continue
		}
		if match != nil {
			return nil, true
		}
		match = entry
	}
	return match, false
}

func artifactEntryScope(identity string) (string, string) {
	index := strings.LastIndex(identity, "!/")
	if index < 0 {
		return "", identity
	}
	return identity[:index], identity[index+2:]
}

func isBundledCompletionCandidate(entry *artifactInventoryEntry, scope, shell, command string) bool {
	if entry.class != artifactEntryCompletion || entry.stagedPath == "" {
		return false
	}
	entryScope, member := artifactEntryScope(entry.identity)
	if entryScope != scope {
		return false
	}
	parts := strings.Split(member, "/")
	for index, part := range parts {
		if !isCompletionDirectory(part) {
			continue
		}
		return exactCompletionPath(parts[index+1:], shell, command)
	}
	return false
}

func isCompletionDirectory(name string) bool {
	switch strings.ToLower(name) {
	case "autocomplete", "completions", "complete":
		return true
	default:
		return false
	}
}

func exactCompletionPath(parts []string, shell, command string) bool {
	switch shell {
	case "bash":
		return (len(parts) == 1 && parts[0] == command+".bash") ||
			(len(parts) == 2 && parts[0] == "bash" && parts[1] == command)
	case "zsh":
		return len(parts) == 1 && parts[0] == "_"+command
	case "fish":
		return len(parts) == 1 && parts[0] == command+".fish"
	default:
		return false
	}
}

func readBundledCompletion(stagedPath string) ([]byte, error) {
	file, err := os.Open(stagedPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, bundledCompletionMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) == 0 || len(content) > bundledCompletionMaxBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, fmt.Errorf("%w: empty, oversized, or malformed text", errInvalidBundledCompletion)
	}
	return content, nil
}

// recordArchiveMemberEligibility captures runtime-dependent and staged-payload
// checks at the artifact-processing boundary. Resolution then operates only on
// this owned inventory metadata.
func recordArchiveMemberEligibility(inventory *artifactInventory) {
	for index := range inventory.entries {
		entry := &inventory.entries[index]
		entry.targetCompatible = len(filterTargetCompatibleAssets([]*Asset{{Name: entry.identity}}, false)) > 0
		entry.runnable = entry.stagedPath != "" && ValidateRunnablePayload(entry.stagedPath, entry.identity) == nil
	}
}

func (f *Filter) resolveReleaseArchiveMember(inventory *artifactInventory) (*artifactInventoryEntry, error) {
	packagePath, logicalName := "", f.repoName
	if f.opts != nil {
		if !f.opts.SkipPathCheck {
			packagePath = f.opts.PackagePath
		}
		if f.opts.PackageName != "" && !looksLikeMetadataAsset(f.opts.PackageName) && !looksLikePackageArtifact(f.opts.PackageName) {
			logicalName = f.opts.PackageName
		}
	}
	request := archiveMemberResolutionRequest{
		packagePath:       packagePath,
		logicalName:       logicalName,
		explicit:          f.containedFile,
		explicitSelection: f.containedFileSelected,
	}
	persistedMember := ""
	if f.opts != nil && f.opts.SelectionIntent != nil {
		persistedMember = f.opts.SelectionIntent.ArchiveMember
	}
	if persistedMember != "" {
		entry, err := resolvePersistedArchiveMember(inventory, persistedMember)
		if err != nil {
			return nil, persistedSelectionError(PersistedSelectionMember, persistedMember)
		}
		return entry, nil
	}
	entry, err := resolveArchiveMember(inventory, request)
	if !errors.Is(err, ErrAmbiguousArchiveMember) {
		return entry, err
	}

	var resolutionErr *ArchiveMemberResolutionError
	if !errors.As(err, &resolutionErr) || len(resolutionErr.Candidates) == 0 {
		return nil, err
	}
	return f.promptForArchiveMember(inventory, resolutionErr)
}

// resolvePersistedArchiveMember preserves an archive-member assertion while
// ignoring only the established versioned top-level wrapper convention.
func resolvePersistedArchiveMember(inventory *artifactInventory, member string) (*artifactInventoryEntry, error) {
	identity, err := normalizeArtifactMemberIdentity(member)
	if err != nil {
		return nil, err
	}
	candidates := eligibleArchiveMembers(inventory)
	if entry := resolveArchiveMemberIdentity(candidates, identity, false); entry != nil {
		return entry, nil
	}
	if entry := resolveArchiveMemberIdentity(candidates, identity, true); entry != nil {
		return entry, nil
	}
	return nil, archiveMemberResolutionError(ArchiveMemberNoEligible, member)
}

func (f *Filter) promptForArchiveMember(inventory *artifactInventory, resolutionErr *ArchiveMemberResolutionError) (*artifactInventoryEntry, error) {
	if f.opts != nil && f.opts.NonInteractive {
		return nil, fmt.Errorf("multiple matches found: %s (use --select to choose one): %w", strings.Join(resolutionErr.Candidates, ", "), resolutionErr)
	}
	if !isInteractive() {
		return nil, fmt.Errorf("multiple matches found without an interactive terminal: %s (use --select to choose one): %w", strings.Join(resolutionErr.Candidates, ", "), resolutionErr)
	}
	options := make([]fmt.Stringer, 0, len(resolutionErr.Candidates))
	for _, candidate := range resolutionErr.Candidates {
		options = append(options, archiveMemberOption(candidate))
	}
	choice, err := selectOption("Multiple matches found, please select one:", options)
	if err != nil {
		return nil, err
	}
	selected, ok := choice.(archiveMemberOption)
	if !ok {
		return nil, fmt.Errorf("invalid archive member selection %v", choice)
	}
	for index := range inventory.entries {
		if inventory.entries[index].identity == string(selected) {
			return &inventory.entries[index], nil
		}
	}
	return nil, fmt.Errorf("selected file %s not found in artifact inventory", selected)
}

type archiveMemberOption string

func (o archiveMemberOption) String() string { return string(o) }

func inventoryAppBundle(inventory *artifactInventory) (string, bool) {
	entries := make(map[string]string)
	for _, entry := range inventory.entries {
		if entry.stagedPath != "" {
			entries[entry.identity] = entry.stagedPath
		}
	}
	return isSingleAppBundleArchive(entries)
}

func (f *Filter) collectArtifact(inputPath, name, scope, root string, tracker *artifactBudgetTracker, inventory *artifactInventory, transformed *bool, alreadyStaged bool) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()
	header := make([]byte, 512)
	n, readErr := io.ReadFull(input, header)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return readErr
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if classifyArtifactEntry(name) == artifactEntryIgnored {
		entry, err := inventory.addScoped(scope, name, artifactEntryIgnored)
		if err != nil {
			return err
		}
		return discardIgnoredArtifact(&payloadBudgetReader{reader: input, tracker: tracker}, name, entry.identity, tracker, inventory)
	}
	format := artifactFormatFor(name, header[:n])
	if err := validateArtifactFormat(format); err != nil {
		return err
	}
	if format == artifactFormatTar {
		*transformed = true
		f.recordTransformation(format)
		return f.collectTar(input, scope, root, tracker, inventory, transformed)
	}
	if format == artifactFormatZip {
		*transformed = true
		f.recordTransformation(format)
		return f.collectZip(input, scope, root, tracker, inventory, transformed)
	}
	if format == artifactFormatGzip || format == artifactFormatXz || format == artifactFormatBzip2 {
		*transformed = true
		f.recordTransformation(format)
		if err := tracker.enterArchive(); err != nil {
			return err
		}
		defer tracker.leaveArchive()
		var decoded io.Reader = input
		decodedName := name
		decodedClass := classifyArtifactEntry(decodedName)
		var gzipInput *bufio.Reader
		switch format {
		case artifactFormatGzip:
			gzipInput = bufio.NewReader(input)
			gzipReader, gzipErr := gzip.NewReader(gzipInput)
			if gzipErr != nil {
				return gzipErr
			}
			gzipReader.Multistream(false)
			decoded = gzipReader
			if gzipReader.Name != "" {
				decodedName, err = normalizeArtifactMemberIdentity(gzipReader.Name)
				if err != nil {
					return err
				}
				decodedClass = classifyArtifactEntry(decodedName)
			}
		case artifactFormatXz:
			decoded, err = xz.NewReader(input, 0)
		case artifactFormatBzip2:
			decoded = bzip2.NewReader(input)
		}
		if err != nil {
			return err
		}
		if decodedClass == artifactEntryIgnored {
			entry, err := inventory.addScoped(scope, decodedName, artifactEntryIgnored)
			if err != nil {
				return err
			}
			if err := discardIgnoredArtifact(&payloadBudgetReader{reader: decoded, tracker: tracker}, decodedName, entry.identity, tracker, inventory); err != nil {
				return err
			}
			return validateAdditionalGzipMembers(gzipInput, decodedName, tracker)
		}
		decodedPath, err := stageReader(root, decoded, tracker, false)
		if err != nil {
			return err
		}
		if decodedClass == artifactEntryCompletion {
			entry, err := inventory.addScoped(scope, decodedName, artifactEntryCompletion)
			if err != nil {
				return err
			}
			entry.stagedPath = decodedPath
			return validateAdditionalGzipMembers(gzipInput, decodedName, tracker)
		}
		if err := validateAdditionalGzipMembers(gzipInput, decodedName, tracker); err != nil {
			return err
		}
		return f.collectArtifact(decodedPath, decodedName, scope, root, tracker, inventory, transformed, true)
	}
	entry, err := inventory.addScoped(scope, name, classifyArtifactEntry(name))
	if err != nil {
		return err
	}
	if entry.class == artifactEntryIgnored {
		return nil
	}
	path := inputPath
	if !alreadyStaged {
		path, err = stageFile(root, input, tracker)
		if err != nil {
			return err
		}
	}
	inventory.entries[len(inventory.entries)-1].stagedPath = path
	_ = entry
	return nil
}

func (f *Filter) collectTar(r io.Reader, scope, root string, tracker *artifactBudgetTracker, inventory *artifactInventory, transformed *bool) error {
	if err := tracker.enterArchive(); err != nil {
		return err
	}
	defer tracker.leaveArchive()
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tracker.visitArchiveEntry(); err != nil {
			return err
		}
		class := classifyArtifactEntry(h.Name)
		if h.FileInfo().IsDir() {
			class = artifactEntryIgnored
		}
		entry, err := inventory.addScoped(scope, h.Name, class)
		if err != nil {
			return err
		}
		if !h.FileInfo().Mode().IsRegular() {
			if err := discardEntry(tr, tracker); err != nil {
				return err
			}
			continue
		}
		if entry.class == artifactEntryIgnored {
			if err := discardIgnoredArtifact(&entryBudgetReader{reader: tr, tracker: tracker}, h.Name, entry.identity, tracker, inventory); err != nil {
				return err
			}
			continue
		}
		path, err := stageReader(root, tr, tracker, true)
		if err != nil {
			return err
		}
		entry.stagedPath = path
		if entry.class != artifactEntryExecutable {
			continue
		}
		if nested, err := isRecognizedArtifact(path, h.Name); err != nil {
			return err
		} else if nested {
			if err := f.collectArtifact(path, h.Name, entry.identity, root, tracker, inventory, transformed, false); err != nil {
				return err
			}
		}
	}
}

func (f *Filter) collectZip(r io.Reader, scope, root string, tracker *artifactBudgetTracker, inventory *artifactInventory, transformed *bool) error {
	if err := tracker.enterArchive(); err != nil {
		return err
	}
	defer tracker.leaveArchive()
	zr := zipstream.NewReader(r)
	for {
		h, err := zr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tracker.visitArchiveEntry(); err != nil {
			return err
		}
		class := classifyArtifactEntry(h.Name)
		if h.Mode().IsDir() {
			class = artifactEntryIgnored
		}
		entry, err := inventory.addScoped(scope, h.Name, class)
		if err != nil {
			return err
		}
		if !h.Mode().IsRegular() {
			if err := discardEntry(zr, tracker); err != nil {
				return err
			}
			continue
		}
		if entry.class == artifactEntryIgnored {
			if err := discardIgnoredArtifact(&entryBudgetReader{reader: zr, tracker: tracker}, h.Name, entry.identity, tracker, inventory); err != nil {
				return err
			}
			continue
		}
		path, err := stageReader(root, zr, tracker, true)
		if err != nil {
			return err
		}
		entry.stagedPath = path
		if entry.class != artifactEntryExecutable {
			continue
		}
		if nested, err := isRecognizedArtifact(path, h.Name); err != nil {
			return err
		} else if nested {
			if err := f.collectArtifact(path, h.Name, entry.identity, root, tracker, inventory, transformed, false); err != nil {
				return err
			}
		}
	}
}

func stageFile(root string, r io.Reader, tracker *artifactBudgetTracker) (string, error) {
	return stageReader(root, r, tracker, false)
}

func discardEntry(r io.Reader, tracker *artifactBudgetTracker) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if err := tracker.addEntryBytes(int64(n)); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type entryBudgetReader struct {
	reader  io.Reader
	tracker *artifactBudgetTracker
}

func (r *entryBudgetReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		if budgetErr := r.tracker.addEntryBytes(int64(n)); budgetErr != nil {
			return n, budgetErr
		}
	}
	return n, err
}

type payloadBudgetReader struct {
	reader     io.Reader
	tracker    *artifactBudgetTracker
	layerBytes int64
}

func (r *payloadBudgetReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.layerBytes += int64(n)
		if budgetErr := r.tracker.addPayloadLayerBytes(r.layerBytes, int64(n)); budgetErr != nil {
			return n, budgetErr
		}
	}
	return n, err
}

func validateAdditionalGzipMembers(reader *bufio.Reader, firstIdentity string, tracker *artifactBudgetTracker) error {
	if reader == nil {
		return nil
	}
	seen := map[string]artifactEntryClass{firstIdentity: classifyArtifactEntry(firstIdentity)}
	foundAdditional := false
	for {
		_, err := reader.Peek(1)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return err
		}
		gzipReader.Multistream(false)
		identity := firstIdentity
		if gzipReader.Name != "" {
			identity, err = normalizeArtifactMemberIdentity(gzipReader.Name)
			if err != nil {
				return err
			}
		}
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateArtifactMember, identity)
		}
		seen[identity] = classifyArtifactEntry(identity)
		if _, err := io.Copy(io.Discard, &payloadBudgetReader{reader: gzipReader, tracker: tracker}); err != nil {
			return err
		}
		foundAdditional = true
	}
	if foundAdditional {
		return fmt.Errorf("%w: concatenated gzip members", ErrUnsupportedArtifactFormat)
	}
	return nil
}

func discardIgnoredArtifact(r io.Reader, name, scope string, tracker *artifactBudgetTracker, inventory *artifactInventory) error {
	reader := bufio.NewReader(r)
	header, peekErr := reader.Peek(512)
	if peekErr != nil && peekErr != io.EOF && peekErr != bufio.ErrBufferFull {
		return peekErr
	}
	format := artifactFormatFor(name, header)
	if err := validateArtifactFormat(format); err != nil {
		return err
	}
	switch format {
	case artifactFormatPlain:
		_, err := io.Copy(io.Discard, reader)
		if err == io.EOF {
			return nil
		}
		return err
	case artifactFormatTar:
		return collectIgnoredTar(reader, scope, tracker, inventory)
	case artifactFormatZip:
		return collectIgnoredZip(reader, scope, tracker, inventory)
	case artifactFormatGzip, artifactFormatXz, artifactFormatBzip2:
		if err := tracker.enterArchive(); err != nil {
			return err
		}
		defer tracker.leaveArchive()
		var decoded io.Reader
		var err error
		decodedName := name
		decodedScope := scope
		switch format {
		case artifactFormatGzip:
			gzipReader, gzipErr := gzip.NewReader(reader)
			if gzipErr != nil {
				return gzipErr
			}
			gzipReader.Multistream(false)
			decoded = gzipReader
			if gzipReader.Name != "" {
				decodedName, err = normalizeArtifactMemberIdentity(gzipReader.Name)
				if err != nil {
					return err
				}
				entry, err := inventory.addScoped(scope, decodedName, artifactEntryIgnored)
				if err != nil {
					return err
				}
				decodedScope = entry.identity
			}
		case artifactFormatXz:
			decoded, err = xz.NewReader(reader, 0)
		case artifactFormatBzip2:
			decoded = bzip2.NewReader(reader)
		}
		if err != nil {
			return err
		}
		if err := discardIgnoredArtifact(&payloadBudgetReader{reader: decoded, tracker: tracker}, decodedName, decodedScope, tracker, inventory); err != nil {
			return err
		}
		if format == artifactFormatGzip {
			return validateAdditionalGzipMembers(reader, decodedName, tracker)
		}
		return nil
	default:
		return nil
	}
}

func collectIgnoredTar(r io.Reader, scope string, tracker *artifactBudgetTracker, inventory *artifactInventory) error {
	if err := tracker.enterArchive(); err != nil {
		return err
	}
	defer tracker.leaveArchive()
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tracker.visitArchiveEntry(); err != nil {
			return err
		}
		entry, err := inventory.addScoped(scope, h.Name, artifactEntryIgnored)
		if err != nil {
			return err
		}
		if err := discardIgnoredArtifact(&entryBudgetReader{reader: tr, tracker: tracker}, h.Name, entry.identity, tracker, inventory); err != nil {
			return err
		}
	}
}

func collectIgnoredZip(r io.Reader, scope string, tracker *artifactBudgetTracker, inventory *artifactInventory) error {
	if err := tracker.enterArchive(); err != nil {
		return err
	}
	defer tracker.leaveArchive()
	zr := zipstream.NewReader(r)
	for {
		h, err := zr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tracker.visitArchiveEntry(); err != nil {
			return err
		}
		entry, err := inventory.addScoped(scope, h.Name, artifactEntryIgnored)
		if err != nil {
			return err
		}
		if err := discardIgnoredArtifact(&entryBudgetReader{reader: zr, tracker: tracker}, h.Name, entry.identity, tracker, inventory); err != nil {
			return err
		}
	}
}

func isRecognizedArtifact(path, name string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	format := artifactFormatFor(name, header[:n])
	if err := validateArtifactFormat(format); err != nil {
		return false, err
	}
	if format != artifactFormatPlain {
		return true, nil
	}
	return false, nil
}

func stageReader(root string, r io.Reader, tracker *artifactBudgetTracker, expanded bool) (string, error) {
	f, err := os.CreateTemp(root, "entry-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	keep := false
	defer func() {
		_ = f.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	buf := make([]byte, 32*1024)
	var layerBytes int64
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			if expanded {
				if err := tracker.addEntryBytes(int64(n)); err != nil {
					return "", err
				}
			} else if tracker != nil {
				layerBytes += int64(n)
				if err := tracker.addPayloadLayerBytes(layerBytes, int64(n)); err != nil {
					return "", err
				}
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	keep = true
	return filepath.Clean(path), nil
}
