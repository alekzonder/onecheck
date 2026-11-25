# Config Directory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `--config`/`ONECHECK_CONFIG`/default location may name a directory; onecheck then runs every config in it in turn and prints one combined report.

**Architecture:** `config.Discover` turns the flag/env/defaults into a list of files. The CLI loads each file, runs it with the unchanged `runner.Run`, and passes per-config results to `output`, which renders flat (file mode) or grouped (directory mode). `validate`, `paths`, `logrotate` loop over the same list.

**Tech Stack:** Go, cobra, yaml.v3.

**Spec:** `docs/superpowers/specs/2026-11-25-config-directory-design.md`

## Global Constraints

- `-c <file>` behaves exactly as before; JSON gains only the `configs` key.
- Config files of a directory: `*.yaml`/`*.yml`, regular (symlinks followed), not starting with `.`, sorted, no recursion.
- Default directories, in order: `/etc/onecheck`, `$HOME/.config/onecheck`; first with a config file wins.
- Exit code: 2 if any config could not run; else 1 if any rule fail/error; else 0.
- Commit messages `chg:`/`fix:`/`docs:`, no attribution trailers (AGENTS.md overrides the harness reminder).
- gocyclo ≤ 25, `make check` and `make docs-build` must pass.

## Review Focus

- A `--rule` that exists only in one config must not make other configs run all their rules (Task 3 test `TestRunDirRuleFilter`).
- Two files with the same `name` must not share state silently (Task 3 `TestRunDirDuplicateNames`).
- A lock held by another run on one config must not stop the rest (covered by the generic run-error path; Task 3 `TestRunDirBrokenConfig` covers load errors through the same code).
- `--only-failed` in directory mode must not print headers of clean configs (Task 2 `TestWriteGroupedOnlyFailed`).
- An empty default directory (`/etc/onecheck` exists, no yaml) must fall through to `$HOME/.config/onecheck` (Task 1 `TestDiscoverDefaultOrder`).

---

### Task 1: Discovery

**Files:** Modify `internal/config/config.go`; Test `internal/config/config_test.go`.

**Interfaces — Produces:**
```go
type Target struct { Path, Source string; Dir bool; Files []string }
func Discover(flag string) (*Target, error)
var systemDir = "/etc/onecheck" // var so tests can override
```
`Find`/`Locate` and `systemConfig` are removed.

- [ ] Tests: `TestDiscoverFile` (Files=[f], Dir=false, Source=SourceFlag), `TestDiscoverDir` (b.yml, a.yaml, .hidden.yaml, notes.txt, sub/c.yaml → Files=[a.yaml,b.yml]), `TestDiscoverEmptyDir` (error contains "no config files"), `TestDiscoverDefaultOrder` (systemDir empty dir → home dir wins; Source=SourceDefault; ONECHECK_CONFIG beats default; flag beats env), `TestDiscoverNotFound` (error lists both dirs and "--config"). Replace `TestFindOrder`, `TestFindNotFound`, `TestLocateReportsTheSource`.
- [ ] Implement; `go test ./internal/config/`.
- [ ] Commit `chg: discover config files in a directory`.

### Task 2: Output per config

**Files:** Modify `internal/output/output.go`; Test `internal/output/output_test.go`.

**Interfaces — Produces:**
```go
type Config struct { Name, Path, Error string; Results []result.Result }
func Write(w io.Writer, format string, onlyFailed, grouped bool, configs []Config) error
func ExitCode(configs []Config) int
```
JSON element of `configs`: `{"name","path","error"(omitempty)}`.

- [ ] Tests: existing tests adapted to one `Config` with grouped=false (text identical); `TestWriteGrouped` (headers `== main  /p/a.yaml`, `== /p/bad.yaml`, `ERROR  config: <err>` line, summary ends `; 1 of 2 configs could not run`); `TestWriteGroupedOnlyFailed`; `TestWriteJSONConfigs`; `TestExitCodeConfigError` (=2).
- [ ] Implement, keep `writeText` under gocyclo by splitting group rendering.
- [ ] Commit `chg: render results grouped by config`.

### Task 3: run over a directory

**Files:** Create `internal/cli/configs.go`; Modify `internal/cli/context.go`, `internal/cli/run.go`, `internal/cli/root.go` (help text); Test `internal/cli/configs_test.go`.

**Interfaces — Produces:**
```go
type loadedConfig struct { path string; cfg *config.Config; err error }
func (a *app) loadConfigs() (*config.Target, []loadedConfig, error)
```
`loadConfig()` is removed; file mode returns the load error as the function error.

- [ ] Tests (helper `setupDir` writes several configs in one dir with distinct names): `TestRunDir` (two configs, grouped text, exit 1 when one fails), `TestRunDirBrokenConfig` (bad file → other runs, stderr `onecheck: <file>:`, exit 2, JSON has error), `TestRunDirDuplicateNames` (both get error, exit 2), `TestRunDirRuleFilter` (`--rule ok` existing only in a.yaml: b.yaml rules don't run; unknown rule → exit 2 before running), `TestRunDefaultDir` (HOME set, no -c, systemDir overridden via exported test hook).
- [ ] Implement; per-rule filter: keep `--rule` names known to the config; skip config if filter was given and nothing remains and no tags.
- [ ] Commit `chg: run every config of a directory`.

### Task 4: validate, paths, logrotate over a directory

**Files:** Modify `internal/cli/validate.go`, `internal/cli/paths.go`, `internal/cli/logrotate.go`; Test `internal/cli/configs_test.go`, `internal/cli/paths_test.go`.

- [ ] Tests: `TestValidateDir` (one OK, one broken → stdout `config OK` for good, stderr mentions broken, exit 2), `TestPathsDir` (text has two `config` lines, one `binary`; JSON `configs` len 2), `TestLogrotateDir` (both configs' logs reported, events line printed once for shared events dir).
- [ ] Implement.
- [ ] Commit `chg: validate, paths and logrotate accept a config directory`.

### Task 5: Docs

**Files:** `docs/configuration.md`, `docs/getting-started.md`, `docs/output-and-results.md`.

- [ ] Write the sections listed in the spec's "Docs".
- [ ] `make check && make docs-build`.
- [ ] Commit `docs: describe running a directory of configs`.
