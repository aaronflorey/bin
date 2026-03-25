package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestEnableSpinner(t *testing.T) {
	command := &cobra.Command{}

	enableSpinner(command)

	if !shouldShowSpinner(command) {
		t.Fatalf("expected spinner annotation to be enabled")
	}
}

func TestShouldShowSpinnerNilCommand(t *testing.T) {
	if shouldShowSpinner(nil) {
		t.Fatalf("expected nil command to not show spinner")
	}
}
