package cmd

import (
	"errors"
	"testing"
)

func TestIsPackagePathSelectionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "tar archive package path mismatch",
			err:  errors.New("no files found in tar archive, use -p flag to manually select . PackagePath [foo/bar]"),
			want: true,
		},
		{
			name: "zip archive package path mismatch",
			err:  errors.New("No files found in zip archive. PackagePath [foo/bar]"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("network timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPackagePathSelectionError(tt.err)
			if got != tt.want {
				t.Fatalf("isPackagePathSelectionError() = %v, want %v", got, tt.want)
			}
		})
	}
}
