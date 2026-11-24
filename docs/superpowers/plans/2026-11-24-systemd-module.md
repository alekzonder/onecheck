# Module `systemd` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** a bundled module `onecheck-systemd` that reports the state of one systemd unit (any type), with timer-specific fields and raw properties.

**Architecture:** `cmd/onecheck-systemd` like `cmd/onecheck-disk`: pure helpers (parse `systemctl show` output, compute fields from properties with an injected "now") tested on fixed text, plus a thin runner that calls `systemctl show` from `PATH`, tested with a fake `systemctl` script in `t.TempDir()`.

**Tech Stack:** Go 1.26.5, `os/exec`, `pkg/module` SDK.

**Spec:** `docs/superpowers/specs/2026-11-24-systemd-module-design.md`

## Global Constraints

- No new dependencies; `systemctl` is the only external program.
- Builds for `linux` and `darwin`, `amd64` and `arm64` (no build tags needed: everything goes through `systemctl` in `PATH`).
- Modules only collect; no comparison in the module.
- Tests: no network, no real systemd, write only under `t.TempDir()`.
- `make check` passes; gocyclo limit 25.
- Commit style `chg:`/`docs:`, lower case, no attribution trailers.

## Review Focus

1. Timer without `Persistent=true` after reboot — `idle_age` counts from activation, so the recipe does not fail right after boot.
2. Never-fired timer (`LastTriggerUSec=n/a` or empty) — `last_trigger_age` absent, not a huge number from the zero time.
3. Unit that does not exist (`LoadState=not-found`, `UnitFileState=` empty) — no error; `unit_file_state` is `""`.
4. Unit name without suffix (`nginx`) — type is taken from the `Id` systemd returns (`nginx.service`), so service fields are present.
5. `systemctl` exits non-zero or writes to stderr (`user: true` without a user bus) — status `error` with systemd's stderr in the message.
6. Values containing `=` (`ExecStart={ path=/usr/bin/x ; argv[]=... }`) in `properties` — split only on the first `=`.

---

### Task 1: parsing and field computation

**Files:**
- Create: `cmd/onecheck-systemd/show.go`, `cmd/onecheck-systemd/fields.go`
- Test: `cmd/onecheck-systemd/show_test.go`, `cmd/onecheck-systemd/fields_test.go`

**Interfaces:**
- Produces:
  - `func parseShow(text string) map[string]string` — `Key=Value` per line, split on the first `=`, lines without `=` skipped.
  - `func parseTime(v string) (time.Time, bool)` — layout `Mon 2006-01-02 15:04:05 MST` (output of systemctl under `TZ=UTC LC_ALL=C`); `""`, `n/a`, `0` → `false`.
  - `func unitType(id string) string` — suffix after the last `.` (`service`, `timer`, …).
  - `func unitFields(p map[string]string, now time.Time) map[string]any` — common fields, plus service fields if `unitType(p["Id"]) == "service"`, plus `service`/`last_trigger_age`/`idle_age`/`next_trigger_in` if `timer` (`idle_age` from the later of `LastTriggerUSec` and `ActiveEnterTimestamp`).
  - `func activatedFields(p map[string]string) map[string]any` — `service_result`, `service_exit_status` (if `ExecMainStatus` present), `service_active_state`.
  - Property lists: `var commonProps = []string{"Id","Description","LoadState","ActiveState","SubState","UnitFileState","Result","StateChangeTimestamp"}`, `serviceProps = {"NRestarts","MainPID","ExecMainStatus"}`, `timerProps = {"Unit","LastTriggerUSec","NextElapseUSecRealtime","ActiveEnterTimestamp"}`, `activatedProps = {"Result","ExecMainStatus","ActiveState"}`.
- Value types: strings as `string`, `restarts`/`main_pid`/`exit_status`/`service_exit_status` as `int64` (field absent if not an integer), durations as `float64` seconds (`now.Sub(t).Seconds()`, `next_trigger_in` clamped at 0).

- [ ] **Step 1: tests**
  - `TestParseShowSplitsOnFirstEquals`: `"ExecStart={ path=/bin/x ; argv[]=/bin/x a=b }\nUnitFileState=\nnoequals\n"` → `ExecStart` = `{ path=/bin/x ; argv[]=/bin/x a=b }`, `UnitFileState` = `""` present, 2 keys.
  - `TestParseTime`: `"Tue 2026-11-24 03:34:19 UTC"` → `2026-11-24T03:34:19Z`; `""`, `"n/a"`, `"0"` → false.
  - `TestUnitFieldsService` (now = `2026-11-24 12:00:00 UTC`): props `Id=nginx.service, ActiveState=active, SubState=running, LoadState=loaded, UnitFileState=enabled, Result=success, StateChangeTimestamp=Tue 2026-11-24 11:00:00 UTC, NRestarts=2, MainPID=123, ExecMainStatus=0, Description=web` → `active_state`=`active`, `state_age`=3600.0, `restarts`=int64(2), `main_pid`=int64(123), `exit_status`=int64(0), no `last_trigger_age`.
  - `TestUnitFieldsTimer`: `Id=backup.timer, Unit=backup.service, LastTriggerUSec=Tue 2026-11-24 02:00:00 UTC, NextElapseUSecRealtime=Wed 2026-11-25 02:00:00 UTC` , `ActiveEnterTimestamp=Fri 2026-11-13 14:00:00 UTC` → `service`=`backup.service`, `last_trigger_age`=36000.0, `idle_age`=36000.0, `next_trigger_in`=50400.0, no `restarts`.
  - `TestUnitFieldsTimerNeverFired`: `LastTriggerUSec=n/a`, `NextElapseUSecRealtime=` , `ActiveEnterTimestamp=Tue 2026-11-24 10:00:00 UTC` → no `last_trigger_age`, `idle_age`=7200.0 (counted from activation), no `next_trigger_in`.
  - `TestUnitFieldsNextTriggerInPastClamped`: next elapse 1h before now → `next_trigger_in` = 0.0.
  - `TestUnitFieldsNotFound`: `Id=nosuch.service, LoadState=not-found, ActiveState=inactive, UnitFileState=, StateChangeTimestamp=` → `load_state`=`not-found`, `unit_file_state`=`""` present, no `state_age`.
  - `TestUnitFieldsMissingBuiltinOmitted`: service props without `NRestarts` → no `restarts` key.
  - `TestActivatedFields`: `Result=exit-code, ExecMainStatus=2, ActiveState=inactive` → `service_result`=`exit-code`, `service_exit_status`=int64(2), `service_active_state`=`inactive`.
- [ ] **Step 2:** `go test ./cmd/onecheck-systemd/` → FAIL (undefined).
- [ ] **Step 3:** implement.
- [ ] **Step 4:** `go test ./cmd/onecheck-systemd/` → PASS.

### Task 2: systemctl runner, collect, describe

**Files:**
- Create: `cmd/onecheck-systemd/main.go`, `cmd/onecheck-systemd/systemctl.go`, `cmd/onecheck-systemd/collect.go`
- Test: `cmd/onecheck-systemd/collect_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces:
  - `func show(ctx context.Context, user bool, unit string, props []string) (map[string]string, error)` — `exec.LookPath("systemctl")`, not found → `systemd is not available: systemctl not found in PATH`; runs `systemctl [--user] show <unit> -p <comma-joined props>` with `os.Environ()` plus `LC_ALL=C`, `TZ=UTC`, and for `user` with no `XDG_RUNTIME_DIR` set, `XDG_RUNTIME_DIR=/run/user/<os.Getuid()>`; non-zero exit → `systemctl show <unit>: <trimmed stderr or exit error>`.
  - `var now = time.Now` (tests replace it).
  - `collect`: params `{unit string (required), user bool, properties []string}`; one `show` call with `commonProps+serviceProps+timerProps+params.properties` (deduplicated); `unitFields`; `properties` object = the requested names present in the output (only if `params.properties` non-empty); for a timer with non-empty `Unit` and any of `service_result`, `service_exit_status`, `service_active_state` wanted (`req.WantsOr(name, true)`), a second `show` of `Unit` with `activatedProps` → `activatedFields`.
  - Params schema: `unit` string `minLength: 1` required, `user` boolean, `properties` array of strings (`minLength: 1`), `additionalProperties: false`.
  - `describe`: all fields of the spec with types (`state_age`, `last_trigger_age`, `idle_age`, `next_trigger_in` → `duration`; `restarts`, `main_pid`, `exit_status`, `service_exit_status` → `number`; `properties` → `object`; the rest `string`).

- Fake systemctl for tests: a `sh` script written to `t.TempDir()` as `systemctl`, `t.Setenv("PATH", dir)`; it appends `"$@"` to `$dir/args` and prints the fixture chosen by the unit argument (or exits 1 with a message on stderr).

- [ ] **Step 1: tests**
  - `TestCollectServiceArgs`: unit `nginx` → fields from fixture (`active_state` = `active`, `restarts` present); recorded args contain `show nginx -p` and the `-p` list contains `ActiveState` and `LastTriggerUSec`; no `--user`.
  - `TestCollectUserFlag`: `user: true`, `XDG_RUNTIME_DIR` unset (`t.Setenv` then `os.Unsetenv`) → args start with `--user show`; fake records `$XDG_RUNTIME_DIR` = `/run/user/<uid>`.
  - `TestCollectTimerReadsActivatedUnit`: fixture `backup.timer` with `Unit=backup.service`, fixture `backup.service` with `Result=exit-code` → `service_result` = `exit-code`; second call has `show backup.service`.
  - `TestCollectTimerSkipsActivatedWhenNotWanted`: `Fields: ["last_trigger_age"]` → only one call recorded.
  - `TestCollectProperties`: `properties: [MemoryCurrent, Nope]`, fixture prints `MemoryCurrent=1073152` → `properties` = `{"MemoryCurrent": "1073152"}`.
  - `TestCollectSystemctlFailsIsError`: fake exits 1 with `Failed to connect to bus` on stderr → error containing `Failed to connect to bus`.
  - `TestCollectNoSystemctlIsError`: `PATH` = empty temp dir → error containing `systemd is not available`.
  - `TestCollectEmptyUnitIsError`.
  - `TestDescribeListsFields`: all 18 fields, types as above.
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS; `GOOS=darwin GOARCH=arm64 go vet ./cmd/onecheck-systemd/` clean; run `go run ./cmd/onecheck-systemd check` by hand against a real unit on the dev host and compare with `systemctl show`.
- [ ] **Step 5:** commit `chg: add the systemd module that reports the state of a unit, timer runs and raw properties`.

### Task 3: installer and docs

**Files:**
- Modify: `install.sh` (`MODULES`, help text), `internal/installsh/install_test.go` (expected call)
- Create: `docs/modules/systemd.md`
- Modify: `README.md`, `docs/getting-started.md`, `docs/module-management.md`, `docs/development.md`, `AGENTS.md` (module lists / repository map)

- [ ] **Step 1:** test expects `... disk@1.2.0 systemd@1.2.0 --modules-dir ...` → FAIL; change `install.sh` → PASS.
- [ ] **Step 2:** docs page following `docs/modules/disk.md`: params, fields per unit type, behaviour (not-found is a value, errors, user units need a user session/bus), the four recipes from the spec, why `idle_age` and not `last_trigger_age` (reboot, `Persistent=true`), missing fields on old systemd, example FAIL output.
- [ ] **Step 3:** `make check` → pass; commit `chg: install the systemd module with install.sh and document it`.
