package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
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
			return runEnsure(args)
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	return root
}

func runEnsure(args []string) error {
	cfg := config.Get()
	binsToProcess, err := resolveBinsToProcess(cfg.Bins, args)
	if err != nil {
		return err
	}

	for _, binCfg := range binsToProcess {
		ep := os.ExpandEnv(binCfg.Path)
		installMode := effectiveInstallMode(binCfg.InstallMode)
		strategy := lifecycleForMode(installMode)
		if installMode == installModeSystemPackage && systempackage.NormalizeType(binCfg.PackageType) == "dmg" && binCfg.AppBundle != "" {
			if resolvedPath, err := resolveAppBundleExecutable(filepath.Join(applicationsDir, binCfg.AppBundle)); err == nil {
				ep = resolvedPath
			}
		}
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
			Version: binCfg.Version,
		}
		if err := strategy.applyStoredFetch(binCfg, &fetchOpts); err != nil {
			return err
		}

		opts := InstallOpts{
			URL:                   binCfg.URL,
			Provider:              binCfg.Provider,
			Path:                  ep,
			Force:                 true,
			FetchOpts:             fetchOpts,
			ResolvePath:           strategy.resolvePath(binCfg),
			ConfigPath:            binCfg.Path,
			AllowProviderFallback: binCfg.Provider != "",
		}
		res, err := strategy.install(opts)
		if err != nil && installMode == installModeBinary && fetchOpts.PackagePath != "" && isPackagePathSelectionError(err) {
			log.Warnf("%s package path %q did not match the latest archive; retrying without package path", ep, fetchOpts.PackagePath)
			opts.FetchOpts.PackagePath = ""
			res, err = strategy.install(opts)
		}
		if err != nil {
			return err
		}
		log.Infof("Done ensuring %s to %s", os.ExpandEnv(binCfg.Path), color.GreenString(res.Version))
	}

	return nil
}

func isPackagePathSelectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no files found in tar archive") ||
		strings.Contains(msg, "no files found in zip archive")
}
