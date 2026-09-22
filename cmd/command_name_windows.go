//go:build windows

package cmd

import (
	"path/filepath"
	"strings"
)

func commandNameMatches(installedName, requestedName string) bool {
	installedName = strings.TrimSuffix(strings.ToLower(installedName), ".exe")
	requestedName = strings.TrimSuffix(strings.ToLower(requestedName), ".exe")
	return installedName == requestedName
}

// isWindowsFilesystemPath recognizes relative and absolute Windows paths.
// filepath.VolumeName also covers drive-relative targets such as C:tools\\bin,
// which do not require a separator.
func isWindowsFilesystemPath(path string) bool {
	return filepath.VolumeName(path) != "" || strings.ContainsAny(path, `/\\`)
}

func isExplicitTargetPath(path string) bool {
	return isWindowsFilesystemPath(path)
}

func isExplicitInstallDestination(path string) bool {
	return isWindowsFilesystemPath(path)
}
