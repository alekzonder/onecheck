# Module protocol

This page is for people writing a module. A module teaches onecheck a new
kind of check: Postgres, Redis, DNS, a message queue, anything.

## The idea

A module is a standalone executable named `onecheck-<name>`. onecheck starts
it once per check, hands it the rule's `params` on stdin as JSON, and reads
**field values** from its stdout as JSON.

A module never decides whether a check passed. It returns facts ("status is
503", "the file is 2 KB"); onecheck evaluates the rule's `check` conditions
on them. That is why every operator, JSON assertions and JSON Schema work for
every module without the module doing anything.

A module can be written in any language. For Go there is a small SDK,
`github.com/alekzonder/onecheck/pkg/module`.

Protocol version described here: **1**.

## Invocations

A module must implement two subcommands.

### `onecheck-<name> describe`

Prints a JSON description on stdout and exits with `0`. It must be fast and
free of side effects: onecheck calls it on every run and on install.

```json
{
  "protocol": 1,
  "name": "redis",
  "version": "0.3.0",
  "description": "Ping a Redis server and read INFO",
  "params_schema": {
    "type": "object",
    "additionalProperties": false,
    "required": ["addr"],
    "properties": {
      "addr": { "type": "string" },
      "password": { "type": "string" }
    }
  },
  "fields": {
    "ping":        { "type": "string",   "description": "reply to PING" },
    "role":        { "type": "string",   "description": "master or replica" },
    "used_memory": { "type": "bytes",    "description": "memory used by Redis" },
    "uptime":      { "type": "duration", "description": "time since start" },
    "info":        { "type": "object",   "description": "all INFO keys" }
  }
}
```

| Key | Required | Description |
|---|---|---|
| `protocol` | yes | protocol version the module speaks |
| `name` | yes | module name; must equal `<name>` in the file name |
| `version` | yes | module version (semver) |
| `description` | no | one line, shown by `onecheck module list` |
| `params_schema` | no | JSON Schema (draft 2020-12) for the rule's `params`; onecheck validates `params` with it before starting a check |
| `fields` | yes | every field the module can return: name → `type`, `description` |

Field types:

| Type | JSON value | Notes |
|---|---|---|
| `string`, `number`, `bool`, `object`, `array` | the JSON type | |
| `bytes` | number of bytes | conditions may use `10MB` etc. |
| `duration` | number of seconds, may be fractional | conditions may use `500ms`, `1h` etc. |

Declaring fields lets `onecheck validate` reject a condition on a field that
does not exist. Use `"additionalProperties": false` in `params_schema` so
that typos in `params` are caught.

### `onecheck-<name> check`

Reads one request from stdin, prints one response on stdout.

Request:

```json
{
  "protocol": 1,
  "rule": "cache-alive",
  "params": { "addr": "127.0.0.1:6379" },
  "fields": ["ping", "used_memory"],
  "max_field_size": 10485760,
  "timeout_ms": 30000
}
```

| Key | Description |
|---|---|
| `protocol` | protocol version onecheck is using for this call |
| `rule` | rule name, for the module's own log messages |
| `params` | the rule's `params`, already validated against `params_schema` |
| `fields` | the fields the rule's conditions use. A module may skip collecting everything else, which matters for expensive fields. Empty when the rule has no conditions: then collect only what is cheap |
| `max_field_size` | largest size in bytes of one string value; do not read more than `max_field_size + 1` bytes of anything |
| `timeout_ms` | the rule's timeout; finish before it |

Response:

```json
{
  "fields": { "ping": "PONG", "used_memory": 1048576 },
  "truncated": [],
  "error": null
}
```

| Key | Description |
|---|---|
| `fields` | field name → value. Omit a field that has no value in this situation |
| `truncated` | names of string fields that were cut to `max_field_size`. onecheck refuses to evaluate conditions on them |
| `error` | `null`, or `{"message": "..."}` when the data could not be collected |

## Rules of behaviour

**Errors versus values.** Return `error` when you could not find out:
connection refused, permission denied, timeout. Return a value when you found
out, even if the answer is bad: an HTTP 500 is `status: 500`, a missing file
is `exists: false`, a non-zero exit code is `exit_code: 3`. An `error` makes
the rule's status `error`; values are judged by the user's conditions.

**Exit code.** Exit with `0` whenever you produced a response, including one
with `error` set. A non-zero exit code, invalid JSON on stdout, or no output
means the module itself is broken; the rule gets status `error`.

**stdout is for the response only.** Write diagnostics to stderr. onecheck
puts a module's stderr into its log at level `debug`.

**Timeouts.** onecheck starts a module in its own process group and kills
the whole group a short moment after `timeout_ms`, and also when onecheck
itself is interrupted. Leave child processes in that group (do not call
`setsid`/`setpgid` for them), so they are cleaned up with the module. Prefer to stop on your own before that and
return an `error` saying what timed out.

**Size limits.** Never read an unbounded amount into memory. Read at most
`max_field_size + 1` bytes; if there was more, cut the value to
`max_field_size` and list the field in `truncated`.

**No state, no side effects.** A module is started fresh for every check and
may run in parallel with other checks, including other instances of itself.

**Environment.** A module inherits onecheck's environment and runs with its
privileges.

## Writing a module in Go

The SDK implements the protocol; you provide a description and one function.

```go
// Command onecheck-uptime reports how long the machine has been up.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	// File lets tests point the module at a fixture.
	File string `json:"file"`
}

func collect(_ context.Context, req module.Request) (map[string]any, error) {
	p := params{File: "/proc/uptime"}
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p.File)
	if err != nil {
		return nil, err // could not find out: an error
	}
	seconds, err := strconv.ParseFloat(strings.Fields(string(data))[0], 64)
	if err != nil {
		return nil, fmt.Errorf("unexpected content of %s", p.File)
	}
	return map[string]any{"uptime": seconds}, nil
}

func main() {
	module.Main(module.Module{
		Description: module.Description{
			Name:        "uptime",
			Version:     "0.1.0",
			Description: "Time since the machine booted",
			ParamsSchema: json.RawMessage(`{
				"type": "object",
				"additionalProperties": false,
				"properties": {"file": {"type": "string"}}
			}`),
			Fields: map[string]module.FieldInfo{
				"uptime": {Type: module.TypeDuration, Description: "time since boot"},
			},
		},
		Collect: collect,
	})
}
```

```sh
go build -o /usr/lib/onecheck/modules/onecheck-uptime .
```

```yaml
rules:
  - name: no-recent-reboot
    module: uptime
    check:
      uptime: { gt: 10m }
```

What the SDK does for you:

- `module.Main` handles `describe` and `check`, fills in `protocol`, applies
  `timeout_ms` to the context passed to `Collect`, turns a returned error
  into the `error` response, and cuts over-long string values while recording
  them in `truncated`.
- `module.DecodeParams` decodes `params` strictly (unknown keys are errors).
- `req.Wants("field")` tells whether a rule uses a field;
  `req.WantsOr("field", cheap)` additionally answers `cheap` when the rule
  has no conditions at all. Use them to skip expensive work.

## Writing a module in another language

Anything that can read stdin and print JSON works. A complete module in
shell:

```sh
#!/bin/sh
# onecheck-loadavg: 1, 5 and 15 minute load averages
case "$1" in
  describe)
    cat <<'JSON'
{"protocol": 1, "name": "loadavg", "version": "0.1.0",
 "description": "System load averages",
 "params_schema": {"type": "object", "additionalProperties": false},
 "fields": {
   "load1":  {"type": "number", "description": "1 minute load average"},
   "load5":  {"type": "number", "description": "5 minute load average"},
   "load15": {"type": "number", "description": "15 minute load average"}}}
JSON
    ;;
  check)
    cat >/dev/null                      # the request is not needed here
    if read -r l1 l5 l15 _ < /proc/loadavg; then
      printf '{"fields": {"load1": %s, "load5": %s, "load15": %s}, "truncated": [], "error": null}\n' "$l1" "$l5" "$l15"
    else
      printf '{"fields": {}, "truncated": [], "error": {"message": "cannot read /proc/loadavg"}}\n'
    fi
    ;;
  *)
    echo "usage: $0 describe|check" >&2
    exit 2
    ;;
esac
```

## Trying a module by hand

```sh
onecheck-uptime describe | jq .

echo '{"protocol":1,"rule":"t","params":{},"fields":["uptime"],"max_field_size":1048576,"timeout_ms":5000}' \
  | onecheck-uptime check | jq .

onecheck module list          # is it found, is it compatible
onecheck validate             # do my rules fit its params and fields
onecheck --rule no-recent-reboot --log-level debug
```

## Distributing a module

Publish a GitHub release with assets named `onecheck-<name>_<os>_<arch>` and
a `checksums.txt`; then anyone can `onecheck module install <owner/repo>`.
Details: [module-management.md](module-management.md#repositories-of-third-party-modules).

## Protocol versions

The protocol version is a single integer. It changes only when the JSON
exchanged above changes incompatibly. onecheck supports a range of versions
(`onecheck version` prints it) and passes the module's own version back in
each request. Adding optional keys to requests or responses does not change
the version: modules must ignore request keys they do not know, and onecheck
ignores response keys it does not know.
