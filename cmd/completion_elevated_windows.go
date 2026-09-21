//go:build windows

package cmd

import "golang.org/x/sys/windows"

func processElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
