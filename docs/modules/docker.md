# Module `docker`

Reports Docker containers, so you know that the containers which must serve
are running and healthy, that nothing restarts in a loop or dies of running
out of memory, and that no unknown container is running on the host.
Works on Linux and macOS with Docker, Docker Desktop, OrbStack, Colima,
rootless Docker and Podman (its Docker-compatible API).

The module always returns the same fields: the list of containers that
match the params, and their number. Params only narrow the list.

## Params

| Key | Required | Description |
|---|---|---|
| `name` | no | container name, exact (`nginx`) or a shell pattern (`app-*`), without the leading `/` |
| `label` | no | `key` (the label is set) or `key=value`, e.g. `com.docker.compose.project=shop` |
| `host` | no | where Docker is: `unix:///path/to/docker.sock` or `tcp://host:port`; found automatically when omitted |

Without `name` and `label` every container is listed, running or not (like
`docker ps -a`). With both, a container must match both. A filter that
matches nothing gives `count: 0`, not an error.

Patterns use the [`glob` syntax](../checks.md#glob-not_glob): `*`, `?`,
`[abc]`, `[!abc]`.

### Where Docker is

When `host` is not set, the module takes the first of:

1. the `DOCKER_HOST` environment variable;
2. the current Docker context (`DOCKER_CONTEXT`, else `docker context use`
   saved in `~/.docker/config.json`; `DOCKER_CONFIG` moves `~/.docker`). The
   `default` context means "no context";
3. the first socket that exists:
   - `/var/run/docker.sock`
   - `$XDG_RUNTIME_DIR/docker.sock` (rootless Docker)
   - macOS: `~/.docker/run/docker.sock` (Docker Desktop),
     `~/.orbstack/run/docker.sock`, `~/.colima/default/docker.sock`
   - `$XDG_RUNTIME_DIR/podman/podman.sock`, `/run/podman/podman.sock`

When onecheck runs from cron or a systemd timer, its environment usually has
no `DOCKER_HOST` and maybe no `HOME`: set `host` in the rule if Docker is not
at one of the default sockets.

Only `unix://` and `tcp://` (plain HTTP) addresses are supported; TLS and
`ssh://` are not.

Every error names the address and where it came from:

```
docker at unix:///var/run/docker.sock (default socket): dial unix /var/run/docker.sock: connect: permission denied; add the user onecheck runs as to the docker group or run it as root
```

Reading the Docker socket is equivalent to root access on the host; give
onecheck that access knowingly.

## Fields

| Field | Type | Description |
|---|---|---|
| `containers` | array | matching containers sorted by name |
| `count` | number | number of matching containers |

Each element of `containers` has all of these keys:

| Key | Type | Value |
|---|---|---|
| `name` | string | container name, without the leading `/` |
| `id` | string | short id (12 characters) |
| `image` | string | image the container was created from, as written in `docker run` or compose: `nginx:1.27`, `registry.local/app:3` |
| `state` | string | `created`, `running`, `paused`, `restarting`, `removing`, `exited`, `dead` |
| `health` | string | `healthy`, `unhealthy`, `starting`; `none` when the container has no healthcheck |
| `restart_count` | number | restarts done by the restart policy (`restart: always`, ...) |
| `exit_code` | number | exit code of the last run; 0 while it runs |
| `oom_killed` | bool | the last run was killed for running out of memory |
| `uptime_seconds` | number | seconds since the container started, while it is `running` or `restarting`; otherwise 0 |
| `labels` | object | container labels |

Element keys are checked with paths (`containers.0.state`) or with
[`every` / `some` and `path`](../checks.md#every-some). Unit literals do not
work inside elements: write `uptime_seconds: { gt: 3600 }`, not `1h`.

A rule that only checks `count` makes one request to Docker. A rule that
checks `containers` asks Docker about each matching container too.

## Recipes

### A container must run

```yaml
- name: nginx
  module: docker
  params: { name: nginx }
  check:
    count: 1                                    # it exists
    containers.0.state: running
    containers.0.health: { not_equals: unhealthy }
```

`count: 1` first: without it a missing container would only show as a
missing path.

### No unknown containers

Every running container must be allowed by its name, a name pattern or an
image pattern. Allowing does not require: a container that is not running is
not reported here (check required ones with the rule above).

```yaml
- name: no-unknown-containers
  module: docker
  check:
    containers.#(state=="running")#:
      every:
        any:
          - { path: name,  in: [nginx, postgres] }
          - { path: name,  glob: "app-*" }
          - { path: image, glob: "registry.local/*" }
```

Drop `#(state=="running")#` to require that stopped containers are known as
well.

### A whole compose project

```yaml
- name: shop
  module: docker
  params: { label: com.docker.compose.project=shop }
  check:
    count: { gte: 3 }
    containers:
      every:
        - { path: state, equals: running }
        - { path: health, not_equals: unhealthy }
```

### Crash loops and OOM kills

```yaml
- name: containers-stable
  module: docker
  check:
    containers:
      every:
        - { path: restart_count, lt: 5 }
        - { path: oom_killed, equals: false }
```

`restart_count` grows for the life of the container and is reset when it is
recreated (a new deploy), so pick a limit above what a normal deploy cycle
reaches.

### Image of a container

```yaml
- name: api-version
  module: docker
  params: { name: api }
  check:
    containers.0.image: { glob: "registry.local/api:2.*" }
```

## Output

```
FAIL   no-unknown-containers  (docker, 140ms)  [transition: fail]
       containers.#(state=="running")#.2: expected any [{"in":["nginx","postgres"],"path":"name"},{"glob":"app-*","path":"name"},{"glob":"registry.local/*","path":"image"}], got {"image":"xmrig/xmrig","name":"miner"}
FAIL   shop                   (docker, 110ms)  [transition: fail]
       containers.1.state: expected equals "running", got "exited"
```

The index (`.2`, `.1`) is the position in the list; the element's keys in
the message tell which container it is.
