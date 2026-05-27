package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type exportCmd struct {
	cmd    *cobra.Command
	format string
}

func newExportCmd() *exportCmd {
	root := &exportCmd{}
	cmd := &cobra.Command{
		Use:           "export [file]",
		Short:         "Exports locally installed binaries",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := normalizeExportFormat(root.format)
			if err != nil {
				return err
			}

			cfg := config.Get()
			exportedBins, err := buildExportBins(cfg.Bins)
			if err != nil {
				return err
			}

			payload, err := buildExportPayload(format, exportedBins)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				return os.WriteFile(args[0], payload, 0o644)
			}

			_, err = cmd.OutOrStdout().Write(payload)
			return err
		},
	}

	root.cmd = cmd
	root.cmd.Flags().StringVar(&root.format, "format", "json", "Output format: json or list")
	enableSpinner(root.cmd)
	return root
}

func normalizeExportFormat(format string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(format)); normalized {
	case "", "json", "list":
		if normalized == "" {
			return "json", nil
		}
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported --format %q", format)
	}
}

func buildExportPayload(format string, exportedBins []*portableBinary) ([]byte, error) {
	if format == "list" {
		urls := make([]string, 0, len(exportedBins))
		for _, bin := range exportedBins {
			urls = append(urls, bin.URL)
		}
		return []byte(strings.Join(urls, "\n") + "\n"), nil
	}

	payload, err := json.MarshalIndent(exportedBins, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// portableBinary is the shared serialization format for export and import.
type portableBinary struct {
	Name             string `json:"name"`
	RemoteName       string `json:"remote_name"`
	Version          string `json:"version"`
	Hash             string `json:"hash"`
	URL              string `json:"url"`
	Provider         string `json:"provider"`
	InstallMode      string `json:"install_mode,omitempty"`
	PackageType      string `json:"package_type,omitempty"`
	AppBundle        string `json:"app_bundle,omitempty"`
	PackagePath      string `json:"package_path"`
	ReleaseTagPrefix string `json:"release_tag_prefix,omitempty"`
	Pinned           bool   `json:"pinned"`
	MinAgeDays       int    `json:"min_age_days,omitempty"`
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
			Name:             filepath.Base(ep),
			RemoteName:       binCfg.RemoteName,
			Version:          binCfg.Version,
			Hash:             hash,
			URL:              binCfg.URL,
			Provider:         binCfg.Provider,
			InstallMode:      binCfg.InstallMode,
			PackageType:      binCfg.PackageType,
			AppBundle:        binCfg.AppBundle,
			PackagePath:      binCfg.PackagePath,
			ReleaseTagPrefix: binCfg.ReleaseTagPrefix,
			Pinned:           binCfg.Pinned,
			MinAgeDays:       binCfg.MinAgeDays,
		})
	}

	return exportedBins, nil
}
