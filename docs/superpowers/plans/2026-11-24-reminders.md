# Reminders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** While a rule stays in confirmed state `fail`, a run writes a `reminder` event every `remind_every` (default 60m).

**Architecture:** `internal/config` loads `remind_every` for a rule and for `defaults`. `state.Advance` decides when a reminder is due and returns it as a third transition; `runner.record` writes the event the same way as for `fail`/`recovered` and warns when the interval is shorter than the time since the rule's previous run.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, standard library only.

**Spec:** `docs/superpowers/specs/2026-11-24-reminders-design.md`

## Global Constraints

- `make check` passes (gofmt, vet, golangci-lint, tests with `-race`); cyclomatic complexity ≤ 25, no `//nolint:gocyclo`.
- Tests use no network and write only under `t.TempDir()`.
- Commit messages: `chg: ...` / `fix: ...` / `docs: ...`, lower case, imperative; **no attribution trailers**; no merge commits.
- The event is written before the state; files are written atomically (existing `store` does this, do not bypass it).
- No matching logic outside `internal/match`; the module protocol is not touched.
- Docs link by site route (`[Checks](/checks#anchor)`), never by `.md` path; `make docs-build` passes.
- Built-in default of `remind_every` is `60m`; `0` turns reminders off.

## Review Focus

1. A state file written by an older version (no `reminded_at`, no `checked_at`) for a rule already in `fail`: loads, and the rule gets its first reminder when due. → Task 2 (`old state`), Task 3 (`TestReminderForAStateFileOfAnOlderVersion`).
2. The host was down for many intervals: exactly one reminder on the next run, none on the run right after it. → Task 2.
3. The clock moved back (`now` before `since` or before `reminded_at`): no reminder, no panic, state still saved. → Task 2.
4. The reminder event cannot be written: the run fails and `reminded_at` is not advanced, so the next run writes it. → Task 3.
5. `remind_every` equal to the run interval, runs arriving with jitter (60.4s for `1m`): no warning. → Task 3 (`TestRemindShorterThanRuns`).

---

### Task 1: `remind_every` in the config

**Files:**
- Modify: `internal/config/types.go`, `internal/config/load.go`, `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Defaults.RemindEvery *Duration` (`yaml:"remind_every"`, nil = not set in the config);
  `Rule.RemindEvery time.Duration` (0 = off); `Rule.RemindExplicit bool` (true when the value comes from the rule or from `defaults` in the config file, false for the built-in default);
  `const DefaultRemindEvery = time.Hour`.

- [ ] **Step 1: Write the failing tests**

`TestRemindEveryDefaultsAndOverride`: one config with rules `plain` (no key), `own` (`remind_every: 10m`), `off` (`remind_every: 0`), loaded twice.

```go
// without defaults
plain: RemindEvery == time.Hour, RemindExplicit == false
own:   RemindEvery == 10*time.Minute, RemindExplicit == true
off:   RemindEvery == 0, RemindExplicit == true
// with "defaults: {remind_every: 2h}"
plain: RemindEvery == 2*time.Hour, RemindExplicit == true
own:   10*time.Minute, true
// with "defaults: {remind_every: 0}"
plain: RemindEvery == 0
```

`TestRemindEveryInvalid`: a rule with `remind_every: -5m` and one with `remind_every: soon` each have `Rule.Err != nil` whose text contains `remind_every` or `invalid duration`; `defaults: {remind_every: -1h}` makes `Load` fail. A rule with the typo `remind_evry: 1h` has an error listing `remind_every` among the known keys.

- [ ] **Step 2: Run** `go test ./internal/config/ -run RemindEvery` — FAIL (unknown field).

- [ ] **Step 3: Implement**

`rawRule.RemindEvery *Duration`; `"remind_every"` in `ruleKeys` and in the "known:" list of `unknownRuleKeys` (after `event_params`). `buildRule` starts from `DefaultRemindEvery`/false, or from `*c.Defaults.RemindEvery`/true when it is set, then the rule's own value/true. `units.ParseDuration` already rejects negative values and accepts `0`.

- [ ] **Step 4: Run** `go test ./internal/config/` — PASS.

- [ ] **Step 5: Commit** `chg: add remind_every to rules and defaults`

---

### Task 2: the `reminder` transition in `state.Advance`

**Files:**
- Modify: `internal/state/state.go`, `internal/runner/runner.go` (the one call site, pass `rule.RemindEvery`)
- Test: `internal/state/state_test.go`

**Interfaces:**
- Produces:
  ```go
  const TransitionReminder = "reminder"
  type State struct { /* existing */
      RemindedAt time.Time `json:"reminded_at,omitzero"` // last reminder of the current failure
      CheckedAt  time.Time `json:"checked_at,omitzero"`  // previous run of the rule
  }
  func Advance(prev *State, ok bool, failAfter, passAfter int, remindEvery time.Duration, now time.Time) (next State, transition string)
  ```
  `next.CheckedAt` is always `now`. `TransitionFail` and `TransitionRecovered` clear `RemindedAt`; `TransitionReminder` sets it to `now`.

- [ ] **Step 1: Write the failing tests**

Update the existing calls (pass `0`: existing expectations stay). Add `TestAdvanceReminders`, a table of steps `{at time.Duration (offset from t0), ok bool, want string}` with `fail_after 1`, `remind_every 1h` unless said:

```go
"due times count from the start": {0,false,"fail"}, {59m,false,""}, {60m,false,"reminder"}, {61m,false,""}, {119m,false,""}, {121m,false,"reminder"}
"late run does not shift the schedule": {0,false,"fail"}, {90m,false,"reminder"}, {120m,false,"reminder"}
"one reminder after a long gap": {0,false,"fail"}, {10h30m,false,"reminder"}, {10h31m,false,""}, {11h,false,"reminder"}
"a passing run is not reminded, the next failing one is" (pass_after 3): {0,false,"fail"}, {60m,true,""}, {61m,false,"reminder"}
"recovery ends reminders": {0,false,"fail"}, {30m,true,"recovered"}, {2h,true,""}
"a new failure starts a new schedule": {0,false,"fail"}, {70m,false,"reminder"}, {80m,true,"recovered"}, {90m,false,"fail"}, {100m,false,""}, {150m,false,"reminder"}
"off" (remind_every 0): {0,false,"fail"}, {5h,false,""}
"not confirmed yet" (fail_after 3): {0,false,""}, {2h,false,""}
"clock moved back": {0,false,"fail"}, {-2h,false,""}, {70m,false,"reminder"}, {30m,false,""}
```

`TestAdvanceReminderBookkeeping`: after `fail` at t0 `RemindedAt.IsZero()`; after the reminder at t0+70m `RemindedAt` equals t0+70m and `Since` equals t0; after `recovered` `RemindedAt.IsZero()`; `CheckedAt` equals `now` after every call.
`old state`: `prev = &State{State: Fail, Since: t0, ConsecutiveFail: 4}` (zero `RemindedAt`, `CheckedAt`), a failing run at t0+3h returns `reminder`.

- [ ] **Step 2: Run** `go test ./internal/state/` — FAIL (does not compile).

- [ ] **Step 3: Implement.** After the existing switch, when no transition happened, the state is `Fail`, the run did not pass and `remindEvery > 0`:

```go
n := now.Sub(next.Since) / remindEvery      // whole intervals since the failure began
due := next.Since.Add(n * remindEvery)      // latest due time that has passed
if n >= 1 && due.After(next.RemindedAt) { transition, next.RemindedAt = TransitionReminder, now }
```

(`RemindedAt` zero is before any `due`; a clock that moved back gives `n < 1` or `due` not after `RemindedAt`.)

- [ ] **Step 4: Run** `go test ./internal/state/ ./internal/runner/` — PASS.

- [ ] **Step 5: Commit** `chg: make a reminder a transition of a rule that stays failed`

---

### Task 3: the reminder event, `since`, the warning

**Files:**
- Modify: `internal/result/result.go`, `internal/runner/runner.go`
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Consumes: Task 1 `Rule.RemindEvery`, `Rule.RemindExplicit`; Task 2 `Advance`, `State.CheckedAt`, `TransitionReminder`.
- Produces: `Event.Since time.Time` (`json:"since"`, placed after `ExpiresAt`): when the confirmed state the event reports about began — `next.Since` (equals `at` for `fail` and `recovered`, the time of the `fail` transition for `reminder`). `From` stays "the confirmed state before the run", so a reminder has `from: fail`, `to: fail`. `Result.Transition` may be `"reminder"`.
  `func remindShorterThanRuns(remindEvery, sinceLastRun time.Duration) bool` in `runner.go`: true when `sinceLastRun > remindEvery + remindEvery/10`.

- [ ] **Step 1: Write the failing tests** (helper `(e *env) backdate(rule string, d time.Duration)`: read `state/main/<rule>.json`, move `since`, `reminded_at` (if set) and `checked_at` back by `d`, write it back; helper to run with a log buffer where the test needs the log)

```go
TestReminderIsWrittenWhenDue
  // rule "absent"-like, fail_after 1, remind_every: 1h
  run → 1 event "fail"; run → still 1 event, Transition nil
  backdate(61m); run → 2 events; the new one: Transition "reminder", From "fail", To "fail",
    Since equals the At of the "fail" event minus 61m (±1s), ID ends with "-reminder",
    Result.Status fail, len(Result.Failures) == 1, ExpiresAt-At == 24h;
    the returned result has *Transition == "reminder" and State "fail"
  run → still 2 events
TestFailAndRecoveredEventsCarrySince      // ev.Since.Equal(ev.At) for both
TestReminderRespectsEventParams           // event_params: false → params null, redacted true on the reminder
TestReminderOffWithZero                   // remind_every: 0, backdate(5h), run → 1 event
TestReminderOfAnErrorRun                  // rule in fail, then module replaced by a missing one ("module: nosuch" in the same rule name),
                                          // backdate(61m), run → reminder with Result.Status error, Result.Error set
TestReminderForAStateFileOfAnOlderVersion // write state {"state":"fail","since":<3h ago>,"consecutive_pass":0,"consecutive_fail":2} by hand, run → reminder
TestReminderEventWriteFailureIsRetried    // after "fail": backdate(61m), chmod events 0555 → Run errors and state has no reminded_at;
                                          // chmod back, run → reminder written (skip as root, like TestRunEventWriteFailureKeepsState)
TestRemindShorterThanRuns
  (time.Minute, 60400*time.Millisecond) → false; (time.Minute, 66*time.Second) → false;
  (time.Minute, 67*time.Second) → true; (time.Minute, 5*time.Minute) → true; (time.Hour, time.Minute) → false
TestShortRemindEveryIsReported
  // remind_every: 1m on the rule, backdate(10m) → run logs a warn containing "remind_every" and the rule name, reminder still written;
  // same with the built-in default (no key) and backdate(3h) → reminder, no warn;
  // remind_every: 1m, state with zero checked_at → no warn
```

- [ ] **Step 2: Run** `go test ./internal/runner/ -run 'Remind|Since'` — FAIL.

- [ ] **Step 3: Implement.** In `record`: fill `Since: next.Since`; for `TransitionReminder` log `reminder written` instead of `state changed`; before writing a reminder, when `rule.RemindExplicit`, `prev.CheckedAt` is set and `remindShorterThanRuns(rule.RemindEvery, now.Sub(prev.CheckedAt))`, log at warn: `remind_every is shorter than the time between runs, a reminder is written on every run` with `rule`, `remind_every`, `since_last_run`. Keep `record` under the complexity limit by moving the event construction into `newEvent(cfg, rule, res, from, next, transition, now) result.Event`. Update the comments on `Result.Transition` and `Event`.

- [ ] **Step 4: Run** `go test ./internal/...` — PASS.

- [ ] **Step 5: Commit** `chg: write reminder events and the start of the problem into events`

---

### Task 4: documentation and examples

**Files:**
- Modify: `docs/state-and-events.md`, `docs/configuration.md`, `docs/output-and-results.md`, `examples/config.yaml`

- [ ] **Step 1:** `docs/state-and-events.md`: a section "Reminders: `remind_every`" after the thresholds (when a reminder is written, the schedule from the start of the problem, no reminder on a passing run, `0` turns it off, default `60m`, the warning about an interval shorter than the runs, one reminder after downtime); `since` in the example JSON and the table of keys; `transition` and `from`/`to` rows mention `reminder`; a second, short example of a reminder event; "No lost transitions" says a reminder whose state could not be saved is written again; the files table says the state holds the time of the last reminder and of the previous run. The intro sentence "once per change" is corrected.
- [ ] **Step 2:** `docs/configuration.md`: `remind_every` in the `defaults` and rule examples and in the reference tables, linking to `/state-and-events#reminders-remind_every` (check the anchor Blume generates with `make docs-build`). `docs/output-and-results.md`: `[transition: reminder]` row and the `transition` key. `examples/config.yaml`: `remind_every: 60m` in `defaults` with a comment.
- [ ] **Step 3: Run** `make check` and `make docs-build` — both pass. Run the built binary against a throw-away config with a failing rule and `remind_every: 1s` twice with a pause, and compare the reminder event with the documented example.
- [ ] **Step 4: Commit** `docs: describe remind_every and reminder events`
