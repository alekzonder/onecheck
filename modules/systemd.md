---
title: systemd module
sidebar:
  label: systemd
---

Reports the state of one systemd unit, so you know that services which must
serve are running and start on boot, that units which must stay off are
stopped and will not come back after a reboot, and that scheduled jobs
(timers) really run and succeed.

Works with any unit type: `service`, `timer`, `socket`, `mount`, `path`,
`target`, ... It calls `systemctl show`, so it needs systemd; on other
systems (macOS, Linux without systemd) every check is an `error`.

## Params

| Key | Required | Description |
|---|---|---|
| `unit` | yes | unit name, e.g. `nginx.service`, `backup.timer`, `docker.socket` |
| `user` | no | `true` checks a unit of the user's own service manager (`systemctl --user`); default `false` |
| `properties` | no | list of extra `systemctl show` property names, returned in the `properties` field |

A name without a suffix (`nginx`) is understood by systemd as a service.
Name exactly one unit: patterns (`*`, `?`, `[`) are rejected, because
systemd would report several units at once. Escaped names such as
`dev-disk-by\x2dlabel-root.device` work as `systemctl` shows them.

`user: true` is for onecheck running as an ordinary user. The user's service
manager must be running: for cron or a timer outside a login session enable
lingering (`loginctl enable-linger <user>`). If `XDG_RUNTIME_DIR` is not set,
the module uses `/run/user/<uid>`. Checking another user's units from root is
not supported.

One rule checks one unit; write one rule per unit.

## Fields

All units:

| Field | Type | Description |
|---|---|---|
| `load_state` | string | `loaded`, `not-found`, `masked`, `bad-setting`, `error` |
| `active_state` | string | `active`, `inactive`, `failed`, `activating`, `deactivating`, `reloading` |
| `sub_state` | string | type-specific state: `running`, `exited`, `dead`, `waiting`, `listening`, ... |
| `unit_file_state` | string | `enabled`, `disabled`, `static`, `masked`, `indirect`, `generated`, ...; empty string if systemd reports none (e.g. the unit does not exist) |
| `result` | string | result of the unit's last run: `success`, `exit-code`, `signal`, `timeout`, `core-dump`, ... |
| `description` | string | unit description |
| `state_age` | **duration** | time since the last change of the active or sub state |

Services:

| Field | Type | Description |
|---|---|---|
| `restarts` | number | automatic restarts (`Restart=`) since the service was started: a crash loop shows here |
| `main_pid` | number | main process id, 0 if not running |
| `exit_status` | number | exit status of the last main process |

Timers:

| Field | Type | Description |
|---|---|---|
| `service` | string | the unit the timer starts |
| `last_trigger_age` | **duration** | time since the timer last fired; missing if it has not fired |
| `idle_age` | **duration** | time since the timer last fired or, if systemd does not know when it last fired (e.g. after a reboot without `Persistent=true`), since the timer was activated |
| `next_trigger_in` | **duration** | time until the next calendar (`OnCalendar=`) run; missing if none is scheduled, so always missing for timers with only `OnBootSec=`/`OnActiveSec=`/`OnUnitActiveSec=` |
| `service_result` | string | result of the last run of the started unit: `success`, `exit-code`, ... |
| `service_exit_status` | number | exit status of the started service's last main process |
| `service_active_state` | string | active state of the started unit |

Raw:

| Field | Type | Description |
|---|---|---|
| `properties` | object | `{Name: value}` for each name in `params.properties`; integers (sizes, counts, ids) as numbers, everything else as text exactly as `systemctl show` prints it (`infinity`, `yes`, `[not set]`); a name systemd does not know is missing |

**duration** fields accept unit literals in conditions:
`idle_age: { lt: 25h }`. See [Checks](/checks#gt-gte-lt-lte).

### Which field to use

- **Must run:** `active_state: active`. **Must be stopped:**
  `active_state: { not_equals: active }` (a `failed` unit is not running
  either). A typo in the unit name makes "must be stopped" pass, because a
  unit that does not exist is not running: add `load_state: loaded` to
  rules for units that must be installed.
- **Starts on boot / must not:** `unit_file_state`. `enabled` starts on boot;
  `disabled` and `masked` do not (`masked` cannot even be started by hand).
  `static` units have no `[Install]` section and are started by something
  else.
- **Timer fires regularly:** prefer `idle_age` to `last_trigger_age`. A timer
  without `Persistent=true` forgets when it last fired on every reboot;
  `last_trigger_age` is then missing and a condition on it fails until the
  timer fires again, up to a whole period after each boot. `idle_age` counts
  from the timer's activation in that case, and still fails when an active
  timer has not fired for longer than the threshold. When systemd knows the
  last run, `idle_age` equals `last_trigger_age`, so restarting a timer that
  stopped firing does not hide it.
- **The job succeeds:** `service_result: success`. Prefer it to
  `service_exit_status`: a unit may declare other exit codes successful
  (`SuccessExitStatus=`), and `service_result` already accounts for that.
  Before the job has run for the first time since boot it is `success`.
- Anything else: name the property in `params.properties` and check
  `properties.<Name>`, e.g. `properties.MemoryCurrent`.

Which fields are present:

- Service fields only for services, timer fields only for timers.
- A unit that does not exist is not an error: systemd reports it with
  `load_state: not-found` and `active_state: inactive` (and default values
  such as `result: success`, `restarts: 0` for services). "Must run" then fails
  with a clear message, "must be stopped" passes.
- Older systemd versions do not have some properties (`NRestarts` appeared in
  systemd 235); their fields are then missing and conditions on them fail
  with "got nothing (path is missing)".

`systemctl` missing or failing (e.g. no user service manager for
`user: true`) is an `error`, not a `fail`; systemd's message is shown.

A rule without `check` passes whenever the state could be read, as with any
module, so always say what you expect.

## Examples

```yaml
# conf.d/services.yaml
rules:
  # must run and start on boot
  - name: nginx
    module: systemd
    params: { unit: nginx.service }
    check:
      active_state: active
      unit_file_state: enabled

  # must be stopped and not come back after a reboot
  - name: telnet-off
    module: systemd
    params: { unit: telnet.socket }
    check:
      active_state: { not_equals: active }
      unit_file_state: { in: [disabled, masked, ""] }

  # running, and not restarting over and over
  - name: app-stable
    module: systemd
    params: { unit: app.service }
    check:
      active_state: active
      restarts: { lt: 3 }

  # daily backup timer fires and the backup succeeds
  - name: backup
    module: systemd
    params: { unit: backup.timer }
    check:
      active_state: active
      idle_age: { lt: 25h }
      service_result: success

  # a property without its own field
  - name: app-memory
    module: systemd
    params: { unit: app.service, properties: [MemoryCurrent] }
    check:
      properties.MemoryCurrent: { lt: 1073741824 }
```

`""` in `telnet-off` covers a unit that is not installed at all.
`properties.MemoryCurrent` is a number (bytes), so `lt` works on it; it is
`[not set]` when memory accounting is off, and `lt` then fails with "not
applicable to a string". Numbers beyond the int64 range
(`18446744073709551615`, systemd's "unlimited") stay text.

A failure names the rule and the value:

```
FAIL   backup   (systemd, 12ms)
       service_result: expected equals "success", got "exit-code"
```
