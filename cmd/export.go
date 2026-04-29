package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type exportCmd struct {
	cmd *cobra.Command
}

func newExportCmd() *exportCmd {
	root := &exportCmd{}
	cmd := &cobra.Command{
		Use:           "export [file]",
		Short:         "Exports locally installed binaries as JSON",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Get()
			exportedBins, err := buildExportBins(cfg.Bins)
			if err != nil {
				return err
			}

			payload, err := json.MarshalIndent(exportedBins, "", "    ")
			if err != nil {
				return err
			}
			payload = append(payload, '\n')

			if len(args) == 1 {
				return os.WriteFile(args[0], payload, 0o644)
			}

			_, err = cmd.OutOrStdout().Write(payload)
			return err
		},
	}

	root.cmd = cmd
	enableSpinner(root.cmd)
	return root
}

// portableBinary is the shared serialization format for export and import.
type portableBinary struct {
	Name        string `json:"name"`
	RemoteName  string `json:"remote_name"`
	Version     string `json:"version"`
	Hash        string `json:"hash"`
	URL         string `json:"url"`
	Provider    string `json:"provider"`
	InstallMode string `json:"install_mode,omitempty"`
	PackageType string `json:"package_type,omitempty"`
	AppBundle   string `json:"app_bundle,omitempty"`
	PackagePath string `json:"package_path"`
	Pinned      bool   `json:"pinned"`
	MinAgeDays  int    `json:"min_age_days,omitempty"`
}

func buildExportBins(bins map[string]*config.Binary) ([]*portableBinary, error) {
	keys := make([]string, 0, len(bins))
	for k := range bins {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	exportedBins := make([]*portableBinary, 0, len(keys))
	for _, k := range keys {
		binCfg := bins[k]
		ep := os.ExpandEnv(binCfg.Path)

		hash, err := hashFile(ep)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}

		exportedBins = append(exportedBins, &portableBinary{
			Name:        filepath.Base(ep),
			RemoteName:  binCfg.RemoteName,
			Version:     binCfg.Version,
			Hash:        hash,
			URL:         binCfg.URL,
			Provider:    binCfg.Provider,
			InstallMode: binCfg.InstallMode,
			PackageType: binCfg.PackageType,
			AppBundle:   binCfg.AppBundle,
			PackagePath: binCfg.PackagePath,
			Pinned:      binCfg.Pinned,
			MinAgeDays:  binCfg.MinAgeDays,
		})
	}

	return exportedBins, nil
}
