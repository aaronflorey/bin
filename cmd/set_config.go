package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aaronflorey/bin/pkg/config"
	"github.com/spf13/cobra"
)

type setConfigCmd struct {
	cmd *cobra.Command
}

func newSetConfigCmd() *setConfigCmd {
	root := &setConfigCmd{}
	validKeys := config.ValidKeys()
	validKeysText := strings.Join(validKeys, ", ")
	cmd := &cobra.Command{
		Use:           "set-config {key} {value}",
		Short:         "Set a config value",
		Long:          fmt.Sprintf("Set a config value. Valid keys: %s", validKeysText),
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			if err := config.Set(key, value); err != nil {
				if errors.Is(err, config.ErrInvalidConfigKey) {
					return fmt.Errorf("unsupported config key %q. Valid keys: %s", key, validKeysText)
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
