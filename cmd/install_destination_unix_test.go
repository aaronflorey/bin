//go:build !windows

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/aaronflorey/bin/pkg/providers"
)

func TestUnixInstallDestinationWithBackslashUsesDefaultPath(t *testing.T) {
	defaultPath := setupTestConfig(t)
	requestedName := `tool\alias`

	originalRegistry := lifecycleRegistry
	originalProviderFactory := installProviderFactory
	t.Cleanup(func() {
		lifecycleRegistry = originalRegistry
		installProviderFactory = originalProviderFactory
	})
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return testFetchProvider{}, nil
	}

	registry := make(map[string]lifecycleStrategy, len(originalRegistry))
	for mode, strategy := range originalRegistry {
		registry[mode] = strategy
	}
	var got InstallOpts
	binaryStrategy := registry[installModeBinary]
	binaryStrategy.install = func(opts InstallOpts) (*InstallResult, error) {
		got = opts
		return &InstallResult{Name: requestedName, Version: "1.0.0", Path: opts.Path}, nil
	}
	registry[installModeBinary] = binaryStrategy
	lifecycleRegistry = registry

	root := newInstallCmd()
	if err := root.installTarget(root.cmd, installTarget{url: "github.com/acme/tool", path: requestedName}); err != nil {
		t.Fatalf("installTarget() error = %v", err)
	}
	want := filepath.Join(defaultPath, requestedName)
	if got.Path != want {
		t.Fatalf("install path = %q, want %q", got.Path, want)
	}
}
