package providers

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/hashicorp/go-version"
)

const genericFallbackFilename = "downloaded-binary"

var semverFilenameRegexp = regexp.MustCompile(`(?i)v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`)

type genericURL struct {
	url    *url.URL
	client *http.Client
}

type metadataResult struct {
	statusCode         int
	finalURL           string
	contentDisposition string
}

func newGenericURL(u *url.URL) (Provider, error) {
	return &genericURL{
		url:    u,
		client: http.DefaultClient,
	}, nil
}

func (g *genericURL) Fetch(_ *FetchOpts) (*File, error) {
	metadata, err := g.probeMetadata()
	if err != nil {
		return nil, err
	}

	filename := resolvedFilename(metadata.contentDisposition, metadata.finalURL, g.url.String())
	version := extractVersionFromFilename(filename)
	if version == "" {
		return nil, fmt.Errorf("unable to infer version from filename %q", filename)
	}
	name := assets.SanitizeName(filename, version)
	if name == "" {
		name = filename
	}

	req, err := http.NewRequest(http.MethodGet, g.url.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		return nil, fmt.Errorf("downloading %s failed with status %d", g.url.String(), resp.StatusCode)
	}

	return &File{
		Data:    resp.Body,
		Name:    name,
		Version: version,
		Length:  resp.ContentLength,
	}, nil
}

func (g *genericURL) GetLatestVersion() (*ReleaseInfo, error) {
	metadata, err := g.probeMetadata()
	if err != nil {
		return nil, err
	}

	filename := resolvedFilename(metadata.contentDisposition, metadata.finalURL, g.url.String())
	ver := extractVersionFromFilename(filename)
	if ver == "" {
		return nil, fmt.Errorf("unable to infer version from filename %q", filename)
	}

	releaseURL := metadata.finalURL
	if releaseURL == "" {
		releaseURL = g.url.String()
	}

	return &ReleaseInfo{
		Version: ver,
		URL:     releaseURL,
	}, nil
}

func (g *genericURL) Cleanup(_ *CleanupOpts) error {
	return nil
}

func (g *genericURL) GetID() string {
	return "generic"
}

func (g *genericURL) probeMetadata() (*metadataResult, error) {
	headResult, err := g.metadataRequest(http.MethodHead, false)
	if err != nil {
		return nil, err
	}

	if headResult.statusCode == http.StatusMethodNotAllowed ||
		headResult.statusCode == http.StatusNotImplemented ||
		(headResult.contentDisposition == "" && filenameFromURL(headResult.finalURL) == "") {
		return g.metadataRequest(http.MethodGet, true)
	}

	return headResult, nil
}

func (g *genericURL) metadataRequest(method string, lightweight bool) (*metadataResult, error) {
	req, err := http.NewRequest(method, g.url.String(), nil)
	if err != nil {
		return nil, err
	}
	if lightweight {
		req.Header.Set("Range", "bytes=0-0")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest &&
		resp.StatusCode != http.StatusMethodNotAllowed &&
		resp.StatusCode != http.StatusNotImplemented {
		return nil, fmt.Errorf("probing %s failed with status %d", g.url.String(), resp.StatusCode)
	}

	result := &metadataResult{
		statusCode:         resp.StatusCode,
		contentDisposition: resp.Header.Get("Content-Disposition"),
	}
	if resp.Request != nil && resp.Request.URL != nil {
		result.finalURL = resp.Request.URL.String()
	}

	return result, nil
}

func extractVersionFromFilename(name string) string {
	matches := semverFilenameRegexp.FindAllString(name, -1)
	if len(matches) == 0 {
		return ""
	}

	var highest *version.Version
	for _, raw := range matches {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		sv, err := version.NewVersion(raw)
		if err != nil {
			continue
		}

		if highest == nil || sv.GreaterThan(highest) {
			highest = sv
		}
	}

	if highest == nil {
		return ""
	}

	return highest.String()
}

func filenameFromContentDisposition(headerValue string) string {
	if headerValue == "" {
		return ""
	}

	_, params, err := mime.ParseMediaType(headerValue)
	if err != nil {
		return ""
	}

	if filename, ok := params["filename"]; ok {
		filename = strings.TrimSpace(filename)
		if filename != "" {
			return filename
		}
	}

	if filename, ok := params["filename*"]; ok {
		if i := strings.Index(filename, "''"); i >= 0 {
			filename = filename[i+2:]
		}
		filename, err = url.QueryUnescape(filename)
		if err != nil {
			return ""
		}
		filename = strings.TrimSpace(filename)
		if filename != "" {
			return filename
		}
	}

	return ""
}

func filenameFromURL(raw string) string {
	if raw == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	base := path.Base(strings.TrimSpace(u.Path))
	switch base {
	case "", ".", "/":
		return ""
	default:
		return base
	}
}

func resolvedFilename(contentDisposition, finalURL, originalURL string) string {
	if n := filenameFromContentDisposition(contentDisposition); n != "" {
		return n
	}
	if n := filenameFromURL(finalURL); n != "" {
		return n
	}
	if n := filenameFromURL(originalURL); n != "" {
		return n
	}
	return genericFallbackFilename
}
