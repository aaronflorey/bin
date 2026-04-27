package assets

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/options"
	bstrings "github.com/aaronflorey/bin/pkg/strings"
	"github.com/caarlos0/log"
	"github.com/cheggaaa/pb"
	"github.com/h2non/filetype"
	"github.com/h2non/filetype/matchers"
	"github.com/h2non/filetype/types"
	"github.com/krolaw/zipstream"
	"github.com/xi2/xz"
)

var (
	msiType = filetype.AddType("msi", "application/octet-stream")
	ascType = filetype.AddType("asc", "text/plain")

	metadataSuffixes = []string{
		".sha256", ".sha512", ".sha1", ".md5",
		".sha256sum", ".sha512sum",
		".sigstore.json", ".intoto.jsonl",
		".sbom.json", ".spdx.json", ".cyclonedx.json",
		".provenance.json", ".attestation.json", ".attest.json",
		".sig", ".minisig", ".pem", ".crt", ".cer", ".asc",
	}

	metadataTokens = []string{
		"checksum", "sha256sum", "sha512sum",
		"sigstore", "intoto",
		"sbom", "spdx", "cyclonedx",
		"provenance", "attestation", "attest",
	}

	packageArtifactSuffixes = []string{
		".apk", ".deb", ".flatpak", ".msi", ".pkg.tar", ".pkg.tar.xz", ".pkg.tar.zst", ".rpm",
	}

	archiveJunkSuffixes = []string{
		".md", ".markdown", ".rst", ".adoc", ".txt", ".rtf",
		".html", ".htm", ".pdf",
		".png", ".jpg", ".jpeg", ".gif", ".svg",
		".yml", ".yaml", ".json", ".toml", ".ini", ".tpl",
		".example", ".sample",
	}

	archiveJunkBaseNames = []string{
		"license", "unlicense", "copying", "notice",
		"readme", "changelog", "changes", "news",
		"authors", "contributors", "contributing",
		"installation",
	}

	archiveJunkDirs = []string{
		"/autocomplete/", "/completions/", "/complete/", "/contrib/",
	}
)

type Asset struct {
	Name string
	// Some providers (like gitlab) have non-descriptive names for files,
	// so we're using this DisplayName as a helper to produce prettier
	// outputs for bin
	DisplayName string
	URL         string
}

func (g Asset) String() string {
	if g.DisplayName != "" {
		return g.DisplayName
	}
	return g.Name
}

type FilteredAsset struct {
	RepoName     string
	Name         string
	DisplayName  string
	URL          string
	score        int
	ExtraHeaders map[string]string
}

type finalFile struct {
	Source      io.Reader
	Name        string
	PackagePath string
}

type cleanupReadCloser struct {
	io.ReadCloser
	cleanup func() error
}

func (c *cleanupReadCloser) Close() error {
	err := c.ReadCloser.Close()
	if c.cleanup != nil {
		if cleanupErr := c.cleanup(); cleanupErr != nil && err == nil {
			err = cleanupErr
		}
	}
	return err
}

type platformResolver interface {
	GetOS() []string
	GetArch() []string
	GetLibC() []string
	GetOSSpecificExtensions() []string
}

type Filter struct {
	opts          *FilterOpts
	repoName      string
	name          string
	packagePath   string
	containedFile string
}

type FilterOpts struct {
	SkipScoring   bool
	SkipPathCheck bool

	// In case of updates, we're sending the previous version package path
	// so in case it's the same one, we can re-use it.
	PackageName string

	// If target file is in a package format (tar, zip,etc) use this
	// variable to filter the resulting outputs. This is very useful
	// so we don't prompt the user to pick the file again on updates
	PackagePath string

	// SystemPackage enables package-manager artifact selection.
	SystemPackage bool

	// PackageType restricts package-manager artifact selection to a specific
	// type (deb, rpm, apk, flatpak).
	PackageType string

	// NonInteractive disables all interactive prompts and auto-selects
	// the best option using tie-breaking heuristics
	NonInteractive bool
}

type runtimeResolver struct{}

func (runtimeResolver) GetOS() []string {
	return config.GetOS()
}

func (runtimeResolver) GetArch() []string {
	return config.GetArch()
}

func (runtimeResolver) GetLibC() []string {
	return config.GetLibC()
}

func (runtimeResolver) GetOSSpecificExtensions() []string {
	return config.GetOSSpecificExtensions()
}

var resolver platformResolver = runtimeResolver{}
var selectOption = options.Select
var isInteractive = options.IsInteractive
var lookPath = exec.LookPath

// httpClient is a shared HTTP client with reasonable timeouts for downloading assets.
var httpClient = &http.Client{
	Timeout: 10 * time.Minute, // Allow long downloads
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

func (g FilteredAsset) String() string {
	if g.DisplayName != "" {
		return g.DisplayName
	}
	return g.Name
}

func NewFilter(opts *FilterOpts) *Filter {
	return &Filter{opts: opts}
}

// ParseAutoSelection parses the autoSelect string which may contain a colon-separated
// outer file and inner path (e.g. "archive.tar.gz:binary"). It stores the inner path
// in f.containedFile and returns the outer filename for asset matching.
func (f *Filter) ParseAutoSelection(autoSelect string) string {
	if autoSelect == "" {
		return ""
	}
	parts := strings.SplitN(autoSelect, ":", 2)
	if len(parts) == 2 {
		f.containedFile = parts[1]
		return parts[0]
	}
	return autoSelect
}

// FilterAssets receives a slice of GL assets and tries to
// select the proper one and ask the user to manually select one
// in case it can't determine it
func (f *Filter) FilterAssets(repoName string, as []*Asset, autoSelect string) (*FilteredAsset, error) {
	f.repoName = repoName
	matchName := f.preferredMatchName(repoName)
	as = filterInstallableAssets(f.opts, as)

	matches := []*FilteredAsset{}
	if len(as) == 1 {
		a := as[0]
		matches = append(matches, &FilteredAsset{RepoName: repoName, Name: a.Name, URL: a.URL, score: 0})
	} else {
		if !f.opts.SkipScoring {
			scores := map[string]int{}
			scoreKeys := []string{}
			scores[matchName] = 1
			for _, os := range resolver.GetOS() {
				scores[os] = 10
			}
			for _, arch := range resolver.GetArch() {
				scores[arch] = 5
			}
			for _, osSpecificExtension := range resolver.GetOSSpecificExtensions() {
				scores[osSpecificExtension] = osSpecificExtensionScore(osSpecificExtension)
			}

			for key := range scores {
				scoreKeys = append(scoreKeys, strings.ToLower(key))
			}

			for _, a := range as {
				highestScoreForAsset := 0
				gf := &FilteredAsset{RepoName: repoName, Name: a.Name, DisplayName: a.DisplayName, URL: a.URL, score: 0}
				candidate := a.Name
				candidateScore := 0
				if bstrings.ContainsAny(strings.ToLower(candidate), scoreKeys) &&
					f.supportsAssetExt(candidate) {
					for toMatch, score := range scores {
						if strings.Contains(strings.ToLower(candidate), strings.ToLower(toMatch)) {
							log.Debugf("Candidate %s contains %s. Adding score %d", candidate, toMatch, score)
							candidateScore += score
						}
					}
					if candidateScore > highestScoreForAsset {
						highestScoreForAsset = candidateScore
						gf.Name = candidate
						gf.score = candidateScore
					}
				}

				if highestScoreForAsset > 0 {
					matches = append(matches, gf)
				}
			}
			highestAssetScore := 0
			for i := range matches {
				if matches[i].score > highestAssetScore {
					highestAssetScore = matches[i].score
				}
			}
			for i := len(matches) - 1; i >= 0; i-- {
				if matches[i].score < highestAssetScore {
					log.Debugf("Removing %v (URL %v) with score %v lower than %v", matches[i].Name, matches[i].URL, matches[i].score, highestAssetScore)
					matches = append(matches[:i], matches[i+1:]...)
				} else {
					log.Debugf("Keeping %v (URL %v) with highest score %v", matches[i].Name, matches[i].URL, matches[i].score)
				}
			}
			matches = rankLinuxLibCMatches(matches)
			matches = rankArchitectureMatches(matches)
			matches = applyTieBreakers(matchName, matches)

		} else {
			log.Debugf("--all flag was supplied, skipping scoring")
			for _, a := range as {
				matches = append(matches, &FilteredAsset{RepoName: repoName, Name: a.Name, DisplayName: a.DisplayName, URL: a.URL, score: 0})
			}
		}
	}

	var gf *FilteredAsset
	if len(matches) == 0 {
		return nil, fmt.Errorf("Could not find any compatible files")
	} else if len(matches) > 1 {
		// If an auto-selection was provided, find the first match by name
		if autoSelect != "" {
			for _, m := range matches {
				if m.String() == autoSelect {
					return m, nil
				}
			}
		}

		generic := make([]fmt.Stringer, 0)
		for _, f := range matches {
			generic = append(generic, f)
		}

		sort.SliceStable(generic, func(i, j int) bool {
			return generic[i].String() < generic[j].String()
		})

		// If non-interactive mode is enabled, auto-select the first match
		if f.opts.NonInteractive {
			log.Infof("Auto-selecting first match in non-interactive mode: %s", generic[0])
			gf = generic[0].(*FilteredAsset)
		} else if !isInteractive() {
			opts := make([]string, 0, len(generic))
			for _, candidate := range generic {
				opts = append(opts, candidate.String())
			}
			return nil, fmt.Errorf(
				"multiple matches found in non-interactive mode: %s (use --select to choose one or use --non-interactive flag)",
				strings.Join(opts, ", "),
			)
		} else {
			choice, err := selectOption("Multiple matches found, please select one:", generic)
			if err != nil {
				return nil, err
			}
			gf = choice.(*FilteredAsset)
		}
	} else {
		gf = matches[0]
	}

	return gf, nil
}

func osSpecificExtensionScore(extension string) int {
	if strings.EqualFold(extension, "AppImage") {
		// AppImages are Linux-compatible, but should not outrank native Linux binaries.
		return 8
	}

	return 15
}

func (f *Filter) preferredMatchName(repoName string) string {
	if f.opts == nil {
		return repoName
	}
	if f.opts.PackageName != "" && !looksLikeMetadataAsset(f.opts.PackageName) && !looksLikePackageArtifact(f.opts.PackageName) {
		return f.opts.PackageName
	}
	if f.opts.PackagePath != "" {
		return filepath.Base(f.opts.PackagePath)
	}
	return repoName
}

func (f *Filter) supportsAssetExt(filename string) bool {
	if f != nil && f.opts != nil && f.opts.SystemPackage {
		ptype, ok := detectSystemPackageType(filename)
		if ok {
			if f.opts.PackageType == "" {
				return true
			}
			return normalizePackageType(f.opts.PackageType) == ptype
		}
	}

	return isSupportedExt(filename)
}

func rankLinuxLibCMatches(matches []*FilteredAsset) []*FilteredAsset {
	if len(matches) <= 1 {
		return matches
	}

	preferred := resolver.GetLibC()
	if len(preferred) == 0 {
		return matches
	}

	preferredSet := make(map[string]struct{}, len(preferred))
	for _, token := range preferred {
		preferredSet[strings.ToLower(token)] = struct{}{}
	}

	bestRank := libCRankUnknown
	filtered := make([]*FilteredAsset, 0, len(matches))
	for _, match := range matches {
		rank := classifyLibC(match.Name, preferredSet)
		if rank < bestRank {
			bestRank = rank
			filtered = filtered[:0]
			filtered = append(filtered, match)
			continue
		}
		if rank == bestRank {
			filtered = append(filtered, match)
		}
	}

	if len(filtered) == len(matches) {
		return matches
	}

	for _, match := range filtered {
		log.Debugf("Keeping %v after Linux libc ranking", match.Name)
	}
	return filtered
}

func rankArchitectureMatches(matches []*FilteredAsset) []*FilteredAsset {
	if len(matches) <= 1 {
		return matches
	}

	preferred := preferredArchTokens()
	if len(preferred) == 0 {
		return matches
	}

	preferredSet := make(map[string]struct{}, len(preferred))
	for _, token := range preferred {
		preferredSet[token] = struct{}{}
	}

	bestRank := archRankUnknown
	filtered := make([]*FilteredAsset, 0, len(matches))
	for _, match := range matches {
		rank := classifyArch(match.Name, preferredSet)
		if rank < bestRank {
			bestRank = rank
			filtered = filtered[:0]
			filtered = append(filtered, match)
			continue
		}
		if rank == bestRank {
			filtered = append(filtered, match)
		}
	}

	if len(filtered) == len(matches) {
		return matches
	}

	for _, match := range filtered {
		log.Debugf("Keeping %v after architecture ranking", match.Name)
	}
	return filtered
}

type libCRank int

const (
	libCRankPreferred libCRank = iota
	libCRankGeneric
	libCRankOpposite
	libCRankUnknown
)

type archRank int

const (
	archRankPreferred archRank = iota
	archRankGeneric
	archRankOpposite
	archRankUnknown
)

var knownLibCTokens = []string{"gnu", "glibc", "musl"}
var knownArchTokens = []string{
	"amd64", "x86_64", "x64", "64bit",
	"arm64", "aarch64",
	"386", "i386", "x86", "32bit",
	"armv7", "armv6", "arm",
	"ppc64le", "s390x", "riscv64", "mips64", "mips64le",
}

func classifyLibC(candidate string, preferredSet map[string]struct{}) libCRank {
	lower := strings.ToLower(candidate)

	hasPreferred := false
	hasKnownLibC := false
	for _, token := range knownLibCTokens {
		if strings.Contains(lower, token) {
			hasKnownLibC = true
			if _, ok := preferredSet[token]; ok {
				hasPreferred = true
			}
		}
	}

	switch {
	case hasPreferred:
		return libCRankPreferred
	case !hasKnownLibC:
		return libCRankGeneric
	default:
		return libCRankOpposite
	}
}

func preferredArchTokens() []string {
	preferred := append([]string{}, resolver.GetArch()...)
	lowerPreferred := make(map[string]struct{}, len(preferred))
	for i := range preferred {
		preferred[i] = strings.ToLower(preferred[i])
		lowerPreferred[preferred[i]] = struct{}{}
	}

	if _, ok := lowerPreferred["amd64"]; ok {
		preferred = append(preferred, "64bit")
	}
	if _, ok := lowerPreferred["x86_64"]; ok {
		preferred = append(preferred, "64bit")
	}

	return appendUnique(nil, preferred...)
}

func classifyArch(candidate string, preferredSet map[string]struct{}) archRank {
	lower := strings.ToLower(candidate)

	hasPreferred := false
	hasKnownArch := false
	for _, token := range knownArchTokens {
		if containsDelimitedToken(lower, token) {
			hasKnownArch = true
			if _, ok := preferredSet[token]; ok {
				hasPreferred = true
			}
		}
	}

	switch {
	case hasPreferred:
		return archRankPreferred
	case !hasKnownArch:
		return archRankGeneric
	default:
		return archRankOpposite
	}
}

func containsDelimitedToken(candidate, token string) bool {
	start := 0
	for {
		index := strings.Index(candidate[start:], token)
		if index < 0 {
			return false
		}
		index += start

		beforeOK := index == 0 || !isAlphaNumeric(candidate[index-1])
		afterIndex := index + len(token)
		afterOK := afterIndex == len(candidate) || !isAlphaNumeric(candidate[afterIndex])
		if beforeOK && afterOK {
			return true
		}
		start = index + 1
	}
}

func isAlphaNumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

// applyTieBreakers applies additional ranking when assets have equal scores.
// This is critical for non-interactive mode to automatically select the best option.
func applyTieBreakers(repoName string, matches []*FilteredAsset) []*FilteredAsset {
	if len(matches) <= 1 {
		return matches
	}

	log.Debugf("Applying tie-breakers to %d matches with equal scores", len(matches))

	// Step 1: Prefer standalone files over archives
	previous := matches
	matches = rankByArchiveType(matches)
	if len(matches) == 0 {
		log.Debugf("Tie-breaker returned no matches after archive ranking; falling back to previous candidates")
		matches = previous
	}
	if len(matches) == 1 {
		log.Debugf("Tie-breaker: selected standalone file")
		return matches
	}

	// Note: Archive format preference is already handled by rankByArchiveType

	// Step 3: Filename similarity to repo name
	previous = matches
	matches = rankByNameSimilarity(repoName, matches)
	if len(matches) == 0 {
		log.Debugf("Tie-breaker returned no matches after name similarity ranking; falling back to previous candidates")
		matches = previous
	}
	if len(matches) == 1 {
		log.Debugf("Tie-breaker: selected by name similarity to %s", repoName)
		return matches
	}
	if len(matches) == 0 {
		return matches
	}

	// Step 4: Alphabetical (deterministic fallback)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})
	log.Debugf("Tie-breaker: selected first alphabetically: %s", matches[0].Name)

	return matches[:1] // Return first after all tie-breaking
}

// archiveType represents the type of file/archive
type archiveType int

const (
	archiveTypeStandalone archiveType = iota // No archive extension
	archiveTypeTarGz                         // .tar.gz
	archiveTypeTarXz                         // .tar.xz
	archiveTypeGz                            // .gz (standalone compressed)
	archiveTypeZip                           // .zip
	archiveTypeOther                         // Other archives
)

// getArchiveType determines what type of archive a file is
func getArchiveType(name string) archiveType {
	lower := strings.ToLower(name)

	// Check for specific archive types (order matters - check .tar.gz before .gz)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return archiveTypeTarGz
	}
	if strings.HasSuffix(lower, ".tar.xz") {
		return archiveTypeTarXz
	}
	if strings.HasSuffix(lower, ".gz") {
		return archiveTypeGz
	}
	if strings.HasSuffix(lower, ".zip") {
		return archiveTypeZip
	}

	// Check if it has any other archive extension
	ext := filepath.Ext(lower)
	if ext == ".xz" || ext == ".bz2" || ext == ".tar" {
		return archiveTypeOther
	}

	// Standalone binary
	return archiveTypeStandalone
}

// rankByArchiveType prefers standalone files over archives
func rankByArchiveType(matches []*FilteredAsset) []*FilteredAsset {
	if len(matches) <= 1 {
		return matches
	}

	// Group by archive type
	byType := make(map[archiveType][]*FilteredAsset)
	for _, match := range matches {
		aType := getArchiveType(match.Name)
		byType[aType] = append(byType[aType], match)
	}

	// Prefer in this order: standalone, .tar.gz, .tar.xz, .gz, .zip, other
	preferenceOrder := []archiveType{
		archiveTypeStandalone,
		archiveTypeTarGz,
		archiveTypeTarXz,
		archiveTypeGz,
		archiveTypeZip,
		archiveTypeOther,
	}

	for _, preferred := range preferenceOrder {
		if candidates := byType[preferred]; len(candidates) > 0 {
			for _, c := range candidates {
				log.Debugf("Keeping %s (archive type preference)", c.Name)
			}
			return candidates
		}
	}

	return matches
}

// rankByNameSimilarity filters matches to keep only those with highest
// similarity to the repository name
func rankByNameSimilarity(repoName string, matches []*FilteredAsset) []*FilteredAsset {
	if len(matches) <= 1 {
		return matches
	}

	// Extract the actual repo name from potential path formats
	// e.g., "owner/repo" -> "repo", "repo" -> "repo"
	parts := strings.Split(repoName, "/")
	shortName := strings.ToLower(parts[len(parts)-1])

	type scoredMatch struct {
		match *FilteredAsset
		score int
	}

	scored := make([]scoredMatch, 0, len(matches))
	for _, match := range matches {
		score := calculateNameSimilarity(shortName, strings.ToLower(match.Name))
		scored = append(scored, scoredMatch{match: match, score: score})
	}
	if len(scored) == 0 {
		return matches
	}

	// Find highest score
	maxScore := scored[0].score
	for _, s := range scored {
		if s.score > maxScore {
			maxScore = s.score
		}
	}

	// Keep only matches with highest score
	filtered := make([]*FilteredAsset, 0, len(matches))
	for _, s := range scored {
		if s.score == maxScore {
			filtered = append(filtered, s.match)
			log.Debugf("Keeping %s (similarity score: %d)", s.match.Name, s.score)
		}
	}
	if len(filtered) == 0 {
		return matches
	}

	return filtered
}

// calculateNameSimilarity returns a similarity score between repo name and asset name
// Higher score means more similar
func calculateNameSimilarity(repoName, assetName string) int {
	score := 0

	// Bonus points if repo name appears in asset name
	if strings.Contains(assetName, repoName) {
		score += 100
	}

	// Additional points for exact prefix match
	if strings.HasPrefix(assetName, repoName) {
		score += 50
	}

	// Penalty for longer names (prefer simpler names)
	// Subtract 1 point per character over the repo name length
	if len(assetName) > len(repoName) {
		score -= (len(assetName) - len(repoName))
	}

	return score
}

// SanitizeName removes irrelevant information from the
// file name in case it exists
func SanitizeName(name, version string) string {
	name = strings.ToLower(name)
	replacementSet := map[string]struct{}{}
	addReplacement := func(value string) {
		if value == "" {
			return
		}
		replacementSet[value] = struct{}{}
	}
	separators := []string{"_", "-", "."}
	innerSeparators := []string{"_", "-", "."}

	osNames := appendUnique(
		resolver.GetOS(),
		"darwin", "macos", "osx", "apple",
		"linux",
		"windows", "win",
		"freebsd", "openbsd", "netbsd", "dragonfly",
		"android",
	)
	archNames := appendUnique(
		resolver.GetArch(),
		"amd64", "x86_64", "x64",
		"arm64", "aarch64", "armv7", "armv6", "arm",
		"386", "i386", "x86",
	)

	for _, osName := range osNames {
		for _, archName := range archNames {
			for _, sep := range separators {
				addReplacement(sep + osName + archName)
				addReplacement(sep + archName + osName)
				for _, innerSep := range innerSeparators {
					addReplacement(sep + osName + innerSep + archName)
					addReplacement(sep + archName + innerSep + osName)
				}
			}
		}

		for _, sep := range separators {
			addReplacement(sep + osName)
		}
	}

	for _, archName := range archNames {
		for _, sep := range separators {
			addReplacement(sep + archName)
		}
	}

	trimmedVersion := strings.TrimPrefix(strings.ToLower(version), "v")
	for _, sep := range separators {
		addReplacement(sep + trimmedVersion)
		addReplacement(sep + "v" + trimmedVersion)
	}

	replacements := make([]string, 0, len(replacementSet)*2)
	keys := make([]string, 0, len(replacementSet))
	for key := range replacementSet {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) == len(keys[j]) {
			return keys[i] < keys[j]
		}
		return len(keys[i]) > len(keys[j])
	})
	for _, key := range keys {
		replacements = append(replacements, key, "")
	}

	r := strings.NewReplacer(replacements...)
	name = r.Replace(name)
	name = strings.Trim(name, "._-")
	name = strings.ReplaceAll(name, "__", "_")
	name = strings.ReplaceAll(name, "--", "-")
	name = strings.ReplaceAll(name, "..", ".")
	return strings.Trim(name, "._-")
}

func appendUnique(values []string, additions ...string) []string {
	out := make([]string, 0, len(values)+len(additions))
	seen := map[string]struct{}{}
	for _, v := range append(values, additions...) {
		v = strings.ToLower(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ProcessURL processes a FilteredAsset by uncompressing/unarchiving the URL of the asset.
func (f *Filter) ProcessURL(gf *FilteredAsset) (*finalFile, error) {
	f.name = gf.Name
	// We're not closing the body here since the caller is in charge of that
	req, err := http.NewRequest(http.MethodGet, gf.URL, nil)
	if err != nil {
		return nil, err
	}
	for name, value := range gf.ExtraHeaders {
		req.Header.Add(name, value)
	}
	log.Debugf("Checking binary from %s", gf.URL)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, fmt.Errorf("%d response when checking binary from %s", res.StatusCode, gf.URL)
	}

	// We're caching the whole file into memory so we can prompt
	// the user which file they want to download

	log.Infof("Starting download of %s", gf.URL)
	bar := pb.Full.Start64(res.ContentLength)
	barReader := bar.NewProxyReader(res.Body)
	defer bar.Finish()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, barReader)
	if err != nil {
		return nil, err
	}
	bar.Finish()
	return f.processReader(buf)
}

func (f *Filter) processReader(r io.Reader) (*finalFile, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(r, &buf)

	t, err := filetype.MatchReader(tee)
	if err != nil {
		return nil, err
	}

	outputFile := io.MultiReader(&buf, r)

	type processorFunc func(repoName string, r io.Reader, autoSelect string) (*finalFile, error)
	var processor processorFunc
	switch t {
	case matchers.TypeGz:
		processor = f.processGz
	case matchers.TypeTar:
		processor = f.processTar
	case matchers.TypeXz:
		processor = f.processXz
	case matchers.TypeBz2:
		processor = f.processBz2
	case matchers.TypeZip:
		processor = f.processZip
	}

	if processor != nil {
		// log.Debugf("Processing %s file %s with %s", repoName, name, runtime.FuncForPC(reflect.ValueOf(processor).Pointer()).Name())
		outFile, err := processor(f.repoName, outputFile, f.containedFile)
		if err != nil {
			return nil, err
		}

		outputFile = outFile.Source

		f.name = outFile.Name
		f.packagePath = outFile.PackagePath

		// In case of e.g. a .tar.gz, process the uncompressed archive by calling recursively
		return f.processReader(outputFile)
	}

	return &finalFile{Source: outputFile, Name: f.name, PackagePath: f.packagePath}, err
}

// processGz receives a tar.gz file and returns the
// correct file for bin to download
func (f *Filter) processGz(name string, r io.Reader, _ string) (*finalFile, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}

	return &finalFile{Source: gr, Name: gr.Name}, nil
}

// matchesPackagePath returns true if the entry name matches the configured
// PackagePath by comparing base filenames. This allows archive entries
// to match even when the directory name changes between versions.
func (f *Filter) matchesPackagePath(entryName string) bool {
	if f.opts.SkipPathCheck || len(f.opts.PackagePath) == 0 {
		return true
	}
	return filepath.Base(entryName) == filepath.Base(f.opts.PackagePath)
}

func (f *Filter) processTar(name string, r io.Reader, autoSelect string) (*finalFile, error) {
	tr := tar.NewReader(r)
	tarFiles := map[string]string{}
	tempDir, err := os.MkdirTemp("", "bin-tar-*")
	if err != nil {
		return nil, err
	}
	cleanupTempDir := true
	defer func() {
		if cleanupTempDir {
			_ = os.RemoveAll(tempDir)
		}
	}()

	if len(f.opts.PackagePath) > 0 {
		log.Debugf("Processing tag with PackagePath %s\n", f.opts.PackagePath)
	}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else if header.FileInfo().IsDir() {
			continue
		}

		if !f.matchesPackagePath(header.Name) {
			continue
		}

		if header.Typeflag == tar.TypeReg {
			entryFile, err := os.CreateTemp(tempDir, "entry-*")
			if err != nil {
				return nil, err
			}

			if _, err := io.Copy(entryFile, tr); err != nil {
				entryFile.Close()
				return nil, err
			}

			if err := entryFile.Close(); err != nil {
				return nil, err
			}

			tarFiles[header.Name] = entryFile.Name()
		}
	}
	if len(tarFiles) == 0 {
		return nil, fmt.Errorf("no files found in tar archive, use -p flag to manually select . PackagePath [%s]", f.opts.PackagePath)
	}

	as := make([]*Asset, 0)
	for f := range tarFiles {
		as = append(as, &Asset{Name: f, URL: ""})
	}
	as = filterArchiveAssets(as)
	choice, err := f.FilterAssets(name, as, autoSelect)
	if err != nil {
		return nil, err
	}
	selectedFile := choice.String()

	selectedPath, ok := tarFiles[selectedFile]
	if !ok {
		return nil, fmt.Errorf("selected file %s not found in tar archive", selectedFile)
	}

	tf, err := os.Open(selectedPath)
	if err != nil {
		return nil, err
	}

	cleanupTempDir = false

	reader := &cleanupReadCloser{
		ReadCloser: tf,
		cleanup: func() error {
			return os.RemoveAll(tempDir)
		},
	}

	return &finalFile{Source: reader, Name: filepath.Base(selectedFile), PackagePath: selectedFile}, nil
}

func (f *Filter) processBz2(name string, r io.Reader, _ string) (*finalFile, error) {
	br := bzip2.NewReader(r)

	return &finalFile{Source: br, Name: name}, nil
}

func (f *Filter) processXz(name string, r io.Reader, _ string) (*finalFile, error) {
	xr, err := xz.NewReader(r, 0)
	if err != nil {
		return nil, err
	}

	return &finalFile{Source: xr, Name: name}, nil
}

func (f *Filter) processZip(name string, r io.Reader, autoSelect string) (*finalFile, error) {
	zr := zipstream.NewReader(r)

	zipFiles := map[string]string{}
	tempDir, err := os.MkdirTemp("", "bin-zip-*")
	if err != nil {
		return nil, err
	}
	cleanupTempDir := true
	defer func() {
		if cleanupTempDir {
			_ = os.RemoveAll(tempDir)
		}
	}()

	if len(f.opts.PackagePath) > 0 {
		log.Debugf("Processing tag with PackagePath %s\n", f.opts.PackagePath)
	}
	for {
		header, err := zr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else if header.Mode().IsDir() {
			continue
		}

		if !f.matchesPackagePath(header.Name) {
			continue
		}

		entryFile, err := os.CreateTemp(tempDir, "entry-*")
		if err != nil {
			return nil, err
		}

		if _, err := io.Copy(entryFile, zr); err != nil {
			entryFile.Close()
			return nil, err
		}

		if err := entryFile.Close(); err != nil {
			return nil, err
		}

		zipFiles[header.Name] = entryFile.Name()
	}
	if len(zipFiles) == 0 {
		return nil, fmt.Errorf("No files found in zip archive. PackagePath [%s]", f.opts.PackagePath)
	}

	as := make([]*Asset, 0)
	for f := range zipFiles {
		as = append(as, &Asset{Name: f, URL: ""})
	}
	as = filterArchiveAssets(as)
	choice, err := f.FilterAssets(name, as, autoSelect)
	if err != nil {
		return nil, err
	}
	selectedFile := choice.String()

	selectedPath, ok := zipFiles[selectedFile]
	if !ok {
		return nil, fmt.Errorf("selected file %s not found in zip archive", selectedFile)
	}

	fr, err := os.Open(selectedPath)
	if err != nil {
		return nil, err
	}

	cleanupTempDir = false

	reader := &cleanupReadCloser{
		ReadCloser: fr,
		cleanup: func() error {
			return os.RemoveAll(tempDir)
		},
	}

	// return base of selected file since tar
	// files usually have folders inside
	return &finalFile{Name: filepath.Base(selectedFile), Source: reader, PackagePath: selectedFile}, nil
}

// isSupportedExt checks if this provider supports
// dealing with this specific file extension
func isSupportedExt(filename string) bool {
	if looksLikePackageArtifact(filename) {
		log.Debugf("Filename %s is a package-manager artifact", filename)
		return false
	}

	if ext := strings.TrimPrefix(filepath.Ext(filename), "."); len(ext) > 0 {
		switch filetype.GetType(ext) {
		case msiType, matchers.TypeDeb, matchers.TypeRpm, ascType:
			log.Debugf("Filename %s doesn't have a supported extension", filename)
			return false
		case matchers.TypeGz, types.Unknown, matchers.TypeZip, matchers.TypeXz, matchers.TypeTar, matchers.TypeBz2, matchers.TypeExe:
			break
		default:
			log.Debugf("Filename %s doesn't have a supported extension", filename)
			return false
		}
	}

	return true
}

// filterAssetsBy removes assets matching the skip predicate, falling back to
// the original list if every asset would be removed.
func filterAssetsBy(as []*Asset, skip func(name string) bool, label string) []*Asset {
	filtered := make([]*Asset, 0, len(as))
	for _, a := range as {
		if skip(a.Name) {
			log.Debugf("Skipping %s asset %s", label, a.Name)
			continue
		}
		filtered = append(filtered, a)
	}

	if len(filtered) == 0 {
		log.Debugf("All %d assets matched %s filter, keeping original list", len(as), label)
		return as
	}
	return filtered
}

func filterInstallableAssets(opts *FilterOpts, as []*Asset) []*Asset {
	if opts != nil && opts.SystemPackage {
		packagesOnly := make([]*Asset, 0, len(as))
		for _, a := range as {
			if looksLikeMetadataAsset(a.Name) {
				continue
			}
			ptype, ok := detectSystemPackageType(a.Name)
			if !ok {
				continue
			}
			if opts.PackageType != "" && normalizePackageType(opts.PackageType) != ptype {
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
	default:
		return false
	}

	_, err := lookPath(tool)
	return err == nil
}

func isSystemPackageOSCompatible(_ string) bool {
	osValues := resolver.GetOS()
	for _, osValue := range osValues {
		if strings.EqualFold(osValue, "linux") {
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

func detectSystemPackageType(name string) (string, bool) {
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
	default:
		return "", false
	}
}

func normalizePackageType(packageType string) string {
	switch strings.ToLower(strings.TrimSpace(packageType)) {
	case "flatpack":
		return "flatpak"
	default:
		return strings.ToLower(strings.TrimSpace(packageType))
	}
}

func looksLikeArchiveJunk(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
	base := path.Base(normalized)

	if bstrings.ContainsAny(normalized, archiveJunkDirs) {
		return true
	}

	if bstrings.HasAnySuffix(base, archiveJunkSuffixes) {
		return true
	}

	ext := path.Ext(base)
	if looksLikeManPageExt(ext) {
		return true
	}

	stem := strings.TrimSuffix(base, ext)
	for _, junk := range archiveJunkBaseNames {
		if stem == junk || strings.HasPrefix(stem, junk+"-") || strings.HasPrefix(stem, junk+"_") {
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
