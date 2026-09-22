//go:build !windows

package cmd

import "strings"

func commandNameMatches(installedName, requestedName string) bool {
	return installedName == requestedName
}

// isExplicitTargetPath keeps separator-containing command targets out of
// managed-name and alias lookup.
func isExplicitTargetPath(path string) bool {
	return strings.ContainsAny(path, `/\\`)
}

// isExplicitInstallDestination preserves Unix filename semantics: a
// backslash without a slash is a name, not a path separator.
func isExplicitInstallDestination(path string) bool {
	return strings.Contains(path, "/")
}
