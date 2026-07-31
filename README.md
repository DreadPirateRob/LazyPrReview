# lazypr

A terminal UI for GitHub pull request review — the reviewer-side complement to lazygit.

`lazypr` is a keyboard-driven TUI for reading and triaging GitHub pull requests through the `gh` CLI. This repository currently implements the Phase 1 MVP: startup probes, PR list/open flow, diff parsing, thread navigation, checks/timeline, viewed-toggle behavior, help/hint surfaces, command logging, and env-gated read-only smoke coverage.

## Status

Current scope is **Phase 1: Read + triage (MVP)**.

Implemented in this repo:
- `gh` startup/auth/repo probes with fatal screens
- PR open flow from repo, PR number, PR URL, and `owner/repo#number`
- PR list filters/tabs
- GitHub detail loading through `gh api graphql`
- Unified diff parsing and thread anchoring
- Files / Threads / Checks / Main shell views
- Viewed-toggle optimistic update + rollback path
- Help overlay, hint bar, command log, copy menu, browser-open actions
- Config loading + keybinding remap support
- Env-gated live smoke test against a public PR

Also shipped beyond that phase (Phase 2 authoring core):
- inline, multi-line and file-level comments against a server-side pending review
- reply, edit and delete drafts; submit-review menu with discard confirm
- resolve/unresolve from the Threads panel
- markdown-rendered comment bodies, foldable PR overview, author bot/human flags

Still product-roadmapped:
- suggestions (`s`), context expansion `{`/`}`, whitespace toggle, Checks watch mode,
  Timeline issue comments, `ctrl+e` draft in `$EDITOR`
- merge/checkout/editor integration
- cross-repo inbox
- GHE hostnames, custom commands

## Requirements

- Go **1.26.4**
- GitHub CLI `gh` **>= 2.40** on `PATH`
- `gh auth status` must succeed for live usage

## Build

```bash
make          # check + build
```

See [Verification](#verification) for the individual targets and the formatting
standard.

## Run

Open PR list for the current repo:

```bash
./bin/lazypr
```

Open a specific PR by number:

```bash
./bin/lazypr 123
```

Open a PR by URL:

```bash
./bin/lazypr https://github.com/owner/repo/pull/123
```

Open a PR outside a repo with explicit repo override:

```bash
./bin/lazypr --repo owner/name 123
```

Open a PR via shorthand:

```bash
./bin/lazypr owner/repo#123
```

## Flags

- `--repo owner/name` — override repo resolution
- `--debug` — reserved debug flag in the current MVP wiring

## Keyboard model

The UI follows a fixed spatial layout:

- `[1]` Status
- `[2]` Pull Requests
- `[3]` Files
- `[4]` Threads
- `[5]` Checks
- `[0]` Main

Common keys:
- `1`–`5`, `0` — jump focus
- `enter` — open / drill in
- `esc` — pop focus stack
- `?` — help overlay
- `@` — command log menu
- `y` — copy menu
- `o` — open current browser target
- `R` — refresh
- `+` / `_` — screen mode cycle
- `t` / `T` — unresolved thread navigation in Main
- `space` — viewed-toggle in Files/Main

## Config

Config is loaded in this order:
1. built-in defaults
2. global config via XDG/app support path
3. repo-local `.lazypr.yml`

Relevant packages:
- `pkg/config`
- `pkg/ui/keymap`

Notable keys:

- `gui.diffPager` — external diff renderer command, e.g. `"delta --paging=never"`. When set, each file's raw diff is piped through the command and its ANSI output is displayed verbatim (built-in colored renderer with line-number gutter is the default when empty).

## Repository layout

- `cmd/lazypr` — CLI entrypoint
- `pkg/domain` — forge-neutral models
- `pkg/ghcli` — `gh` runner, timeout/error handling, command logging
- `pkg/forge` — forge seam
- `pkg/forge/github` — GitHub implementation via `gh`
- `pkg/forge/fake` — fake forge for tests
- `pkg/diff` — unified diff parsing + thread anchoring
- `pkg/config` — config/defaults/validation
- `pkg/ui` — Bubble Tea shell and panel logic
- `docs/PRD.md` — product requirements
- `docs/SPEC.md` — technical/UX specification

## Verification

`make` wraps the whole loop; `make help` lists every target.

```bash
make check   # fmt-check + vet + unit suite (exactly what CI enforces)
make build   # binary into ./bin
make e2e     # live smoke against a real public PR (needs an authenticated gh)
```

The underlying commands still work directly (`go test ./...`, `go build ./cmd/lazypr`)
if you would rather not go through `make`.

### Formatting standard

`gofmt` plus `goimports` with a local-prefix group. Imports are split into up to
three groups, always in this order: stdlib, third-party, then this module.

```bash
make fmt         # apply it
make fmt-check   # fail on drift, never rewrite
```

`goimports` is pinned in the Makefile (`GOIMPORTS_VERSION`) so the standard cannot
drift between machines and CI. `.editorconfig` carries the same whitespace rules for
editors that do not run `gofmt` on save.

## Notes

- `lazypr` shells out through `gh`; it does not manage tokens itself.
- The command log is intentionally transparent: GitHub operations are surfaced rather than hidden.
- For detailed product intent and future phases, see `docs/PRD.md` and `docs/SPEC.md`.
