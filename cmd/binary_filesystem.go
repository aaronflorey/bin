package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aaronflorey/bin/pkg/spinner"
)

const filesystemHelperCommand = "__bin-filesystem"

// binaryFilesystem keeps sudo consent local to one binary's transaction.
// The helper performs only filesystem operations; it never loads config or hooks.
type binaryFilesystem struct {
	nonInteractive bool
	sudo           string
	executable     string
}

func (fs *binaryFilesystem) run(operation string, paths ...string) (string, error) {
	if fs == nil {
		fs = &binaryFilesystem{}
	}
	result, err := performBinaryFilesystemOperation(operation, paths)
	if !errors.Is(err, os.ErrPermission) || runtime.GOOS == "windows" || processElevated() {
		return result, err
	}
	if fs.sudo == "" {
		if fs.nonInteractive || !isPromptInteractive() {
			return "", err
		}
		sudo, lookupErr := lookPathCommand("sudo")
		if lookupErr != nil {
			return "", err
		}
		executable, executableErr := os.Executable()
		if executableErr != nil {
			return "", errors.Join(err, executableErr)
		}
		if confirmErr := confirmDefaultNoPrompt(fmt.Sprintf("Permission denied for %s. Use sudo for this binary's filesystem changes?", paths[len(paths)-1])); confirmErr != nil {
			return "", errors.Join(err, confirmErr)
		}
		fs.sudo, fs.executable = sudo, executable
	}
	args := []string{"--", fs.executable, filesystemHelperCommand, operation}
	// Relative paths must refer to the same working directory under sudo.
	for _, path := range paths {
		absolute, pathErr := filepath.Abs(path)
		if pathErr != nil {
			return "", pathErr
		}
		args = append(args, absolute)
	}
	child := execCommand(fs.sudo, args...)
	var output bytes.Buffer
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, &output, os.Stderr
	resume := spinner.Pause()
	defer resume()
	if sudoErr := child.Run(); sudoErr != nil {
		return "", errors.Join(err, fmt.Errorf("sudo filesystem retry failed: %w", sudoErr))
	}
	return strings.TrimSuffix(output.String(), "\n"), nil
}

func performBinaryFilesystemOperation(operation string, paths []string) (string, error) {
	switch {
	case operation == "mkdir" && len(paths) == 1:
		return "", os.MkdirAll(paths[0], 0o755)
	case operation == "temp" && len(paths) == 1:
		file, err := os.CreateTemp(filepath.Dir(paths[0]), filepath.Base(paths[0])+"-*")
		if err != nil {
			return "", err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(file.Name())
			return "", err
		}
		return file.Name(), nil
	case operation == "rename" && len(paths) == 2:
		return "", os.Rename(paths[0], paths[1])
	case operation == "publish" && len(paths) == 2:
		return "", publishStagedBinary(paths[0], paths[1], false)
	case operation == "replace" && len(paths) == 2:
		return "", publishStagedBinary(paths[0], paths[1], true)
	case operation == "remove" && len(paths) == 1:
		return "", os.Remove(paths[0])
	default:
		return "", fmt.Errorf("invalid binary filesystem operation %q", operation)
	}
}

func executeFilesystemHelper(args []string) error {
	if !processElevated() {
		return errors.New("binary filesystem helper requires elevated privileges")
	}
	if len(args) < 2 {
		return errors.New("missing binary filesystem operation")
	}
	result, err := performBinaryFilesystemOperation(args[0], args[1:])
	if err != nil {
		return err
	}
	if args[0] == "temp" {
		// The caller must be able to fill and validate its staging file without sudo.
		if err := returnStagingFileToCaller(result); err != nil {
			_ = os.Remove(result)
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, result)
	}
	return err
}
