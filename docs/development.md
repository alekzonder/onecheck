---
title: Development
---

Rules for contributors (commit messages, merges, limits) are in
[AGENTS.md](https://github.com/alekzonder/onecheck/blob/main/AGENTS.md). This page is about how to build, test and release.

## Requirements

- Go (the version in `go.mod` or newer)
- `make`
- network access the first time `make lint` runs: it installs a pinned
  `golangci-lint` into `./bin`
- for the documentation site only: Node.js 22 and npm

## Make targets

| Target | What it does |
|---|---|
| `make build` | builds `dist/onecheck` and `dist/modules/onecheck-{file,http,command,disk,systemd,cpu,memory,docker,ports}` |
| `make test` | `go test -race ./...` |
| `make fmt` | formats the code with `gofmt` |
| `make lint` | `golangci-lint`, including cyclomatic complexity |
| `make check` | `fmt-check` + `go vet` + `lint` + `test`; **must pass before a change is done** |
| `make release-assets` | cross-builds release assets and `checksums.txt` into `dist/release/` |
| `make docs-build` | checks documentation links and builds the site into `docs/dist/` |
| `make docs` | builds the site and publishes it to branch `docs` |
| `make clean` | removes `dist/` |

Builds are static (`CGO_ENABLED=0`). The version is injected with
`make build VERSION=1.2.3`; without it the version is `dev`.

A new bundled module is picked up by the Makefile automatically: every
directory `cmd/onecheck-<name>` is built as a module.

## Cyclomatic complexity

`gocyclo` runs as part of `make lint` with a limit of **25** per function
(`.golangci.yml`). It applies to tests too. When a function goes over the
limit, split it; do not raise the limit and do not silence the linter.

## Layout

```
cmd/onecheck/              core CLI entrypoint
cmd/onecheck-file/         bundled modules: main.go (description),
cmd/onecheck-http/           collect.go (data collection)
cmd/onecheck-command/
pkg/module/                public SDK for module authors: protocol types, Main
internal/match/            the matching engine: compile + evaluate "check"
internal/config/           find, load, include, ${VAR} expansion, defaults
internal/modules/          find modules, describe, run a module process
internal/state/            pass/fail state machine (pure logic)
internal/store/            atomic files, run lock, state/result/event files
internal/result/           Result and Event JSON contracts
internal/runner/           validate rules, execute, record
internal/output/           text and JSON rendering, exit code
internal/logging/          slog setup
internal/rotate/           logrotate: logs and expired events
internal/github/           GitHub Releases client
internal/install/          module install/update/remove, self-update
internal/cli/              cobra commands
internal/testmods/         builds bundled modules for tests
examples/                  example config, validated by a test
docs/                      user documentation, built into the site by Blume
scripts/                   documentation link check and publishing
```

How a run flows through the packages:

```
cli.run
 └─ config.Find / config.Load
 └─ runner.Run
     ├─ store.Open, Lock
     ├─ per rule: prepare        modules.Describe, params schema, match.Compile
     ├─ per rule: modules.Check  starts onecheck-<module>, reads field values
     │            match.Eval     conditions → failures
     └─ per rule: state.Advance → store.WriteEvent → SaveState → WriteResult
 └─ output.Write, output.ExitCode
```

## Design rules

- **Modules collect, the core decides.** Never put comparison logic into a
  module. A new way of comparing is a new operator in `internal/match`,
  where it immediately works for all modules.
- **One failing rule never stops the others.** Anything wrong with a single
  rule becomes that rule's `error` result. Only problems of the whole config
  or of onecheck's own files end a run with exit code `2`.
- **No lost transitions.** For each rule the event is written before the
  state. Keep that order.
- **Atomic files.** Everything under results, state and events is written
  with `store.WriteFileAtomic`.
- **The protocol is a contract.** Changing the JSON exchanged with modules
  incompatibly requires a new `module.ProtocolVersion` and a range in
  `internal/modules` that still accepts the old one.

## Tests

- Unit tests sit next to the code. The matching engine and the state machine
  are table-driven.
- Tests that need real modules build them once per package through
  `internal/testmods` (`runner`, `cli`). `internal/modules` uses a fake
  module from `testdata/fakemod` that can misbehave on purpose.
- Module management is tested against an in-memory GitHub client
  (`internal/install`) and against a local HTTP server speaking the GitHub
  API shape (`internal/github`, `internal/cli`).
- `examples/config.yaml` (with its `conf.d/` includes) is loaded and
  validated against all bundled modules by `TestExamplesLoadAndValidate`, so
  examples cannot drift from the code. The test also fails when a bundled
  module (`cmd/onecheck-<name>`) has no example rule: a new module comes with
  an example.
- Tests must not use the network and must not write outside `t.TempDir()`.

Run one package or one test:

```sh
go test ./internal/match
go test ./internal/runner -run TestRunTransitionWritesEvent -v
```

## Adding an operator

1. Add a table-driven case (passing and failing) to
   `internal/match/match_test.go`, and a case to `TestCompileErrors` for bad
   arguments.
2. Implement it in `internal/match/ops.go` and register it in `leafOps`.
3. Document it in `docs/checks.md`.

## Adding a bundled module

1. Create `cmd/onecheck-<name>/` with `main.go` (description, params schema,
   fields) and `collect.go`, following the existing modules.
2. Test `collect` directly; include `TestDescriptionCoversFields`.
3. Add `docs/modules/<name>.md` (with `title` and `sidebar.label`
   frontmatter like the other module pages), list it in
   `docs/modules/meta.ts` and link it from `README.md` and `docs/index.md`.

## Releasing

Releases are built by GitHub Actions (`.github/workflows/release.yml`) when a
tag `vX.Y.Z` is pushed:

```sh
git tag v1.2.0
git push origin v1.2.0
```

The workflow runs `make check`, then `make release-assets VERSION=1.2.0` and
publishes a GitHub release with:

```
onecheck_<os>_<arch>
onecheck-file_<os>_<arch>
onecheck-http_<os>_<arch>
onecheck-command_<os>_<arch>
onecheck-disk_<os>_<arch>
onecheck-systemd_<os>_<arch>
onecheck-cpu_<os>_<arch>
onecheck-memory_<os>_<arch>
onecheck-docker_<os>_<arch>
onecheck-ports_<os>_<arch>
checksums.txt
```

for linux and darwin on amd64 and arm64. These names are what
`onecheck module install` and `onecheck self-update` look for.

After the release is published, the workflow's `docs` job runs `make docs`,
which publishes the documentation of the tagged commit (see
[Documentation site](#documentation-site)).

Environment overrides useful when testing update flows against a fork:

| Variable | Effect |
|---|---|
| `ONECHECK_GITHUB_API` | API base URL instead of `https://api.github.com` |
| `ONECHECK_SELF_UPDATE_REPO` | repository `self-update` downloads from |
| `ONECHECK_SELF_UPDATE_TARGET` | path of the binary `self-update` replaces |

## Documentation site

The pages under `docs/` are published at <https://alekzonder.github.io/onecheck>
by [Blume](https://useblume.dev). `docs/` is the Blume project and its own
content root:

| Path | What it is |
|---|---|
| `docs/blume.config.ts` | site settings: base `/onecheck`, excluded folders |
| `docs/package.json`, `docs/package-lock.json` | the pinned Blume version |
| `docs/index.md` | the home page |
| `docs/meta.ts`, `docs/modules/meta.ts` | sidebar order |
| `docs/public/` | static files served at the site root |

Every `.md` page under `docs/` is published, except `docs/superpowers/`
(internal designs and plans) and build folders. To keep a new folder off the
site, add it to `content.exclude` in `docs/blume.config.ts`.

**Links between pages use site routes, never `.md` paths:**
`[Checks](/checks#gt-gte-lt-lte)`, `[disk](/modules/disk)`. Blume prefixes
the base `/onecheck` itself; it does not rewrite relative `.md` links, so they
would break on the site. `scripts/docs-check-links.sh` fails the build when one
appears. Links from outside the site (README, CLI messages) use the full URL
`https://alekzonder.github.io/onecheck/<route>`.

**A page's title goes in the front matter, never in a `# ` heading:**

```markdown
---
title: Getting started
---

## Install
```

Blume renders the title above the page itself, so a `# Getting started`
line would show it a second time. Sections start at `##`. The same script
fails the build on a `# ` line outside code blocks.

Preview locally with `cd docs && npm ci && npm run dev`.
`make docs-build` runs the link check, `blume doctor` and `blume build`; CI
runs it on every push and pull request (`.github/workflows/check.yml`).

`make docs` (`scripts/publish-docs.sh`) builds the site and commits
`docs/dist` to branch `docs`, creating it on first use. GitHub Pages serves
branch `docs`, path `/`. The release workflow runs it for every tag, so the
site always shows the latest release.
