package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/options"
	"github.com/aaronflorey/bin/pkg/prompt"
	"github.com/aaronflorey/bin/pkg/providers"
)

const releaseHistoryPrefixLimit = 10

type releasePrefixOption struct {
	Prefix       string
	LatestTag    string
	MatchedAsset string
	Assets       []string
}

func (o releasePrefixOption) String() string {
	if o.Prefix == "" {
		return o.LatestTag
	}
	return fmt.Sprintf("%s (%s)", o.Prefix, o.LatestTag)
}

func releaseFetchPrefix(prefix string) string {
	if prefix == "" {
		return providers.BareReleaseTagPrefix
	}
	return prefix
}

func discoverInstallableReleasePrefixes(newProvider providerFactory, url, forcedProvider string, fetchOpts providers.FetchOpts) ([]releasePrefixOption, error) {
	p, err := newProvider(url, forcedProvider)
	if err != nil {
		return nil, err
	}

	history, err := providers.GetReleaseHistory(p, releaseHistoryPrefixLimit)
	if err != nil {
		if errors.Is(err, providers.ErrReleaseHistoryUnsupported) {
			return nil, nil
		}
		return nil, err
	}

	options := make([]releasePrefixOption, 0)
	seen := map[string]struct{}{}
	for _, release := range history {
		if release == nil || len(release.Assets) == 0 {
			continue
		}
		prefix := providers.ReleaseTagPrefix(release.Version)
		if _, ok := seen[prefix]; ok {
			continue
		}
		matchedAsset, ok := compatibleReleaseAsset(release, fetchOpts)
		if !ok {
			continue
		}
		seen[prefix] = struct{}{}
		options = append(options, releasePrefixOption{
			Prefix:       prefix,
			LatestTag:    release.Version,
			MatchedAsset: matchedAsset,
			Assets:       append([]string(nil), release.Assets...),
		})
	}

	return options, nil
}

func compatibleReleaseAsset(release *providers.ReleaseInfo, fetchOpts providers.FetchOpts) (string, bool) {
	if release == nil || len(release.Assets) == 0 {
		return "", false
	}
	candidates := make([]*assets.Asset, 0, len(release.Assets))
	for _, name := range release.Assets {
		if strings.TrimSpace(name) == "" {
			continue
		}
		candidates = append(candidates, &assets.Asset{Name: name, URL: "https://example.invalid/" + name})
	}
	if len(candidates) == 0 {
		return "", false
	}

	f := assets.NewFilter(&assets.FilterOpts{
		SkipScoring:   fetchOpts.All,
		PackagePath:   fetchOpts.PackagePath,
		SkipPathCheck: fetchOpts.SkipPatchCheck,
		PackageName:   fetchOpts.PackageName,
		SystemPackage: fetchOpts.SystemPackage,
		PackageType:   fetchOpts.PackageType,
	})
	autoSelect := f.ParseAutoSelection(fetchOpts.AutoSelect)
	compatible := f.CompatibleAssets(candidates, autoSelect)
	if len(compatible) == 0 {
		return "", false
	}
	return compatible[0].Name, true
}

func selectReleaseTagPrefixesInteractively(releaseOptions []releasePrefixOption, nonInteractive bool) ([]string, error) {
	if len(releaseOptions) == 0 {
		return nil, nil
	}
	if len(releaseOptions) == 1 {
		return []string{releaseFetchPrefix(releaseOptions[0].Prefix)}, nil
	}
	if nonInteractive || !prompt.IsInteractive() {
		lanes := make([]string, 0, len(releaseOptions))
		for _, option := range releaseOptions {
			lanes = append(lanes, option.LatestTag)
		}
		return nil, fmt.Errorf("multiple release lanes found: %s (use an explicit release URL)", strings.Join(lanes, ", "))
	}

	opts := make([]fmt.Stringer, 0, len(releaseOptions))
	for _, option := range releaseOptions {
		opts = append(opts, option)
	}

	selected, err := options.Select(
		"Select a release lane to install:",
		opts,
	)
	if err != nil {
		return nil, err
	}
	return []string{releaseFetchPrefix(selected.(releasePrefixOption).Prefix)}, nil
}
