package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/caarlos0/log"
)

func TestRemoveForgetsBinaryTakenOverByMiseWithoutDeletingFiles(t *testing.T) {
	installDir := setupTestConfig(t)
	trackedPath := filepath.Join(installDir, "mago")
	completionPath := filepath.Join(t.TempDir(), "_mago")
	if err := os.WriteFile(trackedPath, []byte("old bin-managed executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(completionPath, []byte("completion"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertBinary(&config.Binary{
		Path:       trackedPath,
		RemoteName: "mago",
		Version:    "1.55.0",
		URL:        "github.com/carthage-software/mago",
		Provider:   "github",
		CompletionOwnership: map[string]*config.CompletionOwnershipRecord{
			"bash": {Path: completionPath, SHA256: completionHash([]byte("completion"))},
		},
	}); err != nil {
		t.Fatal(err)
	}

	shim := filepath.Join(t.TempDir(), "mise", "shims", "mago")
	previousLookPath := lookPathCommand
	lookPathCommand = func(name string) (string, error) {
		if name == "mago" {
			return shim, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPathCommand = previousLookPath })

	previousLogger := log.Log
	var logs bytes.Buffer
	logger := log.New(&logs)
	logger.Level = log.WarnLevel
	log.Log = logger
	t.Cleanup(func() { log.Log = previousLogger })

	root := newRemoveCmd()
	root.newProvider = func(string, string) (providers.Provider, error) {
		t.Fatal("provider cleanup must not run for a mise takeover")
		return nil, errors.New("unreachable")
	}
	root.cmd.SetArgs([]string{trackedPath})
	if err := root.cmd.Execute(); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, ok := config.Get().Bins[trackedPath]; ok {
		t.Fatal("stale managed record remains")
	}
	if _, err := os.Stat(trackedPath); err != nil {
		t.Fatalf("tracked executable was deleted: %v", err)
	}
	if _, err := os.Stat(completionPath); err != nil {
		t.Fatalf("owned completion was deleted: %v", err)
	}
	if warning := logs.String(); !strings.Contains(warning, "mago resolves to mise shim") || !strings.Contains(warning, trackedPath) {
		t.Fatalf("takeover warning = %q", warning)
	}
}

func TestFindMiseTakeoversIgnoresOtherShadowingAndSystemPackages(t *testing.T) {
	installDir := t.TempDir()
	directPath := filepath.Join(installDir, "direct")
	systemPath := filepath.Join(installDir, "system")
	activePath := filepath.Join(installDir, "active")
	bins := map[string]*config.Binary{
		directPath: {Path: directPath, RemoteName: "direct"},
		systemPath: {Path: systemPath, RemoteName: "system", InstallMode: installModeSystemPackage, PackageType: "dmg"},
		activePath: {Path: activePath, RemoteName: "active"},
	}

	previousLookPath := lookPathCommand
	lookPathCommand = func(name string) (string, error) {
		switch name {
		case "direct":
			return filepath.Join(t.TempDir(), "other", "direct"), nil
		case "system":
			return filepath.Join(t.TempDir(), "mise", "shims", "system"), nil
		case "active":
			return activePath, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	t.Cleanup(func() { lookPathCommand = previousLookPath })

	if takeovers := findMiseTakeovers(bins); len(takeovers) != 0 {
		t.Fatalf("false-positive takeovers: %#v", takeovers)
	}
}

func TestReconcileMiseTakeoversDryRunDoesNotMutateRegistry(t *testing.T) {
	installDir := setupTestConfig(t)
	trackedPath := filepath.Join(installDir, "tool")
	if err := config.UpsertBinary(&config.Binary{Path: trackedPath, RemoteName: "tool"}); err != nil {
		t.Fatal(err)
	}

	previousLookPath := lookPathCommand
	lookPathCommand = func(name string) (string, error) {
		if name == "tool" {
			return filepath.Join(t.TempDir(), "mise", "shims", "tool"), nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPathCommand = previousLookPath })

	previousLogger := log.Log
	var logs bytes.Buffer
	logger := log.New(&logs)
	logger.Level = log.WarnLevel
	log.Log = logger
	t.Cleanup(func() { log.Log = previousLogger })

	remaining, err := reconcileMiseTakeovers(config.Get().Bins, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("dry-run working set = %#v", remaining)
	}
	if _, ok := config.Get().Bins[trackedPath]; !ok {
		t.Fatal("dry-run removed the registry entry")
	}
	if !strings.Contains(logs.String(), "would be removed") {
		t.Fatalf("dry-run warning = %q", logs.String())
	}
}

func TestResolveBinsToProcessFindsManagedAliasBehindMiseShim(t *testing.T) {
	installDir := setupTestConfig(t)
	trackedPath := filepath.Join(installDir, "tool")
	if runtime.GOOS == "windows" {
		trackedPath += ".exe"
	}
	if err := config.UpsertBinary(&config.Binary{Path: trackedPath, RemoteName: "tool"}); err != nil {
		t.Fatal(err)
	}

	shimDir := filepath.Join(t.TempDir(), "mise", "shims")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shimName := "tool"
	if runtime.GOOS == "windows" {
		shimName += ".exe"
		t.Setenv("PATHEXT", ".EXE")
	}
	if err := os.WriteFile(filepath.Join(shimDir, shimName), []byte("shim"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir)

	bins, err := resolveBinsToProcess(config.Get().Bins, []string{"tool"})
	if err != nil {
		t.Fatalf("resolve mise-shadowed alias: %v", err)
	}
	if len(bins) != 1 || bins[trackedPath] == nil {
		t.Fatalf("resolved bins = %#v", bins)
	}
}
