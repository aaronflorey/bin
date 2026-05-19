package providers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/caarlos0/log"
)

var sha256Pattern = regexp.MustCompile(`(?i)\b[a-f0-9]{64}\b`)

type checksumAsset struct {
	Name string
	URL  string
}

type checksumScope int

const (
	checksumScopeArchive checksumScope = iota
	checksumScopeFinal
)

type expectedChecksum struct {
	Hash  string
	Scope checksumScope
}

var checksumHTTPClient = &http.Client{Timeout: 30 * time.Second}

func expectedSHA256ForAsset(name string, assets []checksumAsset, headers map[string]string) (*expectedChecksum, error) {
	hashOrder := fetchChecksumHashOrder(assets, headers)
	checksumCandidates := rankedChecksumAssets(name, assets)
	for _, candidate := range checksumCandidates {
		content, err := fetchChecksumFile(candidate.URL, headers)
		if err != nil {
			log.Debugf("Skipping checksum file %s due to fetch error: %v", candidate.URL, err)
			continue
		}

		expected := parseSHA256Checksum(content, name, candidate.Name, hashOrder)
		if expected != nil {
			return expected, nil
		}
	}

	return nil, nil
}

func rankedChecksumAssets(name string, assets []checksumAsset) []checksumAsset {
	target := strings.ToLower(name)
	targetDot := target + "."

	type scoredAsset struct {
		asset checksumAsset
		score int
	}

	scored := []scoredAsset{}
	for _, asset := range assets {
		lower := strings.ToLower(asset.Name)
		if strings.Contains(lower, "hashes_order") {
			continue
		}
		score := 0

		switch {
		case lower == target+".sha256" || lower == target+".sha256sum":
			score = 100
		case checksumAssetMatchesTarget(lower, target):
			score = 95
		case strings.HasPrefix(lower, targetDot) && strings.Contains(lower, "sha256"):
			score = 90
		case strings.Contains(lower, "sha256"):
			score = 70
		case strings.Contains(lower, "checksum"):
			score = 60
		default:
			continue
		}

		scored = append(scored, scoredAsset{asset: asset, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]checksumAsset, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.asset)
	}

	return result
}

func fetchChecksumHashOrder(assets []checksumAsset, headers map[string]string) []string {
	for _, candidate := range rankedChecksumHashOrderAssets(assets) {
		content, err := fetchChecksumFile(candidate.URL, headers)
		if err != nil {
			log.Debugf("Skipping checksum order file %s due to fetch error: %v", candidate.URL, err)
			continue
		}

		order := parseChecksumHashOrder(content)
		if len(order) > 0 {
			return order
		}
	}

	return nil
}

func rankedChecksumHashOrderAssets(assets []checksumAsset) []checksumAsset {
	candidates := make([]checksumAsset, 0, len(assets))
	for _, asset := range assets {
		lower := strings.ToLower(asset.Name)
		if strings.Contains(lower, "hashes_order") {
			candidates = append(candidates, asset)
		}
	}

	return candidates
}

func fetchChecksumFile(url string, headers map[string]string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := checksumHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func parseSHA256Checksum(content, fileName, checksumFileName string, hashOrder []string) *expectedChecksum {
	targetName := strings.ToLower(fileName)
	targetBase := strings.ToLower(filepath.Base(fileName))
	exactChecksumTarget := checksumFileDirectlyTargetsAsset(checksumFileName, fileName)

	unmatched := []string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, targetName) || strings.Contains(lowerLine, targetBase) {
			if hash := selectSHA256FromOrderedFields(fields, hashOrder); hash != "" {
				return &expectedChecksum{Hash: hash, Scope: checksumScopeArchive}
			}

			hashes := extractSHA256Hashes(line)
			if len(hashes) == 1 {
				return &expectedChecksum{Hash: hashes[0], Scope: checksumScopeArchive}
			}
			continue
		}

		hashes := extractSHA256Hashes(line)
		if len(hashes) == 1 {
			unmatched = append(unmatched, hashes[0])
		}
	}

	if len(unmatched) == 1 && checksumAssetMatchesTarget(checksumFileName, fileName) {
		scope := checksumScopeFinal
		if exactChecksumTarget {
			scope = checksumScopeArchive
		}
		return &expectedChecksum{Hash: unmatched[0], Scope: scope}
	}

	return nil
}

func checksumAssetMatchesTarget(checksumFileName, targetName string) bool {
	return normalizeChecksumTargetName(checksumFileName) == normalizeChecksumTargetName(targetName)
}

func checksumFileDirectlyTargetsAsset(checksumFileName, targetName string) bool {
	lowerChecksum := strings.ToLower(filepath.Base(checksumFileName))
	lowerTarget := strings.ToLower(filepath.Base(targetName))
	return lowerChecksum == lowerTarget+".sha256" || lowerChecksum == lowerTarget+".sha256sum"
}

func normalizeChecksumTargetName(name string) string {
	lower := strings.ToLower(filepath.Base(name))

	for _, suffix := range []string{
		".sha256sum",
		".sha256",
		".sha512sum",
		".sha512",
		".sha1sum",
		".sha1",
		".md5sum",
		".md5",
	} {
		if strings.HasSuffix(lower, suffix) {
			lower = strings.TrimSuffix(lower, suffix)
			break
		}
	}

	for _, suffix := range []string{
		".tar.gz",
		".tar.xz",
		".tar.bz2",
		".tar.zst",
		".tgz",
		".tbz2",
		".txz",
		".zip",
		".gz",
		".xz",
		".bz2",
		".zst",
		".msi",
		".pkg",
		".deb",
		".rpm",
		".apk",
		".dmg",
		".appimage",
	} {
		if strings.HasSuffix(lower, suffix) {
			lower = strings.TrimSuffix(lower, suffix)
			break
		}
	}

	return lower
}

func parseChecksumHashOrder(content string) []string {
	order := []string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		order = append(order, normalizeHashAlgorithm(line))
	}

	return order
}

func extractSHA256Hashes(line string) []string {
	matches := sha256Pattern.FindAllString(line, -1)
	if len(matches) == 0 {
		return nil
	}

	hashes := make([]string, 0, len(matches))
	for _, match := range matches {
		hashes = append(hashes, strings.ToLower(match))
	}

	return hashes
}

func selectSHA256FromOrderedFields(fields, hashOrder []string) string {
	if len(hashOrder) == 0 || len(fields) != len(hashOrder)+1 {
		return ""
	}

	for index, algorithm := range hashOrder {
		if algorithm == "sha256" {
			hash := strings.ToLower(fields[index+1])
			if sha256Pattern.MatchString(hash) && len(hash) == 64 {
				return hash
			}
			return ""
		}
	}

	return ""
}

func normalizeHashAlgorithm(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			builder.WriteRune(ch)
		}
	}

	return strings.ToLower(builder.String())
}
