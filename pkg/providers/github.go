package providers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/caarlos0/log"
	"github.com/google/go-github/v73/github"
)

var runGHAuthToken = sync.OnceValues(func() ([]byte, error) {
	return exec.Command("gh", "auth", "token").Output()
})

type githubReleaseCacheKey struct {
	baseURL  string
	owner    string
	repo     string
	selector string
	auth     [sha256.Size]byte
}

type githubReleaseCacheEntry struct {
	once     sync.Once
	releases []*github.RepositoryRelease
	err      error
}

// ponytail: process-lifetime cache; add eviction only if bin gains a long-running mode.
var githubReleaseCache = &sync.Map{}

type gitHub struct {
	url             *url.URL
	client          *github.Client
	owner           string
	repo            string
	tag             string
	token           string
	authFingerprint [sha256.Size]byte
}

func (g *gitHub) Fetch(opts *FetchOpts) (*File, error) {
	var release *github.RepositoryRelease

	// If we have a tag, let's fetch from there
	var err error
	if len(g.tag) > 0 || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			g.tag = opts.Version
		}
		log.Infof("Getting %s release for %s/%s", g.tag, g.owner, g.repo)
		release, err = g.releaseByTag(g.tag)
	} else if opts.ReleaseTagPrefix != "" {
		log.Infof("Getting latest %q release for %s/%s", opts.ReleaseTagPrefix, g.owner, g.repo)
		release, err = g.releaseByPrefix(opts.ReleaseTagPrefix)
	} else {
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		release, err = g.latestRelease()
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
	release, err := g.latestRelease()
	if err != nil {
		return nil, err
	}

	return githubReleaseInfo(release), nil
}

func (g *gitHub) ListReleases(limit int) ([]*ReleaseInfo, error) {
	if limit <= 0 {
		limit = 100
	}

	releases, err := g.releases(limit)
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

func (g *gitHub) latestRelease() (*github.RepositoryRelease, error) {
	return g.cachedRelease("latest", "GetLatestRelease", func() (*github.RepositoryRelease, *github.Response, error) {
		ctx, cancel := newProviderRequestContext()
		defer cancel()
		return g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
	})
}

func (g *gitHub) releaseByTag(tag string) (*github.RepositoryRelease, error) {
	return g.cachedRelease("tag:"+tag, "GetReleaseByTag", func() (*github.RepositoryRelease, *github.Response, error) {
		ctx, cancel := newProviderRequestContext()
		defer cancel()
		return g.client.Repositories.GetReleaseByTag(ctx, g.owner, g.repo, tag)
	})
}

func (g *gitHub) releaseByPrefix(prefix string) (*github.RepositoryRelease, error) {
	key := g.releaseCacheKey("prefix:" + strings.TrimSpace(prefix))
	if release, ok := cachedGitHubRelease(key); ok {
		return release, nil
	}

	releases, err := g.releases(100)
	if err != nil {
		return nil, err
	}
	for _, release := range releases {
		if release != nil && MatchesReleaseTagPrefix(release.GetTagName(), prefix) {
			return release, nil
		}
	}

	return nil, fmt.Errorf("repository %s/%s does not have a release with prefix %q", g.owner, g.repo, prefix)
}

func (g *gitHub) releases(limit int) ([]*github.RepositoryRelease, error) {
	key := g.releaseCacheKey(fmt.Sprintf("list:%d", limit))
	return loadCachedGitHubReleases(key, func() ([]*github.RepositoryRelease, error) {
		ctx, cancel := newProviderRequestContext()
		defer cancel()

		releases, resp, err := g.client.Repositories.ListReleases(ctx, g.owner, g.repo, &github.ListOptions{PerPage: limit})
		if err != nil {
			g.logGitHubResponse(resp, err, "ListReleases")
			return nil, err
		}

		seenPrefixes := map[string]struct{}{}
		for _, release := range releases {
			if release == nil {
				continue
			}
			g.rememberRelease("tag:"+release.GetTagName(), release)
			prefix := ReleaseTagPrefix(release.GetTagName())
			if prefix == "" {
				prefix = BareReleaseTagPrefix
			}
			if _, seen := seenPrefixes[prefix]; seen {
				continue
			}
			seenPrefixes[prefix] = struct{}{}
			g.rememberRelease("prefix:"+prefix, release)
		}

		return releases, nil
	})
}

func (g *gitHub) cachedRelease(selector, operation string, load func() (*github.RepositoryRelease, *github.Response, error)) (*github.RepositoryRelease, error) {
	key := g.releaseCacheKey(selector)
	releases, err := loadCachedGitHubReleases(key, func() ([]*github.RepositoryRelease, error) {
		release, resp, err := load()
		if err != nil {
			g.logGitHubResponse(resp, err, operation)
			return nil, err
		}
		if release == nil {
			return nil, fmt.Errorf("GitHub %s returned no release for %s/%s", operation, g.owner, g.repo)
		}
		g.rememberRelease("tag:"+release.GetTagName(), release)
		return []*github.RepositoryRelease{release}, nil
	})
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("GitHub %s returned no release for %s/%s", operation, g.owner, g.repo)
	}
	return releases[0], nil
}

func (g *gitHub) rememberRelease(selector string, release *github.RepositoryRelease) {
	if release == nil {
		return
	}
	entry := &githubReleaseCacheEntry{}
	entry.once.Do(func() {
		entry.releases = []*github.RepositoryRelease{release}
	})
	githubReleaseCache.LoadOrStore(g.releaseCacheKey(selector), entry)
}

func (g *gitHub) releaseCacheKey(selector string) githubReleaseCacheKey {
	baseURL := ""
	if g.client != nil && g.client.BaseURL != nil {
		baseURL = g.client.BaseURL.String()
	}
	return githubReleaseCacheKey{
		baseURL:  baseURL,
		owner:    strings.ToLower(g.owner),
		repo:     strings.ToLower(g.repo),
		selector: selector,
		auth:     g.authFingerprint,
	}
}

func loadCachedGitHubReleases(key githubReleaseCacheKey, load func() ([]*github.RepositoryRelease, error)) ([]*github.RepositoryRelease, error) {
	cache := githubReleaseCache
	value, _ := cache.LoadOrStore(key, &githubReleaseCacheEntry{})
	entry := value.(*githubReleaseCacheEntry)
	entry.once.Do(func() {
		entry.releases, entry.err = load()
		if entry.err != nil {
			cache.Delete(key)
		}
	})
	return entry.releases, entry.err
}

func cachedGitHubRelease(key githubReleaseCacheKey) (*github.RepositoryRelease, bool) {
	value, ok := githubReleaseCache.Load(key)
	if !ok {
		return nil, false
	}
	entry := value.(*githubReleaseCacheEntry)
	if entry.err != nil || len(entry.releases) == 0 {
		return nil, false
	}
	return entry.releases[0], true
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

	effectiveToken := token
	if ghesConfigured {
		effectiveToken = gau
	}

	return &gitHub{
		url:             u,
		client:          client,
		owner:           segments[0],
		repo:            segments[1],
		tag:             tag,
		token:           token,
		authFingerprint: sha256.Sum256([]byte(effectiveToken)),
	}, nil
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
