## failed_since in the recovered event

### Intent
onenotify wants to show in a RECOVERED notification how long the rule was
failing: `failed for: 1h 12m (since 2026-11-24 08:11:02 UTC)`. It keeps no
state between events, so onecheck must pass the start of the failure in the
`recovered` event itself.

### Goals
- `result.Event` gets `FailedSince time.Time` (`json:"failed_since,omitzero"`):
  when the failure that just ended began, that is the `at` of its `fail` event.
- Set only for `recovered`; absent from `fail` and `reminder`.
- Written also when `redacted: true`: it is a time, not check details.

### Non-goals / anti-goals
- No ready-made duration (`failed_for`): the consumer computes `at - failed_since`.
- `since` of `recovered` stays equal to `at` (contract).
- No change to the state file format, no change to `Advance`'s signature.

### Approaches
1. **`newEvent` takes `prev *state.State`** (chosen). `record` already loads
   `prev` before `Advance`; on fail → pass `prev.Since` is exactly the time of
   the `fail` event (reminders change only `RemindedAt`). `newEvent` derives
   `from` from `prev` too. Nothing else changes.
2. Return the old `Since` from `Advance`: changes its signature for one caller.
3. Store `failed_since` in `State`: changes the state file format.

### Details
- Zero `prev.Since` → field omitted. A corrupt state gives `prev == nil`,
  the rule starts in pass, so `recovered` cannot happen there.
- State from an older version always has `Since`; no special cases.
- An event written by an older onecheck has no `failed_since`; docs say so.

### Tests (`internal/runner`)
- `recovered` after `fail`: `failed_since` == `at` of `fail`.
- `recovered` after several `reminder`: still the `fail` time.
- `fail` and `reminder` JSON has no `failed_since` key.
- `redacted: true`: the key is present.

### Docs
`docs/state-and-events.md`: row in the event keys table, `recovered` example
with the key, note that `since` of `recovered` is still `at`, note about events
from older versions. No other page lists event fields.
