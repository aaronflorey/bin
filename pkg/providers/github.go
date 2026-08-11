package providers

import (
	"errors"
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
		release, resp, err = g.client.Repositories.GetReleaseByTag(ctx, g.owner, g.repo, g.tag)
		cancel()
		if err != nil {
			g.logGitHubResponse(resp, err, "GetReleaseByTag")
		}
	} else if opts.ReleaseTagPrefix != "" {
		log.Infof("Getting latest %q release for %s/%s", opts.ReleaseTagPrefix, g.owner, g.repo)
		history, historyErr := g.ListReleases(100)
		if historyErr != nil {
			return nil, historyErr
		}
		selected := SelectReleaseByPrefix(history, opts.ReleaseTagPrefix)
		if selected == nil {
			return nil, fmt.Errorf("repository %s/%s does not have a release with prefix %q", g.owner, g.repo, opts.ReleaseTagPrefix)
		}
		ctx, cancel := newProviderRequestContext()
		release, resp, err = g.client.Repositories.GetReleaseByTag(ctx, g.owner, g.repo, selected.Version)
		cancel()
		if err != nil {
			g.logGitHubResponse(resp, err, "GetReleaseByTag")
		}
	} else {
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		ctx, cancel := newProviderRequestContext()
		release, resp, err = g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
		cancel()
		if err != nil {
			g.logGitHubResponse(resp, err, "GetLatestRelease")
		} else if resp != nil && resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("repository %s/%s does not have releases", g.owner, g.repo)
			g.logGitHubResponse(resp, err, "GetLatestRelease")
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

	expectedChecksum, err := expectedSHA256ForAsset(gf.Name, checksumAssets, gf.ExtraHeaders)
	if err != nil {
		log.WithError(err).Debugf("GitHub checksum lookup failed for %s/%s asset %q", g.owner, g.repo, gf.Name)
		return nil, err
	}

	verifyArchiveChecksum := expectedChecksum != nil && expectedChecksum.Scope == checksumScopeArchive
	expectedSHA := ""
	if expectedChecksum != nil {
		expectedSHA = expectedChecksum.Hash
	}

	outFile, err := f.ProcessURL(gf, expectedSHA, verifyArchiveChecksum)
	if err != nil {
		log.WithError(err).Debugf("GitHub asset processing failed for %s/%s asset %q", g.owner, g.repo, gf.Name)
		return nil, err
	}

	finalExpectedSHA := ""
	if expectedChecksum != nil {
		if expectedChecksum.Scope == checksumScopeFinal || outFile.Name == gf.Name {
			finalExpectedSHA = expectedChecksum.Hash
		}
	}

	version := release.GetTagName()

	file := &File{
		Data:             outFile.Source,
		Name:             outFile.Name,
		Version:          version,
		ReleaseTagPrefix: fetchedReleaseTagPrefix(version, opts.ReleaseTagPrefix),
		ExpectedSHA:      finalExpectedSHA,
		PackagePath:      outFile.PackagePath,
		SourceAsset:      gf.Name,
		PublishedAt:      githubPublishedAt(release),
	}

	return file, nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *gitHub) GetLatestVersion() (*ReleaseInfo, error) {
	log.Debugf("Getting latest release for %s/%s", g.owner, g.repo)
	ctx, cancel := newProviderRequestContext()
	defer cancel()

	release, resp, err := g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
	if err != nil {
		g.logGitHubResponse(resp, err, "GetLatestRelease")
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

	releases, resp, err := g.client.Repositories.ListReleases(ctx, g.owner, g.repo, &github.ListOptions{PerPage: limit})
	if err != nil {
		g.logGitHubResponse(resp, err, "ListReleases")
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
		Assets:      githubReleaseAssets(release),
		PublishedAt: githubPublishedAt(release),
		Body:        release.GetBody(),
	}
}

func githubReleaseAssets(release *github.RepositoryRelease) []string {
	if release == nil {
		return nil
	}
	assets := make([]string, 0, len(release.Assets))
	for _, asset := range release.Assets {
		if asset == nil {
			continue
		}
		name := strings.TrimSpace(asset.GetName())
		if name != "" {
			assets = append(assets, name)
		}
	}
	return assets
}

func newGitHub(u *url.URL) (Provider, error) {
	segments := providerPathSegments(u)
	if len(segments) < 2 {
		return nil, fmt.Errorf("error parsing Github URL %s, can't find owner and repo", u.String())
	}

	tag := releaseTagFromSegments(segments)

	token := os.Getenv("GITHUB_AUTH_TOKEN")
	tokenSource := ""
	if len(token) == 0 {
		token = os.Getenv("GITHUB_TOKEN")
		if len(token) > 0 {
			tokenSource = "GITHUB_TOKEN"
		}
	} else {
		tokenSource = "GITHUB_AUTH_TOKEN"
	}

	// GHES client
	gbu := os.Getenv("GHES_BASE_URL")
	guu := os.Getenv("GHES_UPLOAD_URL")
	gau := os.Getenv("GHES_AUTH_TOKEN")
	ghesConfigured := len(gbu) > 0 && len(guu) > 0 && len(gau) > 0

	if token == "" && !ghesConfigured && config.Get().UseGHAuth {
		log.Debugf("GitHub token config: use_gh_for_github_token is enabled, attempting gh CLI token lookup")
		if out, err := runGHAuthToken(); err == nil {
			token = strings.TrimSpace(string(out))
			if token != "" {
				tokenSource = "gh CLI (gh auth token)"
				log.Debugf("GitHub token acquired from gh CLI")
			} else {
				log.Debugf("gh auth token returned an empty token")
			}
		} else {
			log.Debugf("Could not get GitHub token from gh CLI: %v", err)
		}
	} else if token == "" && !ghesConfigured && !config.Get().UseGHAuth {
		log.Debugf("GitHub token config: use_gh_for_github_token is disabled; gh CLI token lookup skipped")
	}

	if ghesConfigured {
		log.Debugf("GitHub token source: GHES_AUTH_TOKEN (base=%q upload=%q)", gbu, guu)
	} else if token != "" {
		log.Debugf("GitHub token source: %s", tokenSource)
	} else {
		log.Warnf("No GitHub token found (GITHUB_AUTH_TOKEN, GITHUB_TOKEN, or gh CLI via use_gh_for_github_token); using anonymous requests which are rate-limited to 60/hour. See `bin set-config use_gh_for_github_token true`")
	}

	tc := newProviderHTTPClient()

	if ghesConfigured {
		tc = newProviderOAuthHTTPClient(gau)
	} else if token != "" {
		tc = newProviderOAuthHTTPClient(token)
	}

	var client *github.Client
	var err error

	if ghesConfigured {
		if client, err = github.NewClient(tc).WithEnterpriseURLs(gbu, guu); err != nil {
			return nil, fmt.Errorf("error initializing GHES client %v", err)
		}
	} else {
		client = github.NewClient(tc)
	}

	return &gitHub{url: u, client: client, owner: segments[0], repo: segments[1], tag: tag, token: token}, nil
}

// logGitHubResponse logs the HTTP status and rate-limit state for a GitHub API
// response. It is safe to call with a nil resp. When err is a known GitHub
// error type (*github.RateLimitError or *github.ErrorResponse), the response
// status and message are logged at warn level so rate-limit and auth problems
// are visible even without --debug.
func (g *gitHub) logGitHubResponse(resp *github.Response, err error, operation string) {
	authed := g.token != ""
	if resp == nil || resp.Response == nil {
		if err != nil {
			log.WithError(err).Warnf("GitHub %s failed for %s/%s (authenticated=%t) with no response", operation, g.owner, g.repo, authed)
		}
		return
	}

	status := resp.StatusCode
	rate := resp.Rate
	rateSummary := fmt.Sprintf("rate %d/%d remaining, resets %s", rate.Remaining, rate.Limit, formatRateResetTime(rate.Reset.Time))

	if err != nil {
		var rateLimitErr *github.RateLimitError
		var errorResp *github.ErrorResponse
		switch {
		case errors.As(err, &rateLimitErr):
			log.Warnf("GitHub %s for %s/%s returned rate-limit error %d (authenticated=%t): %s. %s",
				operation, g.owner, g.repo, status, authed, rateLimitErr.Message, rateSummary)
		case errors.As(err, &errorResp):
			log.Warnf("GitHub %s for %s/%s failed with status %d (authenticated=%t): %s. %s",
				operation, g.owner, g.repo, status, authed, errorResp.Message, rateSummary)
		default:
			log.WithError(err).Warnf("GitHub %s for %s/%s failed with status %d (authenticated=%t). %s",
				operation, g.owner, g.repo, status, authed, rateSummary)
		}
		return
	}

	log.Debugf("GitHub %s for %s/%s: status %d (authenticated=%t). %s", operation, g.owner, g.repo, status, authed, rateSummary)
	if rate.Remaining <= 5 && rate.Limit > 0 {
		log.Warnf("GitHub rate limit almost exhausted for %s/%s: %d/%d remaining, resets %s (authenticated=%t)",
			g.owner, g.repo, rate.Remaining, rate.Limit, formatRateResetTime(rate.Reset.Time), authed)
	}
}

// formatRateResetTime renders a rate-limit reset time as a local clock time,
// or "unknown" if it is the zero value.
func formatRateResetTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("15:04:05 MST")
}
