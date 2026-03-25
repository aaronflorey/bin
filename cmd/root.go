package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
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
	cmd.cmd.SetArgs(args)

	previousLogger := log.Log
	defer func() {
		log.Log = previousLogger
	}()

	if defaultCommand(cmd.cmd, args) {
		if len(args) == 0 {
			cmd.cmd.SetArgs(append([]string{"list"}, args...))
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: bin %s\n", args[0])
			os.Exit(1)
		}
	}

	defer spinner.Stop()

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
	cmd   *cobra.Command
	debug bool
	exit  func(int)
}

func newRootCmd(version string, exit func(int)) *rootCmd {
	root := &rootCmd{
		exit: exit,
	}
	cmd := &cobra.Command{
		Use:           "bin",
		Short:         "Effortless binary manager",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Name() == "version" {
				return
			}

			if root.debug {
				log.SetLevel(log.DebugLevel)
				log.Debugf("debug logs enabled, version: %s\n", version)
			}

			// check and load config after handlers are configured
			err := config.CheckAndLoad()
			if err != nil {
				log.Fatalf("Error loading config file %v", err)
			}

			if shouldShowSpinner(cmd) {
				cmd.SetOut(spinner.Writer(cmd.OutOrStdout()))
				cmd.SetErr(spinner.Writer(cmd.ErrOrStderr()))
				log.Log = newSpinnerLogger()
				spinner.Start("")
			}
		},
	}

	cmd.PersistentFlags().BoolVar(&root.debug, "debug", false, "Enable debug mode")
	cmd.AddCommand(
		newInstallCmd().cmd,
		newEnsureCmd().cmd,
		newOutdatedCmd().cmd,
		newUpdateCmd().cmd,
		newSetConfigCmd().cmd,
		newExportCmd().cmd,
		newImportCmd().cmd,
		newPinCmd().cmd,
		newUnpinCmd().cmd,
		newRemoveCmd().cmd,
		newListCmd().cmd,
		newPruneCmd().cmd,
		newVersionCmd(version).cmd,
	)

	root.cmd = cmd
	return root
}

func newSpinnerLogger() log.Interface {
	logger := log.New(spinner.Writer(os.Stderr))

	if previous, ok := log.Log.(*log.Logger); ok {
		logger.Level = previous.Level
		logger.Padding = previous.Padding
	}

	logger.Writer = colorprofile.NewWriter(spinner.Writer(os.Stderr), os.Environ())
	return logger
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
		if !strings.Contains(name, "/") {
			for _, b := range cfg.Bins {
				if filepath.Base(b.Path) == name {
					return b.Path, nil
				}
			}
		}
		return "", err
	}

	for _, bin := range cfg.Bins {
		if os.ExpandEnv(bin.Path) == f {
			return bin.Path, nil
		}
	}

	return "", fmt.Errorf("binary path %s not found", f)
}
