# Config directory: run every config of a host

## Why

`onecheck` without arguments must show the current state of the whole server.
A host often has several configs (one per interval, per team, per package),
and today one invocation runs exactly one of them. When `--config` (or
`$ONECHECK_CONFIG`, or the default location) names a directory, onecheck runs
every config in it, one after another, as if each had been started with
`-c <file>`, and prints one combined report.

## Goals

- `-c <dir>` and `ONECHECK_CONFIG=<dir>` run every config file in the
  directory.
- `onecheck` without `-c` uses the default config directory: the first of
  `/etc/onecheck/` and `$HOME/.config/onecheck/` that contains a config file.
- One report: text grouped by config, one JSON document, one exit code.
- `validate`, `paths` and `logrotate` accept a directory too.

## Non-goals

- No recursion into subdirectories. `conf.d/` with include files keeps working
  untouched next to `config.yaml`.
- No new execution model: each config still runs with its own lock, log,
  `run.parallel`, paths, state, results and events.
- No parallelism between configs.

## Anti-goals

- A broken config must not hide the others: they still run and are reported.
- A broken config must not be silently skipped (no "a file without `name` is
  an include" guessing): it is an error in the report.
- `-c <file>` must behave exactly as before (same text output, same errors on
  stderr, same exit codes). JSON gets one additive key.

## Approaches

- **A. Loop in the CLI (chosen).** Discovery returns a list of files; the CLI
  loads, logs and runs each with the existing `runner.Run`, collects the
  results and hands them to `output`. Runner, state and store are untouched,
  each config runs exactly as a separate invocation would.
- B. One runner pass over all configs with a shared worker pool. Faster, but
  locks, logs, `run.parallel` and paths are per config; this is a new
  execution model and risks the state/event guarantees.
- C. onecheck re-executes itself per file and merges the JSON. Full process
  isolation that A already gives in practice, at the cost of extra processes,
  parsing its own output and awkward error/signal handling.

## Design

### Discovery (`internal/config`)

`Discover(flag string) (*Target, error)` replaces `Find`/`Locate`:

```go
type Target struct {
    Path   string   // what was given or found: a file or a directory
    Source string   // SourceFlag, SourceEnv, SourceDefault
    Dir    bool     // Path is a directory
    Files  []string // config files to load, in order
}
```

- The path comes from `--config`, else `$ONECHECK_CONFIG`. If it is a file:
  `Files = [path]`, `Dir = false`. If it is a directory: its config files.
  A missing path is an error, as today.
- Without both: the default directories `/etc/onecheck` and
  `$HOME/.config/onecheck`; the first that contains at least one config file
  wins (`Dir = true`). None: an error listing the directories tried and
  suggesting `--config` / `ONECHECK_CONFIG`.
- Config files of a directory: entries ending in `.yaml` or `.yml` that are
  regular files (symlinks followed), not starting with `.`, sorted by name.
  No recursion. An explicitly given directory with no config file is an
  error: `no config files (*.yaml, *.yml) in <dir>`.

### Loading a set (`internal/cli`)

`loadConfigs()` returns the target and one entry per file: path, the loaded
`*config.Config` or the load error. `--log-level` is validated once (a bad
value is an error for the whole invocation, exit 2) and applied to every
loaded config. Two configs with the same `name` in one directory would share
state, results and logs: both get the error
`name "x" is also used by <other file>`.

In file mode (`Dir = false`) a load error is returned as today: printed to
stderr, exit 2, nothing on stdout.

### `run`

For each config, in order: open its log, log `run started`, `runner.Run`,
log `run finished`. A config that failed to load, or whose run failed (lock
held by another run, files not writable), is recorded as a config error and
the next config runs. The error also goes to stderr as
`onecheck: <file>: <error>` and to that config's log when it has one. If the
context is cancelled (SIGINT/SIGTERM), no further config starts and the
command fails with exit 2, as a single interrupted run does today.

`--rule` and `--tag` apply to every config. Each `--rule` name must exist in
at least one loaded config, else exit 2 before anything runs
(`no rule named "x" in <dir>`). A config gets only the `--rule` names it
knows; a config with none of them and no `--tag` is skipped (it must not run
everything because its filter became empty).

In file mode, run behaves exactly as today.

### Output (`internal/output`)

`output.Write(w, format, onlyFailed, grouped, configs []output.Config)` where

```go
type Config struct {
    Name    string // config name, "" if it could not be loaded
    Path    string // config file
    Error   string // why the config could not run, "" if it ran
    Results []result.Result
}
```

`grouped` is true in directory mode.

Text, not grouped: exactly today's output. Text, grouped:

```
== main  /etc/onecheck/config.yaml
PASS   passwd-sane   (file, 2ms)

== web  /etc/onecheck/web.yaml
FAIL   api-health    (http, 124ms)  [transition: fail]
       body json status: expected equals "ok", got "degraded"

== /etc/onecheck/broken.yaml
ERROR  config: /etc/onecheck/broken.yaml: "name" is required: ...

2 rules: 1 passed, 1 failed, 0 errors; 1 of 3 configs could not run
```

- Header `== <name>  <path>`, or `== <path>` when the name is unknown.
- With `--only-failed`, a config with nothing to show (all passed, no error)
  is left out entirely.
- The summary counts all rules of all configs; `; N of M configs could not
  run` is appended only when N > 0.

JSON (both modes) gains `configs`; `summary` and `results` are unchanged in
shape, `results` is the concatenation in config order:

```json
{
  "summary": { "total": 2, "passed": 1, "failed": 1, "errors": 0 },
  "configs": [
    { "name": "main", "path": "/etc/onecheck/config.yaml" },
    { "name": "", "path": "/etc/onecheck/broken.yaml", "error": "..." }
  ],
  "results": [ ... ]
}
```

Exit code: 2 if any config could not run; else 1 if any rule is `fail` or
`error`; else 0.

### `validate`

Each config is validated. OK configs print `config OK: N rules (<file>)` on
stdout as today. Problems go to stderr as today, prefixed per config in
directory mode. Any unusable config makes the command fail with
`N of M configs are not usable`, exit 2. File mode is unchanged.

### `paths`

File mode: unchanged. Directory mode: text prints today's block (without
`binary`) per config separated by blank lines, then one `binary` line; JSON is
`{"configs": [<today's report without binary>...], "binary": {...}}`. A config
that cannot be loaded still shows its `config` line; the command exits 2 if
any config could not be loaded, after printing everything.

### `logrotate`

Directory mode: logs are rotated per config (each under its own lock);
expired events are removed once per distinct `paths.events` directory, with
the TTL of the first config using it. A broken config is reported on stderr
and the command exits 2 after doing the rest.

### Docs

- `docs/configuration.md`: new section "Several configs: a directory"
  (discovery, what counts as a config file, duplicates, broken configs),
  `onecheck paths` in directory mode.
- `docs/getting-started.md`: lookup order; "checks at different intervals"
  mentions that plain `onecheck` runs them all.
- `docs/output-and-results.md`: grouped text output, `configs` in JSON, exit
  code 2 for a config that could not run.
- CLI help (`root`, `run`, `validate`, `paths`, `logrotate`).

### Tests

- config: discovery of file, directory (sorting, `.yml`, hidden files,
  subdirectories ignored), empty directory, default directory order, not
  found message.
- output: grouped text, `--only-failed` hiding clean configs, config error
  lines, JSON `configs`, exit codes.
- cli: run over a directory (combined output, exit codes, broken config does
  not stop others, duplicate names, `--rule` across configs, config without
  matching rules is skipped), validate/paths/logrotate over a directory,
  file mode unchanged.

## Left to the builder

Names of helpers and files, the exact stderr wording beyond what is above.
