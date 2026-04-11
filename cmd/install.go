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
	systemPackage  bool
	nonInteractive bool
}

type installTarget struct {
	url  string
	path string
}

type resolvedFetchRequest struct {
	url                string
	requestedVersion   string
	hasExplicitVersion bool
	fetchOpts          providers.FetchOpts
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

			targets, err := parseInstallTargets(args, root.opts.systemPackage)
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
	root.cmd.Flags().BoolVar(&root.opts.systemPackage, "system-package", false, "Install from compatible system package artifacts (deb, rpm, apk, flatpak)")
	root.cmd.Flags().BoolVar(&root.opts.nonInteractive, "non-interactive", false, "Disable all interactive prompts (auto-select best option)")
	return root
}

func (root *installCmd) installTarget(cmd *cobra.Command, target installTarget) error {
	resolved, err := resolveFetchRequest(target.url, root.opts.provider, providers.FetchOpts{
		All:            root.opts.all,
		AutoSelect:     root.opts.autoSelect,
		PackageName:    "",
		SystemPackage:  root.opts.systemPackage,
		NonInteractive: root.opts.nonInteractive,
	})
	if err != nil {
		return err
	}

	pinVersion := root.opts.pin
	if resolved.hasExplicitVersion && !pinVersion {
		if root.opts.nonInteractive {
			// Auto-pin in non-interactive mode when explicit version is detected
			log.Debugf("Auto-pinning version %s in non-interactive mode", resolved.requestedVersion)
			pinVersion = true
		} else if prompt.IsInteractive() {
			err := prompt.Confirm(fmt.Sprintf("Detected release URL for version %s. Do you want to pin this version?", resolved.requestedVersion))
			if err == nil {
				pinVersion = true
			} else if err.Error() != "command aborted" {
				return err
			}
		} else {
			log.Debugf("Skipping pin prompt for %s in non-interactive mode", resolved.requestedVersion)
		}
	}

	defaultPath := config.Get().DefaultPath
	cfg := config.Get()

	resolvedPath := target.path
	if root.opts.systemPackage {
		if systemPackagePathLooksExplicit(target.path) {
			return fmt.Errorf("--system-package does not accept filesystem paths; optional second argument must be a command name")
		}
		resolvedPath = ""
		resolved.fetchOpts.PackageName = target.path
	} else if resolvedPath == "" {
		resolvedPath = defaultPath
	} else if !strings.Contains(resolvedPath, "/") {
		resolvedPath = filepath.Join(defaultPath, resolvedPath)
	}

	var minAgeDays *int
	if cmd.Flags().Changed("min-age-days") {
		minAgeDays = &root.opts.minAgeDays
	}

	existing := existingBinaryForInstall(cfg.Bins, resolved.url, root.opts.provider, resolvedPath)
	if existing != nil {
		log.Infof("Binary already exists in config (%s). Updating it instead", existing.Path)
		if resolved.fetchOpts.PackagePath == "" {
			resolved.fetchOpts.PackagePath = existing.PackagePath
		}
		if resolved.fetchOpts.PackageName == "" {
			resolved.fetchOpts.PackageName = existing.RemoteName
		}
		if effectiveInstallMode(existing.InstallMode) == installModeSystemPackage {
			resolved.fetchOpts.SystemPackage = true
			resolved.fetchOpts.PackageType = normalizePackageType(existing.PackageType)
		}

		if root.opts.systemPackage {
			resolved.fetchOpts.PackageName = target.path
			logSystemPackageSelected(resolved.fetchOpts.PackageType, target.path)
		}

		installer := installBinary
		if resolved.fetchOpts.SystemPackage {
			installer = installSystemPackage
		}

		res, err := installer(InstallOpts{
			URL:         resolved.url,
			Provider:    root.opts.provider,
			Path:        existing.Path,
			ConfigPath:  existing.Path,
			Force:       true,
			Pinned:      pinVersion,
			MinAgeDays:  minAgeDays,
			FetchOpts:   resolved.fetchOpts,
			ResolvePath: !resolved.fetchOpts.SystemPackage,
		})
		if err != nil {
			return err
		}

		log.Infof("Done updating %s %s", res.Name, res.Version)
		return nil
	}

	installer := installBinary
	if root.opts.systemPackage {
		resolved.fetchOpts.SystemPackage = true
		resolved.fetchOpts.PackageName = target.path
		logSystemPackageSelected(resolved.fetchOpts.PackageType, target.path)
		installer = installSystemPackage
	}

	res, err := installer(InstallOpts{
		URL:         resolved.url,
		Provider:    root.opts.provider,
		Path:        resolvedPath,
		Force:       root.opts.force,
		Pinned:      pinVersion,
		MinAgeDays:  minAgeDays,
		FetchOpts:   resolved.fetchOpts,
		ResolvePath: !root.opts.systemPackage,
	})
	if err != nil {
		return err
	}

	log.Infof("Done installing %s %s", res.Name, res.Version)
	return nil
}

func parseInstallTargets(args []string, systemPackage bool) ([]installTarget, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("expected at least one install target")
	}

	if len(args) == 1 {
		return []installTarget{{url: args[0]}}, nil
	}

	if len(args) == 2 && !looksLikeInstallURL(args[1]) {
		if systemPackage && systemPackagePathLooksExplicit(args[1]) {
			return nil, fmt.Errorf("--system-package does not accept filesystem paths; optional second argument must be a command name")
		}
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

func resolveFetchRequest(rawURL, forcedProvider string, fetchOpts providers.FetchOpts) (*resolvedFetchRequest, error) {
	normalizedURL, requestedVersion, hasExplicitVersion, err := providers.NormalizeGitHubURL(rawURL, forcedProvider)
	if err != nil {
		return nil, err
	}

	if requestedVersion != "" {
		fetchOpts.Version = requestedVersion
	}

	return &resolvedFetchRequest{
		url:                normalizedURL,
		requestedVersion:   requestedVersion,
		hasExplicitVersion: hasExplicitVersion,
		fetchOpts:          fetchOpts,
	}, nil
}
