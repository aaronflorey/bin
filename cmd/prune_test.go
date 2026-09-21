package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/caarlos0/log"
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

func TestPrunePreservesUnsafeModifiedAndConflictingCompletions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, destination string)
		reason  string
	}{
		{
			name: "modified file",
			prepare: func(t *testing.T, destination string) {
				t.Helper()
				if err := os.WriteFile(destination, []byte("modified"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			reason: "was modified",
		},
		{
			name: "non-regular file",
			prepare: func(t *testing.T, destination string) {
				t.Helper()
				if err := os.Remove(destination); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(destination, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			reason: "non-regular",
		},
		{
			name: "another binary claim",
			prepare: func(t *testing.T, destination string) {
				t.Helper()
				otherPath := filepath.Join(t.TempDir(), "other-tool")
				writeTestBinary(t, otherPath)
				if err := config.UpsertBinary(&config.Binary{
					Path: otherPath, InstallMode: installModeBinary,
					CompletionOwnership: map[string]*config.CompletionOwnershipRecord{
						"bash": {Path: destination, SHA256: completionHash([]byte("completion"))},
					},
				}); err != nil {
					t.Fatal(err)
				}
			},
			reason: "another managed binary",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installDir := setupTestConfig(t)
			binaryPath, destination := setupOwnedCompletionRemoval(t, installDir, []byte("completion"))
			if err := os.Remove(binaryPath); err != nil {
				t.Fatal(err)
			}
			tc.prepare(t, destination)

			previousLogger := log.Log
			var logs bytes.Buffer
			logger := log.New(&logs)
			logger.Level = log.WarnLevel
			log.Log = logger
			t.Cleanup(func() { log.Log = previousLogger })

			command := newPruneCmd().cmd
			command.SetArgs([]string{"--force"})
			if err := command.Execute(); err != nil {
				t.Fatalf("prune: %v", err)
			}
			if _, ok := config.Get().Bins[binaryPath]; ok {
				t.Fatalf("prune retained config entry %s", binaryPath)
			}
			if _, err := os.Lstat(destination); err != nil {
				t.Fatalf("prune removed preserved completion: %v", err)
			}
			if !strings.Contains(logs.String(), destination) || !strings.Contains(logs.String(), tc.reason) {
				t.Fatalf("prune warning = %q, want path %q and reason %q", logs.String(), destination, tc.reason)
			}
		})
	}
}
