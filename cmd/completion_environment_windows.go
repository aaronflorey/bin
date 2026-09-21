//go:build windows

package cmd

import (
	"os"
	"path/filepath"
)

func completionEnvironment(workDir string) []string {
	systemRoot := os.Getenv("SystemRoot")
	environment := []string{
		"HOME=" + workDir,
		"PATH=" + filepath.Join(systemRoot, "System32"),
		"TMPDIR=" + workDir,
		"LANG=C",
		"LC_ALL=C",
	}
	if systemRoot != "" {
		environment = append(environment, "SystemRoot="+systemRoot, "TEMP="+workDir, "TMP="+workDir)
	}
	return environment
}
