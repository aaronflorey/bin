package assets

import (
	"path"
	"strings"

	bstrings "github.com/aaronflorey/bin/pkg/strings"
	"github.com/aaronflorey/bin/pkg/systempackage"
	"github.com/caarlos0/log"
)

func filterInstallableAssets(opts *FilterOpts, as []*Asset) []*Asset {
	if opts != nil && opts.SystemPackage {
		packagesOnly := make([]*Asset, 0, len(as))
		for _, a := range as {
			if looksLikeMetadataAsset(a.Name) {
				continue
			}
			ptype, ok := systempackage.DetectType(a.Name)
			if !ok {
				continue
			}
			if opts.PackageType != "" && systempackage.NormalizeType(opts.PackageType) != ptype {
				continue
			}
			if !isCompatibleSystemPackageAsset(a.Name, ptype) {
				continue
			}
			packagesOnly = append(packagesOnly, a)
		}
		return packagesOnly
	}

	return filterAssetsBy(as, func(name string) bool {
		return looksLikeMetadataAsset(name) || looksLikePackageArtifact(name)
	}, "metadata/package")
}

type osRank int

const (
	osRankPreferred osRank = iota
	osRankGeneric
	osRankOpposite
)

var knownOSTokens = []string{
	"darwin", "macos", "macosx", "osx", "apple",
	"linux", "manylinux", "android",
	"windows", "win", "win32", "win64",
	"freebsd", "openbsd", "netbsd", "dragonfly",
}

func filterTargetCompatibleAssets(as []*Asset, preferSpecific bool) []*Asset {
	preferredOS := preferredOSTokens()
	preferredArch := make(map[string]struct{})
	for _, token := range preferredArchTokens() {
		preferredArch[token] = struct{}{}
	}

	type rankedAsset struct {
		asset       *Asset
		specificity int
	}
	ranked := make([]rankedAsset, 0, len(as))
	maxSpecificity := 0
	for _, a := range as {
		osMatch := classifyOS(a.Name, preferredOS)
		archMatch := classifyArch(a.Name, preferredArch)
		if osMatch == osRankOpposite || archMatch == archRankOpposite {
			log.Debugf("Skipping wrong-platform asset %s", a.Name)
			continue
		}

		specificity := 0
		if osMatch == osRankPreferred {
			specificity++
		}
		if archMatch == archRankPreferred {
			specificity++
		}
		if specificity > maxSpecificity {
			maxSpecificity = specificity
		}
		ranked = append(ranked, rankedAsset{asset: a, specificity: specificity})
	}

	filtered := make([]*Asset, 0, len(ranked))
	for _, candidate := range ranked {
		if !preferSpecific || candidate.specificity == maxSpecificity {
			filtered = append(filtered, candidate.asset)
		}
	}
	return filtered
}

func preferredOSTokens() map[string]struct{} {
	preferred := make(map[string]struct{})
	for _, token := range resolver.GetOS() {
		preferred[strings.ToLower(token)] = struct{}{}
	}
	if _, ok := preferred["darwin"]; ok {
		preferred["apple"] = struct{}{}
		preferred["macosx"] = struct{}{}
	}
	if _, ok := preferred["linux"]; ok {
		preferred["manylinux"] = struct{}{}
	}
	if _, ok := preferred["windows"]; ok {
		preferred["win32"] = struct{}{}
		preferred["win64"] = struct{}{}
	}
	return preferred
}

func classifyOS(candidate string, preferred map[string]struct{}) osRank {
	lower := strings.ToLower(candidate)
	hasKnown := false
	for _, token := range knownOSTokens {
		if !containsDelimitedToken(lower, token) {
			continue
		}
		hasKnown = true
		if _, ok := preferred[token]; ok {
			return osRankPreferred
		}
	}
	if hasKnown {
		return osRankOpposite
	}
	return osRankGeneric
}

func isCompatibleSystemPackageAsset(name, packageType string) bool {
	if !isPackageManagerAvailable(packageType) {
		return false
	}
	if !isSystemPackageOSCompatible(packageType) {
		return false
	}
	return isSystemPackageArchCompatible(name)
}

func isPackageManagerAvailable(packageType string) bool {
	var tool string
	switch packageType {
	case "deb":
		tool = "dpkg"
	case "rpm":
		tool = "rpm"
	case "apk":
		tool = "apk"
	case "flatpak":
		tool = "flatpak"
	case "dmg":
		tool = "hdiutil"
	default:
		return false
	}

	_, err := lookPath(tool)
	return err == nil
}

func isSystemPackageOSCompatible(packageType string) bool {
	osValues := resolver.GetOS()
	for _, osValue := range osValues {
		if packageType == "dmg" && (strings.EqualFold(osValue, "darwin") || strings.EqualFold(osValue, "macos") || strings.EqualFold(osValue, "osx")) {
			return true
		}
		if packageType != "dmg" && strings.EqualFold(osValue, "linux") {
			return true
		}
	}
	return false
}

func isSystemPackageArchCompatible(name string) bool {
	lower := strings.ToLower(name)
	archTokens := resolver.GetArch()
	for _, token := range archTokens {
		if strings.Contains(lower, strings.ToLower(token)) {
			return true
		}
	}

	knownArchTokens := []string{
		"amd64", "x86_64", "x64", "arm64", "aarch64", "armv7", "armv6", "386", "i386", "i686",
	}
	for _, token := range knownArchTokens {
		if strings.Contains(lower, token) {
			return false
		}
	}

	return true
}

func filterArchiveAssets(as []*Asset) []*Asset {
	filtered := make([]*Asset, 0, len(as))
	for _, a := range as {
		if looksLikeMetadataAsset(a.Name) || looksLikeArchiveJunk(a.Name) {
			log.Debugf("Skipping non-binary archive asset %s", a.Name)
			continue
		}
		filtered = append(filtered, a)
	}
	log.Debugf("filterArchiveAssets: %d entries before, %d after removing metadata/junk", len(as), len(filtered))
	return filtered
}

func looksLikeMetadataAsset(name string) bool {
	lower := strings.ToLower(name)

	if bstrings.HasAnySuffix(lower, metadataSuffixes) {
		return true
	}

	return bstrings.ContainsAny(lower, metadataTokens)
}

func looksLikePackageArtifact(name string) bool {
	lower := strings.ToLower(name)

	return bstrings.HasAnySuffix(lower, packageArtifactSuffixes)
}

func looksLikeArchiveJunk(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
	normalizedWithRoot := "/" + strings.TrimPrefix(normalized, "/")
	base := path.Base(normalized)
	if strings.Contains(normalizedWithRoot, ".dist-info/") || strings.Contains(normalizedWithRoot, ".egg-info/") {
		return true
	}

	if bstrings.ContainsAny(normalizedWithRoot, archiveJunkDirs) {
		return true
	}

	if bstrings.HasAnySuffix(base, archiveJunkSuffixes) {
		return true
	}

	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if isCompressedArchiveSuffix(ext) {
		innerExt := path.Ext(stem)
		if looksLikeManPageExt(innerExt) {
			return true
		}
		ext = innerExt
		stem = strings.TrimSuffix(stem, innerExt)
	}

	if looksLikeManPageExt(ext) {
		return true
	}

	for _, junk := range archiveJunkBaseNames {
		if stem == junk || strings.HasPrefix(stem, junk+"-") || strings.HasPrefix(stem, junk+"_") {
			return true
		}
	}

	return false
}

func isCompressedArchiveSuffix(ext string) bool {
	for _, suffix := range compressedArchiveSuffixes {
		if ext == suffix {
			return true
		}
	}

	return false
}

func looksLikeManPageExt(ext string) bool {
	if len(ext) != 2 {
		return false
	}

	ch := ext[1]
	return ext[0] == '.' && ch >= '1' && ch <= '9'
}
