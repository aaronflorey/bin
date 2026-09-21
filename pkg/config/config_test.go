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

func TestBinaryIntegrityRecordsLoadAndCloneWithoutMigration(t *testing.T) {
	t.Cleanup(func() { cfg = config{} })

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := t.TempDir()
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)
	oldConfig := fmt.Sprintf(`{"default_path":%q,"bins":{"/tmp/old":{"path":"/tmp/old","remote_name":"old","version":"1.0.0","hash":"old-hash","url":"https://example.test/old","provider":"generic"}}}`, defaultPath)
	if err := os.WriteFile(configPath, []byte(oldConfig), 0o600); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	if err := CheckAndLoad(); err != nil {
		t.Fatalf("load old config: %v", err)
	}
	if got := cfg.Bins["/tmp/old"]; got == nil || got.DownloadIntegrity != nil || got.InstalledIntegrity != nil {
		t.Fatalf("old config integrity fields = %#v, want nil", got)
	}

	bin := &Binary{
		Path: "/tmp/new", Hash: "installed-sha", DownloadIntegrity: &IntegrityRecord{Algorithm: "sha256", Scope: "download", Result: "verified"},
		InstalledIntegrity: &IntegrityRecord{Algorithm: "sha256", Observed: "installed-sha", Scope: "installed", Result: "verified"},
	}
	if err := UpsertBinary(bin); err != nil {
		t.Fatalf("upsert binary: %v", err)
	}
	bin.DownloadIntegrity.Result = "changed"
	bin.InstalledIntegrity.Observed = "changed"
	stored := cfg.Bins[bin.Path]
	if stored.DownloadIntegrity.Result != "verified" || stored.InstalledIntegrity.Observed != "installed-sha" {
		t.Fatalf("upsert retained caller-owned integrity pointers: %#v", stored)
	}

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	stored = cfg.Bins[bin.Path]
	if stored.DownloadIntegrity == nil || stored.InstalledIntegrity == nil || stored.InstalledIntegrity.Observed != "installed-sha" {
		t.Fatalf("integrity records did not round trip: %#v", stored)
	}
}

func TestBinarySelectionIntentLoadsClonesAndPersists(t *testing.T) {
	t.Cleanup(func() { cfg = config{} })

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := t.TempDir()
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)
	oldConfig := fmt.Sprintf(`{"default_path":%q,"bins":{"/tmp/old":{"path":"/tmp/old","remote_name":"old","version":"1.0.0","hash":"old-hash","url":"https://example.test/old","provider":"generic"}}}`, defaultPath)
	if err := os.WriteFile(configPath, []byte(oldConfig), 0o600); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	if err := CheckAndLoad(); err != nil {
		t.Fatalf("load old config: %v", err)
	}
	if got := cfg.Bins["/tmp/old"]; got == nil || got.SelectionIntent != nil {
		t.Fatalf("old config selection intent = %#v, want nil", got)
	}

	bin := &Binary{
		Path: "/tmp/new",
		SelectionIntent: &SelectionDescriptor{
			LogicalProduct: "tool",
			Target:         &SelectionTarget{OS: "linux", Architecture: "amd64", ABI: "gnu", CPUVariant: "avx2"},
			ArchiveMember:  "tool/bin/tool",
		},
	}
	clone := CloneBinary(bin)
	clone.SelectionIntent.Target.CPUVariant = "baseline"
	if bin.SelectionIntent.Target.CPUVariant != "avx2" {
		t.Fatalf("clone mutated source selection intent: %#v", bin.SelectionIntent)
	}
	if err := UpsertBinary(bin); err != nil {
		t.Fatalf("upsert binary: %v", err)
	}
	bin.SelectionIntent.ArchiveMember = "changed/tool"
	stored := cfg.Bins[bin.Path]
	if stored.SelectionIntent == nil || stored.SelectionIntent.ArchiveMember != "tool/bin/tool" {
		t.Fatalf("upsert retained caller-owned selection intent: %#v", stored)
	}

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	stored = cfg.Bins[bin.Path]
	if stored.SelectionIntent == nil || stored.SelectionIntent.LogicalProduct != "tool" ||
		stored.SelectionIntent.Target == nil || stored.SelectionIntent.Target.OS != "linux" ||
		stored.SelectionIntent.Target.Architecture != "amd64" || stored.SelectionIntent.Target.ABI != "gnu" ||
		stored.SelectionIntent.Target.CPUVariant != "avx2" ||
		stored.SelectionIntent.ArchiveMember != "tool/bin/tool" {
		t.Fatalf("selection intent did not round trip: %#v", stored)
	}
}

func TestCompletionOwnershipClonesAndSurvivesOrdinaryUpdates(t *testing.T) {
	configPath, binDir := setupTransactionConfig(t)
	path := filepath.Join(binDir, "tool")
	shell := "bash"
	ownership := map[string]*CompletionOwnershipRecord{
		"bash": {Path: filepath.Join(filepath.Dir(configPath), "completions", "bash", "tool"), SHA256: "bash-hash"},
		"zsh":  {Path: filepath.Join(filepath.Dir(configPath), "completions", "zsh", "_tool"), SHA256: "zsh-hash"},
	}
	initial := &Binary{Path: path, Version: "1.0.0", CompletionShell: &shell, CompletionOwnership: ownership}
	clone := CloneBinary(initial)
	*clone.CompletionShell = "fish"
	clone.CompletionOwnership["bash"].SHA256 = "changed"
	delete(clone.CompletionOwnership, "zsh")
	if *initial.CompletionShell != "bash" || initial.CompletionOwnership["bash"].SHA256 != "bash-hash" || initial.CompletionOwnership["zsh"] == nil {
		t.Fatalf("clone mutated source completion settings: %#v", initial)
	}
	if err := UpsertBinary(initial); err != nil {
		t.Fatalf("seed binary: %v", err)
	}

	if err := UpsertBinary(&Binary{Path: path, Version: "2.0.0"}); err != nil {
		t.Fatalf("update binary: %v", err)
	}
	if got := cfg.Bins[path]; got == nil || got.Version != "2.0.0" || got.CompletionShell == nil || *got.CompletionShell != "bash" || got.CompletionOwnership["bash"].SHA256 != "bash-hash" || got.CompletionOwnership["zsh"].SHA256 != "zsh-hash" {
		t.Fatalf("upsert discarded local completion settings: %#v", got)
	}

	if err := CommitBinaryTransaction(BinaryTransaction{
		ID: "completion-owner", Intended: &Binary{Path: path, Version: "3.0.0"},
		Publish:  func(*Binary) error { return nil },
		Rollback: func(*Binary) error { return nil },
	}); err != nil {
		t.Fatalf("transaction update: %v", err)
	}
	if got := cfg.Bins[path]; got == nil || got.Version != "3.0.0" || got.CompletionShell == nil || *got.CompletionShell != "bash" || got.CompletionOwnership["bash"].SHA256 != "bash-hash" || got.CompletionOwnership["zsh"].SHA256 != "zsh-hash" {
		t.Fatalf("transaction discarded local completion settings: %#v", got)
	}

	off := ""
	if err := UpsertBinary(&Binary{Path: path, Version: "4.0.0", CompletionShell: &off}); err != nil {
		t.Fatalf("disable automatic completions: %v", err)
	}
	if got := cfg.Bins[path]; got == nil || got.CompletionShell != nil {
		t.Fatalf("explicit off did not clear local completion shell: %#v", got)
	}

	cfg = config{}
	if err := CheckAndLoad(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := cfg.Bins[path]; got == nil || got.CompletionShell != nil || got.CompletionOwnership["bash"].Path != ownership["bash"].Path || got.CompletionOwnership["zsh"].SHA256 != "zsh-hash" {
		t.Fatalf("local completion settings did not persist: %#v", got)
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

func TestCheckAndLoadDefaultsUseGHAuthToTrue(t *testing.T) {
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

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	if !cfg.UseGHAuth {
		t.Fatalf("expected UseGHAuth to default to true when key is absent, got false")
	}
}

func TestCheckAndLoadHonorsExplicitUseGHAuthFalse(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)

	if err := os.WriteFile(configPath, []byte(`{"use_gh_for_github_token": false}`+"\n"), 0o644); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	if cfg.UseGHAuth {
		t.Fatalf("expected UseGHAuth to stay false when explicitly set, got true")
	}
}

func TestCheckAndLoadHonorsExplicitUseGHAuthTrue(t *testing.T) {
	t.Cleanup(func() {
		cfg = config{}
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	defaultPath := filepath.Join(t.TempDir(), "bin")
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", defaultPath)

	if err := os.WriteFile(configPath, []byte(`{"use_gh_for_github_token": true}`+"\n"), 0o644); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad returned error: %v", err)
	}

	if !cfg.UseGHAuth {
		t.Fatalf("expected UseGHAuth to be true when explicitly set, got false")
	}
}

func setupTransactionConfig(t *testing.T) (string, string) {
	t.Helper()

	cfg = config{}
	t.Cleanup(func() { cfg = config{} })
	configPath := filepath.Join(t.TempDir(), "config.json")
	binDir := t.TempDir()
	t.Setenv("BIN_CONFIG", configPath)
	t.Setenv("BIN_EXE_DIR", binDir)
	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad: %v", err)
	}
	return configPath, binDir
}

func TestCommitBinaryTransactionReloadsAndMergesCurrentConfig(t *testing.T) {
	configPath, binDir := setupTransactionConfig(t)
	diskBinary := &Binary{Path: filepath.Join(binDir, "from-disk"), Version: "1.0.0"}
	seed := config{DefaultPath: binDir, Bins: map[string]*Binary{diskBinary.Path: diskBinary}}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal seed config: %v", err)
	}
	if err := os.WriteFile(configPath, raw, configFileMode); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	intended := &Binary{Path: filepath.Join(binDir, "new"), Version: "2.0.0"}
	if err := CommitBinaryTransaction(BinaryTransaction{
		ID: "transaction-new", Intended: intended,
		Publish: func(previous *Binary) error {
			if previous != nil {
				t.Fatalf("publish previous = %#v, want nil", previous)
			}
			return nil
		},
		Rollback: func(*Binary) error { return nil },
	}); err != nil {
		t.Fatalf("CommitBinaryTransaction: %v", err)
	}

	var persisted config
	persistedRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if err := json.Unmarshal(persistedRaw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	if persisted.Bins[diskBinary.Path] == nil || persisted.Bins[intended.Path] == nil {
		t.Fatalf("transaction discarded unrelated state: %#v", persisted.Bins)
	}
	if got, err := GetBinary(intended.Path); err != nil || got == nil || got.Version != intended.Version {
		t.Fatalf("in-memory config not synchronized: binary=%#v err=%v", got, err)
	}
}

func TestCommitBinaryTransactionKeepsConcurrentDifferentBinaries(t *testing.T) {
	_, binDir := setupTransactionConfig(t)
	paths := []string{filepath.Join(binDir, "one"), filepath.Join(binDir, "two")}
	errs := make(chan error, len(paths))
	var group sync.WaitGroup
	for _, path := range paths {
		group.Add(1)
		go func(path string) {
			defer group.Done()
			errs <- CommitBinaryTransaction(BinaryTransaction{
				ID: filepath.Base(path), Intended: &Binary{Path: path, Version: "1.0.0"},
				Publish:  func(*Binary) error { return nil },
				Rollback: func(*Binary) error { return nil },
			})
		}(path)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent transaction: %v", err)
		}
	}
	for _, path := range paths {
		if got, err := GetBinary(path); err != nil || got == nil {
			t.Fatalf("transaction for %q missing: binary=%#v err=%v", path, got, err)
		}
	}
}

func TestCommitBinaryTransactionSerializesSameBinaryPublication(t *testing.T) {
	_, binDir := setupTransactionConfig(t)
	path := filepath.Join(binDir, "tool")
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	errs := make(chan error, 2)
	for _, version := range []string{"1.0.0", "2.0.0"} {
		go func(version string) {
			errs <- CommitBinaryTransaction(BinaryTransaction{
				ID: version, Intended: &Binary{Path: path, Version: version},
				Publish: func(*Binary) error {
					entered <- struct{}{}
					<-release
					return nil
				},
				Rollback: func(*Binary) error { return nil },
			})
		}(version)
	}
	<-entered
	select {
	case <-entered:
		t.Fatal("same-path publication callbacks ran concurrently")
	default:
	}
	close(release)
	<-entered
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("same-path transaction: %v", err)
		}
	}
}

func TestCommitBinaryTransactionConfigFailureRestoresPublishedExecutableAndRecord(t *testing.T) {
	_, binDir := setupTransactionConfig(t)
	path := filepath.Join(binDir, "tool")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := &Binary{Path: path, Version: "1.0.0", Hash: "old-hash"}
	if err := UpsertBinary(previous); err != nil {
		t.Fatal(err)
	}
	backup := path + ".rollback"
	originalWrite := writeConfig
	writeConfig = func(string, config) error { return errors.New("config write failed") }
	t.Cleanup(func() { writeConfig = originalWrite })

	err := CommitBinaryTransaction(BinaryTransaction{
		ID: "restore-old", Intended: &Binary{Path: path, Version: "2.0.0", Hash: "new-hash"},
		Publish: func(*Binary) error {
			if err := os.Rename(path, backup); err != nil {
				return err
			}
			return os.WriteFile(path, []byte("new"), 0o755)
		},
		Rollback: func(*Binary) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Rename(backup, path)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "config write failed") {
		t.Fatalf("transaction error = %v, want config write failure", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "old" {
		t.Fatalf("executable after rollback = %q, err=%v", contents, err)
	}
	stored, err := GetBinary(path)
	if err != nil || stored == nil || stored.Version != previous.Version || stored.Hash != previous.Hash {
		t.Fatalf("record after rollback = %#v, err=%v", stored, err)
	}
}

func TestUnresolvedBinaryTransactionBlocksSameKeyAndPersists(t *testing.T) {
	configPath, binDir := setupTransactionConfig(t)
	path := filepath.Join(binDir, "tool")
	writeErr := errors.New("write failed")
	originalWrite := writeConfig
	writes := 0
	writeConfig = func(path string, current config) error {
		writes++
		if writes == 1 {
			return writeErr
		}
		return originalWrite(path, current)
	}
	t.Cleanup(func() { writeConfig = originalWrite })

	err := CommitBinaryTransaction(BinaryTransaction{
		ID: "owner-a", Intended: &Binary{Path: path, Version: "2.0.0"},
		Publish:          func(*Binary) error { return nil },
		Rollback:         func(*Binary) error { return errors.New("rollback failed") },
		RollbackArtifact: func() string { return filepath.Join(binDir, "tool.rollback-owner-a") },
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("transaction error = %v, want config write failure", err)
	}
	if _, err := GetBinary(path); !errors.Is(err, ErrBinaryRecoveryRequired) {
		t.Fatalf("GetBinary error = %v, want unresolved state", err)
	}

	called := false
	err = CommitBinaryTransaction(BinaryTransaction{
		ID: "owner-b", Intended: &Binary{Path: path, Version: "3.0.0"},
		Publish:  func(*Binary) error { called = true; return nil },
		Rollback: func(*Binary) error { return nil },
	})
	if !errors.Is(err, ErrBinaryRecoveryRequired) || called {
		t.Fatalf("same-key transaction error=%v publish-called=%t", err, called)
	}

	persistedRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var persisted config
	if err := json.Unmarshal(persistedRaw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	if state := persisted.UnresolvedTransactions[path]; state == nil || state.ID != "owner-a" || state.ArtifactPath == "" || state.Intended.Version != "2.0.0" {
		t.Fatalf("persisted unresolved state = %#v", state)
	}
	cfg = config{}
	if err := CheckAndLoad(); err != nil {
		t.Fatalf("CheckAndLoad must leave reconciliation available: %v", err)
	}
	if _, err := GetBinary(path); !errors.Is(err, ErrBinaryRecoveryRequired) {
		t.Fatalf("reloaded unresolved record error = %v", err)
	}
}

func TestRecoverBinaryTransactionUsesOwnedHashEvidence(t *testing.T) {
	configPath, binDir := setupTransactionConfig(t)
	path := filepath.Join(binDir, "tool")
	backup := path + ".rollback-owner"
	if err := os.WriteFile(path, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldHash, _, err := transactionFileHash(backup)
	if err != nil {
		t.Fatal(err)
	}
	newHash, _, err := transactionFileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	previous := &Binary{Path: path, Version: "1.0.0", Hash: oldHash}
	intended := &Binary{Path: path, Version: "2.0.0", Hash: newHash}
	loaded := config{DefaultPath: binDir, Bins: map[string]*Binary{path: previous}, UnresolvedTransactions: map[string]*UnresolvedBinaryTransaction{path: {ID: "owner", DestinationPath: path, ArtifactPath: backup, Previous: previous, Intended: intended}}}
	if err := writeConfig(configPath, loaded); err != nil {
		t.Fatal(err)
	}
	if err := writeTransactionJournal(configPath, path, loaded.UnresolvedTransactions[path], transactionJournalIntent); err != nil {
		t.Fatal(err)
	}
	cfg = config{}
	if err := CheckAndLoad(); err != nil {
		t.Fatal(err)
	}
	if err := RecoverBinaryTransaction(path); err != nil {
		t.Fatal(err)
	}
	cfg = config{}
	if err := CheckAndLoad(); err != nil {
		t.Fatal(err)
	}
	if got, err := GetBinary(path); err != nil || got == nil || got.Version != intended.Version {
		t.Fatalf("recovered binary = %#v, err=%v", got, err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("owned backup remains: %v", err)
	}
}

func TestRecoverBinaryTransactionRestoresPreviousOrRemovesUnpublishedNewInstall(t *testing.T) {
	t.Run("restore previous backup", func(t *testing.T) {
		configPath, binDir := setupTransactionConfig(t)
		path := filepath.Join(binDir, "tool")
		backup := path + ".rollback-owner"
		if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		oldHash, _, err := transactionFileHash(backup)
		if err != nil {
			t.Fatal(err)
		}
		previous := &Binary{Path: path, Version: "1.0.0", Hash: oldHash}
		seedRecoveryTransaction(t, configPath, binDir, path, &UnresolvedBinaryTransaction{ID: "owner", DestinationPath: path, ArtifactPath: backup, Previous: previous, Intended: &Binary{Path: path, Version: "2.0.0", Hash: "new-hash"}}, previous)
		if err := RecoverBinaryTransaction(path); err != nil {
			t.Fatal(err)
		}
		cfg = config{}
		if err := CheckAndLoad(); err != nil {
			t.Fatal(err)
		}
		if got, err := GetBinary(path); err != nil || got == nil || got.Version != previous.Version {
			t.Fatalf("recovered record = %#v err=%v", got, err)
		}
		if _, err := os.Stat(backup); !os.IsNotExist(err) {
			t.Fatalf("owned backup remains: %v", err)
		}
	})

	t.Run("new install was never published", func(t *testing.T) {
		configPath, binDir := setupTransactionConfig(t)
		path := filepath.Join(binDir, "tool")
		unresolved := &UnresolvedBinaryTransaction{ID: "owner", DestinationPath: path, Intended: &Binary{Path: path, Version: "1.0.0", Hash: "new-hash"}}
		seedRecoveryTransaction(t, configPath, binDir, path, unresolved, nil)
		if err := RecoverBinaryTransaction(path); err != nil {
			t.Fatal(err)
		}
		cfg = config{}
		if err := CheckAndLoad(); err != nil {
			t.Fatal(err)
		}
		if got, err := GetBinary(path); err != nil || got != nil {
			t.Fatalf("unpublished new install persisted: %#v err=%v", got, err)
		}
		if _, ok := cfg.UnresolvedTransactions[path]; ok {
			t.Fatal("unpublished new install remained unresolved")
		}
	})
}

func seedRecoveryTransaction(t *testing.T, configPath, binDir, path string, unresolved *UnresolvedBinaryTransaction, current *Binary) {
	t.Helper()
	loaded := config{DefaultPath: binDir, Bins: map[string]*Binary{}, UnresolvedTransactions: map[string]*UnresolvedBinaryTransaction{path: unresolved}}
	if current != nil {
		loaded.Bins[path] = current
	}
	if err := writeConfig(configPath, loaded); err != nil {
		t.Fatal(err)
	}
	if err := writeTransactionJournal(configPath, path, unresolved, transactionJournalIntent); err != nil {
		t.Fatal(err)
	}
	cfg = config{}
	if err := CheckAndLoad(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitBinaryTransactionCleanupFailureKeepsRecoveryState(t *testing.T) {
	configPath, binDir := setupTransactionConfig(t)
	path := filepath.Join(binDir, "tool")
	intended := &Binary{Path: path, Version: "2.0.0"}
	transactionID := "intent-owner"
	backup := path + ".rollback-" + transactionID
	err := CommitBinaryTransaction(BinaryTransaction{
		ID: transactionID, Intended: intended, DestinationPath: path,
		ReserveRollbackArtifact: func() (string, error) { return backup, nil },
		Publish: func(*Binary) error {
			if _, err := os.Stat(transactionJournalPath(configPath, transactionID)); err != nil {
				t.Fatalf("publish started without durable intent journal: %v", err)
			}
			data, err := os.ReadFile(transactionJournalPath(configPath, transactionID))
			if err != nil {
				t.Fatal(err)
			}
			var journal transactionJournal
			if err := json.Unmarshal(data, &journal); err != nil || journal.Unresolved.ArtifactPath != backup {
				t.Fatalf("intent journal did not reserve owned backup: %#v err=%v", journal, err)
			}
			return nil
		},
		Rollback: func(*Binary) error { return nil },
		Cleanup:  func() error { return errors.New("backup cleanup failed") },
	})
	if err == nil || !strings.Contains(err.Error(), "backup cleanup failed") {
		t.Fatalf("commit error = %v, want cleanup failure", err)
	}
	if _, err := os.Stat(transactionJournalPath(configPath, transactionID)); err != nil {
		t.Fatalf("cleanup failure unexpectedly removed journal: %v", err)
	}
	cfg = config{}
	if err := CheckAndLoad(); err != nil {
		t.Fatal(err)
	}
	if _, err := GetBinary(path); !errors.Is(err, ErrBinaryRecoveryRequired) {
		t.Fatalf("cleanup failure left ordinary state usable: %v", err)
	}
}

func TestCommitBinaryTransactionRejectsUnsafeJournalOwner(t *testing.T) {
	_, binDir := setupTransactionConfig(t)
	err := CommitBinaryTransaction(BinaryTransaction{
		ID:       "../outside",
		Intended: &Binary{Path: filepath.Join(binDir, "tool")},
		Publish:  func(*Binary) error { return nil },
		Rollback: func(*Binary) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "invalid binary transaction owner") {
		t.Fatalf("transaction error = %v, want unsafe owner rejection", err)
	}
}
