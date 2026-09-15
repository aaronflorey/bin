package cmd

import (
	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type recoverCmd struct{ cmd *cobra.Command }

func newRecoverCmd() *recoverCmd {
	root := &recoverCmd{}
	root.cmd = &cobra.Command{
		Use:           "recover <binary-path>",
		Short:         "Reconciles an interrupted direct-binary installation",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return config.RecoverBinaryTransaction(args[0])
		},
	}
	return root
}
