package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/log"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/hashicorp/go-version"
)

type docker struct {
	client    *client.Client
	repo, tag string
	http      *http.Client
	tagsURL   string
}

type dockerHubTag struct {
	Name        string    `json:"name"`
	LastUpdated time.Time `json:"last_updated"`
}

type dockerHubTagResponse struct {
	Next    string         `json:"next"`
	Results []dockerHubTag `json:"results"`
}

func (d *docker) Fetch(opts *FetchOpts) (*File, error) {
	if len(opts.Version) > 0 {
		// this is used by for the `ensure` command
		d.tag = opts.Version
	}
	log.Infof("Pulling docker image %s:%s", d.repo, d.tag)
	out, err := d.client.ImageCreate(context.Background(), fmt.Sprintf("%s:%s", d.repo, d.tag), image.CreateOptions{})
	if err != nil {
		return nil, err
	}
	defer out.Close()

	err = jsonmessage.DisplayJSONMessagesStream(
		out,
		os.Stderr,
		os.Stdout.Fd(),
		false,
		nil)
	if err != nil {
		return nil, err
	}

	return &File{
		Data:    strings.NewReader(fmt.Sprintf(sh, d.repo, d.tag)),
		Name:    getImageName(d.repo),
		Version: d.tag,
	}, nil
}

func (d *docker) GetLatestVersion() (*ReleaseInfo, error) {
	repo, err := dockerHubRepoPath(d.repo)
	if err != nil {
		return &ReleaseInfo{
			Version: d.tag,
			URL:     fmt.Sprintf("docker://%s:%s", d.repo, d.tag),
		}, nil
	}

	tags, err := d.getDockerHubTags(repo)
	if err != nil {
		return nil, err
	}

	latestTag, publishedAt := latestDockerTag(d.tag, tags)

	return &ReleaseInfo{
		Version:     latestTag,
		URL:         fmt.Sprintf("docker://%s:%s", d.repo, latestTag),
		PublishedAt: publishedAt,
	}, nil
}

func (d *docker) Cleanup(opts *CleanupOpts) error {
	if opts != nil && opts.Version != "" {
		d.tag = opts.Version
	}

	ref := fmt.Sprintf("%s:%s", d.repo, d.tag)
	_, err := d.client.ImageRemove(context.Background(), ref, image.RemoveOptions{PruneChildren: true})
	return err
}

func (d *docker) GetID() string {
	return "docker"
}

func newDocker(imageURL string) (Provider, error) {
	imageURL = strings.TrimPrefix(imageURL, "docker://")

	repo, tag := parseImage(imageURL)

	// Validate repo and tag to prevent shell/batch injection
	if !validateDockerImage(repo) {
		return nil, fmt.Errorf("invalid docker repository name: %q", repo)
	}
	if !validateDockerImage(tag) {
		return nil, fmt.Errorf("invalid docker tag: %q", tag)
	}

	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &docker{
		repo:    repo,
		tag:     tag,
		client:  c,
		http:    &http.Client{Timeout: 10 * time.Second},
		tagsURL: "https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=100",
	}, nil
}

func (d *docker) getDockerHubTags(repo string) ([]dockerHubTag, error) {
	next := fmt.Sprintf(d.tagsURL, repo)
	tags := []dockerHubTag{}

	for next != "" {
		resp, err := d.http.Get(next)
		if err != nil {
			return nil, fmt.Errorf("failed to query docker tags: %w", err)
		}

		var payload dockerHubTagResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
		closeErr := resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to query docker tags: status %d", resp.StatusCode)
		}

		if decodeErr != nil {
			return nil, fmt.Errorf("failed to decode docker tags response: %w", decodeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("failed to close docker tags response body: %w", closeErr)
		}

		tags = append(tags, payload.Results...)
		next = payload.Next
	}

	return tags, nil
}

func latestDockerTag(current string, tags []dockerHubTag) (string, *time.Time) {
	latestTag := current
	latestTime := tagPublishedAt(current, tags)

	currentVersion, currentErr := version.NewVersion(current)
	var latestVersion *version.Version
	if currentErr == nil {
		latestVersion = currentVersion
	}

	for _, tag := range tags {
		candidate, err := version.NewVersion(tag.Name)
		if err != nil {
			continue
		}

		if latestVersion == nil || candidate.GreaterThan(latestVersion) {
			latestVersion = candidate
			latestTag = tag.Name
			t := tag.LastUpdated
			latestTime = &t
		}
	}

	if latestVersion == nil {
		for _, tag := range tags {
			if tag.Name != "latest" {
				continue
			}
			t := tag.LastUpdated
			return tag.Name, &t
		}
		return current, latestTime
	}

	if currentErr == nil && !latestVersion.GreaterThan(currentVersion) {
		return current, latestTime
	}

	return latestTag, latestTime
}

func dockerHubRepoPath(repo string) (string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid docker repository: %q", repo)
	}

	first := parts[0]
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		if first != "docker.io" && first != "index.docker.io" && first != "registry-1.docker.io" {
			return "", fmt.Errorf("docker registry %q is not supported for tag discovery", first)
		}
		repo = strings.Join(parts[1:], "/")
	}

	if repo == "" {
		return "", fmt.Errorf("invalid docker repository: %q", repo)
	}

	return repo, nil
}

func tagPublishedAt(name string, tags []dockerHubTag) *time.Time {
	for _, tag := range tags {
		if tag.Name != name {
			continue
		}
		t := tag.LastUpdated
		return &t
	}

	return nil
}

// parseImage parses the image returning the repository and tag.
// It handles non-canonical URLs like `hashicorp/terraform`.
func parseImage(imageURL string) (string, string) {
	image := imageURL
	tag := "latest"
	if i := strings.LastIndex(imageURL, ":"); i > -1 {
		image = imageURL[:i]
		tag = imageURL[i+1:]
	}

	if strings.Count(imageURL, "/") == 0 {
		image = "library/" + image
	}

	return image, tag
}
