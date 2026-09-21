package cmd

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/caarlos0/log"
)

var (
	mutateLockedBinary          = config.MutateBinaryLocked
	removeManagedCompletionFile = os.Remove
)

// syncNativeCompletion generates and publishes one completion file for an
// unchanged direct managed binary. It deliberately does not discover bundled
// scripts or register any command surface.
func syncNativeCompletion(binaryPath, shell, command string, generatorArgs []string) (string, error) {
	if _, err := completionFilename(shell, command); err != nil {
		return "", err
	}

	binary, err := config.GetBinary(binaryPath)
	if err != nil {
		return "", err
	}
	if err := validateDirectManagedBinary(binary); err != nil {
		return "", err
	}

	executable, err := filepath.Abs(expandTrackedBinaryPath(binary.Path))
	if err != nil {
		return "", fmt.Errorf("resolve managed executable path: %w", err)
	}
	if err := verifyManagedBinaryHash(binary, binary.Hash); err != nil {
		return "", err
	}

	args := generatorArgs
	if args == nil {
		if !commandNameMatches(binary.RemoteName, command) {
			return "", fmt.Errorf("default completion generation is unavailable for renamed command %s", command)
		}
		args = []string{"completion", shell}
	}
	output, err := generateNativeCompletion(executable, args)
	if err != nil {
		return "", err
	}
	return publishManagedCompletion(binary.Path, binary.Hash, shell, command, output)
}

// publishManagedCompletion writes already-validated completion text only when
// the managed executable is still the exact one that authorized its creation.
func publishManagedCompletion(binaryPath, expectedHash, shell, command string, content []byte) (string, error) {
	if _, err := completionFilename(shell, command); err != nil {
		return "", err
	}
	if err := validateCompletionText(content); err != nil {
		return "", err
	}

	published := false
	destination := ""
	err := mutateLockedBinary(binaryPath, func(configPath string, binary *config.Binary, others []*config.Binary) error {
		if err := validateDirectManagedBinary(binary); err != nil {
			return err
		}
		if !strings.EqualFold(binary.Hash, expectedHash) {
			return errors.New("managed binary changed before completion publication")
		}
		if err := verifyManagedBinaryHash(binary, expectedHash); err != nil {
			return err
		}

		var err error
		destination, err = completionDestination(configPath, shell, command)
		if err != nil {
			return err
		}
		if err := ensureCompletionDirectory(filepath.Dir(destination)); err != nil {
			return err
		}
		overwrite, err := completionDestinationIsReplaceable(binary, others, shell, destination)
		if err != nil {
			return err
		}

		stage, err := stageCompletion(destination, content)
		if err != nil {
			return err
		}
		defer os.Remove(stage)
		if err := publishStagedBinary(stage, destination, overwrite); err != nil {
			return err
		}
		published = true
		if binary.CompletionOwnership == nil {
			binary.CompletionOwnership = map[string]*config.CompletionOwnershipRecord{}
		}
		binary.CompletionOwnership[shell] = &config.CompletionOwnershipRecord{
			Path:   destination,
			SHA256: completionHash(content),
		}
		return nil
	})
	if err != nil {
		if published {
			return "", fmt.Errorf("completion published at %s but could not persist ownership; remove it manually and retry: %w", destination, err)
		}
		return "", err
	}
	return destination, nil
}

func completionDestination(configPath, shell, command string) (string, error) {
	filename, err := completionFilename(shell, command)
	if err != nil {
		return "", err
	}

	absoluteConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	directory := filepath.Join(filepath.Dir(absoluteConfigPath), "completions", shell)
	destination := filepath.Join(directory, filename)
	relative, err := filepath.Rel(directory, destination)
	if err != nil || relative != filename {
		return "", errors.New("completion destination escapes owned directory")
	}
	return destination, nil
}

func completionFilename(shell, command string) (string, error) {
	if err := assets.ValidatePortableName(command); err != nil {
		return "", fmt.Errorf("invalid completion command %q: %w", command, err)
	}

	filename := ""
	switch shell {
	case "bash":
		filename = command
	case "zsh":
		filename = "_" + command
	case "fish":
		filename = command + ".fish"
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
	if err := assets.ValidatePortableName(filename); err != nil {
		return "", fmt.Errorf("invalid completion filename %q: %w", filename, err)
	}
	return filename, nil
}

func validateDirectManagedBinary(binary *config.Binary) error {
	if binary == nil {
		return errors.New("binary is not managed")
	}
	if effectiveInstallMode(binary.InstallMode) != installModeBinary {
		return fmt.Errorf("managed binary %s is not a direct binary", binary.Path)
	}
	if binary.Hash == "" {
		return fmt.Errorf("managed binary %s has no installed hash", binary.Path)
	}
	return nil
}

func verifyManagedBinaryHash(binary *config.Binary, expectedHash string) error {
	if !strings.EqualFold(binary.Hash, expectedHash) {
		return errors.New("managed binary changed before completion generation")
	}
	path := expandTrackedBinaryPath(binary.Path)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("lstat managed binary %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed binary %s is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("managed binary %s is not a regular file", path)
	}
	if !completionExecutableFileAllowed(info) {
		return fmt.Errorf("managed binary %s is not executable", path)
	}
	hash, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("hash managed binary %s: %w", path, err)
	}
	if !strings.EqualFold(hash, expectedHash) {
		return fmt.Errorf("managed binary %s has changed", path)
	}
	return nil
}

func completionDestinationIsReplaceable(binary *config.Binary, others []*config.Binary, shell, destination string) (bool, error) {
	if completionClaimedByAnotherBinary(others, destination) {
		return false, fmt.Errorf("completion destination %s is owned by another managed binary", destination)
	}

	ownership := binary.CompletionOwnership[shell]
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		if ownership != nil && ownership.Path != destination {
			return false, fmt.Errorf("completion ownership for %s does not match destination %s", shell, destination)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("refusing to replace symlink completion destination %s", destination)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("refusing to replace non-regular completion destination %s", destination)
	}
	if ownership == nil || ownership.Path != destination {
		return false, fmt.Errorf("completion destination %s is not owned by this binary", destination)
	}
	hash, err := hashFile(destination)
	if err != nil {
		return false, fmt.Errorf("hash completion destination %s: %w", destination, err)
	}
	if !strings.EqualFold(hash, ownership.SHA256) {
		return false, fmt.Errorf("completion destination %s was modified", destination)
	}
	return true, nil
}

func completionClaimedByAnotherBinary(others []*config.Binary, destination string) bool {
	for _, binary := range others {
		if binary == nil {
			continue
		}
		for _, ownership := range binary.CompletionOwnership {
			if ownership != nil && ownership.Path == destination {
				return true
			}
		}
	}
	return false
}

// cleanupManagedCompletions removes only completion files whose recorded
// ownership still proves they are safe to delete. It runs while the config
// record remains locked and is deliberately best-effort so binary removal is
// never blocked by completion cleanup.
func cleanupManagedCompletions(configPath string, binary *config.Binary, others []*config.Binary) {
	if effectiveInstallMode(binary.InstallMode) != installModeBinary {
		return
	}

	for shell, ownership := range binary.CompletionOwnership {
		if ownership == nil {
			continue
		}

		destination, err := canonicalCompletionOwnershipPath(configPath, binary, shell, ownership)
		if err != nil {
			log.Warnf("Leaving completion %s: %v", ownership.Path, err)
			continue
		}
		owned, err := completionDestinationIsReplaceable(binary, others, shell, destination)
		if err != nil {
			log.Warnf("Leaving completion %s: %v", ownership.Path, err)
			continue
		}
		if !owned {
			continue
		}
		if err := removeManagedCompletionFile(destination); err != nil && !os.IsNotExist(err) {
			log.Warnf("Leaving completion %s: %v", ownership.Path, err)
		}
	}
}

// canonicalCompletionOwnershipPath ensures cleanup only removes the same
// destination that publication could have owned for this binary and shell.
func canonicalCompletionOwnershipPath(configPath string, binary *config.Binary, shell string, ownership *config.CompletionOwnershipRecord) (string, error) {
	command := filepath.Base(expandTrackedBinaryPath(binary.Path))
	destination, err := completionDestination(configPath, shell, command)
	if err != nil {
		return "", err
	}
	if ownership.Path != destination {
		return "", fmt.Errorf("completion ownership path is not the canonical managed destination %s", destination)
	}
	return destination, nil
}

func ensureCompletionDirectory(shellDirectory string) error {
	completionDirectory := filepath.Dir(shellDirectory)
	for _, directory := range []string{completionDirectory, shellDirectory} {
		if err := ensureOwnedDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func ensureOwnedDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
			return fmt.Errorf("create completion directory %s: %w", directory, err)
		}
		info, err = os.Lstat(directory)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("refusing non-directory or symlink completion directory %s", directory)
	}
	return nil
}

func stageCompletion(destination string, content []byte) (string, error) {
	stage, err := os.CreateTemp(filepath.Dir(destination), filepath.Base(destination)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("stage completion: %w", err)
	}
	stagePath := stage.Name()
	if _, err := stage.Write(content); err != nil {
		_ = stage.Close()
		_ = os.Remove(stagePath)
		return "", fmt.Errorf("write staged completion: %w", err)
	}
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return "", fmt.Errorf("close staged completion: %w", err)
	}
	return stagePath, nil
}

func validateCompletionText(content []byte) error {
	if len(content) == 0 {
		return errors.New("completion output is empty")
	}
	if len(content) > completionStdoutLimit {
		return errors.New("completion output exceeds limit")
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return errors.New("completion output is not text")
	}
	return nil
}

func completionHash(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}
