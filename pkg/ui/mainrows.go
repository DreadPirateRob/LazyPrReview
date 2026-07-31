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
)

// mainRow is the metadata for a single Main line. It is populated ONLY for the
// built-in single-file diff — exactly the mode where a line anchor is meaningful.
// The PR overview, commit-scoped diffs, directory aggregates, and external-pager
// output all leave Model.MainRows nil, which keeps every line-scoped action inert
// in those modes by construction rather than by scattered guards.
type mainRow struct {
	Kind        mainRowKind
	RenderIndex int    // rowCode only; -1 otherwise
	ThreadID    string // rowThreadHeader / rowComment
	CommentIdx  int    // rowComment: index into Thread.Comments
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
	if folded, ok := m.ThreadFolded[th.ID]; ok {
		return !folded
	}
	return !th.IsResolved && !th.IsOutdated
}

// buildDiffRows renders one diff file into Main lines plus the parallel row
// model, injecting each thread's block immediately after the line it is anchored
// to. Threads that do not map to a diff line (file-level comments, and outdated
// threads whose line is gone) would otherwise be invisible here, so they are
// emitted right below the file header.
func buildDiffRows(m Model, idx int) ([]string, []mainRow) {
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

	lines := make([]string, 0, len(file.Rendered)+8)
	rows := make([]mainRow, 0, len(file.Rendered)+8)
	add := func(s string, r mainRow) {
		lines = append(lines, s)
		rows = append(rows, r)
	}

	add(diffHeaderStyle.Render("File: "+file.Path), mainRow{Kind: rowFileHeader, RenderIndex: -1})
	for _, at := range unanchored {
		appendThreadBlock(m, at, add)
	}
	for _, line := range file.Rendered {
		add(renderDiffLine(line), mainRow{Kind: rowCode, RenderIndex: line.RenderIndex})
		for _, at := range byAnchor[line.RenderIndex] {
			appendThreadBlock(m, at, add)
		}
	}
	return lines, rows
}

// appendThreadBlock emits one thread: a summary line, then its comments when
// expanded. Every emitted line carries the thread id so the cursor can act on the
// thread from any row of the block.
func appendThreadBlock(m Model, at diff.AnchoredThread, add func(string, mainRow)) {
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
		for _, l := range strings.Split(strings.TrimRight(c.Body, "\n"), "\n") {
			// Mentions are styled before any wrapper styling, while the line is
			// still plain text, so no escape sequences get mangled.
			add(threadGutter+"  "+highlightMentions(l, m.ViewerLogin), row)
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
	lines, rows := buildDiffRows(m, idx)
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
	next := make(map[string]bool, len(m.ThreadFolded)+1)
	for k, v := range m.ThreadFolded {
		next[k] = v
	}
	next[threadID] = expanded // was expanded → now folded
	m.ThreadFolded = next
	return refreshMainDiff(m)
}
