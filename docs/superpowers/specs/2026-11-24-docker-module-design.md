# docker module and array conditions — design

## Intent

Bring the Docker checks of `~/github/infra` (`roles/security_monitor/files/security_monitor.py`)
to onecheck:

1. **Required containers** — a named container must exist, be `running` and
   not `unhealthy`.
2. **No strangers** — every running container must be allowed by its name, a
   name pattern or an image pattern; anything else is a FAIL naming the
   container.

Images matter only as a field of a container (user's decision). Network,
volumes, image inventory are out of scope.

## Goals

- Bundled module `docker`, Linux and macOS, one output shape regardless of
  params (user: "results must not differ in format between modes").
- Where Docker lives is configurable and, when not configured, found
  automatically for the system.
- The "no strangers" rule is expressed in the core, not in the module: new
  array conditions `every` / `some`, a `path` key inside conditions, and a
  shell-pattern operator `glob`. They work for every module.

## Non-goals

- TLS (`DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`) and `ssh://` hosts. A
  `tcp://` host is plain HTTP. An `ssh://` or `https`-requiring setup is an
  error that says it is not supported.
- Image inventory (dangling images, image age), `docker system df`, swarm,
  compose files.
- Unit literals (`1h`, `10MB`) inside array elements; element numbers are
  plain numbers.

## Anti-goals

- Docker unreachable must never look like "all clean": it is an `error`.
- Matching logic in the module. The module filters only by its params
  (`name`, `label`), like `systemd` picks one unit.

## Approaches (module shape)

- **A. One module `docker` — chosen.** Initially two modes (one container vs
  list); the user rejected different shapes, so modes became a *filter*:
  output is always the same list.
- B. Two modules `docker-container` / `docker-containers` — two binaries and
  pages for one subject.
- C. Only a list without per-container details — no `restart_count`,
  `exit_code`, `oom_killed` (Docker's list endpoint lacks them).

Transport: Docker Engine HTTP API over a unix socket or TCP with Go's
standard library (decided by me: the official SDK pulls in dozens of
dependencies; the `docker` CLI may not be installed).

## Part 1 — core: `glob`, `every`, `some`, `path`

### `glob`, `not_glob`

Leaf operator on strings; the argument is a shell pattern matched against the
**whole** string, case-sensitive, like Python's `fnmatchcase` used in infra:
`*` any run of characters (including `/`), `?` one character, `[abc]`,
`[a-z]`, `[!abc]` a character class. A malformed pattern (unclosed `[`) is a
compile error. A non-string value fails with "not applicable to a <type>".
The pattern compiler lives in `internal/glob` so the module uses the same
syntax for its `name` param.

### `path` inside a condition

In any condition mapping, the key `path` (a GJSON path) moves the remaining
operators of that mapping to the value found at that path *inside the
current value*:

```yaml
containers.0: { path: state, equals: running }   # same as containers.0.state: running
```

It is what makes element checks possible (below). The failure path shown is
`<outer path>.<path>`. A missing sub-path behaves like any missing path.
`path` with no other operator is a compile error. Unit literals do not apply
under `path`.

### `every`, `some`

Operators on arrays. The argument is a condition; a list is shorthand for
`all`:

```yaml
containers:
  every:
    - { path: state, equals: running }
    - { path: restart_count, lt: 5 }
names: { some: { glob: "web-*" } }
```

- `every` holds when the condition holds for each element. An empty array
  holds. Each failing element reports its own failures with the element index
  in the path: `containers.3.state: expected equals "running", got "exited"`.
- `some` holds when at least one element satisfies it. An empty array fails.
  One failure: `<path>: expected some <condition>, got <array>`.
- A value that is not an array fails ("not applicable to a <type>"); a
  missing path fails.
- Failures about an object element show its scalar keys (nested objects
  such as labels left out) so the container is named: an `any` failure as
  its actual value, other failures as an `(element {...})` note. (Changed
  after review: the whole element with labels overflowed the 1 KiB cut.)

"No strangers":

```yaml
check:
  containers.#(state=="running")#:
    every:
      any:
        - { path: name,  in: [nginx, postgres] }
        - { path: name,  glob: "app-*" }
        - { path: image, glob: "registry.local/*" }
```

## Part 2 — module `docker`

### Params

| Key | Description |
|---|---|
| `name` | container name, exact or a glob pattern (`app-*`); without a leading `/` |
| `label` | `key` (label present) or `key=value` |
| `host` | Docker address: `unix:///path` or `tcp://host:port` |

All optional. `name` and `label` together must both match. Without filters
every container (running or not, `docker ps -a`) is listed. A filter that
matches nothing gives `count: 0`, not an error.

### Fields

| Field | Type | Description |
|---|---|---|
| `containers` | array | matching containers sorted by name, each with the element fields below |
| `count` | number | number of matching containers |

Element fields — always all present:

| Key | Type | Value |
|---|---|---|
| `name` | string | without the leading `/` |
| `id` | string | 12-character short id |
| `image` | string | image reference the container was created from (`Config.Image`, e.g. `nginx:1.27`) |
| `state` | string | `created`, `running`, `paused`, `restarting`, `removing`, `exited`, `dead` |
| `health` | string | `healthy`, `unhealthy`, `starting`, or `none` without a healthcheck |
| `restart_count` | number | restarts by Docker's restart policy |
| `exit_code` | number | exit code of the last run (0 while running) |
| `oom_killed` | bool | the last run was killed for running out of memory |
| `uptime_seconds` | number | seconds since start while running or restarting, else 0 |
| `labels` | object | container labels |

### Collection

1. `GET /containers/json?all=1` → names, labels, ids. Filter by `name` and
   `label` in the module (glob from `internal/glob`).
2. When the request wants `containers` (or names no fields), for each match
   `GET /containers/{id}/json` → state, health, restart count, exit code, OOM,
   start time, image. A container removed between the two calls (404) is
   skipped. A rule checking only `count` makes one request.
3. Unversioned API paths: the daemon answers with its own API version, no
   negotiation needed.
4. Non-2xx responses are errors carrying Docker's `message`.

### Finding Docker

First that applies:

1. `params.host`;
2. `DOCKER_HOST`;
3. the current Docker context: `DOCKER_CONTEXT`, else `currentContext` in
   `$DOCKER_CONFIG/config.json` (default `~/.docker`); its endpoint from
   `contexts/meta/<sha256(name)>/meta.json` (`Endpoints.docker.Host`).
   Context `default` or none → next step;
4. the first existing socket of:
   - `/var/run/docker.sock`
   - `$XDG_RUNTIME_DIR/docker.sock` (rootless)
   - macOS: `~/.docker/run/docker.sock`, `~/.orbstack/run/docker.sock`,
     `~/.colima/default/docker.sock`
   - `$XDG_RUNTIME_DIR/podman/podman.sock`, `/run/podman/podman.sock`
     (Docker-compatible API).

No socket → error listing every place tried. Any connection or HTTP error
names the address and where it came from, e.g.
`docker at unix:///var/run/docker.sock (default socket): permission denied`;
permission denied adds a hint about the `docker` group. Schemes other than
`unix` and `tcp` are rejected as unsupported. All requests use the request
context (rule timeout).

## Tests

- `internal/glob`: table of patterns incl. `/`, classes, negation, escapes of
  regexp metacharacters (`.`, `+`), malformed pattern.
- `internal/match`: `glob`/`not_glob`, `every`/`some` passing and failing,
  empty arrays, non-arrays, missing, `path` inside, failures naming element
  indexes and carrying the element as actual, compile errors (`path` alone,
  `every` without argument).
- module: fake Docker API on a unix socket in `t.TempDir()`; host resolution
  with `t.Setenv`, temporary HOME and fake context files; 404 between list
  and inspect; count-only rule makes no inspect calls; daemon error message
  surfaced; unreachable socket is an error.

## Docs

`docs/checks.md` (new operators and `path`), `docs/modules/docker.md` with
recipes for both infra checks and a compose project, module lists in
README, getting-started, module-management, development, AGENTS.md;
`install.sh` installs `docker`.

## Left to the builder

File layout inside `cmd/onecheck-docker`, HTTP client details, error wording
beyond what is fixed above.
