package config

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aaronflorey/bin/pkg/options"
	"github.com/caarlos0/log"
)

var cfg config

var (
	ErrInvalidConfigKey             = errors.New("invalid config key")
	ErrBinaryRecoveryRequired       = errors.New("binary has unresolved transaction state")
	ErrTransactionOwnershipMismatch = errors.New("transaction does not own unresolved state")
)

const configFileMode os.FileMode = 0o600

func supportsConfigFileMode() bool {
	return runtime.GOOS != "windows"
}

var (
	osStat      = os.Stat
	globFiles   = filepath.Glob
	writeConfig = writeConfigLocked
	cfgMu       sync.Mutex

	linuxLibCOnce   sync.Once
	linuxLibCCached []string
)

type config struct {
	// DefaultPath might not be expanded so it's important that
	// the caller expands this variable with os.ExpandEnv(string)
	// if necessary
	DefaultPath  string `json:"default_path"`
	DefaultChmod string `json:"default_chmod,omitempty"`
	UseGHAuth    bool   `json:"use_gh_for_github_token,omitempty"`
	// useGHAuthExplicit tracks whether use_gh_for_github_token was present in
	// the JSON so we can default absent values to true while honoring an
	// explicit false. It is not serialized.
	useGHAuthExplicit      bool                                    `json:"-"`
	Bins                   map[string]*Binary                      `json:"bins"`
	UnresolvedTransactions map[string]*UnresolvedBinaryTransaction `json:"unresolved_transactions,omitempty"`
	Hooks                  []RunHook                               `json:"hooks,omitempty"`
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
	// SourceAsset stores the raw outer release asset selected for this binary.
	SourceAsset string `json:"source_asset,omitempty"`
	// SelectionIntent records the portable, version-independent choices used to
	// select a release product. SourceAsset remains provenance for the bytes
	// that were installed.
	SelectionIntent *SelectionDescriptor `json:"selection_intent,omitempty"`
	// ReleaseTagPrefix keeps the exact tag lane prefix for multi-track repos.
	ReleaseTagPrefix string `json:"release_tag_prefix,omitempty"`
	// DownloadIntegrity describes verification of the raw release artifact;
	// InstalledIntegrity describes verification of the installed file bytes.
	// Both are optional so configurations written before integrity evidence was
	// recorded continue to load unchanged.
	DownloadIntegrity  *IntegrityRecord `json:"download_integrity,omitempty"`
	InstalledIntegrity *IntegrityRecord `json:"installed_integrity,omitempty"`
	Pinned             bool             `json:"pinned"`
	MinAgeDays         int              `json:"min_age_days,omitempty"`
}

// SelectionDescriptor identifies a deliberate logical product and compatible
// build without relying on a versioned release filename.
type SelectionDescriptor struct {
	LogicalProduct string           `json:"logical_product,omitempty"`
	Target         *SelectionTarget `json:"target,omitempty"`
	ArchiveMember  string           `json:"archive_member,omitempty"`
}

// SelectionTarget records the target constraints for a selected release
// product. Empty fields leave that constraint unspecified.
type SelectionTarget struct {
	OS           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	ABI          string `json:"abi,omitempty"`
	CPUVariant   string `json:"cpu_variant,omitempty"`
}

// UnresolvedBinaryTransaction records a direct-binary commit whose executable
// rollback could not be confirmed. It is keyed by the managed binary path in
// the configuration and owned by ID.
type UnresolvedBinaryTransaction struct {
	ID              string  `json:"id"`
	DestinationPath string  `json:"destination_path"`
	ArtifactPath    string  `json:"artifact_path,omitempty"`
	Previous        *Binary `json:"previous,omitempty"`
	Intended        *Binary `json:"intended"`
}

// BinaryTransaction combines a single managed-record update with the external
// executable publication it represents. Publish and Rollback run while the
// config mutex and cross-process config lock are held, so they must be short.
type BinaryTransaction struct {
	ID                      string
	Intended                *Binary
	DestinationPath         string
	ReserveRollbackArtifact func() (string, error)
	Publish                 func(previous *Binary) error
	Rollback                func(previous *Binary) error
	RollbackArtifact        func() string
	Cleanup                 func() error
}

// IntegrityRecord is the serializable counterpart to provider integrity
// evidence. It lives in config to keep provider transport types out of the
// persistent configuration package.
type IntegrityRecord struct {
	Algorithm string `json:"algorithm"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
	Source    string `json:"source"`
	Scope     string `json:"scope"`
	Result    string `json:"result"`
}

// CloneBinary returns an independent copy of binary, including optional
// integrity records and selection intent.
func CloneBinary(binary *Binary) *Binary {
	if binary == nil {
		return nil
	}

	clone := *binary
	clone.SelectionIntent = CloneSelectionDescriptor(binary.SelectionIntent)
	clone.DownloadIntegrity = CloneIntegrityRecord(binary.DownloadIntegrity)
	clone.InstalledIntegrity = CloneIntegrityRecord(binary.InstalledIntegrity)
	return &clone
}

// CloneSelectionDescriptor returns an independent copy of a selection
// descriptor.
func CloneSelectionDescriptor(descriptor *SelectionDescriptor) *SelectionDescriptor {
	if descriptor == nil {
		return nil
	}

	clone := *descriptor
	if descriptor.Target != nil {
		target := *descriptor.Target
		clone.Target = &target
	}
	return &clone
}

func cloneUnresolvedBinaryTransaction(transaction *UnresolvedBinaryTransaction) *UnresolvedBinaryTransaction {
	if transaction == nil {
		return nil
	}

	return &UnresolvedBinaryTransaction{
		ID:              transaction.ID,
		DestinationPath: transaction.DestinationPath,
		ArtifactPath:    transaction.ArtifactPath,
		Previous:        CloneBinary(transaction.Previous),
		Intended:        CloneBinary(transaction.Intended),
	}
}

// CloneIntegrityRecord returns an independent copy of an integrity record.
func CloneIntegrityRecord(record *IntegrityRecord) *IntegrityRecord {
	if record == nil {
		return nil
	}

	clone := *record
	return &clone
}

func CheckAndLoad() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	return withConfigLock(configPath, func() error {
		loaded, created, err := loadConfigLocked(configPath)
		if err != nil {
			return err
		}

		if len(loaded.DefaultPath) == 0 {
			if exeDir := ForceInstallationDir(); len(exeDir) > 0 {
				loaded.DefaultPath = exeDir
			} else {
				loaded.DefaultPath, err = getDefaultPath()
				if err != nil {
					for {
						log.Info("Could not find a PATH directory automatically, falling back to manual selection")
						reader := bufio.NewReader(os.Stdin)
						var response string
						fmt.Printf("\nPlease specify a download directory: ")
						response, err = reader.ReadString('\n')
						if err != nil {
							return fmt.Errorf("Invalid input")
						}
						response = strings.TrimSpace(response)

						if err = checkDirExistsAndWritable(response); err != nil {
							log.Debugf("Could not set download directory [%s]: [%v]", response, err)
							continue
						}

						loaded.DefaultPath = response
						break
					}
				}
			}

			if created {
				if err := writeConfig(configPath, loaded); err != nil {
					return err
				}
			}
		} else if err := ensureDefaultPathExists(loaded.DefaultPath); err != nil {
			log.Debugf("Cached default path %s was missing or unusable: %v", loaded.DefaultPath, err)
			loaded.DefaultPath, err = getDefaultPath()
			if err != nil {
				return err
			}
			if err := writeConfig(configPath, loaded); err != nil {
				return err
			}
		}

		cfg = loaded
		log.Debugf("Download path set to %s", cfg.DefaultPath)
		return nil
	})
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

	return mutateConfigLocked(func(current *config) error {
		switch key {
		case "default_path":
			current.DefaultPath = value
		case "use_gh_for_github_token":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value %q for %s", value, key)
			}
			current.UseGHAuth = parsed
		default:
			return fmt.Errorf("%w: %s", ErrInvalidConfigKey, key)
		}

		return nil
	})
}

func loadConfigLocked(configPath string) (config, bool, error) {
	log.Debugf("Config directory is: %s", filepath.Dir(configPath))

	created := false
	f, err := os.Open(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return config{}, false, err
		}

		f, err = os.OpenFile(configPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, configFileMode)
		if err != nil {
			if !os.IsExist(err) {
				return config{}, false, err
			}

			f, err = os.Open(configPath)
			if err != nil {
				return config{}, false, err
			}
		} else {
			created = true
		}
	}
	defer f.Close()

	// Read the raw JSON first so we can detect which keys were explicitly
	// present. This lets us apply defaults (e.g. use_gh_for_github_token=true)
	// only when the user did not set a value, while still honoring an explicit
	// false.
	rawBytes, err := io.ReadAll(f)
	if err != nil {
		return config{}, false, err
	}

	rawKeys := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(rawBytes)) > 0 {
		if err := json.Unmarshal(rawBytes, &rawKeys); err != nil {
			return config{}, false, err
		}
	}

	loaded := config{}
	loaded.useGHAuthExplicit = jsonKeyPresent(rawKeys, "use_gh_for_github_token")
	if len(rawBytes) > 0 {
		if err := json.Unmarshal(rawBytes, &loaded); err != nil {
			return config{}, false, err
		}
	}

	if loaded.Bins == nil {
		loaded.Bins = map[string]*Binary{}
	}
	if loaded.UnresolvedTransactions == nil {
		loaded.UnresolvedTransactions = map[string]*UnresolvedBinaryTransaction{}
	}
	if err := loadTransactionJournals(configPath, &loaded); err != nil {
		return config{}, false, err
	}
	if runtime.GOOS == "linux" && len(loaded.DefaultChmod) == 0 {
		loaded.DefaultChmod = "0755"
	}

	// Default use_gh_for_github_token to true so an authenticated `gh` CLI is
	// reused automatically without requiring `bin set-config`. An explicit
	// `false` in the config file is honored via useGHAuthExplicit.
	if !loaded.useGHAuthExplicit {
		loaded.UseGHAuth = true
	}

	return loaded, created, nil
}

// jsonKeyPresent reports whether the given key appears in the raw JSON object.
func jsonKeyPresent(rawKeys map[string]json.RawMessage, key string) bool {
	_, ok := rawKeys[key]
	return ok
}

func mutateConfigLocked(mutate func(*config) error) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	return withConfigLock(configPath, func() error {
		loaded, _, err := loadConfigLocked(configPath)
		if err != nil {
			return err
		}
		if err := mutate(&loaded); err != nil {
			return err
		}
		if err := writeConfig(configPath, loaded); err != nil {
			return err
		}
		cfg = loaded
		return nil
	})
}

// GetBinary returns a copy of a managed record. It refuses a record with
// unresolved transaction state so lifecycle callers cannot treat either side
// of an incomplete commit as successful.
func GetBinary(path string) (*Binary, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	if _, ok := cfg.UnresolvedTransactions[path]; ok {
		return nil, fmt.Errorf("%w: %s", ErrBinaryRecoveryRequired, path)
	}
	return CloneBinary(cfg.Bins[path]), nil
}

// CheckBinaryResolved prevents lifecycle callers from acting on a direct
// binary whose executable/config commit needs reconciliation.
func CheckBinaryResolved(path string) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return checkBinaryResolved(&cfg, path)
}

// CommitBinaryTransaction publishes an external executable mutation and its
// one managed record while holding both config locks. It reloads current state
// before publishing and preserves every record other than Intended.Path.
func CommitBinaryTransaction(transaction BinaryTransaction) error {
	if err := validateBinaryTransaction(transaction); err != nil {
		return err
	}

	cfgMu.Lock()
	defer cfgMu.Unlock()

	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	return withConfigLock(configPath, func() error {
		loaded, _, err := loadConfigLocked(configPath)
		if err != nil {
			return err
		}
		if err := checkBinaryResolved(&loaded, transaction.Intended.Path); err != nil {
			cfg = loaded
			return err
		}

		previous := CloneBinary(loaded.Bins[transaction.Intended.Path])
		unresolved, err := prepareUnresolvedTransaction(transaction, previous)
		if err != nil {
			cfg = loaded
			return err
		}
		if err := writeTransactionJournal(configPath, transaction.Intended.Path, unresolved, transactionJournalIntent); err != nil {
			cfg = loaded
			return fmt.Errorf("persist transaction intent: %w", err)
		}
		if err := transaction.Publish(CloneBinary(previous)); err != nil {
			return finishFailedBinaryTransaction(configPath, &loaded, transaction, previous, unresolved, err)
		}

		loaded.Bins[transaction.Intended.Path] = CloneBinary(transaction.Intended)
		if err := writeConfig(configPath, loaded); err != nil {
			return finishFailedBinaryTransaction(configPath, &loaded, transaction, previous, unresolved, err)
		}
		loaded.UnresolvedTransactions[transaction.Intended.Path] = unresolved
		cfg = loaded
		if err := writeTransactionJournal(configPath, transaction.Intended.Path, unresolved, transactionJournalCommitted); err != nil {
			cfg = loaded
			return fmt.Errorf("mark committed transaction journal: %w", err)
		}
		if transaction.Cleanup == nil {
			if err := removeOwnedTransactionJournal(configPath, transaction.Intended.Path, transaction.ID); err != nil {
				return err
			}
			delete(loaded.UnresolvedTransactions, transaction.Intended.Path)
			cfg = loaded
			return nil
		}
		if err := transaction.Cleanup(); err != nil {
			return err
		}
		if err := removeOwnedTransactionJournal(configPath, transaction.Intended.Path, transaction.ID); err != nil {
			return err
		}
		delete(loaded.UnresolvedTransactions, transaction.Intended.Path)
		cfg = loaded
		return nil
	})
}

// RecoverBinaryTransaction resolves an incomplete transaction from the current
// managed record and owned filesystem evidence. It only accepts unambiguous
// previous, intended, or never-published states.
func RecoverBinaryTransaction(path string) error {
	if path == "" {
		return errors.New("binary transaction recovery requires path")
	}

	cfgMu.Lock()
	defer cfgMu.Unlock()

	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	return withConfigLock(configPath, func() error {
		loaded, _, err := loadConfigLocked(configPath)
		if err != nil {
			return err
		}
		unresolved, ok := loaded.UnresolvedTransactions[path]
		if !ok {
			cfg = loaded
			return fmt.Errorf("%w: %s", ErrTransactionOwnershipMismatch, path)
		}

		current := loaded.Bins[path]
		if !reflect.DeepEqual(current, unresolved.Previous) && !reflect.DeepEqual(current, unresolved.Intended) {
			cfg = loaded
			return errors.New("transaction config evidence is ambiguous")
		}
		resolved, err := resolveTransactionEvidence(*cloneUnresolvedBinaryTransaction(unresolved))
		if err != nil {
			cfg = loaded
			return err
		}

		if resolved == nil {
			delete(loaded.Bins, path)
		} else {
			loaded.Bins[path] = CloneBinary(resolved)
		}
		delete(loaded.UnresolvedTransactions, path)
		if err := writeConfig(configPath, loaded); err != nil {
			loaded.UnresolvedTransactions[path] = unresolved
			cfg = loaded
			return err
		}
		if err := removeOwnedTransactionJournal(configPath, path, unresolved.ID); err != nil {
			loaded.UnresolvedTransactions[path] = unresolved
			cfg = loaded
			return fmt.Errorf("remove resolved transaction journal: %w", err)
		}
		cfg = loaded
		return nil
	})
}

func resolveTransactionEvidence(unresolved UnresolvedBinaryTransaction) (*Binary, error) {
	if unresolved.Intended == nil || unresolved.DestinationPath == "" {
		return nil, errors.New("transaction recovery has unsafe or incomplete filesystem evidence")
	}
	if unresolved.ArtifactPath != "" && !ownedRollbackArtifact(unresolved.DestinationPath, unresolved.ArtifactPath) {
		return nil, errors.New("transaction recovery has unsafe or incomplete filesystem evidence")
	}
	destinationHash, destinationExists, err := transactionFileHash(unresolved.DestinationPath)
	if err != nil {
		return nil, err
	}
	backupHash, backupExists := "", false
	if unresolved.ArtifactPath != "" {
		backupHash, backupExists, err = transactionFileHash(unresolved.ArtifactPath)
		if err != nil {
			return nil, err
		}
	}
	if unresolved.Previous == nil && !destinationExists && !backupExists {
		return nil, nil
	}
	if destinationExists && hashMatches(destinationHash, unresolved.Intended.Hash) {
		if backupExists {
			if unresolved.Previous == nil || !hashMatches(backupHash, unresolved.Previous.Hash) {
				return nil, errors.New("transaction backup does not match previous binary")
			}
			if err := os.Remove(unresolved.ArtifactPath); err != nil {
				return nil, err
			}
		}
		return CloneBinary(unresolved.Intended), nil
	}
	if unresolved.Previous != nil && destinationExists && hashMatches(destinationHash, unresolved.Previous.Hash) {
		if backupExists {
			if !hashMatches(backupHash, unresolved.Previous.Hash) {
				return nil, errors.New("transaction backup does not match previous binary")
			}
			if err := os.Remove(unresolved.ArtifactPath); err != nil {
				return nil, err
			}
		}
		return CloneBinary(unresolved.Previous), nil
	}
	if unresolved.Previous != nil && !destinationExists && backupExists && hashMatches(backupHash, unresolved.Previous.Hash) {
		if err := os.Rename(unresolved.ArtifactPath, unresolved.DestinationPath); err != nil {
			return nil, err
		}
		return CloneBinary(unresolved.Previous), nil
	}
	return nil, errors.New("transaction filesystem evidence is ambiguous")
}

func transactionFileHash(path string) (string, bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), true, nil
}

func hashMatches(actual, expected string) bool {
	return expected != "" && strings.EqualFold(actual, expected)
}

func validateBinaryTransaction(transaction BinaryTransaction) error {
	if transaction.ID == "" || transaction.Intended == nil || transaction.Intended.Path == "" || transaction.Publish == nil || transaction.Rollback == nil {
		return errors.New("binary transaction requires owner, intended binary, publish, and rollback")
	}
	if !safeTransactionID(transaction.ID) {
		return fmt.Errorf("invalid binary transaction owner %q", transaction.ID)
	}
	return nil
}

func finishFailedBinaryTransaction(configPath string, loaded *config, transaction BinaryTransaction, previous *Binary, unresolved *UnresolvedBinaryTransaction, operationErr error) error {
	rollbackErr := transaction.Rollback(CloneBinary(previous))
	if rollbackErr == nil {
		restoreTransactionRecord(loaded, transaction.Intended.Path, previous)
		loaded.UnresolvedTransactions[transaction.Intended.Path] = unresolved
		if err := writeTransactionJournal(configPath, transaction.Intended.Path, unresolved, transactionJournalRolledBack); err != nil {
			cfg = *loaded
			return errors.Join(operationErr, fmt.Errorf("mark rolled-back transaction journal: %w", err))
		}
		cfg = *loaded
		if err := removeOwnedTransactionJournal(configPath, transaction.Intended.Path, transaction.ID); err != nil {
			return errors.Join(operationErr, err)
		}
		delete(loaded.UnresolvedTransactions, transaction.Intended.Path)
		cfg = *loaded
		return operationErr
	}

	restoreTransactionRecord(loaded, transaction.Intended.Path, previous)
	if artifact := rollbackArtifact(transaction); artifact != "" {
		unresolved.ArtifactPath = artifact
	}
	loaded.UnresolvedTransactions[transaction.Intended.Path] = unresolved
	if err := writeTransactionJournal(configPath, transaction.Intended.Path, unresolved, transactionJournalIntent); err != nil {
		cfg = *loaded
		return errors.Join(operationErr, rollbackErr, fmt.Errorf("persist unresolved transaction journal: %w", err))
	}
	if err := writeConfig(configPath, *loaded); err != nil {
		cfg = *loaded
		return errors.Join(operationErr, rollbackErr, fmt.Errorf("persist unresolved transaction: %w", err))
	}
	cfg = *loaded
	return errors.Join(operationErr, rollbackErr)
}

func prepareUnresolvedTransaction(transaction BinaryTransaction, previous *Binary) (*UnresolvedBinaryTransaction, error) {
	destination := transaction.DestinationPath
	if destination == "" {
		destination = transaction.Intended.Path
	}
	unresolved := &UnresolvedBinaryTransaction{ID: transaction.ID, DestinationPath: destination, Previous: CloneBinary(previous), Intended: CloneBinary(transaction.Intended)}
	if transaction.ReserveRollbackArtifact == nil {
		return unresolved, nil
	}
	artifact, err := transaction.ReserveRollbackArtifact()
	if err != nil {
		return nil, fmt.Errorf("reserve rollback artifact: %w", err)
	}
	if !ownedRollbackArtifact(destination, artifact) {
		return nil, errors.New("reserved rollback artifact is not owned by transaction destination")
	}
	unresolved.ArtifactPath = artifact
	return unresolved, nil
}

func rollbackArtifact(transaction BinaryTransaction) string {
	if transaction.RollbackArtifact == nil {
		return ""
	}
	return transaction.RollbackArtifact()
}

func ownedRollbackArtifact(destination, artifact string) bool {
	if destination == "" || artifact == "" {
		return false
	}
	return filepath.Dir(destination) == filepath.Dir(artifact) && strings.HasPrefix(filepath.Base(artifact), filepath.Base(destination)+".rollback-")
}

func restoreTransactionRecord(current *config, path string, previous *Binary) {
	if previous == nil {
		delete(current.Bins, path)
		return
	}
	current.Bins[path] = CloneBinary(previous)
}

func checkBinaryResolved(current *config, path string) error {
	if _, ok := current.UnresolvedTransactions[path]; ok {
		return fmt.Errorf("%w: %s", ErrBinaryRecoveryRequired, path)
	}
	return nil
}

// ensureDefaultPathExists recreates the cached default install directory if
// it was deleted after the first run, then verifies it is writable.
func ensureDefaultPathExists(dir string) error {
	expanded := os.ExpandEnv(dir)
	if err := os.MkdirAll(expanded, 0o755); err != nil {
		return err
	}
	return checkDirExistsAndWritable(expanded)
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

	if c == nil {
		return nil
	}

	return mutateConfigLocked(func(current *config) error {
		if err := checkBinaryResolved(current, c.Path); err != nil {
			return err
		}
		current.Bins[c.Path] = CloneBinary(c)
		return nil
	})
}

// UpsertBinaries adds or updates multiple binary resources
// in the config, writing to disk once.
func UpsertBinaries(binaries []*Binary) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	return mutateConfigLocked(func(current *config) error {
		for _, c := range binaries {
			if c != nil {
				if err := checkBinaryResolved(current, c.Path); err != nil {
					return err
				}
				current.Bins[c.Path] = CloneBinary(c)
			}
		}
		return nil
	})
}

// RemoveBinaries removes the specified paths
// from bin configuration. It doesn't care about the order
func RemoveBinaries(paths []string) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	return mutateConfigLocked(func(current *config) error {
		for _, p := range paths {
			if err := checkBinaryResolved(current, p); err != nil {
				return err
			}
			delete(current.Bins, p)
		}
		return nil
	})
}

func writeConfigLocked(configPath string, current config) error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
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

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "    ")
	err = encoder.Encode(current)
	if err != nil {
		return err
	}

	if supportsConfigFileMode() {
		if err := f.Chmod(configFileMode); err != nil {
			return err
		}
	}

	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		return err
	}

	return nil
}

type transactionJournal struct {
	Path       string                      `json:"path"`
	State      string                      `json:"state"`
	Unresolved UnresolvedBinaryTransaction `json:"unresolved"`
}

const (
	transactionJournalIntent     = "intent"
	transactionJournalCommitted  = "committed"
	transactionJournalRolledBack = "rolled-back"
)

var transactionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func safeTransactionID(id string) bool {
	return transactionIDPattern.MatchString(id) && id != "." && id != ".."
}

func transactionJournalPath(configPath, transactionID string) string {
	return configPath + ".transaction-" + transactionID + ".json"
}

func writeTransactionJournal(configPath, path string, unresolved *UnresolvedBinaryTransaction, state string) error {
	if !safeTransactionID(unresolved.ID) || path == "" || unresolved.Intended == nil {
		return errors.New("invalid transaction journal")
	}
	journalPath := transactionJournalPath(configPath, unresolved.ID)
	journal := transactionJournal{Path: path, State: state, Unresolved: *cloneUnresolvedBinaryTransaction(unresolved)}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	journalFile, err := os.CreateTemp(filepath.Dir(journalPath), filepath.Base(journalPath)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := journalFile.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := journalFile.Write(data); err != nil {
		_ = journalFile.Close()
		return err
	}
	if supportsConfigFileMode() {
		if err := journalFile.Chmod(configFileMode); err != nil {
			_ = journalFile.Close()
			return err
		}
	}
	if err := journalFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, journalPath); err != nil {
		return err
	}
	return nil
}

func loadTransactionJournals(configPath string, loaded *config) error {
	paths, err := globFiles(configPath + ".transaction-*.json")
	if err != nil {
		return err
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var journal transactionJournal
		if err := json.Unmarshal(data, &journal); err != nil {
			return fmt.Errorf("read transaction journal %s: %w", path, err)
		}
		if journal.Path == "" || !safeTransactionID(journal.Unresolved.ID) || journal.Unresolved.Intended == nil || transactionJournalPath(configPath, journal.Unresolved.ID) != path {
			return fmt.Errorf("invalid transaction journal %s", path)
		}
		if journal.State != transactionJournalIntent && journal.State != transactionJournalCommitted && journal.State != transactionJournalRolledBack {
			return fmt.Errorf("invalid transaction journal state %q", journal.State)
		}
		if journal.Unresolved.DestinationPath == "" || (journal.Unresolved.ArtifactPath != "" && !ownedRollbackArtifact(journal.Unresolved.DestinationPath, journal.Unresolved.ArtifactPath)) {
			return fmt.Errorf("unsafe transaction journal paths %s", path)
		}
		loaded.UnresolvedTransactions[journal.Path] = cloneUnresolvedBinaryTransaction(&journal.Unresolved)
	}
	return nil
}

func removeOwnedTransactionJournal(configPath, path, transactionID string) error {
	if !safeTransactionID(transactionID) {
		return fmt.Errorf("invalid transaction journal owner %q", transactionID)
	}
	journalPath := transactionJournalPath(configPath, transactionID)
	data, err := os.ReadFile(journalPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal transactionJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return err
	}
	if journal.Path != path || journal.Unresolved.ID != transactionID {
		return ErrTransactionOwnershipMismatch
	}
	return os.Remove(journalPath)
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

func configPathOverride() (string, bool, error) {
	configPath := os.Getenv("BIN_CONFIG")
	if configPath == "" {
		return "", false, nil
	}

	if _, err := os.Stat(configPath); err == nil || os.IsNotExist(err) {
		return configPath, true, nil
	} else {
		return "", true, err
	}
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
