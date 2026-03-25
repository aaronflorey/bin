package cmd

import (
	"strings"
	"testing"
)

func TestInstallRejectsNonPositiveMinAgeDays(t *testing.T) {
	cmd := newInstallCmd()
	cmd.cmd.SetArgs([]string{"--min-age-days=0", "https://example.test/acme/tool"})

	err := cmd.cmd.Execute()
	if err == nil {
		t.Fatal("expected install command to reject min-age-days=0")
	}
	if !strings.Contains(err.Error(), "--min-age-days must be a positive integer") {
		t.Fatalf("unexpected error: %v", err)
	}
}
