package cmd

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
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

func TestCompletionSyncLoadsOwnedCompletionInSupportedShells(t *testing.T) {
	tests := []struct {
		shell     string
		generator string
		shellArgs func(string) []string
	}{
		{
			shell:     "bash",
			generator: `printf '%s\n' "complete -W 'alpha beta' tool"`,
			shellArgs: func(destination string) []string {
				return []string{"--noprofile", "--norc", "-c", `source "$1"; [[ $(complete -p tool) == *"alpha beta"* ]]`, "bash", destination}
			},
		},
		{
			shell: "zsh",
			// compinit reads #compdef only when it is the first line of the completion file.
			generator: `printf '%s\n' '#compdef tool' 'compadd alpha beta'`,
			shellArgs: func(destination string) []string {
				return []string{"-f", "-c", `fpath=("$1" $fpath); autoload -Uz compinit; compinit -D; [[ ${_comps[tool]} == _tool ]]`, "zsh", filepath.Dir(destination)}
			},
		},
		{
			shell:     "fish",
			generator: `printf '%s\n' "complete -c tool -a 'alpha beta'"`,
			shellArgs: func(destination string) []string {
				return []string{"--no-config", "-c", `set -gx fish_complete_path $argv[1] $fish_complete_path; complete -C 'tool a' | string match -q alpha`, filepath.Dir(destination)}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			shellPath, err := exec.LookPath(test.shell)
			if err != nil {
				t.Skipf("%s completion loading smoke coverage unavailable: executable not found", test.shell)
			}
			if runtime.GOOS == "windows" {
				t.Skipf("%s completion loading smoke coverage unavailable: requires a POSIX local helper", test.shell)
			}

			home := t.TempDir()
			t.Setenv("HOME", home)
			binaryPath := setupCompletionSyncBinary(t, "tool", "tool", test.generator, installModeBinary)
			if _, err := runCompletionSync(t, "tool", test.shell); err != nil {
				t.Fatalf("sync %s completion: %v", test.shell, err)
			}
			destination, err := completionDestination(os.Getenv("BIN_CONFIG"), test.shell, "tool")
			if err != nil {
				t.Fatal(err)
			}

			command := exec.Command(shellPath, test.shellArgs(destination)...)
			command.Env = []string{
				"BIN_CONFIG=" + os.Getenv("BIN_CONFIG"),
				"HOME=" + home,
				"PATH=" + filepath.Dir(binaryPath) + ":" + filepath.Dir(shellPath) + ":/usr/bin:/bin",
				"TMPDIR=" + home,
				"ZDOTDIR=" + home,
				"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
				"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
				"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
			}
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("load %s completion: %v\n%s", test.shell, err, output)
			}
		})
	}
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
		const command = "bin-test-renamed-command"
		setupCompletionSyncBinary(t, command, "upstream", `printf '%s|%s' "$1" "$2"`, installModeBinary)
		if _, err := runCompletionSync(t, command, "fish"); err == nil {
			t.Fatal("sync generated a default completion for renamed command")
		}
		if _, err := runCompletionSync(t, command, "fish", "--", "custom", "fish"); err != nil {
			t.Fatalf("sync explicit argv for renamed command: %v", err)
		}
		assertCompletionContent(t, "fish", command, "custom|fish")
	})
}

func TestCompletionSyncPrefersMatchingBundledCompletion(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	binaryPath := setupCompletionSyncBinary(t, "tool", "tool", "exit 42", installModeBinary)
	binary := configureCompletionSyncSource(t, binaryPath)
	installed, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	stream := &trackingReadCloser{reader: bytes.NewReader(installed)}

	originalFactory := completionProviderFactory
	completionProviderFactory = func(url, provider string) (providers.Provider, error) {
		if url != binary.URL || provider != binary.Provider {
			t.Fatalf("provider construction = (%q, %q), want (%q, %q)", url, provider, binary.URL, binary.Provider)
		}
		return fetchBinaryTestProvider{id: "github", fetchFn: func(opts *providers.FetchOpts) (*providers.File, error) {
			if opts.Version != binary.Version || opts.PackageName != binary.RemoteName || opts.PackagePath != binary.PackagePath || opts.ReleaseTagPrefix != binary.ReleaseTagPrefix {
				t.Fatalf("stored fetch options = %#v", opts)
			}
			if opts.BundledCompletionShell != "bash" || opts.BundledCompletionCommand != "tool" {
				t.Fatalf("bundled completion request = (%q, %q)", opts.BundledCompletionShell, opts.BundledCompletionCommand)
			}
			if opts.SelectionIntent == nil || opts.SelectionIntent.LogicalProduct != "tool" || opts.SelectionIntent.ArchiveMember != "bin/tool" || opts.SelectionIntent.Target == nil || opts.SelectionIntent.Target.CPUVariant != "avx2" {
				t.Fatalf("selection intent = %#v", opts.SelectionIntent)
			}
			return &providers.File{Data: stream, BundledCompletion: []byte("bundled completion")}, nil
		}}, nil
	}
	t.Cleanup(func() { completionProviderFactory = originalFactory })

	if _, err := runCompletionSync(t, "tool", "bash"); err != nil {
		t.Fatalf("sync bundled completion: %v", err)
	}
	assertCompletionContent(t, "bash", "tool", "bundled completion")
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream close count = %d, want 1", stream.closeCount)
	}
}

func TestCompletionSyncFallsBackWhenBundledCompletionIsAbsentOrMismatched(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	for name, fetched := range map[string]func([]byte) *providers.File{
		"absent": func(installed []byte) *providers.File {
			return &providers.File{Data: &trackingReadCloser{reader: bytes.NewReader(installed)}}
		},
		"mismatched executable": func(_ []byte) *providers.File {
			return &providers.File{Data: &trackingReadCloser{reader: strings.NewReader("different executable")}, BundledCompletion: []byte("stale bundled completion")}
		},
	} {
		t.Run(name, func(t *testing.T) {
			binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf native`, installModeBinary)
			configureCompletionSyncSource(t, binaryPath)
			installed, err := os.ReadFile(binaryPath)
			if err != nil {
				t.Fatal(err)
			}
			file := fetched(installed)
			stream := file.Data.(*trackingReadCloser)

			originalFactory := completionProviderFactory
			completionProviderFactory = func(string, string) (providers.Provider, error) {
				return fetchBinaryTestProvider{id: "github", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
					return file, nil
				}}, nil
			}
			t.Cleanup(func() { completionProviderFactory = originalFactory })

			if _, err := runCompletionSync(t, "tool", "bash"); err != nil {
				t.Fatalf("sync fallback completion: %v", err)
			}
			assertCompletionContent(t, "bash", "tool", "native")
			if stream.closeCount != 1 {
				t.Fatalf("fetched stream close count = %d, want 1", stream.closeCount)
			}
		})
	}
}

func TestCompletionSyncBypassesBundledFetchForExplicitAndEffectfulSources(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	for name, setup := range map[string]func(*testing.T) string{
		"explicit argv": func(t *testing.T) string {
			path := setupCompletionSyncBinary(t, "tool", "tool", `printf '%s|%s' "$1" "$2"`, installModeBinary)
			configureCompletionSyncSource(t, path)
			return path
		},
		"docker source": func(t *testing.T) string {
			path := setupCompletionSyncBinary(t, "tool", "tool", `printf '%s|%s' "$1" "$2"`, installModeBinary)
			binary, err := config.GetBinary(path)
			if err != nil {
				t.Fatal(err)
			}
			binary.URL = "docker://example/tool:1.2.3"
			binary.Provider = "docker"
			if err := config.UpsertBinary(binary); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"go-install source": func(t *testing.T) string {
			path := setupCompletionSyncBinary(t, "tool", "tool", `printf '%s|%s' "$1" "$2"`, installModeBinary)
			binary, err := config.GetBinary(path)
			if err != nil {
				t.Fatal(err)
			}
			binary.URL = "goinstall://example.test/acme/tool@v1.2.3"
			binary.Provider = "goinstall"
			if err := config.UpsertBinary(binary); err != nil {
				t.Fatal(err)
			}
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			setup(t)
			originalFactory := completionProviderFactory
			completionProviderFactory = func(string, string) (providers.Provider, error) {
				t.Fatal("bundled provider construction was not bypassed")
				return nil, nil
			}
			t.Cleanup(func() { completionProviderFactory = originalFactory })

			args := []string{"tool", "bash"}
			want := "completion|bash"
			if name == "explicit argv" {
				args = append(args, "--", "custom", "argv")
				want = "custom|argv"
			}
			if _, err := runCompletionSync(t, args...); err != nil {
				t.Fatalf("sync completion: %v", err)
			}
			assertCompletionContent(t, "bash", "tool", want)
		})
	}
}

func TestCompletionSyncFetchFailureClosesResultAndDoesNotFallback(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf native`, installModeBinary)
	configureCompletionSyncSource(t, binaryPath)
	_, destination := publishExistingCompletion(t, "tool", "bash", "tool", "existing")
	stream := &trackingReadCloser{reader: strings.NewReader("failed fetch result")}

	originalFactory := completionProviderFactory
	completionProviderFactory = func(string, string) (providers.Provider, error) {
		return fetchBinaryTestProvider{id: "github", fetchFn: func(*providers.FetchOpts) (*providers.File, error) {
			return &providers.File{Data: stream}, errors.New("fetch failed")
		}}, nil
	}
	t.Cleanup(func() { completionProviderFactory = originalFactory })

	if _, err := runCompletionSync(t, "tool", "bash"); err == nil || !strings.Contains(err.Error(), "fetch failed") {
		t.Fatalf("sync fetch error = %v, want fetch failure", err)
	}
	assertFileContent(t, destination, "existing")
	if stream.closeCount != 1 {
		t.Fatalf("fetched stream close count = %d, want 1", stream.closeCount)
	}
}

func TestCompletionSyncRejectsMissingOrChangedBinaryBeforeBundledFetch(t *testing.T) {
	for name, invalidate := range map[string]func(*testing.T, string){
		"missing": func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		},
		"changed": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf changed\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			binaryPath := setupCompletionSyncBinary(t, "tool", "tool", `printf native`, installModeBinary)
			configureCompletionSyncSource(t, binaryPath)
			invalidate(t, binaryPath)

			originalFactory := completionProviderFactory
			completionProviderFactory = func(string, string) (providers.Provider, error) {
				t.Fatal("bundled provider construction occurred for invalid binary")
				return nil, nil
			}
			t.Cleanup(func() { completionProviderFactory = originalFactory })

			if _, err := runCompletionSync(t, "tool", "bash"); err == nil {
				t.Fatal("sync accepted invalid binary")
			}
		})
	}
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

func configureCompletionSyncSource(t *testing.T, binaryPath string) *config.Binary {
	t.Helper()
	binary, err := config.GetBinary(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	binary.URL = "https://example.test/acme/tool"
	binary.Provider = "github"
	binary.Version = "v1.2.3"
	binary.PackagePath = "bin/tool"
	binary.ReleaseTagPrefix = "nightly-"
	binary.SelectionIntent = &config.SelectionDescriptor{
		LogicalProduct: "tool",
		ArchiveMember:  "bin/tool",
		Target:         &config.SelectionTarget{OS: "linux", Architecture: "amd64", CPUVariant: "avx2"},
	}
	if err := config.UpsertBinary(binary); err != nil {
		t.Fatal(err)
	}
	return binary
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
