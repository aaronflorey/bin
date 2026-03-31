package config

import (
	"os"

	"github.com/caarlos0/log"
)

// getDefaultPath reads the user's PATH variable
// and returns the first directory that's writable by the current
// user in the system
func getDefaultPath() (string, error) {
	return selectWritablePathFromEnv(os.Getenv("PATH"), ";")
}

func checkDirExistsAndWritable(dir string) error {
	log.Debugf("Checking path %s", dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return err
	}
	return checkDirWritable(dir)
}
