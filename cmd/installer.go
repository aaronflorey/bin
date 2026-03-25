package cmd

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
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

	resolvedPath := opts.Path
	if opts.ResolvePath {
		resolvedPath, err = checkFinalPath(resolvedPath, assets.SanitizeName(pResult.Name, pResult.Version))
		if err != nil {
			return nil, err
		}
	}

	hash, err := saveToDisk(pResult, resolvedPath, opts.Force)
	if err != nil {
		return nil, fmt.Errorf("error installing binary: %w", err)
	}

	var configPath string
	if len(opts.ConfigPath) > 0 {
		configPath = opts.ConfigPath
	} else {
		configPath, err = filepath.Abs(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("error converting to absolute path: %w", err)
		}
	}

	pinned := opts.Pinned
	if existing, ok := config.Get().Bins[configPath]; ok {
		pinned = pinned || existing.Pinned
	}

	err = config.UpsertBinary(&config.Binary{
		RemoteName:  pResult.Name,
		Path:        configPath,
		Version:     pResult.Version,
		Hash:        fmt.Sprintf("%x", hash),
		URL:         opts.URL,
		Provider:    p.GetID(),
		PackagePath: pResult.PackagePath,
		Pinned:      pinned,
	})
	if err != nil {
		return nil, err
	}

	return &InstallResult{
		Name:    pResult.Name,
		Version: pResult.Version,
		Path:    configPath,
	}, nil
}

// checkFinalPath checks if path exists and if it's a dir or not
// and returns the correct final file path. It also
// checks if the path already exists and prompts
// the user to override
func checkFinalPath(path, fileName string) (string, error) {
	fi, err := os.Stat(os.ExpandEnv(path))

	// TODO implement file existence and override logic
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if fi != nil && fi.IsDir() {
		return filepath.Join(path, fileName), nil
	}

	return path, nil
}

// saveToDisk saves the specified binary to the desired path
// and makes it executable. It also checks if any other binary
// has the same hash and exists if so.

// TODO check if other binary has the same hash and warn about it.
// TODO if the file is zipped, tared, whatever then extract it
func saveToDisk(f *providers.File, path string, overwrite bool) ([]byte, error) {
	epath := os.ExpandEnv(path)

	extraFlags := os.O_EXCL

	if overwrite {
		extraFlags = 0
		err := os.Remove(epath)
		log.Debugf("Overwrite flag set, removing file %s\n", epath)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	file, err := os.OpenFile(epath, os.O_RDWR|os.O_CREATE|extraFlags, 0o766)
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

	return h.Sum(nil), nil
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
