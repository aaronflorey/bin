//go:build !windows

package providers

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// dockerImagePattern validates Docker image names to prevent shell injection.
// Allows: lowercase letters, digits, hyphens, underscores, periods, slashes, and colons.
var dockerImagePattern = regexp.MustCompile(`^[a-z0-9._/-]+$`)

// validateDockerImage checks if the image name is safe for shell embedding.
func validateDockerImage(image string) bool {
	return dockerImagePattern.MatchString(image)
}

const (
	defaultDockerRunTemplate = `#!/bin/sh
	termflag=$([ -t 0 ] && echo -n "-t")
	docker run --rm -i $termflag -v ${PWD}:/tmp/cmd -w /tmp/cmd '%s:%s' "$@"`
)

func dockerWrapperScript(repo, tag string) string {
	tpl := os.Getenv("BIN_DOCKER_RUN_TEMPLATE")
	if strings.TrimSpace(tpl) == "" {
		tpl = defaultDockerRunTemplate
	}
	return fmt.Sprintf(tpl, repo, tag)
}

// getImageName gets the name of the image from the image repo.
func getImageName(repo string) string {
	image := strings.Split(repo, "/")
	return image[len(image)-1]
}
