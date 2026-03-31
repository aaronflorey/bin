package assets

import (
	"archive/tar"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

type mockOSResolver struct {
	OS                   []string
	Arch                 []string
	LibC                 []string
	OSSpecificExtensions []string
}

func (m *mockOSResolver) GetOS() []string {
	return m.OS
}

func (m *mockOSResolver) GetArch() []string {
	return m.Arch
}

func (m *mockOSResolver) GetLibC() []string {
	return m.LibC
}

func (m *mockOSResolver) GetOSSpecificExtensions() []string {
	return m.OSSpecificExtensions
}

var (
	testLinuxAMDResolver   = &mockOSResolver{OS: []string{"linux"}, Arch: []string{"amd64", "x86_64", "x64", "64"}, LibC: []string{"glibc", "gnu"}, OSSpecificExtensions: []string{"AppImage"}}
	testLinuxMuslResolver  = &mockOSResolver{OS: []string{"linux"}, Arch: []string{"amd64", "x86_64", "x64", "64"}, LibC: []string{"musl"}, OSSpecificExtensions: []string{"AppImage"}}
	testWindowsAMDResolver = &mockOSResolver{OS: []string{"windows", "win"}, Arch: []string{"amd64", "x86_64", "x64", "64"}, OSSpecificExtensions: []string{"exe"}}
	testDarwinARMResolver  = &mockOSResolver{OS: []string{"darwin", "macos", "osx"}, Arch: []string{"arm64", "aarch64"}}
)

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in       string
		v        string
		out      string
		resolver platformResolver
	}{
		{"bin_amd64_linux", "v0.0.1", "bin", testLinuxAMDResolver},
		{"bin_0.0.1_amd64_linux", "0.0.1", "bin", testLinuxAMDResolver},
		{"bin_0.0.1_amd64_linux", "v0.0.1", "bin", testLinuxAMDResolver},
		{"tool-linux-amd64", "v13.2.1", "tool", testLinuxAMDResolver},
		{"tool-linux64", "tool-1.5", "tool", testLinuxAMDResolver},
		{"tool-linux-x64", "1.2.0-rc.1", "tool", testLinuxAMDResolver},
		{"tool-win-x64.exe", "1.2.0-rc.1", "tool.exe", testWindowsAMDResolver},
		{"bin_0.0.1_Windows_x86_64.exe", "0.0.1", "bin.exe", testWindowsAMDResolver},
		{"tool-1.1.3-aarch64-apple-darwin", "v1.1.3", "tool", testDarwinARMResolver},
	}

	for _, c := range cases {
		resolver = c.resolver
		if n := SanitizeName(c.in, c.v); n != c.out {
			t.Fatalf("Error replacing %s: %s does not match %s", c.in, n, c.out)
		}
	}

}

type args struct {
	repoName string
	as       []*Asset
}

func (a args) String() string {
	assetStrings := []string{}
	for _, asset := range a.as {
		assetStrings = append(assetStrings, asset.String())
	}
	return fmt.Sprintf("%s (%v)", a.repoName, strings.Join(assetStrings, ","))
}

func TestFilterAssets(t *testing.T) {
	cases := []struct {
		in       args
		out      string
		resolver platformResolver
	}{
		{args{"bin", []*Asset{
			{Name: "bin_0.0.1_Linux_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.0.1_Linux_x86_64"},
			{Name: "bin_0.0.1_Linux_i386", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.0.1_Linux_i386"},
			{Name: "bin_0.0.1_Darwin_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.0.1_Darwin_x86_64"},
		}}, "bin_0.0.1_Linux_x86_64", testLinuxAMDResolver},
		{args{"bin", []*Asset{
			{Name: "bin_0.1.0_Windows_i386.exe", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Windows_i386.exe"},
			{Name: "bin_0.1.0_Linux_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Linux_x86_64"},
			{Name: "bin_0.1.0_Darwin_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Darwin_x86_64"},
		}}, "bin_0.1.0_Linux_x86_64", testLinuxAMDResolver},
		{args{"bin", []*Asset{
			{Name: "bin_0.1.0_Windows_i386.exe", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Windows_i386.exe"},
			{Name: "bin_0.1.0_Linux_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Linux_x86_64"},
			{Name: "bin_0.1.0_Darwin_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Darwin_x86_64"},
		}}, "bin_0.1.0_Linux_x86_64", testLinuxAMDResolver},
		{args{"tool", []*Asset{
			{Name: "tool-windows-amd64", URL: "https://downloads.example.test/v13.2.1/binaries/tool-windows-amd64.zip"},
			{Name: "tool-linux-amd64", URL: "https://downloads.example.test/v13.2.1/binaries/tool-linux-amd64"},
			{Name: "tool-darwin-amd64", URL: "https://downloads.example.test/v13.2.1/binaries/tool-darwin-amd64"},
		}}, "tool-linux-amd64", testLinuxAMDResolver},
		{args{"tool", []*Asset{
			{Name: "tool_freebsd_amd64", URL: "https://example.test/acme/tool/releases/download/3.3.2/tool_freebsd_amd64"},
			{Name: "tool_linux_amd64", URL: "https://example.test/acme/tool/releases/download/3.3.2/tool_linux_amd64"},
			{Name: "tool_windows_amd64.exe", URL: "https://example.test/acme/tool/releases/download/3.3.2/tool_windows_amd64.exe"},
		}}, "tool_linux_amd64", testLinuxAMDResolver},
		{args{"tool", []*Asset{
			{Name: "tool-win64.exe", URL: "https://example.test/acme/tool/releases/download/tool-1.6/tool-win64.exe"},
			{Name: "tool-linux64", URL: "https://example.test/acme/tool/releases/download/tool-1.6/tool-linux64"},
			{Name: "tool-osx-amd64", URL: "https://example.test/acme/tool/releases/download/tool-1.6/tool-osx-amd64"},
		}}, "tool-linux64", testLinuxAMDResolver},
		{args{"bin", []*Asset{
			{Name: "bin_0.0.1_Windows_x86_64.exe", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.0.1_Windows_x86_64.exe"},
			{Name: "bin_0.1.0_Linux_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Linux_x86_64"},
			{Name: "bin_0.1.0_Darwin_x86_64", URL: "https://example.test/acme/bin/releases/download/v0.0.1/bin_0.1.0_Darwin_x86_64"},
		}}, "bin_0.0.1_Windows_x86_64.exe", testWindowsAMDResolver},
		{args{"toolset", []*Asset{
			{Name: "x86_64-linux-toolset-binaries.tar.gz", URL: "https://packages.example.test/api/v4/projects/123/packages/generic/toolset/8.2.0/x86_64-linux-toolset-binaries.tar.gz"},
		}}, "x86_64-linux-toolset-binaries.tar.gz", testLinuxAMDResolver},
		{args{"tool", []*Asset{
			{Name: "tool-linux-x64", URL: "https://example.test/acme/tool/releases/download/1.2.0-rc.1/tool-linux-x64"},
			{Name: "tool-win-x64.exe", URL: "https://example.test/acme/tool/releases/download/1.2.0-rc.1/tool-win-x64.exe"},
		}}, "tool-linux-x64", testLinuxAMDResolver},
		{args{"tool", []*Asset{
			{Name: "tool-linux-x64", URL: "https://example.test/acme/tool/releases/download/1.2.0-rc.1/tool-linux-x64"},
			{Name: "tool-win-x64.exe", URL: "https://example.test/acme/tool/releases/download/1.2.0-rc.1/tool-win-x64.exe"},
		}}, "tool-win-x64.exe", testWindowsAMDResolver},
		{args{"suite", []*Asset{
			{Name: "suite-4.7.1-Darwin.dmg", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1-Darwin.dmg"},
			{Name: "suite-4.7.1-win64.exe", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1-win64.exe"},
			{Name: "suite-4.7.1-win64.msi", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1-win64.msi"},
			{Name: "suite-4.7.1.AppImage", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1.AppImage"},
			{Name: "suite-4.7.1.AppImage.asc", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1.AppImage.asc"},
		}}, "suite-4.7.1.AppImage", testLinuxAMDResolver},
		{args{"suite", []*Asset{
			{Name: "suite-4.7.1-Darwin.dmg", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1-Darwin.dmg"},
			{Name: "suite-4.7.1-win64.exe", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1-win64.exe"},
			{Name: "suite-4.7.1-win64.msi", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1-win64.msi"},
			{Name: "suite-4.7.1.AppImage", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1.AppImage"},
			{Name: "suite-4.7.1.AppImage.asc", URL: "https://example.test/acme/suite/releases/download/4.7.1/suite-4.7.1.AppImage.asc"},
		}}, "suite-4.7.1-win64.exe", testWindowsAMDResolver},
		{args{"toolset", []*Asset{
			{Name: "toolset-0.8.2-darwin-amd64.tar.bz2", URL: "https://example.test/acme/toolset/releases/download/v0.8.2/toolset-0.8.2-darwin-amd64.tar.bz2"},
			{Name: "toolset-0.8.2-linux-amd64.tar.bz2", URL: "https://example.test/acme/toolset/releases/download/v0.8.2/toolset-0.8.2-linux-amd64.tar.bz2"},
			{Name: "toolset-0.8.2-windows-amd64.zip", URL: "https://example.test/acme/toolset/releases/download/v0.8.2/toolset-0.8.2-windows-amd64.zip"},
		}}, "toolset-0.8.2-linux-amd64.tar.bz2", testLinuxAMDResolver},
		{args{"toolset", []*Asset{
			{Name: "toolset-0.8.2-darwin-amd64.tar.bz2", URL: "https://example.test/acme/toolset/releases/download/v0.8.2/toolset-0.8.2-darwin-amd64.tar.bz2"},
			{Name: "toolset-0.8.2-linux-amd64.tar.bz2", URL: "https://example.test/acme/toolset/releases/download/v0.8.2/toolset-0.8.2-linux-amd64.tar.bz2"},
			{Name: "toolset-0.8.2-windows-amd64.zip", URL: "https://example.test/acme/toolset/releases/download/v0.8.2/toolset-0.8.2-windows-amd64.zip"},
		}}, "toolset-0.8.2-windows-amd64.zip", testWindowsAMDResolver},
		{args{"cli", []*Asset{
			{Name: "cli-tool", URL: ""},
		}}, "cli-tool", testLinuxAMDResolver},
		{args{"mytool", []*Asset{
			{Name: "mytool-v1.0.0-aarch64-apple-darwin.tar.gz", URL: "https://example.com/mytool-v1.0.0-aarch64-apple-darwin.tar.gz"},
			{Name: "mytool-v1.0.0-aarch64-apple-darwin.tar.gz.sha256", URL: "https://example.com/mytool-v1.0.0-aarch64-apple-darwin.tar.gz.sha256"},
			{Name: "mytool-v1.0.0-x86_64-apple-darwin.tar.gz", URL: "https://example.com/mytool-v1.0.0-x86_64-apple-darwin.tar.gz"},
			{Name: "mytool-v1.0.0-x86_64-apple-darwin.tar.gz.sha256", URL: "https://example.com/mytool-v1.0.0-x86_64-apple-darwin.tar.gz.sha256"},
		}}, "mytool-v1.0.0-aarch64-apple-darwin.tar.gz", testDarwinARMResolver},
		{args{"mytool", []*Asset{
			{Name: "mytool-linux-aarch64-musl.zip", URL: "https://example.com/mytool-linux-aarch64-musl.zip"},
			{Name: "mytool-linux-aarch64.zip", URL: "https://example.com/mytool-linux-aarch64.zip"},
			{Name: "mytool-macos-aarch64.zip", URL: "https://example.com/mytool-macos-aarch64.zip"},
		}}, "mytool-macos-aarch64.zip", testDarwinARMResolver},
		{args{"cli", []*Asset{
			{Name: "cli-linux-amd64-musl.gz", URL: "https://example.test/cli-linux-amd64-musl.gz"},
			{Name: "cli-linux-amd64.gz", URL: "https://example.test/cli-linux-amd64.gz"},
			{Name: "cli-linux-amd64-gnu.gz", URL: "https://example.test/cli-linux-amd64-gnu.gz"},
		}}, "cli-linux-amd64-gnu.gz", testLinuxAMDResolver},
		{args{"cli", []*Asset{
			{Name: "cli-linux-amd64-musl.gz", URL: "https://example.test/cli-linux-amd64-musl.gz"},
			{Name: "cli-linux-amd64.gz", URL: "https://example.test/cli-linux-amd64.gz"},
			{Name: "cli-linux-amd64-gnu.gz", URL: "https://example.test/cli-linux-amd64-gnu.gz"},
		}}, "cli-linux-amd64-musl.gz", testLinuxMuslResolver},
		{args{"cli", []*Asset{
			{Name: "cli-linux-amd64.gz", URL: "https://example.test/cli-linux-amd64.gz"},
			{Name: "cli-linux-amd64-musl.gz", URL: "https://example.test/cli-linux-amd64-musl.gz"},
		}}, "cli-linux-amd64.gz", testLinuxAMDResolver},
	}

	f := NewFilter(&FilterOpts{SkipScoring: false})
	for _, c := range cases {
		resolver = c.resolver
		if n, err := f.FilterAssets(c.in.repoName, c.in.as, ""); err != nil {
			for _, a := range c.in.as {
				fmt.Println(a.Name, c.resolver)
			}
			t.Fatalf("Error filtering assets %v", err)
		} else if n.Name != c.out {
			t.Fatalf("Error filtering %+v: %+v does not match %s", c.in, n, c.out)
		}
	}

}

func TestFilterAssetsSelect(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testLinuxAMDResolver
	isInteractive = func() bool { return true }
	// selectOption should NOT be called when autoSelect matches a candidate
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called when autoSelect matches a candidate")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{})
	result, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-linux-amd64", URL: "https://example.test/tool-linux-amd64"},
		{Name: "tool-linux-amd64.gz", URL: "https://example.test/tool-linux-amd64.gz"},
	}, "tool-linux-amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "tool-linux-amd64" {
		t.Fatalf("expected tool-linux-amd64, got %s", result.Name)
	}
}

func TestFilterAssetsPromptsWhenLibCRankingStillTies(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testLinuxAMDResolver
	isInteractive = func() bool { return true }
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called - tie-breaking should resolve this")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{})
	result, err := f.FilterAssets("cli", []*Asset{
		{Name: "cli-linux-amd64-gnu.gz", URL: "https://example.test/cli-linux-amd64-gnu.gz"},
		{Name: "cli-linux-amd64-gnu.zip", URL: "https://example.test/cli-linux-amd64-gnu.zip"},
		{Name: "cli-linux-amd64-musl.gz", URL: "https://example.test/cli-linux-amd64-musl.gz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tie-breaking should prefer standalone .gz over .zip
	if result.Name != "cli-linux-amd64-gnu.gz" {
		t.Fatalf("expected tie-breaking to select cli-linux-amd64-gnu.gz, got %s", result.Name)
	}
}

func TestFilterAssetsPrefersExplicitArchOverGenericSuffix(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testLinuxAMDResolver
	isInteractive = func() bool { return false }
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called when architecture ranking can resolve")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{})
	result, err := f.FilterAssets("jq", []*Asset{
		{Name: "jq-linux-amd64", URL: "https://example.test/jq-linux-amd64"},
		{Name: "jq-linux64", URL: "https://example.test/jq-linux64"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "jq-linux-amd64" {
		t.Fatalf("expected jq-linux-amd64, got %s", result.Name)
	}
}

func TestFilterAssetsFailsNonInteractiveWhenStillAmbiguous(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testLinuxAMDResolver
	isInteractive = func() bool { return false }
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called - tie-breaking should resolve this")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{})
	result, err := f.FilterAssets("cli", []*Asset{
		{Name: "cli-linux-amd64.tar.gz", URL: "https://example.test/cli-linux-amd64.tar.gz"},
		{Name: "cli-linux-amd64.zip", URL: "https://example.test/cli-linux-amd64.zip"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tie-breaking should prefer .tar.gz over .zip
	if result.Name != "cli-linux-amd64.tar.gz" {
		t.Fatalf("expected tie-breaking to select cli-linux-amd64.tar.gz, got %s", result.Name)
	}
}

func TestLooksLikeMetadataAsset(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  bool
	}{
		{
			name: "sha256 suffix",
			in:   "tool-darwin-aarch64.tar.gz.sha256",
			out:  true,
		},
		{
			name: "checksums token",
			in:   "checksums.txt",
			out:  true,
		},
		{
			name: "sigstore sidecar",
			in:   "trivy_0.69.3_Linux-64bit.tar.gz.sigstore.json",
			out:  true,
		},
		{
			name: "binary archive",
			in:   "tool-darwin-aarch64.tar.gz",
			out:  false,
		},
	}

	for _, c := range cases {
		result := looksLikeMetadataAsset(c.in)
		if result != c.out {
			t.Fatalf("%s: expected %v, got %v", c.name, c.out, result)
		}
	}
}

func TestLooksLikeArchiveJunk(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  bool
	}{
		{
			name: "readme markdown",
			in:   "mytool-1.0.0-darwin-arm64/README.md",
			out:  true,
		},
		{
			name: "license with no extension",
			in:   "mytool-1.0.0-darwin-arm64/LICENSE",
			out:  true,
		},
		{
			name: "unlicense with no extension",
			in:   "mytool-1.0.0-darwin-arm64/UNLICENSE",
			out:  true,
		},
		{
			name: "license with suffix",
			in:   "mytool-v1.0.0-aarch64-apple-darwin/LICENSE-MIT",
			out:  true,
		},
		{
			name: "autocomplete file",
			in:   "mytool-v1.0.0-aarch64-apple-darwin/autocomplete/_mytool",
			out:  true,
		},
		{
			name: "man page",
			in:   "mytool-v1.0.0-aarch64-apple-darwin/mytool.1",
			out:  true,
		},
		{
			name: "binary without extension",
			in:   "mytool-1.0.0-darwin-arm64/mytool",
			out:  false,
		},
		{
			name: "windows binary",
			in:   "tool/windows/tool.exe",
			out:  false,
		},
		{
			name: "backslash path license",
			in:   "tool-1.0\\LICENSE",
			out:  true,
		},
		{
			name: "backslash path binary",
			in:   "tool-1.0\\tool",
			out:  false,
		},
		{
			name: "completions directory",
			in:   "tool-1.0/completions/tool.bash",
			out:  true,
		},
		{
			name: "complete directory",
			in:   "tool-1.0/complete/tool.bash",
			out:  true,
		},
		{
			name: "contrib directory",
			in:   "tool-1.0/contrib/report.tpl",
			out:  true,
		},
		{
			name: "template suffix",
			in:   "tool-1.0/report.tpl",
			out:  true,
		},
	}

	for _, c := range cases {
		result := looksLikeArchiveJunk(c.in)
		if result != c.out {
			t.Fatalf("%s: expected %v, got %v", c.name, c.out, result)
		}
	}
}

func TestFilterArchiveAssets(t *testing.T) {
	as := []*Asset{
		{Name: "mytool-1.0.0-darwin-arm64/LICENSE"},
		{Name: "mytool-1.0.0-darwin-arm64/README.md"},
		{Name: "mytool-1.0.0-darwin-arm64/mytool"},
	}

	filtered := filterArchiveAssets(as)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 archive candidate, got %d (%v)", len(filtered), filtered)
	}
	if filtered[0].Name != "mytool-1.0.0-darwin-arm64/mytool" {
		t.Fatalf("unexpected selected archive candidate %s", filtered[0].Name)
	}
}

func TestFilterArchiveAssetsComplexLayout(t *testing.T) {
	as := []*Asset{
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/CHANGELOG.md"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/LICENSE-APACHE"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/LICENSE-MIT"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/README.md"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/autocomplete/_mytool"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/autocomplete/_mytool.ps1"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/autocomplete/mytool.bash"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/autocomplete/mytool.fish"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/mytool"},
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/mytool.1"},
	}

	filtered := filterArchiveAssets(as)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 archive candidate, got %d (%v)", len(filtered), filtered)
	}
	if filtered[0].Name != "mytool-v1.0.0-aarch64-apple-darwin/mytool" {
		t.Fatalf("unexpected selected archive candidate %s", filtered[0].Name)
	}
}

func TestFilterArchiveAssetsAllFiltered(t *testing.T) {
	as := []*Asset{
		{Name: "pkg/README.md"},
		{Name: "pkg/LICENSE"},
	}

	filtered := filterArchiveAssets(as)
	if len(filtered) != len(as) {
		t.Fatalf("expected fallback to original list (%d items), got %d", len(as), len(filtered))
	}
}

func TestLooksLikeManPageExt(t *testing.T) {
	cases := []struct {
		in  string
		out bool
	}{
		{in: ".1", out: true},
		{in: ".8", out: true},
		{in: ".0", out: false},
		{in: ".md", out: false},
		{in: ".10", out: false},
	}

	for _, c := range cases {
		result := looksLikeManPageExt(c.in)
		if result != c.out {
			t.Fatalf("ext %s: expected %v, got %v", c.in, c.out, result)
		}
	}
}

func TestIsSupportedExt(t *testing.T) {
	cases := []struct {
		in  string
		out bool
	}{
		{
			"suite-4.8.0.AppImage",
			true,
		},
		{
			"suite-4.7.1-win64.msi",
			false,
		},
	}

	for _, c := range cases {
		result := isSupportedExt(c.in)
		if result != c.out {
			t.Fatalf("Expected result for extension %v to be %v, but got result %v", c.in, c.out, result)
		}
	}

}

func TestProcessTarMatchesByBasename(t *testing.T) {
	// Build a tar with a new version in directory name
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := map[string]string{
		"tool-v2.0.0-aarch64-apple-darwin/LICENSE": "license text",
		"tool-v2.0.0-aarch64-apple-darwin/tool":    "binary content",
	}
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()

	// PackagePath has old version in directory name but same basename
	f := NewFilter(&FilterOpts{PackagePath: "tool-v1.0.0-aarch64-apple-darwin/tool"})
	result, err := f.processTar("tool", &buf, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "tool" {
		t.Fatalf("expected file name 'tool', got %q", result.Name)
	}
}
