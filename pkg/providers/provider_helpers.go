package providers

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const providerRequestTimeout = 30 * time.Second

func newProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: providerRequestTimeout}
}

func newProviderOAuthHTTPClient(token string) *http.Client {
	client := newProviderHTTPClient()
	client.Transport = &oauth2.Transport{
		Source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
		Base:   http.DefaultTransport,
	}
	return client
}

func newProviderRequestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), providerRequestTimeout)
}

func providerPathSegments(u *url.URL) []string {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func hostIsOrSubdomain(host, target string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	target = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(target)), ".")
	if host == "" || target == "" {
		return false
	}
	return host == target || strings.HasSuffix(host, "."+target)
}

func releaseTagFromSegments(segments []string) string {
	for i := 0; i+2 < len(segments); i++ {
		if segments[i] != "releases" {
			continue
		}
		if segments[i+1] == "tag" || segments[i+1] == "download" {
			return segments[i+2]
		}
	}
	return ""
}

func gitLabReleaseTagFromSegments(segments []string) string {
	for i := 0; i+1 < len(segments); i++ {
		if segments[i] == "releases" {
			return strings.Join(segments[i+1:], "/")
		}
	}
	return ""
}
