# Rule params in events — design

## Intent

A notification about a failed rule should say what was checked: the URL, the
path, the command. Today an event carries the rule name, module, tags,
failures and output, but not the rule's `params`. `onenotify` (the
notification tool that consumes the event queue) cannot show them.

Put the rule's `params` into the event, without exposing secrets, and give a
rule a way to say "send no details about me to third parties".

## Goals

- An event has a new key `params`: the rule's `params` **as written in the
  config, before `${VAR}` expansion**. A header written as
  `Authorization: "Bearer ${API_TOKEN}"` appears in the event exactly like
  that.
- A rule can keep its params out of events: `event_params: false`. The key
  also exists in `defaults`. Default: `true`.
- With `event_params: false`, and for a rule without `params`, the event has
  `"params": null`.
- An event has a new boolean key `redacted`: `true` for a rule with
  `event_params: false`, otherwise `false`. It tells a consumer that the rule's
  details must not be shown: `onenotify` then sends neither the params nor the
  error text and failure messages, only the rule name, module, tags and time.
  The event's `result` itself is unchanged.
- The `http` module no longer puts secrets into its error: a failed request
  is reported with the URL stripped of its query string and userinfo
  (`Get "https://example.com/path": connection refused`). Today
  `cmd/onecheck-http/collect.go` returns the error of `client.Do` as it is,
  and Go puts the full expanded URL into it, so a token in the query string
  reaches `result.error`, the log, the output and the event.
- `$${VAR}` (the escape for a literal `${VAR}`) is written to the event as it
  is in the config, unprocessed.
- Documented in `docs/state-and-events.md` (event format, the table of keys,
  a paragraph on secrets) and `docs/configuration.md` (`event_params` in the
  reference, in `defaults`, and in the section on `${VAR}`).

## Non-goals

- `params` in result files and in the output of `onecheck run`: only events
  change.
- Masking by field name, by module schema or by value pattern.
- Changes to the module protocol, or to any module other than the error text
  of `http`.

## Anti-goals

- An expanded environment value in the `params` key of an event.
- A query-string token or URL password in an error of the `http` module.

## Approaches

1. **Raw params before expansion, with a per-rule off switch — chosen by the
   owner.** Works the same for every module including third-party ones,
   covers free-form places a schema cannot describe (headers, `argv`, request
   body, a password inside a URL), shows exactly what the user sees in the
   config. Cost: a secret written into the config in plain text reaches the
   event; `event_params: false` and the documentation cover that.
2. Modules mark secret params in `params_schema`. Rejected: does not cover
   free-form headers and `argv`, needs work in every module.
3. Each module lists params that are safe to show. Rejected: third-party
   modules without a list show nothing, and `url` or `argv` can still hold a
   password.

## Design

- Config loading keeps the raw `params` of a rule next to the expanded ones
  (today `internal/config/load.go` expands in place with `expandEnv`). Modules
  keep receiving the expanded value; nothing else reads the raw one.
- `result.Event` gets `Params` (`json:"params"`), filled by the runner when it
  writes a transition event: the raw params, or `null` when the rule has
  `event_params: false` or no params.
- `event_params` is a boolean on a rule and in `defaults`, validated like the
  other rule keys (unknown keys stay errors).
- `result.Event` gets `Redacted` (`json:"redacted"`), the negation of the
  rule's effective `event_params`.
- `http` module: when the request fails, the module builds the error from the
  method, the URL without query and userinfo, and the underlying cause
  (`url.Error.Err`). The same redacted URL is used wherever the module puts a
  URL into an error, including redirects. The module version is bumped;
  documented in `docs/modules/http.md`.
- Known and accepted: failure messages contain the actual value
  (`... got <value>`, up to 1024 bytes), so a condition on a response body or
  command output puts a piece of it into the event. `docs/state-and-events.md`
  says so and points to `event_params: false` and `output_tail: 0`.
- This is an additive change to the event JSON: existing consumers ignore the
  new key.
- Event files keep mode `0640` in a `0750` directory.

## Tests

- A transition event of a rule with `${VAR}` in params holds the unexpanded
  text, and the module still receives the expanded value.
- `event_params: false` on the rule, and through `defaults`, gives
  `"params": null`; a rule-level `true` overrides a default `false`.
- A rule without `params` gives `"params": null`.
- `$${VAR}` reaches the event unprocessed.
- `redacted` is `true` exactly when the effective `event_params` is `false`.
- `http`: a request to a closed local port with `?token=secret` and
  `user:pass@` in the URL gives an error containing neither.
- The example in `docs/state-and-events.md` matches the real output.

## Left to the builder

- Names of the Go fields and where the raw params are carried between config
  and runner.
- Wording of the documentation.
