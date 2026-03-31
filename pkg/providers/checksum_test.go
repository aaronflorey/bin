package providers

import "testing"

func TestParseSHA256ChecksumMatchesFileName(t *testing.T) {
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  tool-darwin-arm64\n"
	hash := parseSHA256Checksum(content, "tool-darwin-arm64")
	if hash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected hash: %q", hash)
	}
}

func TestParseSHA256ChecksumSingleHashFallback(t *testing.T) {
	content := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB\n"
	hash := parseSHA256Checksum(content, "tool")
	if hash != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected hash: %q", hash)
	}
}

func TestRankedChecksumAssets(t *testing.T) {
	assets := []checksumAsset{
		{Name: "checksums.txt", URL: "https://example.com/checksums.txt"},
		{Name: "tool.sha256", URL: "https://example.com/tool.sha256"},
		{Name: "tool.sha256sum", URL: "https://example.com/tool.sha256sum"},
	}

	ranked := rankedChecksumAssets("tool", assets)
	if len(ranked) != 3 {
		t.Fatalf("unexpected ranked asset count: %d", len(ranked))
	}
	if ranked[0].Name != "tool.sha256" && ranked[0].Name != "tool.sha256sum" {
		t.Fatalf("unexpected top-ranked asset: %s", ranked[0].Name)
	}
}
