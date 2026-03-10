package cmd

import (
	"path/filepath"
	"strings"

	"github.com/caarlos0/log"
	"github.com/marcosnils/bin/pkg/config"
	"github.com/marcosnils/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type installCmd struct {
	cmd  *cobra.Command
	opts installOpts
}

type installOpts struct {
	force    bool
	provider string
	all      bool
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

			res, err := installBinary(InstallOpts{
				URL:         u,
				Provider:    root.opts.provider,
				Path:        resolvedPath,
				Force:       root.opts.force,
				FetchOpts:   providers.FetchOpts{All: root.opts.all},
				ResolvePath: true,
			})
			if err != nil {
				return err
			}

			log.Infof("Done installing %s %s", res.Name, res.Version)
			return nil
		},
	}

	root.cmd = cmd
	root.cmd.Flags().BoolVarP(&root.opts.force, "force", "f", false, "Force the installation even if the file already exists")
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all possible download options (skip scoring & filtering)")
	root.cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Forces to use a specific provider")
	return root
}
