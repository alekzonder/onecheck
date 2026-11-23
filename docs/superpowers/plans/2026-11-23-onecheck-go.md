# onecheck (Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Python prototype with `onecheck`, a Go CLI that runs YAML-configured checks through executable modules, tracks pass/fail transitions, and manages its own modules and updates.

**Architecture:** The core binary loads config, discovers `onecheck-<name>` executables in the modules directory, asks each for field values over a one-shot JSON stdin/stdout protocol, and evaluates all conditions itself with one matching engine. A per-config state directory holds consecutive counters; confirmed transitions are written to a shared event queue directory. Module install/update and self-update pull assets from GitHub Releases.

**Tech Stack:** Go (current stable, 1.26), `spf13/cobra`, `gopkg.in/yaml.v3`, `tidwall/gjson`, `santhosh-tekuri/jsonschema/v6`, stdlib `log/slog`, `golangci-lint` v2.

**Spec:** `docs/superpowers/specs/2026-11-23-onecheck-go-design.md` (Russian). Read it with this plan; section numbers below (§N) refer to it.

## Global Constraints

- Go module path: `github.com/alekzonder/onecheck`. Builds use `CGO_ENABLED=0`.
- Commit messages: no `Co-Authored-By` / `Co-Author` or similar trailers. Follow the repo's existing style (`chg: ...`). No merge commits.
- `make check` must pass at the end of every task (format, `go vet`, lint incl. `gocyclo` threshold 25, `go test -race ./...`). Never raise the threshold.
- Docs, `AGENTS.md`, `--help` text, log messages: English.
- Names of configs and rules match `^[a-z0-9][a-z0-9_.-]*$`.
- All files under results/state/events are written atomically (temp file in the same dir + rename).
- Exit codes: 0 all pass, 1 any fail/error, 2 onecheck itself could not run.
- Protocol version is `1`; core supports range `[1, 1]`.
- Tests never touch the network or paths outside `t.TempDir()`.

## Review Focus

1. A module (or a `command` child of `sh -c`) that hangs past `timeout` — rule becomes `error` and the whole process group is killed; the run does not block. (Task 5 test `TestCheckTimeoutKillsGroup`.)
2. A file/body larger than `max_field_size` — module reads at most limit+1 bytes, field is listed in `truncated`, a condition on it yields `error`, never a silent pass/fail. (Task 2 `TestRunTruncates`, Task 3 `TestEvalTruncatedField`.)
3. A corrupt or empty `state/<name>/<rule>.json` — treated as a new rule with a warning, not a crash. (Task 7 `TestLoadStateCorrupt`.)
4. Events directory not writable at a transition — state file is left unchanged and the run returns an error (exit 2). (Task 8 `TestRunEventWriteFailureKeepsState`.)
5. Modules directory missing or a referenced module not installed — that rule is `error`, other rules still run, exit code 1. (Task 8 `TestRunMissingModule`.)

## File Structure

```
cmd/onecheck/main.go                 entrypoint → internal/cli
cmd/onecheck-file/main.go            file module
cmd/onecheck-http/main.go            http module
cmd/onecheck-command/main.go         command module
pkg/module/                          public SDK: protocol types, Main/Run helper
internal/version/                    Version var set by -ldflags
internal/units/                      "10MB" / "30d" parsing
internal/match/                      matching engine (compile + eval)
internal/config/                     find, load, include, env expansion, defaults
internal/modules/                    discover, describe, run a module process
internal/state/                      pure transition machine
internal/store/                      atomic files, lock, state/result/event stores
internal/result/                     Result, Event, Status types
internal/runner/                     validate + run rules end to end
internal/output/                     text/json rendering, exit code
internal/logging/                    slog setup (file + stderr)
internal/rotate/                     log rotation, event TTL cleanup
internal/github/                     Releases client (interface + HTTP impl)
internal/install/                    module search/install/update/remove, self-update
internal/cli/                        cobra commands
docs/*.md, README.md, AGENTS.md, Makefile, .golangci.yml, .github/workflows/
```

---

### Task 1: Scaffold, tooling, AGENTS.md

**Files:** delete all Python files (`onecheck/`, `pyproject.toml`, `poetry.*`, `.pylintrc`); create `go.mod`, `.gitignore`, `Makefile`, `.golangci.yml`, `AGENTS.md`, `internal/version/version.go`, `cmd/onecheck/main.go` (prints version only for now).

**Produces:** `version.Version string` (default `"dev"`, set via `-ldflags "-X github.com/alekzonder/onecheck/internal/version.Version=$(VERSION)"`).

- [ ] Remove Python code; `go mod init github.com/alekzonder/onecheck`.
- [ ] `Makefile` targets: `build` (→ `dist/onecheck`, `dist/modules/onecheck-{file,http,command}`; module list derived from `cmd/onecheck-*` dirs), `test` (`go test -race ./...`), `fmt-check` (`gofmt -l` must be empty), `vet`, `lint` (golangci-lint installed into `./bin` at a pinned version via `GOBIN=$(CURDIR)/bin go install`), `check` (= `fmt-check vet lint test`), `release-assets` (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 → `dist/release/onecheck_<os>_<arch>`, `onecheck-<module>_<os>_<arch>`, `checksums.txt` in `sha256sum` format), `clean`.
- [ ] `.golangci.yml`: standard linters plus `gocyclo` with `min-complexity: 25`.
- [ ] `AGENTS.md` per §15.
- [ ] Verify: `make build && make check` → exit 0. Commit.

### Task 2: Units and module SDK

**Files:** `internal/units/units.go` (+test), `pkg/module/protocol.go`, `pkg/module/run.go` (+tests).

**Produces:**

```go
// internal/units
func ParseBytes(s string) (float64, error)    // "10MB" → 10*1024*1024; units B,KB,MB,GB,TB (1024-based), case-insensitive; bare number = bytes
func ParseDuration(s string) (float64, error) // seconds; Go syntax plus "d" (24h) and "w"; bare number = seconds

// pkg/module
const ProtocolVersion = 1
type FieldType string // TypeString "string", TypeNumber, TypeBool, TypeObject, TypeArray, TypeBytes "bytes", TypeDuration "duration"
type FieldInfo struct { Type FieldType `json:"type"`; Description string `json:"description"` }
type Description struct {
    Protocol int `json:"protocol"`; Name, Version, Description string
    ParamsSchema json.RawMessage `json:"params_schema"`; Fields map[string]FieldInfo `json:"fields"`
}
type Request struct {
    Protocol int; Rule string; Params json.RawMessage; Fields []string
    MaxFieldSize int64 `json:"max_field_size"`; TimeoutMS int64 `json:"timeout_ms"`
}
func (r Request) Wants(field string) bool
type Error struct { Message string `json:"message"` }
type Response struct { Fields map[string]any `json:"fields"`; Truncated []string `json:"truncated"`; Error *Error `json:"error"` }
type CollectFunc func(ctx context.Context, req Request) (map[string]any, error)
type Module struct { Description Description; Collect CollectFunc }
func Run(m Module, args []string, stdin io.Reader, stdout, stderr io.Writer) int
func Main(m Module) // os.Exit(Run(m, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
```

`Run`: `describe` prints the description (protocol forced to `ProtocolVersion`); `check` decodes a `Request`, rejects a protocol mismatch, calls `Collect` with a context bounded by `TimeoutMS`, and writes a `Response`. A `Collect` error becomes `error.message` with exit 0. String fields longer than `MaxFieldSize` (>0) are cut to that size and named in `truncated`. Unknown subcommand → usage on stderr, exit 2.

- [ ] Tests first: `TestParseBytes` (`"10MB"`→10485760, `"1.5KB"`→1536, `"7"`→7, `"x"`→error), `TestParseDuration` (`"1h"`→3600, `"30d"`→2592000, `"1.5"`→1.5), `TestRunDescribe`, `TestRunCheck`, `TestRunCollectError` (exit 0, error set), `TestRunTruncates`, `TestRunProtocolMismatch`.
- [ ] Implement; `make check`; commit.

### Task 3: Matching engine (§8)

**Files:** `internal/match/{compile.go,ops.go,eval.go,json.go}` (+ table tests).

**Produces:**

```go
type Failure struct {
    Path string `json:"path"`; JSONPath string `json:"json_path,omitempty"`; Operator string `json:"operator"`
    Expected any `json:"expected"`; Actual any `json:"actual"`; Message string `json:"message"`
}
type FieldTypes map[string]module.FieldType
type Check struct{ /* compiled */ }
func Compile(raw map[string]any, types FieldTypes) (*Check, error)
func (c *Check) Fields() []string                       // sorted top-level field names used
func (c *Check) Eval(fields map[string]any, truncated []string) ([]Failure, error)
```

Decisions:

- Keys of `raw` are gjson paths into the fields object; the first path segment must be a key of `types` (when `types` is non-nil), else compile error naming known fields.
- A condition is a scalar (→ `equals`) or a map whose keys are operators from §8 or `all`/`any`/`not`; multiple keys are ANDed. Unknown operator, wrong argument shape, bad regex, invalid JSON Schema → compile error.
- `gt/gte/lt/lte` arguments: numbers; strings are parsed with `units.ParseBytes`/`ParseDuration` only when the path is exactly a field typed `bytes`/`duration`, otherwise a string argument is a compile error.
- `equals`: deep JSON equality; numbers compare as float64; no string↔number coercion.
- Missing path: every operator fails except `exists: false`; `not` over a missing path is true only if the inner condition is.
- Operator not applicable to the value's type → Failure whose message names the actual type.
- `json`: value must be a string holding valid JSON, else Failure (`operator: "json"`). `assert` items are `{path, ...operators}` evaluated on the parsed document (`JSONPath` set in failures); `schema` is draft 2020-12, each validation error is one Failure with `operator: "schema"`.
- `any` produces one Failure (`operator: "any"`) when no branch holds.
- `Eval` returns an error (not failures) when a used top-level field is in `truncated`.
- `Actual` strings in failures are cut to 1024 bytes. Message format: `<path>[ json <json_path>]: expected <operator> <expected JSON>, got <actual JSON>`; missing path → `got nothing (path is missing)`.

- [ ] Tests first, table-driven: one passing and one failing case per operator; `TestShorthandEquals`; `TestCombinators`; `TestMissingPath`; `TestTypedUnits` (`size: {lt: 10MB}`, `age: {lt: 1h}`); `TestNestedPath` (`headers.content-type`); `TestJSONAssert` (incl. gjson `items.#`); `TestJSONSchema`; `TestJSONInvalid`; `TestCompileErrors` (unknown op, unknown field, bad regex, string arg on plain number); `TestEvalTruncatedField`; `TestFailureMessage` with the exact string `body json status: expected equals "ok", got "degraded"`.
- [ ] Implement; keep each operator a small function in a table (complexity limit); `make check`; commit.

### Task 4: Config (§6)

**Files:** `internal/config/{config.go,load.go,find.go,types.go}` (+tests with `testdata/`).

**Produces:**

```go
type Size int64                 // UnmarshalYAML via units.ParseBytes
type Duration time.Duration     // UnmarshalYAML via units.ParseDuration
type Rotate struct { MaxSize Size; Keep int; MaxAge Duration; Compress bool }
type Config struct {
    Name string; Path string // absolute path of the main file
    Log struct{ Level, Dir, Format string; Rotate Rotate }
    Paths struct{ Modules, Results, Events, State string }
    Include []string
    Events struct{ TTL Duration }
    Run struct{ Parallel int; MaxFieldSize Size }
    Defaults struct{ Timeout Duration; FailAfter, PassAfter int }
    Modules struct{ Repos []string }
    Rules []Rule
}
type Rule struct {
    Name, Module string; Tags []string; Enabled bool
    Timeout time.Duration; FailAfter, PassAfter int
    Params map[string]any; Check map[string]any
    Source string // file the rule came from
    Err error     // rule-level problem found at load (e.g. undefined ${VAR}, bad name)
}
func Find(flag string) (string, error)   // flag → $ONECHECK_CONFIG → /etc/onecheck/config.yaml → $HOME/.config/onecheck/config.yaml
func Load(path string) (*Config, error)  // error = whole-config error (exit 2)
func (c *Config) ResultsDir() string     // <paths.results>/<name>
func (c *Config) StateDir() string       // <paths.state>/<name>
func (c *Config) LogFile() string        // <log.dir>/<name>/onecheck.log
```

Decisions: defaults — `log.level info`, `format text`, rotate `10MB/5/30d/compress true`, `events.ttl 24h`, `run.parallel 4`, `max_field_size 10MB`, `defaults 30s/1/1`, `modules.repos [alekzonder/onecheck]`, `enabled true`. Default dirs: euid 0 → §6 values; otherwise `$HOME/.local/state/onecheck/{results,events,state,log}` and `$HOME/.local/lib/onecheck/modules`. Whole-config errors: unreadable/invalid YAML, unknown keys (`KnownFields(true)`), missing/invalid `name`, duplicate rule name (message names both files), include file with keys other than `rules`, `fail_after`/`pass_after` < 1. Include globs resolve relative to the main file's dir, sorted lexicographically; a glob matching nothing is fine. `${VAR}` expansion applies to string values inside `params` recursively; `$$` escapes a literal `$`; undefined variable or invalid rule name/missing module sets `Rule.Err` instead of failing the load. `Find` error lists every path tried.

- [ ] Tests first: `TestLoadDefaults`, `TestLoadFull`, `TestInclude` (order, relative), `TestDuplicateRule`, `TestUnknownKey`, `TestMissingName`, `TestEnvExpansion`, `TestEnvUndefinedIsRuleError`, `TestFindOrder`, `TestFindNotFound`.
- [ ] Implement; `make check`; commit.

### Task 5: Module registry and process execution (§7)

**Files:** `internal/modules/{modules.go,exec.go}` (+tests using a tiny test module built in `TestMain` from `internal/modules/testdata/fakemod/main.go`).

**Produces:**

```go
const MinProtocol, MaxProtocol = 1, 1
type Info struct { module.Description; Path string }
func (i *Info) Compatible() bool
func Path(dir, name string) string                       // <dir>/onecheck-<name>
func Describe(ctx context.Context, path string) (*Info, error) // 5s cap; validates JSON and non-empty name
func List(ctx context.Context, dir string) ([]*Info, []error)  // every executable onecheck-* file; missing dir → empty
func Check(ctx context.Context, info *Info, req module.Request, timeout time.Duration, stderr io.Writer) (*module.Response, error)
```

`Check` runs the module in its own process group (`Setpgid`), kills the group on timeout (plus `WaitDelay`), caps stdout at `max_field_size*N+1MB` reading, and returns an error for timeout, non-zero exit, invalid JSON. Files ending in `.tmp` or named `installed.json` are ignored by `List`.

- [ ] Tests first: `TestDescribe`, `TestDescribeGarbage`, `TestListIgnoresNonModules`, `TestListMissingDir`, `TestCheckOK`, `TestCheckNonZeroExit`, `TestCheckInvalidJSON`, `TestCheckTimeoutKillsGroup` (fake module spawns `sleep 60` child; assert return within ~2s and child gone).
- [ ] Implement; `make check`; commit.

### Task 6: Modules file, http, command (§9)

**Files:** `cmd/onecheck-file/{main.go,collect.go}`, `cmd/onecheck-http/{main.go,collect.go}`, `cmd/onecheck-command/{main.go,collect.go}` (+tests calling the collect functions directly).

**Consumes:** `pkg/module`. Each `main.go` is `module.Main(mod)`; `Description.Version = version.Version`; each has a `params_schema` with `additionalProperties: false`.

Decisions:

- **file** — fields exactly as §9. `Lstat` for `type`/`target`; other fields from `Stat` (followed). Dangling link → `{exists:false, type:"symlink", target}`. Missing path → `{exists:false}` only. `content` read up to `MaxFieldSize+1` bytes, only when requested and a regular file; `size` of a dir = recursive sum of regular files, computed only when requested; `mode` as `fmt.Sprintf("%04o", perm)`; owner/group names with numeric fallback. Stat errors other than not-exist → Collect error.
- **http** — params `url` (required), `method` (default `GET`), `headers`, `body`, `follow_redirects` (default true), `insecure_skip_verify` (default false). Fields `status`, `headers` (lower-cased names, values joined with `, `), `body` (≤ limit+1), `duration` (seconds, float), `tls_expires_in` (seconds until leaf cert `NotAfter`; absent for plain http). Transport errors → Collect error.
- **command** — params: exactly one of `run` (string → `sh -c`) or `argv` (list); `env` (map, added to current env), `cwd`, `stdin`. Fields `exit_code`, `stdout`, `stderr` (each ≤ limit+1), `duration`. Non-zero exit is a value. Start failure (binary not found) → Collect error. Child runs in its own process group and is killed on context cancel.

- [ ] Tests first — file: `TestFileRegular`, `TestFileMissing`, `TestDirSizeCount`, `TestSymlink`, `TestDanglingSymlink`, `TestContentLimit`, `TestContentNotReadUnlessWanted`; http (`httptest`): `TestHTTPBasic`, `TestHTTPHeadersLowercased`, `TestHTTPNoFollow`, `TestHTTPTLSExpiry`, `TestHTTPConnError`; command: `TestRunShell`, `TestArgv`, `TestExitCodeIsValue`, `TestNotFoundIsError`, `TestEnvCwdStdin`, `TestRunXorArgv`.
- [ ] Implement; `make build` produces three module binaries; `make check`; commit.

### Task 7: Result types, state machine, store (§10)

**Files:** `internal/result/result.go`, `internal/state/state.go` (+test), `internal/store/{atomic.go,store.go,lock.go}` (+tests).

**Produces:**

```go
// internal/result
type Status string // Pass "pass", Fail "fail", Error "error"
type Result struct {
    Config, Rule, Module string; ModuleVersion string `json:"module_version"`; Tags []string
    Status Status; State string; Transition *string; Consecutive int
    FailAfter int `json:"fail_after"`; PassAfter int `json:"pass_after"`
    StartedAt time.Time `json:"started_at"`; DurationMS int64 `json:"duration_ms"`
    Failures []match.Failure; Error *string
} // json keys snake_case as in §10; Failures marshals as [] not null
type Event struct { ID, Config, Rule, Transition, From, To string; At time.Time; ExpiresAt time.Time `json:"expires_at"`; Result Result }

// internal/state
type State struct { State string `json:"state"`; Since time.Time `json:"since"`; ConsecutivePass int `json:"consecutive_pass"`; ConsecutiveFail int `json:"consecutive_fail"` }
func Advance(prev *State, ok bool, failAfter, passAfter int, now time.Time) (next State, transition string) // transition "", "fail", "recovered"

// internal/store
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error
type Store struct{ /* dirs, config name */ }
func Open(cfg *config.Config) (*Store, error)            // mkdir -p results/<name>, state/<name>, events
func (s *Store) Lock() (unlock func(), err error)        // flock state/<name>/.lock non-blocking; ErrLocked; then verify/write state/<name>/.owner == cfg.Path, mismatch → error naming both paths
func (s *Store) LoadState(rule string) (*state.State, error) // nil,nil when absent; corrupt → nil + ErrCorrupt (caller warns, treats as new)
func (s *Store) SaveState(rule string, st state.State) error
func (s *Store) WriteResult(r result.Result) error
func (s *Store) WriteEvent(e result.Event) error          // events/<id>.json
func (s *Store) Prune(keep []string) error                // remove result/state files of rules not in keep
func EventID(at time.Time, cfg, rule, transition string) string // 20261123T100200Z-main-api-health-fail
```

`Advance`: `prev == nil` starts from `pass` with zero counters and `Since = now`. A pass increments `ConsecutivePass` and zeroes `ConsecutiveFail` (and vice versa). In state `pass`, `ConsecutiveFail >= failAfter` → state `fail`, transition `fail`; in state `fail`, `ConsecutivePass >= passAfter` → `pass`, `recovered`. `Result.Consecutive` is the counter matching the current run's status.

- [ ] Tests first: `TestAdvance` table — new+pass (no event), new+fail with fail_after 1 (event), fail_after 3 needs three in a row, a pass in between resets, recovery with pass_after 2, flapping; `TestWriteFileAtomic` (no temp left, content replaced); `TestLockExclusive`; `TestLockOwnerMismatch`; `TestLoadStateCorrupt`; `TestWriteEventName`; `TestPrune`.
- [ ] Implement; `make check`; commit.

### Task 8: Runner and output (§5, §6 error levels, §10, §11)

**Files:** `internal/runner/{runner.go,prepare.go,validate.go}`, `internal/output/output.go` (+tests; runner tests build the real modules once in `TestMain` into a temp dir).

**Produces:**

```go
// internal/runner
type Options struct { Rules, Tags []string }
func Run(ctx context.Context, cfg *config.Config, opts Options, log *slog.Logger) ([]result.Result, error)
func Validate(ctx context.Context, cfg *config.Config) []error // every rule-level problem, prefixed "<source>: rule <name>: "

// internal/output
func Write(w io.Writer, format string, onlyFailed bool, results []result.Result) error // "text" | "json"
func ExitCode(results []result.Result) int // 0 or 1
```

`prepare` (shared by Run and Validate) turns a rule into a ready check or a rule-level error: `Rule.Err`, module missing (`module "x" is not installed in <dir>`), incompatible protocol, `params` vs `params_schema`, `match.Compile` errors. A rule with an empty `check` is valid (passes when the module returns no error).

`Run`: open store, lock (error → returned, CLI exits 2), select rules (`enabled`, `--rule`, `--tag`; an unknown `--rule` name is an error), describe each referenced module once, execute rules with a worker pool of `run.parallel`, results returned in config order. Per rule after execution, in this order: load state (corrupt → warn, treat as new) → `Advance` (ok = status pass) → build Result → if transition: `WriteEvent` (failure → abort the run with an error, state not saved) → `SaveState` → `WriteResult`. `Prune` runs only when no `--rule`/`--tag` filter was given and no error occurred. Module stderr is logged at debug.

Text output: one line per shown rule `PASS|FAIL|ERROR  <rule>  (<module>, <ms>ms)`, `[state: fail]`-style suffix only when state differs from status or a transition happened, indented failure messages beneath, final line `N passed, M failed, K errors`. JSON: `{"summary":{"total","passed","failed","errors"},"results":[...]}`; summary always counts all rules.

- [ ] Tests first: `TestRunPassAndFail` (file module on temp files; result + state files exist), `TestRunTransitionWritesEvent` (fail_after 2: no event on run 1, event on run 2, `recovered` after fix), `TestRunMissingModule`, `TestRunBrokenRuleOthersContinue` (bad operator), `TestRunEventWriteFailureKeepsState` (events path is a file), `TestRunLocked`, `TestRunFilterByRuleAndTag`, `TestRunPrunesRemovedRules`, `TestValidateReportsAll`; output: `TestTextAll`, `TestTextOnlyFailed`, `TestJSONShape`, `TestExitCode`.
- [ ] Implement; `make check`; commit.

### Task 9: Logging and CLI: run, validate, version (§5, §12)

**Files:** `internal/logging/logging.go` (+test), `internal/cli/{root.go,run.go,validate.go,version.go,context.go}` (+tests), `cmd/onecheck/main.go`.

**Produces:**

```go
// internal/logging
func Setup(file, level, format string, stderr io.Writer) (*slog.Logger, io.Closer, error) // file handler at level + stderr handler at warn+; creates the dir
// internal/cli
func Execute(args []string, stdout, stderr io.Writer) int // returns exit code
```

Root: `Use: "onecheck"`, running with no subcommand behaves as `run` (run's flags are also registered on root). Persistent flags `-c/--config`, `--log-level` (overrides config). Every command has `Short`, `Long`, `Example`. `SilenceUsage` on runtime errors; errors print to stderr as `onecheck: <err>`. `validate` prints each error on its own line and exits 2 if any, else prints `config OK: N rules`. `version` prints `onecheck <version> (protocol 1)`. If the log file cannot be opened, warn on stderr and continue logging to stderr only.

- [ ] Tests first (through `Execute` with temp config + built modules): `TestRunDefaultCommand`, `TestRunExitCodes` (0, 1, 2 for missing config), `TestRunJSONOnlyFailed`, `TestValidateOKAndErrors`, `TestLogLevelFlagOverridesConfig`, `TestHelpListsCommands`.
- [ ] Implement; manual check `dist/onecheck --help`; `make check`; commit.

### Task 10: logrotate (§12)

**Files:** `internal/rotate/{logs.go,events.go}` (+tests), `internal/cli/logrotate.go`.

**Produces:**

```go
func Logs(file string, r config.Rotate, now time.Time) (rotated bool, err error)
func Events(dir string, ttl time.Duration, now time.Time) (removed int, err error)
```

`Logs`: when `file` size > `MaxSize`, shift `file.N[.gz]` → `N+1`, move `file` → `file.1` (gzip when `Compress`); then always delete rotated files with index > `Keep` or mtime older than `MaxAge`. `Events`: remove `*.json` whose `expires_at` is past; files that fail to parse are removed when mtime is older than `ttl`; leftover `*.tmp` older than `ttl` removed too. CLI: `onecheck logrotate [--logs] [--events]` (neither flag = both), prints a one-line summary.

- [ ] Tests first: `TestLogsBelowSizeNoop`, `TestLogsRotateShiftCompress`, `TestLogsKeepAndMaxAge`, `TestEventsExpired`, `TestEventsUnparseableOld`, `TestLogrotateCommand`.
- [ ] Implement; `make check`; commit.

### Task 11: GitHub releases, module management, self-update (§13)

**Files:** `internal/github/github.go` (+test with `httptest`), `internal/install/{manager.go,selfupdate.go,manifest.go}` (+tests with a fake client), `internal/cli/{module.go,selfupdate.go}`.

**Produces:**

```go
// internal/github
type Asset struct { Name, URL string }
type Release struct { Tag string; Prerelease bool; Assets []Asset }
type Client interface {
    Releases(ctx context.Context, repo string) ([]Release, error) // newest first, drafts excluded
    Download(ctx context.Context, a Asset, w io.Writer) error
}
func New(baseURL, token string) Client // baseURL "" → https://api.github.com; token from $GITHUB_TOKEN by the CLI

// internal/install
type Found struct { Repo, Tag, Name string; Asset github.Asset }
type Installed struct { Repo, Version, SHA256 string }       // installed.json: map name → Installed
type Manager struct { Client github.Client; Dir, OS, Arch string }
func (m *Manager) Search(ctx context.Context, repos []string) ([]Found, error)
func (m *Manager) Install(ctx context.Context, repo string, specs []string) ([]Found, error) // spec "name" or "name@version"; none = all modules of the latest release
func (m *Manager) Update(ctx context.Context, names []string) (updated []Found, skipped []string, err error)
func (m *Manager) Remove(names []string) error
func SelfUpdate(ctx context.Context, c github.Client, repo, version, exe, goos, goarch string) (newVersion string, err error)
```

Decisions: module asset name `onecheck-<name>_<os>_<arch>`; core asset `onecheck_<os>_<arch>`. "Latest" = newest non-prerelease. `@version` accepts `1.2.0` or `v1.2.0`. Install steps per §13 exactly: download to `<Dir>/.<asset>.tmp` → sha256 must match the `checksums.txt` entry (missing file or entry → error) → `chmod 0755` → `modules.Describe` (name must equal the asset's module name, protocol in range) → rename to `onecheck-<name>` → update `installed.json` atomically. Any failure removes the temp file and leaves the old module intact. `Update` skips modules absent from `installed.json` (reported in `skipped`), skips when the latest tag equals the installed version, and skips an incompatible release with a reason. `SelfUpdate` downloads next to `exe`, verifies sha256, runs `<tmp> version` (must exit 0), renames over `exe`; same version → no-op returning `""`. CLI: `module list` (table: name, version, protocol, compatible, repo), `module search [repo...]` (defaults to `modules.repos`), `module install <owner/repo> [name[@version]]...`, `module update [name...]`, `module remove <name>...`, `self-update [--version X]`. These commands work without a config file when `--modules-dir` is given; otherwise they use the config's `paths.modules`.

- [ ] Tests first: github — `TestReleasesParsesAndSendsToken`, `TestDownload`, `TestAPIError` (404/403 message includes repo); install — `TestSearchMatchesPlatformAssets`, `TestInstallLatestAll`, `TestInstallPinnedVersion`, `TestInstallChecksumMismatch` (nothing installed, no temp left), `TestInstallIncompatibleProtocol`, `TestInstallWrongName`, `TestUpdate`, `TestUpdateUpToDate`, `TestRemove`, `TestSelfUpdate`, `TestSelfUpdateChecksumMismatch` (exe untouched); cli — `TestModuleListCommand`.
- [ ] Implement; `make check`; commit.

### Task 12: Docs, README, CI, release workflow (§14, §15)

**Files:** `README.md`, `docs/{getting-started,configuration,checks,state-and-events,output-and-results,module-management,logging-and-logrotate,module-protocol,development}.md`, `docs/modules/{file,http,command}.md`, `examples/config.yaml`, `examples/conf.d/web.yaml`, `.github/workflows/{check.yml,release.yml}`; test `internal/config/examples_test.go`.

- [ ] Test first: `TestExamplesLoadAndValidate` — `examples/config.yaml` loads and (with built modules) `runner.Validate` returns no errors, so documented examples cannot rot.
- [ ] Write docs: every operator in `checks.md` with a YAML example; every field of every module; the result and event JSON contracts with all keys; cron and systemd-timer snippets for `run` and `logrotate`; a minimal third-party module in `module-protocol.md` (Go via `pkg/module`, plus the raw stdin/stdout exchange for other languages).
- [ ] Workflows: `check.yml` runs `make check` on push and PR; `release.yml` on tag `v*` runs `make release-assets VERSION=${tag#v}` and `gh release create` with `dist/release/*`.
- [ ] End-to-end manual run: `make build`, run `dist/onecheck -c examples/config.yaml` with temp paths, confirm text and `-o json` output, results/state/events files, `logrotate`. `make check`; commit.
