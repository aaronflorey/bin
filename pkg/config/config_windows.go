package config

import (
	"os"
	"path/filepath"

	"github.com/caarlos0/log"
)

// getDefaultPath reads the user's PATH variable
// and returns the first directory that's writable by the current
// user in the system
func getDefaultPath() (string, error) {
	return selectWritablePathFromEnv(os.Getenv("PATH"), ";")
}

func getConfigPath() (string, error) {
	if configPath, ok, err := configPathOverride(); ok {
		return configPath, err
	}

	home, homeErr := os.UserHomeDir()
	if homeErr == nil {
		legacyPath := filepath.Join(home, ".bin", "config.json")
		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			return legacyPath, nil
		}
	}

	appData := os.Getenv("APPDATA")
	if _, err := os.Stat(appData); !os.IsNotExist(err) {
		return filepath.Join(appData, "bin", "config.json"), nil
	}

	if homeErr != nil {
		return "", homeErr
	}

	return filepath.Join(home, ".bin", "config.json"), nil
}

func checkDirExistsAndWritable(dir string) error {
	log.Debugf("Checking path %s", dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return err
	}
	return checkDirWritable(dir)
}
