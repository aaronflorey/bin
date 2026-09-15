package providers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/assets"
)

func TestParseSHA256ChecksumMatchesFileName(t *testing.T) {
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  tool-darwin-arm64\n"
	expected := parseSHA256Checksum(content, "tool-darwin-arm64", "checksums.sha256", nil)
	if expected == nil || expected.Hash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected hash: %#v", expected)
	}
	if expected.Scope != checksumScopeArchive {
		t.Fatalf("unexpected scope: %v", expected.Scope)
	}
}

func TestParseSHA256ChecksumSingleHashFallback(t *testing.T) {
	content := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB\n"
	expected := parseSHA256Checksum(content, "tool", "tool.sha256sum", nil)
	if expected == nil || expected.Hash != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected hash: %#v", expected)
	}
	if expected.Scope != checksumScopeArchive {
		t.Fatalf("unexpected scope: %v", expected.Scope)
	}
}

func TestParseSHA256ChecksumUsesHashOrder(t *testing.T) {
	content := "tool deadbeef 2222222222222222222222222222222222222222222222222222222222222222\n"
	hashOrder := []string{"crc32", "sha256"}
	expected := parseSHA256Checksum(content, "tool", "checksums.txt", hashOrder)
	if expected == nil || expected.Hash != "2222222222222222222222222222222222222222222222222222222222222222" {
		t.Fatalf("unexpected hash: %#v", expected)
	}
	if expected.Scope != checksumScopeArchive {
		t.Fatalf("unexpected scope: %v", expected.Scope)
	}
}

func TestParseSHA256ChecksumMatchesExactFileName(t *testing.T) {
	content := "tool.tar.gz aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
		"tool bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n"
	expected := parseSHA256Checksum(content, "tool", "checksums.txt", []string{"sha256"})
	if expected == nil || expected.Hash != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected hash: %#v", expected)
	}
}

func TestParseSHA256ChecksumIgnoresUnrelatedSingleHashFile(t *testing.T) {
	content := "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC\n"
	expected := parseSHA256Checksum(content, "tool.tar.gz", "other-tool.sha256sum", nil)
	if expected != nil {
		t.Fatalf("unexpected hash: %#v", expected)
	}
}

func TestParseSHA256ChecksumDoesNotTreatStemMatchAsFinalBinaryHash(t *testing.T) {
	content := "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD\n"
	expected := parseSHA256Checksum(content, "tool.tar.gz", "tool.sha256sum", nil)
	if expected != nil {
		t.Fatalf("unexpected hash: %#v", expected)
	}
}

func TestRankedChecksumAssets(t *testing.T) {
	assets := []checksumAsset{
		{Name: "checksums.txt", URL: "https://example.com/checksums.txt"},
		{Name: "tool.sha256", URL: "https://example.com/tool.sha256"},
		{Name: "tool.sha256sum", URL: "https://example.com/tool.sha256sum"},
		{Name: "checksums_hashes_order", URL: "https://example.com/checksums_hashes_order"},
	}

	ranked := rankedChecksumAssets("tool", assets)
	if len(ranked) != 3 {
		t.Fatalf("unexpected ranked asset count: %d", len(ranked))
	}
	if ranked[0].Name != "tool.sha256" && ranked[0].Name != "tool.sha256sum" {
		t.Fatalf("unexpected top-ranked asset: %s", ranked[0].Name)
	}
}

func TestRankedChecksumAssetsSkipsMetadataSidecars(t *testing.T) {
	assets := []checksumAsset{
		{Name: "trivy_0.70.0_checksums.txt.sigstore.json", URL: "https://example.com/checksums.sigstore.json"},
		{Name: "trivy_0.70.0_checksums.txt", URL: "https://example.com/checksums.txt"},
		{Name: "trivy_0.70.0_Linux-64bit.rpm", URL: "https://example.com/trivy.rpm"},
	}

	ranked := rankedChecksumAssets("trivy_0.70.0_Linux-64bit.tar.gz", assets)
	if len(ranked) != 1 {
		t.Fatalf("unexpected ranked asset count: %d", len(ranked))
	}
	if ranked[0].Name != "trivy_0.70.0_checksums.txt" {
		t.Fatalf("unexpected ranked asset: %s", ranked[0].Name)
	}
}

func TestRankedChecksumAssetsDoesNotAssociateArchiveStem(t *testing.T) {
	assets := []checksumAsset{
		{Name: "other.sha256sum", URL: "https://example.com/other.sha256sum"},
		{Name: "zellij-x86_64-unknown-linux-musl.sha256sum", URL: "https://example.com/zellij.sha256sum"},
	}

	ranked := rankedChecksumAssets("zellij-x86_64-unknown-linux-musl.tar.gz", assets)
	if len(ranked) != 2 {
		t.Fatalf("unexpected ranked asset count: %d", len(ranked))
	}
	if ranked[0].Name != "other.sha256sum" {
		t.Fatalf("archive stem changed ranking: %s", ranked[0].Name)
	}
}

func TestParseChecksumHashOrder(t *testing.T) {
	content := "CRC32\nSHA1\nSHA-256\n"
	order := parseChecksumHashOrder(content)
	if len(order) != 3 {
		t.Fatalf("unexpected order length: %d", len(order))
	}
	if order[2] != "sha256" {
		t.Fatalf("unexpected normalized value: %q", order[2])
	}
}

func TestParseChecksumHashOrderRejectsDuplicateAlgorithms(t *testing.T) {
	if _, err := parseChecksumHashOrderReader(strings.NewReader("SHA256\nsha-256\n")); err == nil {
		t.Fatal("expected duplicate algorithm error")
	}
}

func TestParseChecksumManifestSupportsGNUAndBSDExactPaths(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, test := range []struct{ name, content string }{
		{"checksums.sha256", hash + "  dist/Tool.tar.gz\n"},
		{"checksums.txt", "SHA256 (dist/Tool.tar.gz) = " + hash + "\n"},
	} {
		outcome := parseChecksumManifest(test.content, checksumManifestRequest{Target: "dist/Tool.tar.gz", Asset: checksumAsset{Name: test.name}})
		if outcome.Expected == nil || outcome.Expected.Hash != hash {
			t.Fatalf("unexpected outcome: %#v", outcome)
		}
	}
}

func TestParseChecksumManifestDoesNotFoldPathOrFilenameCase(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, target := range []string{"dist/tool.tar.gz"} {
		outcome := parseChecksumManifest(hash+"  dist/Tool.tar.gz\n", checksumManifestRequest{Target: target, Asset: checksumAsset{Name: "checksums.sha256"}})
		if outcome.Expected != nil || outcome.Failure != nil {
			t.Fatalf("unexpected outcome for %q: %#v", target, outcome)
		}
	}
}

func TestParseChecksumManifestSupportsDeclaredHashOrder(t *testing.T) {
	hash := strings.Repeat("b", 64)
	outcome := parseChecksumManifest("tool deadbeef "+hash+"\n", checksumManifestRequest{
		Target: "tool", Asset: checksumAsset{Name: "checksums.txt"}, HashOrder: []string{"crc32", "sha256"},
	})
	if outcome.Expected == nil || outcome.Expected.Hash != hash {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestParseChecksumManifestBareDigestRequiresExactSidecar(t *testing.T) {
	hash := strings.Repeat("c", 64)
	for _, checksumName := range []string{"other.sha256", "tool.sha256", "tool.tar.gz.sha256"} {
		outcome := parseChecksumManifest(hash, checksumManifestRequest{Target: "tool.tar.gz", Asset: checksumAsset{Name: checksumName}})
		want := checksumName == "tool.tar.gz.sha256"
		if (outcome.Expected != nil) != want || (outcome.Expected != nil && outcome.Expected.Scope != checksumScopeArchive) {
			t.Fatalf("unexpected outcome for %q: %#v", checksumName, outcome)
		}
	}
}

func TestParseChecksumManifestExactSidecarAcceptsGNUTargetRecord(t *testing.T) {
	hash := strings.Repeat("c", 64)
	outcome := parseChecksumManifest(hash+"  tool.tar.gz\n", checksumManifestRequest{
		Target: "tool.tar.gz", Asset: checksumAsset{Name: "tool.tar.gz.sha256"},
	})
	if outcome.Failure != nil || outcome.Expected == nil || outcome.Expected.Hash != hash {
		t.Fatalf("unexpected binding: %#v", outcome)
	}
}

func TestParseChecksumManifestRequiresExplicitGNUAlgorithm(t *testing.T) {
	hash := strings.Repeat("c", 64)
	outcome := parseChecksumManifest(hash+"  tool\n", checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "checksums.txt"}})
	if outcome.Failure == nil || outcome.Failure.Reason != checksumUnsupportedAlgorithmFailure {
		t.Fatalf("unexpected binding: %#v", outcome)
	}
}

func TestParseChecksumManifestRejectsDuplicateAndMalformedTargetRecords(t *testing.T) {
	hash := strings.Repeat("d", 64)
	for _, content := range []string{hash + "  tool\n" + hash + "  tool\n", hash + "  tool\n" + strings.Repeat("e", 64) + "  tool\n", "not-a-hash  tool\n"} {
		outcome := parseChecksumManifest(content, checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "checksums.sha256"}})
		if outcome.Failure == nil || outcome.Failure.Reason != checksumParsingFailure {
			t.Fatalf("unexpected outcome: %#v", outcome)
		}
	}
}

func TestParseChecksumManifestAllowsUnambiguousBasename(t *testing.T) {
	hash := strings.Repeat("e", 64)
	outcome := parseChecksumManifest(hash+"  release/tool\n", checksumManifestRequest{
		Target: "linux/tool", Candidates: []string{"linux/tool", "darwin/other"}, Asset: checksumAsset{Name: "checksums.sha256"},
	})
	if outcome.Expected == nil || outcome.Expected.Hash != hash {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestParseChecksumManifestRejectsAmbiguousBasename(t *testing.T) {
	hash := strings.Repeat("e", 64)
	outcome := parseChecksumManifest(hash+"  tool\n", checksumManifestRequest{
		Target: "linux/tool", Candidates: []string{"linux/tool", "darwin/tool"}, Asset: checksumAsset{Name: "checksums.sha256"},
	})
	if outcome.Failure == nil || outcome.Failure.Reason != checksumParsingFailure {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestParseChecksumManifestRejectsMalformedApplicableHashOrderRows(t *testing.T) {
	for _, target := range []string{"tool", "linux/tool"} {
		outcome := parseChecksumManifest(target+" deadbeef\n", checksumManifestRequest{
			Target: target, Candidates: []string{target}, Asset: checksumAsset{Name: "checksums.txt"}, HashOrder: []string{"crc32", "sha256"},
		})
		if outcome.Failure == nil || outcome.Failure.Reason != checksumParsingFailure {
			t.Fatalf("unexpected binding for %q: %#v", target, outcome)
		}
	}
}

func TestChecksumHashOrderBindsToMatchingManifestAndRejectsMultipleDeclarations(t *testing.T) {
	hash := strings.Repeat("e", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			_, _ = w.Write([]byte("tool deadbeef " + hash + "\n"))
		case "/order":
			_, _ = w.Write([]byte("crc32\nsha256\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	binding := checksumBindingForAsset("tool", []checksumAsset{
		{Name: "checksums.txt", URL: server.URL + "/manifest"},
		{Name: "checksums_hashes_order", URL: server.URL + "/order"},
		{Name: "other_hashes_order", URL: "://unrelated"},
	}, nil)
	if binding.Failure != nil || binding.Expected == nil || binding.Expected.Hash != hash {
		t.Fatalf("unexpected binding: %#v", binding)
	}

	binding = checksumBindingForAsset("tool", []checksumAsset{
		{Name: "checksums.txt", URL: server.URL + "/manifest"},
		{Name: "checksums_hashes_order", URL: server.URL + "/order"},
		{Name: "checksums_hashes_order.backup", URL: server.URL + "/order"},
	}, nil)
	if binding.Failure == nil || binding.Failure.Reason != checksumParsingFailure {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestChecksumHashOrderAssociationIsCaseSensitive(t *testing.T) {
	hash := strings.Repeat("e", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			_, _ = w.Write([]byte("SHA256 (tool) = " + hash + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	binding := checksumBindingForAsset("tool", []checksumAsset{
		{Name: "Checksums.txt", URL: server.URL + "/manifest"},
		{Name: "checksums_hashes_order", URL: "://must-not-be-fetched"},
	}, nil)
	if binding.Failure != nil || binding.Expected == nil || binding.Expected.Hash != hash {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestChecksumBindingSkipsUnrelatedGenericManifestsAndContinues(t *testing.T) {
	hash := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/unavailable":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/unrelated":
			_, _ = w.Write([]byte("# release notes\nnot a checksum\nSHA512 (other) = deadbeef\n"))
		case "/valid":
			_, _ = w.Write([]byte(hash + "  tool\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	binding := checksumBindingForAsset("tool", []checksumAsset{
		{Name: "checksums.txt", URL: server.URL + "/unavailable"},
		{Name: "release-checksums.txt", URL: server.URL + "/unrelated"},
		{Name: "checksums.sha256", URL: server.URL + "/valid"},
	}, nil)
	if binding.Failure != nil || binding.Expected == nil || binding.Expected.Hash != hash {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestChecksumBindingIgnoresUnrelatedMixedRecordsBeforeTargetBSDRecord(t *testing.T) {
	hash := strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Join([]string{
			strings.Repeat("a", 64) + "  other",
			"release notes are not checksums",
			"SHA512 (another) = deadbeef",
			"SHA256 (tool) = " + hash,
		}, "\n")))
	}))
	defer server.Close()
	binding := checksumBindingForAsset("tool", []checksumAsset{{Name: "checksums.txt", URL: server.URL}}, nil)
	if binding.Failure != nil || binding.Expected == nil || binding.Expected.Hash != hash {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestChecksumBindingReportsGenericManifestScanFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxChecksumLineBytes+1)))
	}))
	defer server.Close()
	binding := checksumBindingForAsset("tool", []checksumAsset{{Name: "checksums.txt", URL: server.URL}}, nil)
	if binding.Failure == nil || binding.Failure.Reason != checksumParsingFailure {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestParseChecksumManifestReportsUnsupportedAndOversizedInput(t *testing.T) {
	unsupported := parseChecksumManifest("", checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "tool.sha512"}})
	if unsupported.Failure == nil || unsupported.Failure.Reason != checksumUnsupportedAlgorithmFailure {
		t.Fatalf("unexpected unsupported outcome: %#v", unsupported)
	}
	overlong := parseChecksumManifestReader(strings.NewReader(strings.Repeat("x", maxChecksumLineBytes+1)), checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "checksums.txt"}})
	if overlong.Failure == nil || overlong.Failure.Reason != checksumParsingFailure {
		t.Fatalf("unexpected oversized outcome: %#v", overlong)
	}
	bsdUnsupported := parseChecksumManifest("SHA512 (tool) = deadbeef\n", checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "checksums.txt"}})
	if bsdUnsupported.Failure == nil || bsdUnsupported.Failure.Reason != checksumUnsupportedAlgorithmFailure {
		t.Fatalf("unexpected BSD outcome: %#v", bsdUnsupported)
	}
}

func TestChecksumOutcomesKeepExpectationSeparateFromVerification(t *testing.T) {
	hash := strings.Repeat("f", 64)
	expected := parseChecksumManifest(hash, checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "tool.sha256"}})
	if expected.Failure != nil || expected.Expected == nil {
		t.Fatalf("expectation was treated as verified: %#v", expected)
	}
	verified := verifyChecksum(hash, hash)
	if verified.State != checksumVerified {
		t.Fatalf("unexpected verified outcome: %#v", verified)
	}
	mismatch := verifyChecksum(hash, strings.Repeat("0", 64))
	if mismatch.State != checksumFailed || mismatch.Reason != checksumMismatchFailure {
		t.Fatalf("unexpected mismatch outcome: %#v", mismatch)
	}
}

func TestChecksumVerificationErrorMapsAssetMismatch(t *testing.T) {
	err := checksumVerificationError(fmt.Errorf("wrapped: %w", assets.ErrChecksumMismatch))
	var integrityErr *checksumIntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.State != checksumFailed || integrityErr.Reason != checksumMismatchFailure {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseChecksumManifestReportsReaderErrors(t *testing.T) {
	outcome := parseChecksumManifestReader(errorReader{}, checksumManifestRequest{Target: "tool", Asset: checksumAsset{Name: "checksums.txt"}})
	if outcome.Failure == nil || outcome.Failure.Reason != checksumParsingFailure || !errors.Is(outcome.Failure.Err, errReader) {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestChecksumBindingForAssetReportsRetrievalFailure(t *testing.T) {
	outcome := checksumBindingForAsset("tool", []checksumAsset{{Name: "tool.sha256", URL: "://bad-url"}}, nil)
	if outcome.Failure == nil || outcome.Failure.Reason != checksumRetrievalFailure {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestChecksumBindingForAssetIgnoresUnrelatedSidecars(t *testing.T) {
	outcome := checksumBindingForAsset("tool.zip", []checksumAsset{{Name: "other.zip.sha256", URL: "://bad-url"}}, nil)
	if outcome.Expected != nil || outcome.Failure != nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

var errReader = errors.New("read failure")

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errReader }
