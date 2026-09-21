//go:build !windows

package cmd

import "os"

func completionExecutableFileAllowed(info os.FileInfo) bool {
	return info.Mode().Perm()&0o111 != 0
}
