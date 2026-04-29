package cmd

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
var installProviderFactory = providers.New

// applyChmod applies the DefaultChmod setting from config, if set.
// This is a no-op on non-Linux platforms where DefaultChmod is not set by default.
func applyChmod(file *os.File) error {
	defaultChmod := config.Get().DefaultChmod
	if len(defaultChmod) == 0 {
		return nil
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
}

// InstallResult holds the outcome of a successful installation.
type InstallResult struct {
	Name    string
	Version string
	Path    string
}

// installBinary fetches a binary from a provider, saves it to disk,
// and updates the config.
func installBinary(opts InstallOpts) (*InstallResult, error) {
	p, pResult, err := fetchBinary(installProviderFactory, opts.URL, opts.Provider, opts.FetchOpts, opts.AllowProviderFallback)
	if err != nil {
		return nil, err
	}

	existing, _ := existingConfigBinary(opts)

	minAgeDays := 0
	if existing != nil {
		minAgeDays = existing.MinAgeDays
	}
	if opts.MinAgeDays != nil {
		minAgeDays = *opts.MinAgeDays
	}
	if err := ensureReleaseAge(p.GetID(), pResult.Version, pResult.PublishedAt, minAgeDays); err != nil {
		return nil, err
	}

	resolvedPath := opts.Path
	overwrite := opts.Force
	if opts.ResolvePath {
		resolvedPath, overwrite, err = checkFinalPath(resolvedPath, assets.SanitizeName(pResult.Name, pResult.Version), overwrite)
		if err != nil {
			return nil, err
		}
	}

	hash, err := saveToDisk(pResult, resolvedPath, overwrite)
	if err != nil {
		return nil, fmt.Errorf("error installing binary: %w", err)
	}
	hashString := fmt.Sprintf("%x", hash)

	var configPath string
	if len(opts.ConfigPath) > 0 {
		configPath = opts.ConfigPath
	} else {
		configPath, err = absExpandedPath(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("error converting to absolute path: %w", err)
		}
	}

	pinned := opts.Pinned
	if existing != nil {
		pinned = pinned || existing.Pinned
	}

	err = config.UpsertBinary(&config.Binary{
		RemoteName:  pResult.Name,
		Path:        configPath,
		Version:     pResult.Version,
		Hash:        hashString,
		URL:         opts.URL,
		Provider:    p.GetID(),
		InstallMode: installModeBinary,
		PackageType: "",
		AppBundle:   "",
		PackagePath: pResult.PackagePath,
		Pinned:      pinned,
		MinAgeDays:  minAgeDays,
	})
	if err != nil {
		return nil, err
	}

	warnDuplicateManagedHash(configPath, hashString)

	return &InstallResult{
		Name:    pResult.Name,
		Version: pResult.Version,
		Path:    configPath,
	}, nil
}

func fetchBinary(newProvider providerFactory, url, forcedProvider string, fetchOpts providers.FetchOpts, allowProviderFallback bool) (providers.Provider, *providers.File, error) {
	p, err := newProvider(url, forcedProvider)
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("Using provider '%s' for '%s'", p.GetID(), url)

	pResult, err := p.Fetch(&fetchOpts)
	if err == nil {
		return p, pResult, nil
	}

	if !allowProviderFallback || strings.TrimSpace(forcedProvider) == "" || !shouldFallbackProviderFetch(err) {
		return nil, nil, err
	}

	log.Warnf("Provider %q did not yield a compatible asset for %s, retrying with auto-detection", forcedProvider, url)
	fallbackProvider, fallbackErr := newProvider(url, "")
	if fallbackErr != nil {
		return nil, nil, err
	}
	log.Debugf("Using fallback provider '%s' for '%s'", fallbackProvider.GetID(), url)

	fallbackResult, fallbackFetchErr := fallbackProvider.Fetch(&fetchOpts)
	if fallbackFetchErr != nil {
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

func absExpandedPath(path string) (string, error) {
	return filepath.Abs(os.ExpandEnv(path))
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
	fi, err := os.Stat(os.ExpandEnv(path))
	if err != nil && !os.IsNotExist(err) {
		return "", overwrite, err
	}

	finalPath := path

	if fi != nil && fi.IsDir() {
		finalPath = filepath.Join(path, fileName)
	}

	if _, err := os.Stat(os.ExpandEnv(finalPath)); err == nil {
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
	epath := os.ExpandEnv(path)
	if closer, ok := f.Data.(io.Closer); ok {
		defer closer.Close()
	}

	extraFlags := os.O_EXCL

	if overwrite {
		extraFlags = 0
		err := os.Remove(epath)
		log.Debugf("Overwrite flag set, removing file %s\n", epath)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	file, err := os.OpenFile(epath, os.O_RDWR|os.O_CREATE|extraFlags, 0o755)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	h := sha256.New()

	tr := io.TeeReader(f.Data, h)

	log.Infof("Copying for %s@%s into %s", f.Name, f.Version, epath)
	_, err = io.Copy(file, tr)
	if err != nil {
		return nil, err
	}

	if err := applyChmod(file); err != nil {
		return nil, err
	}

	actualHash := fmt.Sprintf("%x", h.Sum(nil))
	if f.ExpectedSHA != "" && !strings.EqualFold(actualHash, f.ExpectedSHA) {
		return nil, fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", f.Name, f.ExpectedSHA, actualHash)
	}

	return h.Sum(nil), nil
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
