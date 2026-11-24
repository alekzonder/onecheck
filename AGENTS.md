# AGENTS.md

Rules for anyone (human or AI agent) changing this repository.

## Commits and merges

- Never add `Co-Authored-By`, `Co-Author`, `Signed-off-by: <AI>` or any similar
  attribution trailer to commit messages.
- Never create merge commits. Integrate branches with rebase only
  ("Rebase and merge" on GitHub, `git rebase` locally). History stays linear.
- Commit message style: `chg: <what changed>` / `fix: <what was fixed>` /
  `docs: <what was documented>`, lower case, imperative.

## Before you finish

- Run `make check`. It must pass: gofmt, `go vet`, `golangci-lint`, tests with
  the race detector.
- Cyclomatic complexity is limited to 25 (`gocyclo` in `.golangci.yml`).
  Do not raise the limit and do not add `//nolint:gocyclo`; split the function.
- New behaviour comes with a test. Tests must not use the network or write
  outside `t.TempDir()`.
- User-visible changes (config keys, operators, module fields, CLI flags,
  result/event JSON) must be reflected in `docs/`.
- `docs/` is published as a site (Blume). Pages link to each other by site
  route, `[Checks](/checks#anchor)`, never by `.md` path; `make docs-build`
  must pass when `docs/` changes. See "Documentation site" in
  `docs/development.md`.

## Repository map

| Path | What lives there |
|---|---|
| `cmd/onecheck` | core CLI entrypoint |
| `cmd/onecheck-<name>` | bundled check modules (`file`, `http`, `command`, `disk`, `systemd`, `cpu`, `memory`, `docker`, `ports`) |
| `pkg/module` | public SDK and protocol types for module authors |
| `internal/match` | the single matching engine (all operators) |
| `internal/config` | config discovery, loading, includes |
| `internal/modules` | module discovery and process execution |
| `internal/state`, `internal/store` | pass/fail transitions, state/result/event files |
| `internal/runner`, `internal/output` | running rules, rendering results |
| `internal/rotate` | `onecheck logrotate` |
| `internal/github`, `internal/install` | module install/update, self-update |
| `internal/cli` | cobra commands |
| `install.sh` | installer script (POSIX sh); tested by `internal/installsh` |
| `docs/` | user documentation (Markdown), the Blume site published to https://alekzonder.github.io/onecheck |
| `scripts/` | documentation link check and publishing |

## Design rules that must hold

- Modules only collect field values. All comparison logic lives in
  `internal/match`; never add matching logic to a module.
- The module protocol is versioned (`pkg/module.ProtocolVersion`). A breaking
  change to the JSON exchanged with modules requires a new protocol version.
- Result, state and event files are written atomically (temp file + rename).
- A state transition must never be lost: write the event before the state.

See `docs/development.md` for build, test and release details.
