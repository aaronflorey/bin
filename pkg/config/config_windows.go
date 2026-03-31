package config

import (
	"errors"
	"fmt"
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

	// Check if the user bit is enabled in file permission
	if info.Mode().Perm()&(1<<(uint(7))) == 0 {
		return errors.New(fmt.Sprintf("Dir %s is not writable", dir))
	}
	return nil

}
