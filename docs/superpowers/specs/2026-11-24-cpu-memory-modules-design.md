# Modules `cpu` and `memory` — design

## Intent

Rules that notice an overloaded host: CPU busy for a long time, load average
too high for the number of cores, memory or swap running out. The bundled
modules so far check one object each (a file, a URL, a disk, a unit); these
two check the host's CPU and the host's memory. Network is out of scope for
now (the user postponed it).

## Goals

- Linux and macOS from the first version (macOS was required up front).
- One module per resource, the same shape as `disk`: the module only
  collects numbers, thresholds live in `check` (`internal/match`).
- CPU usage is measured over a configurable window inside one run.
- A rule on load average or memory answers immediately, without waiting.

## Non-goals

- Network (postponed).
- Container limits (cgroups): inside Docker the modules report the host.
- Per-core usage ("one core at 100 %") and per-process numbers.
- Averaging CPU over minutes: short spikes are already smoothed by
  `fail_after` (consecutive non-passing runs), and load average covers the
  long view.

## Approaches

1. **Two modules `cpu` and `memory` — chosen.** Load average belongs to
   `cpu` because it only makes sense against the number of cores, which
   `cpu` knows, so `load5_per_core: { lt: 1.5 }` works on any host.
   `memory` needs no window.
2. Three modules `cpu`, `load`, `memory`: smallest modules, but both `cpu`
   and `load` need the core count, and it is three binaries, docs and
   install entries for two resources.
3. One module `system` with everything: one binary, but unlike every other
   module it has no single job, and rules are written per resource anyway so
   that `fail_after` and events are per resource.

Platform code: `github.com/shirou/gopsutil/v4` (chosen by the user). It
reads `/proc` on Linux and calls the system through `purego` on macOS, so
binaries still build with `CGO_ENABLED=0` for every release platform
(linux/darwin × amd64/arm64).

## Module `cpu`

### Params

| Key | Required | Description |
|---|---|---|
| `window` | no | how long CPU usage is measured: an onecheck duration (`500ms`, `5s`, `1m`); default `1s`; from `100ms` to `5m` |

`window` is a string parsed with `internal/units.ParseDuration` (a bare
number is seconds), validated in the module: out of range or unparsable is
an `error` naming the value. The window must be shorter than the rule's
`timeout` (default 30s), otherwise the core kills the module; the docs say
so. The wait respects the context (cancelled → error).

### Fields

Usage over the window, percent of all cores together, two decimals, number:

| Field | Computation (deltas of `cpu.Times(false)` over the window) |
|---|---|
| `usage_percent` | `(total − idle − iowait) / total` |
| `user_percent` | `(user + nice) / total` |
| `system_percent` | `(system + irq + softirq) / total` |
| `iowait_percent` | `iowait / total` — Linux only |
| `steal_percent` | `steal / total` — Linux only |

`total` is the sum of all `TimesStat` parts; on Linux `guest` and
`guestNice` are subtracted because they are already counted in `user`/`nice`
(the same as gopsutil's own `Percent`). A zero or negative `total` delta
(counter reset, too short a window) gives 0 for every percent field
(builder's call). Values are clamped to 0..100.

Without waiting:

| Field | Type | Description |
|---|---|---|
| `cores` | number | logical CPUs (`cpu.Counts(true)`) |
| `load1`, `load5`, `load15` | number | load average, two decimals |
| `load1_per_core`, `load5_per_core`, `load15_per_core` | number | load divided by `cores`, two decimals |

On Linux load average counts processes waiting for disk too (state D), so a
high load with low `usage_percent` points to I/O; the docs say so.

### When the window is waited

Only when the request wants one of the `*_percent` fields
(`req.WantsOr(field, false)` — an empty field list means "collect everything
cheap", and waiting is not cheap). A rule that checks only load answers
immediately. `iowait_percent`/`steal_percent` are absent on macOS
(`runtime.GOOS != "linux"`); a condition on them fails with "path is
missing", as for other platform-specific fields.

### Errors

Reading times, counts or load failing is an `error` with gopsutil's
message. `cores` of 0 (cannot happen in practice) → `*_per_core` absent.

## Module `memory`

No params (`additionalProperties: false`, empty object allowed).

| Field | Type | Description |
|---|---|---|
| `total` | bytes | physical memory |
| `available` | bytes | memory that can be given to programs without swapping (Linux `MemAvailable`; macOS free + inactive pages as gopsutil computes it) |
| `used` | bytes | `total − available`: page cache is not counted as used, as in `free` and Activity Monitor |
| `used_percent` | number | `used / total * 100`, two decimals |
| `swap_total` | bytes | swap size |
| `swap_used` | bytes | swap in use |
| `swap_free` | bytes | free swap |
| `swap_used_percent` | number | `swap_used / swap_total * 100`; 0 when there is no swap |

`swap_used_percent` is 0 without swap rather than absent (changed after the
conversation, my call): a host without swap cannot run out of it, and an
absent field would make the recipe `swap_used_percent: { lt: 50 }` fail
forever on such hosts. `disk` treats an empty whole the same way.

`used` is computed by the module from `total` and `available` (not
gopsutil's `Used`, which differs between platforms). On macOS swap is
dynamic: `swap_total` grows as swap files are added; the docs say so.
Reading memory or swap failing is an `error`.

## Shared

- Both modules are added to `install.sh` (`MODULES`, help text) and its test,
  to the module lists in README.md, docs/getting-started.md,
  docs/module-management.md, docs/development.md, AGENTS.md, and get pages
  `docs/modules/cpu.md` and `docs/modules/memory.md` with recipes.
  `Makefile` discovers modules by directory, nothing to change there.
- Tests run without the real system's numbers: the collection functions take
  the gopsutil calls through a small struct of function values (package
  variables in the module) that tests replace with fixtures; pure functions
  (percent computation, field building) are tested directly. One smoke test
  per module may call the real gopsutil and check only that fields exist and
  are in range. No network, nothing written outside `t.TempDir()`.

## Recipes for the docs

```yaml
- name: cpu
  module: cpu
  params: { window: 5s }
  fail_after: 3
  check:
    usage_percent: { lt: 90 }
    load5_per_core: { lt: 1.5 }

- name: memory
  module: memory
  check:
    used_percent: { lt: 90 }
    swap_used_percent: { lt: 50 }
```

## Left to the builder

File layout inside each `cmd/onecheck-<name>`, names of internal helpers,
exact field descriptions in `describe`, exact error wording.
