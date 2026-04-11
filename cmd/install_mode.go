package cmd

import "strings"

const (
	installModeBinary        = "binary"
	installModeSystemPackage = "system-package"
)

func effectiveInstallMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return installModeBinary
	}
	return mode
}
