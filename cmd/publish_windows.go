//go:build windows

package cmd

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func publishStagedBinary(stagePath, destination string, overwrite bool) error {
	if err := rejectDestinationSymlink(destination); err != nil {
		return err
	}
	stage, err := windows.UTF16PtrFromString(stagePath)
	if err != nil {
		return err
	}
	dest, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	flags := uint32(0)
	if overwrite {
		flags = windows.MOVEFILE_REPLACE_EXISTING
	}
	if err := windows.MoveFileEx(stage, dest, flags); err != nil {
		return fmt.Errorf("publish staged binary: %w", err)
	}
	return nil
}
