package cmd

import (
	"os"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type ensureCmd struct {
	cmd *cobra.Command
}

func newEnsureCmd() *ensureCmd {
	root := &ensureCmd{}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "ensure [binary_path]...",
		Aliases:       []string{"e"},
		Short:         "Ensures that all binaries listed in the configuration are present",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Get()
			binsToProcess, err := resolveBinsToProcess(cfg.Bins, args)
			if err != nil {
				return err
			}

			for _, binCfg := range binsToProcess {
				ep := os.ExpandEnv(binCfg.Path)
				installMode := effectiveInstallMode(binCfg.InstallMode)
				_, statErr := os.Stat(ep)

				if statErr == nil {
					hash, err := hashFile(ep)
					if err != nil {
						return err
					}

					if hash == binCfg.Hash {
						continue
					}

					log.Infof("%s hash does not match with config's, re-installing", ep)

				} else if !os.IsNotExist(statErr) {
					return statErr
				}

				fetchOpts := providers.FetchOpts{
					Version:     binCfg.Version,
					PackagePath: binCfg.PackagePath,
					PackageName: binCfg.RemoteName,
				}
				installer := installBinary
				if installMode == installModeSystemPackage {
					fetchOpts.SystemPackage = true
					fetchOpts.PackageType = normalizePackageType(binCfg.PackageType)
					installer = installSystemPackage
				}

				res, err := installer(InstallOpts{
					URL:         binCfg.URL,
					Provider:    binCfg.Provider,
					Path:        ep,
					Force:       true,
					FetchOpts:   fetchOpts,
					ResolvePath: installMode != installModeSystemPackage,
					ConfigPath:  binCfg.Path,
				})
				if err != nil {
					return err
				}
				log.Infof("Done ensuring %s to %s", os.ExpandEnv(binCfg.Path), color.GreenString(res.Version))
			}
			return nil
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	return root
}
