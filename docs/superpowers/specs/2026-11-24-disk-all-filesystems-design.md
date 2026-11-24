# disk: all filesystems — design

## Intent

The `disk` module checks the filesystem of one `path`; a host needs a rule
per disk, and a disk added later is not watched until somebody writes its
rule. Without `path` the module lists the filesystems of the host, so one
rule covers every disk, present and future.

## Goals

- Without `path` the module returns `filesystems` (array) and `count`, like
  `docker` returns `containers`.
- By default only real filesystems are listed, and a device is listed once
  however many times it is mounted.
- `exclude_fstype` and `exclude_mount` narrow the list.
- Sizes in elements accept unit literals (`free: { gt: 5GB }`), using the
  element types of `docs/superpowers/specs/2026-11-24-element-types-design.md`.

## Non-goals

- Changing the mode with `path`: its params and fields stay as they are.
- An "include" list or per-filesystem thresholds: a filesystem with its own
  limits gets a rule with `path` and is excluded from the list.
- Disks that are not mounted, SMART, I/O errors.

## Anti-goals

- A list that always fails out of the box: read-only images (snap's
  squashfs) are 100 % full by nature and must not be in it.
- A filesystem silently missing from the list because it could not be read.

## Approaches

1. **A list of elements checked with `every` — chosen by the owner** ("as
   `docker` returns `containers`"), together with element types in the
   engine so byte thresholds work.
2. One rule generated per filesystem by onecheck. Rejected: rules, state and
   events are keyed by name; generated names change with the host.

## Design (builder's calls)

- **Params.** `path` becomes optional. `exclude_fstype`, `exclude_mount`:
  lists of shell patterns (`internal/glob`) matched against the filesystem
  type and the mount point; allowed only without `path`.
- **Real filesystem.** Listed when all hold:
  - the device is a path (`/dev/sda1`, `/dev/mapper/vg-data`), or the type is
    one of `zfs`, `nfs`, `nfs4`, `cifs`, `smb3`, `smbfs`, `ceph`, `9p`,
    `virtiofs`, `lustre`, `fuse.glusterfs`, `fuse.sshfs`;
  - the type is not a read-only image or a layer: `squashfs`, `iso9660`,
    `udf`, `erofs`, `cramfs`, `overlay`;
  - its size is not zero.
  This drops `proc`, `sysfs`, `tmpfs`, `devtmpfs`, cgroups, `overlay` of
  containers, snap images.
- **One device once.** Entries with the same device string keep the one
  with the shortest mount point (the first in the mount table on a tie):
  bind mounts and btrfs subvolumes of one device are one filesystem. ZFS
  datasets are different devices and are all listed.
- **Fields.** Each element: `mountpoint`, `device`, `fstype`, `total`,
  `free`, `used` (bytes), `used_percent`, and `inodes_total`, `inodes_free`,
  `inodes_used`, `inodes_used_percent` when the filesystem has a fixed inode
  count, computed exactly as with `path`. Sorted by mount point. `count` is
  the length. An empty list is `[]` and 0, not an error.
- **Errors.** The mount table cannot be read, or a listed filesystem cannot
  be queried (`statfs` fails): the rule is an `error` naming the mount
  point, which can then be put into `exclude_mount`. Exclusions are applied
  before querying, so an excluded filesystem is never touched (a hung NFS
  mount can be excluded).
- **Order of work.** Keep real types, drop duplicates, apply the exclude
  patterns, then `statfs` each, then drop zero-sized ones. Duplicates go
  before the patterns so that excluding a filesystem by its mount point
  does not bring it back under another mount point of the same device.
- macOS: the same rules; APFS volumes of one container are different
  devices and are all listed, `exclude_mount: ["/System/Volumes/*"]` narrows
  them. Documented, not special-cased.

## Tests

- Selection on a made-up mount table: pseudo filesystems, overlay and
  squashfs dropped; ext4, xfs, zfs, nfs kept; a device mounted three times
  listed once at its shortest mount point; zero-sized dropped.
- `exclude_fstype` and `exclude_mount` with patterns; an invalid pattern and
  either key together with `path` are "invalid params" errors.
- Elements carry the same numbers as the mode with `path`; inode keys absent
  without a fixed inode count; sorted by mount point; JSON `[]` when empty.
- A `statfs` failure is an error naming the mount point; an excluded mount
  point is not queried.
- `describe` declares `total`, `free`, `used` of elements as bytes; a rule
  `filesystems: { every: { path: free, gt: 1GB } }` validates.
- On the real host: the filesystem of the temp dir's device is in the list.
