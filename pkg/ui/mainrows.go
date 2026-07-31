package ui

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// mainRowKind tags what a Main-pane line IS.
//
// The built-in diff used to be a strict 1:1 mapping — lines[1+i] was Rendered[i]
// — and every line-scoped feature derived its position arithmetically from that
// (anchorAt did row-1, thread jumps did RenderIndex+1). Injecting inline thread
// blocks breaks that invariant, so the renderer emits this parallel model and
// consumers look a row up instead of computing it.
type mainRowKind uint8

const (
	rowFileHeader   mainRowKind = iota // the "File: path" line
	rowCode                            // a diff line; RenderIndex points into File.Rendered
	rowThreadHeader                    // a thread's summary line (fold target)
	rowComment                         // one line of one comment's author/body

	// Overview kinds. The overview is prose, not code, so it has no RenderIndex —
	// its foldable rows are identified by Key instead.
	rowMeta        // PR title / metadata line
	rowSection     // a section rule ("Description", "Conversation …")
	rowEventHeader // one timeline comment's summary line (fold target)
	rowEventBody   // one line of a timeline comment's rendered body
	rowGroupHeader // a collapsed run of bot comments (fold target)
)

// mainRow is the metadata for a single Main line, populated for the built-in
// single-file diff and for the PR overview — the two modes with structure worth
// addressing. Commit-scoped diffs, directory aggregates, and external-pager output
// leave Model.MainRows nil.
//
// Line-scoped DIFF actions additionally gate on MainMode == MainDiff, so overview
// rows can never be mistaken for a code anchor.
type mainRow struct {
	Kind        mainRowKind
	RenderIndex int    // rowCode only; -1 otherwise
	ThreadID    string // rowThreadHeader / rowComment
	CommentIdx  int    // rowComment: index into Thread.Comments
	// Key identifies a foldable overview row across rebuilds. Timeline items carry
	// no server ID, so folding by row index would scramble the moment the content
	// is rebuilt (a draft lands, a refetch arrives, the pane resizes). See
	// timelineKey / groupKey.
	Key string
	// Author is the comment author for overview event/group rows, so acting on the
	// row (flagging a bot) works from a stable identity instead of re-deriving it
	// from rendered text, which grouping and markdown make unreliable.
	Author string
}

var (
	// Threads are drawn in a gutter-marked block so they read as annotation
	// rather than code, at a glance.
	threadGutter       = "▎ "
	threadOpenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	threadSettledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Faint(true)
	threadAuthorStyle  = lipgloss.NewStyle().Bold(true)
	threadBodyStyle    = lipgloss.NewStyle()
)

// threadExpanded reports whether a thread's comments are shown. Unresolved
// threads default open and settled (resolved/outdated) ones default collapsed to
// their summary line; an explicit fold toggle overrides either default.
func threadExpanded(m Model, th domain.Thread) bool {
	if folded, ok := m.Folded[th.ID]; ok {
		return !folded
	}
	return !th.IsResolved && !th.IsOutdated
}

// buildDiffRows renders one diff file into Main lines plus the parallel row
// model, injecting each thread's block immediately after the line it is anchored
// to. Threads that do not map to a diff line (file-level comments, and outdated
// threads whose line is gone) would otherwise be invisible here, so they are
// emitted right below the file header.
func buildDiffRows(m Model, idx int, width int) ([]string, []mainRow) {
	if idx < 0 || idx >= len(m.DiffFiles) {
		return nil, nil
	}
	file := m.DiffFiles[idx]

	byAnchor := map[int][]diff.AnchoredThread{}
	unanchored := make([]diff.AnchoredThread, 0, 4)
	for _, at := range m.AnchoredThreads {
		if at.FilePath != file.Path {
			continue
		}
		if at.Anchored && at.RenderIndex >= 0 {
			byAnchor[at.RenderIndex] = append(byAnchor[at.RenderIndex], at)
			continue
		}
		unanchored = append(unanchored, at)
	}

	hlLines := highlightedDiffLines(file)
	lines := make([]string, 0, len(file.Rendered)+8)
	rows := make([]mainRow, 0, len(file.Rendered)+8)
	add := func(s string, r mainRow) {
		lines = append(lines, s)
		rows = append(rows, r)
	}

	add(diffHeaderStyle.Render("File: "+file.Path), mainRow{Kind: rowFileHeader, RenderIndex: -1})
	for _, at := range unanchored {
		appendThreadBlock(m, at, width, add)
	}
	for _, line := range file.Rendered {
		add(renderDiffLine(line, hlLines[line.RenderIndex]), mainRow{Kind: rowCode, RenderIndex: line.RenderIndex})
		for _, at := range byAnchor[line.RenderIndex] {
			appendThreadBlock(m, at, width, add)
		}
	}
	return lines, rows
}

// appendThreadBlock emits one thread: a summary line, then its comments when
// expanded. Every emitted line carries the thread id so the cursor can act on the
// thread from any row of the block.
func appendThreadBlock(m Model, at diff.AnchoredThread, width int, add func(string, mainRow)) {
	th := at.Thread
	expanded := threadExpanded(m, th)
	add(threadGutter+threadSummary(m, at, expanded), mainRow{
		Kind: rowThreadHeader, ThreadID: th.ID, RenderIndex: -1,
	})
	if !expanded {
		return
	}
	for ci, c := range th.Comments {
		row := mainRow{Kind: rowComment, ThreadID: th.ID, CommentIdx: ci, RenderIndex: -1}
		head := threadAuthorStyle.Render(orDash(c.Author))
		if c.State == "PENDING" {
			head += draftStyle.Render(" [draft]")
		}
		add(threadGutter+head+":", row)
		// Comment bodies are markdown and get the same treatment as the overview:
		// rendered and wrapped to the remaining budget. Emitting raw split lines here
		// showed literal ** and clipped long lines at the pane edge.
		for _, l := range renderMarkdownFor(c.Body, width-4, m.ViewerLogin) {
			add(threadGutter+"  "+l, row)
		}
	}
}

// threadSummary is the thread's one-line header: fold caret, first author,
// state, comment count, and the line span for a multi-line thread. A range
// thread anchors at its LAST line (AnchorThreads resolves Thread.Line), so the
// span is stated here rather than implied by placement.
func threadSummary(m Model, at diff.AnchoredThread, expanded bool) string {
	th := at.Thread
	caret := "▸"
	if expanded {
		caret = "▾"
	}
	// The caret is a prefix, not a field: joining it with " · " like the rest
	// produces a stray leading separator ("▾ · author · unresolved").
	parts := make([]string, 0, 5)
	if len(th.Comments) > 0 {
		parts = append(parts, orDash(th.Comments[0].Author))
	}
	parts = append(parts, threadStatus(th))
	if n := len(th.Comments); n > 1 {
		parts = append(parts, fmt.Sprintf("%d comments", n))
	}
	if th.StartLine != nil && th.Line != nil && *th.StartLine != *th.Line {
		parts = append(parts, fmt.Sprintf("lines %d-%d", *th.StartLine, *th.Line))
	}
	if !at.Anchored {
		badge := at.Badge
		if badge == "" {
			badge = "[no line]"
		}
		parts = append(parts, badge)
	}
	out := caret + " " + strings.Join(parts, " · ")
	if threadHasDraft(th) {
		out += draftStyle.Render(" [draft]")
	}
	// State styling wraps the summary text, so the mention badge is appended AFTER
	// it — a collapsed thread still shows that it is addressed to you, which is the
	// point of flagging mentions at all.
	if th.IsResolved || th.IsOutdated {
		out = threadSettledStyle.Render(out)
	} else {
		out = threadOpenStyle.Render(out)
	}
	if threadMentionsViewer(th, m.ViewerLogin) {
		out += " " + mentionYouStyle.Render("@you")
	}
	return out
}

// rowAt returns the row metadata for a Main line, and ok=false when the current
// Main content has no row model (every mode except the built-in single-file diff).
func rowAt(m Model, line int) (mainRow, bool) {
	if line < 0 || line >= len(m.MainRows) {
		return mainRow{}, false
	}
	return m.MainRows[line], true
}

// codeRenderIndex maps a Main line to its index in File.Rendered, or -1 when the
// line is not a code row (header, thread block) or there is no row model.
func codeRenderIndex(m Model, line int) int {
	r, ok := rowAt(m, line)
	if !ok || r.Kind != rowCode {
		return -1
	}
	return r.RenderIndex
}

// mainLineForRenderIndex finds the Main line showing a given Rendered index, or
// -1. Thread blocks shift code lines down, so callers can no longer assume
// RenderIndex+1.
func mainLineForRenderIndex(m Model, renderIndex int) int {
	for i, r := range m.MainRows {
		if r.Kind == rowCode && r.RenderIndex == renderIndex {
			return i
		}
	}
	return -1
}

// setMainDiff points Main at one file's diff and owns the row model for it.
//
// With an external pager configured the output is opaque text with no line
// mapping, so rows stay nil and every line-scoped action remains inert — the
// same degradation line comments already take.
func setMainDiff(m Model, idx int) Model {
	if m.Config.GUI.DiffPager != "" {
		if lines := renderDiffFileExternal(m, idx); lines != nil {
			return setMainLines(m, lines)
		}
	}
	lines, rows := buildDiffRows(m, idx, mainContentWidth(m))
	m = setMainLines(m, lines)
	m.MainRows = rows // after setMainLines, which clears it
	return m
}

// mainLineForThread returns the Main line of a thread's summary row, or -1.
func mainLineForThread(m Model, threadID string) int {
	if threadID == "" {
		return -1
	}
	for i, r := range m.MainRows {
		if r.Kind == rowThreadHeader && r.ThreadID == threadID {
			return i
		}
	}
	return -1
}

// mainLineSpanForComment returns the Main line range one comment occupies: its
// author line plus every wrapped markdown body row. A comment is a block, not a
// line, so a caller highlighting only the first row would be indistinguishable
// from the ordinary cursor.
func mainLineSpanForComment(m Model, threadID string, commentIdx int) (start, count int) {
	start = -1
	if threadID == "" {
		return start, 0
	}
	for i, r := range m.MainRows {
		if r.Kind != rowComment || r.ThreadID != threadID || r.CommentIdx != commentIdx {
			continue
		}
		if start < 0 {
			start = i
		}
		count++
	}
	return start, count
}

// expandThread forces a thread's block open. Focus entering a thread must be able
// to land on a comment, and resolved/outdated threads default to collapsed — so
// without this, focusing one would have no rows to point at.
func expandThread(m Model, threadID string) Model {
	for _, at := range m.AnchoredThreads {
		if at.Thread.ID != threadID {
			continue
		}
		if threadExpanded(m, at.Thread) {
			return m
		}
		next := make(map[string]bool, len(m.Folded)+1)
		for k, v := range m.Folded {
			next[k] = v
		}
		next[threadID] = false
		m.Folded = next
		return refreshMainDiff(m)
	}
	return m
}

// refreshMainDiff re-renders the diff Main is already showing, keeping the cursor
// on the same THING rather than the same row index — injecting or folding a thread
// block shifts every row below it, so a raw index would drift onto unrelated
// content. Used after the thread set changes (a draft is added, reconciled, or
// rolled back) so new comments appear inline without the reader losing their place.
func refreshMainDiff(m Model) Model {
	if m.MainMode != MainDiff || m.MainFileIndex < 0 || m.MainFileIndex >= len(m.DiffFiles) {
		return m
	}
	// Remember what the cursor is on: a thread (by id, since its row moves) or a
	// code line (by render index). Anything else keeps its raw index.
	var keepThread string
	keepCode := -1
	if r, ok := rowAt(m, m.MainCursor); ok {
		switch r.Kind {
		case rowThreadHeader, rowComment:
			keepThread = r.ThreadID
		case rowCode:
			keepCode = r.RenderIndex
		}
	}
	scroll := m.MainScroll
	m = setMainDiff(m, m.MainFileIndex)
	switch {
	case keepThread != "":
		if line := mainLineForThread(m, keepThread); line >= 0 {
			m.MainCursor = line
		}
	case keepCode >= 0:
		if line := mainLineForRenderIndex(m, keepCode); line >= 0 {
			m.MainCursor = line
		}
	}
	if m.MainCursor > len(m.MainLines)-1 {
		m.MainCursor = maxInt(0, len(m.MainLines)-1)
	}
	m.MainScroll = scroll
	return m
}

// threadRowID returns the thread id when the cursor is on an actual inline thread
// ROW (its summary or one of its comment lines), else "".
//
// Deliberately narrower than threadIDAtMainCursor, which also matches the anchored
// code line: folding is bound to `z`, and `z` is the arming tap for `zz` (center
// cursor). Matching code lines here would silently steal `zz` on every line that
// happens to carry a comment.
func threadRowID(m Model) string {
	r, ok := rowAt(m, m.MainCursor)
	if !ok {
		return ""
	}
	if r.Kind == rowThreadHeader || r.Kind == rowComment {
		return r.ThreadID
	}
	return ""
}

// toggleThreadFold flips one thread's expansion and re-renders. The fold map is
// copy-on-write because a Model is passed by value everywhere; the flipped value
// is recorded explicitly rather than as a negation of absence, since the default
// depends on the thread's resolved state.
func toggleThreadFold(m Model, threadID string) Model {
	expanded := false
	for _, at := range m.AnchoredThreads {
		if at.Thread.ID == threadID {
			expanded = threadExpanded(m, at.Thread)
			break
		}
	}
	next := make(map[string]bool, len(m.Folded)+1)
	for k, v := range m.Folded {
		next[k] = v
	}
	next[threadID] = expanded // was expanded → now folded
	m.Folded = next
	return refreshMainDiff(m)
}

// foldTargetKey returns the fold identity of the row under the cursor, or "".
// Diff thread rows fold by thread ID; overview rows fold by their synthetic Key
// (timeline items carry no server ID). Both live in Model.Folded.
func foldTargetKey(m Model) string {
	r, ok := rowAt(m, m.MainCursor)
	if !ok {
		return ""
	}
	switch r.Kind {
	case rowThreadHeader, rowComment:
		return r.ThreadID
	case rowEventHeader, rowEventBody, rowGroupHeader:
		return r.Key
	}
	return ""
}

// toggleOverviewFold flips one overview row's expansion and rebuilds, keeping the
// cursor on the same row by Key — folding changes the row count, so a raw index
// would drift onto unrelated content.
func toggleOverviewFold(m Model, key string) Model {
	if m.PRDetail == nil || key == "" {
		return m
	}
	next := make(map[string]bool, len(m.Folded)+1)
	for k, v := range m.Folded {
		next[k] = v
	}
	// Record the flip explicitly: the default depends on whether the author is a
	// bot, so absence cannot be negated.
	next[key] = !m.Folded[key]
	if _, had := m.Folded[key]; !had {
		next[key] = isExpandedNow(m, key)
	}
	m.Folded = next

	scroll := m.MainScroll
	d := *m.PRDetail
	d.Threads = effectiveThreads(m)
	m = setMainOverview(m, d)
	if line := mainLineForKey(m, key); line >= 0 {
		m.MainCursor = line
	}
	if m.MainCursor > len(m.MainLines)-1 {
		m.MainCursor = maxInt(0, len(m.MainLines)-1)
	}
	m.MainScroll = scroll
	return m
}

// isExpandedNow reports a foldable overview row's CURRENT visible state, so the
// first toggle inverts what the reader actually sees rather than a default.
func isExpandedNow(m Model, key string) bool {
	for _, r := range m.MainRows {
		if r.Key != key {
			continue
		}
		switch r.Kind {
		case rowEventHeader, rowGroupHeader:
			// A header followed by body rows for the same key is expanded.
			return mainKeyHasBody(m, key)
		}
	}
	return false
}

func mainKeyHasBody(m Model, key string) bool {
	for _, r := range m.MainRows {
		if r.Key == key && r.Kind == rowEventBody {
			return true
		}
	}
	return false
}

// mainLineForKey finds the summary row for a fold key, or -1.
func mainLineForKey(m Model, key string) int {
	for i, r := range m.MainRows {
		if r.Key == key && (r.Kind == rowEventHeader || r.Kind == rowGroupHeader) {
			return i
		}
	}
	return -1
}

// allOverviewFoldKeys enumerates every foldable key the overview CAN have, derived
// from the data rather than the current rows.
//
// Reading the visible row model instead would make fold-all state-dependent: a
// collapsed bot run only exposes its group key, so "expand all" would open the run
// and leave its members shut — the reader presses = and still sees no bodies.
func allOverviewFoldKeys(m Model, d domain.PRDetail) []string {
	keys := make([]string, 0, len(d.Timeline)+len(d.Threads)+4)
	for i := 0; i < len(d.Timeline); {
		it := d.Timeline[i]
		keys = append(keys, timelineKey(it))
		if !isBotAuthor(m, it.Author) {
			i++
			continue
		}
		j := i
		for j < len(d.Timeline) && d.Timeline[j].Author == it.Author {
			j++
		}
		if j-i > 1 {
			keys = append(keys, groupKey(it))
			for _, member := range d.Timeline[i+1 : j] {
				keys = append(keys, timelineKey(member))
			}
		}
		i = j
	}
	for _, th := range d.Threads {
		keys = append(keys, th.ID)
	}
	return keys
}

// setAllOverviewFolds collapses or expands every foldable row in the overview.
func setAllOverviewFolds(m Model, folded bool) Model {
	if m.PRDetail == nil {
		return m
	}
	d := *m.PRDetail
	d.Threads = effectiveThreads(m)

	next := make(map[string]bool, len(m.Folded)+len(d.Timeline))
	for k, v := range m.Folded {
		next[k] = v
	}
	for _, k := range allOverviewFoldKeys(m, d) {
		next[k] = folded
	}
	m.Folded = next

	cursorKey := foldTargetKey(m)
	m = setMainOverview(m, d)
	if line := mainLineForKey(m, cursorKey); line >= 0 {
		m.MainCursor = line
	}
	if m.MainCursor > len(m.MainLines)-1 {
		m.MainCursor = maxInt(0, len(m.MainLines)-1)
	}
	return m
}
