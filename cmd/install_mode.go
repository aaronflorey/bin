package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/aaronflorey/bin/pkg/systempackage"
)

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

var lifecycleRegistry = map[string]lifecycleStrategy{
	installModeBinary: {
		install:   installBinary,
		uninstall: nil,
		applyStoredFetch: func(b *config.Binary, fetchOpts *providers.FetchOpts) error {
			fetchOpts.PackagePath = b.PackagePath
			fetchOpts.PackageName = b.RemoteName
			return nil
		},
		applyRequestFetch: func(_ string, _ *providers.FetchOpts) error {
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
