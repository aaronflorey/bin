package cmd

import "errors"

func returnStagingFileToCaller(string) error {
	return errors.New("sudo filesystem retries are unavailable on Windows")
}
