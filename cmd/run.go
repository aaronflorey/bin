package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

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

			_, file, err := fetchBinary(root.newProvider, resolved.url, root.opts.provider, resolved.fetchOpts)
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

			return executeCachedBinary(cmd, root.execCommand, cachePath, target.args)
		},
	}

	root.cmd = cmd
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
