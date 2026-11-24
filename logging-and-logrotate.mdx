# Logging and logrotate

## Log file

Every run appends to

```
<log.dir>/<config name>/onecheck.log
```

by default `/var/log/onecheck/<name>/onecheck.log` for root and
`$HOME/.local/state/onecheck/log/<name>/onecheck.log` for other users. The
directory is created when needed. `onecheck paths` prints the exact path for
the current config.

```yaml
log:
  level: info      # debug | info | warn | error
  dir: /var/log/onecheck
  format: text     # text | json
```

| Level | What is logged |
|---|---|
| `error` | a run that could not complete |
| `warn` | recoverable problems, e.g. an unreadable state file, a `remind_every` shorter than the time between runs |
| `info` | one line per rule with its outcome, state changes and reminders (`reminder written`), a summary per run |
| `debug` | additionally, per rule: the params and fields requested from the module, the values it returned (long strings cut to 256 bytes), the state counters; the modules found; what modules printed on their stderr |

The level is taken from `--log-level`, then `log.level`, then `info`:

```sh
onecheck --log-level debug --rule api-health
```

`text` format:

```
time=2026-11-23T10:00:00.123Z level=INFO msg="check failed" rule=api-health module=http status=fail state=pass duration_ms=124 failures="[status: expected equals 200, got 503]"
time=2026-11-23T10:00:00.130Z level=INFO msg="state changed" rule=api-health transition=fail event=20261123T100000Z-main-api-health-fail
time=2026-11-23T10:00:00.131Z level=INFO msg="run finished" total=12 passed=11 failed=1 errors=0
```

`json` format writes the same records as one JSON object per line.

Messages at `warn` and above are also printed on stderr, so cron mails them.
With `--log-level` on the command line, stderr shows everything at that level
as well, so a run can be watched in the terminal:

```sh
onecheck --log-level debug --rule api-health
```

`log.level` in the config changes only the log file, never stderr. Log lines
never go to stdout, which carries only results.

At `debug` the log contains rule params and returned values as they are,
including HTTP headers and command output. Do not leave `log.level: debug`
on if those hold secrets.

If the log file cannot be opened, onecheck prints a warning, logs to stderr
for that run, and still runs the checks.

## `onecheck logrotate`

onecheck does not depend on the system's logrotate. One command takes care of
both things that would otherwise grow forever: the log and the event queue.

```sh
onecheck logrotate            # logs and events
onecheck logrotate --logs     # only the log of this config
onecheck logrotate --events   # only expired events
```

Schedule it, for example once a day:

```cron
17 3 * * *  root  onecheck logrotate >/dev/null
```

With several configs, run it once per config (logs are per config):

```cron
17 3 * * *  root  onecheck logrotate -c /etc/onecheck/fast.yaml >/dev/null
18 3 * * *  root  onecheck logrotate -c /etc/onecheck/slow.yaml >/dev/null
```

### Logs

```yaml
log:
  rotate:
    max_size: 10MB   # rotate when the log is larger than this
    keep: 5          # number of rotated files to keep
    max_age: 30d     # also remove rotated files older than this
    compress: true   # gzip rotated files
```

When `onecheck.log` is larger than `max_size`:

- `onecheck.log` becomes `onecheck.log.1` (or `onecheck.log.1.gz`);
- existing `onecheck.log.N` move to `N+1`;
- the next run starts a new `onecheck.log`.

On every call, rotated files with an index above `keep`, or older than
`max_age`, are removed. The log is only rotated when it is too large, so
calling `logrotate` often is harmless.

Rotation takes the config's run lock. If a run is in progress at that moment
the logs are skipped with a message and handled by the next call.

### Events

```yaml
events:
  ttl: 24h
```

An event is written with `expires_at` = time of the event + `events.ttl`.
`onecheck logrotate` removes events whose `expires_at` has passed. This only
matters for events nobody consumed: a notification service normally deletes
events as it processes them (see [State and events](/state-and-events)).

The event queue is shared by all configs, so the cleanup of any one config
covers all of them. Each event keeps the lifetime it was written with.

Files in the queue directory that cannot be read as events, and temporary
files left by an interrupted write, are removed once they are older than
`events.ttl`.
