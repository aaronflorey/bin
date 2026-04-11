package providers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ErrInvalidProvider = errors.New("invalid provider")

type File struct {
	Data        io.Reader
	Name        string
	Version     string
	ExpectedSHA string
	Length      int64
	PackagePath string
	PublishedAt *time.Time
}

type ReleaseInfo struct {
	Version     string
	URL         string
	PublishedAt *time.Time
}

func (f *File) Hash() ([]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, f.Data); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

type FetchOpts struct {
	All            bool
	AutoSelect     string
	PackageName    string
	PackagePath    string
	SystemPackage  bool
	PackageType    string
	SkipPatchCheck bool
	Version        string
	NonInteractive bool
}

type CleanupOpts struct {
	Version string
	Path    string
}

type Provider interface {
	// Fetch returns the file metadata to retrieve a specific binary given
	// for a provider
	Fetch(*FetchOpts) (*File, error)
	// GetLatestVersion returns the version and the URL of the
	// latest version for this binary
	GetLatestVersion() (*ReleaseInfo, error)
	// Cleanup removes provider-specific resources for a managed binary.
	Cleanup(*CleanupOpts) error

	// GetID returns the unique identiifer of this provider
	GetID() string
}

var (
	httpUrlPrefix      = regexp.MustCompile("^https?://")
	dockerUrlPrefix    = regexp.MustCompile("^docker://")
	goinstallUrlPrefix = regexp.MustCompile("^goinstall://")
)

func New(u, provider string) (Provider, error) {
	if dockerUrlPrefix.MatchString(u) {
		return newDocker(u)
	}
	if goinstallUrlPrefix.MatchString(u) || provider == "goinstall" {
		return newGoInstall(u)
	}
	if !httpUrlPrefix.MatchString(u) {
		u = fmt.Sprintf("https://%s", u)
	}

	purl, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	if provider == "generic" {
		return newGenericURL(purl)
	}

	if strings.Contains(purl.Host, "github") || provider == "github" {
		return newGitHub(purl)
	}

	if strings.Contains(purl.Host, "gitlab") || provider == "gitlab" {
		return newGitLab(purl)
	}

	if strings.Contains(purl.Host, "codeberg") || provider == "codeberg" {
		return newCodeberg(purl)
	}

	if strings.Contains(purl.Host, "releases.hashicorp.com") || provider == "hashicorp" {
		return newHashiCorp(purl)
	}

	if strings.EqualFold(purl.Scheme, "http") || strings.EqualFold(purl.Scheme, "https") {
		return newGenericURL(purl)
	}

	return nil, fmt.Errorf("can't find provider for url %s", u)
}
