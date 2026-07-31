package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

const (
	ThreadsTabUnresolved = "Unresolved"
	ThreadsTabAll        = "All"
	ThreadsTabDrafts     = "Drafts"
	ThreadsTabCommits    = "Commits"
)

var threadsTabOrder = []string{ThreadsTabUnresolved, ThreadsTabAll, ThreadsTabDrafts, ThreadsTabCommits}

func visibleThreads(m Model) []diff.AnchoredThread {
	switch m.ThreadTab {
	case ThreadsTabAll:
		return m.AnchoredThreads
	case ThreadsTabDrafts:
		return draftThreads(m)
	default:
		return m.UnresolvedThreadIndex
	}
}

// draftThreads is the single source for the Drafts tab and the draft count:
// PENDING (draft) anchored threads — server draft threads plus optimistic
// drafts, since anchors are built from the merged thread set — augmented with
// any pending-review comments not already surfaced as a thread (SPEC's dual-
// source model), deduped by comment id.
func draftThreads(m Model) []diff.AnchoredThread {
	out := []diff.AnchoredThread{}
	seen := map[string]bool{}
	for _, at := range m.AnchoredThreads {
		if !threadHasDraft(at.Thread) {
			continue
		}
		out = append(out, at)
		for _, c := range at.Thread.Comments {
			if c.ID != "" {
				seen[c.ID] = true
			}
		}
	}
	if m.PRDetail != nil && m.PRDetail.PendingReview != nil {
		for _, c := range m.PRDetail.PendingReview.Comments {
			if c.ID == "" || seen[c.ID] {
				continue
			}
			th := domain.Thread{ID: "pending:" + c.ID, Path: c.Path, Line: c.Line, Comments: []domain.Comment{c}}
			out = append(out, diff.AnchoredThread{Thread: th, FilePath: c.Path, Badge: "[unanchored]"})
		}
	}
	return out
}

func threadHasDraft(th domain.Thread) bool {
	for _, c := range th.Comments {
		if c.State == "PENDING" {
			return true
		}
	}
	return false
}

// filteredThreadRows returns the active tab's thread rows narrowed by the
// Threads panel filter (matched on path:line plus the first comment).
func filteredThreadRows(m Model) []diff.AnchoredThread {
	rows := visibleThreads(m)
	q := m.ThreadsPanel.Filter
	if q == "" {
		return rows
	}
	out := make([]diff.AnchoredThread, 0, len(rows))
	for _, at := range rows {
		if panelFilterMatch(m, at.FilePath+" "+compactThread(at), q) {
			out = append(out, at)
		}
	}
	return out
}

// filteredCommits narrows the Commits tab rows by the Threads panel filter.
func filteredCommits(m Model) []domain.CommitSummary {
	if m.ThreadsPanel.Filter == "" {
		return m.Commits
	}
	out := make([]domain.CommitSummary, 0, len(m.Commits))
	for _, c := range m.Commits {
		if panelFilterMatch(m, c.OID+" "+c.MessageHeadline, m.ThreadsPanel.Filter) {
			out = append(out, c)
		}
	}
	return out
}

// threadsTitle composes the Threads panel border title: all tabs with the
// active one bracketed, plus the filter indicator.
func threadsTitle(m Model) string {
	active := m.ThreadTab
	if active == "" {
		active = ThreadsTabUnresolved
	}
	return tabsTitle(threadsTabOrder, active) + filterSuffix(m, FocusThreads, m.ThreadsPanel)
}

// ThreadsView renders the Threads panel rows; tabs and filter state live in
// the border title (threadsTitle).
func ThreadsView(m Model) string {
	var sb strings.Builder
	active := m.ThreadTab
	if active == "" {
		active = ThreadsTabUnresolved
	}
	if active == ThreadsTabCommits {
		for i, c := range filteredCommits(m) {
			cursor := " "
			if i == m.ThreadsPanel.Cursor {
				cursor = ">"
			}
			sha := c.OID
			if len(sha) > 7 {
				sha = sha[:7]
			}
			sb.WriteString(fmt.Sprintf("%s %s %s\n", cursor, sha, c.MessageHeadline))
		}
		return sb.String()
	}
	for i, row := range filteredThreadRows(m) {
		cursor := " "
		if i == m.ThreadsPanel.Cursor {
			cursor = ">"
		}
		line := "?"
		if row.Thread.Line != nil {
			line = fmt.Sprintf("%d", *row.Thread.Line)
		}
		badge := row.Badge
		if row.Anchored {
			badge = ""
		}
		sb.WriteString(fmt.Sprintf("%s %s:%s %s%s\n", cursor, row.FilePath, line, compactThread(row), badge))
	}
	return sb.String()
}

func compactThread(at diff.AnchoredThread) string {
	prefix := ""
	if threadHasDraft(at.Thread) {
		prefix = draftStyle.Render("[draft] ")
	}
	if len(at.Thread.Comments) == 0 {
		return prefix + "thread"
	}
	body := at.Thread.Comments[0].Body
	body = strings.ReplaceAll(body, "\n", " ")
	if len(body) > 40 {
		body = body[:40]
	}
	return prefix + body
}

func UpdateThreads(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	ks := msg.Keystroke()
	if ks == "/" {
		m = startPanelFilter(m, &m.ThreadsPanel)
		return m, nil
	}
	if m.ThreadTab == ThreadsTabCommits {
		commits := filteredCommits(m)
		switch ks {
		case "j", "down":
			if m.ThreadsPanel.Cursor < len(commits)-1 {
				m.ThreadsPanel.Cursor++
			}
		case "k", "up":
			if m.ThreadsPanel.Cursor > 0 {
				m.ThreadsPanel.Cursor--
			}
		case "[":
			m = cycleThreadsTab(m, -1)
		case "]":
			m = cycleThreadsTab(m, +1)
		case "enter":
			if m.ThreadsPanel.Cursor < len(commits) {
				c := commits[m.ThreadsPanel.Cursor]
				m.Activity = "Loading commit diff…"
				return m, commitDiffCmd(m.Forge, m.Repo, c.OID, c.MessageHeadline)
			}
		}
		return m, nil
	}
	rows := filteredThreadRows(m)
	switch ks {
	case "j", "down":
		if m.ThreadsPanel.Cursor < len(rows)-1 {
			m.ThreadsPanel.Cursor++
		}
	case "k", "up":
		if m.ThreadsPanel.Cursor > 0 {
			m.ThreadsPanel.Cursor--
		}
	case "[":
		m = cycleThreadsTab(m, -1)
	case "]":
		m = cycleThreadsTab(m, +1)
	case "enter":
		if len(rows) > 0 && m.ThreadsPanel.Cursor < len(rows) {
			m = jumpToAnchoredThread(m, rows[m.ThreadsPanel.Cursor], true)
		}
	case "space":
		if len(rows) == 0 || m.ThreadsPanel.Cursor >= len(rows) {
			return m, nil
		}
		th := rows[m.ThreadsPanel.Cursor].Thread
		if !th.ViewerCanResolve {
			m.Toast = NewToast(ToastCannotResolveThread)
			return m, nil
		}
		target := !th.IsResolved
		m = setThreadResolved(m, th.ID, target) // optimistic
		m.Activity = "Updating thread…"
		return m, resolveThreadCmd(m.Forge, th.ID, target)
	}
	return m, nil
}

// setThreadResolved flips a thread's resolved state in the loaded PR detail and
// rebuilds the anchor/unresolved indexes so the panel reflects it immediately
// (optimistic). The forge call confirms or the caller rolls back.
func setThreadResolved(m Model, id string, resolved bool) Model {
	if m.PRDetail == nil {
		return m
	}
	for i := range m.PRDetail.Threads {
		if m.PRDetail.Threads[i].ID == id {
			m.PRDetail.Threads[i].IsResolved = resolved
			break
		}
	}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	if m.ThreadsPanel.Cursor > 0 {
		m.ThreadsPanel.Cursor = 0
	}
	return m
}

// cycleThreadsTab advances the active Threads tab by delta (wrapping) using
// threadsTabOrder.
func cycleThreadsTab(m Model, delta int) Model {
	active := m.ThreadTab
	if active == "" {
		active = ThreadsTabUnresolved
	}
	idx := 0
	for i, t := range threadsTabOrder {
		if t == active {
			idx = i
			break
		}
	}
	n := len(threadsTabOrder)
	return switchThreadsTab(m, threadsTabOrder[((idx+delta)%n+n)%n])
}

// switchThreadsTab changes the active Threads tab; the row domain changes,
// so the panel's cursor and filter reset with it.
func switchThreadsTab(m Model, tab string) Model {
	m.ThreadTab = tab
	m.ThreadsPanel.Cursor = 0
	m.ThreadsPanel.Filter = ""
	return m
}

func jumpToAnchoredThread(m Model, at diff.AnchoredThread, focusThread bool) Model {
	// Not gated on at.Anchored: file-level and outdated threads render inline too
	// (in the file-header block), so they are reachable — they just have no code
	// line to land on, and the thread-id lookup below finds their summary row.
	for i, file := range m.DiffFiles {
		if file.Path != at.FilePath {
			continue
		}
		m.MainFileIndex = i
		m = setMainDiff(m, i)
		m.MainMode = MainDiff
		// Land on the thread's own summary row when it is rendered inline, so the
		// comment is on screen; fall back to its anchored code line.
		// RenderIndex+1 no longer holds — inline blocks shift rows.
		switch {
		case m.Config.GUI.DiffPager != "":
			// External pager output doesn't map to diff lines; land
			// at the top of the file instead of a bogus row.
			m.MainCursor = 0
		case mainLineForThread(m, at.Thread.ID) >= 0:
			m.MainCursor = mainLineForThread(m, at.Thread.ID)
		case mainLineForRenderIndex(m, at.RenderIndex) >= 0:
			m.MainCursor = mainLineForRenderIndex(m, at.RenderIndex)
		default:
			m.MainCursor = 0
		}
		m = syncFilesCursorToMain(m)
		break
	}
	m = followMainCursor(m)
	if focusThread {
		m.ThreadCursor = 0
		m.FocusedThreadID = at.Thread.ID
		if m.CurrentFocus() != FocusThread {
			m = m.PushFocus(FocusThread)
		}
		return m
	}
	if m.CurrentFocus() != FocusMain {
		m = m.PushFocus(FocusMain)
	}
	return m
}

// focusedThread returns the thread being viewed in FocusThread. It resolves the
// pinned FocusedThreadID (set when focus was entered — tab-independent) against
// the anchored threads, falling back to the Threads-panel cursor.
func focusedThread(m Model) (diff.AnchoredThread, bool) {
	if m.FocusedThreadID != "" {
		for _, at := range m.AnchoredThreads {
			if at.Thread.ID == m.FocusedThreadID {
				return at, true
			}
		}
	}
	rows := visibleThreads(m)
	i := m.ThreadsPanel.Cursor
	if i < 0 || i >= len(rows) {
		return diff.AnchoredThread{}, false
	}
	return rows[i], true
}

// focusedComment returns the comment under ThreadCursor within the focused
// thread (clamped), or ok=false when there is none.
func focusedComment(m Model) (domain.Comment, bool) {
	at, ok := focusedThread(m)
	if !ok || len(at.Thread.Comments) == 0 {
		return domain.Comment{}, false
	}
	i := m.ThreadCursor
	if i < 0 || i >= len(at.Thread.Comments) {
		i = 0
	}
	return at.Thread.Comments[i], true
}

// threadIDAtMainCursor returns the id of the thread the Main cursor addresses, or
// "" when there is none.
//
// A thread is addressable from two places: any row of its inline block (summary or
// comment line, each of which carries the id) and the code line it is anchored to.
// The block rows are new; keeping the anchored code line means `enter` stays
// forgiving for readers who never move into the block. Returns "" whenever there
// is no row model, which is exactly the modes where a thread cannot be addressed
// (overview, commit/dir aggregate, external pager).
func threadIDAtMainCursor(m Model) string {
	if m.MainMode != MainDiff {
		return ""
	}
	r, ok := rowAt(m, m.MainCursor)
	if !ok {
		return ""
	}
	if r.Kind == rowThreadHeader || r.Kind == rowComment {
		return r.ThreadID
	}
	if r.Kind != rowCode || m.MainFileIndex < 0 || m.MainFileIndex >= len(m.DiffFiles) {
		return ""
	}
	path := m.DiffFiles[m.MainFileIndex].Path
	for _, at := range m.AnchoredThreads {
		if at.Anchored && at.FilePath == path && at.RenderIndex == r.RenderIndex {
			return at.Thread.ID
		}
	}
	return ""
}

// beginCommentDelete stores the delete target (with the identifiers DeleteComment
// needs for pending vs published) and opens the destructive-action confirm menu.
func beginCommentDelete(m Model, c domain.Comment) Model {
	m.DeleteTarget = forge.CommentRef{ID: c.ID, FullDatabaseID: c.FullDatabaseID, Pending: c.State == "PENDING"}
	return openDeleteConfirmMenu(m)
}
