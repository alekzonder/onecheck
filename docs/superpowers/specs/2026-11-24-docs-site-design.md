# Documentation site — design

## Intent

Publish the user documentation at <https://alekzonder.github.io/onecheck>,
built with Blume, rebuilt automatically whenever a release tag `vX.Y.Z` is
pushed. The site shows the documentation of the latest release. The setup
follows `~/github/tariboy` (Blume project in `docs/`, `make docs` →
`scripts/publish-docs.sh` → branch `docs` → GitHub Pages).

## Goals

- `docs/*.md` and `docs/modules/*.md` stay where they are and become the site.
- Internal material (`docs/superpowers/`) is never published.
- Publishing is part of the release workflow; nobody runs it by hand.
- The site is the place users read the docs: `README.md` and CLI messages link
  to it.

## Non-goals

- Versioned docs (one site per release). Only the latest release is shown.
- Redesigning or rewriting page content, a custom landing design.
- Moving pages into `docs/docs/` like tariboy.

## Approaches

- **A. Blume project in `docs/`, publish via branch `docs` (chosen).** Same as
  tariboy; `make docs` can also publish from a laptop. Costs a branch holding
  the built site.
- B. Same project, publish with `actions/deploy-pages`. No extra branch, but
  differs from tariboy and cannot be run locally. Rejected.
- Blume project at the repository root. Rejected: `node_modules` inside the Go
  module interferes with `go ./...`, and a root `package.json` is out of place.

## Design

**Blume project** in `docs/`: `package.json` (`blume ^1.0.4`, scripts `dev`,
`build`, `doctor`), `package-lock.json`, `blume.config.ts`, `.gitignore`
(`node_modules/`, `dist/`, `.blume/`), `public/favicon.svg`.

`blume.config.ts`:

- `title: "onecheck"`, a one-line description,
- `deployment: { base: "/onecheck", site: "https://alekzonder.github.io" }`,
- `github: { owner: "alekzonder", repo: "onecheck", dir: "docs" }`,
- `content: { root: ".", exclude: ["superpowers/**", "node_modules/**",
  "dist/**", "**/_*", "**/.*"] }`.

**Pages.** A new `docs/index.md` is the home page: what onecheck is, a short
example, links to the other pages (content adapted from the README). `meta.ts`
files order the sidebar: getting started, configuration, checks, modules,
state and events, output and results, module management, logging and
logrotate, module protocol, development.

**Links.** Blume does not rewrite relative `.md` links (verified: they stay
`href="checks.md"` and break). Links between documentation pages are written
as site routes, root-relative without the base: `[checks](/checks#gt-gte-lt-lte)`,
`[disk](/modules/disk)`. Blume prepends `/onecheck` (verified). Heading anchors
match GitHub's slugs (verified), so existing anchors keep working. Trade-off
accepted: these links do not work when browsing `docs/` on GitHub.

`README.md` links point directly to `https://alekzonder.github.io/onecheck/...`.
The `self-update` message in `internal/cli/selfupdate.go` points to the site's
getting-started page.

**Link guard.** `scripts/docs-check-links.sh` fails when a page under `docs/`
(excluding `superpowers/`, `node_modules/`, `dist/`) contains a relative link
to a `.md` file. It runs as part of the docs build, so a regression fails CI.

**Make targets.**

- `make docs-build`: `npm ci` if `docs/node_modules` is missing, the link guard,
  `blume doctor`, `blume build`.
- `make docs`: `scripts/publish-docs.sh`, ported from tariboy: runs the docs
  build, copies `docs/dist` into a worktree of branch `docs` (created as an
  orphan the first time), adds `.nojekyll`, commits `docs: publish <sha>`,
  pushes.

`make check` stays Go-only.

**CI.**

- `check.yml`: a separate `docs` job (Node 22, npm cache on
  `docs/package-lock.json`) runs `make docs-build` on pushes to main and PRs.
- `release.yml`: a `docs` job with `needs: release`, `permissions: contents:
  write`, checks out the tagged commit and runs `make docs`.

**GitHub Pages** is enabled once, source branch `docs`, path `/`, after the
branch exists. First publication: run `make docs` from main after this change
merges, then enable Pages; later releases publish automatically.

**Repository docs.** `docs/development.md` gains a "Documentation site"
section (layout, link rule, `make docs-build`, publishing). `AGENTS.md`: the
repository map row for `docs/` mentions the Blume site, plus a rule that links
between documentation pages use site routes (`/checks`), never `*.md`.

## Left to the builder

- Wording of `docs/index.md` and the site description.
- Whether page titles containing backticks (e.g. ``Module `disk` ``) need a
  frontmatter `title` for a clean sidebar and `<title>`.
- Favicon artwork (a simple SVG is enough).
