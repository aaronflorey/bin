package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/caarlos0/log"
	"github.com/marcosnils/bin/pkg/config"
	"github.com/marcosnils/bin/pkg/prompt"
	"github.com/marcosnils/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type installCmd struct {
	cmd  *cobra.Command
	opts installOpts
}

type installOpts struct {
	force      bool
	provider   string
	all        bool
	autoSelect string
}

func newInstallCmd() *installCmd {
	root := &installCmd{}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "install <url> [name | path]",
		Aliases:       []string{"i"},
		Short:         "Installs the specified binary from a url",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := args[0]
			normalizedURL, requestedVersion, hasExplicitVersion, err := providers.NormalizeGitHubURL(u, root.opts.provider)
			if err != nil {
				return err
			}

			pinVersion := false
			if hasExplicitVersion {
				err := prompt.Confirm(fmt.Sprintf("Detected release URL for version %s. Do you want to pin this version?", requestedVersion))
				if err == nil {
					pinVersion = true
				} else if err.Error() != "command aborted" {
					return err
				}
			}

			fetchOpts := providers.FetchOpts{All: root.opts.all, AutoSelect: root.opts.autoSelect}
			if requestedVersion != "" {
				fetchOpts.Version = requestedVersion
			}

			defaultPath := config.Get().DefaultPath

			var resolvedPath string
			if len(args) > 1 {
				resolvedPath = args[1]
				if !strings.Contains(resolvedPath, "/") {
					resolvedPath = filepath.Join(defaultPath, resolvedPath)
				}

			} else {
				resolvedPath = defaultPath
			}

			// TODO check if binary already exists in config
			// and triger the update process if that's the case

			if err := config.ExecuteHooks(config.GetHooks(config.PreInstall)); err != nil {
				return err
			}

			res, err := installBinary(InstallOpts{
				URL:         normalizedURL,
				Provider:    root.opts.provider,
				Path:        resolvedPath,
				Force:       root.opts.force,
				Pinned:      pinVersion,
				FetchOpts:   fetchOpts,
				ResolvePath: true,
			})
			if err != nil {
				return err
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PostInstall)); err != nil {
				return err
			}

			log.Infof("Done installing %s %s", res.Name, res.Version)
			return nil
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	root.cmd.Flags().BoolVarP(&root.opts.force, "force", "f", false, "Force the installation even if the file already exists")
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all possible download options (skip scoring & filtering)")
	root.cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Forces to use a specific provider")
	root.cmd.Flags().StringVarP(&root.opts.autoSelect, "select", "s", "", "Auto select installation file (skips interactive prompt)")
	return root
}
