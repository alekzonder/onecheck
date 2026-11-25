# Failure fields: which concrete fields failed

## Why

onenotify shows `failures[].message` as is. When the failed operator is
`any`, `not` or `some`, the message carries the whole element and the whole
condition. For example, a rule like this:

```yaml
check:
  containers.#(state=="running")#:
    every:
      any:
        - { path: name, in: ["bot-1", "postgres-1"] }
```

produces lines like `containers.#(state=="running")#.2: expected any
[{"in":[...],"path":"name"}], got {"exit_code":0,...` that get cut off
before they say which container is extra. The user needs to see which
concrete field of which element or object did not pass, and its value:
here, the `name` of every unknown container.

`any` already evaluates the failing leaf (`...2.name in [...]`), but it
does so on a scratch recorder and throws the result away.

## Goals

- Every `match.Failure` carries a short structured list, `fields`, of the
  concrete leaf fields behind it, each with its value.
- `message` and every other existing key stay exactly as they are (the old
  format stays).
- The list goes everywhere a failure goes: result files, `-o json`, and
  events (`result.failures`).

## Non-goals

- Changing onenotify. It will render `fields` in a separate change in its
  own repository.
- Changing the human-readable text output of `onecheck`.
- The module protocol. Nothing changes there, so there is no protocol bump.

## Approaches

1. **A `fields` list on each failure (chosen).** It is structured, so a
   consumer formats it as it likes, and the full failure is kept.
2. A ready-made `summary` string on each failure. Simpler, but consumers
   cannot format it (bold text in Telegram, etc.).
3. Shortening `message` itself. Rejected: the full failures must stay.

## Design

New type in `internal/match`:

```go
type Field struct {
    Path     string `json:"path"`                // full path to the leaf, e.g. containers.#(state=="running")#.2.name
    JSONPath string `json:"json_path,omitempty"` // as in Failure, for json.assert/json.schema
    Operator string `json:"operator"`            // the leaf operator, e.g. "in", "length gt", "exists"
    Actual   any    `json:"actual"`              // value at the leaf, cut like Failure.Actual; null when missing
    Missing  bool   `json:"missing,omitempty"`   // the path is absent (as opposed to an explicit null)
}
```

`Failure` gets `Fields []Field \`json:"fields"\``, always written as an array
(never `null`).

What goes into `fields`:

| Failure | `fields` |
|---|---|
| a leaf operator (`equals`, `in`, `gt`, `length …`, `exists`, `json`, `schema`, ...) | a single entry: the failure's own path, operator and actual value |
| `every` element | already one failure per failing leaf of each element: each failure has its own entry (no change in how many failures there are) |
| `any` | the failing leaves of all branches (taken from the scratch recorders that are thrown away today), with no duplicates (same path + json_path + operator) |
| `not` | the leaves the negated condition looked at and that held, with their values: the inner condition is evaluated with a recorder that also notes passing leaves. When `not` fails because a path is missing, the entry for that path has `missing: true` |
| `some` | the failing leaves of every element (each one says which element it is through its index in `path`) |
| array expected but missing or not an array (`every`/`some`) | the array path itself |

Nested `any`/`not` flatten into the leaves. The list is capped at
**20 entries** per failure, the same scale as the cuts on `actual`. A failure
that hits the cap has no marker: the consumer sees the full `message` and
`fields` stays short. *(My call.)*

The example above then produces, for each extra container:

```json
{"path": "containers.#(state==\"running\")#.2", "operator": "any", "message": "…as today…",
 "fields": [{"path": "containers.#(state==\"running\")#.2.name", "operator": "in", "actual": "smoke-tls-proxy"}]}
```

`expected` is left out of `Field` on purpose: it is often the long part (an
`in` list) and is already in the failure. *(My call.)*

## Docs and tests

- `docs/output-and-results.md`: the `fields` key and its element table;
  `docs/state-and-events.md` example JSON.
- Tests in `internal/match` for every row of the table, including the
  docker example, nesting, the cap, and `missing`; a result/event JSON test
  that `fields` is `[]` and never `null`.

## Left to the builder

How the recorder notes leaves (an extra slice next to `out`, for example),
as long as the design rules hold: matching stays in `internal/match`, and
`gocyclo` ≤ 25.
