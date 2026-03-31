package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/caarlos0/log"
)

type goinstall struct {
	name, repo, subPath, tag, latestURL string
	cachedVersionInfo                   *goInstallVersionInfo
}

type goInstallVersionInfo struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
}

func parseRepo(path string) (string, string, string, string) {
	repo := path
	tag := "latest"
	if i := strings.LastIndex(path, "@"); i > -1 {
		repo = filepath.Clean(path[:i])
		tag = path[i+1:]
	}

	name := path
	if i := strings.LastIndex(repo, "/"); i > -1 {
		name = repo[i+1:]
	}

	latestURL := fmt.Sprintf("https://proxy.golang.org/%s/@latest", repo)

	return repo, tag, name, latestURL
}

func versionInfoURL(repo, version string) string {
	return fmt.Sprintf("https://proxy.golang.org/%s/@v/%s.info", repo, version)
}

func newGoInstall(repo string) (Provider, error) {
	repoUrl := strings.TrimPrefix(repo, "goinstall://")
	repo, tag, name, latestURL := parseRepo(repoUrl)
	return &goinstall{repo: repo, tag: tag, name: name, latestURL: latestURL}, nil
}

// resolveSubPath probes for the Go module root and splits the repo into a
// base module path and sub-path. This is deferred to Fetch time to avoid
// shelling out to "go list -m" during provider construction.
func (g *goinstall) resolveSubPath() {
	repoUrlNoVer := moduleRemoveVersion(g.repo)

	baseRepo, found := baseModulePath(repoUrlNoVer)
	if !found || baseRepo == repoUrlNoVer {
		return
	}

	g.subPath = strings.TrimPrefix(repoUrlNoVer, baseRepo)
	g.repo = baseRepo
	log.Debugf("Using base module %s with sub path %q", baseRepo, g.subPath)
}

// moduleRemoveVersion strips an @version suffix from a module path.
func moduleRemoveVersion(mod string) string {
	if i := strings.LastIndex(mod, "@"); i > -1 {
		return mod[:i]
	}
	return mod
}

// baseModulePath walks the import path from longest to shortest, calling
// "go list -m" to find the longest prefix that is a valid Go module root.
// It returns the module path and true if a shorter root was found, or
// ("", false) if the import path itself is already the module root (or
// if no module could be found at all).
func baseModulePath(noVer string) (string, bool) {
	return baseModulePathWith(noVer, func(mod string) (string, error) {
		out, err := exec.Command("go", "list", "-m", "-f", "{{.Path}}", mod+"@latest").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	})
}

// baseModulePathWith is the testable core of baseModulePath. lister should
// return the resolved module path for the given import path, or a non-nil
// error if the path is not a known module.
func baseModulePathWith(noVer string, lister func(mod string) (string, error)) (string, bool) {
	parts := strings.Split(noVer, "/")
	for len(parts) > 0 {
		mod := strings.Join(parts, "/")
		if found, err := lister(mod); err == nil {
			found = strings.TrimSpace(found)
			// Only report a sub-path case when the resolved root is shorter.
			if found != noVer {
				return found, true
			}
			return found, false
		}
		parts = parts[:len(parts)-1]
	}
	return "", false
}

func getGoPath() (string, error) {
	cmd := exec.Command("go", "env", "GOPATH")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command %v failed: %w, output: %s", cmd, err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *goinstall) Fetch(opts *FetchOpts) (*File, error) {
	g.resolveSubPath()

	goPath, err := getGoPath()
	if err != nil {
		return nil, err
	}

	if (len(g.tag) > 0 && g.tag != "latest") || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			g.tag = opts.Version
		}
		log.Infof("Getting %s release for %s", g.tag, g.repo)
	} else {
		log.Infof("Getting latest release for %s", g.repo)
		versionInfo, err := g.getVersionInfo(g.latestURL)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest version: %w", err)
		}
		g.tag = versionInfo.Version
		g.cachedVersionInfo = versionInfo
	}

	cmd := exec.Command("go", "install", fmt.Sprintf("%s%s@%s", g.repo, g.subPath, g.tag))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to install package: %w", err)
	}

	goBinPath := filepath.Join(goPath, "bin", g.name)

	file, err := os.Open(os.ExpandEnv(goBinPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open path '%s': %w", goBinPath, err)
	}
	defer file.Close()

	// Read file content into memory to avoid leaking file handle
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read binary '%s': %w", goBinPath, err)
	}

	versionInfo := g.cachedVersionInfo
	if versionInfo == nil {
		versionInfo, err = g.getVersionInfo(versionInfoURL(g.repo, g.tag))
		if err != nil {
			return nil, err
		}
	}

	return &File{
		Data:        bytes.NewReader(content),
		Name:        g.name,
		Version:     g.tag,
		PublishedAt: PtrTime(versionInfo.Time),
	}, nil
}

func (g *goinstall) GetLatestVersion() (*ReleaseInfo, error) {
	releaseInfo, err := g.getVersionInfo(g.latestURL)
	if err != nil {
		return nil, err
	}

	return &ReleaseInfo{
		Version:     releaseInfo.Version,
		URL:         g.repo,
		PublishedAt: PtrTime(releaseInfo.Time),
	}, nil
}

// maxVersionInfoSize limits the response size for version info to prevent DoS.
const maxVersionInfoSize = 1 << 20 // 1 MB

func (g *goinstall) getVersionInfo(versionURL string) (*goInstallVersionInfo, error) {
	resp, err := http.Get(versionURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Limit response size to prevent resource exhaustion
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVersionInfoSize))
	if err != nil {
		return nil, err
	}

	var result goInstallVersionInfo
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding version info from %s: %w", versionURL, err)
	}
	if result.Version == "" {
		return nil, fmt.Errorf("version not found in response from %s", versionURL)
	}
	return &result, nil
}

func (g *goinstall) GetID() string {
	return "goinstall"
}

func (g *goinstall) Cleanup(_ *CleanupOpts) error {
	return nil
}
