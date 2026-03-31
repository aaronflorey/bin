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

var checksumHTTPClient = &http.Client{Timeout: 30 * time.Second}

func expectedSHA256ForAsset(name string, assets []checksumAsset, headers map[string]string) (string, error) {
	checksumCandidates := rankedChecksumAssets(name, assets)
	for _, candidate := range checksumCandidates {
		content, err := fetchChecksumFile(candidate.URL, headers)
		if err != nil {
			log.Debugf("Skipping checksum file %s due to fetch error: %v", candidate.URL, err)
			continue
		}

		hash := parseSHA256Checksum(content, name)
		if hash != "" {
			return hash, nil
		}
	}

	return "", nil
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
		score := 0

		switch {
		case lower == target+".sha256" || lower == target+".sha256sum":
			score = 100
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

func parseSHA256Checksum(content, fileName string) string {
	targetName := strings.ToLower(fileName)
	targetBase := strings.ToLower(filepath.Base(fileName))

	unmatched := []string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		hash := sha256Pattern.FindString(line)
		if hash == "" {
			continue
		}

		normalizedHash := strings.ToLower(hash)
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, targetName) || strings.Contains(lowerLine, targetBase) {
			return normalizedHash
		}

		unmatched = append(unmatched, normalizedHash)
	}

	if len(unmatched) == 1 {
		return unmatched[0]
	}

	return ""
}
