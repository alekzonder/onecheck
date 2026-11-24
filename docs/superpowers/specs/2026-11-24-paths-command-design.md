# `onecheck paths` — design

## Intent

Show, with one command, the full path of everything onecheck reads and writes
for the current config. Today these paths have to be worked out by hand: the
config is found through a chain of places, the default directories differ for
root and other users, relative paths are resolved against the config file, and
the config `name` is appended to some directories but not to others.

## Goals

- `onecheck paths` prints the absolute path of: the config file (and how it
  was found), every include file that was read, the log file, the state
  directory, the results directory, the event queue, the modules directory
  and the onecheck binary itself.
- A path that does not exist yet is marked `(missing)`.
- `--json` prints the same information for scripts.
- It works when modules are missing or rules are broken: it only needs the
  config to load.
- When the config was found but cannot be loaded, the path of the config file
  is still printed, then the error; exit code `2`.

## Non-goals

- Listing the configs of other `name`s on the host.
- Listing individual state, result or event files.
- Creating anything: the command only reads.

## Approaches

1. **A new `paths` subcommand** (chosen). Independent of module discovery, so
   it answers "where is everything" even on a half-installed machine.
2. A `--paths` flag on `validate`. Rejected: `validate` fails on missing
   modules and broken rules, exactly when the paths are needed most.
3. Printing the paths at debug level in `run` only. Rejected: it runs the
   checks and changes state just to answer a question.

## Decisions

- `config.Locate(flag) (path, source string, err error)` reports where the
  config came from: `--config`, `ONECHECK_CONFIG` or `default location`.
  `config.Find` stays as a wrapper.
- `Config.Includes []string` holds the include files that were read, in
  reading order.
- Text output: one `label  path` line per item, labels `config`, `name`,
  `include`, `log`, `state`, `results`, `events`, `modules`, `binary`.
- JSON output: `{"name", "config": {"path","exists","source"},
  "includes": [{"path","exists"}], "log": {...}, "state": {...},
  "results": {...}, "events": {...}, "modules": {...}, "binary": {...}}`.
- Documented in `docs/configuration.md` (section "Where everything is") and
  mentioned in `docs/getting-started.md`, `docs/logging-and-logrotate.md` and
  the root help.
