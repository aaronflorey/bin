//go:build !windows

package cmd

func completionEnvironment(workDir string) []string {
	return []string{
		"HOME=" + workDir,
		"PATH=/usr/bin:/bin",
		"TMPDIR=" + workDir,
		"LANG=C",
		"LC_ALL=C",
	}
}
