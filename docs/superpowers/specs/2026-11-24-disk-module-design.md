# Module `disk` — design

## Intent

Notice that a disk is filling up before services on the server fail because
of it. Running out of inodes kills services just as surely as running out of
bytes (millions of small files: sessions, caches, mail queues), so both are
covered.

## Goals

- A bundled module `disk` (`cmd/onecheck-disk`) that reports the space and
  inode usage of one filesystem.
- One rule checks one disk, so a failure names the disk exactly. Several
  disks are several rules (e.g. in `conf.d/disks.yaml`).
- `params.path` is a mount point, any path inside a filesystem (the
  filesystem holding it is checked) or a device such as `/dev/sda1` or
  `/dev/mapper/vg-data` (its mount point is found in the mount table).
- Works on Linux and macOS, no new dependencies.

## Non-goals

- Checking all disks of the host automatically. It needs a generic
  "discover" extension of the module protocol (the core asks a module which
  rules to generate, each generated rule gets its own state and events). That
  is a separate design; `disk` will gain discovery then.
- Default thresholds: a rule without `check` passes, as for every module.
  The documentation gives a ready example with recommended thresholds.
- Any comparison inside the module (AGENTS.md: modules only collect).

## Approaches

1. **One filesystem per rule, explicit `path`** (chosen). Same shape as
   `file`; failure messages name the disk; per-disk thresholds.
2. All disks in one rule with aggregated "worst disk" fields and a
   module-supplied default `check`. Rejected: the failure message names a
   value, not a disk, and default checks need a protocol change.
3. Thresholds in `params`, the module decides. Rejected: comparison belongs
   to `internal/match`.

## Params

| Key | Required | Description |
|---|---|---|
| `path` | yes | mount point, path inside a filesystem, or device under `/dev/` |

## Fields

| Field | Type | Description |
|---|---|---|
| `total` | bytes | size of the filesystem |
| `free` | bytes | space available to unprivileged users (`df` "Avail") |
| `used` | bytes | `total` minus all free blocks (including root-reserved) |
| `used_percent` | number | `used / (used + free) * 100`, like `df` "Use%" (reserved blocks count as unavailable) |
| `inodes_total` | number | inodes of the filesystem |
| `inodes_free` | number | free inodes |
| `inodes_used` | number | `inodes_total - inodes_free` |
| `inodes_used_percent` | number | `inodes_used / inodes_total * 100` |
| `mountpoint` | string | where the filesystem is mounted |
| `device` | string | the mounted device as in the mount table |
| `fstype` | string | filesystem type, e.g. `ext4`, `xfs`, `apfs` |

Percentages are floats rounded to two decimals.

## Behaviour

- A path that does not exist, a device that is not mounted, or a statfs
  failure is an error (status `error`), not a failure: the check cannot be
  carried out.
- Filesystems that report `inodes_total == 0` (btrfs, zfs and others with no
  fixed inode count) return no `inodes_*` fields; a condition on them fails
  with "field is missing". Showing 0 % would hide the fact that the number is
  meaningless. (Our call.)
- `mountpoint`, `device`, `fstype` come from the mount table entry with the
  longest mount point that contains the path (Linux: `/proc/self/mounts`;
  macOS: `getfsstat`). If the mount table cannot be read, these three fields
  are omitted and the numeric fields are still returned.
- A device path is recognised as a path under `/dev/` that is a block or
  character device; it is resolved through the mount table (symlinks such as
  `/dev/disk/by-uuid/...` and `/dev/mapper/...` are resolved before
  comparing). When a device is mounted several times, the first entry wins.

## Other changes

- `install.sh`: `MODULES="file http command disk"`; `internal/installsh`
  test updated.
- `docs/modules/disk.md`; lists of bundled modules in `README.md`,
  `docs/getting-started.md`, `docs/module-management.md`,
  `docs/development.md`.
- `Makefile` and the release pick the module up by its `cmd/onecheck-*`
  directory; nothing to change there.

## Tests

No network, only `t.TempDir()`:

- a path inside the temp dir returns plausible `total >= free`, percentages
  in `[0, 100]`, a non-empty `mountpoint`;
- a missing path is an error;
- the mount-table parser (Linux format, escaped spaces `\040`) and the
  longest-prefix lookup, on fixed text;
- device resolution against a fixed mount table;
- percentage computation, including `inodes_total == 0`.

## Left to the builder

File layout inside `cmd/onecheck-disk`, build-tagged files for the
platform-specific statfs and mount-table code.
