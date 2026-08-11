package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
)

func requestedLogicalName(requestedPath string) string {
	if requestedPath == "" {
		return ""
	}
	return filepath.Base(requestedPath)
}

const (
	installModeBinary        = "binary"
	installModeSystemPackage = "system-package"
)

func effectiveInstallMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return installModeBinary
	}
	return mode
}

type lifecycleStrategy struct {
	install           func(InstallOpts) (*InstallResult, error)
	uninstall         func(*config.Binary) error
	applyStoredFetch  func(*config.Binary, *providers.FetchOpts) error
	applyRequestFetch func(string, *providers.FetchOpts) error
	resolvePath       func(*config.Binary) bool
}

func wrapUpdateFailure(b *config.Binary, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("update failed for %s: %w", b.Path, err)
}

var lifecycleRegistry = map[string]lifecycleStrategy{
	installModeBinary: {
		install:   installBinary,
		uninstall: nil,
		applyStoredFetch: func(b *config.Binary, fetchOpts *providers.FetchOpts) error {
			if err := validateStoredBinaryForReuse(b); err != nil {
				return err
			}
			fetchOpts.PackagePath = b.PackagePath
			// remote_name is the user-facing logical identity, not a versioned
			// source asset hint.
			fetchOpts.PackageName = b.RemoteName
			fetchOpts.ReleaseTagPrefix = providers.EffectiveReleaseTagPrefix(b.Version, b.ReleaseTagPrefix)
			return nil
		},
		applyRequestFetch: func(requestedName string, fetchOpts *providers.FetchOpts) error {
			if requestedName != "" {
				fetchOpts.PackageName = filepath.Base(requestedName)
			}
			return nil
		},
		resolvePath: func(*config.Binary) bool {
			return true
		},
	},
	installModeSystemPackage: {
		install:   installSystemPackage,
		uninstall: uninstallSystemPackage,
		applyStoredFetch: func(b *config.Binary, fetchOpts *providers.FetchOpts) error {
			packageType := systempackage.NormalizeType(b.PackageType)
			if packageType == "" {
				return fmt.Errorf("binary %s is in system-package mode but has no package_type metadata", b.Path)
			}
			fetchOpts.PackagePath = b.PackagePath
			fetchOpts.PackageName = b.RemoteName
			fetchOpts.ReleaseTagPrefix = providers.EffectiveReleaseTagPrefix(b.Version, b.ReleaseTagPrefix)
			fetchOpts.SystemPackage = true
			fetchOpts.PackageType = packageType
			return nil
		},
		applyRequestFetch: func(requestedName string, fetchOpts *providers.FetchOpts) error {
			fetchOpts.SystemPackage = true
			fetchOpts.PackageName = requestedName
			fetchOpts.PackageType = systempackage.NormalizeType(fetchOpts.PackageType)
			return nil
		},
		resolvePath: func(*config.Binary) bool {
			return false
		},
	},
}

func validateStoredBinaryForReuse(b *config.Binary) error {
	if b == nil {
		return nil
	}
	if assets.IsKnownNonRunnableName(b.RemoteName) || assets.IsKnownNonRunnableName(b.SourceAsset) || assets.IsKnownNonRunnableName(b.PackagePath) {
		return fmt.Errorf("stored entry %s has unsafe artifact metadata; remove the managed entry and reinstall it", b.Path)
	}
	ep := os.ExpandEnv(b.Path)
	if _, err := os.Stat(ep); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := assets.ValidateRunnablePayload(ep, b.SourceAsset); err != nil {
		return fmt.Errorf("stored entry %s is not a runnable binary and cannot be updated safely; remove the managed entry and reinstall it: %w", b.Path, err)
	}
	return nil
}

func lifecycleForMode(mode string) lifecycleStrategy {
	strategy, ok := lifecycleRegistry[effectiveInstallMode(mode)]
	if ok {
		return strategy
	}

	return lifecycleRegistry[installModeBinary]
}

func requestedInstallModes(strictSystemPackage, preferSystemPackage bool, requestedPath string) []string {
	if strictSystemPackage {
		return []string{installModeSystemPackage}
	}
	if systemPackagePathLooksExplicit(requestedPath) {
		return []string{installModeBinary}
	}
	if preferSystemPackage {
		return []string{installModeSystemPackage, installModeBinary}
	}
	return []string{installModeBinary, installModeSystemPackage}
}

func shouldFallbackInstallMode(err error) bool {
	return err != nil && (errors.Is(err, assets.ErrNoCompatibleFiles) || errors.Is(err, systempackage.ErrIncompatible))
}
