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
    params:                   # input of the module, see docs/modules/
      url: https://example.com/health
    check:                    # conditions, see docs/checks.md
      status: 200
```

Every key except `name` is optional; the values above are the defaults for
root. Sizes accept `B`, `KB`, `MB`, `GB`, `TB` (1024-based); durations accept
`ms`, `s`, `m`, `h`, `d`, `w` and combinations such as `1h30m`.

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
disappearing.

`onecheck validate` prints all errors of both kinds and exits with `2` if
there are any.
