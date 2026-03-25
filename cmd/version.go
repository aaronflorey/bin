package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type versionCmd struct {
	cmd *cobra.Command
}

func newVersionCmd(version string) *versionCmd {
	root := &versionCmd{}
	cmd := &cobra.Command{
		Use:           "version",
		Short:         "Print the bin version",
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version)
		},
	}

	root.cmd = cmd
	return root
}
