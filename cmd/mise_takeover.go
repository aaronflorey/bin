package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/caarlos0/log"
)

type miseTakeover struct {
	configPath string
	tracked    string
	command    string
	shim       string
}

// reconcileMiseTakeovers forgets direct binaries whose command now resolves
// to a mise shim. It deliberately leaves the old executable and completions
// untouched: this is ownership reconciliation, not an uninstall.
func reconcileMiseTakeovers(bins map[string]*config.Binary, dryRun bool) (map[string]*config.Binary, error) {
	takeovers := findMiseTakeovers(bins)
	if len(takeovers) == 0 {
		return bins, nil
	}

	if !dryRun {
		paths := make([]string, 0, len(takeovers))
		for _, takeover := range takeovers {
			paths = append(paths, takeover.configPath)
		}
		if err := config.RemoveBinaries(paths); err != nil {
			return nil, err
		}
	}

	remaining := make(map[string]*config.Binary, len(bins)-len(takeovers))
	for path, binary := range bins {
		remaining[path] = binary
	}
	for _, takeover := range takeovers {
		delete(remaining, takeover.configPath)
		if dryRun {
			log.Warnf("%s resolves to mise shim %s; stale bin registry entry for %s would be removed", takeover.command, takeover.shim, takeover.tracked)
		} else {
			log.Warnf("%s resolves to mise shim %s; removed stale bin registry entry for %s", takeover.command, takeover.shim, takeover.tracked)
		}
	}
	return remaining, nil
}

func findMiseTakeovers(bins map[string]*config.Binary) []miseTakeover {
	paths := make([]string, 0, len(bins))
	for path := range bins {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	takeovers := make([]miseTakeover, 0)
	for _, configPath := range paths {
		binary := bins[configPath]
		if binary == nil || effectiveInstallMode(binary.InstallMode) != installModeBinary {
			continue
		}
		tracked := expandTrackedBinaryPath(binary.Path)
		command := filepath.Base(tracked)
		if tracked == "" || command == "." || command == string(filepath.Separator) {
			continue
		}
		resolved, err := lookPathCommand(command)
		if err != nil || sameExecutablePath(resolved, tracked) || !isMiseShim(resolved) {
			continue
		}
		takeovers = append(takeovers, miseTakeover{
			configPath: configPath,
			tracked:    tracked,
			command:    command,
			shim:       resolved,
		})
	}
	return takeovers
}

func isMiseShim(path string) bool {
	cleaned := filepath.Clean(path)
	shimDir := filepath.Dir(cleaned)
	if strings.EqualFold(filepath.Base(shimDir), "shims") && strings.EqualFold(filepath.Base(filepath.Dir(shimDir)), "mise") {
		return true
	}

	shimTarget, err := filepath.EvalSymlinks(cleaned)
	if err != nil || !isMiseExecutable(shimTarget) {
		return false
	}
	misePath, err := lookPathCommand("mise")
	if err != nil {
		return false
	}
	miseTarget, err := filepath.EvalSymlinks(misePath)
	return err == nil && sameExecutablePath(shimTarget, miseTarget)
}

func isMiseExecutable(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return name == "mise" || name == "mise.exe"
}

func sameExecutablePath(left, right string) bool {
	left, leftErr := filepath.Abs(filepath.Clean(left))
	right, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	if left == right {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}
