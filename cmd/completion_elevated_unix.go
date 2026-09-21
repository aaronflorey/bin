//go:build !windows

package cmd

import "os"

func processElevated() bool {
	return os.Geteuid() == 0
}
