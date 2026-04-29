package systempackage

import (
	"errors"
	"fmt"
	"strings"
)

var ErrIncompatible = errors.New("incompatible system package")

var knownTypes = map[string]struct{}{
	"apk":     {},
	"deb":     {},
	"dmg":     {},
	"flatpak": {},
	"rpm":     {},
}

func DetectType(name string) (string, bool) {
	lower := strings.ToLower(name)

	switch {
	case strings.HasSuffix(lower, ".flatpak"), strings.HasSuffix(lower, ".flatpack"):
		return "flatpak", true
	case strings.HasSuffix(lower, ".deb"):
		return "deb", true
	case strings.HasSuffix(lower, ".rpm"):
		return "rpm", true
	case strings.HasSuffix(lower, ".apk"):
		return "apk", true
	case strings.HasSuffix(lower, ".dmg"):
		return "dmg", true
	default:
		return "", false
	}
}

func NormalizeType(packageType string) string {
	switch strings.ToLower(strings.TrimSpace(packageType)) {
	case "flatpack":
		return "flatpak"
	default:
		return strings.ToLower(strings.TrimSpace(packageType))
	}
}

func IsKnownType(packageType string) bool {
	_, ok := knownTypes[NormalizeType(packageType)]
	return ok
}

func NewCompatibilityError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrIncompatible, fmt.Sprintf(format, args...))
}
