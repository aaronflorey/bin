package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

			defaultPath := config.Get().DefaultPath
			existingBins := config.Get().Bins
			toUpsert := make([]*config.Binary, 0, len(bins))
			installedCount := 0
			updatedCount := 0
			skippedCount := 0
			for i, b := range bins {
				name := strings.TrimSpace(b.Name)
				if name == "" {
					return fmt.Errorf("binary at index %d has empty name", i)
				}

				target := &config.Binary{
					Path:        filepath.Join(defaultPath, name),
					RemoteName:  b.RemoteName,
					Version:     b.Version,
					Hash:        b.Hash,
					URL:         b.URL,
					Provider:    b.Provider,
					InstallMode: b.InstallMode,
					PackageType: b.PackageType,
					PackagePath: b.PackagePath,
					Pinned:      b.Pinned,
					MinAgeDays:  b.MinAgeDays,
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

func parseImportBins(r io.Reader) ([]*portableBinary, error) {
	var bins []*portableBinary
	if err := json.NewDecoder(r).Decode(&bins); err != nil {
		return nil, err
	}
	return bins, nil
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
		a.PackagePath == b.PackagePath &&
		a.Pinned == b.Pinned &&
		a.MinAgeDays == b.MinAgeDays
}
