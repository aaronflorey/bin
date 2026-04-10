# Technology Stack

**Analysis Date:** 2026-04-10

## Languages

**Primary:**
- Go 1.24.4 - main CLI application, command handlers, providers, config, and tests in `main.go`, `cmd/`, and `pkg/` as declared in `go.mod`.

**Secondary:**
- JavaScript (Node.js runtime) - GitHub composite action helper scripts in `action/setup.js` and `action/install.js`.
- POSIX shell - bootstrap installer and local automation in `install.sh` and `Makefile`.
- YAML/JSON - CI, release, lint, and action metadata in `.github/workflows/build.yml`, `.github/workflows/release-please.yml`, `.goreleaser.yml`, `action.yml`, `release-please-config.json`, and `.release-please-manifest.json`.

## Runtime

**Environment:**
- Go toolchain 1.24.4 from `go.mod`.
- Node.js runtime required for the GitHub Action scripts referenced by `action.yml`.
- Shell runtime required for `install.sh` (`sh`) and archive utilities such as `tar`; `unzip` is required when bootstrap assets are `.zip` in `install.sh` and `action/setup.js`.

**Package Manager:**
- Go modules via `go.mod` and `go.sum`.
- Lockfile: present in `go.sum`.

## Frameworks

**Core:**
- `github.com/spf13/cobra` v1.9.1 - CLI command framework used by `cmd/root.go` and command files under `cmd/`.

**Testing:**
- Go standard `testing` package - unit and package tests in files like `cmd/install_test.go`, `cmd/update_test.go`, and `pkg/assets/assets_test.go`.

**Build/Dev:**
- GoReleaser v2 config - cross-platform release packaging in `.goreleaser.yml`.
- golangci-lint config v2 - lint and formatter policy in `.golangci.yml`.
- GitHub Actions - CI, smoke tests, and release automation in `.github/workflows/build.yml` and `.github/workflows/release-please.yml`.
- GNU Make - local developer tasks in `Makefile`.

## Key Dependencies

**Critical:**
- `github.com/spf13/cobra` v1.9.1 - command tree and CLI UX backbone in `cmd/` and `main.go`.
- `github.com/google/go-github/v73` v73.0.0 - GitHub release API client in `pkg/providers/github.go`.
- `gitlab.com/gitlab-org/api/client-go` v0.137.0 - GitLab release and package API client in `pkg/providers/gitlab.go`.
- `code.gitea.io/sdk/gitea` v0.22.0 - Codeberg/Gitea API client in `pkg/providers/codeberg.go`.
- `github.com/docker/docker` v28.3.2+incompatible - Docker image pull and cleanup support in `pkg/providers/docker.go`.
- `github.com/coreos/go-semver` v0.3.1 and `github.com/hashicorp/go-version` v1.7.0 - version parsing, latest-release selection, and filename version inference in `pkg/providers/gitlab.go`, `pkg/providers/hashicorp.go`, `pkg/providers/docker.go`, and `pkg/providers/generic_url.go`.
- `github.com/yuin/goldmark` v1.8.1 - parses Markdown release descriptions to discover GitLab asset links in `pkg/providers/gitlab.go`.
- `golang.org/x/oauth2` v0.30.0 - authenticated GitHub and GitHub Enterprise API clients in `pkg/providers/github.go`.

**Infrastructure:**
- `github.com/caarlos0/log` v0.5.1 - structured CLI logging across `cmd/`, `pkg/config/`, and `pkg/providers/`.
- `github.com/cheggaaa/pb` v2.0.7+incompatible - progress bar support used by install/update flows in the CLI codebase.
- `github.com/fatih/color` v1.18.0 and `github.com/charmbracelet/colorprofile` v0.3.1 - terminal color and styling support in CLI output.
- `github.com/h2non/filetype` v1.1.3, `github.com/krolaw/zipstream`, and `github.com/xi2/xz` - archive/content handling used by asset extraction code under `pkg/assets/`.

## Configuration

**Environment:**
- Runtime config file is JSON managed by `pkg/config/config.go`; default location resolves from `BIN_CONFIG`, legacy `$HOME/.bin/config.json`, `XDG_CONFIG_HOME`, or `$HOME/.config/bin/config.json`.
- Binary install destination can be forced with `BIN_EXE_DIR` in `pkg/config/config.go`; this is also used by CI in `.github/workflows/build.yml`, the installer bootstrap in `install.sh`, and the GitHub Action in `action/install.js`.
- GitHub provider auth/config uses `GITHUB_AUTH_TOKEN` or `GITHUB_TOKEN`, plus `GHES_BASE_URL`, `GHES_UPLOAD_URL`, and `GHES_AUTH_TOKEN` in `pkg/providers/github.go`.
- GitLab provider auth uses `GITLAB_TOKEN` and host-specific `GITLAB_TOKEN_<hostname>` in `pkg/providers/gitlab.go`.
- Codeberg auth uses `CODEBERG_TOKEN` in `pkg/providers/codeberg.go`.
- Docker wrapper behavior can be customized with `BIN_DOCKER_RUN_TEMPLATE` in `pkg/providers/docker_unix.go` and `pkg/providers/docker_windows.go`; Docker runtime env like `DOCKER_HOST` is documented in `README.md`.
- Bootstrap installer inputs use `BIN_INSTALL_REPO`, `GITHUB_AUTH_TOKEN`, `GITHUB_TOKEN`, and `GH_TOKEN` in `install.sh`.

**Build:**
- Go release packaging and version injection are configured in `.goreleaser.yml`.
- Release PR/version automation is configured in `release-please-config.json`, `.release-please-manifest.json`, and `.github/workflows/release-please.yml`.
- Lint configuration is in `.golangci.yml`.
- Action metadata is in `action.yml` and executed by `action/setup.js` and `action/install.js`.
- Local build/test commands are defined in `Makefile`.

## Platform Requirements

**Development:**
- Go 1.24.4 to build and test per `go.mod` and `.github/workflows/build.yml`.
- Optional Node.js to run the repository GitHub Action locally because `action.yml` calls `action/setup.js` and `action/install.js`.
- Shell utilities `curl` or `wget` are required for `install.sh`; `jq` is optional for better asset selection fallback in `install.sh`; `tar` and sometimes `unzip` are required for archive extraction.
- Docker-compatible runtime is required only when using `docker://` providers through `pkg/providers/docker.go`.
- Go toolchain on PATH is required when using `goinstall://` providers through `pkg/providers/goinstall.go`.

**Production:**
- Distributed as standalone binaries for `darwin`, `linux`, and `windows` on `amd64` and `arm64`, built by `.goreleaser.yml`.
- Published as GitHub Releases and consumable through the GitHub composite action defined in `action.yml`.

---

*Stack analysis: 2026-04-10*
