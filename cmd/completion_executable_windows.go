//go:build windows

package cmd

import "os"

func completionExecutableFileAllowed(os.FileInfo) bool {
	return true
}
