package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type completionsCmd struct {
	cmd *cobra.Command
}

type completionSyncCmd struct {
	cmd *cobra.Command
}

var completionProviderFactory = newProviderWithPolicy

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
	if err := verifyManagedBinaryHash(binary, binary.Hash); err != nil {
		return err
	}

	command := filepath.Base(expandTrackedBinaryPath(binary.Path))
	if generatorArgs != nil || !bundledCompletionFetchEligible(binary) {
		destination, err := syncNativeCompletion(binaryPath, shell, command, generatorArgs)
		if err != nil {
			return err
		}
		writeCompletionSetupGuidance(cmd, shell, destination)
		return nil
	}

	bundled, found, err := fetchBundledCompletion(binary, shell, command)
	if err != nil {
		return err
	}
	if found {
		destination, err := publishManagedCompletion(binary.Path, binary.Hash, shell, command, bundled)
		if err != nil {
			return err
		}
		writeCompletionSetupGuidance(cmd, shell, destination)
		return nil
	}
	destination, err := syncNativeCompletion(binaryPath, shell, command, generatorArgs)
	if err != nil {
		return err
	}
	writeCompletionSetupGuidance(cmd, shell, destination)
	return nil
}

// bundledCompletionFetchEligible keeps sync from repeating providers whose
// fetch operation pulls an image or runs a local build. Other managed sources
// use their ordinary provider construction and fetch path.
func bundledCompletionFetchEligible(binary *config.Binary) bool {
	source := strings.ToLower(strings.TrimSpace(binary.URL))
	if source == "" {
		return false
	}
	if strings.HasPrefix(source, "docker://") || strings.HasPrefix(source, "goinstall://") {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(binary.Provider)) {
	case "docker", "goinstall":
		return false
	default:
		return true
	}
}

// fetchBundledCompletion refetches an HTTP-managed artifact using the same
// stored selection that lifecycle operations use. The artifact stream is
// always closed before its bounded completion bytes are returned.
func fetchBundledCompletion(binary *config.Binary, shell, command string) ([]byte, bool, error) {
	fetchOpts := providers.FetchOpts{
		Version:                  binary.Version,
		BundledCompletionShell:   shell,
		BundledCompletionCommand: command,
	}
	if err := lifecycleForMode(installModeBinary).applyStoredFetch(binary, &fetchOpts); err != nil {
		return nil, false, err
	}

	provider, err := completionProviderFactory(binary.URL, binary.Provider)
	if err != nil {
		return nil, false, err
	}
	file, fetchErr := provider.Fetch(&fetchOpts)
	if fetchErr != nil {
		return nil, false, closeFetchedCompletionFile(file, fetchErr)
	}
	if file == nil {
		return nil, false, errors.New("provider returned no fetched binary")
	}
	if len(file.BundledCompletion) == 0 {
		return nil, false, closeFetchedCompletionFile(file, nil)
	}
	if file.Data == nil {
		return nil, false, closeFetchedCompletionFile(file, errors.New("provider returned bundled completion without executable bytes"))
	}

	fetchedHash, hashErr := file.Hash()
	if err := closeFetchedCompletionFile(file, hashErr); err != nil {
		return nil, false, err
	}
	if !strings.EqualFold(fmt.Sprintf("%x", fetchedHash), binary.Hash) {
		return nil, false, nil
	}

	return append([]byte(nil), file.BundledCompletion...), true, nil
}

func closeFetchedCompletionFile(file *providers.File, resultErr error) error {
	if file == nil {
		return resultErr
	}
	return errors.Join(resultErr, closeFetchedFile(file))
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
