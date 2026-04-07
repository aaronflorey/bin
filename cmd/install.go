package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
	"github.com/spf13/cobra"
)

type installCmd struct {
	cmd  *cobra.Command
	opts installOpts
}

type installOpts struct {
	force          bool
	provider       string
	all            bool
	autoSelect     string
	minAgeDays     int
	pin            bool
	nonInteractive bool
}

type installTarget struct {
	url  string
	path string
}

func newInstallCmd() *installCmd {
	root := &installCmd{}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "install <url> [name | path] | <url>...",
		Aliases:       []string{"i"},
		Short:         "Installs the specified binary from a url",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("min-age-days") && root.opts.minAgeDays <= 0 {
				return fmt.Errorf("--min-age-days must be a positive integer")
			}

			targets, err := parseInstallTargets(args)
			if err != nil {
				return err
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PreInstall)); err != nil {
				return err
			}

			for _, target := range targets {
				if err := root.installTarget(cmd, target); err != nil {
					return err
				}
			}

			if err := config.ExecuteHooks(config.GetHooks(config.PostInstall)); err != nil {
				return err
			}

			return nil
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	root.cmd.Flags().BoolVarP(&root.opts.force, "force", "f", false, "Force the installation even if the file already exists")
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all possible download options (skip scoring & filtering)")
	root.cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Forces to use a specific provider")
	root.cmd.Flags().StringVarP(&root.opts.autoSelect, "select", "s", "", "Auto select installation file (skips interactive prompt)")
	root.cmd.Flags().IntVar(&root.opts.minAgeDays, "min-age-days", 0, "Require the selected release to be at least this many days old")
	root.cmd.Flags().BoolVar(&root.opts.pin, "pin", false, "Pin installed version without prompting")
	root.cmd.Flags().BoolVar(&root.opts.nonInteractive, "non-interactive", false, "Disable all interactive prompts (auto-select best option)")
	return root
}

func (root *installCmd) installTarget(cmd *cobra.Command, target installTarget) error {
	u := target.url
	normalizedURL, requestedVersion, hasExplicitVersion, err := providers.NormalizeGitHubURL(u, root.opts.provider)
	if err != nil {
		return err
	}

	pinVersion := root.opts.pin
	if hasExplicitVersion && !pinVersion {
		if root.opts.nonInteractive {
			// Auto-pin in non-interactive mode when explicit version is detected
			log.Debugf("Auto-pinning version %s in non-interactive mode", requestedVersion)
			pinVersion = true
		} else if prompt.IsInteractive() {
			err := prompt.Confirm(fmt.Sprintf("Detected release URL for version %s. Do you want to pin this version?", requestedVersion))
			if err == nil {
				pinVersion = true
			} else if err.Error() != "command aborted" {
				return err
			}
		} else {
			log.Debugf("Skipping pin prompt for %s in non-interactive mode", requestedVersion)
		}
	}

	fetchOpts := providers.FetchOpts{
		All:            root.opts.all,
		AutoSelect:     root.opts.autoSelect,
		NonInteractive: root.opts.nonInteractive,
	}
	if requestedVersion != "" {
		fetchOpts.Version = requestedVersion
	}

	defaultPath := config.Get().DefaultPath
	cfg := config.Get()

	resolvedPath := target.path
	if resolvedPath == "" {
		resolvedPath = defaultPath
	} else if !strings.Contains(resolvedPath, "/") {
		resolvedPath = filepath.Join(defaultPath, resolvedPath)
	}

	var minAgeDays *int
	if cmd.Flags().Changed("min-age-days") {
		minAgeDays = &root.opts.minAgeDays
	}

	existing := existingBinaryForInstall(cfg.Bins, normalizedURL, root.opts.provider, resolvedPath)
	if existing != nil {
		log.Infof("Binary already exists in config (%s). Updating it instead", existing.Path)
		if fetchOpts.PackagePath == "" {
			fetchOpts.PackagePath = existing.PackagePath
		}
		if fetchOpts.PackageName == "" {
			fetchOpts.PackageName = existing.RemoteName
		}

		res, err := installBinary(InstallOpts{
			URL:         normalizedURL,
			Provider:    root.opts.provider,
			Path:        existing.Path,
			ConfigPath:  existing.Path,
			Force:       true,
			Pinned:      pinVersion,
			MinAgeDays:  minAgeDays,
			FetchOpts:   fetchOpts,
			ResolvePath: false,
		})
		if err != nil {
			return err
		}

		log.Infof("Done updating %s %s", res.Name, res.Version)
		return nil
	}

	res, err := installBinary(InstallOpts{
		URL:         normalizedURL,
		Provider:    root.opts.provider,
		Path:        resolvedPath,
		Force:       root.opts.force,
		Pinned:      pinVersion,
		MinAgeDays:  minAgeDays,
		FetchOpts:   fetchOpts,
		ResolvePath: true,
	})
	if err != nil {
		return err
	}

	log.Infof("Done installing %s %s", res.Name, res.Version)
	return nil
}

func parseInstallTargets(args []string) ([]installTarget, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("expected at least one install target")
	}

	if len(args) == 1 {
		return []installTarget{{url: args[0]}}, nil
	}

	if len(args) == 2 && !looksLikeInstallURL(args[1]) {
		return []installTarget{{url: args[0], path: args[1]}}, nil
	}

	targets := make([]installTarget, 0, len(args))
	for _, arg := range args {
		if !looksLikeInstallURL(arg) {
			return nil, fmt.Errorf("when installing multiple binaries, all arguments must be URLs; got %q", arg)
		}
		targets = append(targets, installTarget{url: arg})
	}

	return targets, nil
}

func looksLikeInstallURL(input string) bool {
	return looksLikeUpdateURL(input)
}

func existingBinaryForInstall(bins map[string]*config.Binary, normalizedURL, forcedProvider, requestedPath string) *config.Binary {
	if requestedPath != "" {
		if b, ok := existingConfigBinary(InstallOpts{Path: requestedPath}); ok {
			return b
		}
	}

	for _, b := range bins {
		if b.URL != normalizedURL {
			continue
		}
		if forcedProvider != "" && b.Provider != forcedProvider {
			continue
		}
		return b
	}

	return nil
}
