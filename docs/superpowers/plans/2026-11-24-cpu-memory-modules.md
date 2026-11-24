# cpu and memory modules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bundled modules `cpu` (usage over a window, load average, cores) and `memory` (RAM and swap) for Linux and macOS.

**Architecture:** Two new `cmd/onecheck-<name>` modules on the `pkg/module` SDK. Platform data comes from gopsutil v4 through package-level function variables that tests replace; field computation is pure functions.

**Tech Stack:** Go 1.26, `github.com/shirou/gopsutil/v4` (v4.26.9), `internal/units`.

**Spec:** `docs/superpowers/specs/2026-11-24-cpu-memory-modules-design.md`

## Global Constraints

- Linux and macOS; `CGO_ENABLED=0` builds for linux/darwin × amd64/arm64 must succeed.
- Modules only collect values; no comparisons in modules.
- Percentages rounded to two decimals; load values two decimals.
- `make check` passes; gocyclo ≤ 25, no `//nolint:gocyclo`.
- Tests: no network, nothing written outside `t.TempDir()`; real gopsutil only in range smoke tests.
- Commits `chg:`/`fix:`/`docs:`, no attribution trailers, no merge commits.

## Review Focus

1. `window` given as a bare number (`2`) or with `ms` — must be accepted as seconds / milliseconds; `0`, `-1s`, `10m`, `abc` rejected with the value named.
2. Rule with an empty field list (no `check`, no saved output) — `cpu` must not wait the window.
3. Counter delta zero or negative between samples — percent fields 0, not NaN/Inf (JSON encoding of NaN would break the module).
4. Host without swap — `swap_used_percent` is 0 and `swap_total` 0, no division by zero.
5. Context cancelled during the window — `cpu` returns an error promptly instead of sleeping on.

---

### Task 1: memory module

**Files:**
- Create: `cmd/onecheck-memory/main.go`, `cmd/onecheck-memory/collect.go`, `cmd/onecheck-memory/collect_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces: package vars `virtualMemory func(context.Context) (*mem.VirtualMemoryStat, error)` and `swapMemory func(context.Context) (*mem.SwapMemoryStat, error)` (defaults `mem.VirtualMemoryWithContext`, `mem.SwapMemoryWithContext`); `memoryFields(total, available, swapTotal, swapUsed, swapFree uint64) map[string]any`; `percent(part, whole uint64) float64` (0 for whole 0, two decimals — same as disk).

- [ ] **Step 1: Write failing tests** in `collect_test.go`:
  - `TestMemoryFields`: `memoryFields(16<<30, 4<<30, 2<<30, 1<<29, 3<<29)` → `total` int64(16<<30), `available` int64(4<<30), `used` int64(12<<30), `used_percent` 75.0, `swap_total` int64(2<<30), `swap_used` int64(1<<29), `swap_free` int64(3<<29), `swap_used_percent` 25.0.
  - `TestMemoryFieldsNoSwap`: swap all 0 → `swap_used_percent` 0.0, `swap_total` int64(0).
  - `TestMemoryAvailableAboveTotal`: available > total → `used` 0, `used_percent` 0.
  - `TestCollectUsesProviders`: replace both vars with fixtures; `collect` returns the fixture-derived fields.
  - `TestCollectMemoryErrorIsError` / `TestCollectSwapErrorIsError`: provider error → error containing it.
  - `TestCollectRejectsParams`: params `{"x":1}` → error.
  - `TestCollectRealMemory`: real providers; `total` > 0, `used_percent` in [0,100].
  - `TestDescribeListsFields`: describe has exactly the 8 fields with types bytes/number as in the spec.
- [ ] **Step 2: Run** `go test ./cmd/onecheck-memory/` — Expected: build failure (undefined).
- [ ] **Step 3: Implement.** `go get github.com/shirou/gopsutil/v4@v4.26.9`. `main.go`: params schema `{"type":"object","additionalProperties":false,"properties":{}}`, description "Report physical memory and swap usage", 8 fields. `collect` decodes params into an empty struct, calls both providers, returns `memoryFields`. Errors wrapped as `read memory: …` / `read swap: …`.
- [ ] **Step 4: Run** `go test ./cmd/onecheck-memory/` — Expected: PASS.
- [ ] **Step 5: Commit** `chg: add the memory module that reports physical memory and swap usage`.

### Task 2: cpu module

**Files:**
- Create: `cmd/onecheck-cpu/main.go`, `cmd/onecheck-cpu/collect.go`, `cmd/onecheck-cpu/fields.go`, `cmd/onecheck-cpu/collect_test.go`, `cmd/onecheck-cpu/fields_test.go`

**Interfaces:**
- Produces: package vars `cpuTimes func(context.Context) (cpu.TimesStat, error)` (default: `cpu.TimesWithContext(ctx, false)`, first element; empty slice → error), `cpuCount func(context.Context) (int, error)` (`cpu.CountsWithContext(ctx, true)`), `loadAvg func(context.Context) (*load.AvgStat, error)` (`load.AvgWithContext`), `sleep func(context.Context, time.Duration) error` (timer + ctx.Done → ctx.Err()), `goos = runtime.GOOS`.
- `usageFields(a, b cpu.TimesStat, linux bool) map[string]any`; `loadFields(avg load.AvgStat, cores int) map[string]any`; `parseWindow(s string) (time.Duration, error)`.

- [ ] **Step 1: Write failing tests.**
  - `fields_test.go`:
    - `TestUsageFieldsLinux`: a = zero, b = {User:30, Nice:10, System:10, Irq:5, Softirq:5, Idle:20, Iowait:10, Steal:10, Guest:5} on linux → total = 100 (guest excluded) → `usage_percent` 70, `user_percent` 40, `system_percent` 20, `iowait_percent` 10, `steal_percent` 10.
    - `TestUsageFieldsDarwin`: same input, linux=false → no `iowait_percent`/`steal_percent` keys; total includes guest (105), busy = 105−20−10 = 75 → `usage_percent` 71.43.
    - `TestUsageFieldsNoDelta`: a == b → every percent field 0.
    - `TestUsageFieldsCounterReset`: b < a → every percent field 0.
    - `TestLoadFields`: avg {1.234, 2.5, 3}, cores 4 → `cores` int64(4), `load1` 1.23, `load5` 2.5, `load15` 3.0, `load1_per_core` 0.31, `load5_per_core` 0.63, `load15_per_core` 0.75.
    - `TestLoadFieldsNoCores`: cores 0 → no `*_per_core`, `cores` absent.
    - `TestParseWindow`: "" → 1s; "500ms" → 500ms; "2" → 2s; "5m" → 5m; "100ms" → ok; errors for "50ms", "5m1s", "0", "abc", "-1s" with the value in the message.
  - `collect_test.go` (fakes for all package vars, restored with `t.Cleanup`):
    - `TestCollectWaitsWindow`: fields `["usage_percent"]`, window "2s" → fake sleep recorded 2s, `cpuTimes` called twice, `usage_percent` present.
    - `TestCollectLoadOnlyDoesNotWait`: fields `["load1"]` → sleep not called, `cpuTimes` not called, `load1` present.
    - `TestCollectEmptyFieldsDoesNotWait`: fields nil → no sleep; load fields and `cores` present, no percent fields.
    - `TestCollectSleepCancelled`: fake sleep returns `context.Canceled` → error.
    - `TestCollectBadWindowIsError`: window "abc" → error, nothing called.
    - `TestCollectProviderErrors`: each of times/count/load failing → error with the provider message.
    - `TestSleepHonoursContext`: real `sleep` with a cancelled context and 1h → returns quickly with `context.Canceled`.
    - `TestCollectRealCPU`: real providers, fields `["usage_percent","load1"]`, window "100ms" → `usage_percent` in [0,100], `cores` ≥ 1.
    - `TestDescribeListsFields`: 12 fields: 5 percent + `cores` + 3 load + 3 per-core, all number.
- [ ] **Step 2: Run** `go test ./cmd/onecheck-cpu/` — Expected: build failure.
- [ ] **Step 3: Implement.** Params `{window string}`; schema: `window` string minLength 1, `additionalProperties:false`. `parseWindow` uses `units.ParseDuration`, range [100ms, 5m]. `collect`: parse window; if any of the five percent fields `WantsOr(f, false)` → times, sleep, times, `usageFields`; always count + load → `loadFields`; merge. Percent: delta parts; total per spec (subtract guest+guestNice on linux); `total <= 0` → zeros; clamp 0..100; round to two decimals.
- [ ] **Step 4: Run** `go test ./cmd/onecheck-cpu/` — Expected: PASS.
- [ ] **Step 5: Commit** `chg: add the cpu module that reports usage over a window, load average and cores`.

### Task 3: install, docs, cross-build

**Files:**
- Modify: `install.sh`, `internal/installsh/install_test.go`, `README.md`, `docs/getting-started.md`, `docs/module-management.md`, `docs/development.md`, `AGENTS.md`
- Create: `docs/modules/cpu.md`, `docs/modules/memory.md`

- [ ] **Step 1: Update the test** expectation to `... disk@1.2.0 systemd@1.2.0 cpu@1.2.0 memory@1.2.0 --modules-dir`; run `go test ./internal/installsh/` — Expected: FAIL.
- [ ] **Step 2:** `install.sh` `MODULES="file http command disk systemd cpu memory"` and help text; rerun — Expected: PASS.
- [ ] **Step 3:** Write both module pages in the style of `docs/modules/disk.md` (params, fields, which fields are present per platform, window vs timeout, load counts I/O wait on Linux, macOS swap is dynamic, containers report the host, recipes from the spec, FAIL output sample); add both modules to every module list.
- [ ] **Step 4: Verify** `make check` and `make release-assets` — Expected: 0 issues, all tests pass, all platform binaries built.
- [ ] **Step 5: Commit** `chg: install the cpu and memory modules with install.sh and document them`.
