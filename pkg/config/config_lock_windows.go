package config

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func withConfigLock(configPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}

	lockPath := configPath + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		return err
	}
	defer windows.UnlockFileEx(handle, 0, 1, 0, overlapped)

	return fn()
}
