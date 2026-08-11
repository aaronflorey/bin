package assets

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"strings"
)

const peImageFileDLL = 0x2000

// ErrNotRunnablePayload indicates that a direct-binary payload cannot be
// executed on the current platform.
var ErrNotRunnablePayload = errors.New("not a runnable payload")

// ValidateRunnablePayload verifies final bytes, rather than trusting a file
// extension or mode bit. It is the shared safety gate for installs and cached
// runs.
func ValidateRunnablePayload(path, sourceName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || IsKnownNonRunnableName(sourceName) || looksLikeLibrary(sourceName) {
		return fmt.Errorf("%w: %s", ErrNotRunnablePayload, sourceName)
	}
	if hasShebang(path) {
		if runtimeMatchesOS("windows", "win") {
			return fmt.Errorf("%w: Unix script is incompatible with this platform", ErrNotRunnablePayload)
		}
		return nil
	}
	if isWindowsBatchScript(path, sourceName) {
		if runtimeMatchesOS("windows", "win") {
			return nil
		}
		return fmt.Errorf("%w: Windows batch script is incompatible with this platform", ErrNotRunnablePayload)
	}
	if isELFExecutable(path, sourceName) && runtimeMatchesOS("linux", "android", "freebsd", "openbsd", "netbsd", "dragonfly") {
		return nil
	}
	if isMachOExecutable(path) && runtimeMatchesOS("darwin", "macos", "osx") {
		return nil
	}
	if isPEExecutable(path) && runtimeMatchesOS("windows", "win") {
		return nil
	}
	return fmt.Errorf("%w: %s is not a compatible executable or script", ErrNotRunnablePayload, sourceName)
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
		return elfMachineCompatible(f.Machine)
	case elf.ET_DYN:
		sonames, _ := f.DynString(elf.DT_SONAME)
		if len(sonames) > 0 || looksLikeLibrary(name) || f.Entry == 0 || !elfMachineCompatible(f.Machine) {
			return false
		}
		for _, program := range f.Progs {
			if program.Type == elf.PT_INTERP {
				return true
			}
		}
		// A dynamic object with an entry point, no SONAME and no interpreter is
		// a static PIE; it is runnable without relying on filename shape.
		return true
	default:
		return false
	}
}

func elfMachineCompatible(machine elf.Machine) bool {
	for _, arch := range resolver.GetArch() {
		switch strings.ToLower(arch) {
		case "amd64", "x86_64", "x64":
			if machine == elf.EM_X86_64 {
				return true
			}
		case "386", "i386", "x86":
			if machine == elf.EM_386 {
				return true
			}
		case "arm64", "aarch64":
			if machine == elf.EM_AARCH64 {
				return true
			}
		case "arm", "armv6", "armv7":
			if machine == elf.EM_ARM {
				return true
			}
		}
	}
	return false
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
	return f.OptionalHeader != nil && f.Characteristics&peImageFileDLL == 0 && peMachineCompatible(f.Machine)
}

func peMachineCompatible(machine uint16) bool {
	for _, arch := range resolver.GetArch() {
		switch strings.ToLower(arch) {
		case "amd64", "x86_64", "x64":
			if machine == pe.IMAGE_FILE_MACHINE_AMD64 {
				return true
			}
		case "386", "i386", "x86":
			if machine == pe.IMAGE_FILE_MACHINE_I386 {
				return true
			}
		case "arm", "armv6", "armv7":
			if machine == pe.IMAGE_FILE_MACHINE_ARM {
				return true
			}
		case "arm64", "aarch64":
			if machine == pe.IMAGE_FILE_MACHINE_ARM64 {
				return true
			}
		}
	}
	return false
}

func isWindowsBatchScript(path, name string) bool {
	lower := strings.ToLower(normalizedAssetBasename(name))
	if !strings.HasSuffix(lower, ".bat") && !strings.HasSuffix(lower, ".cmd") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	text := strings.TrimSpace(strings.ToLower(string(buf[:n])))
	return text != "" && !strings.HasPrefix(text, "#!")
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
