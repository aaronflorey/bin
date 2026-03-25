package cmd

import (
	"fmt"
	"os"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
	"github.com/fatih/color"
	"github.com/hashicorp/go-version"
	"github.com/spf13/cobra"
)

type updateCmd struct {
	cmd         *cobra.Command
	opts        updateOpts
	newProvider providerFactory
}

type updateOpts struct {
	yesToUpdate     bool
	dryRun          bool
	all             bool
	skipPathCheck   bool
	continueOnError bool
}

type updateInfo struct{ version, url string }

func newUpdateCmd() *updateCmd {
	root := &updateCmd{newProvider: providers.New}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "update [binary_path]",
		Aliases:       []string{"u"},
		Short:         "Updates one or multiple binaries managed by bin",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO add support to update from a specific URL.
			// This allows to update binares from a repo that contains
			// multiple tags for different binaries

			// TODO update should check all binaries with a
			// certain configured parallelism (default 10, can be changed with -p) and report
			// which binarines could be potentially upgraded.
			// It's very likely that we have to extend the provider
			// interface to support this use-case

			cfg := config.Get()
			binsToProcess, err := resolveBinsToProcess(cfg.Bins, args)
			if err != nil {
				return err
			}

			updates, updateFailures, err := collectAvailableUpdates(binsToProcess, root.newProvider, root.opts.continueOnError)
			if err != nil {
				return err
			}

			if len(updates) == 0 && len(updateFailures) == 0 {
				log.Infof("All binaries are up to date")
				return nil
			}

			if root.opts.dryRun {
				return wrapErrorWithCode(fmt.Errorf("Updates found, exit (dry-run mode)."), 3, "")
			}

			if len(updates) > 0 && !root.opts.yesToUpdate {
				for _, err := range updateFailures {
					log.Warnf("%v", err)
				}
				updateFailures = map[*config.Binary]error{}

				err := prompt.Confirm("Do you want to continue?")
				if err != nil {
					return err
				}
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PreUpdate)); err != nil {
				return err
			}

			for _, update := range updates {
				b := update.binary
				ui := update.info
				res, err := installBinary(InstallOpts{
					URL:      ui.url,
					Provider: b.Provider,
					Path:     b.Path,
					Force:    true,
					FetchOpts: providers.FetchOpts{
						All:            root.opts.all,
						PackagePath:    b.PackagePath,
						SkipPatchCheck: root.opts.skipPathCheck,
						PackageName:    b.RemoteName,
					},
					ResolvePath: false,
					ConfigPath:  b.Path,
				})
				if err != nil {
					if root.opts.continueOnError {
						updateFailures[b] = fmt.Errorf("Error while fetching %v: %w", ui.url, err)
						continue
					}
					return err
				}

				log.Infof("Done updating %s to %s", os.ExpandEnv(b.Path), color.GreenString(res.Version))
			}
			for _, err := range updateFailures {
				log.Warnf("%v", err)
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PostUpdate)); err != nil {
				return err
			}

			// TODO: Return wrapping error with specific exit code if len(updateFailures) > 0?
			return nil
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	root.cmd.Flags().BoolVarP(&root.opts.dryRun, "dry-run", "", false, "Only show status, don't prompt for update")
	root.cmd.Flags().BoolVarP(&root.opts.yesToUpdate, "yes", "y", false, "Assume yes to update prompt")
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all possible download options (skip scoring & filtering)")
	root.cmd.Flags().BoolVarP(&root.opts.skipPathCheck, "skip-path-check", "p", false, "Skips path checking when looking into packages")
	root.cmd.Flags().BoolVarP(&root.opts.continueOnError, "continue-on-error", "c", false, "Continues to update next package if an error is encountered")
	return root
}

func getLatestVersion(b *config.Binary, p providers.Provider) (*updateInfo, error) {
	log.Debugf("Checking updates for %s", b.Path)
	v, u, err := p.GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("Error checking updates for %s, %w", b.Path, err)
	}

	if b.Version == v {
		return nil, nil
	}

	bSemver, bSemverErr := version.NewVersion(b.Version)
	vSemver, vSemverErr := version.NewVersion(v)
	if bSemverErr == nil && vSemverErr == nil && vSemver.LessThanOrEqual(bSemver) {
		return nil, nil
	}

	log.Debugf("Found new version %s for %s at %s", v, b.Path, u)
	log.Infof("%s %s -> %s (%s)", b.Path, color.YellowString(b.Version), color.GreenString(v), u)
	return &updateInfo{v, u}, nil
}
