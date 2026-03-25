package providers

import (
	"fmt"
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
