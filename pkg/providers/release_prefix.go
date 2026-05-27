package providers

import "strings"

// ReleaseTagPrefix returns the exact prefix before the first digit in a tag.
// Examples: v1.2.3 -> v, pi-v0.1.0 -> pi-v, 1.2.3 -> "".
func ReleaseTagPrefix(tag string) string {
	tag = strings.TrimSpace(tag)
	for i, r := range tag {
		if r >= '0' && r <= '9' {
			return tag[:i]
		}
	}
	return ""
}

func MatchesReleaseTagPrefix(tag, prefix string) bool {
	return ReleaseTagPrefix(tag) == strings.TrimSpace(prefix)
}

func SelectReleaseByPrefix(releases []*ReleaseInfo, prefix string) *ReleaseInfo {
	prefix = strings.TrimSpace(prefix)
	for _, release := range releases {
		if release == nil {
			continue
		}
		if MatchesReleaseTagPrefix(release.Version, prefix) {
			return release
		}
	}
	return nil
}
