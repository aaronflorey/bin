package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func assertConfigFileMode(t *testing.T, configPath string) {
	t.Helper()

	if !supportsConfigFileMode() {
		t.Skip("permission bits are not stable on Windows")
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	if got := info.Mode().Perm(); got != configFileMode {
		t.Fatalf("expected config file mode %04o, got %04o", configFileMode, got)
	}
}

func TestGetArchIncludesAliases(t *testing.T) {
	archs := GetArch()
	contains := func(v string) bool {
		for _, arch := range archs {
			if arch == v {
				return true
			}
		}
		return false
	}

	if !contains(runtime.GOARCH) {
		t.Fatalf("expected GetArch to include runtime arch %s, got %v", runtime.GOARCH, archs)
	}

	if runtime.GOARCH == "amd64" {
		if !contains("x86_64") {
			t.Fatalf("expected amd64 aliases to include x86_64, got %v", archs)
		}
		if !contains("x64") {
			t.Fatalf("expected amd64 aliases to include x64, got %v", archs)
		}
	}

	if runtime.GOARCH == "arm64" && !contains("aarch64") {
		t.Fatalf("expected arm64 aliases to include aarch64, got %v", archs)
	}
}

func resetLibCCache() {
	linuxLibCOnce = sync.Once{}
	linuxLibCCached = nil
}

func TestDetectLinuxLibC(t *testing.T) {
	originalStat := osStat
	originalGlob := globFiles
	defer func() {
		osStat = originalStat
		globFiles = originalGlob
		resetLibCCache()
	}()

	t.Run("alpine prefers musl", func(t *testing.T) {
		resetLibCCache()
		osStat = func(name string) (fs.FileInfo, error) {
			if name == "/etc/alpine-release" {
				return nil, nil
			}
			return nil, errors.New("not found")
		}
		globFiles = func(pattern string) ([]string, error) {
			return nil, errors.New("should not be called")
		}

		if libc := detectLinuxLibC(); len(libc) != 1 || libc[0] != "musl" {
			t.Fatalf("expected musl, got %v", libc)
		}
	})

	t.Run("musl loader marker prefers musl", func(t *testing.T) {
		resetLibCCache()
		osStat = func(name string) (fs.FileInfo, error) {
			return nil, errors.New("not found")
		}
		globFiles = func(pattern string) ([]string, error) {
			if pattern == "/lib/ld-musl*" {
				return []string{"/lib/ld-musl-x86_64.so.1"}, nil
			}
			return nil, nil
		}

		if libc := detectLinuxLibC(); len(libc) != 1 || libc[0] != "musl" {
			t.Fatalf("expected musl, got %v", libc)
		}
	})

	t.Run("default prefers glibc aliases", func(t *testing.T) {
		resetLibCCache()
		osStat = func(name string) (fs.FileInfo, error) {
			return nil, errors.New("not found")
		}
		globFiles = func(pattern string) ([]string, error) {
			return nil, nil
		}

		libc := detectLinuxLibC()
		if len(libc) != 2 || libc[0] != "glibc" || libc[1] != "gnu" {
			t.Fatalf("expected glibc aliases, got %v", libc)
		}
	})
}

func TestCheckAndLoadAllowsFreshBINCONFIGPath(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "nested", "config.json")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	if cfg.DefaultPath != defaultPath {
		t.Fatalf("expected default path %q, got %q", defaultPath, cfg.DefaultPath)
	}
	if cfg.Bins == nil {
		t.Fatal("expected bins map to be initialized")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
	assertConfigFileMode(t, configPath)

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var persisted config
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("expected valid config json, got error: %v", err)
	}
	if persisted.DefaultPath != defaultPath {
		t.Fatalf("expected persisted default path %q, got %q", defaultPath, persisted.DefaultPath)
	}
}

func TestCheckAndLoadDoesNotRewriteExistingConfigWithoutDefaultPath(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)

	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	beforeInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat seeded config file: %v", err)
	}

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	if cfg.DefaultPath != defaultPath {
		t.Fatalf("expected in-memory default path %q, got %q", defaultPath, cfg.DefaultPath)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if got := string(raw); got != "{}\n" {
		t.Fatalf("expected existing config to remain unchanged, got %q", got)
	}

	afterInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file after load: %v", err)
	}

	if supportsConfigFileMode() && afterInfo.Mode().Perm() != beforeInfo.Mode().Perm() {
		t.Fatalf("expected config mode to remain %04o, got %04o", beforeInfo.Mode().Perm(), afterInfo.Mode().Perm())
	}
}

func TestUpsertBinaryPersistsValidConfig(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	binary := &Binary{
		Path:    filepath.Join(defaultPath, "tool"),
		Version: "1.2.3",
		URL:     "https://example.test/tool",
	}
	if err := UpsertBinary(binary); err != nil {
		t.Fatalf("UpsertBinary returned error: %v", err)
	}
	assertConfigFileMode(t, configPath)

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var persisted config
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("expected valid config json, got error: %v", err)
	}
	if got := persisted.Bins[binary.Path]; got == nil || got.Version != binary.Version {
		t.Fatalf("expected persisted binary %+v, got %+v", binary, got)
	}
}

func TestSetRewritesExistingConfigOnExplicitMutation(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	updatedPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)

	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	if err := Set("default_path", updatedPath); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	assertConfigFileMode(t, configPath)

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var persisted config
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("expected valid config json, got error: %v", err)
	}
	if persisted.DefaultPath != updatedPath {
		t.Fatalf("expected persisted default path %q, got %q", updatedPath, persisted.DefaultPath)
	}
}

func TestUpsertBinaryReloadsLatestOnDiskState(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	stale := config{
		DefaultPath: defaultPath,
		Bins: map[string]*Binary{
			filepath.Join(defaultPath, "from-disk"): {
				Path:    filepath.Join(defaultPath, "from-disk"),
				Version: "1.0.0",
				URL:     "https://example.test/from-disk",
			},
		},
	}
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("failed to marshal stale config: %v", err)
	}
	if err := os.WriteFile(configPath, raw, 0o644); err != nil {
		t.Fatalf("failed to overwrite config: %v", err)
	}

	newBinary := &Binary{
		Path:    filepath.Join(defaultPath, "new-binary"),
		Version: "2.0.0",
		URL:     "https://example.test/new-binary",
	}
	if err := UpsertBinary(newBinary); err != nil {
		t.Fatalf("UpsertBinary returned error: %v", err)
	}
	assertConfigFileMode(t, configPath)

	persistedRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var persisted config
	if err := json.Unmarshal(persistedRaw, &persisted); err != nil {
		t.Fatalf("expected valid config json, got error: %v", err)
	}
	if persisted.Bins[filepath.Join(defaultPath, "from-disk")] == nil {
		t.Fatal("expected on-disk binary to be preserved")
	}
	if persisted.Bins[newBinary.Path] == nil {
		t.Fatal("expected new binary to be persisted")
	}
}

func TestCheckAndLoadUsesXDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG config path is Unix-specific")
	}

	t.Cleanup(func() {
		cfg = config{}
	})

	homeDir := t.TempDir()
	xdgDir := filepath.Join(t.TempDir(), "xdg")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(xdgDir, 0o755); err != nil {
		t.Fatalf("failed to create XDG config dir: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	t.Setenv("BIN_EXE_DIR", defaultPath)
	t.Setenv("BIN_CONFIG", "")

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	configPath := filepath.Join(xdgDir, "bin", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file at XDG path: %v", err)
	}
}

func TestGetHooksFiltersByType(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	cfg.Hooks = []RunHook{
		{Type: PreInstall, Command: "pre"},
		{Type: PostInstall, Command: "post"},
	}

	hooks := GetHooks(PreInstall)
	if len(hooks) != 1 {
		t.Fatalf("expected 1 pre-install hook, got %d", len(hooks))
	}
	if hooks[0].Command != "pre" {
		t.Fatalf("unexpected hook command: %q", hooks[0].Command)
	}
}

func TestExecuteHooksReturnsCommandOutputOnFailure(t *testing.T) {
	hooks := []RunHook{{
		Type:    PreInstall,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestConfigHookHelperProcess", "--", "fail"},
	}}

	err := ExecuteHooks(hooks)
	if err == nil {
		t.Fatal("expected ExecuteHooks to fail")
	}
	if !strings.Contains(err.Error(), "hook failure output") {
		t.Fatalf("expected hook output in error, got: %v", err)
	}
}

func TestConfigHookHelperProcess(t *testing.T) {
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			if args[i+1] == "fail" {
				fmt.Fprint(os.Stderr, "hook failure output")
				os.Exit(7)
			}
			break
		}
	}
}
