package providers

import (
	"strings"
	"unicode"

	version "github.com/hashicorp/go-version"
)

const BareReleaseTagPrefix = "@bare"

func fetchedReleaseTagPrefix(version, requestedPrefix string) string {
	if requestedPrefix == BareReleaseTagPrefix {
		return requestedPrefix
	}
	return ReleaseTagPrefix(version)
}

// ReleaseTagPrefix returns the exact prefix before the first digit in a tag.
// Examples: v1.2.3 -> v, pi-v0.1.0 -> pi-v, 1.2.3 -> "".
func ReleaseTagPrefix(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag != "" && strings.IndexFunc(tag, unicode.IsDigit) < 0 {
		return tag
	}
	if prereleaseLane := releaseTagPrereleaseLane(tag); prereleaseLane != "" {
		return prereleaseLane
	}
	return legacyReleaseTagPrefix(tag)
}

func MatchesReleaseTagPrefix(tag, prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == BareReleaseTagPrefix {
		return ReleaseTagPrefix(tag) == ""
	}
	if ReleaseTagPrefix(tag) == prefix {
		return true
	}

	legacyPrefix := legacyReleaseTagPrefix(tag)
	prereleaseLane := releaseTagPrereleaseToken(tag)
	if legacyPrefix == "" || prereleaseLane == "" {
		return false
	}

	return prefix == legacyPrefix+"-"+prereleaseLane
}

func EffectiveReleaseTagPrefix(version, storedPrefix string) string {
	storedPrefix = strings.TrimSpace(storedPrefix)
	legacyPrefix := legacyReleaseTagPrefix(version)
	if storedPrefix != legacyPrefix {
		return storedPrefix
	}

	if prereleaseLane := releaseTagPrereleaseLane(version); prereleaseLane != "" {
		return prereleaseLane
	}

	return storedPrefix
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

func legacyReleaseTagPrefix(tag string) string {
	for i, r := range tag {
		if r >= '0' && r <= '9' {
			return tag[:i]
		}
	}
	return ""
}

func releaseTagPrereleaseLane(tag string) string {
	legacyPrefix := legacyReleaseTagPrefix(tag)
	lane := releaseTagPrereleaseToken(tag)
	if lane == "" {
		return ""
	}
	if legacyPrefix == "" || legacyPrefix == "v" {
		return lane
	}
	return legacyPrefix + "-" + lane
}

func releaseTagPrereleaseToken(tag string) string {
	start := strings.IndexFunc(tag, unicode.IsDigit)
	if start < 0 {
		return ""
	}

	parsed, err := version.NewVersion(tag[start:])
	if err != nil {
		return ""
	}

	prerelease := parsed.Prerelease()
	if prerelease == "" {
		return ""
	}

	tokens := strings.FieldsFunc(prerelease, func(r rune) bool {
		return r == '.' || r == '-'
	})
	for i := len(tokens) - 1; i >= 0; i-- {
		token := strings.ToLower(strings.TrimSpace(tokens[i]))
		if token == "" || isNumericPrereleaseIdentifier(token) || isCommitLikePrereleaseIdentifier(token) {
			continue
		}
		return token
	}

	return ""
}

func isNumericPrereleaseIdentifier(token string) bool {
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return token != ""
}

func isCommitLikePrereleaseIdentifier(token string) bool {
	if len(token) < 7 {
		return false
	}
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
