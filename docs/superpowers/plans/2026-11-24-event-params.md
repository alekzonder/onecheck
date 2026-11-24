# Rule Params in Events Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Events carry the rule's params as written in the config and a `redacted` flag; the `http` module stops leaking the expanded URL in its errors.

**Architecture:** The config loader keeps the raw params next to the expanded ones; the runner copies them into the event. The `http` module rebuilds request errors around a URL without query and userinfo.

**Tech Stack:** Go, as the rest of the repository.

**Spec:** `docs/superpowers/specs/2026-11-24-event-params-design.md`

## Global Constraints

- `AGENTS.md` applies: commit style `chg:` / `fix:` / `docs:`, no attribution trailers, `make check` passes, tests stay inside `t.TempDir()` and off the network (loopback is fine).
- Event JSON change is additive: new keys `params` and `redacted` only.
- Modules keep receiving expanded params.

## Review Focus

- Params holding non-string values (numbers, lists, nested maps): written to the event unchanged.
- A rule that is invalid (`Rule.Err`) and still produces a transition: the event is written with whatever raw params were read, never with expanded ones.
- A redirect to a URL with a token in the query: the error names neither the original nor the redirect query.
- A URL that cannot be parsed: the error does not echo it back whole.
- `event_params` given a non-boolean value: the rule is reported as invalid, like any other bad key.

---

### Task 1: Raw params and `event_params` in the config

**Files:**
- Modify: `internal/config/types.go`, `internal/config/load.go`, `internal/config/config.go` (defaults)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Rule.RawParams map[string]any` (params as written, `nil` when the rule has none); `Rule.EventParams bool`; `Defaults.EventParams bool` with YAML key `event_params`, default `true`.

- [ ] **Step 1: Failing tests** in `config_test.go`:
  - `TestRawParamsKeepVariables`: with `t.Setenv("TOKEN","s3cret")`, params `{url: "https://x/?t=${TOKEN}", n: 3, list: [a, "${TOKEN}"], esc: "$${TOKEN}"}` give `Params["url"] == "https://x/?t=s3cret"`, `RawParams["url"] == "https://x/?t=${TOKEN}"`, `RawParams["n"] == 3`, `RawParams["list"].([]any)[1] == "${TOKEN}"`, `RawParams["esc"] == "$${TOKEN}"`.
  - `TestEventParamsDefaultsAndOverride`: default `true`; `defaults.event_params: false` gives `false`; a rule with `event_params: true` under that default gives `true`; a rule with `event_params: false` gives `false`.
  - `TestEventParamsNotBoolean`: `event_params: maybe` sets `Rule.Err`.
- [ ] **Step 2:** `go test ./internal/config` fails.
- [ ] **Step 3: Implement.** `rawRule.EventParams *bool`; add `event_params` to `ruleKeys` and to the message listing known keys; `buildRule` stores `raw.Params` in `RawParams` before `expandEnv` (which already returns a new map). `Defaults.EventParams` needs a default of `true` while still accepting an explicit `false`: set it in `defaultConfig()`.
- [ ] **Step 4:** `go test ./internal/config` passes.
- [ ] **Step 5:** `git commit -m "chg: keep rule params as written and add event_params to rules and defaults"`

### Task 2: `params` and `redacted` in the event

**Files:**
- Modify: `internal/result/result.go`, `internal/runner/runner.go`
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Consumes: `Rule.RawParams`, `Rule.EventParams`.
- Produces: `result.Event.Params map[string]any` (`json:"params"`, `null` when hidden or absent), `result.Event.Redacted bool` (`json:"redacted"`).

- [ ] **Step 1: Failing tests** in `runner_test.go`, following the existing transition tests:
  - `TestEventCarriesRawParams`: env var in params; the event file has the unexpanded text and `"redacted": false`; the module received the expanded value (assert through the fake module or the result, as the neighbouring tests do).
  - `TestEventParamsHidden`: `event_params: false` gives `"params": null`, `"redacted": true`.
  - `TestEventWithoutParams`: a rule without params gives `"params": null`, `"redacted": false`.
- [ ] **Step 2:** `go test ./internal/runner` fails.
- [ ] **Step 3: Implement** in the place where `result.Event` is built (`runner.go`, near line 233).
- [ ] **Step 4:** `go test ./internal/runner` passes.
- [ ] **Step 5:** `git commit -m "chg: write rule params and the redacted flag into events"`

### Task 3: `http` errors without the query string and userinfo

**Files:**
- Modify: `cmd/onecheck-http/collect.go`
- Test: `cmd/onecheck-http/collect_test.go`

**Interfaces:**
- Produces: `safeURL(raw string) string` — scheme, host and path only; `"<invalid url>"` when it cannot be parsed. `requestError(err error) error` — for a `*url.Error`: `<Op> "<safeURL(URL)>": <Err>`; anything else unchanged.

- [ ] **Step 1: Failing tests:**
  - `TestSafeURL` table: `https://u:p@h.example/a/b?t=secret#f` → `https://h.example/a/b`; `http://h:8080/` → `http://h:8080/`; `"http://[::1"` → `<invalid url>`.
  - `TestRequestErrorHidesSecrets`: request to a closed loopback port with `user:pass@` and `?token=secret`; the error contains neither `secret` nor `pass`, and contains the host and path.
  - `TestRedirectErrorHidesSecrets`: an `httptest` server redirects to a closed port with `?token=secret`; the error does not contain `secret`.
- [ ] **Step 2:** `go test ./cmd/onecheck-http` fails.
- [ ] **Step 3: Implement**; wrap the errors of `http.NewRequestWithContext` and `client(p).Do`.
- [ ] **Step 4:** `go test ./cmd/onecheck-http` passes.
- [ ] **Step 5:** `git commit -m "fix: keep the query string and password of the URL out of http errors"`

### Task 4: Documentation

**Files:**
- Modify: `docs/state-and-events.md`, `docs/configuration.md`, `docs/modules/http.md`, `examples/config.yaml`

- [ ] **Step 1:** `state-and-events.md`: add `params` and `redacted` to the example and the key table; a section "What an event can reveal" — params are as written in the config, failure messages carry the actual value (up to 1024 bytes), how to hide with `event_params: false` and `output_tail: 0`.
- [ ] **Step 2:** `configuration.md`: `event_params` in the full reference (rule and `defaults`) and in the `${VAR}` section.
- [ ] **Step 3:** `modules/http.md`: errors name the URL without query and userinfo.
- [ ] **Step 4:** `examples/config.yaml`: `event_params: true` under `defaults` with a comment.
- [ ] **Step 5:** `make check && make docs-build` pass.
- [ ] **Step 6:** `git commit -m "docs: describe params and redacted in events and event_params"`
