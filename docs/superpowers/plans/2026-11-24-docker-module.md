# docker module and array conditions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core operators `glob`, `every`, `some` and `path` in conditions, plus a bundled `docker` module listing containers with a uniform shape.

**Architecture:** `internal/glob` compiles shell patterns to anchored regexps and is shared by `internal/match` and the module. New nodes in `internal/match`. The module talks to the Docker Engine HTTP API over a unix socket or TCP with net/http; host resolution is a pure function over env, files and GOOS.

**Tech Stack:** Go 1.26 standard library, `github.com/tidwall/gjson` (already used).

**Spec:** `docs/superpowers/specs/2026-11-24-docker-module-design.md`

## Global Constraints

- Linux and macOS; `CGO_ENABLED=0`; no new dependencies.
- Modules only collect; filtering by `name`/`label` params is selection, not checking.
- `make check` passes; gocyclo ≤ 25, no `//nolint:gocyclo`.
- Tests: no network beyond loopback/unix sockets, nothing outside `t.TempDir()`.
- Commits `chg:`/`fix:`/`docs:`, no attribution trailers, no merge commits.

## Review Focus

1. `containers.#(state=="running")#` when no container runs — gjson must yield an existing empty array so `every` passes (test in Task 2 with real gjson lookup through `Eval`).
2. Element failure inside `every` → `any`: message must contain the element's `name` (test in Task 2).
3. Docker socket present but permission denied — error names address, source and the docker group hint (Task 3).
4. Container removed between list and inspect (404) — skipped, not an error (Task 3).
5. Glob pattern with regexp metacharacters (`registry.local/*`, `a+b`) — `.`/`+` literal (Task 1).

---

### Task 1: `internal/glob` and the `glob` / `not_glob` operators

**Files:** Create `internal/glob/glob.go`, `internal/glob/glob_test.go`; modify `internal/match/ops.go`, `internal/match/match_test.go`, `docs/checks.md`.

**Interfaces — Produces:** `glob.Compile(pattern string) (*regexp.Regexp, error)` (anchored `^...$`, `(?s)`), `glob.Has(pattern string) bool` (contains `*`, `?` or `[`).

- [ ] Tests: `TestCompile` table: `app-*` matches `app-web`, not `xapp-web`; `registry.local/*` matches `registry.local/team/app:1`, not `registryxlocal/a`; `a+b` matches only `a+b`; `?` one char; `[a-c]x`, `[!a]x`; `[` → error. Match tests: `glob` pass/fail, `not_glob`, non-string value note, compile error for non-string arg and bad pattern.
- [ ] Run `go test ./internal/glob ./internal/match` — FAIL (undefined).
- [ ] Implement: translate rune by rune, `regexp.QuoteMeta` for literals, classes copied with `!`→`^`; register `glob`, `not_glob` in `leafOps`.
- [ ] Run — PASS. Document in `docs/checks.md` (operator table + section).
- [ ] Commit `chg: add the glob operator for shell patterns`.

### Task 2: `path`, `every`, `some`

**Files:** Modify `internal/match/compile.go`, `internal/match/eval.go`, `internal/match/match_test.go`, `docs/checks.md`.

**Interfaces — Produces:** nodes `pathNode{path string; inner node}`, `everyNode{inner node; raw any}`, `someNode{inner node; raw any}`. Recorder path for an element: `<path>.<index>`; under `path`: `<path>.<sub>`.

- [ ] Tests (through `Compile` + `Eval`):
  - `TestPathInCondition`: `{"c.0": {path: state, equals: running}}` pass/fail; failure Path `c.0.state`.
  - `TestEvery`: all match → no failures; one bad → one failure whose message starts `c.1.state:`.
  - `TestEveryEmptyArrayHolds`, `TestSomeEmptyArrayFails`, `TestEveryNotArray`, `TestEveryMissing`.
  - `TestEveryListIsAll`: `every: [cond, cond]`.
  - `TestEveryAnyNamesElement`: element `{name: miner, image: x}` failing `any` → message contains `"miner"`.
  - `TestEveryFilteredNoMatches`: fields `{c: [{state: exited}]}`, path `c.#(state=="running")#` with `every` → passes.
  - `TestSome` pass/fail with message `expected some`.
  - Compile errors: `{path: x}` alone, `path` not a string, `every` with empty list.
- [ ] Run `go test ./internal/match` — FAIL.
- [ ] Implement: in `compileCond`, if the map has `path`, compile the rest with ft "" and wrap in `pathNode`; `every`/`some` in `compileOp` (list → `compileList` as all). `pathNode.eval` marshals `v.v` and looks up via gjson; `missingAware` = inner's. Add names to `operatorNames`.
- [ ] Run — PASS; document `every`, `some`, `path` in `docs/checks.md`.
- [ ] Commit `chg: add every, some and path to check array elements`.

### Task 3: `docker` module

**Files:** Create `cmd/onecheck-docker/{main.go,collect.go,host.go,client.go}` with tests `collect_test.go`, `host_test.go`.

**Interfaces:**
- `type endpoint struct{ addr, source string }`; `resolveHost(param string, env func(string) string, home, goos string, exists func(string) bool) (endpoint, error)`.
- `newClient(ep endpoint) (*client, error)`; `(*client).get(ctx, path string, out any) (status int, err error)`.
- `containerFields(list listItem, in inspect, now time.Time) map[string]any`.
- package var `resolve = func(param string) (endpoint, error)` defaulting to `resolveHost(param, os.Getenv, home, runtime.GOOS, fileExists)`.

- [ ] Tests:
  - host: param wins; `DOCKER_HOST`; context via `DOCKER_CONTEXT` and via config `currentContext` (meta dir named by sha256 hex); `default` context skipped; socket candidates in order for linux and darwin; nothing found → error listing paths; `ssh://` → unsupported error.
  - collect against fake API on `unix://<tmp>/d.sock`: two containers, uniform 10 element keys, sorted by name, `count`; `name` exact and glob; `label` key and key=value; fields `["count"]` → no inspect request; inspect 404 → skipped; list 500 with `{"message":"boom"}` → error contains `boom`; health `none` without Health; `uptime_seconds` from StartedAt; socket missing → error naming address and source; describe lists `containers` array and `count` number.
- [ ] Run `go test ./cmd/onecheck-docker` — FAIL.
- [ ] Implement per spec; HTTP over unix via `http.Transport{DialContext}` with base URL `http://docker`; permission-denied hint `add the user to the docker group or run as root`.
- [ ] Run — PASS; e2e against the real daemon if present.
- [ ] Commit `chg: add the docker module that reports containers`.

### Task 4: install and docs

**Files:** `install.sh`, `internal/installsh/install_test.go`, `docs/modules/docker.md`, README.md, docs/getting-started.md, docs/module-management.md, docs/development.md, AGENTS.md.

- [ ] Update test expectation `... memory@1.2.0 docker@1.2.0 --modules-dir`, run → FAIL; add `docker` to `MODULES` → PASS.
- [ ] Write `docs/modules/docker.md`: params, host discovery, fields, recipes (required container, no strangers, compose project, crash loop, OOM), FAIL sample. Add module to every list.
- [ ] `make check` and `make release-assets` — pass.
- [ ] Commit `chg: install the docker module with install.sh and document it`.
