# ports module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bundled module `ports` that lists listening TCP/UDP sockets of the host.

**Architecture:** Pure parsing of `/proc/net/*` under a replaceable root, interface addresses through a replaceable function, process names by scanning `/proc/<pid>/fd`.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-11-24-ports-module-design.md`

## Global Constraints

- No new dependencies, no external commands, no root required.
- `make check` passes; gocyclo ≤ 25.
- Tests only under `t.TempDir()`, no network.
- Commits `chg:`/`fix:`/`docs:`, no attribution trailers, no merge commits.

## Review Focus

1. `/proc/net/tcp` unreadable — error, never an empty list.
2. Address decoding: `0100007F` is 127.0.0.1, IPv6 words are per-word native order, `::ffff:a.b.c.d` becomes IPv4.
3. Connected UDP sockets and non-LISTEN TCP sockets are not reported.
4. Unknown `interface` — error listing the interfaces.
5. Malformed line in a proc file — error naming file and line, not a skipped socket.

---

### Task 1: module

**Files:** Create `cmd/onecheck-ports/{main.go,collect.go,proc.go}`, tests `collect_test.go`, `proc_test.go`.

**Interfaces:**
- `type socket struct{ proto string; addr netip.Addr; port uint16; inode uint64 }`
- `parseSockets(r io.Reader, proto string, v6 bool) ([]socket, error)`
- `processNames(root string, inodes map[uint64]bool) map[uint64]string`
- package vars `procRoot = "/proc"`, `interfaceAddrs = func() (map[string][]netip.Addr, error)`

- [ ] Tests: parsing (listen vs established, udp connected, v4/v6/mapped decoding, malformed line); collect (default drops loopback, `interface`, `proto`, dedup, sort, count-only skips process scan, process names from a fake `/proc`, missing tcp → error, missing tcp6 ok, unknown interface → error, bad params, describe).
- [ ] Run `go test ./cmd/onecheck-ports` — FAIL; implement; PASS.
- [ ] Commit `chg: add the ports module that reports listening sockets`.

### Task 2: install and docs

**Files:** `install.sh`, `internal/installsh/install_test.go`, `docs/modules/ports.md`, README.md, docs/getting-started.md, docs/module-management.md, docs/development.md, AGENTS.md.

- [ ] Test expectation `... docker@1.2.0 ports@1.2.0 --modules-dir` → FAIL; add `ports` to `MODULES` → PASS.
- [ ] `docs/modules/ports.md`: params, fields, recipes (no unknown ports, required port, one interface), limits (firewall, UDP noise, root for process names).
- [ ] `make check`, `make release-assets`; e2e on this host.
- [ ] Commit `chg: install the ports module with install.sh and document it`.
