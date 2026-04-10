# Testing Patterns

**Analysis Date:** 2026-04-10

## Test Framework

**Runner:**
- Go `testing` package
- Config: no dedicated test config file; test execution is driven by `Makefile` and `.github/workflows/build.yml`

**Assertion Library:**
- Standard library only (`testing` with `t.Fatal`, `t.Fatalf`, and explicit comparisons)

**Run Commands:**
```bash
make test                 # Run all tests via `go test ./...`
go test ./...             # Run all tests directly
go test -v ./pkg/assets/...  # Run a focused package with verbose output
```

## Test File Organization

**Location:**
- Tests are colocated with source files in the same package directory: `cmd/install_test.go` beside `cmd/install.go`, `pkg/assets/assets_test.go` beside `pkg/assets/assets.go`.
- Package names stay the same as production code instead of using `_test` packages, allowing access to unexported helpers and package variables.

**Naming:**
- Use Go `*_test.go` files and `TestXxx` function names: `cmd/update_test.go`, `pkg/providers/generic_url_test.go`, `pkg/config/config_unix_test.go`.
- Use descriptive scenario names in subtests when table-driven tests are nested: `pkg/providers/normalize_test.go`, `pkg/config/config_test.go`, `pkg/providers/docker_test.go`.

**Structure:**
```
cmd/
  install.go
  install_test.go
  update.go
  update_test.go
pkg/
  assets/
    assets.go
    assets_test.go
  providers/
    generic_url.go
    generic_url_test.go
```

## Test Structure

**Suite Organization:**
```typescript
// Table-driven case list + loop
cases := []struct {
    in  *config.Binary
    out *updateInfo
    err string
}{ /* ... */ }

for _, c := range cases {
    if v, err := getLatestVersion(c.in, p); c.err != "" {
        if err == nil || !strings.Contains(err.Error(), c.err) {
            t.Fatalf("expected error %q, got %v", c.err, err)
        }
    } else if err != nil {
        t.Fatalf("Error during getLatestVersion(%#v, %#v): %v", c.in, p, err)
    } else if !reflect.DeepEqual(v, c.out) {
        t.Fatalf("For case %#v: %#v does not match %#v", c.in, v, c.out)
    }
}
```

**Patterns:**
- Prefer direct, readable setup inside each test over heavy shared fixtures. Example: `cmd/install_test.go` builds `bins` inline; `pkg/providers/generic_url_test.go` builds a server inline.
- Use subtests for multiple variants of the same behavior: `pkg/config/config_test.go`, `pkg/prompt/prompt_test.go`, `pkg/providers/normalize_test.go`.
- Use helper functions only when setup is repeated across a command area, such as `setupTestConfig(t)` in `cmd/export_import_test.go`.
- Cleanup is usually handled with `defer` restoring globals or state after mutation, for example restoring `resolver`, `selectOption`, and `isInteractive` in `pkg/assets/assets_test.go`, or restoring `config.Get().Bins` in `cmd/install_test.go`.

## Mocking

**Framework:**
- No mocking library; use hand-written fakes, package-variable overrides, and `httptest`

**Patterns:**
```typescript
type mockProvider struct {
    providers.Provider
    id            string
    latestVersion string
    err           error
}

func (m mockProvider) GetLatestVersion() (*providers.ReleaseInfo, error) {
    if m.err != nil {
        return nil, m.err
    }
    return &providers.ReleaseInfo{Version: m.latestVersion}, nil
}
```

```typescript
originalResolver := resolver
originalSelect := selectOption
defer func() {
    resolver = originalResolver
    selectOption = originalSelect
}()

resolver = testLinuxAMDResolver
selectOption = func(msg string, opts []fmt.Stringer) (interface{}, error) {
    t.Fatal("should not be called")
    return nil, nil
}
```

**What to Mock:**
- External interfaces and provider behavior with small local fake structs: `mockProvider` in `cmd/update_test.go`.
- Package-level seams for interactivity, OS probing, or selection UI: `resolver`, `selectOption`, `isInteractive` in `pkg/assets/assets_test.go`; `osStat` and `globFiles` in `pkg/config/config_test.go`; `stdin` in `pkg/prompt/prompt_test.go`.
- Network behavior with `httptest.NewServer` in `pkg/providers/generic_url_test.go`.

**What NOT to Mock:**
- File system interactions when temporary directories and files are cheap and deterministic. Tests use `t.TempDir()` plus real `os.WriteFile` and `os.Stat` in `cmd/export_import_test.go`, `cmd/installer_test.go`, and `pkg/config/config_unix_test.go`.
- Cobra command execution. Command tests typically instantiate the real command and run `cmd.Execute()` with controlled args, stdout, and stdin in `cmd/install_test.go`, `cmd/export_import_test.go`, and `cmd/set_config_test.go`.

## Fixtures and Factories

**Test Data:**
```typescript
func setupTestConfig(t *testing.T) string {
    t.Helper()

    cfgPath := filepath.Join(t.TempDir(), "config.json")
    defaultPath := t.TempDir()
    // marshal config, write file, set BIN_CONFIG, call config.CheckAndLoad()
    return defaultPath
}
```

**Location:**
- Fixtures are usually inline within the test file.
- Reusable setup helpers live next to the tests that need them, not in a global test package: `setupTestConfig(t)` in `cmd/export_import_test.go`.
- Synthetic assets, URLs, and provider payloads use placeholder domains like `example.test` and `example.com` throughout `cmd/update_test.go`, `pkg/assets/assets_test.go`, and `pkg/providers/generic_url_test.go`.

## Coverage

**Requirements:** None enforced

**View Coverage:**
```bash
go test ./... -cover
```

- `Makefile` has no implemented `coverage` target even though `coverage` appears in `.PHONY`.
- CI in `.github/workflows/build.yml` runs `go test ./...` but does not publish or gate on coverage percentages.

## Test Types

**Unit Tests:**
- Primary test style in this repository.
- Focus on pure helpers, parsing, selection heuristics, and command validation in `pkg/providers/normalize_test.go`, `pkg/config/config_test.go`, `cmd/install_test.go`, and `cmd/update_test.go`.

**Integration Tests:**
- Lightweight integration tests are present where behavior crosses process boundaries or packages.
- Examples include real Cobra command execution with config loading in `cmd/export_import_test.go`, file-system checks in `cmd/installer_test.go`, and HTTP request/response behavior in `pkg/providers/generic_url_test.go`.

**E2E Tests:**
- No dedicated Go E2E framework detected.
- Repository-level smoke coverage exists in GitHub Actions via `.github/workflows/build.yml` for installer script execution, binary installation, pinned update behavior, and the repository action.

## Common Patterns

**Async Testing:**
```typescript
// Not a major pattern in this repository.
// Tests are synchronous and rely on deterministic inputs.
```

- No widespread goroutine, channel, or timing-based test style was detected.
- Time-sensitive logic is usually controlled with explicit timestamps instead of sleeps, as in `cmd/update_test.go` using `time.Now().AddDate(...)`.

**Error Testing:**
```typescript
err := cmd.cmd.Execute()
if err == nil {
    t.Fatal("expected install command to reject min-age-days=0")
}
if !strings.Contains(err.Error(), "--min-age-days must be a positive integer") {
    t.Fatalf("unexpected error: %v", err)
}
```

- Error assertions usually check both presence and message content with `strings.Contains`, especially for CLI validation and provider parsing in `cmd/install_test.go`, `cmd/update_test.go`, and `pkg/providers/generic_url_test.go`.
- Nil-vs-non-nil outputs are asserted explicitly when an error path should suppress a result, as in `pkg/providers/generic_url_test.go` and `cmd/update_test.go`.

---

*Testing analysis: 2026-04-10*
