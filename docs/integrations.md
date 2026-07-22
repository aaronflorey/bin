# Integrations and providers

`bin` chooses a provider from the URL or from `--provider` (`pkg/providers/providers.go`).

## Provider matrix

| Provider | URL shape | Auth / environment | Notes |
| --- | --- | --- | --- |
| GitHub | `github.com/owner/repo` or release URL | `GITHUB_AUTH_TOKEN`, `GITHUB_TOKEN`, `GHES_BASE_URL`, `GHES_UPLOAD_URL`, `GHES_AUTH_TOKEN`, optional `gh auth token` when `use_gh_for_github_token` is enabled | Uses the latest non-draft, non-prerelease release when no tag is specified (`pkg/providers/github.go`, README). |
| GitLab | `gitlab.com/owner/repo` or release URL | `GITLAB_TOKEN` or `GITLAB_TOKEN_<hostname>` | Uses the GitLab API and release/package metadata (`pkg/providers/gitlab.go`). |
| Codeberg | `codeberg.org/owner/repo` or release URL | `CODEBERG_TOKEN` | Uses the Gitea SDK against Codeberg (`pkg/providers/codeberg.go`). |
| Docker | `docker://repo:tag` | Any environment variable supported by the Docker CLI / `client.FromEnv` | Uses the local Docker daemon and pulls the image before wrapping it as an executable (`pkg/providers/docker.go`). |
| HashiCorp | `https://releases.hashicorp.com/...` | None documented in source | Dedicated release pages; no provider-specific env vars are defined (`README.md`, `pkg/providers/providers.go`). |
| Go install | `goinstall://module/path@version` | `go` must be on `PATH` | Uses `go env GOPATH`, `go install`, and `proxy.golang.org` version metadata (`pkg/providers/goinstall.go`). |
| Generic URL | Any direct HTTP(S) URL | None | Infers the version from `Content-Disposition` or the filename in the redirect/original URL. Fails if no semver-like token can be inferred (`pkg/providers/generic_url.go`). |

## Common behaviors

- Asset selection is shared across providers. It filters foreign targets and unsupported artifact shapes, prefers an explicitly requested or stored product, and prompts or fails when multiple products remain.
- `--all` skips product scoring but still lists only compatible candidates. `--select` selects an exact candidate before ranking but cannot bypass downloaded executable validation.
- Release downloads are inspected without execution. Native executables, shebang scripts, and executable archive entries are accepted; shared libraries, language packages, metadata, and ordinary files are rejected.
- For macOS GUI app releases on GitHub that publish `.dmg` assets, prefer `bin install --system-package --package-type dmg github.com/getpaseo/paseo Paseo`; non-Windows installs do not treat Windows `.exe` assets as valid binary fallbacks.
- `run` uses the same provider resolution logic but caches binaries in the user cache instead of writing to config.
- `update` reuses the stored provider and asset-selection metadata from the config entry.
