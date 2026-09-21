//go:build windows

package cmd

import "strings"

func commandNameMatches(installedName, requestedName string) bool {
	installedName = strings.TrimSuffix(strings.ToLower(installedName), ".exe")
	requestedName = strings.TrimSuffix(strings.ToLower(requestedName), ".exe")
	return installedName == requestedName
}
