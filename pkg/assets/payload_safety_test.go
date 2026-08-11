package assets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeltaArtifactsNeverBecomeCandidates(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testLinuxAMDResolver

	for _, suffix := range []string{".bsdiff", ".bspatch", ".patch", ".diff", ".delta", ".zsync", ".bsdiff.gz", ".patch.xz", ".delta.bz2", ".zsync.zst"} {
		name := "deno-x86_64-unknown-linux-gnu.from-2.9.4" + suffix
		if !IsKnownNonRunnableName(name) {
			t.Fatalf("%q was not classified as non-runnable", name)
		}
		if _, err := NewFilter(&FilterOpts{}).FilterAssets("deno", []*Asset{{Name: name}}, ""); !errors.Is(err, ErrNoCompatibleFiles) {
			t.Fatalf("FilterAssets(%q) error = %v, want ErrNoCompatibleFiles", name, err)
		}
	}

	archive := &Asset{Name: "deno-x86_64-unknown-linux-gnu.zip"}
	delta := &Asset{Name: "deno-x86_64-unknown-linux-gnu.from-2.9.4.bsdiff"}
	got, err := NewFilter(&FilterOpts{}).FilterAssets("deno", []*Asset{archive, delta}, "")
	if err != nil {
		t.Fatalf("FilterAssets() error = %v", err)
	}
	if got.Name != archive.Name {
		t.Fatalf("selected %q, want archive %q", got.Name, archive.Name)
	}
	if got := NewFilter(&FilterOpts{SkipScoring: true}).CompatibleAssets([]*Asset{delta}, ""); len(got) != 0 {
		t.Fatalf("CompatibleAssets returned delta candidate: %+v", got)
	}
	if _, err := NewFilter(&FilterOpts{}).FilterAssets("deno", []*Asset{archive, delta}, delta.Name); err == nil {
		t.Fatal("expected explicitly selected delta to be rejected")
	}
}

func TestValidateRunnablePayload(t *testing.T) {
	dir := t.TempDir()
	write := func(name, contents string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if err := ValidateRunnablePayload(write("unusual.payload", "#!/bin/sh\nexit 0\n"), "unusual.payload"); err != nil {
		t.Fatalf("shebang script rejected: %v", err)
	}
	for _, tc := range []struct{ name, bytes string }{
		{"plain", "just executable text"},
		{"patch", "BSDIFF40 synthetic patch"},
		{"library.so", "#!/bin/sh\nexit 0\n"},
	} {
		err := ValidateRunnablePayload(write(tc.name, tc.bytes), tc.name)
		if !errors.Is(err, ErrNotRunnablePayload) {
			t.Fatalf("ValidateRunnablePayload(%q) = %v, want ErrNotRunnablePayload", tc.name, err)
		}
	}
	if !strings.Contains(ErrNotRunnablePayload.Error(), "runnable") {
		t.Fatal("sentinel should remain actionable")
	}
}
