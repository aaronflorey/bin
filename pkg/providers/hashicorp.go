package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/options"
	"github.com/caarlos0/log"
	"github.com/coreos/go-semver/semver"
)

const (
	releasesURLBase = "https://releases.hashicorp.com"
)

type hashiCorp struct {
	url     *url.URL
	client  *http.Client
	owner   string
	repo    string
	tag     string
	baseURL *url.URL
}

func (g *hashiCorp) buildHashiCorpAPIURL(args ...string) string {
	apiURL := &url.URL{}
	*apiURL = *g.baseURL

	args = append(args, "index.json")
	apiURL.Path = path.Join(args...)

	return apiURL.String()
}

func (g *hashiCorp) getRelease(repoName, version string) (*hashiCorpRelease, error) {
	releaseURL := g.buildHashiCorpAPIURL(repoName, version)
	resp, err := g.client.Get(releaseURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var release hashiCorpRelease
	if err := decoder.Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release from %s: %w", releaseURL, err)
	}
	return &release, nil
}

func (g *hashiCorp) listReleases(repoName string) (*hashiCorpRepo, error) {
	repoURL := g.buildHashiCorpAPIURL(repoName)
	resp, err := g.client.Get(repoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var repo hashiCorpRepo
	if err := decoder.Decode(&repo); err != nil {
		return nil, fmt.Errorf("decoding releases from %s: %w", repoURL, err)
	}
	return &repo, nil
}

func (g *hashiCorp) GetID() string {
	return "hashicorp"
}

func (g *hashiCorp) Cleanup(_ *CleanupOpts) error {
	return nil
}

func (g *hashiCorp) Fetch(opts *FetchOpts) (*File, error) {
	var release *hashiCorpRelease

	// If we have a tag, let's fetch from there
	var err error
	if len(g.tag) > 0 || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			g.tag = opts.Version
		}
		log.Infof("Getting %s release for %s", g.tag, g.repo)
		release, err = g.getRelease(g.repo, g.tag)
	} else {
		var releaseInfo *ReleaseInfo
		releaseInfo, err = g.GetLatestVersion()
		if err != nil {
			return nil, err
		}
		release, err = g.getRelease(g.repo, releaseInfo.Version)
	}

	if err != nil {
		return nil, err
	}
	log.Debugf("Loaded HashiCorp release %q for %s with %d builds", release.Version, g.repo, len(release.Builds))

	candidates := []*assets.Asset{}
	checksumAssets := []checksumAsset{}
	for _, link := range release.Builds {
		candidates = append(candidates, &assets.Asset{Name: link.Filename, URL: link.URL})
		checksumAssets = append(checksumAssets, checksumAsset{Name: link.Filename, URL: link.URL})
	}

	f := assets.NewFilter(&assets.FilterOpts{
		SkipScoring:    opts.All,
		PackagePath:    opts.PackagePath,
		SkipPathCheck:  opts.SkipPatchCheck,
		SystemPackage:  opts.SystemPackage,
		PackageType:    opts.PackageType,
		NonInteractive: opts.NonInteractive,
	})
	autoSelect := f.ParseAutoSelection(opts.AutoSelect)
	log.Debugf("Filtering %d HashiCorp assets for %s (autoSelect=%q)", len(candidates), g.repo, autoSelect)
	gf, err := f.FilterAssets(g.repo, candidates, autoSelect)
	if err != nil {
		log.WithError(err).Debugf("HashiCorp asset filtering failed for %s", g.repo)
		return nil, err
	}
	log.Debugf("Selected HashiCorp asset %q for %s", gf.Name, g.repo)

	expectedChecksum, err := expectedSHA256ForAsset(gf.Name, checksumAssets, gf.ExtraHeaders)
	if err != nil {
		log.WithError(err).Debugf("HashiCorp checksum lookup failed for %s asset %q", g.repo, gf.Name)
		return nil, err
	}

	verifyArchiveChecksum := expectedChecksum != nil && expectedChecksum.Scope == checksumScopeArchive
	expectedSHA := ""
	if expectedChecksum != nil {
		expectedSHA = expectedChecksum.Hash
	}

	outFile, err := f.ProcessURL(gf, expectedSHA, verifyArchiveChecksum)
	if err != nil {
		log.WithError(err).Debugf("HashiCorp asset processing failed for %s asset %q", g.repo, gf.Name)
		return nil, err
	}

	finalExpectedSHA := ""
	if expectedChecksum != nil {
		if expectedChecksum.Scope == checksumScopeFinal || outFile.Name == gf.Name {
			finalExpectedSHA = expectedChecksum.Hash
		}
	}

	version := release.Version

	file := &File{Data: outFile.Source, Name: outFile.Name, Version: version, ExpectedSHA: finalExpectedSHA}

	return file, nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *hashiCorp) GetLatestVersion() (*ReleaseInfo, error) {
	log.Debugf("Getting latest release for %s", g.repo)

	releases, err := g.listReleases(g.repo)
	if err != nil {
		return nil, err
	}
	if len(releases.Versions) == 0 {
		return nil, fmt.Errorf("no releases found for %s", g.repo)
	}
	var svs semver.Versions
	for _, version := range releases.Versions {
		sv, err := semver.NewVersion(version.Version)
		if err != nil {
			log.Debugf("unable to parse %q as a semantic version: %+v", version.Version, err)
			continue
		}
		if sv.PreRelease == "" && sv.Metadata == "" {
			svs = append(svs, sv)
		}
	}
	if len(svs) == 0 {
		return nil, fmt.Errorf("no semver versions found for %s", g.repo)
	}
	sort.Sort(svs)
	highestVersion := svs[len(svs)-1]
	tied := map[string]*semver.Version{}
	for i := len(svs) - 1; i >= 0; i-- {
		sv := svs[i]
		if sv.Compare(*highestVersion) == 0 {
			tied[sv.String()] = sv
		}
	}
	if len(tied) > 1 {
		tiedKeys := []string{}
		for key := range tied {
			tiedKeys = append(tiedKeys, key)
		}
		sort.Strings(tiedKeys)
		generic := make([]fmt.Stringer, 0)
		for _, key := range tiedKeys {
			generic = append(generic, tied[key])
		}
		choice, err := options.Select("Select file to download:", generic)
		if err != nil {
			return nil, err
		}
		highestVersion = choice.(*semver.Version)
	}
	release, err := g.getRelease(g.repo, highestVersion.String())
	if err != nil {
		return nil, err
	}

	return &ReleaseInfo{
		Version: release.Version,
		URL:     g.buildHashiCorpAPIURL(g.repo, release.Version),
	}, nil
}

func newHashiCorp(u *url.URL) (Provider, error) {
	segments := providerPathSegments(u)
	if len(segments) == 0 {
		return nil, fmt.Errorf("Error parsing HashiCorp releases URL %s, can't find repo", u.String())
	}

	var tag string
	if len(segments) > 1 {
		tag = segments[1]
	}

	baseURL, _ := url.Parse(releasesURLBase)

	return &hashiCorp{url: u, client: newProviderHTTPClient(), owner: "", repo: segments[0], tag: tag, baseURL: baseURL}, nil
}
