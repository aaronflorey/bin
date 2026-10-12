//go:build !windows

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func protectedBinaryDirectory(t *testing.T) string {
	t.Helper()
	if processElevated() {
		t.Skip("requires an unprivileged user to exercise permission errors")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return dir
}

func mockFilesystemSudo(t *testing.T, dir string, exitCode int) *int {
	t.Helper()
	previousLookPath, previousExec := lookPathCommand, execCommand
	previousInteractive, previousConfirm := isPromptInteractive, confirmDefaultNoPrompt
	t.Cleanup(func() {
		lookPathCommand, execCommand = previousLookPath, previousExec
		isPromptInteractive, confirmDefaultNoPrompt = previousInteractive, previousConfirm
	})
	prompts := 0
	isPromptInteractive = func() bool { return true }
	confirmDefaultNoPrompt = func(string) error { prompts++; return nil }
	lookPathCommand = func(name string) (string, error) {
		if name != "sudo" {
			t.Fatalf("unexpected lookup %s", name)
		}
		return "/mock/sudo", nil
	}
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "/mock/sudo" || len(args) < 5 || args[0] != "--" || args[2] != filesystemHelperCommand {
			t.Fatalf("unexpected sudo invocation: %s %v", name, args)
		}
		executable, err := os.Executable()
		if err != nil || args[1] != executable {
			t.Fatalf("retry must use the current executable: %v", args)
		}
		for _, path := range args[4:] {
			if !filepath.IsAbs(path) {
				t.Fatalf("helper path is not absolute: %s", path)
			}
		}
		child := exec.Command(os.Args[0], "-test.run=^TestFilesystemSudoProcess$", "--")
		child.Args = append(child.Args, args[3:]...)
		child.Env = append(os.Environ(), "BIN_TEST_SUDO_DIR="+dir, fmt.Sprintf("BIN_TEST_SUDO_EXIT=%d", exitCode))
		return child
	}
	return &prompts
}

// Emulate only directory write access, without ever invoking sudo or the root helper.
func TestFilesystemSudoProcess(t *testing.T) {
	dir := os.Getenv("BIN_TEST_SUDO_DIR")
	if dir == "" {
		return
	}
	if os.Getenv("BIN_TEST_SUDO_EXIT") != "0" {
		os.Exit(1)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		os.Exit(2)
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	result, err := performBinaryFilesystemOperation(args[1], args[2:])
	_ = os.Chmod(dir, 0o555)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if result != "" {
		fmt.Fprintln(os.Stdout, result)
	}
	os.Exit(0)
}

func TestInstallBinarySudoKeepsUserConfigAndPromptsOnce(t *testing.T) {
	setupTestConfig(t)
	dir := protectedBinaryDirectory(t)
	prompts := mockFilesystemSudo(t, dir, 0)
	previousProvider := installProviderFactory
	installProviderFactory = func(string, string) (providers.Provider, error) {
		return &runTestProvider{name: "tool", version: "1.2.3", content: string(sharedRunnableTestPayload)}, nil
	}
	t.Cleanup(func() { installProviderFactory = previousProvider })
	path := filepath.Join(dir, "tool")
	result, err := installBinary(InstallOpts{URL: "https://example.test/tool", Path: path, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != path || *prompts != 1 {
		t.Fatalf("result = %+v, prompts = %d", result, *prompts)
	}
	binary, err := config.GetBinary(path)
	if err != nil || binary == nil || binary.Hash == "" {
		t.Fatalf("managed config = %+v, err = %v", binary, err)
	}
	payload, err := os.ReadFile(path)
	if err != nil || string(payload) != string(sharedRunnableTestPayload) {
		t.Fatalf("installed payload = %q, err = %v", payload, err)
	}
}

func TestSudoPublicationRollbackRestoresExistingBinary(t *testing.T) {
	dir := protectedBinaryDirectory(t)
	prompts := mockFilesystemSudo(t, dir, 0)
	fs := &binaryFilesystem{}
	path := filepath.Join(dir, "tool")
	write := func(path, content string) {
		t.Helper()
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	write(path, "old binary")
	candidate := filepath.Join(dir, "candidate")
	write(candidate, "new binary")
	publication := stagedBinaryPublication{fs: fs, candidate: candidate, destination: path, overwrite: true}
	if _, err := publication.reserveBackup(); err != nil {
		t.Fatal(err)
	}
	if err := publication.publish(); err != nil {
		t.Fatal(err)
	}
	if err := publication.rollback(); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil || string(payload) != "old binary" {
		t.Fatalf("rollback payload = %q, err = %v", payload, err)
	}
	if *prompts != 1 {
		t.Fatalf("prompts = %d, want 1", *prompts)
	}
}

func TestFilesystemPermissionRetryRejections(t *testing.T) {
	for _, scenario := range []string{"non-interactive terminal", "explicit non-interactive", "sudo missing", "declined", "sudo failed"} {
		t.Run(scenario, func(t *testing.T) {
			dir := protectedBinaryDirectory(t)
			exitCode := 0
			if scenario == "sudo failed" {
				exitCode = 1
			}
			prompts := mockFilesystemSudo(t, dir, exitCode)
			fs := &binaryFilesystem{}
			switch scenario {
			case "non-interactive terminal":
				isPromptInteractive = func() bool { return false }
			case "explicit non-interactive":
				fs.nonInteractive = true
			case "sudo missing":
				lookPathCommand = func(string) (string, error) { return "", exec.ErrNotFound }
			case "declined":
				confirmDefaultNoPrompt = func(string) error { *prompts++; return errors.New("command aborted") }
			}
			_, err := fs.run("temp", filepath.Join(dir, "tool.tmp"))
			if !errors.Is(err, os.ErrPermission) {
				t.Fatalf("expected permission error, got %v", err)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("failed retry left files: %v, err = %v", entries, readErr)
			}
			wantPrompts := 0
			if scenario == "declined" || scenario == "sudo failed" {
				wantPrompts = 1
			}
			if *prompts != wantPrompts {
				t.Fatalf("prompts = %d, want %d", *prompts, wantPrompts)
			}
		})
	}
}

func TestFilesystemDoesNotElevateUnrelatedErrors(t *testing.T) {
	dir := t.TempDir()
	prompts := mockFilesystemSudo(t, dir, 0)
	fs := &binaryFilesystem{}
	_, err := fs.run("temp", filepath.Join(dir, "missing", "tool"))
	if !errors.Is(err, os.ErrNotExist) || *prompts != 0 {
		t.Fatalf("error = %v, prompts = %d", err, *prompts)
	}
	_, err = fs.run("publish", "missing", dir)
	if err == nil || strings.Contains(err.Error(), "sudo filesystem retry") || *prompts != 0 {
		t.Fatalf("error = %v, prompts = %d", err, *prompts)
	}
}

func TestSudoStagingRejectsInvalidPayloadAndCleansUp(t *testing.T) {
	for _, scenario := range []string{"checksum mismatch", "invalid executable"} {
		t.Run(scenario, func(t *testing.T) {
			setupTestConfig(t)
			dir := protectedBinaryDirectory(t)
			prompts := mockFilesystemSudo(t, dir, 0)
			file := &providers.File{Name: "tool", Data: strings.NewReader(string(sharedRunnableTestPayload))}
			if scenario == "checksum mismatch" {
				file.ExpectedSHA = "bad-sha"
			} else {
				file.Data = strings.NewReader("<html>download error</html>")
			}
			_, err := prepareStagedBinary(file, filepath.Join(dir, "tool"), false, &binaryFilesystem{})
			if err == nil {
				t.Fatal("invalid payload was accepted")
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("validation failure left files: %v, err = %v", entries, readErr)
			}
			if *prompts != 1 {
				t.Fatalf("prompts = %d, want 1", *prompts)
			}
		})
	}
}

func TestSudoRemovesProtectedBinary(t *testing.T) {
	dir := protectedBinaryDirectory(t)
	prompts := mockFilesystemSudo(t, dir, 0)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tool")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	fs := &binaryFilesystem{}
	if _, err := fs.run("remove", path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("binary still exists: %v", err)
	}
	if *prompts != 1 {
		t.Fatalf("prompts = %d, want 1", *prompts)
	}
}
