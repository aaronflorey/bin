package providers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/caarlos0/log"
	"github.com/google/go-github/v73/github"
)

var runGHAuthToken = func() ([]byte, error) {
	return exec.Command("gh", "auth", "token").Output()
}

type gitHub struct {
	url    *url.URL
	client *github.Client
	owner  string
	repo   string
	tag    string
	token  string
}

func (g *gitHub) Fetch(opts *FetchOpts) (*File, error) {
	var release *github.RepositoryRelease

	// If we have a tag, let's fetch from there
	var err error
	var resp *github.Response
	if len(g.tag) > 0 || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			g.tag = opts.Version
		}
		log.Infof("Getting %s release for %s/%s", g.tag, g.owner, g.repo)
		ctx, cancel := newProviderRequestContext()
		release, _, err = g.client.Repositories.GetReleaseByTag(ctx, g.owner, g.repo, g.tag)
		cancel()
	} else {
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		ctx, cancel := newProviderRequestContext()
		release, resp, err = g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
		cancel()
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("repository %s/%s does not have releases", g.owner, g.repo)
		}
	}

	if err != nil {
		return nil, err
	}
	log.Debugf("Loaded GitHub release %q for %s/%s with %d assets", release.GetTagName(), g.owner, g.repo, len(release.Assets))

	candidates := []*assets.Asset{}
	checksumAssets := []checksumAsset{}
	for _, a := range release.Assets {
		name := a.GetName()
		url := a.GetURL()
		candidates = append(candidates, &assets.Asset{Name: name, URL: url})
		checksumAssets = append(checksumAssets, checksumAsset{Name: name, URL: url})
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
	log.Debugf("Filtering %d GitHub assets for %s/%s (autoSelect=%q)", len(candidates), g.owner, g.repo, autoSelect)
	gf, err := f.FilterAssets(g.repo, candidates, autoSelect)
	if err != nil {
		log.WithError(err).Debugf("GitHub asset filtering failed for %s/%s", g.owner, g.repo)
		return nil, err
	}
	log.Debugf("Selected GitHub asset %q from %s/%s", gf.Name, g.owner, g.repo)

	gf.ExtraHeaders = map[string]string{"Accept": "application/octet-stream"}
	if g.token != "" {
		gf.ExtraHeaders["Authorization"] = fmt.Sprintf("token %s", g.token)
	}

	expectedSHA, err := expectedSHA256ForAsset(gf.Name, checksumAssets, gf.ExtraHeaders)
	if err != nil {
		log.WithError(err).Debugf("GitHub checksum lookup failed for %s/%s asset %q", g.owner, g.repo, gf.Name)
		return nil, err
	}

	outFile, err := f.ProcessURL(gf, expectedSHA)
	if err != nil {
		log.WithError(err).Debugf("GitHub asset processing failed for %s/%s asset %q", g.owner, g.repo, gf.Name)
		return nil, err
	}

	finalExpectedSHA := ""
	if outFile.Name == gf.Name {
		finalExpectedSHA = expectedSHA
	}

	version := release.GetTagName()

	file := &File{
		Data:        outFile.Source,
		Name:        outFile.Name,
		Version:     version,
		ExpectedSHA: finalExpectedSHA,
		PackagePath: outFile.PackagePath,
		PublishedAt: githubPublishedAt(release),
	}

	return file, nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *gitHub) GetLatestVersion() (*ReleaseInfo, error) {
	log.Debugf("Getting latest release for %s/%s", g.owner, g.repo)
	ctx, cancel := newProviderRequestContext()
	defer cancel()

	release, _, err := g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
	if err != nil {
		return nil, err
	}

	return githubReleaseInfo(release), nil
}

func (g *gitHub) ListReleases(limit int) ([]*ReleaseInfo, error) {
	if limit <= 0 {
		limit = 100
	}

	ctx, cancel := newProviderRequestContext()
	defer cancel()

	releases, _, err := g.client.Repositories.ListReleases(ctx, g.owner, g.repo, &github.ListOptions{PerPage: limit})
	if err != nil {
		return nil, err
	}

	history := make([]*ReleaseInfo, 0, len(releases))
	for _, release := range releases {
		history = append(history, githubReleaseInfo(release))
	}

	return history, nil
}

func (g *gitHub) GetID() string {
	return "github"
}

func (g *gitHub) Cleanup(_ *CleanupOpts) error {
	return nil
}

func githubPublishedAt(release *github.RepositoryRelease) *time.Time {
	if release == nil || release.PublishedAt == nil {
		return nil
	}
	return PtrTime(release.PublishedAt.Time)
}

func githubReleaseInfo(release *github.RepositoryRelease) *ReleaseInfo {
	if release == nil {
		return nil
	}

	return &ReleaseInfo{
		Version:     release.GetTagName(),
		URL:         release.GetHTMLURL(),
		PublishedAt: githubPublishedAt(release),
		Body:        release.GetBody(),
	}
}

func newGitHub(u *url.URL) (Provider, error) {
	segments := providerPathSegments(u)
	if len(segments) < 2 {
		return nil, fmt.Errorf("error parsing Github URL %s, can't find owner and repo", u.String())
	}

	tag := releaseTagFromSegments(segments)

	token := os.Getenv("GITHUB_AUTH_TOKEN")
	if len(token) == 0 {
		token = os.Getenv("GITHUB_TOKEN")
	}

	// GHES client
	gbu := os.Getenv("GHES_BASE_URL")
	guu := os.Getenv("GHES_UPLOAD_URL")
	gau := os.Getenv("GHES_AUTH_TOKEN")

	if token == "" && (len(gbu) == 0 || len(guu) == 0 || len(gau) == 0) && config.Get().UseGHAuth {
		if out, err := runGHAuthToken(); err == nil {
			token = strings.TrimSpace(string(out))
		} else {
			log.Debugf("Could not get GitHub token from gh CLI: %v", err)
		}
	}

	tc := newProviderHTTPClient()

	if len(gbu) > 0 && len(guu) > 0 && len(gau) > 0 {
		tc = newProviderOAuthHTTPClient(gau)
	} else if token != "" {
		tc = newProviderOAuthHTTPClient(token)
	}

	var client *github.Client
	var err error

	if len(gbu) > 0 && len(guu) > 0 && len(gau) > 0 {
		if client, err = github.NewClient(tc).WithEnterpriseURLs(gbu, guu); err != nil {
			return nil, fmt.Errorf("error initializing GHES client %v", err)
		}
	} else {
		client = github.NewClient(tc)
	}

	return &gitHub{url: u, client: client, owner: segments[0], repo: segments[1], tag: tag, token: token}, nil
}
