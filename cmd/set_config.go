package cmd

import (
	"errors"
	"fmt"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type setConfigCmd struct {
	cmd *cobra.Command
}

func newSetConfigCmd() *setConfigCmd {
	root := &setConfigCmd{}
	cmd := &cobra.Command{
		Use:           "set-config {key} {value}",
		Short:         "Set a config value",
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			if err := config.Set(key, value); err != nil {
				if errors.Is(err, config.ErrInvalidConfigKey) {
					return fmt.Errorf("unsupported config key %q", key)
				}
				return err
			}

			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Set %s to %s\n", key, value)
			return err
		},
	}

	root.cmd = cmd
	return root
}
