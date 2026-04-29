package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aaronflorey/bin/pkg/options"
	"github.com/caarlos0/log"
)

var cfg config

var ErrInvalidConfigKey = errors.New("invalid config key")

var (
	osStat    = os.Stat
	globFiles = filepath.Glob
	cfgMu     sync.Mutex

	linuxLibCOnce   sync.Once
	linuxLibCCached []string
)

type config struct {
	// DefaultPath might not be expanded so it's important that
	// the caller expands this variable with os.ExpandEnv(string)
	// if necessary
	DefaultPath  string             `json:"default_path"`
	DefaultChmod string             `json:"default_chmod,omitempty"`
	UseGHAuth    bool               `json:"use_gh_for_github_token,omitempty"`
	Bins         map[string]*Binary `json:"bins"`
	Hooks        []RunHook          `json:"hooks,omitempty"`
}

// HookType represents lifecycle hook event names.
type HookType string

const (
	// PreInstall is triggered before an installation begins.
	PreInstall HookType = "pre-install"
	// PostInstall is triggered after an installation completes.
	PostInstall HookType = "post-install"
	// PreUpdate is triggered before an update begins.
	PreUpdate HookType = "pre-update"
	// PostUpdate is triggered after an update completes.
	PostUpdate HookType = "post-update"
	// PreRemove is triggered before a removal begins.
	PreRemove HookType = "pre-remove"
	// PostRemove is triggered after a removal completes.
	PostRemove HookType = "post-remove"
)

// RunHook defines a shell command to run at a specific lifecycle event.
type RunHook struct {
	Type    HookType `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// GetHooks returns all configured hooks matching the given HookType.
func GetHooks(t HookType) []RunHook {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	hooks := make([]RunHook, 0)
	for _, hook := range cfg.Hooks {
		if hook.Type == t {
			hooks = append(hooks, hook)
		}
	}
	return hooks
}

// ExecuteHooks runs each hook in sequence. If any hook fails, execution
// stops and the error is returned to the caller.
func ExecuteHooks(hooks []RunHook) error {
	for _, hook := range hooks {
		if hook.Command == "" {
			continue
		}
		log.Infof("Executing %s hook: %s %v", hook.Type, hook.Command, hook.Args)
		output, err := exec.Command(hook.Command, hook.Args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hook %s failed: %v — output: %s", hook.Command, err, string(output))
		}
		log.Debugf("Hook %s completed successfully", hook.Command)
	}
	return nil
}

type Binary struct {
	Path       string `json:"path"`
	RemoteName string `json:"remote_name"`
	Version    string `json:"version"`
	Hash       string `json:"hash"`
	URL        string `json:"url"`
	Provider   string `json:"provider"`
	// InstallMode indicates whether this binary is managed as a direct
	// downloaded executable ("binary") or a system package ("system-package").
	InstallMode string `json:"install_mode,omitempty"`
	// PackageType stores the selected package artifact type for system-package
	// installs (deb, rpm, apk, flatpak, dmg).
	PackageType string `json:"package_type,omitempty"`
	// AppBundle stores the installed macOS app bundle name for dmg-backed app
	// installs so lifecycle commands can verify and remove the app correctly.
	AppBundle string `json:"app_bundle,omitempty"`
	// if file is installed from a package format (zip, tar, etc) store
	// the package path in config so we don't ask the user to select
	// the path again when upgrading
	PackagePath string `json:"package_path"`
	Pinned      bool   `json:"pinned"`
	MinAgeDays  int    `json:"min_age_days,omitempty"`
}

func CheckAndLoad() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	confDir := filepath.Dir(configPath)

	if err := os.MkdirAll(confDir, 0755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating config directory [%v]", err)
	}

	log.Debugf("Config directory is: %s", confDir)
	f, err := os.OpenFile(configPath, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	defer f.Close()

	cfg = config{}

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		if err == io.EOF {
			// Empty file and/or was just created, initialize cfg.Bins
			cfg.Bins = map[string]*Binary{}
		} else {
			return err
		}
	}

	if len(cfg.DefaultPath) == 0 {
		if exeDir := ForceInstallationDir(); len(exeDir) > 0 {
			cfg.DefaultPath = exeDir
		} else {
			cfg.DefaultPath, err = getDefaultPath()
			if err != nil {
				for {
					log.Info("Could not find a PATH directory automatically, falling back to manual selection")
					reader := bufio.NewReader(os.Stdin)
					var response string
					fmt.Printf("\nPlease specify a download directory: ")
					response, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("Invalid input")
					}
					response = strings.TrimSpace(response)

					if err = checkDirExistsAndWritable(response); err != nil {
						log.Debugf("Could not set download directory [%s]: [%v]", response, err)
						// Keep looping until writable and existing dir is selected
						continue
					}

					cfg.DefaultPath = response
					break
				}
			}
		}

		if err := writeLocked(); err != nil {
			return err
		}

	}

	if cfg.Bins == nil {
		cfg.Bins = map[string]*Binary{}
	}

	if runtime.GOOS == "linux" && len(cfg.DefaultChmod) == 0 {
		cfg.DefaultChmod = "0755"
	}

	log.Debugf("Download path set to %s", cfg.DefaultPath)
	return nil
}

func Get() *config {
	return &cfg
}

func ValidKeys() []string {
	keys := []string{"default_path", "use_gh_for_github_token"}
	sort.Strings(keys)
	return keys
}

func Set(key, value string) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	switch key {
	case "default_path":
		cfg.DefaultPath = value
	case "use_gh_for_github_token":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value %q for %s", value, key)
		}
		cfg.UseGHAuth = parsed
	default:
		return fmt.Errorf("%w: %s", ErrInvalidConfigKey, key)
	}

	return writeLocked()
}

// ForceInstallationDir returns the directory specified by the BIN_EXE_DIR
// environment variable, creating it if necessary. Returns an empty string
// if the variable is unset or the directory cannot be created.
func ForceInstallationDir() string {
	exeDir := os.Getenv("BIN_EXE_DIR")
	if len(exeDir) == 0 {
		return ""
	}
	if err := os.MkdirAll(exeDir, 0755); err != nil && !os.IsExist(err) {
		log.Debugf("Could not create BIN_EXE_DIR %s: %v", exeDir, err)
		return ""
	}
	return exeDir
}

func selectWritablePathFromEnv(pathEnv, separator string) (string, error) {
	log.Debugf("User PATH is [%s]", pathEnv)
	opts := map[fmt.Stringer]struct{}{}

	for _, path := range strings.Split(pathEnv, separator) {
		log.Debugf("Checking path %s", path)
		if err := checkDirExistsAndWritable(path); err != nil {
			log.Debugf("Error [%s] checking path", err)
			continue
		}

		log.Debugf("%s seems to be a dir and writable, adding option.", path)
		opts[options.LiteralStringer(path)] = struct{}{}
	}

	if len(opts) == 0 {
		return "", errors.New("Automatic path detection didn't return any results")
	}

	sopts := make([]fmt.Stringer, 0, len(opts))
	for option := range opts {
		sopts = append(sopts, option)
	}

	choice, err := options.SelectCustom("Pick a default download dir: ", sopts)
	if err != nil {
		return "", err
	}

	return choice.(fmt.Stringer).String(), nil
}

func checkDirWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".bin-write-check-*")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	return os.Remove(probePath)
}

// UpsertBinary adds or updates an existing
// binary resource in the config
func UpsertBinary(c *Binary) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	if c != nil {
		cfg.Bins[c.Path] = c
		err := writeLocked()
		if err != nil {
			return err
		}
	}

	return nil
}

// UpsertBinaries adds or updates multiple binary resources
// in the config, writing to disk once.
func UpsertBinaries(binaries []*Binary) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	for _, c := range binaries {
		if c != nil {
			cfg.Bins[c.Path] = c
		}
	}
	return writeLocked()
}

// RemoveBinaries removes the specified paths
// from bin configuration. It doesn't care about the order
func RemoveBinaries(paths []string) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	for _, p := range paths {
		delete(cfg.Bins, p)
	}

	return writeLocked()
}

func write() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	return writeLocked()
}

func writeLocked() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	f, err := os.CreateTemp(configDir, filepath.Base(configPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := f.Name()

	defer func() {
		_ = f.Close()
		_ = os.Remove(tempPath)
	}()

	decoder := json.NewEncoder(f)
	decoder.SetIndent("", "    ")
	err = decoder.Encode(cfg)
	if err != nil {
		return err
	}

	if err := f.Chmod(0664); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		return err
	}

	return nil
}

// GetArch returns the running program's architecture target
// and common aliases (e.g. aarch64 for arm64).
func GetArch() []string {
	res := []string{runtime.GOARCH}
	switch runtime.GOARCH {
	case "amd64":
		// Adding x86_64 manually since the uname syscall (man 2 uname)
		// is not implemented in all systems
		res = append(res, "x86_64")
		res = append(res, "x64")
	case "arm64":
		// Many release assets (especially on macOS) use aarch64
		res = append(res, "aarch64")
	}
	return res
}

// GetOS returns the running program's operating system target
// and common aliases (e.g. macos for darwin).
func GetOS() []string {
	res := []string{runtime.GOOS}
	switch runtime.GOOS {
	case "darwin":
		// Many release assets use macos or osx instead of darwin
		res = append(res, "macos", "osx")
	case "windows":
		// Adding win since some repositories release with that as the indicator of a windows binary
		res = append(res, "win")
	}
	return res
}

// GetLibC returns Linux libc preference aliases for release asset matching.
// Non-Linux platforms do not expose libc aliases.
func GetLibC() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	linuxLibCOnce.Do(func() {
		linuxLibCCached = detectLinuxLibC()
	})
	return linuxLibCCached
}

func detectLinuxLibC() []string {
	if _, err := osStat("/etc/alpine-release"); err == nil {
		return []string{"musl"}
	}

	for _, pattern := range []string{"/lib/ld-musl*", "/lib64/ld-musl*"} {
		matches, err := globFiles(pattern)
		if err == nil && len(matches) > 0 {
			return []string{"musl"}
		}
	}

	log.Debugf("No musl markers found, defaulting to glibc")
	return []string{"glibc", "gnu"}
}

// getConfigPath returns the path to the configuration directory respecting
// the `XDG Base Directory specification` using the following strategy:
//   - honor BIN_CONFIG is set
//   - to prevent breaking of existing configurations, check if "$HOME/.bin/config.json"
//     exists and return "$HOME/.bin"
//   - if "XDG_CONFIG_HOME" is set, return "$XDG_CONFIG_HOME/bin"
//   - if "$HOME/.config" exists, return "$home/.config/bin"
//   - default to "$HOME/.bin/"
//
// ToDo: move the function to config_unix.go and add a similar function for windows,
//
//	%APPDATA% might be the right place on windows
func getConfigPath() (string, error) {

	c := os.Getenv("BIN_CONFIG")
	if len(c) > 0 {
		if _, err := os.Stat(c); err == nil || os.IsNotExist(err) {
			return c, nil
		} else {
			return "", err
		}
	}

	home, homeErr := os.UserHomeDir()
	if homeErr == nil {
		if _, err := os.Stat(filepath.Join(home, ".bin", "config.json")); !os.IsNotExist(err) {
			return filepath.Join(path.Join(home, ".bin", "config.json")), nil
		}
	}

	c = os.Getenv("XDG_CONFIG_HOME")
	if _, err := os.Stat(c); !os.IsNotExist(err) {
		return filepath.Join(c, "bin", "config.json"), nil
	}

	if homeErr != nil {
		return "", homeErr
	}
	c = filepath.Join(home, ".config")
	if _, err := os.Stat(c); !os.IsNotExist(err) {
		return filepath.Join(c, "bin", "config.json"), nil
	}

	return filepath.Join(home, ".bin", "config.json"), nil
}

func GetOSSpecificExtensions() []string {
	switch runtime.GOOS {
	case "linux":
		return []string{"AppImage"}
	case "windows":
		return []string{"exe"}
	default:
		return nil
	}
}
