package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestPruneKeepsTrackedDMGExecutable(t *testing.T) {
	setupTestConfig(t)
	applications := t.TempDir()
	path := filepath.Join(applications, "Fastpotify.app", "Contents", "MacOS", "Fastpotify-bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestBinary(t, path)
	if err := config.UpsertBinary(&config.Binary{
		Path: path, RemoteName: "spotify", InstallMode: installModeSystemPackage, PackageType: "dmg", AppBundle: "Fastpotify.app",
	}); err != nil {
		t.Fatal(err)
	}

	cmd := newPruneCmd().cmd
	cmd.SetArgs([]string{"--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("prune error: %v", err)
	}
	if config.Get().Bins[path] == nil {
		t.Fatal("prune removed the managed DMG with its persisted executable present")
	}
}
