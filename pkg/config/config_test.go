package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

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
