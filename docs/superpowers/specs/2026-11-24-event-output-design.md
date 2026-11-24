# Output fields a notification may show — design

## Intent

An event already carries the saved output of the run (`result.output`: a
command's stdout and stderr, a response body and headers). `onenotify` never
shows it, because output may hold anything. A rule should be able to say
"this part of my output is fine to show": then a notification about a failed
backup can include the last lines of the script.

`onenotify` is changed separately; this design covers onecheck only.

## Goals

- A new rule key `event_output`: a list of names of the module's output
  fields, e.g. `event_output: [stderr]`. Default: nothing.
- An event has a new key `output_fields`: the list from the rule, `[]` when
  the rule sets none. It is a mark for the consumer; the output itself stays
  where it is, in `result.output`.
- `event_params: false` is stronger: such an event has `output_fields: []`.
- A name that is not an output field of the rule's module makes the rule an
  `error`, like a condition on a field the module does not have.

## Non-goals

- Changes to `onenotify`.
- `event_output` in `defaults`: output fields differ per module (decided by
  the owner: the key belongs to a rule).
- Marking ordinary fields (`status`, `exists`, ...): they are not in
  `result.output`. The values a notification needs from them are already in
  the failure messages.
- Changing what is saved: `output_tail` still decides whether and how much
  output is kept. With `output_tail: 0` there is nothing to show, and
  `event_output` does not bring it back.

## Anti-goals

- Output shown by default.
- An event of a rule with `event_params: false` that invites a consumer to
  show output.

## Approaches

1. **A mark in the event — chosen by the owner.** The event format grows by
   one key; a consumer that does not know it shows no output, which is the
   safe default.
2. Copy the chosen fields into a separate event key. Rejected: the same data
   twice in one file.
3. Drop unlisted fields from the event's `result.output`. Rejected: the
   event's result would no longer equal the saved result, and a consumer on
   the host loses data it may read today.

## Design (builder's calls unless said)

- `internal/config`: `event_output` on a rule only; a list of strings.
- `internal/runner`: `prepare` checks every name against the module's output
  fields; the error names the module's output fields. `newEvent` fills
  `output_fields`; all transitions carry it, reminders and `recovered`
  included (what to show for which transition is the consumer's choice).
- `internal/result`: `Event.OutputFields []string`, `json:"output_fields"`,
  never `null`.
- Additive change to the event JSON; the module protocol is not touched.
- Docs: `docs/state-and-events.md` (key, example, "What an event can
  reveal"), `docs/configuration.md`.

## Tests

- Config: the key is read; a value that is not a list is a rule error; the
  unknown-key error lists `event_output`.
- Runner: no key gives `"output_fields": []` (not `null`); `[stderr]` on a
  `command` rule gives `["stderr"]` and `result.output` keeps both stdout and
  stderr; with `event_params: false` the list is empty; a name the module
  does not have (and any name for a module without output fields) is a rule
  error that names the valid fields.
