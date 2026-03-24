package providers

import "testing"

func TestNormalizeGitHubURL(t *testing.T) {
	cases := []struct {
		name               string
		rawURL             string
		provider           string
		expectedURL        string
		expectedVersion    string
		expectedHasVersion bool
	}{
		{
			name:               "normalizes github https repository URL",
			rawURL:             "https://github.com/foo/bar",
			expectedURL:        "github.com/foo/bar",
			expectedVersion:    "",
			expectedHasVersion: false,
		},
		{
			name:               "normalizes github schemeless repository URL",
			rawURL:             "github.com/foo/bar",
			expectedURL:        "github.com/foo/bar",
			expectedVersion:    "",
			expectedHasVersion: false,
		},
		{
			name:               "extracts release tag URL",
			rawURL:             "https://github.com/foo/bar/releases/tag/v0.24.4",
			expectedURL:        "github.com/foo/bar",
			expectedVersion:    "v0.24.4",
			expectedHasVersion: true,
		},
		{
			name:               "extracts release download URL",
			rawURL:             "https://github.com/foo/bar/releases/download/v1.2.3/tool-linux-amd64",
			expectedURL:        "github.com/foo/bar",
			expectedVersion:    "v1.2.3",
			expectedHasVersion: true,
		},
		{
			name:               "leaves non github URL unchanged",
			rawURL:             "https://gitlab.com/foo/bar",
			expectedURL:        "https://gitlab.com/foo/bar",
			expectedVersion:    "",
			expectedHasVersion: false,
		},
		{
			name:               "normalizes when provider forced to github",
			rawURL:             "https://example.test/foo/bar",
			provider:           "github",
			expectedURL:        "github.com/foo/bar",
			expectedVersion:    "",
			expectedHasVersion: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualURL, actualVersion, actualHasVersion, err := NormalizeGitHubURL(tc.rawURL, tc.provider)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if actualURL != tc.expectedURL {
				t.Fatalf("unexpected URL: got %q, want %q", actualURL, tc.expectedURL)
			}
			if actualVersion != tc.expectedVersion {
				t.Fatalf("unexpected version: got %q, want %q", actualVersion, tc.expectedVersion)
			}
			if actualHasVersion != tc.expectedHasVersion {
				t.Fatalf("unexpected hasVersion: got %t, want %t", actualHasVersion, tc.expectedHasVersion)
			}
		})
	}
}
