package providers

import (
	"context"
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
	"golang.org/x/oauth2"
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
		release, _, err = g.client.Repositories.GetReleaseByTag(context.Background(), g.owner, g.repo, g.tag)
	} else {
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		release, resp, err = g.client.Repositories.GetLatestRelease(context.Background(), g.owner, g.repo)
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("repository %s/%s does not have releases", g.owner, g.repo)
		}
	}

	if err != nil {
		return nil, err
	}

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
	gf, err := f.FilterAssets(g.repo, candidates, autoSelect)
	if err != nil {
		return nil, err
	}

	gf.ExtraHeaders = map[string]string{"Accept": "application/octet-stream"}
	if g.token != "" {
		gf.ExtraHeaders["Authorization"] = fmt.Sprintf("token %s", g.token)
	}

	outFile, err := f.ProcessURL(gf)
	if err != nil {
		return nil, err
	}

	expectedSHA := ""
	if outFile.Name == gf.Name {
		expectedSHA, err = expectedSHA256ForAsset(outFile.Name, checksumAssets, gf.ExtraHeaders)
		if err != nil {
			return nil, err
		}
	}

	version := release.GetTagName()

	file := &File{
		Data:        outFile.Source,
		Name:        outFile.Name,
		Version:     version,
		ExpectedSHA: expectedSHA,
		PackagePath: outFile.PackagePath,
		PublishedAt: githubPublishedAt(release),
	}

	return file, nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *gitHub) GetLatestVersion() (*ReleaseInfo, error) {
	log.Debugf("Getting latest release for %s/%s", g.owner, g.repo)
	release, _, err := g.client.Repositories.GetLatestRelease(context.Background(), g.owner, g.repo)
	if err != nil {
		return nil, err
	}

	return &ReleaseInfo{
		Version:     release.GetTagName(),
		URL:         release.GetHTMLURL(),
		PublishedAt: githubPublishedAt(release),
	}, nil
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

func newGitHub(u *url.URL) (Provider, error) {
	s := strings.Split(u.Path, "/")
	if len(s) < 3 {
		return nil, fmt.Errorf("error parsing Github URL %s, can't find owner and repo", u.String())
	}

	// it's a specific releases URL
	var tag string
	if strings.Contains(u.Path, "/releases/") {
		// For release and download URL's, the
		// path is usually /releases/tag/v0.1
		// or /releases/download/v0.1.
		ps := strings.Split(u.Path, "/")
		for i, p := range ps {
			if p == "releases" {
				tag = strings.Join(ps[i+2:], "/")
			}
		}

	}

	token := os.Getenv("GITHUB_AUTH_TOKEN")
	if len(token) == 0 {
		token = os.Getenv("GITHUB_TOKEN")
	}

	// GHES client
	gbu := os.Getenv("GHES_BASE_URL")
	guu := os.Getenv("GHES_UPLOAD_URL")
	gau := os.Getenv("GHES_AUTH_TOKEN")

	if token == "" && !(len(gbu) > 0 && len(guu) > 0 && len(gau) > 0) && config.Get().UseGHAuth {
		if out, err := runGHAuthToken(); err == nil {
			token = strings.TrimSpace(string(out))
		} else {
			log.Debugf("Could not get GitHub token from gh CLI: %v", err)
		}
	}

	var tc *http.Client

	if len(gbu) > 0 && len(guu) > 0 && len(gau) > 0 {
		tc = oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: gau},
		))
	} else if token != "" {
		tc = oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		))
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

	return &gitHub{url: u, client: client, owner: s[1], repo: s[2], tag: tag, token: token}, nil
}
