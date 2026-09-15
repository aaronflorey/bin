//go:build !linux && !darwin && !windows

package cmd

import (
	"fmt"
	"os"
)

func publishStagedBinary(stagePath, destination string, overwrite bool) error {
	if err := rejectDestinationSymlink(destination); err != nil {
		return err
	}
	if overwrite {
		return os.Rename(stagePath, destination)
	}

	// The staged file is a sibling of the destination, so linking it publishes
	// its inode atomically without replacing an existing destination.
	if err := os.Link(stagePath, destination); err != nil {
		return fmt.Errorf("publish %s without overwriting %s: %w", stagePath, destination, err)
	}
	if err := os.Remove(stagePath); err != nil {
		return fmt.Errorf("remove published staging binary %s: %w", stagePath, err)
	}
	return nil
}
