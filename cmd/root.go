package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/spinner"
	"github.com/caarlos0/log"
	"github.com/charmbracelet/colorprofile"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func Execute(version string, exit func(int), args []string) {
	// enable colored output on travis
	if os.Getenv("CI") != "" {
		color.NoColor = false
	}

	// fmt.Println()
	// defer fmt.Println()
	newRootCmd(version, exit).Execute(args)
}

func (cmd *rootCmd) Execute(args []string) {
	cmd.args = append([]string(nil), args...)
	cmd.cmd.SetArgs(args)

	previousLogger := log.Log
	defer func() {
		log.Log = previousLogger
	}()
	defer func() {
		if cmd.logWriter != nil {
			_ = cmd.logWriter.Close()
			cmd.logWriter = nil
		}
	}()

	defer spinner.Stop()

	if defaultCommand(cmd.cmd, args) {
		if len(args) == 0 {
			cmd.cmd.SetArgs(append([]string{"list"}, args...))
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: bin %s\n", args[0])
			cmd.exit(1)
			return
		}
	}

	if err := cmd.cmd.Execute(); err != nil {
		code := 1
		msg := "command failed"
		if eerr, ok := err.(*exitError); ok {
			code = eerr.code
			if eerr.details != "" {
				msg = eerr.details
			}
		}
		log.WithError(err).Error(msg)
		cmd.exit(code)
	}
}

type rootCmd struct {
	cmd               *cobra.Command
	logFile           string
	verbose           bool
	allowInsecureHTTP bool
	exit              func(int)
	args              []string

	logWriter   io.WriteCloser
	openLogFile func(string) (io.WriteCloser, error)
}

func newRootCmd(version string, exit func(int)) *rootCmd {
	root := &rootCmd{
		exit:        exit,
		openLogFile: openRootLogFile,
	}
	cmd := &cobra.Command{
		Use:           "bin",
		Short:         "Effortless binary manager",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			allowInsecureHTTP = root.allowInsecureHTTP

			// inspect is read-only: it must not create, load, or mutate
			// configuration, default paths, caches, log files, or install state.
			// Handle it before configureLogger so --log-file cannot create a
			// durable log file.
			if cmd.Name() == "inspect" {
				if strings.TrimSpace(root.logFile) != "" {
					return fmt.Errorf("inspect is read-only and cannot write --log-file")
				}
				if root.verbose {
					log.SetLevel(log.DebugLevel)
				}
				return nil
			}

			if err := root.configureLogger(); err != nil {
				return err
			}

			if root.verbose {
				log.SetLevel(log.DebugLevel)
				log.Debugf("verbose logs enabled, version: %s", version)
			}

			// version is read-only: it must not create, load, or mutate config,
			// default paths, caches, or install state.
			if cmd.Name() == "version" {
				return nil
			}

			// check and load config after handlers are configured
			err := config.CheckAndLoad()
			if err != nil {
				return fmt.Errorf("error loading config file: %w", err)
			}

			spinnerCmd := resolveSpinnerCommand(cmd, root.args)
			if shouldShowSpinner(spinnerCmd) && !root.verbose {
				cmd.SetOut(spinner.Writer(cmd.OutOrStdout()))
				cmd.SetErr(spinner.Writer(cmd.ErrOrStderr()))
				log.Log = newSpinnerLogger(root.logWriter)
				spinner.Start("")
			} else if shouldShowSpinner(spinnerCmd) {
				log.Debugf("Skipping spinner for %s because verbose logging is enabled", spinnerCmd.Name())
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&root.logFile, "log-file", "", "Write logs to the specified file")
	cmd.PersistentFlags().BoolVar(&root.verbose, "verbose", false, "Enable verbose logging")
	cmd.PersistentFlags().BoolVar(&root.verbose, "debug", false, "Enable verbose logging")
	cmd.PersistentFlags().BoolVar(&root.allowInsecureHTTP, "allow-insecure-http", false, "Allow generic http:// downloads")
	cmd.AddCommand(
		newInstallCmd().cmd,
		newRunCmd().cmd,
		newEnsureCmd().cmd,
		newOutdatedCmd().cmd,
		newUpdateCmd().cmd,
		newSetConfigCmd().cmd,
		newExportCmd().cmd,
		newImportCmd().cmd,
		newPinCmd().cmd,
		newUnpinCmd().cmd,
		newRemoveCmd().cmd,
		newRecoverCmd().cmd,
		newListCmd().cmd,
		newPruneCmd().cmd,
		newCompletionsCmd().cmd,
		newVersionCmd(version).cmd,
		newInspectCmd().cmd,
	)

	root.cmd = cmd
	return root
}

var allowInsecureHTTP bool

func newProviderWithPolicy(u, provider string) (providers.Provider, error) {
	if err := providers.ValidateURL(u, provider, allowInsecureHTTP); err != nil {
		return nil, err
	}
	return providers.New(u, provider)
}

func (cmd *rootCmd) configureLogger() error {
	if cmd.logWriter != nil || strings.TrimSpace(cmd.logFile) == "" {
		return nil
	}

	writer, err := cmd.openLogFile(cmd.logFile)
	if err != nil {
		return err
	}

	cmd.logWriter = writer
	logger := log.New(writer)
	copyLoggerSettings(logger, log.Log)
	log.Log = logger
	return nil
}

func openRootLogFile(path string) (io.WriteCloser, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("--log-file requires a path")
	}

	return os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

func newSpinnerLogger(extra io.Writer) log.Interface {
	writer := spinner.Writer(os.Stderr)
	if extra != nil {
		writer = io.MultiWriter(writer, extra)
	}

	logger := log.New(writer)

	copyLoggerSettings(logger, log.Log)

	if extra != nil {
		logger.Writer = colorprofile.NewWriter(io.MultiWriter(spinner.Writer(os.Stderr), extra), os.Environ())
		return logger
	}

	logger.Writer = colorprofile.NewWriter(spinner.Writer(os.Stderr), os.Environ())
	return logger
}

func copyLoggerSettings(dst *log.Logger, src log.Interface) {
	if previous, ok := src.(*log.Logger); ok {
		dst.Level = previous.Level
		dst.Padding = previous.Padding
	}
}

func resolveSpinnerCommand(cmd *cobra.Command, args []string) *cobra.Command {
	if cmd == nil {
		return nil
	}

	if shouldShowSpinner(cmd) || len(args) == 0 {
		return cmd
	}

	resolved, _, err := cmd.Root().Find(args)
	if err != nil {
		return cmd
	}

	return resolved
}

func defaultCommand(cmd *cobra.Command, args []string) bool {
	// find current cmd, if its not root, it means the user actively
	// set a command, so let it go
	xmd, _, _ := cmd.Find(args)
	if xmd != cmd {
		return false
	}

	// special case for cobra's default completion command
	// ref: https://github.com/kubernetes/kubectl/blob/04af20f5a9d2b56d910a36fec84f21164df65d32/pkg/cmd/cmd.go#L132
	if len(args) > 0 &&
		(args[0] == "completion" ||
			args[0] == cobra.ShellCompRequestCmd ||
			args[0] == cobra.ShellCompNoDescRequestCmd) {
		return false
	}

	// if we have == 0 args, assume its a ls
	if len(args) == 0 {
		return true
	}

	// given that its 1, check if its one of the valid standalone flags
	// for the root cmd
	for _, s := range []string{"-h", "--help", "-v", "--version", "help"} {
		if s == args[0] {
			// if it is, we should run the root cmd
			return false
		}
	}

	// otherwise, we should probably prepend ls
	return true
}

func getBinPath(name string) (string, error) {
	var f string
	f, err := exec.LookPath(name)
	cfg := config.Get()
	if err != nil {
		log.Log.Debugf("binary %s not found in PATH %v", name, err)
		if !isExplicitTargetPath(name) {
			managedPath, resolveErr := resolveManagedBinPath(cfg.Bins, name, func(b *config.Binary) bool {
				return commandNameMatches(filepath.Base(b.Path), name)
			})
			if resolveErr != nil {
				return "", resolveErr
			}
			if managedPath != "" {
				return managedPath, nil
			}
		}
		return "", err
	}

	managedPath, err := resolveManagedBinPath(cfg.Bins, f, func(b *config.Binary) bool {
		return expandTrackedBinaryPath(b.Path) == f
	})
	if err != nil {
		return "", err
	}
	if managedPath != "" {
		return managedPath, nil
	}

	return "", fmt.Errorf("binary path %s is not managed by bin", f)
}

// resolveManagedBinPath returns one matching stored path. A configuration can
// contain more than one entry with the same command name, so never depend on
// map iteration order when resolving a target.
func resolveManagedBinPath(bins map[string]*config.Binary, input string, matches func(*config.Binary) bool) (string, error) {
	return selectManagedBinPath(bins, input, func(_ string, bin *config.Binary) (string, bool) {
		return bin.Path, matches(bin)
	})
}

// selectManagedBinPath selects one matching path without depending on map
// iteration order. Callers retain their own matching predicate and path value.
func selectManagedBinPath(bins map[string]*config.Binary, input string, matches func(string, *config.Binary) (string, bool)) (string, error) {
	candidates := make([]string, 0, 1)
	for path, bin := range bins {
		if bin == nil {
			continue
		}
		if candidate, ok := matches(path, bin); ok {
			candidates = append(candidates, candidate)
		}
	}

	switch len(candidates) {
	case 0:
		return "", nil
	case 1:
		return candidates[0], nil
	}

	sort.Strings(candidates)
	return "", fmt.Errorf("binary %q is ambiguous; matching managed paths: %s", input, strings.Join(candidates, ", "))
}
