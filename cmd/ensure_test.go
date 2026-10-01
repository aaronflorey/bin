package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func TestIsPackagePathSelectionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "typed missing archive member",
			err:  &assets.ArchiveMemberResolutionError{Reason: assets.ArchiveMemberNoEligible},
			want: true,
		},
		{
			name: "typed invalid stored archive member",
			err:  &assets.ArchiveMemberResolutionError{Reason: assets.ArchiveMemberInvalidSelection},
			want: true,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "tar archive package path mismatch",
			err:  errors.New("no files found in tar archive, use -p flag to manually select . PackagePath [foo/bar]"),
			want: true,
		},
		{
			name: "zip archive package path mismatch",
			err:  errors.New("No files found in zip archive. PackagePath [foo/bar]"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("network timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPackagePathSelectionError(tt.err)
			if got != tt.want {
				t.Fatalf("isPackagePathSelectionError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureAppliesPersistedSelectionIntent(t *testing.T) {
	installDir := setupTestConfig(t)
	path := filepath.Join(installDir, "missing-tool")
	shell := "bash"
	if err := config.UpsertBinary(&config.Binary{Path: path, RemoteName: "missing-tool", Version: "1.0.0", URL: "https://example.test/tool", Provider: "github", SourceAsset: "tool-cli-v1.0.0-linux-amd64-musl.tar.gz", PackagePath: "tool-v1/bin/tool", CompletionShell: &shell}); err != nil {
		t.Fatal(err)
	}

	previousFactory := installProviderFactory
	t.Cleanup(func() { installProviderFactory = previousFactory })
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "github", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if opts.SelectionIntent == nil || opts.SelectionIntent.LogicalProduct != "tool" || opts.SelectionIntent.ArchiveMember != "bin/tool" {
				t.Fatalf("ensure derived selection intent = %#v", opts.SelectionIntent)
			}
			if opts.BundledCompletionShell != shell || opts.BundledCompletionCommand != "missing-tool" {
				t.Fatalf("ensure bundled completion request = (%q, %q)", opts.BundledCompletionShell, opts.BundledCompletionCommand)
			}
			return &providers.File{
				Data:              strings.NewReader(testRunnablePayloadString(t)),
				Name:              "missing-tool",
				Version:           "1.0.0",
				BundledCompletion: []byte("bundled completion"),
				SourceAsset:       "tool-cli-v2.0.0-linux-amd64-musl.tar.gz",
				PackagePath:       "tool-v2/bin/tool",
				SelectionIntent:   opts.SelectionIntent,
			}, nil
		}}, nil
	}

	if err := runEnsure(nil); err != nil {
		t.Fatalf("runEnsure() error = %v", err)
	}
	ensured := config.Get().Bins[path]
	if ensured.SourceAsset != "tool-cli-v2.0.0-linux-amd64-musl.tar.gz" || ensured.PackagePath != "tool-v2/bin/tool" {
		t.Fatalf("ensure did not preserve updated provenance: %#v", ensured)
	}
	if ensured.SelectionIntent == nil || ensured.SelectionIntent.LogicalProduct != "tool" || ensured.SelectionIntent.ArchiveMember != "bin/tool" {
		t.Fatalf("ensure did not persist derived selection intent: %#v", ensured.SelectionIntent)
	}
	if ensured.CompletionShell == nil || *ensured.CompletionShell != shell {
		t.Fatalf("ensure did not preserve completion policy: %#v", ensured)
	}
	assertCompletionContent(t, shell, "missing-tool", "bundled completion")
}

func TestEnsureUsesPersistedDMGExecutablePath(t *testing.T) {
	setupTestConfig(t)
	applications := t.TempDir()
	previousApplicationsDir := applicationsDir
	applicationsDir = applications
	t.Cleanup(func() { applicationsDir = previousApplicationsDir })

	actualPath := filepath.Join(applications, "Fastpotify.app", "Contents", "MacOS", "Fastpotify-bin")
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o755); err != nil {
		t.Fatalf("create app executable directory: %v", err)
	}
	writeTestBinary(t, actualPath)
	if err := config.UpsertBinary(&config.Binary{
		Path: actualPath, RemoteName: "spotify", Version: "1.0.0", Hash: "different", URL: "https://example.test/fastpotify", Provider: "github",
		InstallMode: installModeSystemPackage, PackageType: "dmg", AppBundle: "Fastpotify.app",
	}); err != nil {
		t.Fatal(err)
	}

	originalRegistry := lifecycleRegistry
	t.Cleanup(func() { lifecycleRegistry = originalRegistry })
	installed := false
	lifecycleRegistry = map[string]lifecycleStrategy{
		installModeSystemPackage: {
			applyStoredFetch: originalRegistry[installModeSystemPackage].applyStoredFetch,
			install: func(opts InstallOpts) (*InstallResult, error) {
				installed = true
				if opts.Path != actualPath || opts.AppBundle != "Fastpotify.app" || opts.FetchOpts.PackageName != "spotify" {
					t.Fatalf("ensure install options = %#v, want persisted executable and bundle", opts)
				}
				return &InstallResult{Version: opts.FetchOpts.Version, Path: opts.Path}, nil
			},
			resolvePath: originalRegistry[installModeSystemPackage].resolvePath,
		},
	}

	if err := runEnsure(nil); err != nil {
		t.Fatalf("runEnsure() error = %v", err)
	}
	if !installed {
		t.Fatal("expected ensure to reinstall the mismatched executable")
	}
}
