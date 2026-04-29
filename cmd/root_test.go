package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caarlos0/log"
	"github.com/spf13/cobra"
)

func TestRootHasVerboseFlag(t *testing.T) {
	root := newRootCmd("test", func(int) {})

	if root.cmd.PersistentFlags().Lookup("verbose") == nil {
		t.Fatal("expected --verbose flag to be registered")
	}
	if root.cmd.PersistentFlags().Lookup("debug") == nil {
		t.Fatal("expected --debug flag alias to remain registered")
	}
	if root.cmd.PersistentFlags().Lookup("log-file") == nil {
		t.Fatal("expected --log-file flag to be registered")
	}
}

func TestRootVerboseSkipsSpinner(t *testing.T) {
	setupTestConfig(t)

	root := newRootCmd("test", func(int) {})
	root.shouldLaunchTUI = func([]string) bool { return false }

	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stdout)

	var outType string
	var errType string
	trace := &cobra.Command{
		Use: "trace",
		RunE: func(cmd *cobra.Command, args []string) error {
			outType = fmt.Sprintf("%T", cmd.OutOrStdout())
			errType = fmt.Sprintf("%T", cmd.ErrOrStderr())
			return nil
		},
	}
	enableSpinner(trace)
	root.cmd.AddCommand(trace)

	previousLogger := log.Log
	var logs bytes.Buffer
	logger := log.New(&logs)
	log.Log = logger
	defer func() {
		log.Log = previousLogger
	}()

	root.Execute([]string{"--verbose", "trace"})

	if strings.Contains(outType, "stopWriter") {
		t.Fatalf("expected verbose mode to skip spinner stdout wrapper, got %q", outType)
	}
	if strings.Contains(errType, "stopWriter") {
		t.Fatalf("expected verbose mode to skip spinner stderr wrapper, got %q", errType)
	}
	if logger.Level != log.DebugLevel {
		t.Fatalf("expected verbose mode to enable debug level logging")
	}
	if !strings.Contains(logs.String(), "verbose logs enabled") {
		t.Fatalf("expected verbose startup log, got %q", logs.String())
	}
	if !strings.Contains(logs.String(), "Skipping spinner for trace because verbose logging is enabled") {
		t.Fatalf("expected verbose spinner skip log, got %q", logs.String())
	}
	if outType != "*bytes.Buffer" {
		t.Fatalf("unexpected stdout writer type: %q", outType)
	}
	if errType != "*bytes.Buffer" {
		t.Fatalf("unexpected stderr writer type: %q", errType)
	}
}

func TestRootLogFileCapturesVerboseLogs(t *testing.T) {
	setupTestConfig(t)

	root := newRootCmd("test", func(int) {})
	root.shouldLaunchTUI = func([]string) bool { return false }

	var stdout bytes.Buffer
	root.cmd.SetOut(&stdout)
	root.cmd.SetErr(&stdout)

	trace := &cobra.Command{
		Use: "trace",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Debug("trace command ran")
			return nil
		},
	}
	enableSpinner(trace)
	root.cmd.AddCommand(trace)

	logPath := filepath.Join(t.TempDir(), "bin.log")
	root.Execute([]string{"--verbose", "--log-file", logPath, "trace"})

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	logs := string(raw)
	if !strings.Contains(logs, "verbose logs enabled, version: test") {
		t.Fatalf("expected verbose startup log in file, got %q", logs)
	}
	if !strings.Contains(logs, "Skipping spinner for trace because verbose logging is enabled") {
		t.Fatalf("expected spinner skip log in file, got %q", logs)
	}
	if !strings.Contains(logs, "trace command ran") {
		t.Fatalf("expected command log in file, got %q", logs)
	}
}
