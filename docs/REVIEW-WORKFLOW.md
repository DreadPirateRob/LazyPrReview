# Reviewing a PR with lazypr

A walkthrough of the normal loop, from opening a PR to submitting the review.
Every key here is the shipped default — press `?` in the app for the live,
context-scoped list.

**Keys are literal and case-sensitive.** A capital letter means hold shift:
`S` is shift+s, `T` is shift+t, `J`/`K` are shift+j/k. Lowercase keys are
unshifted — `t` and `T` are two different commands, and plain `s` is not bound
to anything.

The one idea that shapes everything: **nothing you write goes public until you
press `S` (shift+s) and submit.** Comments, replies, and edits all accumulate as
drafts on a single pending review, so you compose the whole review before anyone
sees it.

---

## 0. Orientation

Five numbered panels on the left, one Main pane on the right.

| Key | Goes to |
|---|---|
| `1` | Status — repo, review decision, **pending-review banner** |
| `2` | Pull Requests |
| `3` | Files |
| `4` | Threads |
| `5` | Checks |
| `0` | Main (does **not** repaint it — whatever you were reading stays) |
| `esc` | Back: pops focus; from a file diff it returns Main to the PR overview |
| `tab` / `h` `l` | Previous / next panel |
| `?` | Help, scoped to where you are |

`+` / `_` cycle screen modes (normal → half → fullscreen) when you want Main to
take the whole terminal.

---

## 1. Pick a PR

In **`[2]` Pull Requests**, `[` / `]` switch tabs: `Review requested` · `Mine` ·
`All open` · `Search`. The first three fetch server-side (cached for
`github.autoRefreshInterval`, default 60s); `/` filters the loaded list
client-side. On the `Search` tab, `/` takes a raw GitHub query
(`is:merged`, `label:bug`, `author:x`) and `enter` runs it.

`enter` opens the PR → loads detail + diff → **Main shows the PR overview**
(description, reviewers, labels, timeline, threads) and takes focus.

### Reading the overview

The overview is the PR page: a compact header, the description, then the whole
conversation. Comment bodies are **markdown-rendered and wrapped** — bold, code
spans, lists, quotes, and fenced code blocks with syntax highlighting — so you're
not reading raw `**asterisks**`, and nothing is cut off at the right edge.

```
#6574  Cj 10354 notification on cade insufficient bal2 with a longer
       title that wraps
● OPEN   ✓ APPROVED   fazil56   +2873 -89 · 34 files   ✎ 3 draft
CJ-10354-notification-on-cade-insufficient-bal2 → master.jul23old
reviewers DreadPirateRob
labels release-blocker, Frontend

── Description ─────────────────────────────────────────────────
JIRA https://example.test/browse/CJ-10354

── Conversation · 24 (16 human · 8 bot) ────────────────────────

▸ github-actions  5 comments · Apr 07 – Apr 21   latest ✓ Approved

▾ DreadPirateRob  comment · Apr 22 20:30
  ▎ @codex take a look at `notification_service.py`

── Review threads · 3 ──────────────────────────────────────────
```

| Key | Does |
|---|---|
| `j` / `k` | Move (comment rows are navigable) |
| `z` | Fold / unfold the comment or bot run under the cursor |
| `-` / `=` | Collapse / expand everything |
| `b` | Flag the author under the cursor as **bot** or **human** |
| `t` / `T`, `m` / `M` | Jump to unresolved threads / threads mentioning you |

Three things worth knowing:

- **Bot runs collapse.** Consecutive comments from one bot become a single row with
  the count, date span, and latest verdict. On a CI-heavy PR that's the difference
  between 15 rows of repeated `✓ Approved` and one. Expand it with `z` to see the
  individual comments, each of which folds on its own.
- **Bots start collapsed, humans start expanded.** So the human conversation reads
  immediately and the machine noise stays out of the way until you want it.
- **`✎ 3 draft`** in the header means you have unsubmitted comments waiting — press
  `S` to submit them.

#### When something isn't classified right

lazypr ships a default list of bot accounts (`github-actions`, `dependabot`,
`renovate`, …) and treats any `…[bot]` login as a bot. No fixed list keeps up, so
**`b` flags the author under the cursor** and the choice sticks:

- `b` on a human's comment → treated as a bot from now on (collapses, groups)
- `b` on a bot's comment → treated as a human (expands, stops grouping)

It saves to `~/.config/lazypr/authors.yml`, which you can hand-edit:

```yaml
# Managed by lazypr. Safe to hand-edit.
authors:
    chatgpt-codex-connector: bot
    github-actions: human
```

`b` is inert on rows with nobody to flag — the PR header, a section rule, or a
thread's `file.go:12` summary line.

---

## 2. Work through the files

Go to **`[3]` Files**. Moving the cursor *follows* into Main, so you read by
navigating.

| Key | Does |
|---|---|
| `j` / `k` | Move — Main follows the selection |
| `enter` | Open the file and focus Main |
| `space` | Toggle **viewed** |
| `v` | Start a range, then `space` to batch-toggle viewed |
| `` ` `` | Tree ⇄ flat (keeps the selected file across the flip) |
| `-` / `=` | Collapse / expand all directories |
| `t` | Jump to the first unresolved thread in this file |
| `c` | File-level comment (not tied to a line) |
| `/` | Filter the file list |

**Directory rows** are worth knowing about:

- `enter` folds/unfolds them
- landing on one shows **every diff beneath it** in Main, under a
  `Directory: <path>/ — N files` header
- `space` marks that whole subtree viewed
- all three are scoped to the **visible** subtree, so an active `/` filter
  narrows them to match the `✓` and `+N -N` the row displays

A directory view is a concatenation, so it has no per-line anchors — `c`,
`enter`-on-thread and header-`space` are inert there. Move onto a file row to
get them back.

The border title tracks progress: `Files [tree] 3/7 viewed`.

---

## 3. Read a diff

In Main (`0` or `enter` from Files):

| Key | Does |
|---|---|
| `j` / `k` | Line cursor |
| `J` / `K`, `ctrl+d` / `ctrl+u` | Half-page (cursor holds its screen row) |
| `PgDn` / `PgUp` | Full page |
| `<` / `>` | File top / bottom |
| `zz` | Center the cursor line |
| `h` / `l` | Previous / next hunk |
| `[` / `]` | Previous / next file — the Files row follows along |
| `t` / `T` | Next / previous unresolved thread, PR-wide |
| `m` / `M` | Next / previous thread that **@mentions you** |
| `z` | Fold / unfold the thread block under the cursor |
| `space` | Toggle viewed for the file you're reading (on its header row) |
| `enter` | Focus the thread under the cursor |

### Comments appear in the diff

Review threads render **inline, right under the line they're anchored to**:

```
  403 │ + app.post('/create-exchange-account', async (req, res) => {
      ▎ ▾ DreadPirateRob · unresolved
      ▎ DreadPirateRob:
      ▎   lazypr submit smoke — safe to ignore
  404 │ +   const { body } = req;
```

- **Unresolved** threads start expanded; **resolved** and **outdated** ones
  collapse to their summary line. `z` overrides either way.
- Your **unsubmitted drafts** show up here too, marked `[draft]` — they appear the
  moment you write them, so you can re-read your own review in context before `S`.
- A thread that **@mentions you** gets an `@you` badge (visible even collapsed),
  and `@handles` in comment bodies are highlighted — yours most strongly.
- **File-level comments** and **outdated** threads have no line to attach to, so
  they sit in a block just under the `File:` header rather than vanishing.
- A **range** thread anchors at its last line and states its span (`lines 400-406`).
- The cursor walks into these rows: `j` / `k` step onto them, `enter` focuses the
  thread, `z` folds it. `c` is inert on a comment row — there's no code line to
  anchor a new comment to; move back onto code first.

Two places don't show them, by design: an external `gui.diffPager` (its output has
no line mapping) and the directory aggregate view (a concatenation, so it's
read-only for line actions). Open a single file to get the inline view back.

---

## 4. Write comments (they become drafts)

### On a line
Put the Main cursor on a diff line and press `c`. The composer opens as a modal
over the UI, showing **the exact line(s) you're commenting on** above the editor.
`ctrl+s` submits the draft, `esc` cancels.

### On a range
Press `v` to start a selection, `j` / `k` to extend, then `c`. The composer
previews every line in the span.

> One side only. A range must be all added or all context/removed rows —
> GitHub can't anchor a comment across both sides of a diff. Mixing them toasts
> *"Select lines on one side only"*.

### On a whole file
`c` from the **Files** panel — no line anchor.

Each of these lands in the **Drafts** tab of `[4]` Threads, is marked `[draft]`
in the overview, and bumps the Status banner:

```
PENDING review: 3 draft comment(s) (S to submit)
```

Drafts appear instantly (optimistically) and are reconciled against the server in
the background, so a slow network never blocks you or spawns duplicates.

---

## 5. Work the threads

Go to **`[4]` Threads** — tabs `Unresolved` · `All` · `Drafts` · `Commits` via
`[` / `]`.

Moving the cursor **follows into Main**, just like the Files panel: Main shows that
thread's file scrolled to the thread, while focus stays in the panel — so you can
walk the list with `j` / `k` and read each one in place. `enter` when you want to act
on one.

| Key | Does |
|---|---|
| `j` / `k` | Move — Main follows to that thread |
| `enter` | Jump Main to the anchor **and** focus the thread |
| `space` | Resolve / unresolve |

The `Commits` tab deliberately does **not** follow: its `enter` fetches a commit
diff, so following the cursor would fire a network request per keystroke. Following
is also off when `gui.diffPager` is set, for the same reason it's off in Files.

Once a thread is focused (its own context):

**What you should see** when you press `enter`: the Main pane border lights up, the
cursor jumps onto the comment, and the whole comment — author line and every wrapped
body row — is highlighted. A resolved or outdated thread is collapsed by default and
unfolds. If the thread has only one comment, `j` / `k` have nowhere to go and
deliberately do nothing.

`enter` keeps your place: press it on the third comment and the third is focused, not
the first.

| Key | Does |
|---|---|
| `j` / `k` | Move between comments in the thread |
| `r` | Reply — also a draft |
| `e` | Edit your own comment |
| `d` | Delete your own comment (confirm) |
| `o` | Open the permalink in a browser |
| `esc` | Back to the Main cursor |

`e` and `d` only work on your own comments; anything else toasts *"You can only
edit/delete your own comments."*

---

## 6. Check CI

**`[5]` Checks** collapses to a one-line summary when unfocused — `✓ CI passing`,
`✗ 3 checks failing`, `◔ 2 running`, `(no checks)`. Focus it to expand to the
full list; `[` / `]` switches to the `Timeline` tab. `enter` shows detail in
Main, `o` opens the check in a browser.

---

## 7. Submit

Press **`S`** from anywhere. The menu is titled with your draft count:

```
Submit review — 3 draft comment(s)
```

| Key | Choice | Message |
|---|---|---|
| `a` | **Approve** | optional |
| `r` | **Request changes** | required |
| `c` | **Comment** | required |
| `d` | **Discard pending review** | — |

Picking one opens the composer for the review body; `ctrl+s` sends, `esc` backs
out. The required/optional split is GitHub's rule, enforced before the request
goes out — an empty body on Approve just submits.

You do **not** need drafts to submit: `S` → `a` on a PR you've only read creates
the review and approves it in one call.

**On success:** toast *"Review submitted"*, focus returns to the PR list, and
both the list and the PR detail refetch so the row's review decision is current.

**Discard** (`d`) asks for confirmation first — *"Discard pending review? This
deletes all drafts"* — and defaults to **No, keep**. Confirming deletes every
draft on the pending review. There is no undo.

---

## Quick reference

```
Triage      2 · [ ] tabs · / filter · enter to open
Overview    0 · z fold · -/= fold all · b flag bot/human · t/T · m/M
Read        3 · j/k (Main follows) · enter · space viewed · ` tree/flat
Diff        0 · j/k · h/l hunks · [ ] files · t/T threads · zz center
Comment     c line · v then c range · c in Files = file-level · ctrl+s
Threads     4 · enter jump+focus · space resolve · r reply · e/d own comment
CI          5 · enter detail · o browser
Ship        S · a approve / r request / c comment / d discard · ctrl+s
```

Anywhere: `y` copy menu · `o` open in browser · `R` refresh · `@` command log ·
`?` help · `q` quit.

---

## Things that will save you a confused minute

- **`0` doesn't reset Main.** It focuses Main and leaves your file diff and
  scroll alone. `esc` is what returns Main to the PR overview.
- **`space` means two things, consistently.** In Files or on a Main file header
  it's *viewed*. On a thread it's *resolve*. Never both.
- **`enter` always focuses the thread under the cursor**, pinned by ID — not
  whatever the Threads panel happens to have selected. `r` / `e` / `d` then act
  on that thread.
- **`[` / `]` are contextual.** In a side panel they switch tabs; in Main they
  move between files (Main has no tabs).
- **Commit-scoped diffs and directory views are read-only for line actions.**
  Neither maps 1:1 to a single file's lines, so `c` is inert. Open the file
  itself to comment.
- **An external `gui.diffPager` disables cursor-follow and line comments.** The
  pager's output can't be mapped back to diff lines. You still get diffs on
  `enter`; drop the setting to author inline.
- **`R` bypasses every cache** when you think you're looking at stale data.
- **`z` is context-sensitive.** On a foldable row (a thread block, a comment, a bot
  run) it folds. Anywhere else it's the first tap of `zz` (center cursor). So
  centering still works on code lines that happen to carry a comment.
- **Folding a thread folds it everywhere.** Review threads fold by thread ID, so
  collapsing one in the diff also collapses it in the overview's `Review threads`
  section — it's one thread, not two views of one.
- **Fold state survives refetches.** Writing a draft or hitting `R` won't reopen
  everything you collapsed, and won't move your cursor off the row you were on.
- **A collapsed bot run still reports its verdict**, so you don't have to expand it
  just to learn whether CI was happy.
