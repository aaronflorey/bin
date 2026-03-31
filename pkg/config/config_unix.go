//go:build !windows

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/caarlos0/log"
	"golang.org/x/sys/unix"
)

// getDefaultPath reads the user's PATH variable
// and returns the first directory that's writable by the current
// user in the system
func getDefaultPath() (string, error) {
	localBin, err := ensureUserLocalBinDir()
	if err == nil {
		return localBin, nil
	}
	log.Debugf("Could not prepare ~/.local/bin: %v", err)

	return selectWritablePathFromEnv(os.Getenv("PATH"), ":")
}

func ensureUserLocalBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	if err := checkDirExistsAndWritable(dir); err != nil {
		return "", err
	}

	return dir, nil
}

func checkDirExistsAndWritable(dir string) error {
	if fi, err := os.Stat(dir); err != nil {
		return fmt.Errorf("Error setting download path [%w]", err)
	} else if !fi.IsDir() {
		return errors.New("Download path is not a directory")
	}
	err := unix.Access(dir, unix.W_OK)
	return err
}
