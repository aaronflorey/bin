package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/marcosnils/bin/pkg/config"
	"github.com/marcosnils/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type outdatedCmd struct {
	cmd         *cobra.Command
	opts        outdatedOpts
	newProvider providerFactory
}

type outdatedOpts struct {
	format string
}

type outdatedBin struct {
	Path           string `json:"path"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	URL            string `json:"url"`
}

func newOutdatedCmd() *outdatedCmd {
	root := &outdatedCmd{newProvider: providers.New}
	cmd := &cobra.Command{
		Use:           "outdated [binary_path]",
		Short:         "Lists binaries that have updates available",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutdatedFormat(root.opts.format); err != nil {
				return err
			}

			cfg := config.Get()
			binsToProcess, err := resolveBinsToProcess(cfg.Bins, args)
			if err != nil {
				return err
			}

			updates, _, err := collectAvailableUpdates(binsToProcess, root.newProvider, false)
			if err != nil {
				return err
			}

			outdated := make([]outdatedBin, 0, len(updates))
			for _, u := range updates {
				outdated = append(outdated, outdatedBin{
					Path:           u.binary.Path,
					CurrentVersion: u.binary.Version,
					LatestVersion:  u.info.version,
					URL:            u.info.url,
				})
			}

			return writeOutdatedOutput(cmd, root.opts.format, outdated)
		},
	}

	root.cmd = cmd
	root.cmd.Flags().StringVar(&root.opts.format, "format", "text", "Output format: text|json")
	return root
}

func validateOutdatedFormat(format string) error {
	switch format {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("invalid format %q: expected one of text,json", format)
	}
}

func writeOutdatedOutput(cmd *cobra.Command, format string, outdated []outdatedBin) error {
	if format == "json" {
		payload, err := json.MarshalIndent(outdated, "", "    ")
		if err != nil {
			return err
		}
		payload = append(payload, '\n')
		_, err = cmd.OutOrStdout().Write(payload)
		return err
	}

	if len(outdated) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "All binaries are up to date")
		return err
	}

	for _, b := range outdated {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s (%s)\n", b.Path, b.CurrentVersion, b.LatestVersion, b.URL); err != nil {
			return err
		}
	}
	return nil
}
