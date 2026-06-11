package providers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModuleRemoveVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{name: "no version", in: "github.com/example/tool", out: "github.com/example/tool"},
		{name: "with version", in: "github.com/example/tool@v1.2.3", out: "github.com/example/tool"},
		{name: "with latest", in: "github.com/example/tool@latest", out: "github.com/example/tool"},
		{name: "sub-path with version", in: "github.com/example/tool/cmd/mytool@v1.0.0", out: "github.com/example/tool/cmd/mytool"},
		{name: "empty string", in: "", out: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := moduleRemoveVersion(c.in)
			if got != c.out {
				t.Errorf("moduleRemoveVersion(%q) = %q, want %q", c.in, got, c.out)
			}
		})
	}
}

func TestBaseModulePathWith(t *testing.T) {
	// lister simulates go list -m: returns the module path only for known modules.
	lister := func(knownModules map[string]string) func(mod string) (string, error) {
		return func(mod string) (string, error) {
			if v, ok := knownModules[mod]; ok {
				return v, nil
			}
			return "", fmt.Errorf("no module")
		}
	}

	cases := []struct {
		name         string
		input        string
		modules      map[string]string
		wantPath     string
		wantSubFound bool
	}{
		{
			name:  "no sub-path: module root matches full path",
			input: "github.com/example/tool",
			modules: map[string]string{
				"github.com/example/tool": "github.com/example/tool",
			},
			wantPath:     "github.com/example/tool",
			wantSubFound: false,
		},
		{
			name:  "sub-path: cmd/mytool lives under base module",
			input: "github.com/example/tool/cmd/mytool",
			modules: map[string]string{
				"github.com/example/tool": "github.com/example/tool",
			},
			wantPath:     "github.com/example/tool",
			wantSubFound: true,
		},
		{
			name:         "no module found at all",
			input:        "github.com/nonexistent/thing/cmd/foo",
			modules:      map[string]string{},
			wantPath:     "",
			wantSubFound: false,
		},
		{
			name:  "sub-path two levels deep",
			input: "github.com/example/suite/pkg/sub/cmd/tool",
			modules: map[string]string{
				"github.com/example/suite": "github.com/example/suite",
			},
			wantPath:     "github.com/example/suite",
			wantSubFound: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPath, gotFound := baseModulePathWith(c.input, lister(c.modules))
			if gotPath != c.wantPath {
				t.Errorf("path: got %q, want %q", gotPath, c.wantPath)
			}
			if gotFound != c.wantSubFound {
				t.Errorf("found: got %v, want %v", gotFound, c.wantSubFound)
			}
		})
	}
}

func TestNewGoInstallSubPath(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		wantRepo    string
		wantSubPath string
		wantTag     string
		wantName    string
	}{
		{
			name:        "simple module, no sub-path",
			url:         "goinstall://github.com/example/tool",
			wantRepo:    "github.com/example/tool",
			wantSubPath: "",
			wantTag:     "latest",
			wantName:    "tool",
		},
		{
			name:        "simple module with version, no sub-path",
			url:         "goinstall://github.com/example/tool@v1.2.3",
			wantRepo:    "github.com/example/tool",
			wantSubPath: "",
			wantTag:     "v1.2.3",
			wantName:    "tool",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := newGoInstall(c.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			g, ok := p.(*goinstall)
			if !ok {
				t.Fatalf("expected *goinstall")
			}
			if g.repo != c.wantRepo {
				t.Errorf("repo: got %q, want %q", g.repo, c.wantRepo)
			}
			if g.subPath != c.wantSubPath {
				t.Errorf("subPath: got %q, want %q", g.subPath, c.wantSubPath)
			}
			if g.tag != c.wantTag {
				t.Errorf("tag: got %q, want %q", g.tag, c.wantTag)
			}
			if g.name != c.wantName {
				t.Errorf("name: got %q, want %q", g.name, c.wantName)
			}
		})
	}
}

func TestSubPathLatestUsesBaseModule(t *testing.T) {
	// Simulate: repo is a sub-package, resolveSubPath finds the base module.
	// After resolution, latestURL() must point to the base module, not the sub-package.
	p, err := newGoInstall("goinstall://go.kenn.io/kata/cmd/kata@latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := p.(*goinstall)

	// Mock the module resolver: go.kenn.io/kata is the base module.
	resolver := func(noVer string) (string, bool) {
		if noVer == "go.kenn.io/kata/cmd/kata" {
			return "go.kenn.io/kata", true
		}
		return "", false
	}

	g.resolveSubPathWith(resolver)

	if g.repo != "go.kenn.io/kata" {
		t.Errorf("repo: got %q, want %q", g.repo, "go.kenn.io/kata")
	}
	if g.subPath != "/cmd/kata" {
		t.Errorf("subPath: got %q, want %q", g.subPath, "/cmd/kata")
	}

	wantLatestURL := "https://proxy.golang.org/go.kenn.io/kata/@latest"
	if got := g.latestURL(); got != wantLatestURL {
		t.Errorf("latestURL(): got %q, want %q", got, wantLatestURL)
	}

	// Also verify GetLatestVersion sends the request to the base module URL.
	var requestedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"Version":"v1.2.3","Time":"2024-01-01T00:00:00Z"}`)
	}))
	defer ts.Close()

	g.httpClient = ts.Client()
	// Point the proxy base to our test server.
	g.repo = strings.TrimPrefix(ts.URL, "https://")
	// Since we're testing URL routing, override latestURL to use the test server.
	// Reset repo to the base module so latestURL() builds the right path.
	g.repo = "go.kenn.io/kata"

	// Replace httpClient with a transport that redirects to our test server.
	g.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = strings.TrimPrefix(ts.URL, "http://")
			return ts.Client().Transport.RoundTrip(req)
		}),
	}

	_, _ = g.GetLatestVersion()

	// The request should have gone to /go.kenn.io/kata/@latest, not /go.kenn.io/kata/cmd/kata/@latest.
	if !strings.Contains(requestedURL, "/go.kenn.io/kata/@latest") {
		t.Errorf("expected request to base module @latest, got %q", requestedURL)
	}
	if strings.Contains(requestedURL, "/cmd/kata") {
		t.Errorf("request should not include sub-path, got %q", requestedURL)
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetVersionInfoNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "module not found")
	}))
	defer ts.Close()

	g := &goinstall{httpClient: ts.Client()}
	_, err := g.getVersionInfo(ts.URL + "/@latest")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "404") {
		t.Errorf("error should mention status code 404, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "module not found") {
		t.Errorf("error should include response body snippet, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, ts.URL) {
		t.Errorf("error should include request URL, got: %s", errMsg)
	}
}

func TestGetVersionInfo500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
	}))
	defer ts.Close()

	g := &goinstall{httpClient: ts.Client()}
	_, err := g.getVersionInfo(ts.URL + "/@latest")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "500") {
		t.Errorf("error should mention status code 500, got: %s", errMsg)
	}
}
