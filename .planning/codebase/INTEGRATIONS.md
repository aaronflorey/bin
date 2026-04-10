# External Integrations

**Analysis Date:** 2026-04-10

## APIs & External Services

**Source control release APIs:**
- GitHub Releases - fetches latest or tagged releases, asset metadata, and release download URLs for installed binaries.
  - SDK/Client: `github.com/google/go-github/v73` with `golang.org/x/oauth2` in `pkg/providers/github.go`
  - Auth: `GITHUB_AUTH_TOKEN` or `GITHUB_TOKEN`; GitHub Enterprise uses `GHES_BASE_URL`, `GHES_UPLOAD_URL`, and `GHES_AUTH_TOKEN` in `pkg/providers/github.go`
- GitLab Releases and Packages - fetches releases, package files, and asset links for GitLab-hosted binaries.
  - SDK/Client: `gitlab.com/gitlab-org/api/client-go` in `pkg/providers/gitlab.go`
  - Auth: `GITLAB_TOKEN` or host-specific `GITLAB_TOKEN_<hostname>` in `pkg/providers/gitlab.go`
- Codeberg / Gitea API - fetches release attachments from Codeberg-hosted repositories.
  - SDK/Client: `code.gitea.io/sdk/gitea` in `pkg/providers/codeberg.go`
  - Auth: `CODEBERG_TOKEN` in `pkg/providers/codeberg.go`

**Package and artifact registries:**
- HashiCorp Releases - reads release index JSON and build artifact URLs from `https://releases.hashicorp.com`.
  - SDK/Client: Go `net/http` client in `pkg/providers/hashicorp.go`
  - Auth: None detected
- Go module proxy - resolves latest version metadata for `goinstall://` installs through `https://proxy.golang.org`.
  - SDK/Client: Go `net/http` client and local `go install` execution in `pkg/providers/goinstall.go`
  - Auth: None detected
- Generic HTTP(S) downloads - downloads direct binary URLs and infers versions from filenames.
  - SDK/Client: Go `net/http` client in `pkg/providers/generic_url.go`
  - Auth: None detected

**Container registries:**
- Docker Engine and Docker Hub tags API - pulls container images locally and queries Docker Hub tag metadata for update detection.
  - SDK/Client: `github.com/docker/docker` in `pkg/providers/docker.go`; raw HTTP to `https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=100`
  - Auth: Docker environment variables from the local Docker client environment; custom wrapper template via `BIN_DOCKER_RUN_TEMPLATE` in `pkg/providers/docker_unix.go` and `pkg/providers/docker_windows.go`

**Repository self-distribution:**
- GitHub Releases for this repository - the bootstrap installer and GitHub Action both download `bin` release assets from the current repo.
  - SDK/Client: raw HTTPS in `install.sh` and Node `https` in `action/setup.js`
  - Auth: `GITHUB_AUTH_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN` in `install.sh`; `github-token` input passed to `action/setup.js` and `action/install.js` from `action.yml`

## Data Storage

**Databases:**
- None detected.
  - Connection: Not applicable
  - Client: Not applicable

**File Storage:**
- Local filesystem only for managed binaries and JSON config written by `pkg/config/config.go`.
- Temporary files/directories are used by `install.sh` and `action/setup.js` during bootstrap downloads and extraction.

**Caching:**
- None detected as a dedicated cache service.
- Local Docker image cache is used implicitly by the Docker daemon when `pkg/providers/docker.go` pulls images.

## Authentication & Identity

**Auth Provider:**
- Token-based API authentication only; no user login system or identity provider is implemented in this repository.
  - Implementation: environment-variable tokens injected into provider clients in `pkg/providers/github.go`, `pkg/providers/gitlab.go`, `pkg/providers/codeberg.go`, `install.sh`, and `action/setup.js`

## Monitoring & Observability

**Error Tracking:**
- None detected.

**Logs:**
- CLI/application logs use `github.com/caarlos0/log` across `cmd/`, `pkg/config/config.go`, and provider implementations.
- CI logs come from GitHub Actions workflows in `.github/workflows/build.yml` and `.github/workflows/release-please.yml`.

## CI/CD & Deployment

**Hosting:**
- GitHub repository with GitHub Releases as the binary distribution channel, configured by `.goreleaser.yml` and `.github/workflows/release-please.yml`.
- GitHub Actions marketplace/composite action distribution via `action.yml`.

**CI Pipeline:**
- GitHub Actions in `.github/workflows/build.yml` for linting, tests, installer smoke tests, action smoke tests, and install smoke tests.
- GitHub Actions in `.github/workflows/release-please.yml` for release PR creation and GoReleaser publishing.

## Environment Configuration

**Required env vars:**
- `GITLAB_TOKEN` for GitLab installs, per `README.md` and `pkg/providers/gitlab.go`.
- One of `GITHUB_AUTH_TOKEN` or `GITHUB_TOKEN` is effectively required for higher-rate/private GitHub installs and is used in CI and bootstrap paths in `pkg/providers/github.go`, `.github/workflows/build.yml`, `install.sh`, and `action/setup.js`.
- `BIN_EXE_DIR` is optional but operationally important for deterministic install destinations in CI, the installer, and the action, handled by `pkg/config/config.go`, `.github/workflows/build.yml`, `install.sh`, and `action/install.js`.

**Secrets location:**
- GitHub Actions secrets/context variables in `.github/workflows/build.yml`, `.github/workflows/release-please.yml`, and `action.yml`.
- Local developer/runtime secrets are read from process environment variables in provider files under `pkg/providers/` and bootstrap scripts `install.sh` and `action/setup.js`.

## Webhooks & Callbacks

**Incoming:**
- GitHub Actions workflow triggers on repository events (`push`, `pull_request`, and `workflow_dispatch`) in `.github/workflows/build.yml` and `.github/workflows/release-please.yml`.

**Outgoing:**
- HTTPS API calls to GitHub, GitLab, Codeberg, HashiCorp Releases, Docker Hub, arbitrary HTTP(S) asset hosts, and the Go proxy from `pkg/providers/`, `install.sh`, and `action/setup.js`.

---

*Integration audit: 2026-04-10*
