package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type removeCmd struct {
	cmd           *cobra.Command
	opts          removeOpts
	selectTargets func(string, []prompt.MultiSelectOption) ([]string, error)
	isInteractive func() bool
}

type removeOpts struct {
	yes bool
}

type removeTarget struct {
	configPath string
	deletePath string
	binary     *config.Binary
}

func newRemoveCmd() *removeCmd {
	root := &removeCmd{
		selectTargets: prompt.MultiSelect,
		isInteractive: prompt.IsInteractive,
	}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "remove [<name> | <paths...>]",
		Aliases:       []string{"rm", "r", "uninstall"},
		Short:         "Removes binaries managed by bin",
		SilenceUsage:  true,
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Get()

			targets, err := root.resolveTargets(cmd, cfg.Bins, args)
			if err != nil {
				return err
			}

			if len(targets) == 0 {
				return nil
			}

			// Execute pre-remove hooks before any changes
			if err := config.ExecuteHooks(config.GetHooks(config.PreRemove)); err != nil {
				return err
			}

			for _, target := range targets {
				strategy := lifecycleForMode(target.binary.InstallMode)
				if strategy.uninstall != nil {
					if !root.opts.yes {
						if !root.isInteractive() {
							return fmt.Errorf("system-package removal requires --yes in non-interactive mode")
						}
						if err := prompt.Confirm(fmt.Sprintf("Uninstall system package backing %s?", target.binary.Path)); err != nil {
							return err
						}
					}

					if err := strategy.uninstall(target.binary); err != nil {
						return err
					}
					if err := config.RemoveBinaries([]string{target.configPath}); err != nil {
						return err
					}
					continue
				}

				p, err := providers.New(target.binary.URL, target.binary.Provider)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not initialize provider cleanup for %s: %v\n", target.binary.Path, err)
				} else {
					err = p.Cleanup(&providers.CleanupOpts{Version: target.binary.Version, Path: target.deletePath})
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: cleanup failed for %s: %v\n", target.binary.Path, err)
					}
				}

				if err := os.Remove(target.deletePath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("error removing path %s: %v", target.deletePath, err)
				}

				if err := config.RemoveBinaries([]string{target.configPath}); err != nil {
					return err
				}
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PostRemove)); err != nil {
				return err
			}
			return nil
		},
	}

	root.cmd = cmd
	root.cmd.Flags().BoolVarP(&root.opts.yes, "yes", "y", false, "Assume yes for system package uninstall confirmation")
	enableSpinner(root.cmd)
	return root
}

func (root *removeCmd) resolveTargets(cmd *cobra.Command, bins map[string]*config.Binary, args []string) ([]removeTarget, error) {
	if len(args) == 0 {
		if len(bins) == 0 {
			return nil, nil
		}

		if !root.isInteractive() {
			return nil, fmt.Errorf("remove without arguments requires an interactive terminal")
		}

		selectionOptions := makeRemoveSelectionOptions(bins)
		selected, err := root.selectTargets("Select binaries to remove", selectionOptions)
		if err != nil {
			return nil, err
		}

		return resolveTargetsFromConfigPaths(bins, selected), nil
	}

	resolvedPaths := map[string]string{}
	targets := make([]removeTarget, 0, len(args))
	seen := map[string]struct{}{}

	for _, p := range args {
		bp, ok := resolvedPaths[p]
		if !ok {
			var err error
			bp, err = getBinPath(p)

			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
				if aliasPath := findManagedBinByAlias(bins, p); aliasPath != "" {
					bp = aliasPath
					err = nil
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "binary %s not found in PATH, skipping\n", p)
					continue
				}
			}
			if err != nil {
				return nil, err
			}

			resolvedPaths[p] = bp
		}

		ebp := os.ExpandEnv(bp)
		bin, ok := bins[ebp]
		if !ok {
			continue
		}
		if _, duplicate := seen[ebp]; duplicate {
			continue
		}

		seen[ebp] = struct{}{}
		targets = append(targets, removeTarget{configPath: ebp, deletePath: ebp, binary: bin})
	}

	return targets, nil
}

func makeRemoveSelectionOptions(bins map[string]*config.Binary) []prompt.MultiSelectOption {
	paths := make([]string, 0, len(bins))
	for p := range bins {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	options := make([]prompt.MultiSelectOption, 0, len(paths))
	for _, p := range paths {
		b := bins[p]
		expanded := os.ExpandEnv(b.Path)
		name := filepath.Base(expanded)
		label := fmt.Sprintf("%s (%s)", name, expanded)
		if effectiveInstallMode(b.InstallMode) == installModeSystemPackage {
			label = fmt.Sprintf("%s [system-package]", label)
		}

		options = append(options, prompt.MultiSelectOption{
			Label: label,
			Value: p,
		})
	}

	return options
}

func resolveTargetsFromConfigPaths(bins map[string]*config.Binary, configPaths []string) []removeTarget {
	targets := make([]removeTarget, 0, len(configPaths))
	seen := map[string]struct{}{}

	for _, configPath := range configPaths {
		bin, ok := bins[configPath]
		if !ok {
			continue
		}
		if _, duplicate := seen[configPath]; duplicate {
			continue
		}

		seen[configPath] = struct{}{}
		targets = append(targets, removeTarget{
			configPath: configPath,
			deletePath: os.ExpandEnv(bin.Path),
			binary:     bin,
		})
	}

	return targets
}
