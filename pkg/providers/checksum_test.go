package providers

import "testing"

func TestParseSHA256ChecksumMatchesFileName(t *testing.T) {
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  tool-darwin-arm64\n"
	expected := parseSHA256Checksum(content, "tool-darwin-arm64", "checksums.txt", nil)
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

func TestParseSHA256ChecksumTreatsStemMatchAsFinalBinaryHash(t *testing.T) {
	content := "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD\n"
	expected := parseSHA256Checksum(content, "tool.tar.gz", "tool.sha256sum", nil)
	if expected == nil || expected.Hash != "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("unexpected hash: %#v", expected)
	}
	if expected.Scope != checksumScopeFinal {
		t.Fatalf("unexpected scope: %v", expected.Scope)
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

func TestRankedChecksumAssetsPrefersMatchingStem(t *testing.T) {
	assets := []checksumAsset{
		{Name: "other.sha256sum", URL: "https://example.com/other.sha256sum"},
		{Name: "zellij-x86_64-unknown-linux-musl.sha256sum", URL: "https://example.com/zellij.sha256sum"},
	}

	ranked := rankedChecksumAssets("zellij-x86_64-unknown-linux-musl.tar.gz", assets)
	if len(ranked) != 2 {
		t.Fatalf("unexpected ranked asset count: %d", len(ranked))
	}
	if ranked[0].Name != "zellij-x86_64-unknown-linux-musl.sha256sum" {
		t.Fatalf("unexpected top-ranked asset: %s", ranked[0].Name)
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
