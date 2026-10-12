//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"strconv"
)

func returnStagingFileToCaller(path string) error {
	uid, err := strconv.Atoi(os.Getenv("SUDO_UID"))
	if err != nil || uid < 0 {
		return fmt.Errorf("invalid sudo caller UID")
	}
	gid, err := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err != nil || gid < 0 {
		return fmt.Errorf("invalid sudo caller GID")
	}
	return os.Chown(path, uid, gid)
}
