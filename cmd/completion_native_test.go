package cmd

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aaronflorey/bin/pkg/config"
)

const completionNativeHelperTestFlag = "-test.run=^TestCompletionNativeHelperProcess$"

func TestGenerateNativeCompletionWithTestHelperProcess(t *testing.T) {
	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	originalDirectory := mustGetwd(t)
	t.Setenv("BIN_COMPLETION_TEST_SECRET", "secret")

	output, err := generateNativeCompletion(executable, []string{
		completionNativeHelperTestFlag,
		"--",
		"completion",
		"bash",
		originalDirectory,
	})
	if err != nil {
		t.Fatalf("generate completion with test helper: %v", err)
	}
	if got, want := string(output), "completion|bash"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestCompletionNativeHelperProcess(_ *testing.T) {
	args, ok := completionNativeHelperArgs(os.Args)
	if !ok {
		return
	}
	if len(args) != 3 || args[0] != "completion" || args[1] != "bash" {
		completionNativeHelperFailure("unexpected argv: %q", args)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		completionNativeHelperFailure("get working directory: %v", err)
	}
	if workingDirectory == args[2] {
		completionNativeHelperFailure("generator inherited caller working directory")
	}
	if os.Getenv("BIN_COMPLETION_TEST_SECRET") != "" {
		completionNativeHelperFailure("generator inherited secret environment variable")
	}
	if os.Getenv("HOME") != workingDirectory || os.Getenv("TMPDIR") != workingDirectory {
		completionNativeHelperFailure("generator environment does not use its temporary working directory")
	}
	var input [1]byte
	if count, err := os.Stdin.Read(input[:]); count != 0 || err != io.EOF {
		completionNativeHelperFailure("generator stdin = count %d, error %v; want EOF", count, err)
	}

	fmt.Printf("%s|%s", args[0], args[1])
	os.Exit(0)
}

func completionNativeHelperArgs(args []string) ([]string, bool) {
	for _, argument := range args {
		if argument == completionNativeHelperTestFlag {
			for index, argument := range args {
				if argument == "--" {
					return args[index+1:], true
				}
			}
		}
	}
	return nil, false
}

func completionNativeHelperFailure(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func TestGenerateNativeCompletionUsesDirectBoundedExecution(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	t.Run("argv, closed stdin, temporary cwd, and minimal environment", func(t *testing.T) {
		script := completionTestScript(t, `
[ -z "$COMPLETION_TEST_SECRET" ] || exit 31
[ "$PWD" != "$COMPLETION_TEST_ORIGINAL_CWD" ] || exit 32
if read value; then exit 33; fi
printf '%s|%s' "$1" "$2"
`)
		t.Setenv("COMPLETION_TEST_SECRET", "secret")
		t.Setenv("COMPLETION_TEST_ORIGINAL_CWD", mustGetwd(t))

		output, err := generateNativeCompletion(script, []string{"completion", "bash"})
		if err != nil {
			t.Fatalf("generate completion: %v", err)
		}
		if got, want := string(output), "completion|bash"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("does not interpret arguments as shell", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "marker")
		script := completionTestScript(t, `printf '%s' "$1"`)
		argument := "$(touch " + marker + ")"

		output, err := generateNativeCompletion(script, []string{argument})
		if err != nil {
			t.Fatalf("generate completion: %v", err)
		}
		if got := string(output); got != argument {
			t.Fatalf("output = %q, want literal argument %q", got, argument)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("argument was interpreted by a shell, marker stat error = %v", err)
		}
	})

	for name, body := range map[string]string{
		"empty":    `exit 0`,
		"non-text": `printf '\377'`,
		"nonzero":  `printf output; exit 7`,
		"overflow": `head -c 1048577 /dev/zero`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := generateNativeCompletion(completionTestScript(t, body), nil); err == nil {
				t.Fatal("generateNativeCompletion succeeded")
			}
		})
	}
}

func TestGenerateNativeCompletionTimesOut(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	started := time.Now()
	_, err := generateNativeCompletion(completionTestScript(t, "sleep 30"), nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("generateNativeCompletion error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > completionGeneratorTimeout+2*time.Second {
		t.Fatalf("timeout took %s, want bounded wait", elapsed)
	}
}

func TestSyncNativeCompletionUsesDefaultAndExplicitArgv(t *testing.T) {
	requirePOSIXShellFixture(t)

	originalElevated := completionProcessElevated
	completionProcessElevated = func() bool { return false }
	t.Cleanup(func() { completionProcessElevated = originalElevated })

	defaultPath := setupTestConfig(t)
	binaryPath := filepath.Join(defaultPath, "tool")
	script := "#!/bin/sh\nprintf '%s:%s' \"$1\" \"$2\"\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(script)))
	if err := config.UpsertBinary(&config.Binary{Path: binaryPath, RemoteName: "tool", Hash: hash, InstallMode: installModeBinary}); err != nil {
		t.Fatal(err)
	}

	for _, shell := range []string{"bash", "zsh", "fish"} {
		destination, err := syncNativeCompletion(binaryPath, shell, "tool", nil)
		if err != nil {
			t.Fatalf("default %s completion: %v", shell, err)
		}
		content, err := os.ReadFile(destination)
		if err != nil || string(content) != "completion:"+shell {
			t.Fatalf("default %s content = %q, err = %v", shell, content, err)
		}
	}

	destination, err := syncNativeCompletion(binaryPath, "bash", "tool", []string{"custom", "argv"})
	if err != nil {
		t.Fatalf("explicit argv completion: %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "custom:argv" {
		t.Fatalf("explicit argv content = %q, err = %v", content, err)
	}
}

func TestPublishManagedCompletionPreservesOwnershipBoundaries(t *testing.T) {
	defaultPath := setupTestConfig(t)
	binaryPath := filepath.Join(defaultPath, "tool")
	binaryBytes := []byte("managed binary")
	if err := os.WriteFile(binaryPath, binaryBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(binaryBytes))
	if err := config.UpsertBinary(&config.Binary{Path: binaryPath, Hash: hash, InstallMode: installModeBinary}); err != nil {
		t.Fatal(err)
	}

	destination, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("first"))
	if err != nil {
		t.Fatalf("initial publication: %v", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "first" {
		t.Fatalf("initial destination = %q, err = %v", got, err)
	}
	if ownership := config.Get().Bins[binaryPath].CompletionOwnership["bash"]; ownership == nil || ownership.Path != destination || ownership.SHA256 != completionHash([]byte("first")) {
		t.Fatalf("ownership = %#v", ownership)
	}

	if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("second")); err != nil {
		t.Fatalf("refresh publication: %v", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "second" {
		t.Fatalf("refreshed destination = %q, err = %v", got, err)
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("recreated")); err != nil {
		t.Fatalf("recreate owned completion: %v", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "recreated" {
		t.Fatalf("recreated destination = %q, err = %v", got, err)
	}

	if err := os.WriteFile(destination, []byte("user change"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("third")); err == nil {
		t.Fatal("publication replaced modified completion")
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "user change" {
		t.Fatalf("modified destination = %q, err = %v", got, err)
	}
}

func TestPublishManagedCompletionRejectsStaleAndCompetingOwnership(t *testing.T) {
	defaultPath := setupTestConfig(t)
	binaryPath := filepath.Join(defaultPath, "tool")
	binaryBytes := []byte("managed binary")
	if err := os.WriteFile(binaryPath, binaryBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(binaryBytes))
	if err := config.UpsertBinary(&config.Binary{Path: binaryPath, Hash: hash, InstallMode: installModeBinary}); err != nil {
		t.Fatal(err)
	}

	newBytes := []byte("updated managed binary")
	newHash := fmt.Sprintf("%x", sha256.Sum256(newBytes))
	if err := os.WriteFile(binaryPath, newBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertBinary(&config.Binary{Path: binaryPath, Hash: newHash, InstallMode: installModeBinary}); err != nil {
		t.Fatal(err)
	}
	if _, err := publishManagedCompletion(binaryPath, hash, "fish", "tool", []byte("complete")); err == nil {
		t.Fatal("publication accepted stale managed binary")
	}

	configPath := os.Getenv("BIN_CONFIG")
	destination, err := completionDestination(configPath, "fish", "tool")
	if err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(defaultPath, "other")
	if err := config.UpsertBinary(&config.Binary{Path: otherPath, Hash: "other", InstallMode: installModeBinary, CompletionOwnership: map[string]*config.CompletionOwnershipRecord{
		"fish": {Path: destination, SHA256: "other"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := publishManagedCompletion(binaryPath, newHash, "fish", "tool", []byte("complete")); err == nil {
		t.Fatal("publication accepted destination claimed by another binary")
	}
}

func TestPublishManagedCompletionRejectsUnsafeDestinationEntries(t *testing.T) {
	t.Run("non-regular destination", func(t *testing.T) {
		defaultPath := setupTestConfig(t)
		binaryPath, hash := setupPublicationBinary(t, defaultPath)
		destination, err := completionDestination(os.Getenv("BIN_CONFIG"), "bash", "tool")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatal(err)
		}

		if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("completion")); err == nil {
			t.Fatal("publication replaced a non-regular completion destination")
		}
		if info, err := os.Lstat(destination); err != nil || !info.IsDir() {
			t.Fatalf("destination after rejected publication = %#v, err = %v", info, err)
		}
	})

	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires privileges on Windows")
	}

	t.Run("symlink destination", func(t *testing.T) {
		defaultPath := setupTestConfig(t)
		binaryPath, hash := setupPublicationBinary(t, defaultPath)
		destination, err := completionDestination(os.Getenv("BIN_CONFIG"), "bash", "tool")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "completion-target")
		if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, destination); err != nil {
			t.Fatal(err)
		}

		if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("completion")); err == nil {
			t.Fatal("publication replaced a symlink completion destination")
		}
		assertFileContent(t, target, "existing")
	})

	t.Run("symlink owned directory", func(t *testing.T) {
		defaultPath := setupTestConfig(t)
		binaryPath, hash := setupPublicationBinary(t, defaultPath)
		completionDirectory := filepath.Join(filepath.Dir(os.Getenv("BIN_CONFIG")), "completions")
		targetDirectory := t.TempDir()
		if err := os.Symlink(targetDirectory, completionDirectory); err != nil {
			t.Fatal(err)
		}

		if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("completion")); err == nil {
			t.Fatal("publication accepted a symlinked completion directory")
		}
		if _, err := os.Stat(filepath.Join(targetDirectory, "bash", "tool")); !os.IsNotExist(err) {
			t.Fatalf("publication wrote through symlinked completion directory: %v", err)
		}
	})
}

func setupPublicationBinary(t *testing.T, directory string) (string, string) {
	t.Helper()
	binaryPath := filepath.Join(directory, "tool")
	binaryBytes := []byte("managed binary")
	if err := os.WriteFile(binaryPath, binaryBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(binaryBytes))
	if err := config.UpsertBinary(&config.Binary{Path: binaryPath, Hash: hash, InstallMode: installModeBinary}); err != nil {
		t.Fatal(err)
	}
	return binaryPath, hash
}

func TestPublishManagedCompletionSerializesConcurrentRefreshes(t *testing.T) {
	defaultPath := setupTestConfig(t)
	binaryPath := filepath.Join(defaultPath, "tool")
	binaryBytes := []byte("managed binary")
	if err := os.WriteFile(binaryPath, binaryBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(binaryBytes))
	if err := config.UpsertBinary(&config.Binary{Path: binaryPath, Hash: hash, InstallMode: installModeBinary}); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, content := range [][]byte{[]byte("one"), []byte("two")} {
		group.Add(1)
		go func(content []byte) {
			defer group.Done()
			_, err := publishManagedCompletion(binaryPath, hash, "zsh", "tool", content)
			errs <- err
		}(content)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent publication: %v", err)
		}
	}

	destination, err := completionDestination(os.Getenv("BIN_CONFIG"), "zsh", "tool")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	ownership := config.Get().Bins[binaryPath].CompletionOwnership["zsh"]
	if ownership == nil || ownership.SHA256 != completionHash(content) {
		t.Fatalf("ownership does not match concurrently published file: %#v", ownership)
	}
}

func TestPublishManagedCompletionNamesPublishedPathWhenOwnershipSaveFails(t *testing.T) {
	defaultPath := setupTestConfig(t)
	binaryPath := filepath.Join(defaultPath, "tool")
	binaryBytes := []byte("managed binary")
	if err := os.WriteFile(binaryPath, binaryBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(binaryBytes))
	binary := &config.Binary{Path: binaryPath, Hash: hash, InstallMode: installModeBinary}
	if err := config.UpsertBinary(binary); err != nil {
		t.Fatal(err)
	}

	originalMutation := mutateLockedBinary
	mutateLockedBinary = func(path string, mutate func(string, *config.Binary, []*config.Binary) error) error {
		if err := mutate(os.Getenv("BIN_CONFIG"), config.CloneBinary(binary), nil); err != nil {
			return err
		}
		return errors.New("config persistence failed")
	}
	t.Cleanup(func() { mutateLockedBinary = originalMutation })

	destination, err := completionDestination(os.Getenv("BIN_CONFIG"), "bash", "tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publishManagedCompletion(binaryPath, hash, "bash", "tool", []byte("completion")); err == nil || !strings.Contains(err.Error(), destination) {
		t.Fatalf("publication error = %v, want exact published path %s", err, destination)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "completion" {
		t.Fatalf("published completion = %q, err = %v", got, err)
	}
	if config.Get().Bins[binaryPath].CompletionOwnership != nil {
		t.Fatalf("failed persistence adopted completion ownership: %#v", config.Get().Bins[binaryPath])
	}
}

func completionTestScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "generator")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func requirePOSIXShellFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell fixture")
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
