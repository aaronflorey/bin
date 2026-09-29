package assets

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
)

func TestAuditForeignArchitectureAndSidecars(t *testing.T) {
	original := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = original })
	for _, foreign := range []string{"i686", "i586", "powerpc64", "powerpc64le", "riscv64gc", "mips64el", "loongarch64"} {
		t.Run(foreign, func(t *testing.T) {
			native := "dua-v2.45.0-x86_64-unknown-linux-musl.tar.gz"
			other := "dua-v2.45.0-" + foreign + "-unknown-linux-gnu.tar.gz"
			f := NewFilter(&FilterOpts{NonInteractive: true})
			selected, err := f.FilterAssets("dua", []*Asset{{Name: other}, {Name: native}}, "")
			if err != nil || selected.Name != native {
				t.Fatalf("selection = %#v, %v", selected, err)
			}
			if _, err := f.FilterAssets("dua", []*Asset{{Name: other}}, other); !errors.Is(err, ErrIncompatibleReleaseTarget) {
				t.Fatalf("explicit foreign selection = %v", err)
			}
		})
	}
	for _, suffix := range []string{".proof", ".gpgsig", ".b3", ".sbom", ".proof.gz"} {
		t.Run(suffix, func(t *testing.T) {
			native := "age-v1.2.1-linux-amd64.tar.gz"
			sidecar := native + suffix
			f := NewFilter(&FilterOpts{NonInteractive: true})
			selected, err := f.FilterAssets("age", []*Asset{{Name: sidecar}, {Name: native}}, "")
			if err != nil || selected.Name != native {
				t.Fatalf("selection = %#v, %v", selected, err)
			}
			if _, err := f.FilterAssets("age", []*Asset{{Name: sidecar}}, sidecar); err == nil {
				t.Fatal("explicit sidecar accepted")
			}
		})
	}
	f := NewFilter(&FilterOpts{NonInteractive: true})
	selected, err := f.FilterAssets("neovim", []*Asset{{Name: "nvim.appimage"}, {Name: "nvim-linux64.tar.gz"}}, "")
	if err != nil || selected.Name != "nvim-linux64.tar.gz" {
		t.Fatalf("linux64 selection = %#v, %v", selected, err)
	}
}

func TestAuditVersionedMemberUpdates(t *testing.T) {
	for _, tc := range []struct{ product, old, next string }{
		{"ripgrep", "ripgrep-14.1.1-x86_64-unknown-linux-musl/rg", "ripgrep-15.1.0-x86_64-unknown-linux-musl/rg"},
		{"watchexec", "watchexec-1.25.1-x86_64-unknown-linux-musl/watchexec", "watchexec-2.3.0-x86_64-unknown-linux-musl/watchexec"},
		{"helix", "helix-24.07-x86_64-linux/hx", "helix-25.07-x86_64-linux/hx"},
		{"prometheus", "prometheus-2.47.0.linux-amd64/prometheus", "prometheus-3.5.0.linux-amd64/prometheus"},
		{"dua", "dua-v2.44.0-x86_64-unknown-linux-musl/dua", "dua-v2.45.0-x86_64-unknown-linux-musl/dua"},
		{"nu", "nu-0.103.0-x86_64-unknown-linux-gnu/nu", "nu-0.104.0-x86_64-unknown-linux-gnu/nu"},
		{"nvim", "nvim-linux64/bin/nvim", "nvim-linux-x86_64/bin/nvim"},
	} {
		t.Run(tc.product, func(t *testing.T) {
			f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{LogicalProduct: tc.product, ArchiveMember: tc.old}})
			entry, err := f.resolveReleaseArchiveMember(resolverInventory(t, tc.next))
			if err != nil || entry.identity != tc.next {
				t.Fatalf("update %q -> %q = %#v, %v", tc.old, tc.next, entry, err)
			}
			_, err = f.resolveReleaseArchiveMember(resolverInventory(t, strings.ReplaceAll(tc.next, "/", "/helpers/")))
			if !errors.Is(err, ErrUnavailablePersistedSelection) {
				t.Fatalf("internal layout changed without rejection: %v", err)
			}
		})
	}
	f := NewFilter(&FilterOpts{SelectionIntent: &config.SelectionDescriptor{LogicalProduct: "ripgrep", ArchiveMember: "rg"}})
	_, err := f.resolveReleaseArchiveMember(resolverInventory(t, "ripgrep-v2-linux-amd64/rg", "rg-v2-linux-amd64/rg"))
	if !errors.Is(err, ErrAmbiguousArchiveMember) {
		t.Fatalf("colliding product and executable wrappers were not rejected: %v", err)
	}
}

func TestAuditSelectedBuildVariantSurvivesUpdate(t *testing.T) {
	original := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = original })
	for _, tc := range []struct{ repo, selected, old, next, other string }{
		{"opa", "opa_linux_amd64_static", "opa_linux_amd64_static", "opa_linux_amd64_static", "opa_linux_amd64"},
		{"opa", "opa_linux_amd64", "opa_linux_amd64", "opa_linux_amd64", "opa_linux_amd64_static"},
		{"bottom", "bottom_x86_64-unknown-linux-gnu-2-17.tar.gz", "bottom_x86_64-unknown-linux-gnu-2-17.tar.gz", "bottom_x86_64-unknown-linux-gnu-2-17.tar.gz", "bottom_x86_64-unknown-linux-gnu.tar.gz"},
		{"tool", "tool-v1.2.0-linux-amd64-static.tar.gz", "tool-v1.2.0-linux-amd64-static.tar.gz", "tool-v2.0.0-linux-amd64-static.tar.gz", "tool-v2.0.0-linux-amd64.tar.gz"},
	} {
		t.Run(tc.selected, func(t *testing.T) {
			f := NewFilter(&FilterOpts{NonInteractive: true})
			if _, err := f.FilterAssets(tc.repo, []*Asset{{Name: tc.old}}, tc.selected); err != nil {
				t.Fatal(err)
			}
			for _, intent := range []*config.SelectionDescriptor{f.SelectionIntent(), StoredSelectionDescriptor(&config.Binary{SourceAsset: tc.old, SelectionIntent: &config.SelectionDescriptor{LogicalProduct: tc.repo}})} {
				if intent.Variant == nil {
					t.Fatal("variant not recorded")
				}
				next := NewFilter(&FilterOpts{NonInteractive: true, SelectionIntent: intent})
				selected, err := next.FilterAssets(tc.repo, []*Asset{{Name: tc.other}, {Name: tc.next}}, "")
				if err != nil || selected.Name != tc.next {
					t.Fatalf("next selection = %#v, %v (intent %#v)", selected, err, intent)
				}
				if _, err := next.FilterAssets(tc.repo, []*Asset{{Name: tc.other}}, ""); !errors.Is(err, ErrUnavailablePersistedSelection) {
					t.Fatalf("missing variant = %v", err)
				}
			}
		})
	}
}

func TestAuditPersistedTargetUsesInstallCompatibility(t *testing.T) {
	original := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = original })
	intent := &config.SelectionDescriptor{LogicalProduct: "diskonaut", Target: &config.SelectionTarget{OS: "linux", Architecture: "amd64", ABI: "glibc"}}
	for _, name := range []string{"diskonaut-0.11.0-x86_64-unknown-linux-musl.tar.gz", "diskonaut-linux.tar.gz"} {
		selected, err := NewFilter(&FilterOpts{NonInteractive: true, SelectionIntent: intent}).FilterAssets("diskonaut", []*Asset{{Name: name}}, "")
		if err != nil || selected.Name != name {
			t.Fatalf("selection = %#v, %v", selected, err)
		}
	}
	for _, name := range []string{"diskonaut-linux-arm64.tar.gz", "other-linux-amd64.tar.gz"} {
		if _, err := NewFilter(&FilterOpts{NonInteractive: true, SelectionIntent: intent}).FilterAssets("diskonaut", []*Asset{{Name: name}}, ""); !errors.Is(err, ErrUnavailablePersistedSelection) {
			t.Fatalf("incompatible selection = %v", err)
		}
	}
}

func TestAuditUnknownArchiveProductListsEligibleMembers(t *testing.T) {
	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "nushell"
	_, err := f.resolveReleaseArchiveMember(resolverInventory(t, "bin/nu", "bin/nu_plugin_query"))
	var resolutionErr *ArchiveMemberResolutionError
	if !errors.As(err, &resolutionErr) || resolutionErr.Reason != ArchiveMemberAmbiguous || len(resolutionErr.Candidates) != 2 || !strings.Contains(err.Error(), "--select") {
		t.Fatalf("member selection = %v", err)
	}
}

func TestAuditCompressedNameAndMemberFailure(t *testing.T) {
	original := resolver
	resolver = testLinuxAMDResolver
	t.Cleanup(func() { resolver = original })
	var payload bytes.Buffer
	writer := gzip.NewWriter(&payload)
	if _, err := writer.Write([]byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	bzip, err := hex.DecodeString("425a6839314159265359b39191e3000002598000106800c00012610c4020003100000826684d36a0bd989f55c6e1218fc5dc914e14242ce46478c0")
	if err != nil {
		t.Fatal(err)
	}
	for suffix, compressed := range map[string][]byte{"gz": payload.Bytes(), "bz2": bzip} {
		f := NewFilter(&FilterOpts{NonInteractive: true})
		result, err := f.ProcessReader("restic_0.18.0_linux_amd64."+suffix, int64(len(compressed)), bytes.NewReader(compressed), "", false)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := closeReader(result.Source); err != nil {
				t.Error(err)
			}
		}()
		if got := SanitizeName(result.Name, "v0.18.0"); got != "restic" {
			t.Fatalf("installed name = %q", got)
		}
		if f.SelectionIntent().ArchiveMember != "" {
			t.Fatalf("compressed executable recorded as archive member: %#v", f.SelectionIntent())
		}
	}
	_, err = NewFilter(&FilterOpts{NonInteractive: true}).ProcessReader("tool", 4, strings.NewReader("junk"), "", false)
	if !errors.Is(err, ErrNoEligibleArchiveMember) {
		t.Fatalf("payload failure lost member reason: %v", err)
	}
}
