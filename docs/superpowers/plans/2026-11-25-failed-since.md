# failed_since Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** the `recovered` event carries `failed_since`, the time of the `fail` event of the failure that ended.

**Architecture:** `newEvent` takes the previous state (`prev *state.State`) instead of `from string`; for `recovered` it copies `prev.Since` into `Event.FailedSince`. `Advance` and the state file are unchanged.

**Tech Stack:** Go, `encoding/json` `omitzero`.

**Spec:** `docs/superpowers/specs/2026-11-25-failed-since-design.md`

## Global Constraints

- Field: `FailedSince time.Time` with tag `json:"failed_since,omitzero"` in `result.Event`.
- `since` of `recovered` stays equal to `at`.
- Written regardless of `event_params` / `redacted`.
- No `failed_for`; no state file format change; `state.Advance` signature unchanged.
- `make check` and `make docs-build` pass; commits `chg:` / `docs:`, no attribution trailers (AGENTS.md).

## Review Focus

- Recovered after reminders: `prev.Since` must not be confused with `RemindedAt` — covered by test 2.
- State file written by an older version, rule recovers: `failed_since` equals that file's `since` — test `TestRecoveredFromAStateFileOfAnOlderVersion`.
- Key absent (not `"0001-01-01..."`) on `fail`/`reminder`: raw JSON check — test 3.

---

### Task 1: failed_since in the event

**Files:**
- Modify: `internal/result/result.go` (Event struct)
- Modify: `internal/runner/runner.go` (`record`, `newEvent`)
- Test: `internal/runner/reminder_test.go`

**Interfaces:**
- Produces: `newEvent(cfg *config.Config, rule config.Rule, res *result.Result, prev *state.State, next state.State, now time.Time) result.Event`; `from` is `state.Pass` when `prev == nil`, else `prev.State`.

- [ ] **Step 1: Write failing tests** in `reminder_test.go`, with helper `(e *env) rawEvents(transition string) []map[string]any` reading `events/*-<transition>.json`:
  - `TestRecoveredEventCarriesFailedSince`: fail run, touch file, pass run → `recovered.FailedSince.Equal(fail.At)`, `recovered.Since.Equal(recovered.At)`.
  - `TestFailedSinceSurvivesReminders`: `goneRule("    remind_every: 1h\n")`, fail; two rounds of backdate 61m + run (2 reminders); then rerun with a passing rule of same name (`check: {exists: false}`) → `FailedSince` equals `fail.At` shifted by -122m (the backdated `since` from state), and not equal to any reminder's `At`.
  - `TestFailedSinceOnlyInRecovered`: raw `fail` and `reminder` events have no `failed_since` key; raw `recovered` has it.
  - `TestFailedSinceWhenRedacted`: `event_params: false`, fail then recover → `Redacted` and `!FailedSince.IsZero()`.
  - `TestRecoveredFromAStateFileOfAnOlderVersion`: write state `{state: fail, since: T, consecutive_*}` without `checked_at`/`reminded_at`, run passing rule → `FailedSince.Equal(T)`.
- [ ] **Step 2:** `go test ./internal/runner/ -run 'FailedSince|OlderVersion'` → compile FAIL (no field).
- [ ] **Step 3:** add the field (doc comment: when the failure that ended began, the `at` of its `fail` event; only in `recovered`); change `newEvent` to take `prev`, set `FailedSince = prev.Since` when transition is `recovered` and `prev != nil`; `record` passes `prev` (keep its local `from` for logging).
- [ ] **Step 4:** tests PASS.
- [ ] **Step 5:** commit `chg: add failed_since to the recovered event`.

### Task 2: docs

**Files:** Modify `docs/state-and-events.md`.

- [ ] **Step 1:** add `failed_since` row to the event keys table; add a `recovered` example JSON with it; state that `since` of `recovered` is still `at`; state that an event from an older onecheck has no `failed_since`.
- [ ] **Step 2:** `make docs-build` and `make check` pass.
- [ ] **Step 3:** commit `docs: describe failed_since of the recovered event` (with the spec and plan).
