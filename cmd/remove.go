package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
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

			type removeTarget struct {
				configPath string
				deletePath string
				binary     *config.Binary
			}

			targets := []removeTarget{}
			pathsToDelete := []string{}
			resolvedPaths := map[string]string{}

			bins := cfg.Bins

			for _, p := range args {
				bp, ok := resolvedPaths[p]
				if !ok {
					var err error
					bp, err = getBinPath(p)

					if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
						fmt.Fprintf(cmd.ErrOrStderr(), "binary %s not found in PATH, skipping\n", p)
						continue
					} else if err != nil {
						return err
					}

					resolvedPaths[p] = bp
				}

				ebp := os.ExpandEnv(bp)
				if _, ok := bins[ebp]; ok {
					targets = append(targets, removeTarget{configPath: ebp, deletePath: os.ExpandEnv(bp), binary: bins[ebp]})
					pathsToDelete = append(pathsToDelete, os.ExpandEnv(bp))
				}
			}

			if len(targets) == 0 {
				return nil
			}

			existingToRemove := make([]string, 0, len(targets))
			for _, target := range targets {
				existingToRemove = append(existingToRemove, target.configPath)
			}

			// Execute pre-remove hooks before any changes
			if err := config.ExecuteHooks(config.GetHooks(config.PreRemove)); err != nil {
				return err
			}

			// Update config first to maintain consistency
			if err := config.RemoveBinaries(existingToRemove); err != nil {
				return err
			}

			for _, target := range targets {
				p, err := providers.New(target.binary.URL, target.binary.Provider)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not initialize provider cleanup for %s: %v\n", target.binary.Path, err)
					continue
				}

				err = p.Cleanup(&providers.CleanupOpts{Version: target.binary.Version, Path: target.deletePath})
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: cleanup failed for %s: %v\n", target.binary.Path, err)
				}
			}

			// Now delete the files
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
