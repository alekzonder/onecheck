# Failure Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every `match.Failure` carries `fields`, a short list of the concrete leaf fields behind it with their values. `message` stays as it is.

**Architecture:** The recorder in `internal/match/eval.go` builds a default
one-entry `fields` for every failure it records. `any`, `some` and `not`
replace that entry with leaves collected from their scratch evaluations. `not`
uses a new optional `held *[]Field` on the recorder, which notes leaves that
held.

**Tech Stack:** Go, gjson, the existing `internal/match` tests.

**Spec:** `docs/superpowers/specs/2026-11-25-failure-fields-design.md`

## Global Constraints

- `message`, `path`, `json_path`, `operator`, `expected`, `actual` keep their current values.
- `fields` is always a JSON array, never `null`.
- At most 20 entries per failure (`maxFields = 20`), deduplicated by path + json_path + operator.
- No protocol change; matching stays in `internal/match`; gocyclo ≤ 25; `make check` passes.
- Commit style `chg:`/`docs:`, no attribution trailers (AGENTS.md).

## Review Focus

- `any` nested inside `every` (the docker case): one failure per element, `fields` holds `name` with the container name.
- `not` over a missing path: the entry has `missing: true`, not a silently empty list.
- `some` over a large array: the cap holds at 20.
- `json.assert` leaf under `any`: the field keeps `json_path`.
- A `Failure` built outside the recorder (e.g. `output_test.go`) still marshals `fields` as `[]`.

---

### Task 1: `Field` type and failure fields in the matcher

**Files:**
- Modify: `internal/match/eval.go`, `internal/match/json.go` (no change expected beyond what `r.add` does)
- Test: `internal/match/fields_test.go` (new)

**Interfaces:**
- Produces: `type Field struct{ Path, JSONPath string; Operator string; Actual any; Missing bool }`
  with tags `path`, `json_path,omitempty`, `operator`, `actual`, `missing,omitempty`.
  `Failure.Fields []Field \`json:"fields"\``, plus `func (f Failure) MarshalJSON() ([]byte, error)` that writes nil as `[]`.

- [ ] **Step 1: Write failing tests** in `fields_test.go`, using `evalOn`/`eval`:
  - `TestFieldsLeaf`: `status: 201` → `Fields == [{Path:"status", Operator:"equals", Actual:200.0}]`.
  - `TestFieldsMissing`: `nothing: {equals: x}` → one field with `Missing: true`, `Actual: nil`.
  - `TestFieldsEveryAny`: the docker case (containers with states and names, `containers.#(state=="running")#: {every: {any: [{path: name, in: [a, b]}]}}`) → one failure per unknown container, each with `Fields == [{Path:"containers.#(state==\"running\")#.N.name", Operator:"in", Actual:"<name>"}]`; the `Message` is the same as before the change.
  - `TestFieldsAnyBranchesDeduped`: `any` with two branches on different paths → both leaves; the same leaf twice → one entry.
  - `TestFieldsNot`: `{path: x, not: {equals: 1}}` where x = 1 → field `x`, operator `equals`, actual 1. `not` over a missing path → field with `Missing: true`.
  - `TestFieldsSomeCapped`: `some` over 30 elements with none matching → `len(Fields) == 20`.
  - `TestFieldsJSONAssertUnderAny`: `body: {any: [{json: {assert: {status: ok}}}]}` → field with `JSONPath: "status"`.
  - `TestFailureFieldsMarshalEmpty`: `json.Marshal(Failure{})` contains `"fields":[]`.

- [ ] **Step 2: Run** `go test ./internal/match -run 'Fields|FailureFields'` → FAIL (no `Fields`).

- [ ] **Step 3: Implement** in `eval.go`:
  - `recorder.add`: if `f.Fields == nil`, set it to one entry built from the failure (`r.path`, the failure's json path, operator, actual). `fail` marks `Missing` when `!v.exists`; go through a helper `failFields(op, expected, v, note string, fields []Field)` and let `fail` call it with nil.
  - `recorder.held *[]Field`, copied by `scratch`/`child`. `leaf.eval` and `existsNode.eval` append a held entry when the condition holds and `r.held != nil`. `jsonNode` sub-recorders must carry `held` too.
  - `anyNode`: collect the `Fields` of the scratch failures of every branch → `failFields("any", …, merge(...))`.
  - `someNode`: the same over every element's scratch failures.
  - `notNode`: run the inner condition on `scratch()` with a fresh `held`. For a missing path, use the default entry (Missing). Otherwise the fields are held + scratch failure fields.
  - `merge(fields []Field) []Field`: dedupe by path + json_path + operator, keeping order, cut to `maxFields`.

- [ ] **Step 4: Run** `go test ./internal/match` → PASS (old tests unchanged).

- [ ] **Step 5: Commit** `chg: list the concrete failed fields in each failure`.

### Task 2: Docs

**Files:** `docs/output-and-results.md` (failure table, plus a `fields` table and example), `docs/state-and-events.md` (event example JSON).

- [ ] **Step 1:** Add the `fields` row and an element table (`path`, `json_path`, `operator`, `actual`, `missing`), with a short `any` example showing the docker case.
- [ ] **Step 2:** `make docs-build` and `make check` pass.
- [ ] **Step 3: Commit** `docs: describe the failed fields of a failure`.
