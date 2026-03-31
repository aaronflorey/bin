//go:build !windows

package providers

import (
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
	// TODO: this probably won't work on windows so we might need how we mount
	// TODO: there might be a way were users can configure a template for the
	// actual execution since some CLIs require some other folders to be mounted
	// or networks to be shared
	sh = `#!/bin/sh
	termflag=$([ -t 0 ] && echo -n "-t")
	docker run --rm -i $termflag -v ${PWD}:/tmp/cmd -w /tmp/cmd '%s:%s' "$@"`
)

// getImageName gets the name of the image from the image repo.
func getImageName(repo string) string {
	image := strings.Split(repo, "/")
	return image[len(image)-1]
}
