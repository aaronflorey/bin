package cmd

import (
	"errors"
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
	if err := config.UpsertBinary(&config.Binary{Path: path, RemoteName: "alias", Version: "1.0.0", URL: "https://example.test/tool", Provider: "github", SourceAsset: "tool-cli-v1.0.0-linux-amd64-musl.tar.gz", PackagePath: "tool-v1/bin/tool"}); err != nil {
		t.Fatal(err)
	}

	previousFactory := installProviderFactory
	t.Cleanup(func() { installProviderFactory = previousFactory })
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "github", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if opts.SelectionIntent == nil || opts.SelectionIntent.LogicalProduct != "tool" || opts.SelectionIntent.ArchiveMember != "bin/tool" {
				t.Fatalf("ensure derived selection intent = %#v", opts.SelectionIntent)
			}
			return &providers.File{
				Data:            strings.NewReader("#!/bin/sh\nexit 0\n"),
				Name:            "tool",
				Version:         "1.0.0",
				SourceAsset:     "tool-cli-v2.0.0-linux-amd64-musl.tar.gz",
				PackagePath:     "tool-v2/bin/tool",
				SelectionIntent: opts.SelectionIntent,
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
}
