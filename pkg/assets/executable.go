package assets

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const peImageFileDLL = 0x2000

func isRunnablePayload(path, name string, mode fs.FileMode) bool {
	if looksLikeLibrary(name) {
		return false
	}

	if hasShebang(path) {
		return !runtimeMatchesOS("windows", "win")
	}
	if isELFExecutable(path, name) {
		return runtimeMatchesOS("linux", "android", "freebsd", "openbsd", "netbsd", "dragonfly")
	}
	if isMachOExecutable(path) {
		return runtimeMatchesOS("darwin", "macos", "osx")
	}
	if isPEExecutable(path) {
		return runtimeMatchesOS("windows", "win")
	}

	return mode&0o111 != 0 && filepath.Ext(normalizedAssetBasename(name)) == ""
}

func looksLikeLibrary(name string) bool {
	lower := strings.ToLower(normalizedAssetBasename(name))
	for _, suffix := range []string{".so", ".dylib", ".dll"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func hasShebang(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := []byte{0, 0}
	if _, err := f.Read(header); err != nil {
		return false
	}
	return header[0] == '#' && header[1] == '!'
}

func isELFExecutable(path, name string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	switch f.Type {
	case elf.ET_EXEC:
		return true
	case elf.ET_DYN:
		sonames, _ := f.DynString(elf.DT_SONAME)
		if len(sonames) > 0 || looksLikeLibrary(name) {
			return false
		}
		for _, program := range f.Progs {
			if program.Type == elf.PT_INTERP {
				return true
			}
		}
		return filepath.Ext(normalizedAssetBasename(name)) == ""
	default:
		return false
	}
}

func isMachOExecutable(path string) bool {
	f, err := macho.Open(path)
	if err == nil {
		defer f.Close()
		return f.Type == macho.TypeExec && machOCPUCompatible(f.Cpu)
	}

	fat, err := macho.OpenFat(path)
	if err != nil {
		return false
	}
	defer fat.Close()
	for _, architecture := range fat.Arches {
		if architecture.Type == macho.TypeExec && machOCPUCompatible(architecture.Cpu) {
			return true
		}
	}
	return false
}

func machOCPUCompatible(cpu macho.Cpu) bool {
	for _, arch := range resolver.GetArch() {
		switch strings.ToLower(arch) {
		case "amd64", "x86_64", "x64":
			if cpu == macho.CpuAmd64 {
				return true
			}
		case "arm64", "aarch64":
			if cpu == macho.CpuArm64 {
				return true
			}
		case "386", "i386", "x86":
			if cpu == macho.Cpu386 {
				return true
			}
		case "arm", "armv6", "armv7":
			if cpu == macho.CpuArm {
				return true
			}
		}
	}
	return false
}

func isPEExecutable(path string) bool {
	f, err := pe.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return f.OptionalHeader != nil && f.Characteristics&peImageFileDLL == 0
}

func runtimeMatchesOS(names ...string) bool {
	for _, current := range resolver.GetOS() {
		for _, name := range names {
			if strings.EqualFold(current, name) {
				return true
			}
		}
	}
	return false
}
