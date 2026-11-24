---
title: disk module
sidebar:
  label: disk
---

Reports free space and inodes of one filesystem, or of every filesystem of
the host, so a disk that is filling up
is noticed before services on it start to fail. Running out of inodes (many
small files: sessions, caches, mail queues) breaks writes just like running
out of space, so check both.

## Params

| Key | Required | Description |
|---|---|---|
| `path` | no | a mount point, any path inside a filesystem, or a device; without it [all filesystems](#all-filesystems) are listed |
| `exclude_fstype` | no | without `path`: list of shell patterns of filesystem types to leave out |
| `exclude_mount` | no | without `path`: list of shell patterns of mount points to leave out |

- A mount point (`/var`) or any path inside a filesystem
  (`/var/lib/postgresql`): the filesystem holding that path is reported.
  Symlinks are followed.
- A device under `/dev/` (`/dev/sda1`, `/dev/mapper/vg-data`,
  `/dev/disk/by-uuid/...`): its mount point is looked up in the mount table.
  Symlinks are resolved on both sides, so any name of the device works.
  A root filesystem listed in the mount table under a name that does not
  exist on disk (`/dev/root` on some systems) cannot be found by device;
  use its mount point, `/`, instead.

A rule with `path` checks one filesystem. A rule without it checks all of
them at once, including disks added later.

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
See [Checks](/checks#gt-gte-lt-lte).

Which fields are present:

- `inodes_*` are missing on filesystems without a fixed number of inodes
  (btrfs and some others report 0). A condition on them then fails with
  "got nothing (path is missing)"; leave inode conditions out for such disks.
  ZFS reports an inode count that grows and shrinks with free space, so
  `inodes_used_percent` on ZFS mostly repeats `used_percent`.
- `mountpoint`, `device` and `fstype` are missing if the mount table cannot
  be read; the numbers are still reported.

A path that does not exist, a device that is not mounted, or a filesystem
that cannot be queried is an `error`, not a `fail`.

A rule without `check` passes whenever the numbers could be read, as with any
module, so always give thresholds.

## All filesystems

Without `path` the module returns the filesystems of the host, and nothing
else:

| Field | Type | Description |
|---|---|---|
| `filesystems` | array | the filesystems, sorted by mount point |
| `count` | number | how many there are |

Each element has the fields of a single filesystem described above:
`mountpoint`, `device`, `fstype`, `total`, `free`, `used` (**bytes**),
`used_percent`, and the `inodes_*` fields where the filesystem has a fixed
number of inodes. Conditions go through
[`every` / `some` and `path`](/checks#every-some); `total`, `free` and
`used` accept unit literals there too.

```yaml
- name: disks
  module: disk
  check:
    count: { gte: 1 }
    filesystems:
      every:
        - { path: used_percent, lt: 90 }
        - { path: free, gt: 5GB }
```

A failure names the filesystem:

```
FAIL   disks   (disk, 5ms)
       filesystems.0.used_percent: expected lt 90, got 93.74 (element {"device":"/dev/vda1","free":10046664704,"fstype":"ext4","inodes_free":8166363,"inodes_total":10383360,"inodes_used":2216997,"inodes_used_percent":21.35,"mountpoint":"/","total":168998809600,"used":150345506816,"used_percent":93.74})
```

**Which filesystems are listed.** Only those that hold data and can fill up:

- the device is a path (`/dev/sda1`, `/dev/mapper/vg-data`), or the type is
  a network or pooled one: `zfs`, `nfs`, `nfs4`, `cifs`, `smb3`, `smbfs`,
  `ceph`, `9p`, `virtiofs`, `lustre`, `fuse.glusterfs`, `fuse.sshfs`;
- not a read-only image or a container layer: `squashfs` (snap), `iso9660`,
  `udf`, `erofs`, `cramfs`, `overlay`;
- not of size zero.

So `proc`, `sysfs`, `tmpfs`, cgroups, the overlays of containers and snap
images are never in the list.

**One device once.** A device mounted in several places (bind mounts, Docker
volumes, btrfs subvolumes) is listed once, at its shortest mount point. ZFS
datasets are separate devices and are all listed.

**Narrowing the list.** `exclude_fstype` and `exclude_mount` take
[`glob` patterns](/checks#glob-not_glob):

```yaml
- name: disks
  module: disk
  params:
    exclude_fstype: [nfs*, vfat]
    exclude_mount: [/mnt/backup, "/media/*"]
  check:
    filesystems: { every: { path: used_percent, lt: 90 } }
```

A filesystem that needs its own limits gets a rule with `path` and is
excluded here. `exclude_mount` matches the mount point the filesystem is
listed at.

Inodes: a filesystem without a fixed number of inodes has no `inodes_*`
keys, and `every` would fail on it with "path is missing". Check inodes
only where they exist:

```yaml
    filesystems.#(inodes_total>0)#:
      every: { path: inodes_used_percent, lt: 90 }
```

Things to know:

- If a listed filesystem cannot be queried (a stale NFS mount, no
  permission), the rule is an `error` that names the mount point: fix the
  mount or put it into `exclude_mount`. An excluded filesystem is not
  touched at all, so a network mount that hangs can be left out.
- No filesystems after the exclusions is `filesystems: []`, `count: 0`, not
  an error; `count: { gte: 1 }` catches a list that became empty.
- The fields of a single filesystem (`used_percent`, ...) are missing at the
  top level in this mode, and `filesystems`, `count` are missing with
  `path`. `exclude_fstype` and `exclude_mount` together with `path` are an
  error.
- macOS: the APFS volumes of one container are separate devices sharing the
  same space and are all listed; `exclude_mount: ["/System/Volumes/*"]`
  leaves the system ones out.

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
