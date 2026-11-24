# Unit literals inside array elements — design

## Intent

Modules that return a list (`docker` today, `disk` and `systemd` next)
describe each element as an object. A condition on a key of an element
cannot use unit literals: `filesystems: { every: { path: free, gt: 5GB } }`
is a compile error, because only top-level fields have a type. The owner
decided the engine learns the types of element keys, so a list of disks can
be checked by free space the same way one disk is.

## Goals

- A module can declare, for an array field, the types of the keys of its
  elements: `"elements": {"free": "bytes", "uptime": "duration"}` next to
  `type` in `describe`.
- Unit literals then work on those keys wherever a condition addresses one
  element key:
  - under `every` / `some` with `path: <key>`, also through `all`, `any`,
    `not` and a list of conditions;
  - on a path to one element: `field.0.key`, `field.#(query).key`;
  - on one element with `path`: `field.0: { path: key, ... }`;
  - under `every` / `some` on a filtered list: `field.#(query)#: { every: ... }`.
- `docker` declares `uptime_seconds` as `duration`.
- Documented in `docs/checks.md`, `docs/module-protocol.md`,
  `docs/modules/docker.md`.

## Non-goals

- Types for nested objects or for arrays inside elements.
- Types for values inside `json`.
- Validating that a `path` under `every` names a declared key: elements may
  carry keys the module did not type.

## Anti-goals

- A literal silently compared as a string: where a key has no declared type,
  `gt: 5GB` stays the compile error it is today.

## Approaches

1. **Element key types in `describe`, threaded through compilation — chosen
   by the owner** (over leaving literals unsupported in elements). The engine
   stays the only place that understands conditions; modules only declare.
2. Modules return extra pre-formatted fields. Rejected: comparison logic
   would leak into modules.

## Design (builder's calls)

- **Protocol.** `FieldInfo.Elements map[string]FieldType`,
  `json:"elements,omitempty"`. Additive: an older onecheck ignores the key
  (literals in elements stay an error there), an older module simply has
  none. `ProtocolVersion` stays 1.
- **Engine.** `match.Compile(raw, types, elements)` with
  `ElementTypes map[string]FieldTypes`. Compilation carries a context
  instead of a bare field type: the type of the current value, the key types
  of the current object, the key types of the elements of the current array.
  `every`/`some` turn "elements of the array" into "keys of the object";
  `path: key` turns "keys of the object" into "type of the value".
- **Top-level paths.** After the field name: `.<index>` or `.#(query)`
  addresses one element, `.#(query)#` a list of elements, and one more
  `.<key>` a typed key. Anything else is untyped, as today.
- Only `bytes` and `duration` change behaviour; other declared types are
  documentation.

## Tests

- Literals accepted and evaluated in every form listed under Goals; the
  failure of an `every` still names the element.
- Still a compile error: a key without a declared type; `field.#.key` (a
  list of numbers); a module without `elements`.
- `describe` of a module round-trips `elements`; the runner passes them to
  the engine (a rule on a test module compiles a literal in an element).
- `docker`: `containers: { every: { path: uptime_seconds, gt: 1h } }`
  compiles.
