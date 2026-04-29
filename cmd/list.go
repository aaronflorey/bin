package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Pad given string with spaces to the right
func _rPad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func formatInstallMode(b *config.Binary) string {
	mode := effectiveInstallMode(b.InstallMode)
	if mode == installModeSystemPackage && b.PackageType != "" {
		return mode + ":" + b.PackageType
	}
	return mode
}

func listRowVersion(b *config.Binary) string {
	if b.Pinned {
		return "*" + b.Version
	}
	return b.Version
}

func writeList(out io.Writer, bins map[string]*config.Binary) error {
	binPaths := []string{}
	for k := range bins {
		binPaths = append(binPaths, k)
	}
	sort.Strings(binPaths)

	maxLengths := make([]int, 4)
	for _, k := range binPaths {
		b := bins[k]
		p := os.ExpandEnv(b.Path)
		if len(p) > maxLengths[0] {
			maxLengths[0] = len(p)
		}
		if versionLength := len(listRowVersion(b)); versionLength > maxLengths[1] {
			maxLengths[1] = versionLength
		}
		if len(b.URL) > maxLengths[2] {
			maxLengths[2] = len(b.URL)
		}
		if modeLength := len(formatInstallMode(b)); modeLength > maxLengths[3] {
			maxLengths[3] = modeLength
		}
	}

	pL, vL, uL, mL := maxLengths[0], maxLengths[1], maxLengths[2], maxLengths[3]
	magentaItalic := color.New(color.FgMagenta, color.Italic).Sprint
	p := magentaItalic(_rPad("Path", pL))
	v := magentaItalic(_rPad("Version", vL))
	u := magentaItalic(_rPad("URL", uL))
	m := magentaItalic(_rPad("Mode", mL))
	s := magentaItalic("Status")

	if _, err := fmt.Fprintf(out, "\n%s  %s  %s  %s  %s", p, v, u, m, s); err != nil {
		return err
	}

	for _, k := range binPaths {
		b := bins[k]
		p := os.ExpandEnv(b.Path)
		status := color.GreenString("OK")
		if _, err := os.Stat(p); err != nil {
			status = color.RedString("missing %s", p)
		}

		if _, err := fmt.Fprintf(out, "\n%s  %s  %s  %s  %s", _rPad(p, pL), _rPad(listRowVersion(b), vL), _rPad(b.URL, uL), _rPad(formatInstallMode(b), mL), status); err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(out, "\n\n")
	return err
}

type listCmd struct {
	cmd  *cobra.Command
	opts struct {
		format string
	}
}

type listedBinary struct {
	Path        string `json:"path"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Pinned      bool   `json:"pinned"`
	InstallMode string `json:"install_mode"`
	PackageType string `json:"package_type,omitempty"`
	AppBundle   string `json:"app_bundle,omitempty"`
	Provider    string `json:"provider,omitempty"`
	RemoteName  string `json:"remote_name,omitempty"`
}

func listStatus(path string) string {
	if _, err := os.Stat(path); err != nil {
		return "missing"
	}
	return "ok"
}

func listEntries(bins map[string]*config.Binary) []listedBinary {
	binPaths := make([]string, 0, len(bins))
	for path := range bins {
		binPaths = append(binPaths, path)
	}
	sort.Strings(binPaths)

	entries := make([]listedBinary, 0, len(binPaths))
	for _, path := range binPaths {
		bin := bins[path]
		expandedPath := os.ExpandEnv(bin.Path)
		entries = append(entries, listedBinary{
			Path:        expandedPath,
			Version:     bin.Version,
			URL:         bin.URL,
			Status:      listStatus(expandedPath),
			Pinned:      bin.Pinned,
			InstallMode: effectiveInstallMode(bin.InstallMode),
			PackageType: bin.PackageType,
			AppBundle:   bin.AppBundle,
			Provider:    bin.Provider,
			RemoteName:  bin.RemoteName,
		})
	}

	return entries
}

func writeListJSON(out io.Writer, bins map[string]*config.Binary) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(listEntries(bins))
}

func newListCmd() *listCmd {
	root := &listCmd{}
	// nolint: dupl
	cmd := &cobra.Command{
		Use:           "list",
		Aliases:       []string{"ls"},
		Short:         "List binaries managed by bin",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(strings.TrimSpace(root.opts.format)) {
			case "", "table":
				return writeList(cmd.OutOrStdout(), config.Get().Bins)
			case "json":
				return writeListJSON(cmd.OutOrStdout(), config.Get().Bins)
			default:
				return fmt.Errorf("unsupported --format %q", root.opts.format)
			}
		},
	}

	root.cmd = cmd
	root.cmd.Flags().StringVar(&root.opts.format, "format", "table", "Output format: table or json")
	return root
}
