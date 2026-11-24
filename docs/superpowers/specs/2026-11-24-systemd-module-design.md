# Module `systemd` — design

## Intent

Know that the services a server depends on are in the state they must be in:
running and enabled when they must serve, stopped (and disabled or masked)
when they must not come back, and that scheduled jobs (timers) really run
and succeed. One rule checks one unit, so a failure names the unit.

## Goals

- A bundled module `systemd` (`cmd/onecheck-systemd`) that reports the state
  of any systemd unit named in `params.unit` (service, timer, socket, mount,
  path, target, …).
- Both directions are expressible with ordinary conditions: "must run"
  (`active_state: { equals: active }`) and "must be stopped"
  (`active_state: { not_equals: active }`), plus "must start on boot" /
  "must not" via `unit_file_state` (`enabled`, `disabled`, `masked`, …).
- Timers: "fires regularly" as `idle_age: { lt: 25h }` (or the strict
  `last_trigger_age`), "the job it
  runs succeeds" as `service_result: { equals: success }`.
- Any other systemd property is reachable through `params.properties` and the
  `properties` object field.
- System units by default, user units with `user: true`.

## Non-goals

- Comparison or expectations inside the module (`expect: running`):
  comparison lives in `internal/match` (AGENTS.md).
- Deriving "overdue" from the timer's calendar schedule. The rule states the
  maximum age; `next_trigger_in` is reported for diagnostics.
- Checking all units automatically (the future `discover` protocol
  extension).
- macOS and non-systemd Linux: the module returns an error.
- D-Bus, `golang.org/x/sys` or any new dependency.

## Approaches

1. **Curated fields + raw properties** (chosen by the user). Readable rules,
   computed ages as `duration`, and an escape hatch to every property.
2. Raw `systemctl show` properties only. Rejected: timestamps come as dates,
   so "triggered within 25h" cannot be written as a condition.
3. Expectation in params, the module decides. Rejected: violates "modules
   only collect"; failure messages would differ from every other module.

## Params

| Key | Required | Description |
|---|---|---|
| `unit` | yes | unit name with its type suffix, e.g. `nginx.service`, `backup.timer` (schema: `minLength: 1`) |
| `user` | no | `true` → `systemctl --user` (units of the user onecheck runs as); default `false` |
| `properties` | no | list of extra `systemctl show` property names returned in the `properties` field |

A name without a suffix is passed to systemctl as is (systemctl treats it
as `.service`). Our call: no normalisation, systemd decides.

## Fields

All units:

| Field | Type | Source / description |
|---|---|---|
| `load_state` | string | `LoadState`: `loaded`, `not-found`, `masked`, `bad-setting`, `error` |
| `active_state` | string | `ActiveState`: `active`, `inactive`, `failed`, `activating`, `deactivating`, `reloading` |
| `sub_state` | string | `SubState`: `running`, `exited`, `dead`, `waiting`, `listening`, … |
| `unit_file_state` | string | `UnitFileState`: `enabled`, `disabled`, `static`, `masked`, `indirect`, … ; empty string when systemd reports none (e.g. `not-found`) |
| `result` | string | `Result` of the unit's last run: `success`, `exit-code`, `signal`, `timeout`, `core-dump`, … |
| `state_age` | duration | now − `StateChangeTimestamp`: time since the last change of the active or sub state; omitted if systemd reports no timestamp |
| `description` | string | `Description` |

Services only (`*.service`):

| Field | Type | Source / description |
|---|---|---|
| `restarts` | number | `NRestarts`: automatic restarts since the unit was started (crash loops) |
| `main_pid` | number | `MainPID`, 0 if not running |
| `exit_status` | number | `ExecMainStatus`: exit status of the last main process |

Timers only (`*.timer`):

| Field | Type | Source / description |
|---|---|---|
| `service` | string | `Unit`: the unit the timer activates |
| `last_trigger_age` | duration | now − `LastTriggerUSec`; omitted if the timer never fired (so `lt: 25h` fails with "field is missing") |
| `idle_age` | duration | now − the later of `LastTriggerUSec` and the timer's `ActiveEnterTimestamp`: how long the timer has been watching without firing; omitted if neither is known |
| `next_trigger_in` | duration | `NextElapseUSecRealtime` − now; omitted if no next elapse is scheduled; negative values clamped to 0 |
| `service_result` | string | `Result` of the activated unit (last run of the job) |
| `service_exit_status` | number | `ExecMainStatus` of the activated unit (if it is a service) |
| `service_active_state` | string | `ActiveState` of the activated unit |

Raw:

| Field | Type | Description |
|---|---|---|
| `properties` | object | `{Name: value}` for every name in `params.properties`, integers as numbers (so `lt`/`gt` work; our call after the end-to-end run showed the core does not compare strings numerically), everything else as strings exactly as `systemctl show` prints them; a property systemd does not know is absent |

Durations are seconds (float), like the `file` module's `age`.

## Behaviour

- **Collection:** one `systemctl show <unit> -p <list>` call (plus
  `--user` when asked), environment `LC_ALL=C TZ=UTC` so timestamps print as
  `Tue 2026-11-24 03:34:19 UTC` and parse with a fixed layout. For a timer a
  second call reads the activated unit's `Result`, `ExecMainStatus`,
  `ActiveState`. Values `n/a`, empty or `0` for timestamps mean "none".
- **Unit does not exist:** not an error. systemd answers `LoadState=not-found`,
  `ActiveState=inactive`; the module returns that. "Must run" then fails with
  a clear message and "must be stopped" passes: a removed unit does not run.
  Our call.
- **Errors (status `error`):** `systemctl` not in `PATH`, which covers macOS
  ("systemd is not available"), `systemctl` exits non-zero (its stderr in the
  message, e.g. no user bus for `user: true`), output that cannot be parsed,
  invalid params.
- **Timers after a reboot (our call, from the builder check):** a timer
  without `Persistent=true` loses `LastTriggerUSec` on reboot and reads `n/a`
  until it fires again. `last_trigger_age` is then absent and a condition on
  it fails for up to one period after every boot. `idle_age` counts from the
  timer's activation in that case, so the recipe uses it: it still fails when
  an active timer has not fired for longer than the threshold. A timer that
  was restarted gets a fresh grace period; that is accepted.
- **Older systemd (our call):** a built-in property missing from the output
  (e.g. `NRestarts` before systemd 235) leaves its field out; conditions on it
  fail with "field is missing". No minimum version is enforced.
- **`user: true` (our call):** meant for onecheck running as an ordinary
  user (with lingering for cron/timers). The environment is inherited; if
  `XDG_RUNTIME_DIR` is unset, the module sets it to `/run/user/<uid>`.
  Checking another user's units from root is out of scope for now.
- Only the requested fields are worked for when cheap: the timer's second
  call is skipped if no `service_*` field is requested (`Request.WantsOr`).
- Time "now" comes from one clock read per check; the clock is injectable
  for tests.

## Recipes for the docs page

```yaml
# must run and start on boot
- name: nginx
  module: systemd
  params: { unit: nginx.service }
  check:
    active_state: { equals: active }
    unit_file_state: { equals: enabled }

# must be stopped and not come back after reboot
- name: telnet-off
  module: systemd
  params: { unit: telnet.socket }
  check:
    active_state: { not_equals: active }
    unit_file_state: { in: [disabled, masked, ""] }

# no crash loop
- name: app-stable
  module: systemd
  params: { unit: app.service }
  check:
    active_state: { equals: active }
    restarts: { lt: 3 }

# daily backup fires and succeeds
- name: backup
  module: systemd
  params: { unit: backup.timer }
  check:
    active_state: { equals: active }
    idle_age: { lt: 25h }
    service_result: { equals: success }
```

## Other changes

- `install.sh`: `MODULES="file http command disk systemd"`; the
  `internal/installsh` test updated.
- `docs/modules/systemd.md`; bundled module lists in `README.md`,
  `docs/getting-started.md`, `docs/module-management.md`,
  `docs/development.md`, `AGENTS.md` repository map.
- The module builds for darwin too (release builds every module for every
  platform) and errors at runtime there.

## Tests

No network, no real systemd needed, only `t.TempDir()`:

- parser of `systemctl show` output (key=value lines, `=` inside values,
  empty values) on fixed text;
- field computation from parsed properties with a fixed "now": service,
  timer, never-fired timer, `not-found` unit, `n/a` timestamps;
- `properties` param: present and unknown names;
- the systemctl call through a fake `systemctl` script put first in `PATH`
  (in `t.TempDir()`), checking arguments (`--user`, `-p` list, unit) and the
  error path (non-zero exit, missing binary);
- `describe` lists all fields with types.

## Left to the builder

File layout in `cmd/onecheck-systemd`, the exact property list per unit
type, how the fake `systemctl` is wired in tests.
