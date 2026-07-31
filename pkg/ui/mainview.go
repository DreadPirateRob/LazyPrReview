package ui

import (
	"fmt"
	"image"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// setMainLines replaces the Main pane content and bumps the render generation,
// invalidating any active multi-line selection tied to the previous content.
func setMainLines(m Model, lines []string) Model {
	m.MainLines = lines
	m.MainGen++
	// Any new Main content drops the row model and the diff-source descriptor; the
	// diff builders re-set theirs immediately after their own call, so those are the
	// only places that opt back in.
	m.MainDiff = mainDiffSource{}
	m.MainRows = nil
	return m
}

func MainView(m Model) string {
	if m.PRDetail == nil {
		return "Main\n"
	}
	if len(m.MainLines) == 0 {
		body := strings.TrimSpace(m.PRDetail.Body)
		if body == "" {
			body = m.PRDetail.Title
		}
		return "Main\n" + body + "\n"
	}
	var sb strings.Builder
	sb.WriteString("Main\n")
	mark := m.MainCursor
	if mainRangeActive(m) {
		mark, _ = mainRangeBounds(m)
	}
	for i, line := range m.MainLines {
		cursor := " "
		if i == mark {
			cursor = ">"
		}
		sb.WriteString(cursor + " " + line + "\n")
	}
	return sb.String()
}

func UpdateMain(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	ks, bound := m.resolveKey(msg)
	if !bound {
		return m, nil
	}

	// zz: two-tap vim-style center. The first z arms; a second z centers the
	// cursor line. Any other key disarms and is handled normally.
	//
	// Inside an inline thread block, a single z folds/unfolds that thread instead
	// of arming — the block is what you want to collapse there, and `zz` stays
	// available on every code row. threadRowID (not threadIDAtMainCursor) is the
	// narrow check, so an anchored code line keeps its centering behavior.
	if m.MainPendingZ {
		m.MainPendingZ = false
		if ks == "z" {
			return centerMainCursor(m), nil
		}
	} else if ks == "z" {
		// Fold whatever is foldable under the cursor: an inline diff thread block, or
		// an overview comment / bot run. Plain prose and code rows fall through so
		// `zz` centering still works everywhere else.
		if key := foldTargetKey(m); key != "" {
			if m.MainMode == MainOverview {
				return toggleOverviewFold(m, key), nil
			}
			return toggleThreadFold(m, key), nil
		}
		// In a multi-file view, z folds/unfolds the file whose header the cursor sits on.
		if m.MainDiff.Kind == mainDiffDir || m.MainDiff.Kind == mainDiffCommit {
			if r, ok := rowAt(m, m.MainCursor); ok && r.Kind == rowFileHeader && r.Key != "" {
				path := strings.TrimPrefix(r.Key, "file:")
				return toggleFileFold(m, path), nil
			}
		}
		m.MainPendingZ = true
		return m, nil
	}

	// Collapse / expand every file in a multi-file diff view. Distinct from the
	// overview fold-all below: multi-file views are MainDiff kind dir/commit, not
	// MainOverview.
	if (m.MainDiff.Kind == mainDiffDir || m.MainDiff.Kind == mainDiffCommit) && (ks == "-" || ks == "=") {
		return setAllFileFolds(m, ks == "-"), nil
	}

	// Collapse / expand every foldable overview row. Same keys and meaning as the
	// Files tree, so the idiom carries over.
	if m.MainMode == MainOverview && (ks == "-" || ks == "=") && m.PRDetail != nil {
		return setAllOverviewFolds(m, ks == "-"), nil
	}

	// Flag the comment author under the cursor as bot/human. Overview only: it acts
	// on a comment, and a diff has none.
	if m.MainMode == MainOverview && ks == "b" {
		return toggleAuthorRole(m)
	}

	// Side-by-side is a diff-only view; the overview has no old/new sides to pair.
	if ks == "|" {
		return toggleSplitView(m)
	}

	switch ks {
	case "j", "down":
		m.MainCursor = stepMainCursor(m, 1)
		m = followMainCursor(m)
	case "k", "up":
		m.MainCursor = stepMainCursor(m, -1)
		m = followMainCursor(m)
	case "J", "ctrl+d":
		m = scrollMain(m, mainHalfPage(m))
	case "K", "ctrl+u":
		m = scrollMain(m, -mainHalfPage(m))
	case "pgdown", "pgdn":
		m = scrollMain(m, mainFullPage(m))
	case "pgup":
		m = scrollMain(m, -mainFullPage(m))
	case "<":
		m.MainCursor = 0
		m = followMainCursor(m)
	case ">":
		if len(m.MainLines) > 0 {
			m.MainCursor = len(m.MainLines) - 1
		}
		m = followMainCursor(m)
	case "[", "]":
		// Gate on the DESCRIPTOR, not MainFileIndex. MainFileIndex is -1 in every
		// read-only view (side-by-side, directory, commit), where `[` was dead and `]`
		// evaluated -1 < len-1 and jumped to file 0 instead of the next file. The
		// descriptor carries the real index in both modes, and Kind pins this to
		// single-file views, where "next file" is the only place it means anything.
		if m.MainDiff.Kind != mainDiffFile {
			return m, nil
		}
		next := m.MainDiff.FileIndex - 1
		if ks == "]" {
			next = m.MainDiff.FileIndex + 1
		}
		if next >= 0 && next < len(m.DiffFiles) {
			m = setMainFile(m, next)
			m = followMainCursor(m)
		}
	case "space":
		// The file-header row *is* the file, so space there toggles its viewed
		// state — same meaning as space in the Files panel. Keyed off the row model
		// rather than "cursor == 0": it also proves we are in a built-in single-file
		// diff, since no other Main mode populates rows.
		if m.MainMode != MainDiff {
			return m, nil
		}
		if r, ok := rowAt(m, m.MainCursor); !ok || r.Kind != rowFileHeader {
			return m, nil
		}
		idx := prFileIndexForMain(m)
		if idx < 0 {
			return m, nil
		}
		return toggleViewedFiles(m, []int{idx})
	case "t":
		if len(m.UnresolvedThreadIndex) == 0 {
			m.Toast = NewToast(ToastNoUnresolvedThreads)
			return m, nil
		}
		m = cycleThreadJump(m, +1)
	case "T":
		if len(m.UnresolvedThreadIndex) == 0 {
			m.Toast = NewToast(ToastNoUnresolvedThreads)
			return m, nil
		}
		m = cycleThreadJump(m, -1)
	case "m":
		if len(mentionThreads(m)) == 0 {
			m.Toast = NewToast(ToastNoMentions)
			return m, nil
		}
		m = cycleMentionJump(m, +1)
	case "M":
		if len(mentionThreads(m)) == 0 {
			m.Toast = NewToast(ToastNoMentions)
			return m, nil
		}
		m = cycleMentionJump(m, -1)
	case "enter":
		if id := threadIDAtMainCursor(m); id != "" {
			m = enterThreadFocus(m, id)
		}
	case "v":
		if m.MainMode != MainDiff || m.Config.GUI.DiffPager != "" {
			return m, nil
		}
		if mainRangeActive(m) {
			m.MainRangeActive = false
		} else if _, _, _, ok := mainCommentAnchor(m); ok {
			m.MainRangeActive = true
			m.MainRangeStart = m.MainCursor
			m.MainRangeGen = m.MainGen
		}
		return m, nil
	case "c":
		if mainRangeActive(m) {
			return openRangeComment(m)
		}
		path, line, side, ok := mainCommentAnchor(m)
		if !ok {
			if m.MainMode == MainDiff && m.Config.GUI.DiffPager != "" {
				m.Toast = NewToast(ToastLineCommentPager)
			}
			return m, nil
		}
		m = openComposer(m, composeState{
			Kind: composeComment, Required: true, SubjectType: "LINE",
			Title: fmt.Sprintf("Comment %s:%d (%s) — ctrl+s submit", path, line, side),
			Path:  path, Line: line, Side: side,
			Preview: previewRows(m, m.MainCursor, m.MainCursor),
		})
		return m, nil
	}
	return m, nil
}

// mainVisibleRows returns how many content rows the Main pane shows
// (non-positive when dimensions are unknown, i.e. headless).
func mainVisibleRows(m Model) int {
	return ComputeLayout(m).MainHeight - 2
}

// mainHalfPage is the ctrl+d/ctrl+u stride; mainFullPage the PgDn/PgUp one.
// Headless or degenerate panes fall back to the historical fixed page.
func mainHalfPage(m Model) int {
	if rows := mainVisibleRows(m); rows > 1 {
		return maxInt(1, rows/2)
	}
	return 10
}

func mainFullPage(m Model) int {
	if rows := mainVisibleRows(m); rows > 1 {
		return rows
	}
	return 10
}

// scrollMain moves the cursor and the view together, vim-style: after
// ctrl+d/ctrl+u the cursor keeps its relative row on screen.
func scrollMain(m Model, delta int) Model {
	m.MainCursor += delta
	if last := len(m.MainLines) - 1; m.MainCursor > last {
		m.MainCursor = last
	}
	if m.MainCursor < 0 {
		m.MainCursor = 0
	}
	m.MainScroll += delta
	return followMainCursor(m)
}

// followMainCursor clamps the Main scroll origin so the cursor stays inside
// the visible window and the window inside the content. With unknown
// dimensions the origin resets and rendering derives the window from the
// cursor alone.
func followMainCursor(m Model) Model {
	rows := mainVisibleRows(m)
	if rows < 1 {
		m.MainScroll = 0
		return m
	}
	if m.MainCursor < m.MainScroll {
		m.MainScroll = m.MainCursor
	}
	if m.MainCursor >= m.MainScroll+rows {
		m.MainScroll = m.MainCursor - rows + 1
	}
	if maxTop := len(m.MainLines) - rows; m.MainScroll > maxTop {
		m.MainScroll = maxTop
	}
	if m.MainScroll < 0 {
		m.MainScroll = 0
	}
	return m
}

// centerMainCursor centers the cursor's line vertically in the pane (zz).
func centerMainCursor(m Model) Model {
	if mainVisibleRows(m) < 1 {
		return m
	}
	m.MainScroll = m.MainCursor - mainVisibleRows(m)/2
	return followMainCursor(m)
}

func UpdateThreadFocus(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	at, ok := focusedThread(m)
	n := 0
	if ok {
		n = len(at.Thread.Comments)
	}
	ks, bound := m.resolveKey(msg)
	if !bound {
		return m, nil
	}
	switch ks {
	case "j", "down":
		if m.ThreadCursor < n-1 {
			m.ThreadCursor++
			m = syncMainCursorToComment(m)
		}
	case "k", "up":
		if m.ThreadCursor > 0 {
			m.ThreadCursor--
			m = syncMainCursorToComment(m)
		}
	case "e":
		c, ok := focusedComment(m)
		if !ok {
			return m, nil
		}
		if !c.ViewerDidAuthor {
			m.Toast = NewToast(ToastNotYourComment)
			return m, nil
		}
		m = openComposer(m, composeState{Kind: composeEdit, Required: true, CommentID: c.ID, Title: "Edit comment — ctrl+s save · esc cancel"})
		m.Composer.SetValue(c.Body)
		return m, nil
	case "d":
		c, ok := focusedComment(m)
		if !ok {
			return m, nil
		}
		if !c.ViewerDidAuthor {
			m.Toast = NewToast(ToastNotYourComment)
			return m, nil
		}
		return beginCommentDelete(m, c), nil
	case "r":
		at, ok := focusedThread(m)
		if !ok {
			return m, nil
		}
		if !at.Thread.ViewerCanReply {
			m.Toast = NewToast(ToastCannotReply)
			return m, nil
		}
		m = openComposer(m, composeState{Kind: composeReply, Required: true, ThreadID: at.Thread.ID, Title: "Reply to thread — ctrl+s submit · esc cancel"})
		return m, nil
	}
	return m, nil
}

func cycleThreadJump(m Model, delta int) Model {
	if len(m.UnresolvedThreadIndex) == 0 {
		return m
	}
	// Locate the thread the cursor is on so t/T advance from there.
	// threadIDAtMainCursor matches from anywhere inside an inline block as well as
	// from the anchored code line, which is all the old arithmetic could reach.
	current := 0
	if id := threadIDAtMainCursor(m); id != "" {
		for i, at := range m.UnresolvedThreadIndex {
			if at.Thread.ID == id {
				current = i
				break
			}
		}
	}
	next := (current + delta + len(m.UnresolvedThreadIndex)) % len(m.UnresolvedThreadIndex)
	return jumpToAnchoredThread(m, m.UnresolvedThreadIndex[next], false)
}

var draftStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan [draft]

// reviewerList returns reviewer display names in order, teams prefixed with @.
// The joined form below is built on this so both share one ordering.
func reviewerList(rs []domain.RequestedReviewer) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		if r.Kind == "team" {
			out[i] = "@" + r.Name
		} else {
			out[i] = r.Name
		}
	}
	return out
}

func reviewerNames(rs []domain.RequestedReviewer) string {
	if len(rs) == 0 {
		return "—"
	}
	return strings.Join(reviewerList(rs), ", ")
}

func labelList(ls []domain.Label) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Name
	}
	return out
}

func labelNames(ls []domain.Label) string {
	if len(ls) == 0 {
		return "—"
	}
	return strings.Join(labelList(ls), ", ")
}

func joinOrDash(vs []string) string {
	if len(vs) == 0 {
		return "—"
	}
	return strings.Join(vs, ", ")
}

// timelineVerb phrases what a timeline event did. The kinds here are the DECODER's
// vocabulary ("comment" / "review" / "merged", from buildTimeline) — this switch
// previously matched the GraphQL typenames instead, so every human review fell to
// the default and rendered without its state: an approval was indistinguishable
// from a drive-by comment unless the body happened to carry a bot verdict.
func timelineVerb(it domain.TimelineItem) string {
	switch it.Kind {
	case "review":
		switch strings.ToUpper(it.State) {
		case "APPROVED":
			return "approved"
		case "CHANGES_REQUESTED":
			return "requested changes"
		case "DISMISSED":
			return "review dismissed"
		case "COMMENTED", "":
			return "reviewed"
		}
		return "reviewed (" + strings.ToLower(it.State) + ")"
	case "comment":
		return "commented"
	case "merged":
		return "merged this pull request"
	default:
		return orDash(it.Kind)
	}
}

// timelineStateGlyph is the collapsed-row signal for review outcomes, using the
// SPEC §8 vocabulary. Bot verdicts get theirs from the body text; a human approval
// usually has NO body, so without this the collapsed row carried no outcome at all.
func timelineStateGlyph(it domain.TimelineItem) string {
	switch it.Kind {
	case "review":
		switch strings.ToUpper(it.State) {
		case "APPROVED":
			return ovOpenStyle.Render("✓")
		case "CHANGES_REQUESTED":
			return ovClosedStyle.Render("±")
		}
	case "merged":
		return ovMergedStyle.Render("✔")
	}
	return ""
}

func threadStatus(th domain.Thread) string {
	switch {
	case th.IsResolved:
		return "resolved"
	case th.IsOutdated:
		return "outdated"
	default:
		return "unresolved"
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func fullView(m Model) string {
	layout := ComputeLayout(m)
	header := fmt.Sprintf("mode: %s portrait=%t side=%d main=%d", m.ScreenMode, layout.Portrait, layout.SidePanelWidth, layout.MainWidth)
	if m.Toast.Message != "" {
		header += " | toast: " + m.Toast.Message
	} else if m.Activity != "" {
		header += " | activity: " + m.Activity
	}

	focus := m.CurrentFocus()
	fi := focusedPanelIndex(focus)
	// Thread focus acts on content in Main, so no side panel wears the focus border.
	// focusedPanelIndex still reports 3 for FocusThread because layout.go uses it to
	// allocate panel heights — this override is render-only, deliberately.
	if focus == FocusThread {
		fi = -1
	}

	sidePanes := func(w int) []string {
		return []string{
			renderPane(paneBox{title: "Status", number: 1, content: StatusView(m), width: w, height: layout.PanelHeights[0], anchor: m.Status.Cursor, focused: fi == 0}),
			renderPane(paneBox{title: prListTitle(m), number: 2, content: PRListView(m, layout.PanelHeights[1]), width: w, height: layout.PanelHeights[1], anchor: 2 * m.PRPanel.Cursor, focused: fi == 1, loading: m.LoadingPRs, selectionSpan: 2}),
			renderPane(paneBox{title: filesTitle(m), number: 3, content: FilesView(m, w), width: w, height: layout.PanelHeights[2], anchor: m.FilesPanel.Cursor, focused: fi == 2, loading: m.LoadingDetail}),
			renderPane(paneBox{title: threadsTitle(m), number: 4, content: ThreadsView(m), width: w, height: layout.PanelHeights[3], anchor: m.ThreadsPanel.Cursor, focused: fi == 3, loading: m.LoadingDetail}),
			renderPane(paneBox{title: checksTitle(m), number: 5, content: ChecksView(m), width: w, height: layout.PanelHeights[4], anchor: m.ChecksPanel.Cursor, focused: fi == 4, loading: m.LoadingDetail}),
		}
	}
	mainPane := func(w, h int) string {
		anchor, span := mainSelection(m, focus)
		// Thread focus acts on content in Main, so Main is the pane that owns the
		// user's attention — the border must say so.
		return renderPane(paneBox{titled: true, number: 0, content: MainView(m), width: w, height: h, anchor: anchor, focused: focus == FocusMain || focus == FocusThread, scroll: &m.MainScroll, loading: m.LoadingDetail, selectionSpan: span})
	}

	body := ""
	switch {
	case m.ScreenMode == ScreenFullscreen:
		body = mainPane(layout.MainWidth, layout.MainHeight)
	case layout.Portrait:
		body = joinPanes(append(sidePanes(layout.Width), mainPane(layout.Width, layout.MainHeight)))
	default:
		left := joinPanes(sidePanes(layout.SidePanelWidth))
		main := mainPane(layout.MainWidth, layout.MainHeight)
		body = renderColumns(left, main, layout.SidePanelWidth, layout.MainWidth)
	}

	// Floating overlays (composer, menus, confirms, help) draw as a centered box
	// ON TOP of the live UI, so the panels stay visible behind them.
	if content, anchor, ok := activeOverlay(m); ok {
		body = floatOverlay(body, content, anchor, layout)
	}

	// Header and hint bar render outside the pane boxes; clamp them to the
	// terminal width so an overlong line can never wrap and shear the frame.
	parts := []string{fitWidth(header, layout.Width), body}
	if m.CommandLogOn {
		parts = append(parts, renderPane(paneBox{titled: true, number: -1, content: commandLogView(m), width: layout.Width, height: layout.CommandLogHeight, anchor: 1 + m.CommandLogOffset, focused: m.CommandLogFocused}))
	}
	if hint := HintBarView(m); hint != "" {
		parts = append(parts, fitWidth(hint, layout.Width))
	}
	return strings.Join(parts, "\n")
}

// mainSelection reports the Main pane's selection: the anchor row (1-based, past
// the title line) and how many rows it covers. Extracted from the render closure
// so the thread-focus case is directly assertable — burying it there is why the
// focused comment could go unhighlighted with no test noticing.
func mainSelection(m Model, focus FocusContext) (anchor, span int) {
	switch {
	case mainRangeActive(m):
		lo, hi := mainRangeBounds(m)
		return 1 + lo, hi - lo + 1
	case focus == FocusThread:
		// Cover the whole focused comment. The cursor already sits on its first
		// row, but a comment is a block of wrapped markdown, and marking one row
		// of it looks identical to ordinary cursor movement.
		if at, ok := focusedThread(m); ok {
			if lo, n := mainLineSpanForComment(m, at.Thread.ID, m.ThreadCursor); n > 0 {
				return 1 + lo, n
			}
		}
	}
	return 1 + m.MainCursor, 1
}

// activeOverlay returns the content and selection anchor of the floating overlay
// to draw over the UI, in priority order (composer, menu, help), or ok=false when
// none is active.
func activeOverlay(m Model) (string, int, bool) {
	switch focus := m.CurrentFocus(); {
	case focus == FocusCompose:
		return ComposeView(m), 0, true
	case len(m.MenuItems) > 0 && focus == FocusMenu:
		return MenuView(m), 1 + m.MenuCursor, true
	case m.HelpVisible:
		return helpOverlayView(m), 1 + m.HelpCursor, true
	}
	return "", 0, false
}

// floatOverlay renders content as a centered floating box over base. The box is
// sized to its content and capped so the UI behind it stays visible, and it goes
// through renderPane so anchor/scroll clipping keeps long content reachable with
// the cursor. With unknown terminal dimensions (headless/tests) there is no grid
// to composite onto, so it degrades to rendering the overlay in place of body.
func floatOverlay(base, content string, anchor int, layout Layout) string {
	if layout.Width <= 0 || layout.MainHeight <= 0 {
		return renderPane(paneBox{titled: true, number: -1, content: content, width: layout.MainWidth, height: layout.MainHeight, anchor: anchor, focused: true})
	}
	w, h := modalDims(content, lipgloss.Width(base), lipgloss.Height(base))
	box := renderPane(paneBox{titled: true, number: -1, content: content, width: w, height: h, anchor: anchor, focused: true})
	return overlayCenter(base, box)
}

// modalDims sizes a floating box to its content, capped to leave a margin so the
// UI behind it frames the modal instead of being covered edge to edge.
func modalDims(content string, bw, bh int) (int, int) {
	w := lipgloss.Width(content) + 2  // left + right border
	h := lipgloss.Height(content) + 2 // top + bottom border
	if lim := bw - 4; lim > 0 && w > lim {
		w = lim
	}
	if lim := bh - 2; lim > 0 && h > lim {
		h = lim
	}
	if w > bw {
		w = bw
	}
	if h > bh {
		h = bh
	}
	return w, h
}

// overlayCenter composites modal centered over base on a lipgloss cell canvas,
// leaving base cells outside the modal's rectangle untouched. The result keeps
// base's exact dimensions, so the header and hint bar never shift.
func overlayCenter(base, modal string) string {
	bw, bh := lipgloss.Width(base), lipgloss.Height(base)
	mw, mh := lipgloss.Width(modal), lipgloss.Height(modal)
	if bw <= 0 || bh <= 0 || mw <= 0 || mh <= 0 {
		return base
	}
	if mw > bw {
		mw = bw
	}
	if mh > bh {
		mh = bh
	}
	x, y := (bw-mw)/2, (bh-mh)/2
	canvas := lipgloss.NewCanvas(bw, bh)
	lipgloss.NewLayer(base).Draw(canvas, canvas.Bounds())
	lipgloss.NewLayer(modal).Draw(canvas, image.Rect(x, y, x+mw, y+mh))
	return canvas.Render()
}

// joinPanes stacks rendered panes, dropping panes that got no rows at all so
// degenerate layouts never add stray blank lines to the frame.
func joinPanes(panes []string) string {
	kept := panes[:0]
	for _, p := range panes {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

// paneBox describes one panel for the render layer.
type paneBox struct {
	title         string // border title, used when the content has no embedded title line
	titled        bool   // content's first line is a title; it moves into the border
	number        int    // jump-key digit shown in the border; -1 hides it
	content       string
	width         int // outer box width, borders included
	height        int // outer box height, borders included
	anchor        int // content-coordinate line index kept visible (the selection row)
	focused       bool
	scroll        *int // explicit top row (content coords, post-title); nil derives from anchor
	loading       bool // dim + interaction-blocked while its data refetches
	selectionSpan int  // rows the selection covers (PR rows span 2); 0/1 = single
}

// renderPane renders one panel. With known terminal dimensions it draws a
// rounded bordered box — jump number and title embedded in the top edge, the
// focused panel accented — keeps the anchor row visible through a bubbles
// viewport, and repaints the selection row as a background highlight instead
// of a chevron. When dimensions are unknown (headless/tests) it falls back to
// plain clipping so the full content is still emitted without ANSI.
func renderPane(p paneBox) string {
	if p.width <= 0 {
		// Headless/test path: plain output, no ANSI. Explicit border titles
		// (tabs, filter state) are prepended as a line so the text rendering
		// stays faithful; anchor remains in content coordinates.
		content := p.content
		if !p.titled && p.title != "" {
			content = p.title + "\n" + content
		}
		if p.focused {
			content = markFocusedPane(content)
		}
		return clipPane(content, p.height, p.anchor)
	}
	if p.height <= 0 {
		// Real terminal, but the layout granted no rows: render nothing so
		// the pane is dropped instead of leaking unclipped content.
		return ""
	}

	title, body, anchor := p.title, p.content, p.anchor
	if p.titled {
		parts := strings.SplitN(p.content, "\n", 2)
		title = parts[0]
		body = ""
		if len(parts) > 1 {
			body = parts[1]
		}
		anchor--
	}
	if anchor < 0 {
		anchor = 0
	}
	span := p.selectionSpan
	if span < 1 {
		span = 1
	}
	if p.loading {
		// Dim the (stale) content and mark the title while a refetch is in
		// flight; interaction is blocked in handleKey.
		title = "⟳ " + title
		body = dimLines(body)
	}

	if p.height < 3 || p.width < 4 {
		// Too small for a border; render the bare scroll window.
		return paneWindow(body, p.width, p.height, anchor, p.scroll, span)
	}

	innerW, innerH := p.width-2, p.height-2
	inner := paneWindow(highlightSelection(body, anchor, innerW, p.focused, span), innerW, innerH, anchor, p.scroll, span)

	border := blurredBorderStyle
	if p.focused {
		border = focusedBorderStyle
	}
	side := border.Render("│")

	var sb strings.Builder
	sb.WriteString(border.Render(topBorder(title, p.number, innerW)))
	rows := strings.Split(inner, "\n")
	for i := 0; i < innerH; i++ {
		row := ""
		if i < len(rows) {
			row = rows[i]
		}
		sb.WriteString("\n")
		sb.WriteString(side)
		sb.WriteString(padRight(fitWidth(row, innerW), innerW))
		sb.WriteString(side)
	}
	sb.WriteString("\n")
	sb.WriteString(border.Render("╰" + strings.Repeat("─", innerW) + "╯"))
	return sb.String()
}

// paneWindow renders content through a bubbles viewport. A non-nil scroll
// pins the top row (the caller owns the origin, e.g. Main's sticky scroll);
// EnsureVisible then only corrects when the anchor would fall outside.
func paneWindow(content string, width, height, anchor int, scroll *int, span int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if span < 1 {
		span = 1
	}
	total := strings.Count(strings.TrimRight(content, "\n"), "\n") + 1
	clamp := func(a int) int {
		if a > total-1 {
			a = total - 1
		}
		if a < 0 {
			a = 0
		}
		return a
	}
	vp := viewport.New(
		viewport.WithWidth(width),
		viewport.WithHeight(height),
	)
	vp.SetContent(content)
	if scroll != nil {
		vp.SetYOffset(*scroll)
	}
	// Keep the whole selection ([anchor, anchor+span)) in view: ensure the
	// last row first, then the first, so the top wins if both can't fit.
	vp.EnsureVisible(clamp(anchor+span-1), 0, 0)
	vp.EnsureVisible(clamp(anchor), 0, 0)
	return vp.View()
}

// dimLines greys out every line (stripping any existing color) so a loading,
// non-interactive pane reads as inactive.
func dimLines(content string) string {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = dimStyle.Render(ansi.Strip(l))
	}
	return strings.Join(lines, "\n")
}

// topBorder builds a pane's top edge with the jump number and title embedded,
// e.g. ╭─[3]─Files [tree]──────╮.
func topBorder(title string, number int, innerW int) string {
	label := ""
	if title != "" {
		if number >= 0 {
			label = fmt.Sprintf("─[%d]─%s", number, title)
		} else {
			label = "─" + title
		}
		label = ansi.Truncate(label, innerW, "…")
	}
	fill := innerW - ansi.StringWidth(label)
	if fill < 0 {
		fill = 0
	}
	return "╭" + label + strings.Repeat("─", fill) + "╮"
}

// highlightSelection repaints the selection row as a full-width background
// highlight, dropping the `> ` chevron the views emit. Rows without the
// chevron prefix (scroll-only panes, placeholders, empty lists) are left
// untouched, so panes without a selection concept are unaffected.
func highlightSelection(body string, anchor, width int, focused bool, span int) string {
	if width <= 0 || anchor < 0 || span < 1 {
		return body
	}
	lines := strings.Split(body, "\n")
	if anchor >= len(lines) || !strings.HasPrefix(lines[anchor], "> ") {
		return body
	}
	style := blurredSelectStyle
	if focused {
		style = focusedSelectStyle
	}
	// Highlight every row of the selected entry (PR rows span two). Drop the
	// "> " marker from the first row; the row's own fg colors are kept and the
	// selection background is re-asserted after each inner reset so it runs
	// unbroken across the full width.
	for r := anchor; r < anchor+span && r < len(lines); r++ {
		text := lines[r]
		if r == anchor {
			text = "  " + lines[r][2:]
		}
		lines[r] = selectionRow(style, padRight(fitWidth(text, width), width))
	}
	return strings.Join(lines, "\n")
}

// selectionRow lays style's background under content while keeping content's own
// foreground colors intact — the shared background compositor, used both for the
// selection highlight and for the add/delete diff bands. lipgloss resets
// (\x1b[m / \x1b[0m) inside the row would otherwise clear the background mid-line,
// so the background opener is re-applied after each reset. Degrades to plain
// content when the active color profile emits no styling (e.g. NO_COLOR).
//
// A diff row arrives with its band already baked in. Selection SUPPRESSES that band
// rather than layering over part of it: the band opener is swapped for this row's
// background so exactly one background owns the whole row.
func selectionRow(style lipgloss.Style, content string) string {
	open := bandOpener(style)
	if open == "" {
		return content
	}
	sample := style.Render(" ")
	closer := sample[strings.IndexByte(sample, ' ')+1:]
	for _, band := range []string{bandOpener(diffAddBandStyle), bandOpener(diffDelBandStyle)} {
		if band != "" && band != open {
			content = strings.ReplaceAll(content, band, open)
		}
	}
	content = strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+open)
	content = strings.ReplaceAll(content, "\x1b[m", "\x1b[m"+open)
	return open + content + closer
}

// bandOpener returns the escape sequence that switches a style's background on, or
// "" when the active color profile emits no styling.
func bandOpener(style lipgloss.Style) string {
	sample := style.Render(" ")
	if sp := strings.IndexByte(sample, ' '); sp > 0 {
		return sample[:sp]
	}
	return ""
}

var (
	// Border styling: the focused panel reads accent+bold, the rest stay dim.
	focusedBorderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	blurredBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	// Selection rows render as a background "hover" instead of a chevron;
	// subdued when the owning pane is not focused.
	focusedSelectStyle = lipgloss.NewStyle().Background(lipgloss.Color("24"))
	blurredSelectStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	// dimStyle greys out a pane's content while it is loading.
	dimStyle = lipgloss.NewStyle().Faint(true)
)

// markFocusedPane prefixes the pane's title line with an active-panel marker so
// the focused panel is visually distinct from the others. Used on the headless
// path where ANSI styling is intentionally omitted.
func markFocusedPane(content string) string {
	lines := strings.SplitN(content, "\n", 2)
	lines[0] = "▌" + lines[0]
	return strings.Join(lines, "\n")
}

func clipPane(content string, height int, anchor int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if height <= 0 || len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if anchor < 0 {
		anchor = 0
	}
	if anchor >= len(lines) {
		anchor = len(lines) - 1
	}
	start := anchor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(lines) {
		end = len(lines)
		start = maxInt(0, end-height)
	}
	return strings.Join(lines[start:end], "\n")
}

func renderColumns(left, right string, leftWidth, rightWidth int) string {
	leftLines := strings.Split(strings.TrimRight(left, "\n"), "\n")
	rightLines := strings.Split(strings.TrimRight(right, "\n"), "\n")
	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	var sb strings.Builder
	for i := 0; i < maxLines; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = fitWidth(leftLines[i], leftWidth)
		}
		if i < len(rightLines) {
			r = fitWidth(rightLines[i], rightWidth)
		}
		sb.WriteString(padRight(l, leftWidth))
		sb.WriteString(r)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// fitWidth truncates s to the given display width, appending an ellipsis when
// content is cut. Measurement is ANSI-aware so styled lines (zero-width escape
// sequences) and wide characters are handled correctly.
func fitWidth(s string, width int) string {
	if width <= 0 || ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

// padRight pads s with spaces to the given display width, measuring visible
// columns rather than runes so ANSI-styled lines stay aligned.
func padRight(s string, width int) string {
	if width <= 0 {
		return s
	}
	w := ansi.StringWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func helpOverlayView(m Model) string {
	scoped := FilterHelpEntries(scopedHelpEntries(m), m.HelpQuery)
	var sb strings.Builder
	sb.WriteString("Help\n")
	for i, entry := range scoped {
		cursor := " "
		if i == m.HelpCursor {
			cursor = ">"
		}
		sb.WriteString(fmt.Sprintf("%s %s %s %s\n", cursor, entry.Context, strings.Join(entry.Keys, ","), entry.Action))
	}
	return sb.String()
}

func commandLogView(m Model) string {
	var sb strings.Builder
	title := "Command log"
	if m.CommandLogFocused {
		title += " [focused]"
	}
	sb.WriteString(title + "\n")
	start := m.CommandLogOffset
	if start < 0 {
		start = 0
	}
	if start > len(m.CommandLog) {
		start = len(m.CommandLog)
	}
	for _, entry := range m.CommandLog[start:] {
		sb.WriteString(strings.Join(entry.Command, " "))
		sb.WriteString("\n")
	}
	return sb.String()
}

func copyToastForFocus(m Model) string {
	switch m.CurrentFocus() {
	case FocusFiles:
		return ToastCopiedFilePath
	case FocusThread:
		return ToastCopiedThreadPermalink
	case FocusMain:
		return ToastCopiedFilePath
	default:
		return ToastCopiedPRURL
	}
}
