package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaronflorey/bin/pkg/assets"
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

// portableBinary is defined in export.go.

type importCmd struct {
	cmd        *cobra.Command
	skipEnsure bool
	runEnsure  func(args []string) error
}

func newImportCmd() *importCmd {
	root := &importCmd{runEnsure: runEnsure}
	cmd := &cobra.Command{
		Use:           "import [file]",
		Short:         "Imports binaries from a JSON export",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := cmd.InOrStdin()
			if len(args) == 1 {
				f, err := os.Open(args[0])
				if err != nil {
					return err
				}
				defer f.Close()
				in = f
			}

			bins, err := parseImportBins(in)
			if err != nil {
				return err
			}

			validatedNames := make([]string, len(bins))
			for i, b := range bins {
				name, err := safePortableBinaryName(b.Name, i)
				if err != nil {
					return err
				}
				validatedNames[i] = name
			}

			defaultPath := config.Get().DefaultPath
			existingBins := config.Get().Bins
			toUpsert := make([]*config.Binary, 0, len(bins))
			installedCount := 0
			updatedCount := 0
			skippedCount := 0
			for i, b := range bins {
				name := validatedNames[i]
				path, err := importedBinaryPath(defaultPath, name, b)
				if err != nil {
					return fmt.Errorf("binary at index %d: %w", i, err)
				}

				target := &config.Binary{
					Path:               path,
					RemoteName:         b.RemoteName,
					Version:            b.Version,
					Hash:               b.Hash,
					URL:                b.URL,
					Provider:           b.Provider,
					InstallMode:        b.InstallMode,
					PackageType:        b.PackageType,
					AppBundle:          b.AppBundle,
					PackagePath:        b.PackagePath,
					SourceAsset:        b.SourceAsset,
					SelectionIntent:    config.CloneSelectionDescriptor(b.SelectionIntent),
					ReleaseTagPrefix:   b.ReleaseTagPrefix,
					DownloadIntegrity:  config.CloneIntegrityRecord(b.DownloadIntegrity),
					InstalledIntegrity: importedInstalledIntegrity(b.InstalledIntegrity),
					Pinned:             b.Pinned,
					MinAgeDays:         b.MinAgeDays,
				}

				status := "installed"
				if current, ok := existingBins[target.Path]; ok {
					if equalBinaryConfig(current, target) {
						status = "skipped"
					} else {
						status = "updated"
					}
				}

				switch status {
				case "installed":
					installedCount++
					toUpsert = append(toUpsert, target)
				case "updated":
					updatedCount++
					toUpsert = append(toUpsert, target)
				default:
					skippedCount++
				}

				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", status, target.Path); err != nil {
					return err
				}
			}

			if len(toUpsert) > 0 {
				if err := config.UpsertBinaries(toUpsert); err != nil {
					return err
				}
			}

			toEnsure := make([]string, 0, len(toUpsert))
			for _, bin := range toUpsert {
				toEnsure = append(toEnsure, bin.Path)
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"import complete: installed=%d updated=%d skipped=%d\n",
				installedCount,
				updatedCount,
				skippedCount,
			)
			if err != nil {
				return err
			}

			if root.skipEnsure || len(toEnsure) == 0 {
				return nil
			}

			return root.runEnsure(toEnsure)
		},
	}

	root.cmd = cmd
	root.cmd.Flags().BoolVar(&root.skipEnsure, "skip-ensure", false, "Do not run ensure after importing")
	enableSpinner(root.cmd)
	return root
}

func importedBinaryPath(defaultPath, name string, binary *portableBinary) (string, error) {
	if effectiveInstallMode(binary.InstallMode) == installModeSystemPackage &&
		strings.EqualFold(binary.PackageType, "dmg") && binary.AppBundle != "" {
		bundlePath, err := managedDMGBundlePath(binary.AppBundle)
		if err != nil {
			return "", err
		}
		return filepath.Join(bundlePath, "Contents", "MacOS", name), nil
	}
	return filepath.Join(defaultPath, name), nil
}

func importedInstalledIntegrity(record *config.IntegrityRecord) *config.IntegrityRecord {
	if record == nil {
		return nil
	}

	clone := config.CloneIntegrityRecord(record)
	// An export describes verification on another machine. Keep that evidence
	// as provenance, but do not present it as a fresh assertion about local
	// bytes until this installation is fetched and verified again.
	clone.Result = "imported"
	return clone
}

func parseImportBins(r io.Reader) ([]*portableBinary, error) {
	var bins []*portableBinary
	if err := json.NewDecoder(r).Decode(&bins); err != nil {
		return nil, err
	}
	return bins, nil
}

func safePortableBinaryName(raw string, index int) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("binary at index %d has empty name", index)
	}
	if raw != trimmed {
		return "", fmt.Errorf("binary at index %d has invalid name %q", index, raw)
	}
	if err := assets.ValidatePortableName(raw); err != nil {
		return "", fmt.Errorf("binary at index %d has invalid name %q", index, raw)
	}
	return raw, nil
}

func equalBinaryConfig(a, b *config.Binary) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Path == b.Path &&
		a.RemoteName == b.RemoteName &&
		a.Version == b.Version &&
		a.Hash == b.Hash &&
		a.URL == b.URL &&
		a.Provider == b.Provider &&
		a.InstallMode == b.InstallMode &&
		a.PackageType == b.PackageType &&
		a.AppBundle == b.AppBundle &&
		a.PackagePath == b.PackagePath &&
		a.SourceAsset == b.SourceAsset &&
		equalSelectionDescriptor(a.SelectionIntent, b.SelectionIntent) &&
		a.ReleaseTagPrefix == b.ReleaseTagPrefix &&
		equalIntegrityRecord(a.DownloadIntegrity, b.DownloadIntegrity) &&
		equalIntegrityRecord(a.InstalledIntegrity, b.InstalledIntegrity) &&
		a.Pinned == b.Pinned &&
		a.MinAgeDays == b.MinAgeDays
}

func equalSelectionDescriptor(a, b *config.SelectionDescriptor) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.LogicalProduct != b.LogicalProduct || a.ArchiveMember != b.ArchiveMember {
		return false
	}
	if a.Target == nil || b.Target == nil {
		return a.Target == b.Target
	}
	return *a.Target == *b.Target
}

func equalIntegrityRecord(a, b *config.IntegrityRecord) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
