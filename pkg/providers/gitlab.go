package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/caarlos0/log"
	"github.com/yuin/goldmark"
	goldast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type gitLab struct {
	url    *url.URL
	client *gitlab.Client
	token  string
	owner  string
	repo   string
	tag    string
}

func (g *gitLab) Fetch(opts *FetchOpts) (*File, error) {
	var release *gitlab.Release

	// If we have a tag, let's fetch from there
	var err error
	projectPath := fmt.Sprintf("%s/%s", g.owner, g.repo)
	if len(g.tag) > 0 || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			g.tag = opts.Version
		}
		log.Infof("Getting %s release for %s/%s", g.tag, g.owner, g.repo)
		release, _, err = g.client.Releases.GetRelease(projectPath, g.tag)
	} else {
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		var name string
		releaseInfo, releaseErr := g.GetLatestVersion()
		if releaseErr != nil {
			return nil, releaseErr
		}
		name = releaseInfo.Version
		release, _, err = g.client.Releases.GetRelease(projectPath, name, gitlab.WithContext(context.Background()))
	}

	if err != nil {
		return nil, err
	}
	log.Debugf("Loaded GitLab release %q for %s/%s", release.TagName, g.owner, g.repo)

	candidates := []*assets.Asset{}
	checksumAssets := []checksumAsset{}
	candidateURLs := map[string]struct{}{}

	project, _, err := g.client.Projects.GetProject(projectPath, &gitlab.GetProjectOptions{})
	if err != nil {
		return nil, err
	}
	projectIsPublic := g.token == "" || project.Visibility == "" || project.Visibility == gitlab.PublicVisibility
	log.Debugf("Project is public: %v", projectIsPublic)
	tryPackages := projectIsPublic || project.PackagesEnabled
	if tryPackages {
		packages, resp, err := g.client.Packages.ListProjectPackages(projectPath, &gitlab.ListProjectPackagesOptions{
			OrderBy: gitlab.Ptr("version"),
			Sort:    gitlab.Ptr("desc"),
		})
		if err != nil && (resp == nil || resp.StatusCode != http.StatusForbidden) {
			return nil, err
		}
		tagVersion := strings.TrimPrefix(release.TagName, "v")
		for _, v := range packages {
			if strings.TrimPrefix(v.Version, "v") == tagVersion {
				totalPages := -1
				for page := 0; page != totalPages; page++ {
					packageFiles, resp, err := g.client.Packages.ListPackageFiles(projectPath, v.ID, &gitlab.ListPackageFilesOptions{
						Page: page + 1,
					})
					if err != nil {
						return nil, err
					}
					totalPages = resp.TotalPages
					for _, f := range packageFiles {
						assetURL := fmt.Sprintf("%sprojects/%s/packages/%s/%s/%s/%s",
							g.client.BaseURL().String(),
							url.PathEscape(projectPath),
							v.PackageType,
							v.Name,
							v.Version,
							f.FileName,
						)
						if _, exists := candidateURLs[assetURL]; !exists {
							asset := &assets.Asset{
								Name:        f.FileName,
								DisplayName: fmt.Sprintf("%s (%s package)", f.FileName, v.PackageType),
								URL:         assetURL,
							}
							candidates = append(candidates, asset)
							checksumAssets = append(checksumAssets, checksumAsset{Name: f.FileName, URL: assetURL})
							log.Debugf("Adding %s with URL %s", asset, asset.URL)
						}
						candidateURLs[assetURL] = struct{}{}
					}
				}
			}
		}
	}

	projectUploadsURL := fmt.Sprintf("%s/uploads/", project.WebURL)
	for _, link := range release.Assets.Links {
		if projectIsPublic || !strings.HasPrefix(link.URL, projectUploadsURL) {
			if _, exists := candidateURLs[link.URL]; !exists {
				asset := &assets.Asset{
					Name:        link.Name,
					DisplayName: fmt.Sprintf("%s (asset link)", link.Name),
					URL:         link.URL,
				}
				candidates = append(candidates, asset)
				checksumAssets = append(checksumAssets, checksumAsset{Name: link.Name, URL: link.URL})
				log.Debugf("Adding %s with URL %s", asset, asset.URL)
			}
			candidateURLs[link.URL] = struct{}{}
		}
	}

	node := goldmark.DefaultParser().Parse(text.NewReader([]byte(release.Description)))
	walker := func(n goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		if n.Type() == goldast.TypeInline && n.Kind() == goldast.KindLink {
			link := n.(*goldast.Link)
			name := string(link.Title)
			assetURL := string(link.Destination)
			if projectIsPublic || !strings.HasPrefix(assetURL, projectUploadsURL) {
				if _, exists := candidateURLs[assetURL]; !exists {
					asset := &assets.Asset{
						Name:        name,
						DisplayName: fmt.Sprintf("%s (from release description)", name),
						URL:         assetURL,
					}
					candidates = append(candidates, asset)
					checksumAssets = append(checksumAssets, checksumAsset{Name: name, URL: assetURL})
					log.Debugf("Adding %s with URL %s", asset, asset.URL)
				}
				candidateURLs[assetURL] = struct{}{}
			}
		}
		return goldast.WalkContinue, nil
	}
	if err := goldast.Walk(node, walker); err != nil {
		return nil, err
	}
	log.Debugf("Collected %d GitLab candidate assets for %s/%s", len(candidates), g.owner, g.repo)

	f := assets.NewFilter(&assets.FilterOpts{
		SkipScoring:    opts.All,
		PackagePath:    opts.PackagePath,
		SkipPathCheck:  opts.SkipPatchCheck,
		SystemPackage:  opts.SystemPackage,
		PackageType:    opts.PackageType,
		NonInteractive: opts.NonInteractive,
	})

	autoSelect := f.ParseAutoSelection(opts.AutoSelect)
	log.Debugf("Filtering %d GitLab assets for %s/%s (autoSelect=%q)", len(candidates), g.owner, g.repo, autoSelect)
	gf, err := f.FilterAssets(g.repo, candidates, autoSelect)
	if err != nil {
		log.WithError(err).Debugf("GitLab asset filtering failed for %s/%s", g.owner, g.repo)
		return nil, err
	}
	log.Debugf("Selected GitLab asset %q from %s/%s", gf.Name, g.owner, g.repo)

	if g.token != "" {
		if gf.ExtraHeaders == nil {
			gf.ExtraHeaders = map[string]string{}
		}
		gf.ExtraHeaders["PRIVATE-TOKEN"] = g.token
	}

	outFile, err := f.ProcessURL(gf)
	if err != nil {
		log.WithError(err).Debugf("GitLab asset processing failed for %s/%s asset %q", g.owner, g.repo, gf.Name)
		return nil, err
	}

	expectedSHA := ""
	if outFile.Name == gf.Name {
		expectedSHA, err = expectedSHA256ForAsset(outFile.Name, checksumAssets, gf.ExtraHeaders)
		if err != nil {
			log.WithError(err).Debugf("GitLab checksum lookup failed for %s/%s asset %q", g.owner, g.repo, outFile.Name)
			return nil, err
		}
	}

	version := release.TagName

	file := &File{
		Data:        outFile.Source,
		Name:        outFile.Name,
		Version:     version,
		ExpectedSHA: expectedSHA,
		PublishedAt: gitLabPublishedAt(release),
	}

	return file, nil
}

func (g *gitLab) GetID() string {
	return "gitlab"
}

func (g *gitLab) Cleanup(_ *CleanupOpts) error {
	return nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *gitLab) GetLatestVersion() (*ReleaseInfo, error) {
	log.Debugf("Getting latest release for %s/%s", g.owner, g.repo)
	projectPath := fmt.Sprintf("%s/%s", g.owner, g.repo)

	release, resp, err := g.client.Releases.GetLatestRelease(projectPath, gitlab.WithContext(context.Background()))
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository %s/%s does not have releases", g.owner, g.repo)
	}
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, fmt.Errorf("no release metadata found for %s/%s", g.owner, g.repo)
	}

	return gitLabReleaseInfo(release), nil
}

func (g *gitLab) ListReleases(limit int) ([]*ReleaseInfo, error) {
	if limit <= 0 {
		limit = 100
	}

	projectPath := fmt.Sprintf("%s/%s", g.owner, g.repo)
	releases, resp, err := g.client.Releases.ListReleases(projectPath, &gitlab.ListReleasesOptions{
		ListOptions: gitlab.ListOptions{PerPage: limit},
		Sort:        gitlab.Ptr("desc"),
	})
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository %s/%s does not have releases", g.owner, g.repo)
	}
	if err != nil {
		return nil, err
	}

	history := make([]*ReleaseInfo, 0, len(releases))
	for _, release := range releases {
		history = append(history, gitLabReleaseInfo(release))
	}

	return history, nil
}

func gitLabPublishedAt(release *gitlab.Release) *time.Time {
	if release == nil {
		return nil
	}
	if release.ReleasedAt != nil {
		return release.ReleasedAt
	}
	return release.CreatedAt
}

func gitLabReleaseInfo(release *gitlab.Release) *ReleaseInfo {
	if release == nil {
		return nil
	}

	url := release.Links.Self
	if url == "" {
		url = release.TagPath
	}

	return &ReleaseInfo{
		Version:     release.TagName,
		URL:         url,
		PublishedAt: gitLabPublishedAt(release),
		Body:        release.Description,
	}
}

func newGitLab(u *url.URL) (Provider, error) {
	s := strings.Split(u.Path, "/")
	if len(s) < 3 {
		return nil, fmt.Errorf("Error parsing GitLab URL %s, can't find owner and repo", u.String())
	}

	// it's a specific releases URL
	var tag string
	if strings.Contains(u.Path, "/releases/") {
		// For release URL's, the
		// path is usually /releases/v0.1.
		ps := strings.Split(u.Path, "/")
		for i, p := range ps {
			if p == "releases" {
				tag = strings.Join(ps[i+1:], "/")
			}
		}

	}

	token := os.Getenv("GITLAB_TOKEN")
	hostnameSpecificEnvVarName := fmt.Sprintf("GITLAB_TOKEN_%s", strings.ReplaceAll(u.Hostname(), `.`, "_"))
	hostnameSpecificToken := os.Getenv(hostnameSpecificEnvVarName)
	if hostnameSpecificToken != "" {
		token = hostnameSpecificToken
	}
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(fmt.Sprintf("https://%s/api/v4", u.Hostname())))
	if err != nil {
		return nil, err
	}
	return &gitLab{url: u, client: client, token: token, owner: s[1], repo: s[2], tag: tag}, nil
}
