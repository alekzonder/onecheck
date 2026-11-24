# Configuration

The config is YAML. Unknown keys are errors, so a typo never goes unnoticed.
Check a config without running it: `onecheck validate -c <file>`.

## Full reference

```yaml
name: main                    # REQUIRED. [a-z0-9][a-z0-9_.-]*

log:
  level: info                 # debug | info | warn | error
  dir: /var/log/onecheck      # log file: <dir>/<name>/onecheck.log
  format: text                # text | json
  rotate:                     # rules for "onecheck logrotate"
    max_size: 10MB            # rotate when the log is larger
    keep: 5                   # rotated files to keep
    max_age: 30d              # remove rotated files older than this
    compress: true            # gzip rotated files

paths:
  modules: /usr/lib/onecheck/modules   # where onecheck-<name> executables live
  results: /var/lib/onecheck/results   # results go to <results>/<name>/
  events:  /var/lib/onecheck/events    # event queue, shared by all configs
  state:   /var/lib/onecheck/state     # state goes to <state>/<name>/

include:                      # more rule files (globs)
  - conf.d/*.yaml

events:
  ttl: 24h                    # lifetime of an event in the queue

run:
  parallel: 4                 # checks running at the same time
  max_field_size: 10MB        # largest single value a module may return

defaults:                     # used by rules that do not set the key
  timeout: 30s
  fail_after: 1
  pass_after: 1
  output_tail: 10k            # how much module output is kept in a result
  event_params: true          # events carry the rule's params; false hides a rule's details
  remind_every: 60m           # a rule that stays failed writes a reminder event this often; 0 = never

modules:
  repos:                      # default repositories for "onecheck module search"
    - alekzonder/onecheck

rules:
  - name: api-health          # REQUIRED, unique. [a-z0-9][a-z0-9_.-]*
    module: http              # REQUIRED. module name: [a-z0-9][a-z0-9_-]*
    enabled: true             # false: the rule is skipped
    tags: [api, web]          # for "onecheck run --tag"
    timeout: 10s              # the module is killed after this
    fail_after: 3             # consecutive failures before state "fail"
    pass_after: 2             # consecutive passes before state "pass" again
    output_tail: 1k           # output kept in the result; 0 keeps none
    event_params: true        # false: no params in events, events marked "redacted"
    event_output: [body]      # output fields a notification may show; default: none
    remind_every: 60m         # reminder events while the state stays "fail"; 0 = never
    params:                   # input of the module, see docs/modules/
      url: https://example.com/health
    check:                    # conditions, see docs/checks.md
      status: 200
```

Every key except `name` is optional; the values above are the defaults for
root. Sizes accept `B`, `KB`, `MB`, `GB`, `TB` (1024-based); durations accept
`ms`, `s`, `m`, `h`, `d`, `w` and combinations such as `1h30m`.

`remind_every` is described in [State and events](/state-and-events#reminders),
`event_output` in [State and events](/state-and-events#output-in-notifications).

## `name`

The name is required and keeps configs on the same host apart:

- results: `<paths.results>/<name>/<rule>.json`
- state and run lock: `<paths.state>/<name>/`
- log: `<log.dir>/<name>/onecheck.log`

Rule names only have to be unique within one config. The event queue
(`paths.events`) is shared by all configs; every event carries the config
name.

Two different config files must not use the same name: onecheck remembers
which file owns a name and refuses to run the second one (exit code `2`).
If you move or rename a config file and keep its name, delete
`<paths.state>/<name>/.owner` once; the next run records the new location.

## Default paths

| | root | other users |
|---|---|---|
| `log.dir` | `/var/log/onecheck` | `$HOME/.local/state/onecheck/log` |
| `paths.modules` | `/usr/lib/onecheck/modules` | `$HOME/.local/lib/onecheck/modules` |
| `paths.results` | `/var/lib/onecheck/results` | `$HOME/.local/state/onecheck/results` |
| `paths.events` | `/var/lib/onecheck/events` | `$HOME/.local/state/onecheck/events` |
| `paths.state` | `/var/lib/onecheck/state` | `$HOME/.local/state/onecheck/state` |

Relative paths in the config are resolved against the directory of the config
file.

onecheck creates these directories with mode `0750` and the files in them
(results, state, events, logs) with mode `0640`. Results and events can
contain pieces of what was checked, such as file content or command output,
so they are not readable by everyone. To let another service read results or
events, put its user into the group owning the directories.

## Where everything is: `onecheck paths`

`onecheck paths` prints the full path of everything onecheck reads and writes
for the config it would use, so the paths do not have to be worked out from
the tables above:

```
$ onecheck paths
config    /etc/onecheck/config.yaml  (from default location)
name      main
include   /etc/onecheck/conf.d/api.yaml
include   /etc/onecheck/conf.d/db.yaml
log       /var/log/onecheck/main/onecheck.log
state     /var/lib/onecheck/state/main
results   /var/lib/onecheck/results/main
events    /var/lib/onecheck/events  (missing)
modules   /usr/lib/onecheck/modules
binary    /usr/local/lib/onecheck/versions/0.1.2/onecheck
```

| Line | Meaning |
|---|---|
| `config` | the config file and how it was found: `--config`, `ONECHECK_CONFIG` or `default location` |
| `name` | the config's `name`; it is part of the log, state and results paths |
| `include` | every include file that was read, in reading order |
| `log` | the log file, see [Logging and logrotate](/logging-and-logrotate) |
| `state` | the confirmed state of each rule, see [State and events](/state-and-events#files) |
| `results` | the latest result of each rule |
| `events` | the event queue, shared by all configs on the host |
| `modules` | where modules are looked for |
| `binary` | the onecheck executable itself, with symlinks resolved |

`(missing)` marks a path that does not exist yet. The log, state, results and
events paths are created by the first run, so they are missing on a fresh
install.

The command changes nothing, does not need the modules to be installed and
works with rules that are broken. If the config file is found but cannot be
loaded, its path is still printed, followed by the error, and the exit code
is `2`.

`onecheck paths --json` prints the same for scripts:

```json
{
  "name": "main",
  "config": { "path": "/etc/onecheck/config.yaml", "exists": true, "source": "default location" },
  "includes": [
    { "path": "/etc/onecheck/conf.d/api.yaml", "exists": true }
  ],
  "log": { "path": "/var/log/onecheck/main/onecheck.log", "exists": true },
  "state": { "path": "/var/lib/onecheck/state/main", "exists": true },
  "results": { "path": "/var/lib/onecheck/results/main", "exists": true },
  "events": { "path": "/var/lib/onecheck/events", "exists": false },
  "modules": { "path": "/usr/lib/onecheck/modules", "exists": true },
  "binary": { "path": "/usr/local/lib/onecheck/versions/0.1.2/onecheck", "exists": true }
}
```

## Includes (`conf.d/`)

```yaml
include:
  - conf.d/*.yaml
  - /etc/onecheck/team-*/rules.yaml
```

- Patterns are globs; relative ones are resolved against the config file's
  directory. A pattern that matches nothing is fine.
- Files are read in lexicographic order, after the rules of the main file.
- An include file may contain only the `rules` key:

  ```yaml
  # conf.d/web.yaml
  rules:
    - name: site-up
      module: http
      params: { url: https://example.com/ }
      check: { status: 200 }
  ```

- A rule name used twice, in any files, is an error naming both files.

## Environment variables in `params`

Strings inside `params` may reference environment variables as `${VAR}`,
which keeps secrets out of the config file:

```yaml
params:
  url: https://api.example.com/health
  headers:
    Authorization: "Bearer ${API_TOKEN}"
```

- Only the `${VAR}` form is expanded. A plain `$VAR` is left alone, so shell
  commands in the `command` module keep working.
- Write `$${VAR}` to get a literal `${VAR}`. Every other `$` (including the
  shell's `$$`, `$1`, `$(...)`) is passed through unchanged.
- An undefined variable makes that rule an error; it is never replaced by an
  empty string.
- `check` is never expanded.
- Events carry `params` as written here, with `${VAR}` not replaced, so a
  secret passed this way does not reach a notification. A secret written into
  the config as plain text does. `event_params: false` on a rule (or in
  `defaults`) keeps the params out of its events altogether, see
  [State and events](/state-and-events#what-an-event-can-reveal).

## Logging level

The level comes from, in order: `--log-level`, `log.level`, `info`.

## Two kinds of config errors

**Errors of the whole config** stop onecheck with exit code `2` before any
check runs: unreadable or invalid YAML, an unknown top-level key, a missing or
invalid `name`, a rule without a name, a duplicate rule name, an include file
with keys other than `rules`.

**Errors of a single rule** make only that rule unusable. On `onecheck run`
the rule gets the status `error` with the reason, and every other rule still
runs. This covers: an unknown key in the rule, an invalid rule name, a missing
`module`, a module that is not installed or not compatible, `params` the
module does not accept, a condition on a field the module does not have, an
unknown operator or a bad operator argument, an undefined `${VAR}`.

Such an `error` counts as a failure for the rule's state, so after
`fail_after` runs it produces a `fail` event: a broken rule is reported
through the same channel as a broken service, instead of silently
disappearing. The keys of the rule that could be read still apply to that
event: a rule with a misspelt key and `event_params: false` stays hidden, and
`remind_every` is honoured. A rule that cannot be checked allows no output
to be shown, whatever its `event_output` says.

`onecheck validate` prints all errors of both kinds and exits with `2` if
there are any.
