package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
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
		SkipScoring:    fetchOpts.All,
		PackagePath:    fetchOpts.PackagePath,
		SkipPathCheck:  fetchOpts.SkipPatchCheck,
		PackageName:    fetchOpts.PackageName,
		SystemPackage:  fetchOpts.SystemPackage,
		PackageType:    fetchOpts.PackageType,
		NonInteractive: true,
	})
	autoSelect := f.ParseAutoSelection(fetchOpts.AutoSelect)
	selected, err := f.FilterAssets("", candidates, autoSelect)
	if err != nil {
		return "", false
	}
	return selected.Name, true
}

func selectReleaseTagPrefixesInteractively(options []releasePrefixOption) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}
	if len(options) == 1 || !prompt.IsInteractive() {
		return []string{options[0].Prefix}, nil
	}

	opts := make([]prompt.MultiSelectOption, 0, len(options))
	for _, option := range options {
		label := option.LatestTag
		if option.Prefix != "" {
			label = fmt.Sprintf("%s (%s)", option.Prefix, option.LatestTag)
		}
		opts = append(opts, prompt.MultiSelectOption{
			Value: option.Prefix,
			Label: label,
		})
	}

	selected, err := prompt.SelectMultiple(
		"Select release lanes to install",
		opts,
	)
	if err != nil {
		return nil, err
	}
	sort.Strings(selected)
	return selected, nil
}

