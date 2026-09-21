package cmd

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestCompletionSyncGeneratesDefaultAndExplicitCompletions(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell+" default", func(t *testing.T) {
			setupCompletionSyncBinary(t, "tool", "tool", `printf '%s|%s' "$1" "$2"`, installModeBinary)
			output, err := runCompletionSync(t, "tool", shell)
			if err != nil {
				t.Fatalf("sync %s completion: %v", shell, err)
			}
			assertCompletionContent(t, shell, "tool", "completion|"+shell)
			if !strings.Contains(output, "Installed completion:") || !strings.Contains(output, shellSetupTerm(shell)) {
				t.Fatalf("sync output = %q, want installed path and %s setup guidance", output, shell)
			}
		})

		t.Run(shell+" explicit argv", func(t *testing.T) {
			setupCompletionSyncBinary(t, "tool", "tool", `printf '%s|%s' "$1" "$2"`, installModeBinary)
			marker := filepath.Join(t.TempDir(), "shell-interpreted")
			argument := "$(touch " + marker + ")"
			if _, err := runCompletionSync(t, "tool", shell, "--", "custom", argument); err != nil {
				t.Fatalf("sync explicit %s completion: %v", shell, err)
			}
			assertCompletionContent(t, shell, "tool", "custom|"+argument)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("generator argument was interpreted by a shell, marker stat error = %v", err)
			}
		})
	}

	t.Run("literal double dash is a generator argument", func(t *testing.T) {
		setupCompletionSyncBinary(t, "tool", "tool", `printf '%s|%s|%s' "$1" "$2" "$3"`, installModeBinary)
		if _, err := runCompletionSync(t, "tool", "bash", "--", "custom", "--", "literal"); err != nil {
			t.Fatalf("sync completion with literal double dash: %v", err)
		}
		assertCompletionContent(t, "bash", "tool", "custom|--|literal")
	})
}

func TestCompletionSyncRejectsUnsafeTargetsWithoutReplacingCompletion(t *testing.T) {
	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	t.Run("invalid shell", func(t *testing.T) {
		setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		if _, err := runCompletionSync(t, "tool", "Bash"); err == nil {
			t.Fatal("sync accepted mixed-case shell")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("unmanaged", func(t *testing.T) {
		setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		if _, err := runCompletionSync(t, "not-managed", "bash"); err == nil {
			t.Fatal("sync accepted unmanaged binary")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("missing", func(t *testing.T) {
		binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		if err := os.Remove(binaryPath); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync accepted missing binary")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("changed", func(t *testing.T) {
		binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nprintf changed\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync accepted changed binary")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("not executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not use Unix executable permission bits")
		}
		binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		if err := os.Chmod(binaryPath, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync accepted a non-executable binary")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("non-regular", func(t *testing.T) {
		binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		if err := os.Remove(binaryPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(binaryPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync accepted a non-regular binary")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating a symlink requires privileges on Windows")
		}
		binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
		target := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf completion\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(binaryPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, binaryPath); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync accepted symlinked binary")
		}
		assertFileContent(t, destination, "existing")
	})

	t.Run("system package", func(t *testing.T) {
		setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeSystemPackage)
		destination, err := completionDestination(os.Getenv("BIN_CONFIG"), "bash", "tool")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync accepted system-package target")
		}
		assertFileContent(t, destination, "existing")
	})
}

func TestCompletionSyncGeneratorFailuresPreserveExistingCompletion(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	for name, body := range map[string]string{
		"empty":    `exit 0`,
		"non-text": `printf '\377'`,
		"nonzero":  `printf output; exit 7`,
		"overflow": `head -c 1048577 /dev/zero`,
		"timeout":  `sleep 30`,
	} {
		t.Run(name, func(t *testing.T) {
			setupCompletionSyncBinary(t, "tool", "tool", body, installModeBinary)
			_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
			if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
				t.Fatal("sync accepted failed generator")
			}
			assertFileContent(t, destination, "existing")
		})
	}
}

func TestCompletionSyncRejectsOwnershipConflictAndAllowsExplicitRenamedCommand(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	t.Run("ownership conflict", func(t *testing.T) {
		setupCompletionSyncBinary(t, "tool", "tool", `printf completion`, installModeBinary)
		destination, err := completionDestination(os.Getenv("BIN_CONFIG"), "bash", "tool")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, []byte("other owner"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := config.UpsertBinary(&config.Binary{Path: filepath.Join(t.TempDir(), "other"), Hash: "other", InstallMode: installModeBinary, CompletionOwnership: map[string]*config.CompletionOwnershipRecord{
			"bash": {Path: destination, SHA256: completionHash([]byte("other owner"))},
		}}); err != nil {
			t.Fatal(err)
		}
		if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
			t.Fatal("sync replaced a completion owned by another binary")
		}
		assertFileContent(t, destination, "other owner")
	})

	t.Run("renamed command requires explicit argv", func(t *testing.T) {
		setupCompletionSyncBinary(t, "alias", "upstream", `printf '%s|%s' "$1" "$2"`, installModeBinary)
		if _, err := runCompletionSync(t, "alias", "fish"); err == nil {
			t.Fatal("sync generated a default completion for renamed command")
		}
		if _, err := runCompletionSync(t, "alias", "fish", "--", "custom", "fish"); err != nil {
			t.Fatalf("sync explicit argv for renamed command: %v", err)
		}
		assertCompletionContent(t, "fish", "alias", "custom|fish")
	})
}

func TestCompletionsCommandCoexistsWithCobraCompletion(t *testing.T) {
	root := newRootCmd("test", func(int) {})
	plural, _, err := root.cmd.Find([]string{"completions", "sync"})
	if err != nil || plural == nil || plural.Name() != "sync" {
		t.Fatalf("plural completion sync command = %#v, err = %v", plural, err)
	}
	root.cmd.InitDefaultCompletionCmd()
	singular, _, err := root.cmd.Find([]string{"completion"})
	if err != nil || singular == nil || singular.Name() != "completion" {
		t.Fatalf("Cobra completion command = %#v, err = %v", singular, err)
	}
}

func setupCompletionSyncBinary(t *testing.T, name, remoteName, body, installMode string) string {
	t.Helper()
	directory := setupTestConfig(t)
	path := filepath.Join(directory, name)
	contents := []byte("#!/bin/sh\n" + body + "\n")
	if err := os.WriteFile(path, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(contents))
	if err := config.UpsertBinary(&config.Binary{Path: path, RemoteName: remoteName, Hash: hash, InstallMode: installMode}); err != nil {
		t.Fatal(err)
	}
	return path
}

func publishExistingCompletion(t *testing.T, binary, shell, command, content string) (string, string) {
	t.Helper()
	binaryPath, err := getBinPath(binary)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := config.GetBinary(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := publishManagedCompletion(binaryPath, managed.Hash, shell, command, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return binaryPath, destination
}

func runCompletionSync(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newCompletionsCmd()
	var output bytes.Buffer
	root.cmd.SetOut(&output)
	root.cmd.SetErr(&output)
	root.cmd.SetArgs(append([]string{"sync"}, args...))
	err := root.cmd.Execute()
	return output.String(), err
}

func assertCompletionContent(t *testing.T, shell, command, want string) {
	t.Helper()
	destination, err := completionDestination(os.Getenv("BIN_CONFIG"), shell, command)
	if err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, destination, want)
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("file %s = %q, err = %v, want %q", path, got, err, want)
	}
}

func shellSetupTerm(shell string) string {
	switch shell {
	case "bash":
		return "source"
	case "zsh":
		return "fpath"
	case "fish":
		return "fish_complete_path"
	default:
		return ""
	}
}
