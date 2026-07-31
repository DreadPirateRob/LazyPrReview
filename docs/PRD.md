# lazypr — Product Requirements Document

> **One-liner:** A simple terminal UI for GitHub pull request review — the reviewer-side complement to lazygit.

## 1. Problem & motivation

Reviewing pull requests on github.com is slow and mousy. The interface imposes five compounding frictions on every review session:

1. **Context switching.** The diff, the thread list, the CI status, and the merge button live on different tabs and page sections. Following a thread to its file context means scrolling through a long unified page, often losing your place.
2. **Collapsed large diffs.** GitHub collapses files beyond a threshold and hides full-file context behind extra clicks. Reviewers of large refactors spend a disproportionate amount of time just uncollapsing content.
3. **Scattered thread tracking.** Unresolved threads are not presented as a navigable, ordered queue. Deciding which threads still need replies requires scrolling the entire PR or switching to a summary view that omits diff context.
4. **Fragile pending reviews.** A batch review in progress is stored in browser local state. Switching tabs, reloading the page, or starting a review in one browser session and resuming in another silently discards draft comments. There is no recovery path.
5. **CI buried in tabs.** Check run details require leaving the review context entirely, opening a separate tab, and navigating through nested job views. Correlating a failing check with the diff line that caused it requires manual cross-referencing.

**lazypr** collapses the whole reviewer loop — triage → diff → comment → approve/request-changes → merge — into one keyboard-driven TUI with lazygit's spatial model. Every pane is reachable by number; every action is a key; nothing requires a pointing device. The server-side GitHub pending review is the durable primitive: draft comments are always visible, always navigable, and never lost to a page reload.

## 2. Target users

### Primary — IC reviewer

An individual contributor who reviews teammates' pull requests daily as a core part of their workflow. They typically handle 3–10 PRs per day, spend meaningful time reading diffs and participating in threads, and care about the per-PR detail: understanding what changed, why, and whether the implementation is correct. Their pain is the repetitive friction of the GitHub web UI — the mouse, the page reloads, the lost drafts. lazypr's Phase 1 and Phase 2 scope is built for this persona end to end.

### Secondary — maintainer triaging many repos

An open-source maintainer or platform engineer who monitors PRs across multiple repositories, often without deep-diving each one immediately. Their primary need is triage: seeing what needs attention across all repos, routing, and quick approvals. This persona is served fully by Phase 3's cross-repo inbox (`gh search prs --review-requested=@me` when launched outside a repo), which surfaces all review-requested PRs across every repo in a single list.

## 3. Landscape & gap

The table below scores existing tools on the capabilities that define a complete PR review workflow. `✓` = fully supported, `~` = partial, `✗` = absent.

| Capability | gh CLI | gh-dash | octo.nvim | prr | Web UI |
|---|---|---|---|---|---|
| List / triage | ✓ | ✓ | ✓ | ✗ | ✓ |
| Diff view | ✓ | ✗ | ✓ | ✓ | ✓ |
| Inline comment display | ✗ | ✗ | ✓ | ✗ | ✓ |
| Inline authoring | ✗ | ✗ | ✓ | ✓ | ✓ |
| Batch review (server-side pending) | ✗ | ✗ | ✓ | ~ | ✓ |
| Thread resolve | ✗ | ✗ | ✓ | ✗ | ✓ |
| Viewed tracking | ✗ | ✗ | ~ | ✗ | ✓ |
| CI status | ~ | ~ | ~ | ✗ | ✓ |
| Merge | ✓ | ~ | ✓ | ✗ | ✓ |
| Standalone TUI | ✗ | ✓ | ✗ | ✗ | ✗ |

**gh CLI (`gh pr`):** Body-level review only — `gh pr review` posts a top-level review body with no inline comment support, no thread navigation, and no TUI; it is a scriptable helper, not a review client.

**gh-dash:** A dashboard launcher that surfaces PR lists and status glyphs; it opens the web UI for anything beyond triage and has no diff view, inline comment display, or review authoring of its own.

**octo.nvim:** A full review workflow inside Neovim — inline comments, batch review, thread resolve — but it is a Neovim plugin and therefore unavailable outside that editor; not a standalone TUI.

**prr:** An offline file-annotation workflow that applies diff-style comments as local edits; it cannot read or reply to existing review threads, making it unsuitable for iterative review conversations.

**Web UI:** Complete functionality but entirely mouse-driven; large diffs collapse behind clicks, the pending review is fragile browser-local state, and CI details are buried across separate tabs.

**The gap:** No existing tool ships a standalone TUI combining diff rendering, inline thread overlays, navigable unresolved-thread queue, server-side batched pending review, resolve/unresolve, CI panel, and merge — without requiring a specific editor or the browser.

## 4. Product pillars

### Pillar 1 — Spatial UI you never re-learn

Five numbered side panels and one main pane mirror lazygit's fixed spatial model. `1`–`5` jump to any panel; `0` focuses the main view. The layout never reflows unexpectedly: muscle memory built on day one holds on day one hundred. An accordion effect gives the focused panel proportionally more height without requiring manual resizing.

### Pillar 2 — Batch-first reviewing

The server-side GitHub PENDING review is the core object, not a local file or browser tab. Draft comments are always visible as cyan overlays at their diff anchors, always listed in the Drafts tab, and always editable and deletable before submission — even across restarts, because they live on GitHub's servers. If a pending review was started elsewhere (e.g. in the web UI), lazypr adopts the most recently created one on load: the Status panel shows the banner `PENDING review: N draft comments (S to submit)`, drafts render in the diff, and `S` submits or discards it. The "lost draft review" failure mode is eliminated by design.

### Pillar 3 — Entire loop without leaving the terminal

From first `lazypr` invocation to approved-and-merged PR, every step is available at the keyboard: triage the list, open the diff, read threads, post inline comments, suggest changes, resolve threads, check CI, approve or request changes, and merge — all without switching applications or opening a browser.

### Pillar 4 — Transparency

Every `gh` invocation is streamed to a command log at the bottom of the screen showing the exact command, duration, and exit code. Any error surfaces the literal `gh` command that failed alongside the stderr output; `ctrl+o` copies it. There is no magic: lazypr is a keyboard-driven shell around `gh`, and it never hides that.

### Pillar 5 — Zero-config on top of `gh`

lazypr borrows authentication from `gh auth login` — there is no token management, no credential store, and no separate login flow. A working `gh` installation is the only hard dependency. Sensible defaults cover the common case; a single YAML file handles the rest.

## 5. Feature requirements by phase

> **Scope boundaries (all phases):** No AI-assisted review features. No PR authoring features (create/edit PR, push code) beyond checkout handoff. GitHub-only v1; GitLab and Bitbucket are excluded behind the forge abstraction seam.

### Performance budgets

| Metric | Budget |
|---|---|
| startup → first paint | < 300ms (probes async) |
| PR list load, 50 PRs | < 1.5s to interactive |
| PR open → diff interactive, 100-file PR | < 2s |
| optimistic toggle paint (viewed/resolve) | < 100ms |
| UI update after mutation API return | < 300ms |
| input latency during any fetch | never blocked (all I/O async) |

### Phase 1 — Read + triage (MVP)

| Feature | Acceptance check |
|---|---|
| Startup probes & error screens | Given `gh` is not installed, launching `lazypr` → the fatal full-screen install-instructions screen appears within < 300ms. |
| Repo resolve + `--repo` + PR-number/URL args | Given a valid git repo in cwd, running `lazypr` → repo is resolved and PR list is interactive within < 1.5s to interactive; `lazypr --repo owner/name` and `lazypr <url>` resolve without a git repo present. |
| PR list with 3 tabs, filter | Given a repo with 50 open PRs, the list is interactive within < 1.5s to interactive; switching tabs switches filter sets; `/` filters the visible list in place. |
| Mega-query load | Given any PR selected from the list, pressing `enter` → PR detail loads and diff is interactive within < 2s. |
| Files panel (tree/flat, colors, accordion) with viewed toggle (single + range) | Given an open PR, pressing `space` on a file → viewedState toggles to `VIEWED` and the row repaints within < 100ms; a forced API failure rolls back the state visibly with a toast. |
| Own diff renderer with syntax highlight + thread overlays (read + fold via `z`) | Given a PR with inline review threads, opening any file in the diff → thread overlays render at their anchored lines with syntax-highlighted code; pressing `z` on a thread overlay folds/unfolds it. |
| `t`/`T` unresolved-thread navigation | Given a PR with unresolved threads across multiple files, pressing `t` repeatedly → each anchored unresolved thread is visited in file-then-anchor order, wrapping back to the first after the last. |
| Threads panel (Unresolved/All) with jump+focus | Given a PR with an unresolved thread listed in panel 4, pressing `enter` → Main jumps to the thread's anchor line and the thread context is focused. |
| Checks panel + v1 Timeline | Given a PR with CI checks and issue comments, opening panel 5 → check states match those shown on github.com for the same PR; the Timeline tab shows issue comments and submitted reviews in chronological order. |
| Status panel incl. reviewDecision + requested reviewers + rate limit | Given an open PR with a pending review containing 4 draft comments, panel 1 → shows `PENDING review: 4 draft comments (S to submit)`, the correct `reviewDecision`, and all requested reviewers. |
| Open-in-browser + copy menus | Given any focused PR, file, or thread context, pressing `o` → the OS browser opens the context-appropriate URL; pressing `y` → the copy menu offers PR URL, branch name, head SHA, file path, or thread permalink as applicable. |
| `?` overlay + hint bar per tab | Given any panel and tab combination, pressing `?` → a searchable overlay lists every active binding for that context; the hint bar at the bottom reflects the active tab's bindings at all times. |
| Command log | Given any `gh` invocation triggered by a lazypr action, the command log → shows the exact command, duration, and exit code; pressing `@` toggles the log panel. |
| Config load + keybinding remap | Given a `config.yml` with a remapped action key, launching `lazypr` → the remapped key performs its target action and `?` reflects the updated binding; `<disabled>` suppresses an action. |
| Screen modes + portrait | Given a terminal narrower than 90 columns, launching `lazypr` → portrait mode activates with panels stacked automatically; `+`/`_` cycle normal/half/fullscreen main pane at any terminal width. |

**Phase 1 Definition of Done (all must hold):**
1. `gh` missing, unauthed, and no-repo cases each land on the correct fatal screen with the named fix.
2. On a 50-PR repo, list is interactive within budget; tabs switch filter sets; `/` filters.
3. Opening a 100-file PR renders the diff within budget; every unresolved thread is anchored exactly per D7 (RIGHT and LEFT cases; outdated threads appear only in panel 4 with badge).
4. `t`/`T` visits every anchored unresolved thread across files in anchor order and wraps.
5. `space` viewed-toggle round-trips (refetch shows `VIEWED`), paints within budget, and rolls back visibly on a forced API failure.
6. Checks panel states match the web UI for the same PR; Timeline shows comments + reviews in chronological order.
7. Every active binding appears in `?` and the hint bar; every `gh` call appears in the command log with duration.

### Phase 2 — Authoring

> Note: gate testing of write features happens against a dedicated throwaway repo owned by the developer, never third-party repos.

| Feature | Acceptance check |
|---|---|
| Pending-review lifecycle (ensure/adopt, inline + multi-line comments, suggestions, edit/delete drafts, submit menu, discard) | Given no existing pending review, pressing `c` on a diff line and submitting a comment → a PENDING review is ensured server-side and the draft overlay appears at the anchored line within < 300ms after API return; `S` opens the submit menu showing the draft count. |
| Drafts tab + draft overlays + first-draft toast | Given a first-ever draft comment being submitted, → a one-time toast reads `Draft saved to your pending review — nothing is public until you press S`; the Drafts tab in panel 4 lists the draft with jump navigation. |
| Reply (immediate) | Given a focused thread with `r` pressed and a reply body submitted, → the reply appears in the correct thread on github.com within < 300ms after API return. |
| Resolve/unresolve | Given an unresolved thread in Main or panel 4, pressing `space` → the thread collapses and the optimistic toggle repaints within < 100ms; a forced API failure rolls back with a visible toast. |
| Issue comments | Given the Timeline tab focused, pressing `a` and submitting a body → the issue comment appears on the PR's github.com timeline within < 300ms after API return. |
| File-level comments | Given a file selected in the Files panel, pressing `c` → a file-level comment popup opens; after submission the draft overlay appears associated with that file. |
| Context expansion `{`/`}` | Given a diff hunk in Main, pressing `{` → additional context lines appear around the hunk without displacing or breaking existing thread anchors. |
| Whitespace toggle | Given a diff containing whitespace-only changes, pressing `ctrl+w` → whitespace changes are toggled on/off in the rendered diff. |
| Checks watch mode | Given the Checks panel focused and a CI run in progress, pressing `w` → check states poll and update every 15 seconds while the panel remains focused. |
| `ctrl+e` opens comment draft in `$EDITOR` | Given an in-progress draft textarea, pressing `ctrl+e` → the draft body opens in the configured `$EDITOR`; saving and exiting returns the body to the textarea. |

**Phase 2 Definition of Done (all must hold):**
1. A full review — one single-line comment, one multi-line, one suggestion, then Request changes with summary — arrives on github.com as ONE atomic review.
2. A pending review started in the web UI is adopted (banner + drafts visible) and submittable/discardable from lazypr.
3. Drafts survive restart (server-side), are listed in the Drafts tab, and `e`/`d` on them behaves per the D6 matrix.
4. Resolve/unresolve round-trips and the optimistic paint rolls back on forced failure.
5. Reply lands in the correct thread on github.com.
6. Submit menu blocks empty body for Request changes / Comment with an inline validation message.

### Phase 3 — Power loop

> Note: gate testing of write features happens against a dedicated throwaway repo owned by the developer, never third-party repos.

| Feature | Acceptance check |
|---|---|
| Merge menu (+auto, delete-branch) | Given an open PR with passing CI, pressing `M` and choosing squash with `--auto` → `gh pr merge` is invoked with `--squash --auto`; ineligible strategies (e.g. draft, failing checks) are rendered dim with the reason shown. |
| Checkout handoff | Given a PR selected and cwd matching the PR's repo, pressing `C` → `gh pr checkout <n>` checks out the branch and returns input latency never blocked; from a non-repo cwd → an error popup names the precondition. |
| `e` open-in-$EDITOR at line | Given a diff line focused and `$EDITOR` configured, pressing `e` → the editor opens at the exact file and line; if the branch is not checked out → a warning popup appears. |
| Commits tab (diff scoped per commit) | Given a PR with multiple commits, switching to the Commits tab in panel 3 and selecting a commit → Main scopes the diff to that commit's changes only. |
| Custom commands | Given a custom command defined in `config.yml`, triggering it from its configured context → the templated shell command executes and the invocation appears in the command log. |
| GHE hostname support | Given a GitHub Enterprise instance configured, running `lazypr --repo owner/name` against it → list, diff, and review flows operate against the GHE host. |
| Cross-repo inbox | Given a cwd that is not a git repo, running `lazypr` → a cross-repo inbox lists PRs with `review-requested:@me` across all repos, interactive within < 1.5s to interactive. |
| Re-run failed checks (stretch) | Given a PR with a failed check run, triggering re-run → `gh run rerun` is invoked with the correct run ID and the check state updates in the Checks panel. |

**Phase 3 Definition of Done (all must hold):**
1. Merge menu executes the chosen strategy with `--auto`/`--delete-branch` honored; ineligible options are dimmed with the reason.
2. Checkout precondition matrix: non-repo cwd → error popup; mismatched repo → error popup; match → branch checked out.
3. `e` opens the configured editor at the exact file:line.
4. `{`/`}` expands context without breaking existing thread anchors.
5. Commits tab scopes the diff to the selected commit.

## 6. UX principles

lazypr carries lazygit's interaction DNA directly into the PR-review domain. Every principle below is a concrete rule, not a preference.

**Numbered spatial panels.** Five side panels and one main pane have fixed positions and permanent number keys (`1`–`5`, `0`). The layout never reflows. A reviewer who learns the spatial model on day one retains it without re-learning.

**Context verbs + `?`.** Every active key is discoverable: `?` opens a searchable keybinding overlay scoped to the current context, and the hint bar at the bottom of the screen always reflects the active tab's bindings. No action is hidden.

**Drill-down with `enter`/`esc` — a real focus stack, not a hardcoded chain.** `enter` drills into a thing (opens a PR, focuses a file diff, focuses a thread); `esc` pops back to exactly the context you drilled in from. The path Files → Main diff → Thread-focused → Main cursor → Files is fully reversible by pressing `esc` at each step. `esc` at the top level (PR list, nothing open) is a no-op; quitting requires `q`.

**Menus for variants; confirms for danger.** Actions with multiple outcomes open a menu (`S` for submit-review variants: Approve / Request changes / Comment / Discard; `M` for merge strategies; `y` for copy targets). Destructive or irreversible actions — discard pending review, delete a published comment — require an explicit confirm step. There is no accidental data loss.

**One meaning per key per context.**
`t` / `T` in Main = jump to next / previous **unresolved** thread, ordered by file order then anchor position, wrapping across files. `t` in the Files panel = open diff at the first unresolved thread of the selected file.

A thread is either *rendered inline* (Main), *listed* (panel 4), or **focused** (its own context).

- `space` on a listed or focused thread = resolve/unresolve toggle (P2, optimistic).
- `space` anywhere in Main outside a focused thread = toggle **viewed** state of the current file (the one invariant meaning of `space` in Main).
- `z` in Main on a thread overlay = fold/unfold it (cosmetic only). Unresolved threads default expanded; resolved/outdated default collapsed to one summary line.
- `enter` from panel 4 = jump Main to the anchor AND focus the thread. `enter` in Main with cursor on a thread overlay = focus it. `enter` on any other Main line = no-op.
- `esc` returns to the Main cursor at the anchor.

`[`/`]` mean prev/next tab everywhere except Main, where they mean prev/next file — this rebind is documented in `?` and the hint bar reflects it. No key silently does different things in different contexts without being declared.

**Transparency via command log.** Every `gh` invocation — including the exact command, its arguments, duration, and exit code — is streamed to the command log at the bottom right. Errors surface the literal failing command with stderr; `ctrl+o` copies it to the clipboard. The operator always knows exactly what the tool did on their behalf.

## 7. Success metrics

The following outcomes, measurable against a real PR and a real reviewer, constitute success for lazypr at the end of Phase 2:

1. **Time from launch to first review action under 5 seconds.** A reviewer who runs `lazypr` cold in a repo directory — from blank terminal to the first `space` or `enter` — completes that first interaction in under 5 seconds. This includes PR list load and PR open.

2. **Full review of a 20-file PR without opening a browser.** A reviewer reads the full diff, navigates all unresolved threads, posts inline comments (including at least one suggestion), and submits a Request-changes review entirely inside lazypr. The browser is never opened during the session.

3. **Zero lost draft comments (adoption rule).** A pending review started in the web UI — or in a previous lazypr session — is never silently discarded. lazypr adopts the existing pending review on load, surfaces it in the Status panel, and makes its drafts navigable and submittable. Drafts survive restart because they live server-side.

4. **Keyboard-only coverage of 100% of Phase 1 and Phase 2 flows.** Every action in Phase 1 (read, triage, diff, thread navigation, viewed toggle, CI view, copy, open-in-browser) and Phase 2 (comment, suggest, edit/delete draft, reply, resolve, submit review) is reachable with a keyboard sequence starting from any lazypr context. No flow requires a mouse or a browser.

## 8. Open questions

1. **GHE priority ranking.** GitHub Enterprise (GHE) hostname support is scoped to Phase 3, but the user base of GHE customers may justify pulling it into Phase 2. What is the relative priority of GHE support versus the Phase 2 authoring features?

2. **Cross-repo inbox priority.** The cross-repo inbox (Phase 3, `gh search prs --review-requested=@me` when outside a git repo) is the primary feature for the maintainer persona. Should it be pulled forward to Phase 2 alongside authoring, or deferred as currently planned?

3. **Distribution channels beyond brew and releases.** The planned distribution is a goreleaser binary matrix plus a brew tap post-v1. Are there additional channels — nixpkgs, AUR, Scoop, asdf/mise plugin — that should be part of the initial release plan, or addressed post-v1?
