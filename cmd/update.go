package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

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
	selectItems updateSelectionFunc
}

type updateSelectionFunc func([]availableUpdate) ([]availableUpdate, error)

type updateOpts struct {
	yesToUpdate     bool
	dryRun          bool
	all             bool
	parallelism     int
	skipPathCheck   bool
	continueOnError bool
}

type updateInfo struct{ version, url string }

func newUpdateCmd() *updateCmd {
	root := &updateCmd{newProvider: providers.New, selectItems: selectUpdatesForInteractiveSession}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "update [binary_path]",
		Aliases:       []string{"u"},
		Short:         "Updates one or multiple binaries managed by bin",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Get()
			binsToProcess, explicitVersion, hasExplicitVersion, err := resolveUpdateTargets(cfg.Bins, args)
			if err != nil {
				return err
			}

			var updates []availableUpdate
			updateFailures := map[*config.Binary]error{}

			if hasExplicitVersion {
				updates = collectExplicitVersionUpdates(binsToProcess, explicitVersion)
			} else {
				updates, updateFailures, err = collectAvailableUpdates(binsToProcess, root.newProvider, root.opts.continueOnError, root.opts.parallelism)
				if err != nil {
					return err
				}
			}

			if len(args) == 0 && !hasExplicitVersion && !root.opts.yesToUpdate && !root.opts.dryRun && len(updates) > 0 {
				updates, err = root.selectItems(updates)
				if err != nil {
					return err
				}
				if len(updates) == 0 {
					for _, err := range updateFailures {
						log.Warnf("%v", err)
					}
					log.Infof("No binaries selected for update")
					return nil
				}
			}

			if len(updates) == 0 && len(updateFailures) == 0 {
				log.Infof("All binaries are up to date")
				return nil
			}

			for _, update := range updates {
				log.Infof(
					"%s %s -> %s (%s)",
					update.binary.Path,
					color.YellowString(update.binary.Version),
					color.GreenString(update.info.version),
					update.info.url,
				)
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
				fetchOpts := providers.FetchOpts{
					All:            root.opts.all,
					PackagePath:    b.PackagePath,
					SkipPatchCheck: root.opts.skipPathCheck,
					PackageName:    b.RemoteName,
					Version:        ui.version,
				}

				installer := installBinary
				if effectiveInstallMode(b.InstallMode) == installModeSystemPackage {
					packageType := normalizePackageType(b.PackageType)
					if packageType == "" {
						err = fmt.Errorf("binary %s is in system-package mode but has no package_type metadata", b.Path)
						if root.opts.continueOnError {
							updateFailures[b] = err
							continue
						}
						return err
					}
					fetchOpts.SystemPackage = true
					fetchOpts.PackageType = packageType
					installer = installSystemPackage
				}

				res, err := installer(InstallOpts{
					URL:         ui.url,
					Provider:    b.Provider,
					Path:        b.Path,
					Force:       true,
					FetchOpts:   fetchOpts,
					ResolvePath: effectiveInstallMode(b.InstallMode) != installModeSystemPackage,
					ConfigPath:  b.Path,
				})
				if err != nil {
					if effectiveInstallMode(b.InstallMode) == installModeSystemPackage && strings.Contains(strings.ToLower(err.Error()), "compatible") {
						err = fmt.Errorf("update failed for %s: latest release no longer exposes a compatible %s package", b.Path, b.PackageType)
					}
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

			if len(updateFailures) > 0 {
				return wrapErrorWithCode(fmt.Errorf("some updates failed"), 4, "")
			}

			return nil
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	root.cmd.Flags().BoolVarP(&root.opts.dryRun, "dry-run", "", false, "Only show status, don't prompt for update")
	root.cmd.Flags().BoolVarP(&root.opts.yesToUpdate, "yes", "y", false, "Assume yes to update prompt")
	root.cmd.Flags().BoolVarP(&root.opts.all, "all", "a", false, "Show all possible download options (skip scoring & filtering)")
	root.cmd.Flags().IntVarP(&root.opts.parallelism, "parallelism", "p", defaultUpdateParallelism, "Maximum number of binaries to check for updates concurrently")
	root.cmd.Flags().BoolVarP(&root.opts.skipPathCheck, "skip-path-check", "", false, "Skips path checking when looking into packages")
	root.cmd.Flags().BoolVarP(&root.opts.continueOnError, "continue-on-error", "c", false, "Continues to update next package if an error is encountered")
	return root
}

func getLatestVersion(b *config.Binary, p providers.Provider) (*updateInfo, error) {
	log.Debugf("Checking updates for %s", b.Path)
	releaseInfo, err := p.GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("Error checking updates for %s, %w", b.Path, err)
	}
	if releaseInfo == nil {
		return nil, nil
	}

	if err := ensureReleaseAge(p.GetID(), releaseInfo.Version, releaseInfo.PublishedAt, b.MinAgeDays); err != nil {
		if releaseInfo.PublishedAt != nil {
			log.Infof("Skipping %s update to %s because it is newer than the configured %d day minimum age", b.Path, releaseInfo.Version, b.MinAgeDays)
			return nil, nil
		}
		return nil, fmt.Errorf("Error checking updates for %s, %w", b.Path, err)
	}

	if b.Version == releaseInfo.Version {
		return nil, nil
	}

	bSemver, bSemverErr := version.NewVersion(b.Version)
	vSemver, vSemverErr := version.NewVersion(releaseInfo.Version)
	if bSemverErr == nil && vSemverErr == nil && vSemver.LessThanOrEqual(bSemver) {
		return nil, nil
	}

	log.Debugf("Found new version %s for %s at %s", releaseInfo.Version, b.Path, releaseInfo.URL)
	return &updateInfo{releaseInfo.Version, releaseInfo.URL}, nil
}

func resolveUpdateTargets(allBins map[string]*config.Binary, args []string) (map[string]*config.Binary, string, bool, error) {
	if len(args) != 1 || !looksLikeUpdateURL(args[0]) {
		bins, err := resolveBinsToProcess(allBins, args)
		return bins, "", false, err
	}

	normalizedURL, requestedVersion, hasExplicitVersion, err := providers.NormalizeGitHubURL(args[0], "")
	if err != nil {
		return nil, "", false, err
	}

	bins := map[string]*config.Binary{}
	for path, b := range allBins {
		if b.URL == normalizedURL || b.URL == args[0] {
			bins[path] = b
		}
	}
	if len(bins) == 0 {
		return nil, "", false, fmt.Errorf("binary with url %q not found in configuration", args[0])
	}

	return bins, requestedVersion, hasExplicitVersion, nil
}

func looksLikeUpdateURL(input string) bool {
	if strings.Contains(input, "://") {
		return true
	}
	lower := strings.ToLower(input)
	return strings.HasPrefix(lower, "github.com/") ||
		strings.HasPrefix(lower, "gitlab.com/") ||
		strings.HasPrefix(lower, "codeberg.org/") ||
		strings.HasPrefix(lower, "releases.hashicorp.com/")
}

func collectExplicitVersionUpdates(bins map[string]*config.Binary, explicitVersion string) []availableUpdate {
	paths := make([]string, 0, len(bins))
	for p := range bins {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	updates := []availableUpdate{}
	for _, p := range paths {
		b := bins[p]
		if !shouldUpdateToExplicitVersion(b.Version, explicitVersion) {
			continue
		}

		updates = append(updates, availableUpdate{
			binary: b,
			info: &updateInfo{
				version: explicitVersion,
				url:     b.URL,
			},
		})
	}

	return updates
}

func shouldUpdateToExplicitVersion(currentVersion, explicitVersion string) bool {
	if currentVersion == explicitVersion {
		return false
	}

	currentSemVer, currentErr := version.NewVersion(currentVersion)
	explicitSemVer, explicitErr := version.NewVersion(explicitVersion)
	if currentErr == nil && explicitErr == nil {
		return explicitSemVer.GreaterThan(currentSemVer)
	}

	return true
}

func selectUpdatesForInteractiveSession(updates []availableUpdate) ([]availableUpdate, error) {
	if len(updates) == 0 || !prompt.IsInteractive() {
		return updates, nil
	}

	items := make([]prompt.MultiSelectItem, 0, len(updates))
	for _, update := range updates {
		items = append(items, prompt.MultiSelectItem{
			Value:       update.binary.Path,
			Label:       fmt.Sprintf("%s (%s -> %s)", update.binary.Path, update.binary.Version, update.info.version),
			Description: update.info.url,
			Selected:    true,
		})
	}

	selectedPaths, err := prompt.MultiSelect(
		"Select binaries to update",
		"up/down: move  space: toggle  a: toggle all  enter: confirm  q: abort",
		items,
	)
	if err != nil {
		return nil, err
	}

	selected := map[string]struct{}{}
	for _, path := range selectedPaths {
		selected[path] = struct{}{}
	}

	filtered := make([]availableUpdate, 0, len(selectedPaths))
	for _, update := range updates {
		if _, ok := selected[update.binary.Path]; ok {
			filtered = append(filtered, update)
		}
	}

	return filtered, nil
}
