package cmd

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
	"github.com/caarlos0/log"
)

var isPromptInteractive = prompt.IsInteractive
var confirmPrompt = prompt.Confirm
var confirmDefaultNoPrompt = prompt.ConfirmDefaultNo
var installProviderFactory = newProviderWithPolicy
var commitBinaryTransaction = config.CommitBinaryTransaction

// applyChmod applies the configured mode for installed binaries.
// When unset, direct binary installs default to 0755 on non-Windows systems.
func applyChmod(file *os.File) error {
	defaultChmod := config.Get().DefaultChmod
	if len(defaultChmod) == 0 {
		if runtime.GOOS == "windows" {
			return nil
		}
		defaultChmod = "0755"
	}

	var chmodVal int64
	if _, err := fmt.Sscanf(defaultChmod, "%o", &chmodVal); err != nil {
		log.Warnf("Could not parse default_chmod value '%s', skipping chmod", defaultChmod)
		return nil
	}

	return file.Chmod(os.FileMode(chmodVal))
}

// InstallOpts captures all parameters needed to fetch, save, and
// record a binary in the config.
type InstallOpts struct {
	// URL is the provider URL (e.g. github repo release URL).
	URL string

	// Provider forces a specific provider (e.g. "github", "gitlab").
	// Empty string means auto-detect.
	Provider string

	// Path is the destination file path. When ResolvePath is true and
	// this is a directory, the binary name will be appended.
	Path string

	// Force overwrites existing files without prompting.
	Force bool

	// FetchOpts are passed directly to the provider's Fetch method.
	// Callers can pre-fill PackagePath, PackageName, Version, etc.
	FetchOpts providers.FetchOpts

	// ResolvePath controls whether checkFinalPath is called to resolve
	// directory paths and sanitize filenames. Set to false when the
	// path is already fully resolved (update, ensure).
	ResolvePath bool

	// ConfigPath, if set, is stored in config instead of the resolved
	// path. Used by ensure where the config stores unexpanded env vars.
	ConfigPath string

	// Pinned marks this binary as pinned in config after installation.
	Pinned bool

	// MinAgeDays, when set, persists the minimum allowed release age
	// for this binary in config.
	MinAgeDays *int

	// AllowProviderFallback retries with provider auto-detection when a stored
	// provider no longer yields a compatible release asset.
	AllowProviderFallback bool

	// LogicalName is the stable command name. It is deliberately distinct from
	// the provider's versioned source asset.
	LogicalName string
}

// InstallResult holds the outcome of a successful installation.
type InstallResult struct {
	Name    string
	Version string
	Path    string
}

// installBinary fetches a binary from a provider, saves it to disk,
// and updates the config.
func installBinary(opts InstallOpts) (result *InstallResult, err error) {
	log.Debugf("Installing %q with provider=%q path=%q resolvePath=%t", opts.URL, opts.Provider, opts.Path, opts.ResolvePath)

	p, pResult, err := fetchBinary(installProviderFactory, opts.URL, opts.Provider, opts.FetchOpts, opts.AllowProviderFallback)
	if err != nil {
		return nil, err
	}
	inputClosed := false
	defer func() {
		if inputClosed {
			return
		}
		if closeErr := closeFetchedFile(pResult); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	log.Debugf("Fetched %s version %s from provider %q", pResult.Name, pResult.Version, p.GetID())

	_, minAgeDays, pinned := resolveInstallState(opts)
	if err := ensureReleaseAge(p.GetID(), pResult.Version, pResult.PublishedAt, minAgeDays); err != nil {
		return nil, err
	}
	if err := assets.ValidatePortableName(pResult.Name); err != nil {
		return nil, fmt.Errorf("invalid provider executable name %q: %w", pResult.Name, err)
	}

	logicalName := opts.LogicalName
	if logicalName == "" {
		logicalName = assets.SanitizeName(pResult.Name, pResult.Version)
	}
	if logicalName == "" {
		logicalName = pResult.Name
	}
	if opts.LogicalName == "" {
		if err := assets.ValidatePortableName(logicalName); err != nil {
			return nil, fmt.Errorf("invalid provider executable name %q: %w", logicalName, err)
		}
	}

	var resolvedPath string
	if opts.ConfigPath != "" {
		resolvedPath = expandTrackedBinaryPath(opts.Path)
	} else {
		resolvedPath = os.ExpandEnv(opts.Path)
	}
	overwrite := opts.Force
	if opts.ResolvePath {
		resolvedPath, overwrite, err = checkFinalPath(resolvedPath, logicalName, overwrite)
		if err != nil {
			return nil, err
		}
	}
	log.Debugf("Resolved final install path to %q (overwrite=%t)", resolvedPath, overwrite)

	candidate, err := prepareStagedBinary(pResult, resolvedPath, true)
	inputClosed = true
	if err != nil {
		return nil, fmt.Errorf("error installing binary: %w", err)
	}
	defer candidate.cleanup()
	hashString := fmt.Sprintf("%x", candidate.hash)

	configPath, err := resolveTrackedConfigPath(opts, resolvedPath)
	if err != nil {
		return nil, err
	}

	installed := &config.Binary{
		RemoteName:         logicalName,
		Path:               configPath,
		Version:            pResult.Version,
		Hash:               hashString,
		URL:                opts.URL,
		Provider:           p.GetID(),
		InstallMode:        installModeBinary,
		PackageType:        "",
		AppBundle:          "",
		PackagePath:        pResult.PackagePath,
		SourceAsset:        pResult.SourceAsset,
		ReleaseTagPrefix:   pResult.ReleaseTagPrefix,
		DownloadIntegrity:  configIntegrityRecord(pResult.DownloadIntegrity),
		InstalledIntegrity: installedIntegrityRecord(pResult.InstalledIntegrity, hashString),
		Pinned:             pinned,
		MinAgeDays:         minAgeDays,
	}
	publication := stagedBinaryPublication{candidate: candidate.path, destination: resolvedPath, overwrite: overwrite}
	transactionID := fmt.Sprintf("%x", sha256.Sum256([]byte(candidate.path)))
	err = commitBinaryTransaction(config.BinaryTransaction{
		ID:                      transactionID,
		Intended:                installed,
		DestinationPath:         resolvedPath,
		ReserveRollbackArtifact: publication.reserveBackup,
		Publish:                 func(*config.Binary) error { return publication.publish() },
		Rollback:                func(*config.Binary) error { return publication.rollback() },
		Cleanup:                 publication.cleanup,
	})
	if err != nil {
		return nil, err
	}
	warnDuplicateManagedHash(installed.Path, installed.Hash)
	log.Debugf("Saved installed binary config for %q at %s", logicalName, configPath)

	return &InstallResult{
		Name:    logicalName,
		Version: pResult.Version,
		Path:    configPath,
	}, nil
}

func configIntegrityRecord(record *providers.IntegrityRecord) *config.IntegrityRecord {
	if record == nil {
		return nil
	}

	return &config.IntegrityRecord{
		Algorithm: record.Algorithm,
		Expected:  record.Expected,
		Observed:  record.Observed,
		Source:    record.Source,
		Scope:     record.Scope,
		Result:    record.Result,
	}
}

func installedIntegrityRecord(record *providers.IntegrityRecord, installedHash string) *config.IntegrityRecord {
	if record == nil || !strings.EqualFold(record.Observed, installedHash) {
		return nil
	}
	return configIntegrityRecord(record)
}

func fetchBinary(newProvider providerFactory, url, forcedProvider string, fetchOpts providers.FetchOpts, allowProviderFallback bool) (providers.Provider, *providers.File, error) {
	p, err := newProvider(url, forcedProvider)
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("Using provider '%s' for '%s'", p.GetID(), url)
	log.Debugf("Fetch options for %q: version=%q all=%t system-package=%t package-type=%q package-name=%q package-path=%q", url, fetchOpts.Version, fetchOpts.All, fetchOpts.SystemPackage, fetchOpts.PackageType, fetchOpts.PackageName, fetchOpts.PackagePath)

	pResult, err := p.Fetch(&fetchOpts)
	if err == nil {
		return p, pResult, nil
	}
	log.WithError(err).Debugf("Provider '%s' fetch failed for '%s'", p.GetID(), url)

	if !allowProviderFallback || strings.TrimSpace(forcedProvider) == "" || !shouldFallbackProviderFetch(err) {
		return nil, nil, err
	}

	fallbackProvider, fallbackErr := newProvider(url, "")
	if fallbackErr != nil {
		return nil, nil, err
	}
	if fallbackProvider.GetID() == p.GetID() {
		return nil, nil, err
	}
	log.Warnf("Provider %q did not yield a compatible asset for %s, retrying with auto-detection", forcedProvider, url)
	log.Debugf("Using fallback provider '%s' for '%s'", fallbackProvider.GetID(), url)

	fallbackResult, fallbackFetchErr := fallbackProvider.Fetch(&fetchOpts)
	if fallbackFetchErr != nil {
		log.WithError(fallbackFetchErr).Debugf("Fallback provider '%s' fetch failed for '%s'", fallbackProvider.GetID(), url)
		return nil, nil, err
	}

	return fallbackProvider, fallbackResult, nil
}

func shouldFallbackProviderFetch(err error) bool {
	return isCompatibilityError(err)
}

func isCompatibilityError(err error) bool {
	return err != nil && (errors.Is(err, assets.ErrNoCompatibleFiles) || errors.Is(err, systempackage.ErrIncompatible))
}

func existingConfigBinary(opts InstallOpts) (*config.Binary, bool) {
	if len(opts.ConfigPath) > 0 {
		b, ok := config.Get().Bins[opts.ConfigPath]
		return b, ok
	}

	absPath, err := absExpandedPath(opts.Path)
	if err != nil {
		return nil, false
	}

	b, ok := config.Get().Bins[absPath]
	return b, ok
}

func resolveInstallState(opts InstallOpts) (*config.Binary, int, bool) {
	existing, _ := existingConfigBinary(opts)

	minAgeDays := 0
	pinned := opts.Pinned
	if existing != nil {
		minAgeDays = existing.MinAgeDays
		pinned = pinned || existing.Pinned
	}
	if opts.MinAgeDays != nil {
		minAgeDays = *opts.MinAgeDays
	}

	return existing, minAgeDays, pinned
}

func resolveTrackedConfigPath(opts InstallOpts, resolvedPath string) (string, error) {
	if len(opts.ConfigPath) > 0 {
		return opts.ConfigPath, nil
	}

	configPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("error converting to absolute path: %w", err)
	}
	return configPath, nil
}

func persistInstalledBinary(bin *config.Binary) error {
	if err := config.UpsertBinary(bin); err != nil {
		return err
	}

	warnDuplicateManagedHash(bin.Path, bin.Hash)
	return nil
}

func absExpandedPath(path string) (string, error) {
	return filepath.Abs(os.ExpandEnv(path))
}

// expandTrackedBinaryPath expands only the trusted directory portion of a
// persisted path. The final filename is provider-derived or user-chosen and
// must remain literal.
func expandTrackedBinaryPath(path string) string {
	dir, name := filepath.Split(path)
	return os.ExpandEnv(dir) + name
}

func ensureReleaseAge(providerID, version string, publishedAt *time.Time, minAgeDays int) error {
	if minAgeDays == 0 {
		return nil
	}
	if publishedAt == nil {
		return providers.ReleaseAgeError(providerID, version)
	}

	minAllowedTime := time.Now().AddDate(0, 0, -minAgeDays)
	if publishedAt.After(minAllowedTime) {
		return fmt.Errorf(
			"release %s from provider %q is only %d days old; requires at least %d days",
			version,
			providerID,
			int(time.Since(*publishedAt).Hours()/24),
			minAgeDays,
		)
	}

	return nil
}

// checkFinalPath checks if path exists and if it's a dir or not
// and returns the correct final file path. It also
// checks if the path already exists and prompts
// the user to override
func checkFinalPath(path, fileName string, overwrite bool) (string, bool, error) {
	fi, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return "", overwrite, err
	}

	finalPath := path

	if fi != nil && fi.IsDir() {
		finalPath = filepath.Join(path, fileName)
	}

	if _, err := os.Stat(finalPath); err == nil {
		if overwrite {
			return finalPath, true, nil
		}

		if !prompt.IsInteractive() {
			return "", overwrite, fmt.Errorf("path %s already exists, use --force to overwrite", finalPath)
		}

		if err := prompt.Confirm(fmt.Sprintf("Path %s already exists. Overwrite?", finalPath)); err != nil {
			return "", overwrite, err
		}

		overwrite = true
	}

	return finalPath, overwrite, nil
}

// saveToDisk saves the specified binary to the desired path
// and makes it executable. It also checks if any other binary
// has the same hash and exists if so.
func saveToDisk(f *providers.File, path string, overwrite bool) ([]byte, error) {
	return saveToDiskWithInputClose(f, path, overwrite, false)
}

// saveToDiskAndCloseInput prepares and publishes a direct-binary candidate.
// Its input must be closed before publication so a close failure preserves the
// existing destination.
func saveToDiskAndCloseInput(f *providers.File, path string, overwrite bool) ([]byte, error) {
	return saveToDiskWithInputClose(f, path, overwrite, true)
}

func saveToDiskWithInputClose(f *providers.File, path string, overwrite, closeInput bool) (hash []byte, err error) {
	candidate, err := prepareStagedBinary(f, path, closeInput)
	if err != nil {
		return nil, err
	}
	defer candidate.cleanup()
	if err := publishStagedBinary(candidate.path, path, overwrite); err != nil {
		return nil, err
	}
	return candidate.hash, nil
}

type stagedBinaryCandidate struct {
	path string
	hash []byte
}

func (candidate *stagedBinaryCandidate) cleanup() {
	if candidate != nil && candidate.path != "" {
		_ = os.Remove(candidate.path)
	}
}

func prepareStagedBinary(f *providers.File, path string, closeInput bool) (candidate *stagedBinaryCandidate, err error) {
	inputClosed := false
	defer func() {
		if !closeInput || inputClosed {
			return
		}
		if closeErr := closeFetchedFile(f); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()

	defer func() {
		if file != nil {
			_ = file.Close()
		}
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	h := sha256.New()

	tr := io.TeeReader(f.Data, h)

	log.Infof("Copying for %s@%s into %s", f.Name, f.Version, path)
	_, copyErr := io.Copy(file, tr)
	var inputCloseErr error
	if closeInput {
		inputCloseErr = closeFetchedFile(f)
		inputClosed = true
	}
	if copyErr != nil || inputCloseErr != nil {
		return nil, errors.Join(copyErr, inputCloseErr)
	}

	actualHash := fmt.Sprintf("%x", h.Sum(nil))
	if f.ExpectedSHA != "" && !strings.EqualFold(actualHash, f.ExpectedSHA) {
		return nil, fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", f.Name, f.ExpectedSHA, actualHash)
	}

	closeErr := file.Close()
	file = nil
	if closeErr != nil {
		return nil, closeErr
	}

	if err := assets.ValidateRunnablePayload(tempPath, f.Name); err != nil {
		return nil, err
	}

	// Only assign executable permissions after payload validation. A failed
	// validation leaves both the destination and config untouched.
	if err := applyChmodToPath(tempPath); err != nil {
		return nil, err
	}

	return &stagedBinaryCandidate{path: tempPath, hash: h.Sum(nil)}, nil
}

type stagedBinaryPublication struct {
	candidate   string
	destination string
	overwrite   bool
	backup      string
	backedUp    bool
	published   bool
}

func (publication *stagedBinaryPublication) reserveBackup() (string, error) {
	backup, err := os.CreateTemp(filepath.Dir(publication.destination), filepath.Base(publication.destination)+".rollback-*")
	if err != nil {
		return "", err
	}
	publication.backup = backup.Name()
	if err := backup.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(publication.backup); err != nil {
		return "", err
	}
	return publication.backup, nil
}

func (publication *stagedBinaryPublication) publish() error {
	if err := rejectDestinationSymlink(publication.destination); err != nil {
		return err
	}
	if err := publication.backupDestination(); err != nil {
		return err
	}
	if err := publishStagedBinary(publication.candidate, publication.destination, false); err != nil {
		return errors.Join(err, publication.restoreBackup())
	}
	publication.published = true
	return nil
}

func (publication *stagedBinaryPublication) backupDestination() error {
	info, err := os.Lstat(publication.destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("refusing to replace directory destination %s", publication.destination)
	}
	if !publication.overwrite {
		return fmt.Errorf("path %s already exists, use --force to overwrite", publication.destination)
	}
	if publication.backup == "" {
		return errors.New("rollback artifact was not reserved")
	}
	if err := os.Rename(publication.destination, publication.backup); err != nil {
		return err
	}
	publication.backedUp = true
	return nil
}

func (publication *stagedBinaryPublication) rollback() error {
	if !publication.published {
		if !publication.backedUp {
			return publication.cleanup()
		}
		return publication.restoreBackup()
	}
	if err := os.Remove(publication.destination); err != nil {
		return err
	}
	publication.published = false
	return publication.restoreBackup()
}

func (publication *stagedBinaryPublication) restoreBackup() error {
	if publication.backup == "" {
		return nil
	}
	if err := os.Rename(publication.backup, publication.destination); err != nil {
		return err
	}
	publication.backup = ""
	publication.backedUp = false
	return nil
}

func (publication *stagedBinaryPublication) cleanup() error {
	if publication.backup == "" {
		return nil
	}
	err := os.Remove(publication.backup)
	if err == nil || os.IsNotExist(err) {
		publication.backup = ""
		return nil
	}
	return err
}

func applyChmodToPath(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	chmodErr := applyChmod(file)
	closeErr := file.Close()
	return errors.Join(chmodErr, closeErr)
}

func warnDuplicateManagedHash(installedPath, hash string) {
	if duplicatePath, ok := findManagedDuplicateByHash(config.Get().Bins, installedPath, hash); ok {
		log.Warnf("binary %s has the same hash as managed binary %s", installedPath, duplicatePath)
	}
}

func findManagedDuplicateByHash(bins map[string]*config.Binary, installedPath, hash string) (string, bool) {
	for path, b := range bins {
		if path == installedPath {
			continue
		}
		if b.Hash != hash {
			continue
		}
		return path, true
	}

	return "", false
}

// resolveBinsToProcess returns the set of binaries to operate on,
// either filtered by the provided args or all configured binaries.
func resolveBinsToProcess(allBins map[string]*config.Binary, args []string) (map[string]*config.Binary, error) {
	if len(args) == 0 {
		return allBins, nil
	}

	bins := map[string]*config.Binary{}
	for _, a := range args {
		bin, err := getBinPath(a)
		if err != nil {
			if !errors.Is(err, exec.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}

			suggestedBin, suggestErr := resolveManagedBinSuggestion(allBins, a)
			if suggestErr != nil {
				return nil, suggestErr
			}
			if suggestedBin == "" {
				return nil, err
			}

			bin = suggestedBin
		}
		binCfg, ok := allBins[bin]
		if !ok {
			bin = findManagedBinByAlias(allBins, a)
			if bin != "" {
				binCfg, ok = allBins[bin]
			}
		}
		if !ok {
			return nil, fmt.Errorf("binary %q not found in configuration", a)
		}
		bins[bin] = binCfg
	}
	return bins, nil
}

func findManagedBinByAlias(allBins map[string]*config.Binary, input string) string {
	target := strings.ToLower(strings.TrimSpace(input))
	if target == "" {
		return ""
	}

	for path, bin := range allBins {
		if bin == nil {
			continue
		}
		if strings.EqualFold(bin.RemoteName, input) {
			return path
		}
		if strings.EqualFold(strings.TrimSuffix(bin.AppBundle, ".app"), input) {
			return path
		}
	}

	return ""
}

func resolveManagedBinSuggestion(allBins map[string]*config.Binary, input string) (string, error) {
	if strings.Contains(input, "/") {
		return "", nil
	}

	target := strings.ToLower(input)
	type candidate struct {
		name string
		path string
	}

	var candidates []candidate
	for path, bin := range allBins {
		name := filepath.Base(path)
		if bin != nil && bin.Path != "" {
			name = filepath.Base(bin.Path)
		}
		if bin != nil && strings.EqualFold(strings.TrimSuffix(bin.AppBundle, ".app"), input) {
			candidates = append(candidates, candidate{name: strings.TrimSuffix(bin.AppBundle, ".app"), path: path})
			continue
		}
		if bin != nil && strings.HasPrefix(strings.ToLower(bin.RemoteName), target) {
			candidates = append(candidates, candidate{name: bin.RemoteName, path: path})
			continue
		}

		if !strings.HasPrefix(strings.ToLower(name), target) {
			continue
		}

		candidates = append(candidates, candidate{name: name, path: path})
	}

	if len(candidates) == 0 {
		return "", nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].name != candidates[j].name {
			return candidates[i].name < candidates[j].name
		}
		return candidates[i].path < candidates[j].path
	})

	if len(candidates) == 1 {
		suggested := candidates[0]
		if isPromptInteractive() {
			if err := confirmPrompt(fmt.Sprintf("Did you mean %q?", suggested.name)); err != nil {
				return "", err
			}
			return suggested.path, nil
		}

		return "", fmt.Errorf("%w: binary %q not found in configuration; did you mean %q?", exec.ErrNotFound, input, suggested.name)
	}

	matches := make([]string, 0, len(candidates))
	for _, c := range candidates {
		matches = append(matches, c.name)
	}

	return "", fmt.Errorf("%w: binary %q not found in configuration; multiple matches: %s", exec.ErrNotFound, input, strings.Join(matches, ", "))
}

// hashFile computes the hex-encoded SHA256 hash of the file at path.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
