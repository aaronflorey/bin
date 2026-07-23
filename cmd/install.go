package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
	"github.com/caarlos0/log"
	"github.com/spf13/cobra"
)

type installCmd struct {
	cmd  *cobra.Command
	opts installOpts
}

type installOpts struct {
	force               bool
	provider            string
	all                 bool
	autoSelect          string
	minAgeDays          int
	pin                 bool
	systemPackage       bool
	preferSystemPackage bool
	packageType         string
	nonInteractive      bool
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
			if root.opts.packageType != "" {
				if !systempackage.IsKnownType(root.opts.packageType) {
					return fmt.Errorf("unsupported --package-type %q", root.opts.packageType)
				}
				root.opts.packageType = systempackage.NormalizeType(root.opts.packageType)
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
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all compatible download options (skip product scoring)")
	root.cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Forces to use a specific provider")
	root.cmd.Flags().StringVarP(&root.opts.autoSelect, "select", "s", "", "Auto select installation file (skips interactive prompt)")
	root.cmd.Flags().IntVar(&root.opts.minAgeDays, "min-age-days", 0, "Require the selected release to be at least this many days old")
	root.cmd.Flags().BoolVar(&root.opts.pin, "pin", false, "Pin installed version without prompting")
	root.cmd.Flags().BoolVar(&root.opts.systemPackage, "system-package", false, "Install from compatible system package artifacts (deb, rpm, apk, flatpak, dmg on macOS)")
	root.cmd.Flags().BoolVar(&root.opts.preferSystemPackage, "prefer-system-package", false, "Prefer compatible system package artifacts before direct binaries when installing")
	root.cmd.Flags().StringVar(&root.opts.packageType, "package-type", "", "Restrict system package selection to a specific type (deb, rpm, apk, flatpak, dmg)")
	root.cmd.Flags().BoolVar(&root.opts.nonInteractive, "non-interactive", false, "Disable prompts and fail on ambiguous choices")
	return root
}

func (root *installCmd) installTarget(cmd *cobra.Command, target installTarget) error {
	log.Debugf("Preparing install target url=%q path=%q", target.url, target.path)

	resolved, err := resolveFetchRequest(target.url, root.opts.provider, providers.FetchOpts{
		All:            root.opts.all,
		AutoSelect:     root.opts.autoSelect,
		PackageName:    "",
		PackageType:    root.opts.packageType,
		NonInteractive: root.opts.nonInteractive,
	})
	if err != nil {
		return err
	}
	log.Debugf("Resolved install request %q to %q (requested version=%q, explicit=%t)", target.url, resolved.url, resolved.requestedVersion, resolved.hasExplicitVersion)

	pinVersion := root.opts.pin
	if resolved.hasExplicitVersion && !pinVersion {
		if root.opts.nonInteractive {
			log.Debugf("Skipping pin prompt for %s in non-interactive mode", resolved.requestedVersion)
		} else if prompt.IsInteractive() {
			err := prompt.Confirm(fmt.Sprintf("Detected release URL for version %s. Do you want to pin this version?", resolved.requestedVersion))
			if err == nil {
				pinVersion = true
			} else if err.Error() != "command aborted" {
				return err
			}
		}
	}

	defaultPath := config.Get().DefaultPath
	cfg := config.Get()

	requestedName := target.path
	resolvedPath := target.path
	if root.opts.systemPackage {
		if systemPackagePathLooksExplicit(target.path) {
			return fmt.Errorf("--system-package does not accept filesystem paths; optional second argument must be a command name")
		}
		resolvedPath = ""
	} else if resolvedPath == "" {
		resolvedPath = defaultPath
	} else if !strings.Contains(resolvedPath, "/") {
		resolvedPath = filepath.Join(defaultPath, resolvedPath)
	}
	log.Debugf("Install target %q resolved to path %q (system-package=%t)", resolved.url, resolvedPath, root.opts.systemPackage)

	prefixes := []string{resolved.fetchOpts.ReleaseTagPrefix}
	if !resolved.hasExplicitVersion {
		options, err := discoverInstallableReleasePrefixes(installProviderFactory, resolved.url, root.opts.provider, resolved.fetchOpts)
		if err != nil {
			log.WithError(err).Debugf("Skipping release lane discovery for %q", resolved.url)
		} else if len(options) > 1 {
			prefixes, err = selectReleaseTagPrefixesInteractively(options, root.opts.nonInteractive)
			if err != nil {
				return err
			}
			if len(prefixes) == 0 {
				return fmt.Errorf("no release lanes selected")
			}
		} else if len(options) == 1 {
			prefixes = []string{releaseFetchPrefix(options[0].Prefix)}
		}
	}

	var minAgeDays *int
	if cmd.Flags().Changed("min-age-days") {
		minAgeDays = &root.opts.minAgeDays
	}

	for _, prefix := range prefixes {
		resolved.fetchOpts.ReleaseTagPrefix = prefix
		existing := existingBinaryForInstall(cfg.Bins, resolved.url, root.opts.provider, resolvedPath, prefix)
		if existing != nil {
			log.Debugf("Found existing managed binary for %q at %s", resolved.url, existing.Path)
			log.Infof("Binary already exists in config (%s). Updating it instead", existing.Path)
			strategy := lifecycleForMode(existing.InstallMode)
			attemptFetchOpts := resolved.fetchOpts
			requestedReleaseTagPrefix := attemptFetchOpts.ReleaseTagPrefix
			if err := strategy.applyStoredFetch(existing, &attemptFetchOpts); err != nil {
				return err
			}
			// Preserve stored asset metadata, but let an explicit release URL or a
			// lane discovered for this install request select the release lane.
			if strings.TrimSpace(requestedReleaseTagPrefix) != "" {
				attemptFetchOpts.ReleaseTagPrefix = requestedReleaseTagPrefix
			}

			if root.opts.systemPackage {
				attemptFetchOpts.PackageName = target.path
				logSystemPackageSelected(attemptFetchOpts.PackageType, target.path)
			}

			if attemptFetchOpts.SystemPackage {
				strategy = lifecycleForMode(installModeSystemPackage)
			}

			res, err := strategy.install(InstallOpts{
				URL:                   resolved.url,
				Provider:              root.opts.provider,
				Path:                  existing.Path,
				ConfigPath:            existing.Path,
				Force:                 true,
				Pinned:                pinVersion,
				MinAgeDays:            minAgeDays,
				FetchOpts:             attemptFetchOpts,
				ResolvePath:           strategy.resolvePath(existing),
				AllowProviderFallback: root.opts.provider == "" && existing.Provider != "",
			})
			if err != nil {
				log.WithError(err).Debugf("Failed to update existing install for %q", resolved.url)
				return err
			}

			log.Infof("Done updating %s %s", res.Name, res.Version)
			continue
		}

		modes := requestedInstallModes(root.opts.systemPackage, root.opts.preferSystemPackage, target.path)
		var lastErr error
		installed := false
		for idx, mode := range modes {
			strategy := lifecycleForMode(mode)
			attemptFetchOpts := resolved.fetchOpts
			if err := strategy.applyRequestFetch(requestedName, &attemptFetchOpts); err != nil {
				return err
			}
			if mode == installModeSystemPackage {
				logSystemPackageSelected(attemptFetchOpts.PackageType, requestedName)
			}

			attemptPath := resolvedPath
			if !strategy.resolvePath(nil) {
				attemptPath = ""
			}
			log.Debugf("Attempting %q install for %q (path=%q, provider=%q, packageType=%q, release-prefix=%q)", mode, resolved.url, attemptPath, root.opts.provider, attemptFetchOpts.PackageType, attemptFetchOpts.ReleaseTagPrefix)

			res, err := strategy.install(InstallOpts{
				URL:                   resolved.url,
				Provider:              root.opts.provider,
				Path:                  attemptPath,
				Force:                 root.opts.force,
				Pinned:                pinVersion,
				MinAgeDays:            minAgeDays,
				FetchOpts:             attemptFetchOpts,
				ResolvePath:           strategy.resolvePath(nil),
				AllowProviderFallback: false,
			})
			if err == nil {
				log.Infof("Done installing %s %s", res.Name, res.Version)
				installed = true
				break
			}
			log.WithError(err).Debugf("Install mode %q failed for %q", mode, resolved.url)
			lastErr = err
			if idx == len(modes)-1 || !shouldFallbackInstallMode(err) {
				return err
			}
			log.Warnf("Install mode %q did not yield a compatible asset for %s, trying %q", mode, resolved.url, modes[idx+1])
		}
		if !installed {
			return lastErr
		}
	}

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

func existingBinaryForInstall(bins map[string]*config.Binary, normalizedURL, forcedProvider, requestedPath, requestedReleaseTagPrefix string) *config.Binary {
	if requestedPath != "" {
		if b, ok := existingConfigBinary(InstallOpts{Path: requestedPath}); ok {
			return b
		}
	}

	var matched *config.Binary
	for _, b := range bins {
		if b.URL != normalizedURL {
			continue
		}
		if forcedProvider != "" && b.Provider != forcedProvider {
			continue
		}
		effectiveStoredPrefix := providers.EffectiveReleaseTagPrefix(b.Version, b.ReleaseTagPrefix)
		if strings.TrimSpace(requestedReleaseTagPrefix) != strings.TrimSpace(effectiveStoredPrefix) {
			continue
		}
		if matched != nil {
			return nil
		}
		matched = b
	}

	return matched
}

func resolveFetchRequest(rawURL, forcedProvider string, fetchOpts providers.FetchOpts) (*resolvedFetchRequest, error) {
	normalizedURL, requestedVersion, hasExplicitVersion, err := providers.NormalizeGitHubURL(rawURL, forcedProvider)
	if err != nil {
		return nil, err
	}

	if requestedVersion != "" {
		fetchOpts.Version = requestedVersion
		fetchOpts.ReleaseTagPrefix = releaseFetchPrefix(providers.ReleaseTagPrefix(requestedVersion))
	}

	return &resolvedFetchRequest{
		url:                normalizedURL,
		requestedVersion:   requestedVersion,
		hasExplicitVersion: hasExplicitVersion,
		fetchOpts:          fetchOpts,
	}, nil
}
