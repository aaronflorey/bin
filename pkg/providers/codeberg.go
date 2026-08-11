package providers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/caarlos0/log"
)

type codeberg struct {
	url    *url.URL
	client *gitea.Client
	owner  string
	repo   string
	tag    string
	token  string
}

func (c *codeberg) Fetch(opts *FetchOpts) (*File, error) {
	var release *gitea.Release

	// If we have a tag, let's fetch from there
	var err error
	var resp *gitea.Response
	if len(c.tag) > 0 || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			c.tag = opts.Version
		}
		log.Infof("Getting %s release for %s/%s", c.tag, c.owner, c.repo)
		release, _, err = c.client.GetReleaseByTag(c.owner, c.repo, c.tag)
	} else if opts.ReleaseTagPrefix != "" {
		log.Infof("Getting latest %q release for %s/%s", opts.ReleaseTagPrefix, c.owner, c.repo)
		history, historyErr := c.ListReleases(100)
		if historyErr != nil {
			return nil, historyErr
		}
		selected := SelectReleaseByPrefix(history, opts.ReleaseTagPrefix)
		if selected == nil {
			return nil, fmt.Errorf("repository %s/%s does not have a release with prefix %q", c.owner, c.repo, opts.ReleaseTagPrefix)
		}
		release, _, err = c.client.GetReleaseByTag(c.owner, c.repo, selected.Version)
	} else {
		log.Infof("Getting latest release for %s/%s", c.owner, c.repo)
		release, resp, err = c.client.GetLatestRelease(c.owner, c.repo)
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("repository %s/%s does not have releases", c.owner, c.repo)
		}
	}

	if err != nil {
		return nil, err
	}
	log.Debugf("Loaded Codeberg release %q for %s/%s with %d attachments", release.TagName, c.owner, c.repo, len(release.Attachments))

	candidates := []*assets.Asset{}
	checksumAssets := []checksumAsset{}
	for _, a := range release.Attachments {
		candidates = append(candidates, &assets.Asset{Name: a.Name, URL: a.DownloadURL})
		checksumAssets = append(checksumAssets, checksumAsset{Name: a.Name, URL: a.DownloadURL})
	}
	f := assets.NewFilter(&assets.FilterOpts{
		SkipScoring:    opts.All,
		PackagePath:    opts.PackagePath,
		SkipPathCheck:  opts.SkipPatchCheck,
		PackageName:    opts.PackageName,
		SystemPackage:  opts.SystemPackage,
		PackageType:    opts.PackageType,
		NonInteractive: opts.NonInteractive,
	})

	autoSelect := f.ParseAutoSelection(opts.AutoSelect)
	log.Debugf("Filtering %d Codeberg assets for %s/%s (autoSelect=%q)", len(candidates), c.owner, c.repo, autoSelect)
	gf, err := f.FilterAssets(c.repo, candidates, autoSelect)
	if err != nil {
		log.WithError(err).Debugf("Codeberg asset filtering failed for %s/%s", c.owner, c.repo)
		return nil, err
	}
	log.Debugf("Selected Codeberg asset %q from %s/%s", gf.Name, c.owner, c.repo)

	gf.ExtraHeaders = map[string]string{"Accept": "application/octet-stream"}
	if c.token != "" {
		gf.ExtraHeaders["Authorization"] = fmt.Sprintf("token %s", c.token)
	}

	expectedChecksum, err := expectedSHA256ForAsset(gf.Name, checksumAssets, gf.ExtraHeaders)
	if err != nil {
		log.WithError(err).Debugf("Codeberg checksum lookup failed for %s/%s asset %q", c.owner, c.repo, gf.Name)
		return nil, err
	}

	verifyArchiveChecksum := expectedChecksum != nil && expectedChecksum.Scope == checksumScopeArchive
	expectedSHA := ""
	if expectedChecksum != nil {
		expectedSHA = expectedChecksum.Hash
	}

	outFile, err := f.ProcessURL(gf, expectedSHA, verifyArchiveChecksum)
	if err != nil {
		log.WithError(err).Debugf("Codeberg asset processing failed for %s/%s asset %q", c.owner, c.repo, gf.Name)
		return nil, err
	}

	finalExpectedSHA := ""
	if expectedChecksum != nil {
		if expectedChecksum.Scope == checksumScopeFinal || outFile.Name == gf.Name {
			finalExpectedSHA = expectedChecksum.Hash
		}
	}

	version := release.TagName

	file := &File{
		Data:             outFile.Source,
		Name:             outFile.Name,
		Version:          version,
		ReleaseTagPrefix: fetchedReleaseTagPrefix(version, opts.ReleaseTagPrefix),
		ExpectedSHA:      finalExpectedSHA,
		PackagePath:      outFile.PackagePath,
		SourceAsset:      gf.Name,
		PublishedAt:      codebergPublishedAt(release),
	}

	return file, nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (c *codeberg) GetLatestVersion() (*ReleaseInfo, error) {
	log.Debugf("Getting latest release for %s/%s", c.owner, c.repo)
	release, _, err := c.client.GetLatestRelease(c.owner, c.repo)
	if err != nil {
		return nil, err
	}

	return codebergReleaseInfo(release), nil
}

func (c *codeberg) ListReleases(limit int) ([]*ReleaseInfo, error) {
	if limit <= 0 {
		limit = 100
	}

	releases, _, err := c.client.ListReleases(c.owner, c.repo, gitea.ListReleasesOptions{
		ListOptions: gitea.ListOptions{PageSize: limit},
	})
	if err != nil {
		return nil, err
	}

	history := make([]*ReleaseInfo, 0, len(releases))
	for _, release := range releases {
		history = append(history, codebergReleaseInfo(release))
	}

	return history, nil
}

func (c *codeberg) GetID() string {
	return "codeberg"
}

func (c *codeberg) Cleanup(_ *CleanupOpts) error {
	return nil
}

func codebergPublishedAt(release *gitea.Release) *time.Time {
	if release == nil || release.PublishedAt.IsZero() {
		return nil
	}
	return PtrTime(release.PublishedAt)
}

func codebergReleaseInfo(release *gitea.Release) *ReleaseInfo {
	if release == nil {
		return nil
	}

	return &ReleaseInfo{
		Version:     release.TagName,
		URL:         release.HTMLURL,
		Assets:      codebergReleaseAssets(release),
		PublishedAt: codebergPublishedAt(release),
		Body:        release.Note,
	}
}

func codebergReleaseAssets(release *gitea.Release) []string {
	if release == nil {
		return nil
	}
	assets := make([]string, 0, len(release.Attachments))
	for _, attachment := range release.Attachments {
		name := strings.TrimSpace(attachment.Name)
		if name != "" {
			assets = append(assets, name)
		}
	}
	return assets
}

func newCodeberg(u *url.URL) (Provider, error) {
	segments := providerPathSegments(u)
	if len(segments) < 2 {
		return nil, fmt.Errorf("error parsing Codeberg URL %s, can't find owner and repo", u.String())
	}

	tag := releaseTagFromSegments(segments)

	token := os.Getenv("CODEBERG_TOKEN")

	// Codeberg uses Gitea/Forgejo, use the Gitea SDK
	baseURL := fmt.Sprintf("https://%s/", u.Hostname())

	var client *gitea.Client
	var err error

	if token != "" {
		client, err = gitea.NewClient(baseURL, gitea.SetToken(token), gitea.SetHTTPClient(newProviderHTTPClient()))
	} else {
		client, err = gitea.NewClient(baseURL, gitea.SetHTTPClient(newProviderHTTPClient()))
	}

	if err != nil {
		return nil, fmt.Errorf("error initializing Codeberg client %v", err)
	}

	return &codeberg{url: u, client: client, owner: segments[0], repo: segments[1], tag: tag, token: token}, nil
}
