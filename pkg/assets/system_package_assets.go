package assets

import (
	"path"
	"strings"

	bstrings "github.com/aaronflorey/bin/pkg/strings"
	"github.com/aaronflorey/bin/pkg/systempackage"
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
	return filterAssetsBy(as, func(name string) bool {
		return looksLikeMetadataAsset(name) || looksLikeArchiveJunk(name)
	}, "non-binary archive")
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
