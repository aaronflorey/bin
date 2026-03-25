package cmd

import "github.com/spf13/cobra"

const spinnerAnnotation = "bin.spinner"

func enableSpinner(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	cmd.Annotations[spinnerAnnotation] = "true"
}

func shouldShowSpinner(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	return cmd.Annotations[spinnerAnnotation] == "true"
}
