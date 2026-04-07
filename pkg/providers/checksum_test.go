package providers

import "testing"

func TestParseSHA256ChecksumMatchesFileName(t *testing.T) {
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  tool-darwin-arm64\n"
	hash := parseSHA256Checksum(content, "tool-darwin-arm64", nil)
	if hash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected hash: %q", hash)
	}
}

func TestParseSHA256ChecksumSingleHashFallback(t *testing.T) {
	content := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB\n"
	hash := parseSHA256Checksum(content, "tool", nil)
	if hash != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected hash: %q", hash)
	}
}

func TestParseSHA256ChecksumUsesHashOrder(t *testing.T) {
	content := "tool deadbeef 2222222222222222222222222222222222222222222222222222222222222222\n"
	hashOrder := []string{"crc32", "sha256"}
	hash := parseSHA256Checksum(content, "tool", hashOrder)
	if hash != "2222222222222222222222222222222222222222222222222222222222222222" {
		t.Fatalf("unexpected hash: %q", hash)
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
