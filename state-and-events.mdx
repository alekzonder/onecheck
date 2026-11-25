# State and events

onecheck does not only tell whether a check passes right now. For every rule
it keeps a **confirmed state** between runs and records each change of that
state as an **event**. This is what a notification service builds on: it gets
told when something broke and when it recovered, once per change, and is
reminded while the problem stays, regardless of which module the rule uses.

[onenotify](https://alekzonder.github.io/onenotify) is such a service: it
takes the events described here and delivers them to Telegram, mail or ntfy.
onecheck itself sends nothing.

## Status and state

Two different things are reported for each rule:

| | Values | Meaning |
|---|---|---|
| `status` | `pass`, `fail`, `error` | the outcome of **this run** |
| `state` | `pass`, `fail` | the **confirmed** state, which changes only after enough runs in a row |

`status` values:

- `pass` — every condition held;
- `fail` — at least one condition did not hold;
- `error` — the check could not be carried out: module missing or
  incompatible, invalid rule, timeout, no connection, value too large.

For the state, `error` counts the same as `fail`: a check that cannot run is
not known to be healthy. The distinction stays visible in `status`.

## Thresholds: `fail_after`, `pass_after`

```yaml
rules:
  - name: api-health
    module: http
    fail_after: 3     # 3 non-passing runs in a row → state becomes fail
    pass_after: 2     # 2 passing runs in a row → state becomes pass again
    params: { url: https://example.com/health }
    check: { status: 200 }
```

Both default to `1` (from `defaults` in the config), which means the state
follows the status immediately.

The rules:

- In state `pass`, after `fail_after` consecutive runs that did not pass, the
  state becomes `fail`.
- In state `fail`, after `pass_after` consecutive passing runs, the state
  becomes `pass`.
- A run with the opposite outcome resets the count.
- A rule seen for the first time starts in state `pass` without an event. If
  it is failing, it goes to `fail` through the normal `fail_after` rule.

Example with `fail_after: 3`, `pass_after: 2`:

| Run | status | consecutive | state | event |
|---|---|---|---|---|
| 1 | pass | 1 | pass | |
| 2 | fail | 1 | pass | |
| 3 | fail | 2 | pass | |
| 4 | pass | 1 | pass | (count reset) |
| 5 | fail | 1 | pass | |
| 6 | fail | 2 | pass | |
| 7 | fail | 3 | **fail** | `fail` |
| 8 | fail | 4 | fail | |
| 9 | pass | 1 | fail | |
| 10 | pass | 2 | **pass** | `recovered` |

Thresholds count **runs**, not time: with a cron line every minute,
`fail_after: 3` means roughly three minutes.

The process exit code and the printed output follow `status`, not `state`: a
rule that failed once with `fail_after: 3` already makes `onecheck` exit with
`1`. The text output shows the gap, e.g. `[state: pass, 1/3]`.

A manual `onecheck run --rule <name>` updates the state and writes events
exactly like a scheduled run.

## Reminders

A `fail` event is written once. If nobody fixed the problem, onecheck says so
again: while a rule's state stays `fail`, it writes a `reminder` event every
`remind_every`.

```yaml
defaults:
  remind_every: 60m     # the built-in default

rules:
  - name: api-health
    module: http
    remind_every: 15m   # this one more often
    params: { url: https://example.com/health }
    check: { status: 200 }

  - name: nightly-report
    module: file
    remind_every: 0     # one notification is enough: no reminders
    params: { path: /var/reports/today.csv }
    check: { exists: true }
```

- Reminders are on by default, every 60 minutes. `remind_every: 0` turns them
  off, for one rule or in `defaults` for all.
- Reminders are due at fixed times counted from the start of the problem:
  after `remind_every`, after twice that, and so on. A reminder is written by
  the first run after a due time, so a run that starts late does not shift
  the following ones. A run up to a tenth of `remind_every` early counts as
  on time: with `remind_every` equal to the interval the config is run at,
  every run of a failed rule writes a reminder.
- One run writes at most one reminder for a rule. After the host was down
  for a day there is one reminder, not twenty-four.
- A run that **passed** writes no reminder, even if the state is still `fail`
  because `pass_after` has not been reached. If the next run fails again, the
  reminder is written then.
- A run that ended with `error` is reminded of like any failing run.
- The reminder carries the result of the run that wrote it, so the
  notification shows the problem as it is now, and `since`: when it began.

onecheck has no scheduler, so a reminder cannot come more often than the
config is run. A `remind_every` shorter than the time since the rule's
previous run means "on every run"; onecheck then logs a warning:

```
level=WARN msg="remind_every is shorter than the time between runs, a reminder is written on every run" rule=api-health remind_every=1m0s since_last_run=5m0s
```

Raise `remind_every` or run the config more often. The warning is given only
for a value set in the config, not for the built-in 60 minutes of a config
that runs once a day. After the host was down, one such warning is expected.

## Events

Every change of a rule's confirmed state, and every
[reminder](#reminders), writes one file into the event queue directory,
`paths.events`:

```
/var/lib/onecheck/events/20261123T100200Z-main-api-health-fail.json
```

The file name is the event id: `<UTC time>-<config name>-<rule>-<transition>`.

```json
{
  "id": "20261123T100200Z-main-api-health-fail",
  "config": "main",
  "rule": "api-health",
  "transition": "fail",
  "from": "pass",
  "to": "fail",
  "at": "2026-11-23T10:02:00Z",
  "expires_at": "2026-11-24T10:02:00Z",
  "since": "2026-11-23T10:02:00Z",
  "params": {
    "url": "https://example.com/health",
    "headers": { "Authorization": "Bearer ${API_TOKEN}" }
  },
  "redacted": false,
  "output_fields": [],
  "result": {
    "config": "main",
    "rule": "api-health",
    "module": "http",
    "module_version": "1.2.0",
    "tags": ["api"],
    "status": "fail",
    "state": "fail",
    "transition": "fail",
    "consecutive": 3,
    "fail_after": 3,
    "pass_after": 2,
    "started_at": "2026-11-23T10:01:59.874Z",
    "duration_ms": 124,
    "failures": [
      {
        "path": "status",
        "operator": "equals",
        "expected": 200,
        "actual": 503,
        "message": "status: expected equals 200, got 503",
        "fields": [
          { "path": "status", "operator": "equals", "actual": 503 }
        ]
      }
    ],
    "error": null,
    "output": {
      "body": "upstream connect error",
      "headers": { "content-type": "text/plain" }
    }
  }
}
```

| Key | Description |
|---|---|
| `id` | unique id, equal to the file name without `.json` |
| `config` | `name` of the config the rule belongs to |
| `rule` | rule name |
| `transition` | `fail` (pass → fail), `recovered` (fail → pass) or `reminder` (still fail) |
| `from`, `to` | confirmed state before and after; both `fail` for a reminder |
| `at` | when the event was recorded (UTC, RFC 3339) |
| `expires_at` | `at` + `events.ttl`; after this the event may be deleted |
| `since` | when the state in `to` began: equal to `at` for `fail` and `recovered`, the time of the `fail` event for a reminder |
| `params` | the rule's `params` exactly as written in the config, before `${VAR}` is replaced; `null` when the rule has none or has `event_params: false` |
| `redacted` | `true` when the rule has `event_params: false`: a consumer must not show the details of this event (params, failures, error) to third parties |
| `output_fields` | names of the keys of `result.output` the rule allows a notification to show, from the rule's [`event_output`](#output-in-notifications); `[]` unless the rule says otherwise, and always `[]` with `event_params: false` |
| `result` | the full result of the run that wrote the event, see [Output and results](/output-and-results#result-format) |

For a `recovered` event, `result` is the passing run (`failures` is empty).
For a `fail` event caused by a check that could not run, `result.status` is
`error` and `result.error` holds the reason.

A reminder is the same file with `reminder` in its name and `since` pointing
back to the start of the problem (`result` is left out here):

```json
{
  "id": "20261123T110215Z-main-api-health-reminder",
  "config": "main",
  "rule": "api-health",
  "transition": "reminder",
  "from": "fail",
  "to": "fail",
  "at": "2026-11-23T11:02:15Z",
  "expires_at": "2026-11-24T11:02:15Z",
  "since": "2026-11-23T10:02:00Z",
  "params": { "url": "https://example.com/health" },
  "redacted": false,
  "output_fields": [],
  "result": { "status": "fail", "state": "fail", "transition": "reminder", "consecutive": 63 }
}
```

### What an event can reveal

An event is meant to be turned into a notification, which usually leaves the
host: a chat, a mail, a push service. Two parts of it deserve a look.

**Params.** `params` holds what the config file says, not what the module
received. A secret passed as `${VAR}` stays `${VAR}`:

```yaml
params:
  url: https://example.com/health
  headers: { Authorization: "Bearer ${API_TOKEN}" }   # the event says ${API_TOKEN}
```

A secret typed into the config as plain text is copied into the event as it
is. Use `${VAR}`, see
[Configuration](/configuration#environment-variables-in-params).

**Failures and errors.** A failure message names the value that was found:
`body: expected contains "ok", got "<the first 1024 bytes of the body>"`. A
condition on a response body or on a command's output therefore puts a piece
of that body or output into the event. `result.output` holds its tail as
well, see [Output and results](/output-and-results#saved-output). A
notification shows saved output only for the fields a rule names in
[`event_output`](#output-in-notifications).

To keep a rule's details out of notifications, set `event_params: false`:

```yaml
rules:
  - name: token-check
    module: http
    event_params: false   # the event has "params": null and "redacted": true
    output_tail: 0        # and no saved output
    params: { url: https://example.com/token }
    check: { status: 200 }
```

The event is still written, with its full `result`: `redacted` is an
instruction to the consumer. onenotify then sends only the rule name, the
module, the tags and the time. `event_params` can be set for all rules in
`defaults`.

### Output in notifications

`result.output` holds what the module saw: a command's stdout and stderr, a
response body and headers. It is in every event, but a notification does not
show it: output can contain anything. A rule whose output is safe and useful
to read says which part may be shown:

```yaml
rules:
  - name: backup
    module: command
    event_output: [stderr]      # a notification may show stderr, not stdout
    params: { run: /usr/local/bin/backup.sh }
    check: { exit_code: 0 }
```

The event then has `"output_fields": ["stderr"]`. This is only a mark for
the consumer: the event's `result.output` is the same with or without it.

- Only output fields of the rule's module can be named: `stdout` and
  `stderr` for `command`, `body` and `headers` for `http`. Another name, or
  any name for a module without output fields, makes the rule an `error`.
- The default is to show nothing. `event_output` is a key of a rule; there is
  none in `defaults`, because output fields differ between modules.
- `event_params: false` is stronger: the event has `"output_fields": []`.
- How much output exists is still decided by `output_tail`, see
  [Output and results](/output-and-results#saved-output). With
  `output_tail: 0` there is nothing to show.

### Consuming the queue

The queue is a plain directory, shared by all configs on the host. The
contract for a consumer (the notification service):

1. List `*.json` files. Files are written atomically (temp file + rename), so
   a visible `.json` file is always complete. Ignore names starting with `.`.
2. Process the event.
3. Delete the file.

Ordering: file names start with a UTC timestamp, so a lexicographic sort is
chronological.

Delivery is at-least-once: if a consumer crashes between processing and
deleting, it will see the event again. Use `id` to deduplicate.

Events nobody consumed are removed by `onecheck logrotate` once `expires_at`
has passed (`events.ttl`, default `24h`). See
[Logging and logrotate](/logging-and-logrotate).

### No lost transitions

For each rule onecheck writes the event first and the new state after it. If
the event cannot be written (disk full, no permission), the state is left
unchanged and the run ends with exit code `2`; the next run detects the same
change again and retries. A change of state is therefore either in the queue
or reported as a failed run, never silently dropped.

Reminders follow the same order. If the state cannot be saved after a
reminder was written, the next run writes that reminder once more: a
consumer may see a duplicate, never a gap.

## Files

| Path | Content | Lifetime |
|---|---|---|
| `<paths.state>/<name>/<rule>.json` | confirmed state, consecutive counters, time of the last reminder and of the previous run | internal, do not edit |
| `<paths.state>/<name>/.lock`, `.owner` | run lock; which config file owns the name | internal |
| `<paths.results>/<name>/<rule>.json` | latest result of the rule | overwritten every run |
| `<paths.events>/<id>.json` | one state change or reminder | until consumed or expired |

State and result files of a rule that was removed from the config (or
disabled) are deleted by the next full run, that is a run without `--rule` or
`--tag`. A state file that cannot be read is logged as a warning and the rule
is treated as new.

Renaming a rule starts it from scratch: the state is keyed by rule name.
Changing a rule's module or conditions keeps its state.
