package assets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
	originalResolver := resolver
	defer func() { resolver = originalResolver }()

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
		{"fff-mcp-x86_64-unknown-linux-gnu", "v0.10.1", "fff-mcp", testLinuxAMDResolver},
		{"fff-mcp-aarch64-pc-windows-msvc.exe", "v0.10.1", "fff-mcp.exe", testWindowsAMDResolver},
	}

	for _, c := range cases {
		resolver = c.resolver
		if n := SanitizeName(c.in, c.v); n != c.out {
			t.Fatalf("Error replacing %s: %s does not match %s", c.in, n, c.out)
		}
	}

}

func TestProcessURLValidatesArchiveChecksum(t *testing.T) {
	archiveData := buildTestZipArchive(t, map[string]string{"tool": "#!/bin/sh\nhello from archive"})
	expectedHash := sha256.Sum256(archiveData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"

	result, err := f.ProcessURL(&FilteredAsset{Name: "tool-linux-amd64.zip", URL: server.URL}, fmt.Sprintf("%x", expectedHash[:]), true)
	if err != nil {
		t.Fatalf("ProcessURL returned error: %v", err)
	}

	data, err := io.ReadAll(result.Source)
	if err != nil {
		t.Fatalf("failed to read processed archive entry: %v", err)
	}
	if closer, ok := result.Source.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("failed to close processed archive entry: %v", err)
		}
	}

	if string(data) != "#!/bin/sh\nhello from archive" {
		t.Fatalf("unexpected archive contents: %q", string(data))
	}
	if result.Name != "tool" {
		t.Fatalf("unexpected extracted file name: %s", result.Name)
	}
}

func TestProcessURLRejectsArchiveChecksumMismatch(t *testing.T) {
	archiveData := buildTestZipArchive(t, map[string]string{"tool": "hello from archive"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"

	_, err := f.ProcessURL(&FilteredAsset{Name: "tool-linux-amd64.zip", URL: server.URL}, strings.Repeat("0", 64), true)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessURLPreservesNameForTarGzArchives(t *testing.T) {
	archiveData := buildTestTarGzArchive(t, map[string]string{
		"ripgrep-13.0.0-x86_64-unknown-linux-musl/rg": "rg binary",
	})
	expectedHash := sha256.Sum256(archiveData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "ripgrep"

	result, err := f.ProcessURL(&FilteredAsset{Name: "ripgrep-13.0.0-x86_64-unknown-linux-musl.tar.gz", URL: server.URL}, fmt.Sprintf("%x", expectedHash[:]), true)
	if err != nil {
		t.Fatalf("ProcessURL returned error: %v", err)
	}

	if result.Name != "rg" {
		t.Fatalf("unexpected extracted file name: %s", result.Name)
	}
	if result.PackagePath != "ripgrep-13.0.0-x86_64-unknown-linux-musl/rg" {
		t.Fatalf("unexpected package path: %s", result.PackagePath)
	}

	data, err := io.ReadAll(result.Source)
	if err != nil {
		t.Fatalf("failed to read processed tar.gz entry: %v", err)
	}
	if closer, ok := result.Source.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("failed to close processed tar.gz entry: %v", err)
		}
	}
	if string(data) != "rg binary" {
		t.Fatalf("unexpected tar.gz contents: %q", string(data))
	}
}

func buildTestZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, contents := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o755)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatalf("failed to write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip archive: %v", err)
	}

	return buf.Bytes()
}

func buildTestTarGzArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, contents := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(contents)); err != nil {
			t.Fatalf("failed to write tar entry %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar archive: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip stream: %v", err)
	}

	return buf.Bytes()
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
		{args{"goreleaser", []*Asset{
			{Name: "goreleaser_2.15.2_linux_amd64.flatpak", URL: "https://example.test/goreleaser/goreleaser/releases/download/v2.15.2/goreleaser_2.15.2_linux_amd64.flatpak"},
			{Name: "goreleaser-2.15.2-1-x86_64.pkg.tar.zst", URL: "https://example.test/goreleaser/goreleaser/releases/download/v2.15.2/goreleaser-2.15.2-1-x86_64.pkg.tar.zst"},
			{Name: "goreleaser_2.15.2_amd64.deb", URL: "https://example.test/goreleaser/goreleaser/releases/download/v2.15.2/goreleaser_2.15.2_amd64.deb"},
			{Name: "goreleaser_Linux_x86_64.tar.gz", URL: "https://example.test/goreleaser/goreleaser/releases/download/v2.15.2/goreleaser_Linux_x86_64.tar.gz"},
		}}, "goreleaser_Linux_x86_64.tar.gz", testLinuxAMDResolver},
		{args{"wesm/agentsview", []*Asset{
			{Name: "AgentsView_0.25.0_amd64.AppImage", URL: "https://example.test/wesm/agentsview/releases/download/v0.25.0/AgentsView_0.25.0_amd64.AppImage"},
			{Name: "agentsview_0.25.0_linux_amd64.tar.gz", URL: "https://example.test/wesm/agentsview/releases/download/v0.25.0/agentsview_0.25.0_linux_amd64.tar.gz"},
			{Name: "agentsview_0.25.0_windows_amd64.zip", URL: "https://example.test/wesm/agentsview/releases/download/v0.25.0/agentsview_0.25.0_windows_amd64.zip"},
		}}, "agentsview_0.25.0_linux_amd64.tar.gz", testLinuxAMDResolver},
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

func TestFilterAssetsSelectsFFFExecutableProduct(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testDarwinARMResolver

	assets := []*Asset{
		{Name: "aarch64-apple-darwin.dylib"},
		{Name: "aarch64-unknown-linux-gnu.so"},
		{Name: "c-lib-aarch64-apple-darwin.dylib"},
		{Name: "fff-mcp-aarch64-apple-darwin"},
		{Name: "fff-mcp-x86_64-apple-darwin"},
		{Name: "fff_search-0.10.1-cp310-abi3-macosx_11_0_arm64.whl"},
		{Name: "fff_search-0.10.1.tar.gz"},
		{Name: "x86_64-pc-windows-msvc.dll"},
	}

	f := NewFilter(&FilterOpts{NonInteractive: true})
	result, err := f.FilterAssets("fff", assets, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "fff-mcp-aarch64-apple-darwin" {
		t.Fatalf("unexpected selected asset: %s", result.Name)
	}

	compatible := NewFilter(&FilterOpts{SkipScoring: true}).CompatibleAssets(assets, "")
	if len(compatible) != 1 || compatible[0].Name != "fff-mcp-aarch64-apple-darwin" {
		t.Fatalf("unexpected --all candidates: %+v", compatible)
	}
}

func TestFilterAssetsFailsNonInteractiveForMultipleProducts(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testLinuxAMDResolver

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.FilterAssets("arrow-tools", []*Asset{
		{Name: "csv2arrow-x86_64-unknown-linux-gnu.tar.xz"},
		{Name: "csv2parquet-x86_64-unknown-linux-gnu.tar.xz"},
		{Name: "json2arrow-x86_64-unknown-linux-gnu.tar.xz"},
	}, "")
	if err == nil {
		t.Fatal("expected multiple products to fail")
	}
	if !strings.Contains(err.Error(), "use --select") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilterAssetsExactSelectionPrecedesProductRanking(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testLinuxAMDResolver

	f := NewFilter(&FilterOpts{NonInteractive: true})
	result, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool"},
		{Name: "tool-linux-amd64"},
	}, "tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "tool" {
		t.Fatalf("unexpected selected asset: %s", result.Name)
	}
}

func TestFilterAssetsPrefersCLIProductAlias(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testLinuxAMDResolver

	f := NewFilter(&FilterOpts{PackageName: "weave", NonInteractive: true})
	result, err := f.FilterAssets("weave", []*Asset{
		{Name: "weave-cli-linux-amd64.tar.gz"},
		{Name: "weave-driver-linux-amd64.tar.gz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "weave-cli-linux-amd64.tar.gz" {
		t.Fatalf("unexpected selected asset: %s", result.Name)
	}
}

func TestProcessURLRejectsWheelMetadata(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	header := &zip.FileHeader{Name: "fff_search-0.10.1.dist-info/WHEEL", Method: zip.Deflate}
	header.SetMode(0o755)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("Wheel-Version: 1.0\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "fff"
	_, err = f.ProcessURL(&FilteredAsset{Name: "fff_search-0.10.1.whl", URL: server.URL}, "", false)
	if !errors.Is(err, ErrNoCompatibleFiles) {
		t.Fatalf("expected no compatible executable, got %v", err)
	}
}

func TestProcessURLRejectsExplicitLibrarySelection(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testDarwinARMResolver

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\n"))
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	_, err := f.ProcessURL(&FilteredAsset{Name: "tool-aarch64-apple-darwin.dylib", URL: server.URL}, "", false)
	if !errors.Is(err, ErrNoCompatibleFiles) {
		t.Fatalf("expected library selection to fail, got %v", err)
	}
}

func TestProcessURLAcceptsUniversalMachOExecutable(t *testing.T) {
	originalResolver := resolver
	defer func() { resolver = originalResolver }()
	resolver = testDarwinARMResolver

	payload := buildTestFatMachO(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	f := NewFilter(&FilterOpts{NonInteractive: true})
	f.repoName = "tool"
	result, err := f.ProcessURL(&FilteredAsset{Name: "tool-universal-apple-darwin", URL: server.URL}, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result.Source.(io.Closer).Close()
}

func buildTestFatMachO(t *testing.T) []byte {
	t.Helper()

	thinAMD64 := buildTestThinMachO(t, 0x01000007, 3)
	thinARM64 := buildTestThinMachO(t, 0x0100000c, 0)

	var fat bytes.Buffer
	for _, value := range []uint32{
		0xcafebabe, // fat Mach-O magic
		2,
		0x01000007,
		3,
		48,
		uint32(len(thinAMD64)),
		0,
		0x0100000c,
		0,
		48 + uint32(len(thinAMD64)),
		uint32(len(thinARM64)),
		0,
	} {
		if err := binary.Write(&fat, binary.BigEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fat.Write(thinAMD64); err != nil {
		t.Fatal(err)
	}
	if _, err := fat.Write(thinARM64); err != nil {
		t.Fatal(err)
	}
	return fat.Bytes()
}

func buildTestThinMachO(t *testing.T, cpu, subtype uint32) []byte {
	t.Helper()
	var thin bytes.Buffer
	for _, value := range []uint32{
		0xfeedfacf,
		cpu,
		subtype,
		2, // MH_EXECUTE
		0, 0, 0, 0,
	} {
		if err := binary.Write(&thin, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	return thin.Bytes()
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

func TestFilterAssetsPrefersPackageNameForMultiToolRepos(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testDarwinARMResolver
	isInteractive = func() bool { return false }
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called when package name can break the tie")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{PackageName: "csv2parquet"})
	result, err := f.FilterAssets("arrow-tools", []*Asset{
		{Name: "csv2arrow-aarch64-apple-darwin.tar.xz", URL: "https://example.test/domoritz/arrow-tools/releases/download/v0.26.0/csv2arrow-aarch64-apple-darwin.tar.xz"},
		{Name: "csv2parquet-aarch64-apple-darwin.tar.xz", URL: "https://example.test/domoritz/arrow-tools/releases/download/v0.26.0/csv2parquet-aarch64-apple-darwin.tar.xz"},
		{Name: "json2arrow-aarch64-apple-darwin.tar.xz", URL: "https://example.test/domoritz/arrow-tools/releases/download/v0.26.0/json2arrow-aarch64-apple-darwin.tar.xz"},
		{Name: "json2parquet-aarch64-apple-darwin.tar.xz", URL: "https://example.test/domoritz/arrow-tools/releases/download/v0.26.0/json2parquet-aarch64-apple-darwin.tar.xz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "csv2parquet-aarch64-apple-darwin.tar.xz" {
		t.Fatalf("expected csv2parquet-aarch64-apple-darwin.tar.xz, got %s", result.Name)
	}
}

func TestFilterAssetsFallsBackToPackagePathBasename(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testDarwinARMResolver
	isInteractive = func() bool { return false }
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called when package path can break the tie")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{PackagePath: "csv2parquet-aarch64-apple-darwin/csv2parquet"})
	result, err := f.FilterAssets("arrow-tools", []*Asset{
		{Name: "csv2arrow-aarch64-apple-darwin.tar.xz", URL: "https://example.test/domoritz/arrow-tools/releases/download/v0.26.0/csv2arrow-aarch64-apple-darwin.tar.xz"},
		{Name: "csv2parquet-aarch64-apple-darwin.tar.xz", URL: "https://example.test/domoritz/arrow-tools/releases/download/v0.26.0/csv2parquet-aarch64-apple-darwin.tar.xz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "csv2parquet-aarch64-apple-darwin.tar.xz" {
		t.Fatalf("expected csv2parquet-aarch64-apple-darwin.tar.xz, got %s", result.Name)
	}
}

func TestFilterAssetsIgnoresMetadataPackageName(t *testing.T) {
	originalResolver := resolver
	originalSelect := selectOption
	originalIsInteractive := isInteractive
	defer func() {
		resolver = originalResolver
		selectOption = originalSelect
		isInteractive = originalIsInteractive
	}()

	resolver = testDarwinARMResolver
	isInteractive = func() bool { return false }
	selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
		t.Fatal("selectOption should not be called when metadata package name is ignored")
		return nil, nil
	}

	f := NewFilter(&FilterOpts{PackageName: "ghtkn_darwin_arm64.tar.gz.sbom.json"})
	result, err := f.FilterAssets("ghtkn", []*Asset{
		{Name: "ghtkn_darwin_amd64.tar.gz", URL: "https://example.test/suzuki-shunsuke/ghtkn/releases/download/v0.2.4/ghtkn_darwin_amd64.tar.gz"},
		{Name: "ghtkn_darwin_arm64.tar.gz", URL: "https://example.test/suzuki-shunsuke/ghtkn/releases/download/v0.2.4/ghtkn_darwin_arm64.tar.gz"},
		{Name: "ghtkn_darwin_arm64.tar.gz.sbom.json", URL: "https://example.test/suzuki-shunsuke/ghtkn/releases/download/v0.2.4/ghtkn_darwin_arm64.tar.gz.sbom.json"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "ghtkn_darwin_arm64.tar.gz" {
		t.Fatalf("expected ghtkn_darwin_arm64.tar.gz, got %s", result.Name)
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
			name: "sbom file",
			in:   "DockerSandboxes-linux-amd64.sbom.json",
			out:  true,
		},
		{
			name: "provenance artifact",
			in:   "tool-linux-amd64.provenance.json",
			out:  true,
		},
		{
			name: "attestation sidecar suffix",
			in:   "tool-darwin-arm64.attestation.json",
			out:  true,
		},
		{
			name: "cyclonedx sidecar suffix",
			in:   "tool-linux-amd64.cyclonedx.json",
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

func TestLooksLikePackageArtifact(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  bool
	}{
		{
			name: "flatpak package",
			in:   "goreleaser_2.15.2_linux_amd64.flatpak",
			out:  true,
		},
		{
			name: "arch package",
			in:   "goreleaser-2.15.2-1-x86_64.pkg.tar.zst",
			out:  true,
		},
		{
			name: "binary tarball",
			in:   "goreleaser_Linux_x86_64.tar.gz",
			out:  false,
		},
	}

	for _, c := range cases {
		result := looksLikePackageArtifact(c.in)
		if result != c.out {
			t.Fatalf("%s: expected %v, got %v", c.name, c.out, result)
		}
	}
}

func TestFilterAssetsIgnoresPackageArtifactsByDefault(t *testing.T) {
	originalResolver := resolver
	originalLookPath := lookPath
	defer func() {
		resolver = originalResolver
		lookPath = originalLookPath
	}()

	resolver = testLinuxAMDResolver
	lookPath = func(string) (string, error) { return "/usr/bin/dpkg", nil }

	f := NewFilter(&FilterOpts{})
	result, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-linux-amd64.deb", URL: "https://example.test/tool-linux-amd64.deb"},
		{Name: "tool-linux-amd64.tar.gz", URL: "https://example.test/tool-linux-amd64.tar.gz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "tool-linux-amd64.tar.gz" {
		t.Fatalf("expected non-package artifact, got %s", result.Name)
	}
}

func TestFilterAssetsSelectsCompatibleSystemPackageWhenEnabled(t *testing.T) {
	originalResolver := resolver
	originalLookPath := lookPath
	defer func() {
		resolver = originalResolver
		lookPath = originalLookPath
	}()

	resolver = testLinuxAMDResolver
	lookPath = func(name string) (string, error) {
		if name == "dpkg" {
			return "/usr/bin/dpkg", nil
		}
		return "", exec.ErrNotFound
	}

	f := NewFilter(&FilterOpts{SystemPackage: true, PackageType: "deb", NonInteractive: true})
	result, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-linux-amd64.deb", URL: "https://example.test/tool-linux-amd64.deb"},
		{Name: "tool-linux-amd64.tar.gz", URL: "https://example.test/tool-linux-amd64.tar.gz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "tool-linux-amd64.deb" {
		t.Fatalf("expected deb package artifact, got %s", result.Name)
	}
}

func TestFilterAssetsFailsWhenRequiredSystemPackageUnavailable(t *testing.T) {
	originalResolver := resolver
	originalLookPath := lookPath
	defer func() {
		resolver = originalResolver
		lookPath = originalLookPath
	}()

	resolver = testLinuxAMDResolver
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	f := NewFilter(&FilterOpts{SystemPackage: true, PackageType: "deb", NonInteractive: true})
	_, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-linux-amd64.deb", URL: "https://example.test/tool-linux-amd64.deb"},
	}, "")
	if err == nil {
		t.Fatal("expected filtering to fail when package manager tool is unavailable")
	}
	if !strings.Contains(err.Error(), "Could not find any compatible files") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilterAssetsSelectsCompatibleDMGOnMacOS(t *testing.T) {
	originalResolver := resolver
	originalLookPath := lookPath
	defer func() {
		resolver = originalResolver
		lookPath = originalLookPath
	}()

	resolver = testDarwinARMResolver
	lookPath = func(name string) (string, error) {
		if name == "hdiutil" {
			return "/usr/bin/hdiutil", nil
		}
		return "", exec.ErrNotFound
	}

	f := NewFilter(&FilterOpts{SystemPackage: true, PackageType: "dmg", NonInteractive: true})
	result, err := f.FilterAssets("paseo", []*Asset{
		{Name: "Paseo-0.1.64-arm64.dmg", URL: "https://example.test/Paseo-0.1.64-arm64.dmg"},
		{Name: "Paseo-Setup-0.1.64-arm64.exe", URL: "https://example.test/Paseo-Setup-0.1.64-arm64.exe"},
		{Name: "paseo-darwin-arm64.tar.gz", URL: "https://example.test/paseo-darwin-arm64.tar.gz"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Paseo-0.1.64-arm64.dmg" {
		t.Fatalf("expected dmg package artifact, got %s", result.Name)
	}
}

func TestFilterAssetsRejectsWrongPlatformExeInBinaryMode(t *testing.T) {
	originalResolver := resolver
	defer func() {
		resolver = originalResolver
	}()

	resolver = testDarwinARMResolver

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.FilterAssets("paseo", []*Asset{
		{Name: "Paseo-Setup-0.1.104-arm64.exe", URL: "https://example.test/Paseo-Setup-0.1.104-arm64.exe"},
		{Name: "Paseo-0.1.104-arm64.dmg", URL: "https://example.test/Paseo-0.1.104-arm64.dmg"},
	}, "")
	if err == nil {
		t.Fatal("expected no compatible files; wrong-platform .exe should not be selected in binary mode")
	}
	if !errors.Is(err, ErrNoCompatibleFiles) && !strings.Contains(err.Error(), "Could not find any compatible files") {
		t.Fatalf("expected no-compatible-files error, got %v", err)
	}
}

func TestFilterAssetsRejectsMetadataOnlyCandidatesInBinaryMode(t *testing.T) {
	originalResolver := resolver
	defer func() {
		resolver = originalResolver
	}()

	resolver = testLinuxAMDResolver

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-linux-amd64.tar.gz.sha256", URL: "https://example.test/tool-linux-amd64.tar.gz.sha256"},
		{Name: "tool-linux-amd64.tar.gz.sig", URL: "https://example.test/tool-linux-amd64.tar.gz.sig"},
		{Name: "tool-linux-amd64.tar.gz.minisig", URL: "https://example.test/tool-linux-amd64.tar.gz.minisig"},
		{Name: "tool-linux-amd64.tar.gz.sbom.json", URL: "https://example.test/tool-linux-amd64.tar.gz.sbom.json"},
	}, "")
	if err == nil {
		t.Fatal("expected no compatible files for metadata-only candidates")
	}
	if !errors.Is(err, ErrNoCompatibleFiles) && !strings.Contains(err.Error(), "Could not find any compatible files") {
		t.Fatalf("expected no-compatible-files error, got %v", err)
	}
}

func TestFilterAssetsRejectsPackageOnlyCandidatesInBinaryMode(t *testing.T) {
	originalResolver := resolver
	defer func() {
		resolver = originalResolver
	}()

	resolver = testDarwinARMResolver

	f := NewFilter(&FilterOpts{NonInteractive: true})
	_, err := f.FilterAssets("tool", []*Asset{
		{Name: "tool-darwin-arm64.dmg", URL: "https://example.test/tool-darwin-arm64.dmg"},
		{Name: "tool-linux-amd64.msi", URL: "https://example.test/tool-linux-amd64.msi"},
		{Name: "tool-linux-amd64.deb", URL: "https://example.test/tool-linux-amd64.deb"},
		{Name: "tool-linux-amd64.rpm", URL: "https://example.test/tool-linux-amd64.rpm"},
	}, "")
	if err == nil {
		t.Fatal("expected no compatible files for package-only candidates")
	}
	if !errors.Is(err, ErrNoCompatibleFiles) && !strings.Contains(err.Error(), "Could not find any compatible files") {
		t.Fatalf("expected no-compatible-files error, got %v", err)
	}
}

func TestFilterAssetsUsesNormalizedAssetNameOnlyForCompatibility(t *testing.T) {
	originalResolver := resolver
	defer func() {
		resolver = originalResolver
	}()

	resolver = testDarwinARMResolver

	t.Run("url and display metadata do not override asset name", func(t *testing.T) {
		f := NewFilter(&FilterOpts{NonInteractive: true})
		result, err := f.FilterAssets("paseo", []*Asset{
			{
				Name:        "tool.tar.gz",
				DisplayName: "Paseo-Setup-0.1.104-arm64.exe",
				URL:         "https://example.test/download/tool.exe?sig=1#tool.exe",
			},
			{Name: "Paseo-0.1.104-arm64.dmg", URL: "https://example.test/Paseo-0.1.104-arm64.dmg"},
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Name != "tool.tar.gz" {
			t.Fatalf("expected FilterAssets to use Asset.Name only, got %s", result.Name)
		}
	})

	for _, tc := range []struct {
		name  string
		asset *Asset
	}{
		{
			name: "metadata suffix stays filtered even when display metadata looks executable",
			asset: &Asset{
				Name:        "tool.exe.sha256",
				DisplayName: "Paseo-Setup-0.1.104-arm64.exe",
				URL:         "https://example.test/download/Paseo-Setup-0.1.104-arm64.exe?sig=1",
			},
		},
		{
			name: "windows path separators do not bypass asset-name checks",
			asset: &Asset{
				Name:        `dir\\tool.exe`,
				DisplayName: "Paseo-Setup-0.1.104-arm64.exe",
				URL:         "https://example.test/download/Paseo-Setup-0.1.104-arm64.exe?sig=1",
			},
		},
		{
			name: "parent path segments do not bypass asset-name checks",
			asset: &Asset{
				Name:        "../tool.exe",
				DisplayName: "Paseo-Setup-0.1.104-arm64.exe",
				URL:         "https://example.test/download/Paseo-Setup-0.1.104-arm64.exe?sig=1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFilter(&FilterOpts{NonInteractive: true})
			_, err := f.FilterAssets("paseo", []*Asset{
				tc.asset,
				{Name: "Paseo-0.1.104-arm64.dmg", URL: "https://example.test/Paseo-0.1.104-arm64.dmg"},
			}, "")
			if err == nil {
				t.Fatal("expected no compatible files")
			}
			if !errors.Is(err, ErrNoCompatibleFiles) && !strings.Contains(err.Error(), "Could not find any compatible files") {
				t.Fatalf("expected no-compatible-files error, got %v", err)
			}
		})
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
			name: "top level completions directory",
			in:   "completions/tool.bash",
			out:  true,
		},
		{
			name: "compressed manpage",
			in:   "tool-1.0/manpages/tool.1.gz",
			out:  true,
		},
		{
			name: "top level manpages directory",
			in:   "manpages/tool.1.gz",
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
		{Name: "mytool-v1.0.0-aarch64-apple-darwin/manpages/mytool.1.gz"},
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
	if len(filtered) != 0 {
		t.Fatalf("expected no archive candidates, got %d", len(filtered))
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
			"tool-linux-amd64.tar.gz.sha256",
			false,
		},
		{
			"tool-linux-amd64.tar.gz.sig",
			false,
		},
		{
			"tool-linux-amd64.tar.gz.minisig",
			false,
		},
		{
			"tool-linux-amd64.tar.gz.sbom.json",
			false,
		},
		{
			"tool-darwin-arm64.dmg",
			false,
		},
		{
			"goreleaser_2.15.2_linux_amd64.flatpak",
			false,
		},
		{
			"tool-linux-amd64.deb",
			false,
		},
		{
			"goreleaser-2.15.2-1-x86_64.pkg.tar.zst",
			false,
		},
		{
			"tool-linux-amd64.rpm",
			false,
		},
		{
			"suite-4.7.1-win64.msi",
			false,
		},
		{
			"codebase-memory-mcp-darwin-arm64.tar.gz.bundle",
			false,
		},
		{
			"codebase-memory-mcp-darwin-arm64.tar.gz",
			true,
		},
		{
			"codebase-memory-mcp",
			true,
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

func TestProcessTarIgnoresCompressedManpages(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := map[string]string{
		"LICENSE":                    "license text",
		"README.md":                  "readme text",
		"completions/infisical.bash": "completion",
		"completions/infisical.fish": "completion",
		"completions/infisical.zsh":  "completion",
		"manpages/infisical.1.gz":    "manpage",
		"infisical":                  "binary content",
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
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	f := NewFilter(&FilterOpts{})
	result, err := f.processTar("cli", &buf, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "infisical" {
		t.Fatalf("expected file name 'infisical', got %q", result.Name)
	}
	if result.PackagePath != "infisical" {
		t.Fatalf("expected package path 'infisical', got %q", result.PackagePath)
	}
}

func TestProcessTarPrefersExecutableWhenRepoNameDiffers(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := map[string]string{
		"infisical":  "binary content",
		"install.sh": "#!/bin/sh\nexit 0\n",
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
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	f := NewFilter(&FilterOpts{})
	result, err := f.processTar("cli", &buf, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "infisical" {
		t.Fatalf("expected file name 'infisical', got %q", result.Name)
	}
	if result.PackagePath != "infisical" {
		t.Fatalf("expected package path 'infisical', got %q", result.PackagePath)
	}
}

func TestPreferArchiveExecutableCandidates(t *testing.T) {
	candidates := preferArchiveExecutableCandidates([]*Asset{
		{Name: "bin/tool"},
		{Name: "tool.exe"},
		{Name: "tool"},
		{Name: "scripts/install.sh"},
	})

	if len(candidates) != 2 {
		t.Fatalf("expected 2 preferred archive candidates, got %d", len(candidates))
	}
	if candidates[0].Name != "tool.exe" || candidates[1].Name != "tool" {
		t.Fatalf("unexpected preferred archive candidates: %+v", candidates)
	}
}
