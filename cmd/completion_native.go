package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	completionGeneratorTimeout = 5 * time.Second
	completionPipeWait         = 250 * time.Millisecond
	completionStdoutLimit      = 1 << 20
	completionStderrLimit      = 64 << 10
)

var completionProcessElevated = processElevated

var errCompletionOutputTooLarge = errors.New("completion generator output exceeds limit")

// generateNativeCompletion runs the installed executable directly. Arguments
// are passed as argv, never through a shell.
func generateNativeCompletion(executable string, args []string) ([]byte, error) {
	if !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("completion generator must be an absolute executable path: %s", executable)
	}
	if completionProcessElevated() {
		return nil, errors.New("refusing to generate completions while elevated")
	}

	workDir, err := os.MkdirTemp("", "bin-completion-*")
	if err != nil {
		return nil, fmt.Errorf("create completion working directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	ctx, cancel := context.WithTimeout(context.Background(), completionGeneratorTimeout)
	defer cancel()

	stdout := newCompletionOutputBuffer(completionStdoutLimit, cancel)
	stderr := newCompletionOutputBuffer(completionStderrLimit, cancel)
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workDir
	command.Env = completionEnvironment(workDir)
	// A nil Stdin connects the child to the null device instead of inheriting
	// the caller's terminal or input stream.
	command.Stdin = nil
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = completionPipeWait

	waitErr := command.Run()
	if err := stdout.Err(); err != nil {
		return nil, fmt.Errorf("completion stdout: %w", err)
	}
	if err := stderr.Err(); err != nil {
		return nil, fmt.Errorf("completion stderr: %w", err)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("completion generator timed out after %s", completionGeneratorTimeout)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("completion generator failed: %w", waitErr)
	}

	output := stdout.Bytes()
	if len(output) == 0 {
		return nil, errors.New("completion generator produced no output")
	}
	if bytes.IndexByte(output, 0) >= 0 || !utf8.Valid(output) {
		return nil, errors.New("completion generator produced non-text output")
	}
	return output, nil
}

type completionOutputBuffer struct {
	limit  int
	cancel context.CancelFunc

	mu  sync.Mutex
	buf bytes.Buffer
	err error
}

func newCompletionOutputBuffer(limit int, cancel context.CancelFunc) *completionOutputBuffer {
	return &completionOutputBuffer{limit: limit, cancel: cancel}
}

func (buffer *completionOutputBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	remaining := buffer.limit - buffer.buf.Len()
	if len(data) <= remaining {
		return buffer.buf.Write(data)
	}
	if remaining > 0 {
		_, _ = buffer.buf.Write(data[:remaining])
	}
	if buffer.err == nil {
		buffer.err = errCompletionOutputTooLarge
		buffer.cancel()
	}
	return remaining, buffer.err
}

func (buffer *completionOutputBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.buf.Bytes()...)
}

func (buffer *completionOutputBuffer) Err() error {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.err
}
