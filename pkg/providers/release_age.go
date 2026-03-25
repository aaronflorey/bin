package providers

import (
	"fmt"
	"time"
)

func PtrTime(t time.Time) *time.Time {
	return &t
}

func ReleaseAgeError(providerID, version string) error {
	return fmt.Errorf("provider %q does not expose release publication time for version %s", providerID, version)
}
