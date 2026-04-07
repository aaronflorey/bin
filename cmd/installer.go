package cmd

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
)

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
	p, err := providers.New(opts.URL, opts.Provider)
	if err != nil {
		return nil, err
	}
	log.Debugf("Using provider '%s' for '%s'", p.GetID(), opts.URL)

	pResult, err := p.Fetch(&opts.FetchOpts)
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
			return nil, err
		}
		binCfg, ok := allBins[bin]
		if !ok {
			return nil, fmt.Errorf("binary %q not found in configuration", a)
		}
		bins[bin] = binCfg
	}
	return bins, nil
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
