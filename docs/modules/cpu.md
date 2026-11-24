# Module `cpu`

Reports how busy the host's CPUs are, the load average and the number of
cores, so a host that is overloaded for a long time is noticed before its
services slow down. Works on Linux and macOS.

## Params

| Key | Required | Description |
|---|---|---|
| `window` | no | how long CPU usage is measured: `500ms`, `5s`, `1m`, or a bare number of seconds; default `1s`; from `100ms` to `5m` |

CPU usage is the difference of two readings of the CPU counters: the module
reads them, waits `window`, and reads them again. The window must be shorter
than the rule's `timeout` (default 30s), otherwise the module is killed and
the rule is an `error`. A longer window smooths short bursts; for sustained
load combine a short window with `fail_after` (see
[state-and-events.md](../state-and-events.md#thresholds-fail_after-pass_after)).

The module waits only when a `*_percent` field is checked (or saved as
output). A rule that checks only load average answers immediately.

## Fields

Usage over the window, in percent of all cores together (a 4-core host with
one core fully busy shows 25), two decimals:

| Field | Type | Description |
|---|---|---|
| `usage_percent` | number | busy time: everything except idle and iowait |
| `user_percent` | number | programs (user and nice time) |
| `system_percent` | number | kernel (system, irq and softirq time) |
| `iowait_percent` | number | idle while waiting for disk; **Linux only** |
| `steal_percent` | number | time a virtual machine waited for the hypervisor; **Linux only** |

Without waiting:

| Field | Type | Description |
|---|---|---|
| `cores` | number | logical CPUs |
| `load1`, `load5`, `load15` | number | load average over 1, 5 and 15 minutes |
| `load1_per_core`, `load5_per_core`, `load15_per_core` | number | load average divided by `cores` |

### Which field to use

- **CPU busy:** `usage_percent`. A high `steal_percent` on a virtual machine
  means the hypervisor is short of CPU, not your programs.
- **Load:** prefer `*_per_core` to `load*`: the same threshold then fits
  hosts with any number of cores. Load above 1 per core means work is
  waiting for a CPU. `load5`/`load15` react slower and ignore short spikes.
- On Linux load average also counts processes waiting for disk, so high
  load with low `usage_percent` and high `iowait_percent` points to slow
  storage rather than to the CPU.

Which fields are present:

- `iowait_percent` and `steal_percent` are missing on macOS, which does not
  report these times; a condition on them fails with "got nothing (path is
  missing)".
- `*_percent` fields are missing when no rule condition or saved output asks
  for them.

Inside a container the module reports the host's CPUs and load, not the
container's limits.

Reading the counters failing is an `error`, not a `fail`.

## Examples

```yaml
# conf.d/cpu.yaml
rules:
  # busy for three runs in a row (three minutes from cron)
  - name: cpu
    module: cpu
    params: { window: 5s }
    fail_after: 3
    check:
      usage_percent: { lt: 90 }

  # load too high for the number of cores; answers without waiting
  - name: load
    module: cpu
    check:
      load5_per_core: { lt: 1.5 }
```

A failure names the rule and the value:

```
FAIL   cpu   (cpu, 5006ms)
       usage_percent: expected lt 90, got 97.32
```
