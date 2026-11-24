# systemd: units by state — design

## Intent

The `systemd` module checks one named unit. The most useful host-wide check,
"nothing is failed", needs a rule per unit and misses units nobody listed.
Without `unit` the module lists the units in a state, so one rule covers the
host, and its failure says which units are failed.

## Goals

- `params.state: failed` without `unit`: the module returns the units in
  that state as `units` (array), their number `count`, and their names
  `names` (array of strings), like `docker` returns `containers`.
- `params.exclude`: a list of shell patterns; units whose name matches one
  are left out.
- The failure message names the units. The recommended check is `names: []`,
  which fails with `names: expected equals [], got ["a.service","b.service"]`
  (the owner chose a `names` field over changing the engine or checking
  through `every`).
- `user: true` works as for one unit.

## Non-goals

- Changing the one-unit mode: its params and fields stay as they are.
- Per-unit details in the list (restarts, timers, properties): a rule with
  `unit` gives those.
- Matching logic in the module: `exclude` narrows what is collected, like
  `name` in `docker`; conditions stay in `check`.

## Approaches

1. **`names` next to `units` and `count` — chosen by the owner.** No change
   to the engine; the message is short and names exactly the units.
2. `units: { every: ... }`: works today, but prints the whole element per
   unit. Kept possible, not recommended.
3. Teach `internal/match` to add names to a `count` failure. Rejected:
   module-specific logic in the shared engine.

## Design (builder's calls)

- **Params.** Exactly one of `unit` and `state`. `state` is one of
  the load and active states of systemd (`failed`, `active`, `inactive`,
  `activating`, `deactivating`, `reloading`, `maintenance`, `refreshing`,
  `loaded`, `not-found`, `bad-setting`, `error`, `masked`), passed to
  `systemctl list-units --state=`. Any other name is an error (changed after
  the review: systemctl answers an unknown state with an empty list, so a
  misspelt `faild` passed forever). Sub states (`running`, `dead`) are not
  accepted. `exclude` only with `state`, `properties` only with
  `unit`; the wrong combination is an "invalid params" error.
- **Command.** `systemctl [--user] list-units --all --plain --no-legend
  --no-pager --state=<state>`; five columns: unit, load, active, sub,
  description. Works on old systemd (no JSON output needed).
- **Fields.** Each element of `units`: `name`, `load_state`, `active_state`,
  `sub_state`, `description`. Sorted by name. Nothing in that state gives
  `units: []`, `names: []`, `count: 0`, not an error.
- A rule in one mode that checks a field of the other gets "got nothing
  (path is missing)", as for a timer field on a service.
- `exclude` uses `internal/glob`, the syntax of the `glob` operator.
- `systemctl` missing or failing is an `error`, as today.

## Tests

- Parsing of `list-units` output, including a description with spaces and
  an escaped unit name.
- `state: failed` returns units sorted, `names`, `count`; nothing failed
  gives empty arrays (JSON `[]`) and 0.
- `exclude` drops matching units from all three fields; an invalid pattern
  is an error.
- `user: true` adds `--user`; the arguments are exactly as designed.
- Params: neither `unit` nor `state`, both, `exclude` with `unit`,
  `properties` with `state`, a bad state name: errors.
- End to end with the engine: `names: []` on two failed units gives one
  failure whose message contains both unit names.
- The one-unit tests stay green unchanged.
