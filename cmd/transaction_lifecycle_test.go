package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
)

func setupUnresolvedDirectBinary(t *testing.T) string {
	t.Helper()
	installDir := setupTestConfig(t)
	path := filepath.Join(installDir, "tool")
	configPath := os.Getenv("BIN_CONFIG")
	state := map[string]any{
		"default_path": installDir,
		"bins": map[string]*config.Binary{path: {
			Path: path, RemoteName: "tool", Version: "1.0.0", URL: "https://example.test/tool", Provider: "test", InstallMode: installModeBinary,
		}},
		"unresolved_transactions": map[string]any{path: map[string]any{
			"id":       "transaction-owner",
			"intended": map[string]any{"path": path, "version": "2.0.0"},
		}},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.CheckAndLoad(); err != nil {
		t.Fatal(err)
	}
	// A failing hook makes it observable that lifecycle validation runs first.
	config.Get().Hooks = []config.RunHook{{Type: config.PreInstall, Command: "false"}, {Type: config.PreUpdate, Command: "false"}, {Type: config.PreRemove, Command: "false"}}
	return path
}

func TestUnresolvedDirectTransactionBlocksLifecycleBeforeHooksOrMutation(t *testing.T) {
	t.Run("install", func(t *testing.T) {
		setupUnresolvedDirectBinary(t)
		command := newInstallCmd().cmd
		command.SetArgs([]string{"https://example.test/tool"})
		if err := command.Execute(); !errors.Is(err, config.ErrBinaryRecoveryRequired) {
			t.Fatalf("install error = %v, want recovery error before pre-install hook", err)
		}
	})

	t.Run("unrelated install", func(t *testing.T) {
		setupUnresolvedDirectBinary(t)
		command := newInstallCmd().cmd
		command.SetArgs([]string{"https://example.test/other"})
		if err := command.Execute(); err == nil || errors.Is(err, config.ErrBinaryRecoveryRequired) {
			t.Fatalf("unrelated install error = %v, want its pre-install hook error", err)
		}
	})

	t.Run("different URL at unresolved destination", func(t *testing.T) {
		path := setupUnresolvedDirectBinary(t)
		previousFactory := installProviderFactory
		called := false
		installProviderFactory = func(string, string) (providers.Provider, error) {
			called = true
			return nil, errors.New("provider must not run")
		}
		t.Cleanup(func() { installProviderFactory = previousFactory })
		command := newInstallCmd().cmd
		command.SetArgs([]string{"https://example.test/other", path})
		if err := command.Execute(); !errors.Is(err, config.ErrBinaryRecoveryRequired) {
			t.Fatalf("same destination error = %v, want recovery error", err)
		}
		if called {
			t.Fatal("same destination reached provider or staging")
		}
	})

	t.Run("ensure", func(t *testing.T) {
		path := setupUnresolvedDirectBinary(t)
		if err := runEnsure(nil); !errors.Is(err, config.ErrBinaryRecoveryRequired) {
			t.Fatalf("ensure error = %v, want recovery error", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("ensure mutated unresolved executable: %v", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		setupUnresolvedDirectBinary(t)
		root := newUpdateCmd()
		root.opts.yesToUpdate = true
		root.cmd.SetArgs(nil)
		if err := root.cmd.Execute(); err == nil {
			t.Fatal("update succeeded despite unresolved transaction")
		}
	})

	t.Run("remove", func(t *testing.T) {
		path := setupUnresolvedDirectBinary(t)
		if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		command := newRemoveCmd().cmd
		command.SetArgs([]string{path})
		if err := command.Execute(); !errors.Is(err, config.ErrBinaryRecoveryRequired) {
			t.Fatalf("remove error = %v, want recovery error before pre-remove hook", err)
		}
		if contents, err := os.ReadFile(path); err != nil || string(contents) != "old" {
			t.Fatalf("remove changed unresolved executable = %q, err=%v", contents, err)
		}
	})
}

func TestUnresolvedDirectTransactionDoesNotBlockSystemPackageRemoval(t *testing.T) {
	installDir := setupTestConfig(t)
	directPath := filepath.Join(installDir, "direct")
	systemPath := filepath.Join(installDir, "system")
	config.Get().Bins[directPath] = &config.Binary{Path: directPath, InstallMode: installModeBinary}
	config.Get().Bins[systemPath] = &config.Binary{Path: systemPath, InstallMode: installModeSystemPackage, PackageType: "flatpak"}
	config.Get().UnresolvedTransactions[directPath] = &config.UnresolvedBinaryTransaction{ID: "owner", Intended: &config.Binary{Path: directPath}}

	if err := ensureDirectBinariesResolved(map[string]*config.Binary{systemPath: config.Get().Bins[systemPath]}); err != nil {
		t.Fatalf("system package lifecycle was blocked by unrelated direct transaction: %v", err)
	}
}
