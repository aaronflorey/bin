package providers

import (
	"fmt"

	"github.com/google/go-github/v80/github"
)

const githubRepositorySearchLimit = 20

type GitHubRepository struct {
	FullName    string
	Stars       int
	Description string
}

// SearchGitHubRepositories searches GitHub repository metadata without
// fetching or executing repository contents.
func SearchGitHubRepositories(query string) ([]GitHubRepository, error) {
	client, _, _, err := newGitHubClient()
	if err != nil {
		return nil, err
	}

	return searchGitHubRepositories(client, query)
}

func searchGitHubRepositories(client *github.Client, query string) ([]GitHubRepository, error) {
	ctx, cancel := newProviderRequestContext()
	defer cancel()

	result, _, err := client.Search.Repositories(ctx, query, &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: githubRepositorySearchLimit},
	})
	if err != nil {
		return nil, fmt.Errorf("search GitHub repositories: %w", err)
	}

	repositories := make([]GitHubRepository, 0, len(result.Repositories))
	for _, repository := range result.Repositories {
		repositories = append(repositories, GitHubRepository{
			FullName:    repository.GetFullName(),
			Stars:       repository.GetStargazersCount(),
			Description: repository.GetDescription(),
		})
	}

	return repositories, nil
}
