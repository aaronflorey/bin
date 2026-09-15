//go:build linux

package cmd

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func publishStagedBinary(stagePath, destination string, overwrite bool) error {
	if err := rejectDestinationSymlink(destination); err != nil {
		return err
	}
	if overwrite {
		return os.Rename(stagePath, destination)
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stagePath, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("publish %s without overwriting %s: %w", stagePath, destination, err)
	}
	return nil
}
