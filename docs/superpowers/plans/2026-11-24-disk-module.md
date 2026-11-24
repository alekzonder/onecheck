# Module `disk` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** a bundled module `onecheck-disk` that reports space and inode usage of the filesystem holding one path or device.

**Architecture:** `cmd/onecheck-disk` like `cmd/onecheck-file`: pure helpers (mount-table parsing and lookup, field computation) tested on fixed input, plus small build-tagged files for the platform calls (`statfs`, mount table) on Linux and macOS.

**Tech Stack:** Go 1.26.5, `syscall` only, `pkg/module` SDK.

**Spec:** `docs/superpowers/specs/2026-11-24-disk-module-design.md`

## Global Constraints

- No new dependencies (`syscall`, not `golang.org/x/sys`).
- Builds for `linux` and `darwin`, `amd64` and `arm64`.
- Modules only collect; no comparison in the module.
- Tests: no network, write only under `t.TempDir()`.
- `make check` passes; gocyclo limit 25.
- Commit style `chg:`/`docs:`, lower case, no attribution trailers.

## Review Focus

1. A path with spaces in the mount point (`/proc/self/mounts` escapes as `\040`) — must be found.
2. A symlinked path (`/var/data -> /mnt/data`) — the filesystem of the target is reported.
3. `/` as the only matching mount point — longest-prefix lookup must not match `/var` for `/variable`.
4. `inodes_total == 0` — no `inodes_*` fields, no division by zero.
5. Device given through a symlink (`/dev/mapper/x`, `/dev/disk/by-uuid/...`) — matched after resolving symlinks on both sides.

---

### Task 1: mount table parsing and lookup

**Files:**
- Create: `cmd/onecheck-disk/mounts.go`
- Test: `cmd/onecheck-disk/mounts_test.go`

**Interfaces:**
- Produces:
  - `type mount struct { Device, Mountpoint, FSType string }`
  - `func parseMounts(text string) []mount` — Linux `/proc/self/mounts` format, octal escapes (`\040`, `\011`, `\134`) decoded.
  - `func mountOf(ms []mount, path string) (mount, bool)` — entry with the longest mount point that equals `path` or is a directory prefix of it (last entry wins on equal mount points, as the kernel shows overmounts later).
  - `func mountOfDevice(ms []mount, device string, resolve func(string) string) (mount, bool)` — first entry whose `resolve(Device) == resolve(device)`.

- [ ] **Step 1: tests** `TestParseMountsDecodesEscapes`, `TestMountOfLongestPrefix` (`/variable` → `/`, `/var/lib/x` → `/var`, `/var` → `/var`), `TestMountOfLaterEntryWins`, `TestMountOfDeviceResolvesSymlinks` (resolve maps `/dev/mapper/vg-data` → `/dev/dm-0`; table has `/dev/dm-0`).
- [ ] **Step 2:** `go test ./cmd/onecheck-disk/` → FAIL (undefined).
- [ ] **Step 3:** implement.
- [ ] **Step 4:** `go test ./cmd/onecheck-disk/` → PASS.

### Task 2: field computation

**Files:**
- Create: `cmd/onecheck-disk/fields.go`
- Test: `cmd/onecheck-disk/fields_test.go`

**Interfaces:**
- Produces:
  - `type usage struct { Total, Free, Avail, Files, FilesFree uint64 }` — bytes for the first three (`Free` = all free blocks, `Avail` = for unprivileged users).
  - `func usageFields(u usage) map[string]any` — keys `total`, `free` (= Avail), `used` (= Total-Free), `used_percent` (= used/(used+Avail)*100, 0 if the sum is 0), and if `Files > 0`: `inodes_total`, `inodes_free`, `inodes_used`, `inodes_used_percent`. Percentages rounded to 2 decimals. Byte values as `int64`, counts as `uint64`→`int64`.

- [ ] **Step 1: tests** `TestUsageFieldsLikeDf` (Total 100, Free 30, Avail 20 → used 70, used_percent 77.78), `TestUsageFieldsNoInodes` (Files 0 → no `inodes_*` keys), `TestUsageFieldsInodes` (Files 1000, FilesFree 250 → used 750, 75), `TestUsageFieldsEmpty` (all 0 → used_percent 0).
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS.

### Task 3: platform code, collect, describe

**Files:**
- Create: `cmd/onecheck-disk/main.go`, `collect.go`, `sys_linux.go`, `sys_darwin.go`
- Test: `cmd/onecheck-disk/collect_test.go`

**Interfaces:**
- Consumes: Task 1, Task 2.
- Produces (per platform): `func statfs(path string) (usage, error)`, `func mountTable() ([]mount, error)` (Linux: read `/proc/self/mounts`; macOS: `syscall.Getfsstat`, `Mntfromname`/`Mntonname`/`Fstypename`). Linux block size: `Frsize` if non-zero, else `Bsize`.
- `collect`: decode params (`path` required, schema `minLength: 1`); if `path` is under `/dev/` and is a device → `mountOfDevice` (resolve = `filepath.EvalSymlinks`, fallback to input), error `device <p> is not mounted` if not found, then stat its mount point; otherwise `filepath.Abs` + `EvalSymlinks` (error on missing path), `statfs`, `mountOf` for `mountpoint`/`device`/`fstype` (omitted if the table cannot be read or no entry).

- [ ] **Step 1: tests** `TestCollectTempDir` (fields `total`≥`free`, `0 ≤ used_percent ≤ 100`, `mountpoint` non-empty and a prefix of the temp dir's real path), `TestCollectMissingPathIsError`, `TestCollectEmptyPathIsError`, `TestDescribeListsFields` (description has all 11 fields, types `bytes` for `total`/`free`/`used`).
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS; `GOOS=darwin GOARCH=arm64 go vet ./cmd/onecheck-disk/` clean.
- [ ] **Step 5:** commit `chg: add the disk module that reports space and inode usage of a filesystem`.

### Task 4: installer and docs

**Files:**
- Modify: `install.sh:12`, `internal/installsh/install_test.go:228`
- Create: `docs/modules/disk.md`
- Modify: `README.md`, `docs/getting-started.md`, `docs/module-management.md`, `docs/development.md` (bundled module lists)

- [ ] **Step 1:** test expects `... command@1.2.0 disk@1.2.0 --modules-dir ...` → FAIL; change `install.sh` → PASS.
- [ ] **Step 2:** docs page following `docs/modules/file.md` (params, fields, behaviour, recommended example with `used_percent < 90`, `free > 5GB`, `inodes_used_percent < 90`, several disks in `conf.d/disks.yaml`).
- [ ] **Step 3:** `make check` → pass; commit `chg: install the disk module with install.sh and document it`.
