package providers

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeGitHubURL canonicalizes github repository URLs to host/owner/repo.
// It also extracts a version tag when a release URL is provided.
func NormalizeGitHubURL(rawURL, provider string) (normalizedURL, version string, hasExplicitVersion bool, err error) {
	if strings.TrimSpace(rawURL) == "" {
		return rawURL, "", false, nil
	}

	if !looksLikeGitHubURL(rawURL, provider) {
		return rawURL, "", false, nil
	}

	parseTarget := rawURL
	if !httpUrlPrefix.MatchString(parseTarget) {
		parseTarget = fmt.Sprintf("https://%s", parseTarget)
	}

	u, err := url.Parse(parseTarget)
	if err != nil {
		return "", "", false, err
	}

	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(segments) < 2 {
		return rawURL, "", false, nil
	}

	owner, repo := segments[0], segments[1]
	normalized := fmt.Sprintf("github.com/%s/%s", owner, repo)

	if len(segments) >= 5 && segments[2] == "releases" {
		switch segments[3] {
		case "tag", "download":
			version = segments[4]
			if version != "" {
				return normalized, version, true, nil
			}
		}
	}

	return normalized, "", false, nil
}

func looksLikeGitHubURL(rawURL, provider string) bool {
	if provider == "github" {
		return true
	}

	candidate := strings.TrimSpace(rawURL)
	candidate = strings.TrimPrefix(candidate, "https://")
	candidate = strings.TrimPrefix(candidate, "http://")
	candidate = strings.TrimPrefix(candidate, "www.")
	return strings.HasPrefix(strings.ToLower(candidate), "github.com/")
}
