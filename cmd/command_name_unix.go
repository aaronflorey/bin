//go:build !windows

package cmd

func commandNameMatches(installedName, requestedName string) bool {
	return installedName == requestedName
}
