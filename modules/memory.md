---
title: memory module
sidebar:
  label: memory
---

Reports physical memory and swap usage of the host, so memory running out
is noticed before the system starts swapping heavily or killing processes.
Works on Linux and macOS.

## Params

None. `params` may be omitted.

## Fields

| Field | Type | Description |
|---|---|---|
| `total` | **bytes** | physical memory |
| `available` | **bytes** | memory that can be given to programs without swapping |
| `used` | **bytes** | `total` minus `available` |
| `used_percent` | number | `used / total * 100` |
| `swap_total` | **bytes** | swap size |
| `swap_used` | **bytes** | swap in use |
| `swap_free` | **bytes** | free swap |
| `swap_used_percent` | number | `swap_used / swap_total * 100`; 0 when there is no swap |

Percentages have two decimals.

On Linux `available` is the kernel's `MemAvailable`: page cache and other
memory the kernel can reclaim at once are not counted in `used`, as in the
"available" column of `free`. A host with a large file cache therefore
shows low `used_percent` even though little memory is "free"; that is
normal.

On macOS `available` is free plus inactive memory. `used` then still
includes active file cache and compressed and wired memory, so it reads
higher than "Memory Used" in Activity Monitor, often 75–90 % on an idle Mac.
On Macs prefer a threshold on `available` (`available: { gt: 2GB }`) or a
higher `used_percent` limit.

macOS has no fixed swap: it adds swap files as needed, so `swap_total`
grows and shrinks, and a high `swap_used_percent` there is less alarming
than on Linux.

**bytes** fields accept unit literals in conditions:
`available: { gt: 1GB }`. See [Checks](/checks#gt-gte-lt-lte).

Inside a container the module reports the host's memory, not the
container's limit.

Reading memory or swap failing is an `error`, not a `fail`.

## Examples

```yaml
# conf.d/memory.yaml
rules:
  - name: memory
    module: memory
    check:
      used_percent: { lt: 90 }
      swap_used_percent: { lt: 50 }

  # a small host: keep at least 512 MB available
  - name: memory-available
    module: memory
    check:
      available: { gt: 512MB }
```

A failure names the rule and the value:

```
FAIL   memory   (memory, 2ms)
       used_percent: expected lt 90, got 93.41
```
