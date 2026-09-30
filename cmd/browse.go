package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aaronflorey/bin/pkg/options"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const browseReleaseHistoryLimit = 20

type browseCmd struct {
	cmd           *cobra.Command
	opts          browseOpts
	newProvider   providerFactory
	isInteractive func(io.Writer) bool
	selectRelease func(string, []fmt.Stringer) (interface{}, error)
}

type browseOpts struct {
	provider       string
	version        string
	nonInteractive bool
}

type browseReleaseOption struct {
	release *providers.ReleaseInfo
}

func (o browseReleaseOption) String() string {
	return o.release.Version
}

func newBrowseCmd() *browseCmd {
	root := &browseCmd{
		newProvider:   newProviderWithPolicy,
		isInteractive: browseOutputIsInteractive,
		selectRelease: options.Select,
	}
	cmd := &cobra.Command{
		Use:   "browse <source>",
		Short: "Browse recent compatible release assets without installing",
		Long: "Browse recent releases from a provider with release-history support. Asset compatibility " +
			"is based on release metadata and the current platform; payloads are not downloaded or validated. " +
			"Use the printed `bin install` command to install a release.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          root.run,
	}
	root.cmd = cmd
	cmd.Flags().StringVarP(&root.opts.provider, "provider", "p", "", "Force a provider (github, gitlab, or codeberg)")
	cmd.Flags().StringVar(&root.opts.version, "version", "", "Show this release tag from the recent history")
	cmd.Flags().BoolVar(&root.opts.nonInteractive, "non-interactive", false, "List recent releases without prompting")
	return root
}

func (root *browseCmd) run(cmd *cobra.Command, args []string) error {
	source := strings.TrimSpace(args[0])
	if err := validateBrowseSource(source, root.opts.provider); err != nil {
		return err
	}

	resolved, err := resolveFetchRequest(source, root.opts.provider, providers.FetchOpts{NonInteractive: true})
	if err != nil {
		return err
	}

	provider, err := root.newProvider(resolved.url, root.opts.provider)
	if err != nil {
		return err
	}

	releases, err := providers.GetReleaseHistory(provider, browseReleaseHistoryLimit)
	if err != nil {
		if errors.Is(err, providers.ErrReleaseHistoryUnsupported) {
			return fmt.Errorf("browse requires release history: %w", err)
		}
		return err
	}
	if len(releases) > browseReleaseHistoryLimit {
		releases = releases[:browseReleaseHistoryLimit]
	}

	requestedVersion := resolved.requestedVersion
	if !resolved.hasExplicitVersion {
		requestedVersion = explicitBrowseReleaseVersion(source, provider.GetID())
	}
	if root.opts.version != "" {
		if requestedVersion != "" && root.opts.version != requestedVersion {
			return fmt.Errorf("--version %q conflicts with release URL tag %q", root.opts.version, requestedVersion)
		}
		requestedVersion = root.opts.version
	}

	if requestedVersion != "" {
		release := findBrowseRelease(releases, requestedVersion)
		if release == nil {
			return fmt.Errorf("release tag %q was not found in the most recent %d releases", requestedVersion, browseReleaseHistoryLimit)
		}
		return writeBrowseReleases(cmd.OutOrStdout(), source, []*providers.ReleaseInfo{release}, resolved.fetchOpts, provider.GetID(), root.opts.provider)
	}

	if len(releases) == 0 {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "No releases found for %s.\n", source)
		return err
	}

	if root.opts.nonInteractive || !root.isInteractive(cmd.OutOrStdout()) {
		return writeBrowseReleases(cmd.OutOrStdout(), source, releases, resolved.fetchOpts, provider.GetID(), root.opts.provider)
	}

	release, err := root.chooseRelease(releases)
	if err != nil {
		return err
	}
	return writeBrowseReleases(cmd.OutOrStdout(), source, []*providers.ReleaseInfo{release}, resolved.fetchOpts, provider.GetID(), root.opts.provider)
}

func browseOutputIsInteractive(output io.Writer) bool {
	file, ok := output.(*os.File)
	return ok && prompt.IsInteractive() && term.IsTerminal(int(file.Fd()))
}

func validateBrowseSource(source, forcedProvider string) error {
	lower := strings.ToLower(source)
	switch {
	case strings.HasPrefix(lower, "docker://"):
		return fmt.Errorf("browse does not support docker:// sources: image pulls are effectful")
	case strings.HasPrefix(lower, "goinstall://"):
		return fmt.Errorf("browse does not support goinstall:// sources: builds execute processes")
	}

	switch forcedProvider {
	case "docker", "goinstall":
		return fmt.Errorf("browse does not support the %q provider: it is effectful", forcedProvider)
	}
	return nil
}

func findBrowseRelease(releases []*providers.ReleaseInfo, version string) *providers.ReleaseInfo {
	for _, release := range releases {
		if release != nil && release.Version == version {
			return release
		}
	}
	return nil
}

func (root *browseCmd) chooseRelease(releases []*providers.ReleaseInfo) (*providers.ReleaseInfo, error) {
	choices := make([]fmt.Stringer, 0, len(releases))
	for _, release := range releases {
		if release != nil {
			choices = append(choices, browseReleaseOption{release: release})
		}
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("no releases found")
	}

	selected, err := root.selectRelease("Select a release to browse:", choices)
	if err != nil {
		return nil, err
	}
	option, ok := selected.(browseReleaseOption)
	if !ok || option.release == nil {
		return nil, fmt.Errorf("release selection returned an invalid choice")
	}
	return option.release, nil
}

func writeBrowseReleases(w io.Writer, source string, releases []*providers.ReleaseInfo, fetchOpts providers.FetchOpts, providerID, forcedProvider string) error {
	if _, err := fmt.Fprintf(w,
		"Recent releases for %s (asset compatibility is metadata-only; payloads are not validated):\n",
		source,
	); err != nil {
		return fmt.Errorf("write browse output: %w", err)
	}

	for _, release := range releases {
		if release == nil {
			continue
		}
		compatibleAssets := compatibleReleaseAssets(release, fetchOpts)
		installURL := release.URL
		if len(compatibleAssets) > 0 {
			if providerID == "gitlab" {
				var err error
				installURL, err = gitLabBrowseReleaseURL(source, release)
				if err != nil {
					return fmt.Errorf("release tag %q has no usable GitLab release URL for install guidance: %w", release.Version, err)
				}
			} else if installURL == "" {
				return fmt.Errorf("release tag %q has no release URL for install guidance", release.Version)
			}
		}
		if _, err := fmt.Fprintf(w, "\nRelease %q", release.Version); err != nil {
			return fmt.Errorf("write browse output: %w", err)
		}
		if release.PublishedAt != nil {
			if _, err := fmt.Fprintf(w, " (published %s)", release.PublishedAt.Format("2006-01-02")); err != nil {
				return fmt.Errorf("write browse output: %w", err)
			}
		}
		if _, err := fmt.Fprintln(w, ":"); err != nil {
			return fmt.Errorf("write browse output: %w", err)
		}

		if len(compatibleAssets) == 0 {
			if _, err := fmt.Fprintln(w, "  No metadata-compatible assets for the current platform."); err != nil {
				return fmt.Errorf("write browse output: %w", err)
			}
			continue
		}
		for _, asset := range compatibleAssets {
			command := fmt.Sprintf("bin install %s --select %s", shellQuote(installURL), shellQuote(asset))
			if forcedProvider != "" {
				command += " --provider " + shellQuote(forcedProvider)
			}
			if _, err := fmt.Fprintf(w, "  %s\n  Install (POSIX shell): %s\n", asset, command); err != nil {
				return fmt.Errorf("write browse output: %w", err)
			}
		}
	}
	return nil
}

func gitLabBrowseReleaseURL(source string, release *providers.ReleaseInfo) (string, error) {
	sourceURL, err := browseSourceURL(source)
	if err != nil {
		return "", err
	}
	projectSegments := browseURLPathSegments(sourceURL)
	if len(projectSegments) < 2 || release == nil || release.Version == "" {
		return "", fmt.Errorf("source or release metadata is missing the GitLab project or tag")
	}

	sourceURL.Path = "/" + strings.Join(projectSegments[:2], "/") + "/-/releases/" + release.Version
	sourceURL.RawPath = ""
	return sourceURL.String(), nil
}

func browseSourceURL(source string) (*url.URL, error) {
	if !strings.Contains(source, "://") {
		source = "https://" + source
	}
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("source is not an absolute URL")
	}
	return u, nil
}

func browseURLPathSegments(u *url.URL) []string {
	var segments []string
	for _, segment := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func explicitBrowseReleaseVersion(source, providerID string) string {
	parseURL := source
	if !strings.Contains(parseURL, "://") {
		parseURL = "https://" + parseURL
	}
	u, err := url.Parse(parseURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, segment := range segments {
		if segment != "releases" || i+1 >= len(segments) {
			continue
		}
		if providerID == "gitlab" {
			return strings.Join(segments[i+1:], "/")
		}
		switch segments[i+1] {
		case "tag":
			return strings.Join(segments[i+2:], "/")
		case "download":
			end := len(segments)
			if end > i+3 {
				end--
			}
			return strings.Join(segments[i+2:end], "/")
		}
	}
	return ""
}
