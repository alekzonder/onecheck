# Output and results

A run reports its outcome in four places:

| Where | What | For |
|---|---|---|
| exit code | did everything pass | cron, systemd, scripts |
| stdout | every rule, or only the failing ones; text or JSON | people, pipelines |
| `<paths.results>/<name>/<rule>.json` | the latest result of each rule | dashboards, other tools |
| `<paths.events>/` | changes of state | notifications, see [state-and-events.md](state-and-events.md) |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | every rule that ran passed (also when there were no rules to run) |
| `1` | at least one rule has status `fail` or `error` |
| `2` | onecheck could not do its job: no or invalid config, unknown flag, another run of the same config in progress, results or events could not be written, the run was interrupted (SIGINT, SIGTERM) |

The exit code follows the status of **this run**, not the confirmed state: a
rule with `fail_after: 3` that failed once already gives `1`.

With code `2` nothing is printed on stdout; the reason is on stderr, prefixed
with `onecheck:`.

An interrupted run (Ctrl-C, `systemctl stop`, shutdown) kills the running
modules together with everything they started and records nothing: no
results, no state changes, no events. A half-finished check says nothing
about what it was checking, so it must not produce a `fail` event.

## Flags

```
onecheck run [-c <config>] [-o text|json] [--only-failed] [--rule <name>]... [--tag <tag>]...
```

| Flag | Effect |
|---|---|
| `-o, --output text\|json` | output format, default `text` |
| `--only-failed` | list only rules with status `fail` or `error`; the summary still counts all rules |
| `--rule <name>` | run only this rule; repeatable |
| `--tag <tag>` | run only rules carrying this tag; repeatable |
| `--log-level <level>` | override `log.level` for this run and also print the log at that level on stderr |

With both `--rule` and `--tag`, a rule runs if it matches either. Disabled
rules (`enabled: false`) never run.

Results are printed on stdout only. Log lines never go to stdout, so
`onecheck -o json | jq` is safe; warnings and errors of onecheck itself go to
stderr and to the log file.

## Text output

```
PASS   passwd-sane   (file, 2ms)
FAIL   disk-usage    (command, 14ms)  [state: pass, 1/3]
       stdout: expected matches "^[0-7]\\d?%", got "91%\n"
FAIL   api-health    (http, 124ms)  [transition: fail]
       body json status: expected equals "ok", got "degraded"
       body json /checks/1/state: schema violation: value must be 'ok'
ERROR  cache-ping    (redis, 0ms)
       module "redis" is not installed in /usr/lib/onecheck/modules (see: onecheck module install --help)

4 rules: 1 passed, 2 failed, 1 errors
```

One line per rule: status, rule name, module and duration. Under a failing
rule, one line per condition that did not hold; under an `error`, the reason.

The note in brackets appears when the confirmed state needs explaining:

| Note | Meaning |
|---|---|
| `[transition: fail]` | this run changed the state to `fail`; an event was written |
| `[transition: recovered]` | this run changed the state back to `pass`; an event was written |
| `[state: pass, 1/3]` | the run did not pass, but the state is still `pass`: 1 of `fail_after: 3` |
| `[state: fail, 1/2]` | the run passed, but the state is still `fail`: 1 of `pass_after: 2` |

## JSON output

```sh
onecheck -o json
onecheck -o json --only-failed
```

```json
{
  "summary": { "total": 4, "passed": 1, "failed": 2, "errors": 1 },
  "results": [
    { "config": "main", "rule": "passwd-sane", "status": "pass", "...": "..." }
  ]
}
```

`summary` always counts every rule that ran. `results` lists them in config
order; with `--only-failed` it contains only the non-passing ones (possibly
`[]`). Each element has the [result format](#result-format) below.

```sh
# names of everything that is not fine
onecheck -o json | jq -r '.results[] | select(.status != "pass") | .rule'

# all failure messages
onecheck -o json --only-failed | jq -r '.results[] | .rule as $r | (.failures[].message, .error // empty) | "\($r): \(.)"'
```

## Result files

After every run each rule's result is stored at

```
<paths.results>/<config name>/<rule>.json
```

Passing and failing rules alike. There is no history: the file is replaced on
each run (atomically, so a reader never sees a half-written file). The
directory therefore always shows the current picture of all rules of a config.

Results of rules that were removed from the config or disabled are deleted by
the next full run (one without `--rule` / `--tag`).

## Result format

The same object is used in JSON output, in result files and inside events.

```json
{
  "config": "main",
  "rule": "api-health",
  "module": "http",
  "module_version": "1.2.0",
  "tags": ["api"],
  "status": "fail",
  "state": "pass",
  "transition": null,
  "consecutive": 2,
  "fail_after": 3,
  "pass_after": 2,
  "started_at": "2026-11-23T10:00:00.123Z",
  "duration_ms": 124,
  "failures": [
    {
      "path": "body",
      "json_path": "status",
      "operator": "equals",
      "expected": "ok",
      "actual": "degraded",
      "message": "body json status: expected equals \"ok\", got \"degraded\""
    }
  ],
  "error": null,
  "output": {
    "body": "{\"status\":\"degraded\",\"db\":\"timeout\"}",
    "headers": { "content-type": "application/json", "server": "nginx" }
  }
}
```

| Key | Type | Description |
|---|---|---|
| `config` | string | `name` of the config |
| `rule` | string | rule name |
| `module` | string | module name |
| `module_version` | string | version of the module that ran; empty if the module could not be loaded |
| `tags` | array | the rule's tags, `[]` if none |
| `status` | string | outcome of this run: `pass`, `fail`, `error` |
| `state` | string | confirmed state after this run: `pass`, `fail` |
| `transition` | string or null | `fail` or `recovered` if this run changed the state, else `null` |
| `consecutive` | number | how many runs in a row ended like this one (passing, or not passing) |
| `fail_after`, `pass_after` | number | the rule's thresholds |
| `started_at` | string | start of the check, UTC, RFC 3339 |
| `duration_ms` | number | how long the check took |
| `failures` | array | conditions that did not hold; `[]` unless `status` is `fail` |
| `error` | string or null | why the check could not run; set only when `status` is `error` |
| `output` | object | what the module saw, kept for diagnosis; `{}` if the module has no output fields or `output_tail` is `0`. See [Saved output](#saved-output) |

Each element of `failures`:

| Key | Description |
|---|---|
| `path` | the key of `check` the condition belongs to, e.g. `body`, `headers.content-type` |
| `json_path` | location inside a parsed JSON document, for `json.assert` (GJSON path) and `json.schema` (JSON pointer); absent otherwise |
| `operator` | the operator that did not hold, e.g. `equals`, `length gt`, `any`, `not`, `json`, `schema` |
| `expected` | the operator's argument from the config |
| `actual` | the value found; `null` if the path was missing; long values are cut to about 1 KB |
| `message` | a ready-to-show sentence combining the above |

## Saved output

A failed `exit_code: 0` alone does not say what went wrong. So onecheck keeps
the raw output of the check in the result, **whether or not a condition uses
it and whether the rule passed or failed**:

| Module | Saved fields |
|---|---|
| `command` | `stdout`, `stderr` |
| `http` | `body`, `headers` |
| `file` | nothing |

Which fields are saved is decided by the module (it marks them as output, see
[module-protocol.md](module-protocol.md)); how much is kept is decided by the
config:

```yaml
defaults:
  output_tail: 10k      # default for every rule; this is also the built-in default

rules:
  - name: backup
    module: command
    output_tail: 100k   # more for this rule
    params: { run: /opt/backup/run.sh }
    check: { exit_code: 0 }

  - name: token-check
    module: http
    output_tail: 0      # keep nothing: the response holds secrets
    params: { url: https://example.com/token }
    check: { status: 200 }
```

- The size is per field and accepts `k`, `m` (`1k`, `10k`, `1m`) as well as
  `KB`, `MB`; a bare number is bytes.
- A longer text is cut to its **last** `output_tail` bytes and then starts
  with `…`: the end of the output is where the error usually is.
- A value that is not text (`headers`) is kept whole if it fits into
  `output_tail` and left out otherwise.
- A command killed by its `timeout` still has the output it printed before
  that in `output`, next to `status: error`.
- Output is cut from what the module returned, which is itself limited by
  `run.max_field_size` (default `10MB`): of a larger output only the first
  `max_field_size` bytes are seen at all.

The result file is overwritten by the next run, so `output` always belongs to
the latest run. An [event](state-and-events.md) embeds the result of the run
that changed the state, including its `output`, so a notification can show
why the check failed.

Saved output may contain secrets (tokens in a response body, passwords
printed by a script). Result directories are created with mode `0750` and
files with `0640`; set `output_tail: 0` on rules whose output must not be
stored.
