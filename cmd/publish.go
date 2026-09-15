package cmd

import (
	"fmt"
	"os"
)

func rejectDestinationSymlink(destination string) error {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to replace symlink destination %s", destination)
	}
	return nil
}
