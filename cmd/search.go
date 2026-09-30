package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/aaronflorey/bin/pkg/providers"
	"github.com/spf13/cobra"
)

type searchCmd struct {
	cmd                *cobra.Command
	searchRepositories func(string) ([]providers.GitHubRepository, error)
}

var searchGitHubRepositories = providers.SearchGitHubRepositories

func newSearchCmd() *searchCmd {
	root := &searchCmd{searchRepositories: searchGitHubRepositories}
	cmd := &cobra.Command{
		Use:           "search <query>...",
		Short:         "Search GitHub repositories",
		Long:          "Search GitHub repository metadata. Use `bin browse https://github.com/owner/repo` to browse a result.",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          root.run,
	}

	root.cmd = cmd
	return root
}

func (root *searchCmd) run(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	repositories, err := root.searchRepositories(query)
	if err != nil {
		return err
	}
	return writeGitHubSearchResults(cmd.OutOrStdout(), query, repositories)
}

func writeGitHubSearchResults(w io.Writer, query string, repositories []providers.GitHubRepository) error {
	if len(repositories) == 0 {
		_, err := fmt.Fprintf(w, "No GitHub repositories found for %q.\n", query)
		return err
	}

	for i, repository := range repositories {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w,
			"%s\n  Stars: %d\n  Description: %s\n  Browse this repository with: bin browse https://github.com/%s\n",
			repository.FullName, repository.Stars, repository.Description, repository.FullName,
		); err != nil {
			return err
		}
	}

	return nil
}
