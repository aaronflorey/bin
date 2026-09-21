package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type completionsCmd struct {
	cmd *cobra.Command
}

type completionSyncCmd struct {
	cmd *cobra.Command
}

func newCompletionsCmd() *completionsCmd {
	root := &completionsCmd{}
	cmd := &cobra.Command{
		Use:           "completions",
		Short:         "Manage completions for binaries installed by bin",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.cmd = cmd
	cmd.AddCommand(newCompletionSyncCmd().cmd)
	return root
}

func newCompletionSyncCmd() *completionSyncCmd {
	root := &completionSyncCmd{}
	cmd := &cobra.Command{
		Use:           "sync <binary> <shell> [-- <generator-args...>]",
		Short:         "Install or refresh one managed binary completion",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			_, _, _, err := parseCompletionSyncArgs(args, cmd.ArgsLenAtDash())
			return err
		},
		RunE: root.run,
	}

	root.cmd = cmd
	return root
}

func (root *completionSyncCmd) run(cmd *cobra.Command, args []string) error {
	binaryInput, shell, generatorArgs, err := parseCompletionSyncArgs(args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}

	binaryPath, err := getBinPath(binaryInput)
	if err != nil {
		return err
	}
	binary, err := config.GetBinary(binaryPath)
	if err != nil {
		return err
	}
	if err := validateDirectManagedBinary(binary); err != nil {
		return err
	}

	command := filepath.Base(expandTrackedBinaryPath(binary.Path))
	destination, err := syncNativeCompletion(binaryPath, shell, command, generatorArgs)
	if err != nil {
		return err
	}
	writeCompletionSetupGuidance(cmd, shell, destination)
	return nil
}

func parseCompletionSyncArgs(args []string, argsLenAtDash int) (string, string, []string, error) {
	commandArgs := args
	var generatorArgs []string
	if argsLenAtDash >= 0 {
		if argsLenAtDash > len(args) {
			return "", "", nil, fmt.Errorf("invalid completion generator arguments")
		}
		commandArgs = args[:argsLenAtDash]
		generatorArgs = append([]string(nil), args[argsLenAtDash:]...)
	}
	if len(generatorArgs) == 0 {
		generatorArgs = nil
	}
	if len(commandArgs) != 2 {
		return "", "", nil, fmt.Errorf("completions sync requires exactly one binary and one shell")
	}
	if _, err := completionFilename(commandArgs[1], "command"); err != nil {
		return "", "", nil, err
	}
	return commandArgs[0], commandArgs[1], generatorArgs, nil
}

func writeCompletionSetupGuidance(cmd *cobra.Command, shell, destination string) {
	directory := filepath.Dir(destination)
	fmt.Fprintf(cmd.OutOrStdout(), "Installed completion: %s\n", destination)
	switch shell {
	case "bash":
		fmt.Fprintf(cmd.OutOrStdout(), "Bash: source this file, or add %s to your existing completion setup.\n", directory)
	case "zsh":
		fmt.Fprintf(cmd.OutOrStdout(), "Zsh: add %s to fpath before running compinit.\n", directory)
	case "fish":
		fmt.Fprintf(cmd.OutOrStdout(), "Fish: add %s to fish_complete_path.\n", directory)
	}
}
