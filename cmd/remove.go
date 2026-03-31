package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type removeCmd struct {
	cmd *cobra.Command
}

func newRemoveCmd() *removeCmd {
	root := &removeCmd{}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "remove [<name> | <paths...>]",
		Aliases:       []string{"rm"},
		Short:         "Removes binaries managed by bin",
		SilenceUsage:  true,
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Get()

			existingToRemove := []string{}
			pathsToDelete := []string{}

			bins := cfg.Bins

			for _, p := range args {
				// TODO: avoid calling getBinPath each time and make it
				// once at the beginning for each arg
				bp, err := getBinPath(p)

				if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
					fmt.Fprintf(cmd.ErrOrStderr(), "binary %s not found in PATH, skipping\n", p)
					continue
				} else if err != nil {
					return err
				}
				ebp := os.ExpandEnv(bp)
				if _, ok := bins[ebp]; ok {
					existingToRemove = append(existingToRemove, ebp)
					pathsToDelete = append(pathsToDelete, os.ExpandEnv(bp))
				}
			}

			if len(existingToRemove) == 0 {
				return nil
			}

			// Execute pre-remove hooks before any changes
			if err := config.ExecuteHooks(config.GetHooks(config.PreRemove)); err != nil {
				return err
			}

			// Update config first to maintain consistency
			if err := config.RemoveBinaries(existingToRemove); err != nil {
				return err
			}

			// Now delete the files
			// TODO some providers (like docker) might download
			// additional things somewhere else, maybe we should
			// call the provider to do a cleanup here.
			for _, path := range pathsToDelete {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("error removing path %s: %v", path, err)
				}
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PostRemove)); err != nil {
				return err
			}
			return nil
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	return root
}
