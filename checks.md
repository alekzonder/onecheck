---
title: Checks
---

The `check` block of a rule says what must be true about the values a module
returned. It is evaluated by onecheck itself, by one engine, so everything on
this page works the same for every module: bundled or third-party.

```yaml
rules:
  - name: api-health
    module: http
    params: { url: https://example.com/health }
    check:
      status: 200                                  # field: condition
      headers.content-type: { contains: json }     # path into a field
      body:
        json:
          assert:
            - { path: status, equals: ok }
```

A rule **passes** when every condition holds. Each condition that does not
hold is reported separately, with what was expected and what was found:

```
body json status: expected equals "ok", got "degraded"
```

A rule without `check` passes whenever its module could collect the data.

## Paths

Each key of `check` is a path to a value. The first part is a field of the
module (see the module's page for its fields); further parts go into objects
and arrays:

| Path | Meaning |
|---|---|
| `status` | the field `status` |
| `headers.content-type` | key `content-type` of the object field `headers` |
| `items.0` | first element of an array |
| `items.#` | number of elements of an array |

Paths use [GJSON path syntax](https://github.com/tidwall/gjson/blob/master/SYNTAX.md).
A key that itself contains a dot is written with a backslash: `headers.x\.y`.

A field that does not exist for the module is a config error, caught by
`onecheck validate`.

## Conditions

A condition is one of:

- **a plain value** — shorthand for `equals`:

  ```yaml
  status: 200          # same as  status: { equals: 200 }
  exists: true
  ```

- **a mapping of operators** — all of them must hold:

  ```yaml
  size: { gt: 0, lt: 10MB }
  ```

- **a combinator** — `all`, `any`, `not`, described [below](#combining-conditions).

To compare with an object, use `equals` explicitly, because a bare mapping is
read as operators.

## Operators

| Operator | Applies to | Holds when |
|---|---|---|
| [`equals`](#equals-not_equals) / `not_equals` | anything | the value is (not) exactly this |
| [`contains`](#contains-not_contains) / `not_contains` | string, array, object | substring / element / key is (not) present |
| [`starts_with`](#starts_with-ends_with) / `ends_with` | string | the string starts / ends with this |
| [`matches`](#matches-not_matches) / `not_matches` | string | the regular expression matches (does not match) |
| [`glob`](#glob-not_glob) / `not_glob` | string | the whole string matches (does not match) a shell pattern |
| [`gt`, `gte`, `lt`, `lte`](#gt-gte-lt-lte) | number | greater / greater-or-equal / less / less-or-equal |
| [`in`](#in-not_in) / `not_in` | anything | the value is (not) one of the list |
| [`exists`](#exists) | anything | the path is present (`true`) or absent (`false`) |
| [`type`](#type) | anything | the value has this JSON type |
| [`length`](#length) | string, array, object | a nested condition holds for the length |
| [`every`](#every-some) / `some` | array | a nested condition holds for each / at least one element |
| [`path`](#path) | anything | the other operators apply to a value inside this one |
| [`json`](#json) | string | the string is JSON and satisfies `assert` / `schema` |

An operator applied to a value of the wrong type (for example `gt` on a
string) does not hold, and the message says which type was found. An unknown
operator or a malformed argument is a config error.

### `equals`, `not_equals`

Exact comparison of JSON values. There is no conversion between types: the
number `200` is not equal to the string `"200"`.

```yaml
status: { equals: 200 }
mode: { equals: "0644" }           # quote it: mode is a string
exists: { equals: true }
headers.x-env: { not_equals: staging }
tags: { equals: [a, b] }           # arrays and objects compare as a whole
```

### `contains`, `not_contains`

```yaml
body: { contains: "status: ok" }       # string: substring
stdout: { not_contains: ERROR }
roles: { contains: admin }             # array: has an element equal to this
headers: { contains: x-request-id }    # object: has this key
```

### `starts_with`, `ends_with`

```yaml
headers.content-type: { starts_with: application/json }
target: { ends_with: .conf }
```

### `matches`, `not_matches`

Regular expressions in [Go RE2 syntax](https://pkg.go.dev/regexp/syntax). The
expression may match anywhere in the string; anchor it with `^` and `$` for a
full match.

```yaml
stdout: { matches: 'version \d+\.\d+' }
body: { not_matches: '(?i)exception|traceback' }     # (?i): ignore case
```

Command output usually ends with a newline, and `$` only matches at the very
end of the text. Use the multi-line flag `(?m)` to let `^` and `$` match at
line boundaries:

```yaml
stdout: { matches: '(?m)^active$' }
```

Write regular expressions in single quotes so YAML leaves backslashes alone.

### `glob`, `not_glob`

Shell patterns, matched against the **whole** string, case-sensitive:

| Pattern | Matches |
|---|---|
| `*` | any run of characters, including `/` and none at all |
| `?` | exactly one character |
| `[abc]`, `[a-z]` | one character from the set or range |
| `[!abc]` | one character not in the set |

Everything else is literal: `.` is a dot, `+` is a plus.

```yaml
image: { glob: "registry.local/*" }      # any image from this registry
name: { not_glob: "tmp-*" }
```

Quote patterns that start with `*` or `[`: YAML reads those characters
specially.

### `gt`, `gte`, `lt`, `lte`

Numeric comparison.

```yaml
status: { gte: 200, lt: 300 }
exit_code: { lte: 1 }
```

Fields that hold a size or a time span accept literals with units. The
module's page marks such fields as **bytes** or **duration**:

```yaml
size: { lt: 10MB }             # B, KB, MB, GB, TB (1024-based)
age: { lt: 1h }                # ms, s, m, h, d, w; also 1h30m
duration: { lte: 500ms }
tls_expires_in: { gt: 14d }
```

Unit literals work for `equals`, `not_equals`, `in`, `not_in` too, and only
directly on such a field (not on values inside JSON).

A module that returns a list can mark keys of its elements as **bytes** or
**duration** as well. A literal then works wherever a condition addresses
that key of one element:

```yaml
filesystems: { every: { path: free, gt: 5GB } }       # under every / some
filesystems.0.free: { gt: 5GB }                        # by index
filesystems.#(mountpoint=="/").free: { gt: 5GB }       # first match of a query
filesystems.#(fstype=="ext4")#:                        # a filtered list
  every: { path: free, gt: 5GB }
```

It does not work on a list of values (`filesystems.#.free`) or on a key the
module's page does not mark.

### `in`, `not_in`

```yaml
status: { in: [200, 204, 304] }
type: { not_in: [symlink, other] }
```

### `exists`

Whether the path is present at all. Use it for optional keys and for the
absence of something:

```yaml
headers.strict-transport-security: { exists: true }
headers.x-debug-token: { exists: false }
```

`exists` is the only operator that can hold for an absent path; see
[Missing values](#missing-values).

> The `file` module also has a *field* named `exists` (does the file exist).
> `check: { exists: true }` compares that field; it is the plain-value
> shorthand, not this operator.

### `type`

One of `string`, `number`, `bool`, `object`, `array`, `null`.

```yaml
headers: { type: object }
body:
  json:
    assert:
      - { path: items, type: array }
      - { path: deleted_at, type: "null" }     # quote null
```

### `length`

Applies a nested condition to the length of a string (in characters), an
array (elements) or an object (keys). A plain number means `equals`.

```yaml
stdout: { length: { gt: 0 } }          # not empty
body: { length: { lte: 4096 } }
items: { length: 3 }
```

### `every`, `some`

Apply a condition to the elements of an array. The argument is a condition;
a list of conditions means all of them.

```yaml
containers:
  every:                                   # each element must satisfy all
    - { path: state, equals: running }
    - { path: restart_count, lt: 5 }
tags: { some: { glob: "env-*" } }          # at least one element
```

| | holds when | empty array |
|---|---|---|
| `every` | the condition holds for each element | holds |
| `some` | the condition holds for at least one element | does not hold |

A value that is not an array, or a missing path, does not hold.

`every` reports each failing element separately, with its index in the
path. For an element that is an object the message also shows its simple
keys (strings, numbers, booleans; nested objects such as labels are left
out), so you see which element it was:

```
containers.2.state: expected equals "running", got "exited" (element {"id":"3f2a…","name":"old","state":"exited",...})
containers.#(state=="running")#.1: expected any [...], got {"id":"9c1e…","image":"xmrig/xmrig","name":"miner",...}
```

The index counts elements of the array the condition is applied to; after a
query such as `#(state=="running")#` that is the filtered array.

`some` reports one failure with the whole array.

Combine with a [GJSON query](#paths) to pick elements first, for example only
running containers:

```yaml
containers.#(state=="running")#:
  every:
    any:
      - { path: name,  in: [nginx, postgres] }
      - { path: image, glob: "registry.local/*" }
```

A query that matches nothing gives an empty array: `every` holds, `some`
does not.

### `path`

Inside any condition mapping, `path` moves the other operators of that
mapping to a value inside the current one. It is how conditions reach the
fields of array elements:

```yaml
containers.0: { path: state, equals: running }   # same as containers.0.state: running
```

`path` uses the same [GJSON syntax](#paths) as check keys and needs at least
one operator next to it. A missing sub-path is handled like any
[missing value](#missing-values). Unit literals (`10MB`, `1h`) apply under
`path` only when it names a key of an element that the module marks as
**bytes** or **duration**, see [above](#gt-gte-lt-lte); otherwise compare
with plain numbers.

### `json`

Parses a string field as JSON and checks the document. It works on any string
field: an HTTP `body`, a command's `stdout`, a file's `content`. A string that
is not valid JSON does not hold.

```yaml
body:
  json:
    assert:      # optional: list of path + operators
      - { path: status, equals: ok }
    schema:      # optional: a JSON Schema
      type: object
      required: [status]
```

At least one of `assert` and `schema` is required; both may be used together.

#### `json.assert`

A list of assertions. Each has a `path` into the parsed document and any of
the operators above (several operators on one line are ANDed):

```yaml
body:
  json:
    assert:
      - { path: status, equals: ok }
      - { path: version, matches: '^2\.' }
      - { path: data.items, type: array }
      - { path: data.items.#, gte: 1 }                   # at least one item
      - { path: data.items.0.id, type: number }
      - { path: data.user.email, exists: true }
      - { path: error, exists: false }
      - { path: 'data.items.#(state=="failed")#', length: 0 }   # no failed items
      - { path: 'data.items.#.name', contains: primary }        # some item is named primary
      - path: latency_ms
        any:
          - { lt: 100 }
          - { equals: null }
```

Paths are [GJSON paths](https://github.com/tidwall/gjson/blob/master/SYNTAX.md):

| Path | Selects |
|---|---|
| `a.b.c` | nested keys |
| `items.0` | array element by index |
| `items.#` | number of elements |
| `items.#.name` | the `name` of every element, as an array |
| `items.#(state=="ok")` | first element whose `state` is `ok` |
| `items.#(state=="ok")#` | all such elements, as an array |
| `items.#(size>100)#.name` | names of all elements with `size` over 100 |

#### `json.schema`

A [JSON Schema](https://json-schema.org/) (draft 2020-12) written in YAML.
Use it when the *shape* of the document matters: required keys, types,
enumerations, formats of nested objects.

```yaml
body:
  json:
    schema:
      type: object
      required: [status, checks]
      properties:
        status: { enum: [ok, degraded] }
        checks:
          type: array
          minItems: 1
          items:
            type: object
            required: [name, state]
            properties:
              state: { enum: [ok] }
```

Every schema violation becomes its own failure, with the location inside the
document:

```
body json /checks/1/state: schema violation: value must be 'ok'
```

`assert` is shorter for "this key has this value"; `schema` is better for
structure. They combine freely.

## Combining conditions

Operators written side by side are ANDed. For other logic use combinators;
they nest to any depth.

```yaml
# any: at least one must hold
status:
  any:
    - { equals: 200 }
    - { equals: 304 }

# all: every one must hold (same as writing the operators side by side,
# but allows the same operator twice)
body:
  all:
    - { contains: "db: ok" }
    - { contains: "cache: ok" }

# not: must not hold
stdout:
  not: { matches: '(?i)error|fatal' }

# nested
body:
  any:
    - { contains: "status: ok" }
    - all:
        - { contains: "status: degraded" }
        - not: { contains: "db: down" }
```

When `any` does not hold, one failure is reported for the whole `any`.

## Missing values

A path can be absent: an optional header, a key that is not in the JSON, a
field the module does not return in this situation (the `file` module returns
no `size` for a file that does not exist).

For an absent path **every operator fails**, including the negative ones
(`not_equals`, `not_contains`, `not_matches`, `not_in`) and `not: {...}`. The
only exceptions are `exists: false` and a `not` around `exists`. The message
says `got nothing (path is missing)`.

The reason: missing data must never make a check pass by accident. If an API
stops returning the key you are watching, the check fails instead of going
quiet. When absence is acceptable, say so explicitly:

```yaml
- path: error
  any:
    - { exists: false }
    - { equals: null }
```

An explicit JSON `null` is a present value: `exists: true` holds, and so does
`type: "null"`.

## Large values

A module returns at most `run.max_field_size` (default `10MB`) per field.
When a condition uses a field that had to be cut, the rule ends with the
status `error` rather than passing or failing on partial data. Raise the
limit, or check something smaller (a file's `size` instead of its `content`).

## Recipes

```yaml
# a file exists, is not empty, and is fresh
check:
  exists: true
  size: { gt: 0 }
  age: { lt: 25h }

# a file must NOT exist
check:
  exists: false

# a JSON config file has a value
check:
  content:
    json:
      assert:
        - { path: db.host, equals: localhost }

# HTTP: status, header, body substring, response time
check:
  status: { in: [200, 204] }
  headers.content-type: { starts_with: application/json }
  body: { contains: '"healthy":true' }
  duration: { lt: 1s }

# TLS certificate valid for at least two more weeks
check:
  tls_expires_in: { gt: 14d }

# command: exit code and a number parsed from output
check:
  exit_code: 0
  stdout:
    json:
      assert:
        - { path: queue.length, lt: 1000 }

# command output has no error lines and is not empty
check:
  stdout:
    length: { gt: 0 }
    not_matches: '(?im)^error'
```
