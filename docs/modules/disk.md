# Module `disk`

Reports free space and inodes of one filesystem, so a disk that is filling up
is noticed before services on it start to fail. Running out of inodes (many
small files: sessions, caches, mail queues) breaks writes just like running
out of space, so check both.

## Params

| Key | Required | Description |
|---|---|---|
| `path` | yes | a mount point, any path inside a filesystem, or a device |

- A mount point (`/var`) or any path inside a filesystem
  (`/var/lib/postgresql`): the filesystem holding that path is reported.
  Symlinks are followed.
- A device under `/dev/` (`/dev/sda1`, `/dev/mapper/vg-data`,
  `/dev/disk/by-uuid/...`): its mount point is looked up in the mount table.
  Symlinks are resolved on both sides, so any name of the device works.

One rule checks one filesystem; write one rule per disk. Checking all disks
automatically is not supported yet.

## Fields

| Field | Type | Description |
|---|---|---|
| `total` | **bytes** | size of the filesystem |
| `free` | **bytes** | space available to ordinary users, like `df` "Avail" |
| `used` | **bytes** | used space: `total` minus all free blocks |
| `used_percent` | number | `used / (used + free) * 100`, like `df` "Use%" |
| `inodes_total` | number | inodes of the filesystem |
| `inodes_free` | number | free inodes |
| `inodes_used` | number | used inodes |
| `inodes_used_percent` | number | `inodes_used / inodes_total * 100` |
| `mountpoint` | string | where the filesystem is mounted |
| `device` | string | the mounted device, as in the mount table |
| `fstype` | string | filesystem type, e.g. `ext4`, `xfs`, `apfs` |

Percentages have two decimals (`89.1`); `df` rounds them up (`90%`).

Blocks reserved for root (5 % on ext4 by default) count as unavailable, as in
`df`: `used_percent` reaches 100 when ordinary users can no longer write,
even though root still can.

**bytes** fields accept unit literals in conditions: `free: { gt: 5GB }`.
See [checks.md](../checks.md#gt-gte-lt-lte).

Which fields are present:

- `inodes_*` are missing on filesystems without a fixed number of inodes
  (btrfs, zfs and some others report 0). A condition on them then fails with
  "got nothing (path is missing)"; leave inode conditions out for such disks.
- `mountpoint`, `device` and `fstype` are missing if the mount table cannot
  be read; the numbers are still reported.

A path that does not exist, a device that is not mounted, or a filesystem
that cannot be queried is an `error`, not a `fail`.

A rule without `check` passes whenever the numbers could be read, as with any
module, so always give thresholds.

## Examples

```yaml
# conf.d/disks.yaml
rules:
  # recommended thresholds for a system disk
  - name: disk-root
    module: disk
    params: { path: / }
    check:
      used_percent: { lt: 90 }
      inodes_used_percent: { lt: 90 }

  # data disk: alert by absolute free space, after two runs in a row
  - name: disk-data
    module: disk
    fail_after: 2
    params: { path: /var/lib/postgresql }
    check:
      free: { gt: 20GB }
      used_percent: { lt: 85 }

  # by device name; also make sure it is mounted where expected
  - name: disk-backup
    module: disk
    params: { path: /dev/mapper/vg-backup }
    check:
      mountpoint: /backup
      used_percent: { lt: 95 }
```

A failure names the rule and the value:

```
FAIL   disk-root   (disk, 1ms)
       used_percent: expected lt 90, got 93.4
```
