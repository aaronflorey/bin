package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
	"github.com/spf13/cobra"
)

type runCmd struct {
	cmd          *cobra.Command
	opts         runOpts
	newProvider  providerFactory
	userCacheDir func() (string, error)
	execCommand  func(string, ...string) *exec.Cmd
}

type runOpts struct {
	provider       string
	all            bool
	autoSelect     string
	nonInteractive bool
}

type runTarget struct {
	url  string
	args []string
}

func newRunCmd() *runCmd {
	root := &runCmd{
		newProvider:  providers.New,
		userCacheDir: os.UserCacheDir,
		execCommand:  exec.Command,
	}
	cmd := &cobra.Command{
		Use:           "run <url> [-- args...]",
		Short:         "Downloads a binary into cache and runs it",
		Long:          "Downloads the resolved executable into the user cache and runs it without adding it to config.json. Cached binaries are stored under os.UserCacheDir()/bin (for example ~/.cache/bin on Linux). Remove individual cached files or clear that directory manually to clean up.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := parseRunTarget(args, cmd.ArgsLenAtDash())
			if err != nil {
				return err
			}

			resolved, err := resolveFetchRequest(target.url, root.opts.provider, providers.FetchOpts{
				All:            root.opts.all,
				AutoSelect:     root.opts.autoSelect,
				NonInteractive: root.opts.nonInteractive,
			})
			if err != nil {
				return err
			}

			versionKey, err := runVersionKey(root.newProvider, resolved.url, root.opts.provider, resolved.fetchOpts.Version)
			if err != nil {
				return err
			}
			if versionKey != "" {
				cachedPath, ok, err := lookupCachedRunBinary(root.userCacheDir, resolved.url, versionKey)
				if err != nil {
					return err
				}
				if ok {
					if err := pruneOldCachedRunBinaries(root.userCacheDir, resolved.url, versionKey, cachedPath); err != nil {
						return err
					}
					log.Infof("Reusing cached binary %s", cachedPath)
					return executeCachedBinary(cmd, root.execCommand, cachedPath, target.args)
				}
			}

			_, file, err := fetchBinary(root.newProvider, resolved.url, root.opts.provider, resolved.fetchOpts, false)
			if err != nil {
				return err
			}

			cachePath, err := runCachePath(root.userCacheDir, file.Name, file.Version)
			if err != nil {
				closeFetchedFile(file)
				return err
			}

			if err := ensureCachedBinary(file, cachePath); err != nil {
				return err
			}

			if file.Version != "" {
				if err := recordCachedRunBinary(root.userCacheDir, resolved.url, file.Version, cachePath); err != nil {
					return err
				}
				if err := pruneOldCachedRunBinaries(root.userCacheDir, resolved.url, file.Version, cachePath); err != nil {
					return err
				}
			}

			return executeCachedBinary(cmd, root.execCommand, cachePath, target.args)
		},
	}

	root.cmd = cmd
	root.cmd.Flags().SetInterspersed(false)
	enableSpinner(root.cmd)
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all possible download options (skip scoring & filtering)")
	root.cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Forces to use a specific provider")
	root.cmd.Flags().StringVarP(&root.opts.autoSelect, "select", "s", "", "Auto select installation file (skips interactive prompt)")
	root.cmd.Flags().BoolVar(&root.opts.nonInteractive, "non-interactive", false, "Disable all interactive prompts (auto-select best option)")
	return root
}

func parseRunTarget(args []string, argsLenAtDash int) (*runTarget, error) {
	urlArgs := args
	passthroughArgs := []string{}
	if argsLenAtDash >= 0 {
		urlArgs = args[:argsLenAtDash]
		passthroughArgs = args[argsLenAtDash:]
		if len(passthroughArgs) > 0 && passthroughArgs[0] == "--" {
			passthroughArgs = passthroughArgs[1:]
		}
	} else if len(args) >= 1 {
		urlArgs = args[:1]
		passthroughArgs = args[1:]
		if len(passthroughArgs) > 0 && passthroughArgs[0] == "--" {
			passthroughArgs = passthroughArgs[1:]
		}
	}

	if len(urlArgs) != 1 {
		return nil, fmt.Errorf("expected exactly one run target URL")
	}

	return &runTarget{url: urlArgs[0], args: passthroughArgs}, nil
}

func runCachePath(userCacheDir func() (string, error), name, version string) (string, error) {
	cacheDir, err := userCacheDir()
	if err != nil {
		return "", err
	}

	fileName := assets.SanitizeName(name, version)
	if fileName == "" {
		fileName = name
	}
	if version != "" {
		fileName = fmt.Sprintf("%s-%s", fileName, version)
	}

	return filepath.Join(cacheDir, "bin", fileName), nil
}

func ensureCachedBinary(file *providers.File, cachePath string) error {
	if info, err := os.Stat(cachePath); err == nil {
		if info.IsDir() {
			closeFetchedFile(file)
			return fmt.Errorf("cache path %s is a directory", cachePath)
		}
		closeFetchedFile(file)
		log.Infof("Reusing cached binary %s", cachePath)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		closeFetchedFile(file)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		closeFetchedFile(file)
		return err
	}

	_, err := saveToDisk(file, cachePath, false)
	if err != nil {
		return fmt.Errorf("error caching binary: %w", err)
	}

	return nil
}

func closeFetchedFile(file *providers.File) {
	if file == nil {
		return
	}
	closer, ok := file.Data.(io.Closer)
	if !ok {
		return
	}
	if err := closer.Close(); err != nil {
		log.Debugf("Error closing fetched binary stream: %v", err)
	}
}

func runVersionKey(newProvider providerFactory, normalizedURL, forcedProvider, requestedVersion string) (string, error) {
	if requestedVersion != "" {
		return requestedVersion, nil
	}

	p, err := newProvider(normalizedURL, forcedProvider)
	if err != nil {
		return "", err
	}

	release, err := p.GetLatestVersion()
	if err != nil {
		return "", err
	}
	if release == nil {
		return "", nil
	}

	return release.Version, nil
}

func runCacheIndexPath(userCacheDir func() (string, error)) (string, error) {
	cacheDir, err := userCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(cacheDir, "bin", "run-index.json"), nil
}

func runCacheKey(url, version string) string {
	return fmt.Sprintf("%s|%s", url, version)
}

func runCacheKeyPrefix(url string) string {
	return url + "|"
}

func loadRunCacheIndex(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	out := map[string]string{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func saveRunCacheIndex(path string, index map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(index, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, raw, 0o644)
}

func lookupCachedRunBinary(userCacheDir func() (string, error), normalizedURL, version string) (string, bool, error) {
	indexPath, err := runCacheIndexPath(userCacheDir)
	if err != nil {
		return "", false, err
	}

	index, err := loadRunCacheIndex(indexPath)
	if err != nil {
		return "", false, err
	}

	path, ok := index[runCacheKey(normalizedURL, version)]
	if !ok {
		return "", false, nil
	}

	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, true, nil
	}

	delete(index, runCacheKey(normalizedURL, version))
	if err := saveRunCacheIndex(indexPath, index); err != nil {
		return "", false, err
	}

	return "", false, nil
}

func recordCachedRunBinary(userCacheDir func() (string, error), normalizedURL, version, path string) error {
	indexPath, err := runCacheIndexPath(userCacheDir)
	if err != nil {
		return err
	}

	index, err := loadRunCacheIndex(indexPath)
	if err != nil {
		return err
	}

	index[runCacheKey(normalizedURL, version)] = path
	return saveRunCacheIndex(indexPath, index)
}

func pruneOldCachedRunBinaries(userCacheDir func() (string, error), normalizedURL, currentVersion, currentPath string) error {
	indexPath, err := runCacheIndexPath(userCacheDir)
	if err != nil {
		return err
	}

	index, err := loadRunCacheIndex(indexPath)
	if err != nil {
		return err
	}

	prefix := runCacheKeyPrefix(normalizedURL)
	pathsToMaybeDelete := []string{}
	changed := false
	for key, path := range index {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if key == runCacheKey(normalizedURL, currentVersion) {
			continue
		}
		pathsToMaybeDelete = append(pathsToMaybeDelete, path)
		delete(index, key)
		changed = true
	}

	if !changed {
		return nil
	}

	if err := saveRunCacheIndex(indexPath, index); err != nil {
		return err
	}

	referencedPaths := map[string]struct{}{}
	for _, path := range index {
		referencedPaths[path] = struct{}{}
	}

	for _, path := range pathsToMaybeDelete {
		if path == "" || path == currentPath {
			continue
		}
		if _, stillReferenced := referencedPaths[path]; stillReferenced {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func executeCachedBinary(cmd *cobra.Command, execCommand func(string, ...string) *exec.Cmd, path string, args []string) error {
	child := execCommand(path, args...)
	child.Stdin = cmd.InOrStdin()
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()

	if err := child.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return wrapErrorWithCode(err, exitErr.ExitCode(), "")
		}
		return err
	}

	return nil
}
