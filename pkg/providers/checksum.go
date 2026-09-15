package providers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aaronflorey/bin/pkg/assets"
)

const (
	maxChecksumManifestBytes = 2 * 1024 * 1024
	maxChecksumLineBytes     = 64 * 1024
)

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var bsdChecksumPattern = regexp.MustCompile(`^([[:alnum:]-]+) \((.*)\) = ([[:xdigit:]]+)$`)

var checksumMetadataSuffixes = []string{
	".sigstore.json", ".intoto.jsonl", ".sbom.json", ".spdx.json", ".cyclonedx.json",
	".provenance.json", ".attestation.json", ".attest.json", ".sig", ".minisig", ".pem",
	".crt", ".cer", ".asc", ".blockmap",
}

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

// checksumOutcomeState records completed integrity checks only.
type checksumOutcomeState int

const (
	checksumNotSupplied checksumOutcomeState = iota
	checksumVerified
	checksumFailed
)

type checksumFailureReason string

const (
	checksumRetrievalFailure            checksumFailureReason = "retrieval"
	checksumParsingFailure              checksumFailureReason = "parsing"
	checksumUnsupportedAlgorithmFailure checksumFailureReason = "unsupported-algorithm"
	checksumMismatchFailure             checksumFailureReason = "mismatch"
)

type checksumOutcome struct {
	State  checksumOutcomeState
	Reason checksumFailureReason
	Err    error
}

// checksumIntegrityError preserves a failure reason across provider error
// boundaries while retaining the underlying transport, parsing, or asset error.
type checksumIntegrityError struct {
	State  checksumOutcomeState
	Reason checksumFailureReason
	Err    error
}

func (e *checksumIntegrityError) Error() string {
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *checksumIntegrityError) Unwrap() error { return e.Err }

// checksumBindingResult is an unverified digest bound to a specific download.
// It is intentionally separate from checksumOutcome: an expectation is not an
// integrity result until the bytes have been checked.
type checksumBindingResult struct {
	Expected *expectedChecksum
	Failure  *checksumOutcome
}

type checksumManifestRequest struct {
	Target        string
	Candidates    []string
	Asset         checksumAsset
	HashOrder     []string
	Authoritative bool
}

var checksumHTTPClient = &http.Client{Timeout: 30 * time.Second}

func checksumBindingForAsset(name string, assets []checksumAsset, headers map[string]string) checksumBindingResult {
	for _, candidate := range rankedChecksumAssets(name, assets) {
		applicable, unsupported := checksumAssetApplicability(candidate.Name, name)
		if unsupported {
			return failedBinding(checksumUnsupportedAlgorithmFailure, fmt.Errorf("checksum %q: %w", candidate.Name, errUnsupportedChecksumAlgorithm))
		}
		content, err := fetchChecksumFile(candidate.URL, headers)
		if err != nil {
			if applicable {
				return failedBinding(checksumRetrievalFailure, fmt.Errorf("checksum %q: %w", candidate.URL, err))
			}
			continue
		}
		request := checksumManifestRequest{Target: name, Candidates: checksumAssetNames(assets), Asset: candidate, Authoritative: applicable}
		hasHashOrder := len(matchingChecksumHashOrderAssets(candidate.Name, assets)) > 0
		mentionsTarget, err := manifestMentionsTarget(content, request, hasHashOrder)
		if err != nil {
			return failedBinding(checksumParsingFailure, err)
		}
		if !applicable && !mentionsTarget {
			continue
		}
		hashOrder, failure := checksumHashOrderForManifest(candidate, assets, headers)
		if failure != nil {
			return checksumBindingResult{Failure: failure}
		}
		request.HashOrder = hashOrder
		binding := parseChecksumManifest(content, request)
		if binding.Failure != nil {
			return binding
		}
		if binding.Expected != nil {
			return binding
		}
	}

	return checksumBindingResult{}
}

func checksumAssetNames(assets []checksumAsset) []string {
	names := make([]string, len(assets))
	for i, asset := range assets {
		names[i] = asset.Name
	}
	return names
}

// checksumAssetApplicability identifies authoritative exact sidecars. Generic
// manifests are only potential until their content names the selected target.
func checksumAssetApplicability(checksumName, target string) (applicable, unsupported bool) {
	if checksumName == target+".sha256" || checksumName == target+".sha256sum" {
		return true, false
	}
	if strings.HasPrefix(checksumName, target+".") && isChecksumNamedAsset(strings.ToLower(checksumName)) {
		return false, true
	}
	if hasChecksumSidecarSuffix(strings.ToLower(checksumName)) && !isNamedSHA256Manifest(strings.ToLower(checksumName)) {
		return false, false
	}
	return false, false
}

func isNamedSHA256Manifest(name string) bool {
	return strings.Contains(name, "sha256sums") || strings.Contains(name, "checksums.sha256")
}

func hasChecksumSidecarSuffix(name string) bool {
	for _, suffix := range []string{".sha256sum", ".sha256", ".sha512sum", ".sha512", ".sha1sum", ".sha1", ".md5sum", ".md5"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func isExactSHA256Sidecar(checksumName, target string) bool {
	return checksumName == target+".sha256" || checksumName == target+".sha256sum"
}

func rankedChecksumAssets(name string, assets []checksumAsset) []checksumAsset {
	type scoredAsset struct {
		asset checksumAsset
		score int
	}
	scored := []scoredAsset{}
	for _, asset := range assets {
		lower := strings.ToLower(asset.Name)
		if strings.Contains(lower, "hashes_order") || isChecksumMetadataAsset(lower) || !isChecksumNamedAsset(lower) {
			continue
		}
		score := 1
		if asset.Name == name+".sha256" || asset.Name == name+".sha256sum" {
			score = 2
		}
		scored = append(scored, scoredAsset{asset, score})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	result := make([]checksumAsset, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.asset)
	}
	return result
}

func isChecksumMetadataAsset(name string) bool {
	for _, suffix := range checksumMetadataSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func isChecksumNamedAsset(name string) bool {
	if strings.Contains(name, "checksum") || strings.Contains(name, "sha256") {
		return true
	}
	for _, suffix := range []string{".sha512sum", ".sha512", ".sha1sum", ".sha1", ".md5sum", ".md5"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func checksumHashOrderForManifest(manifest checksumAsset, assets []checksumAsset, headers map[string]string) ([]string, *checksumOutcome) {
	candidates := matchingChecksumHashOrderAssets(manifest.Name, assets)
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) > 1 {
		return nil, checksumFailure(checksumParsingFailure, fmt.Errorf("multiple checksum order declarations for %q", manifest.Name))
	}
	candidate := candidates[0]
	content, err := fetchChecksumFile(candidate.URL, headers)
	if err != nil {
		return nil, checksumFailure(checksumRetrievalFailure, fmt.Errorf("checksum order %q: %w", candidate.URL, err))
	}
	order, err := parseChecksumHashOrderReader(strings.NewReader(content))
	if err != nil {
		return nil, checksumFailure(checksumParsingFailure, fmt.Errorf("checksum order %q: %w", candidate.Name, err))
	}
	return order, nil
}

func matchingChecksumHashOrderAssets(manifest string, assets []checksumAsset) []checksumAsset {
	var candidates []checksumAsset
	for _, asset := range assets {
		if checksumManifestStem(asset.Name) == checksumManifestStem(manifest)+"_hashes_order" {
			candidates = append(candidates, asset)
		}
	}
	return candidates
}

func checksumManifestStem(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[:index]
	}
	return name
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
	content, err := readChecksumManifest(resp.Body)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func parseSHA256Checksum(content, fileName, checksumFileName string, hashOrder []string) *expectedChecksum {
	binding := parseChecksumManifest(content, checksumManifestRequest{Target: fileName, Candidates: []string{fileName}, Asset: checksumAsset{Name: checksumFileName}, HashOrder: hashOrder})
	return binding.Expected
}

func parseChecksumManifest(content string, request checksumManifestRequest) checksumBindingResult {
	return parseChecksumManifestReader(strings.NewReader(content), request)
}

func parseChecksumManifestReader(reader io.Reader, request checksumManifestRequest) checksumBindingResult {
	content, err := readChecksumManifest(reader)
	if err != nil {
		return failedBinding(checksumParsingFailure, err)
	}
	if request.Target == "" {
		return failedBinding(checksumParsingFailure, errors.New("checksum target is required"))
	}
	if request.Asset.Name == request.Target+".sha512" || request.Asset.Name == request.Target+".sha512sum" {
		return failedBinding(checksumUnsupportedAlgorithmFailure, errUnsupportedChecksumAlgorithm)
	}

	if isExactSHA256Sidecar(request.Asset.Name, request.Target) {
		request.Authoritative = true
		return parseExactSHA256Sidecar(content, request)
	}
	return parseNamedChecksumManifest(content, request)
}

var errUnsupportedChecksumAlgorithm = errors.New("unsupported checksum algorithm")

func readChecksumManifest(reader io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxChecksumManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxChecksumManifestBytes {
		return nil, fmt.Errorf("checksum manifest exceeds %d bytes", maxChecksumManifestBytes)
	}
	return content, nil
}

func parseExactSHA256Sidecar(content []byte, request checksumManifestRequest) checksumBindingResult {
	value := strings.TrimSpace(string(content))
	if sha256Pattern.MatchString(value) {
		return checksumBindingResult{Expected: &expectedChecksum{Hash: strings.ToLower(value), Scope: checksumScopeArchive}}
	}
	return parseNamedChecksumManifest(content, request)
}

func parseNamedChecksumManifest(content []byte, request checksumManifestRequest) checksumBindingResult {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), maxChecksumLineBytes)
	var hashes []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		hash, matched, err := parseChecksumRecord(line, request)
		if errors.Is(err, errUnsupportedChecksumAlgorithm) {
			return failedBinding(checksumUnsupportedAlgorithmFailure, err)
		}
		if err != nil {
			return failedBinding(checksumParsingFailure, err)
		}
		if matched {
			hashes = append(hashes, hash)
		}
	}
	if err := scanner.Err(); err != nil {
		return failedBinding(checksumParsingFailure, err)
	}
	if len(hashes) == 0 {
		if request.Authoritative {
			return failedBinding(checksumParsingFailure, errors.New("checksum sidecar has no record for target"))
		}
		return checksumBindingResult{}
	}
	if len(hashes) != 1 {
		return failedBinding(checksumParsingFailure, errors.New("duplicate checksum records for target"))
	}
	return checksumBindingResult{Expected: &expectedChecksum{Hash: hashes[0], Scope: checksumScopeArchive}}
}

func manifestMentionsTarget(content string, request checksumManifestRequest, hasHashOrder bool) (bool, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 4096), maxChecksumLineBytes)
	for scanner.Scan() {
		line := scanner.Text()
		if match := bsdChecksumPattern.FindStringSubmatch(line); match != nil {
			if recordCouldBind(match[2], request) || ambiguousBasename(match[2], request) {
				return true, nil
			}
			continue
		}
		if separator := strings.Index(line, "  "); separator >= 0 {
			name := line[separator+2:]
			if recordCouldBind(name, request) || ambiguousBasename(name, request) {
				return true, nil
			}
			continue
		}
		if separator := strings.Index(line, " *"); separator >= 0 {
			name := line[separator+2:]
			if recordCouldBind(name, request) || ambiguousBasename(name, request) {
				return true, nil
			}
			continue
		}
		fields := strings.Fields(line)
		if hasHashOrder && len(fields) > 0 && (recordCouldBind(fields[0], request) || ambiguousBasename(fields[0], request)) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func parseChecksumRecord(line string, request checksumManifestRequest) (string, bool, error) {
	if request.HashOrder != nil {
		return parseOrderedChecksumRecord(line, request)
	}
	if match := bsdChecksumPattern.FindStringSubmatch(line); match != nil {
		if ambiguousBasename(match[2], request) {
			return "", false, errors.New("ambiguous checksum basename")
		}
		if !recordCouldBind(match[2], request) {
			return "", false, nil
		}
		if normalizeHashAlgorithm(match[1]) != "sha256" {
			return "", false, errUnsupportedChecksumAlgorithm
		}
		return bindChecksumRecord(match[2], match[3], request)
	}
	return parseGNUChecksumRecord(line, request)
}

func parseGNUChecksumRecord(line string, request checksumManifestRequest) (string, bool, error) {
	separator := strings.Index(line, "  ")
	if separator < 0 {
		separator = strings.Index(line, " *")
	}
	if separator < 0 {
		if !request.Authoritative {
			return "", false, nil
		}
		return "", false, errors.New("unsupported checksum record format")
	}
	hash, name := line[:separator], line[separator+2:]
	if ambiguousBasename(name, request) {
		return "", false, errors.New("ambiguous checksum basename")
	}
	if !recordCouldBind(name, request) {
		return "", false, nil
	}
	if !isExactSHA256Sidecar(request.Asset.Name, request.Target) && !isNamedSHA256Manifest(strings.ToLower(request.Asset.Name)) {
		return "", false, errUnsupportedChecksumAlgorithm
	}
	if !sha256Pattern.MatchString(hash) {
		return "", false, errors.New("invalid SHA-256 record for target")
	}
	return bindChecksumRecord(name, hash, request)
}

func parseOrderedChecksumRecord(line string, request checksumManifestRequest) (string, bool, error) {
	fields := strings.Fields(line)
	if len(fields) != len(request.HashOrder)+1 {
		if len(fields) > 0 && (recordCouldBind(fields[0], request) || ambiguousBasename(fields[0], request)) {
			return "", false, errors.New("invalid checksum record field count for target")
		}
		return "", false, nil
	}
	sha256Index := -1
	for index, algorithm := range request.HashOrder {
		if normalizeHashAlgorithm(algorithm) == "sha256" {
			sha256Index = index + 1
			break
		}
	}
	if ambiguousBasename(fields[0], request) {
		return "", false, errors.New("ambiguous checksum basename")
	}
	if !recordCouldBind(fields[0], request) {
		return "", false, nil
	}
	if sha256Index < 0 {
		return "", false, errUnsupportedChecksumAlgorithm
	}
	if !sha256Pattern.MatchString(fields[sha256Index]) {
		return "", false, errors.New("invalid SHA-256 record for target")
	}
	return bindChecksumRecord(fields[0], fields[sha256Index], request)
}

func bindChecksumRecord(recordName, hash string, request checksumManifestRequest) (string, bool, error) {
	if recordName == request.Target {
		if !sha256Pattern.MatchString(hash) {
			return "", false, errors.New("invalid SHA-256 record for target")
		}
		return strings.ToLower(hash), true, nil
	}
	if ambiguousBasename(recordName, request) {
		return "", false, errors.New("ambiguous checksum basename")
	}
	if !unambiguousBasename(recordName, request.Target, request.Candidates) {
		return "", false, nil
	}
	if !sha256Pattern.MatchString(hash) {
		return "", false, errors.New("invalid SHA-256 record for target")
	}
	return strings.ToLower(hash), true, nil
}

func ambiguousBasename(recordName string, request checksumManifestRequest) bool {
	return basename(recordName) == basename(request.Target) && basenameMatchCount(recordName, request.Candidates) > 1
}

func recordCouldBind(recordName string, request checksumManifestRequest) bool {
	return recordName == request.Target || unambiguousBasename(recordName, request.Target, request.Candidates)
}

func unambiguousBasename(recordName, target string, candidates []string) bool {
	base := basename(recordName)
	if base != basename(target) {
		return false
	}
	if len(candidates) == 0 {
		candidates = []string{target}
	}
	return basenameMatchCount(recordName, candidates) == 1
}

func basenameMatchCount(name string, candidates []string) int {
	base := basename(name)
	matches := 0
	for _, candidate := range candidates {
		if basename(candidate) == base {
			matches++
		}
	}
	return matches
}

func basename(name string) string {
	if index := strings.LastIndexAny(name, "/\\"); index >= 0 {
		return name[index+1:]
	}
	return name
}

func failedChecksum(reason checksumFailureReason, err error) checksumOutcome {
	integrityErr := &checksumIntegrityError{State: checksumFailed, Reason: reason, Err: err}
	return checksumOutcome{State: checksumFailed, Reason: reason, Err: integrityErr}
}

func failedBinding(reason checksumFailureReason, err error) checksumBindingResult {
	return checksumBindingResult{Failure: checksumFailure(reason, err)}
}

func checksumFailure(reason checksumFailureReason, err error) *checksumOutcome {
	outcome := failedChecksum(reason, err)
	return &outcome
}

func verifyChecksum(expected, actual string) checksumOutcome {
	if expected == actual {
		return checksumOutcome{State: checksumVerified}
	}
	return failedChecksum(checksumMismatchFailure, errors.New("checksum mismatch"))
}

func checksumVerificationError(err error) error {
	if errors.Is(err, assets.ErrChecksumMismatch) {
		return failedChecksum(checksumMismatchFailure, err).Err
	}
	return err
}

func parseChecksumHashOrder(content string) []string {
	order, err := parseChecksumHashOrderReader(strings.NewReader(content))
	if err != nil {
		return nil
	}
	return order
}

func parseChecksumHashOrderReader(reader io.Reader) ([]string, error) {
	content, err := readChecksumManifest(reader)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), maxChecksumLineBytes)
	var order []string
	seen := map[string]struct{}{}
	for scanner.Scan() {
		if value := strings.TrimSpace(scanner.Text()); value != "" {
			algorithm := normalizeHashAlgorithm(value)
			if _, exists := seen[algorithm]; exists {
				return nil, fmt.Errorf("duplicate checksum algorithm %q", algorithm)
			}
			seen[algorithm] = struct{}{}
			order = append(order, algorithm)
		}
	}
	return order, scanner.Err()
}

func normalizeHashAlgorithm(value string) string {
	var builder strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			builder.WriteRune(ch)
		}
	}
	return strings.ToLower(builder.String())
}
