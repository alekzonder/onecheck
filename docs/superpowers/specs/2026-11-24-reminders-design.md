# Reminders for rules that stay failed — design

## Intent

An event is written once, when a rule's confirmed state changes. A problem
nobody fixed after the first notification is then silent until it recovers.
While a rule stays in state `fail`, onecheck repeats the signal: a `reminder`
event with the current result and the time the problem began.

`onenotify` is changed separately; this design covers onecheck only.

## Goals

- A new key `remind_every` (a duration: `30m`, `1h`, `1d`) on a rule and in
  `defaults`. Built-in default: `60m`. `remind_every: 0` turns reminders off,
  on a rule or in `defaults`.
- While a rule's confirmed state is `fail`, a run writes an event with
  `transition: reminder` when a reminder is due.
- The reminder carries the result of the run that wrote it and the time the
  problem began.
- A `remind_every` shorter than the interval the config is run at is reported.

## Non-goals

- Changes to `onenotify`.
- A scheduler, or a config key that declares the run interval.
- Reminders for a rule in state `pass` (a rule failing below `fail_after`).
- Escalation: growing intervals, a limit on the number of reminders.

## Anti-goals

- A batch of reminders after the host was down: one run writes at most one
  reminder per rule.
- A reminder that says "still failing" with a passing result.
- A lost or reordered `fail` / `recovered` event because of reminders.

## Behaviour

Decided by the owner:

- Key name, where it lives, the transition name `reminder`, the content
  (current result, start of the problem).
- Reminders are on by default, every 60 minutes.
- The run interval is not declared anywhere: it is measured as the time since
  the rule's previous run.
- A run with status `pass` writes no reminder, even when the state is still
  `fail` (waiting for `pass_after`) and a reminder is due. If a later run
  fails again, the reminder is written then.

Calls made in the design (not discussed in detail, correct if wrong):

- **When a reminder is due.** Due times are counted from the start of the
  problem: `since + N × remind_every`, N = 1, 2, ... A run writes a reminder
  when the latest due time that has passed is later than the last reminder
  (or than `since`, when there was none). Delays of the scheduler therefore
  do not add up, and a long gap gives one reminder. A run up to a tenth of
  `remind_every` early counts as on time (added after the review: `since` is
  itself a run time, so with `remind_every` equal to the run interval runs
  land just before or after each due time and early ones were skipped).
- **`error` runs.** For the state `error` already counts as `fail`; a rule in
  state `fail` whose run ends with `error` gets its reminder, with the reason
  in `result.error`.
- **Event format.** `from: fail`, `to: fail`, a new key `since` (when the
  confirmed state became `fail`). `since` is present in every event: for
  `fail` and `recovered` it equals `at`. The id ends with `-reminder`.
  `params`, `redacted`, `expires_at` work as for other events.
- **Result.** `result.transition` is `reminder` for that run, so the result
  file and `onecheck run` show that an event was written.
- **State file.** Two new keys: `reminded_at` (last reminder, cleared by
  `recovered` and by a new `fail`) and `checked_at` (the rule's previous
  run). An old state file without them is valid: no reminder was sent yet,
  the previous run is unknown.
- **Too short an interval.** When a reminder is written and `remind_every`
  is shorter than the time since the rule's previous run, onecheck logs a
  warning naming both values; the reminder is still written, which means on
  every run. Only a value set in the config (rule or `defaults`) is
  reported, not the built-in 60 minutes: a config run once a day must not
  warn about a key it never set. One false warning after the host was down is
  accepted.
- **Order of writes** is unchanged: event, then state, then result. If the
  state cannot be saved after a reminder was written, the next run writes it
  again: a duplicate, never a loss.
- **Validation.** A negative `remind_every` is an error of the rule (or of
  the config, in `defaults`).

## Approaches

1. **In `state.Advance` — chosen (builder's call, the owner left the how).**
   `Advance` already takes `now` and returns the transition; it gets
   `remindEvery` and the outcome of the run, and `reminder` becomes a third
   transition. All transition rules stay in one pure package and are tested
   as a table.
2. In `runner.record`, after `Advance`. Leaves `state` untouched, but splits
   the rules for writing events between two packages. Rejected.

## Touches

- `internal/config`: `remind_every` in `Defaults`, `rawRule`, `Rule`,
  `ruleKeys` and its error text, validation; whether the value was set
  explicitly.
- `internal/state`: `TransitionReminder`, `RemindedAt`, `CheckedAt`, the due
  rule in `Advance`.
- `internal/result`: `Event.Since`; the comment on `Result.Transition`.
- `internal/runner`: `record` passes `remind_every`, fills `since`, logs the
  warning; the "state changed" log line must not be used for a reminder.
- `internal/output`: whatever prints `[transition: ...]`.
- `docs/state-and-events.md` (transitions, event keys, state file),
  `docs/configuration.md` (`remind_every` on a rule and in `defaults`),
  `docs/output-and-results.md` (`transition`), `examples/`.

## Tests

- `Advance` table: no reminder before the first due time; one at it; none
  again until the next; one after a gap of several intervals; none on a
  passing run while the state is `fail`, and one on the next failing run;
  `recovered` and a new `fail` clear `reminded_at`; `remind_every: 0` never
  reminds; an old state without the new keys.
- Runner: the reminder event has `transition: reminder`, `from`/`to` `fail`,
  `since` equal to the time of the `fail` event, the current result;
  `event_params: false` gives `params: null` and `redacted: true`.
- Runner: the warning is logged for an explicit short `remind_every` and not
  for the built-in default.
- Config: rule value overrides `defaults`; `0` on a rule turns off a default;
  a negative value is an error; an unknown-key error lists `remind_every`.
- The event example in `docs/state-and-events.md` matches the real output.

## Left to the builder

- Names of Go fields, the exact text of the warning and of the log line for a
  reminder.
- Whether `0` parses with the existing `Duration` type or needs handling.
