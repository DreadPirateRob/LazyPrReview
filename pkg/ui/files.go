package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

// visibleFileIndices returns indices into PRDetail.Files whose path matches
// the Files panel filter.
func visibleFileIndices(m Model) []int {
	if m.PRDetail == nil {
		return nil
	}
	out := make([]int, 0, len(m.PRDetail.Files))
	for i, f := range m.PRDetail.Files {
		if panelFilterMatch(m, f.Path, m.FilesPanel.Filter) {
			out = append(out, i)
		}
	}
	return out
}

// filesTitle composes the Files panel border title: the tree/flat mode plus
// the filter indicator.
func filesTitle(m Model) string {
	mode := "tree"
	if m.FilesFlat {
		mode = "flat"
	}
	title := fmt.Sprintf("Files [%s]", mode)
	if m.PRDetail != nil && len(m.PRDetail.Files) > 0 {
		viewed := 0
		for _, f := range m.PRDetail.Files {
			if f.ViewerViewedState == "VIEWED" {
				viewed++
			}
		}
		title += fmt.Sprintf(" %d/%d viewed", viewed, len(m.PRDetail.Files))
	}
	return title + filterSuffix(m, FocusFiles, m.FilesPanel)
}

// visibleFileRows builds the ordered rows for the Files panel: flat mode yields
// one row per filtered file; tree mode interleaves directory rows. Nil-safe.
func visibleFileRows(m Model) []fileRow {
	if m.PRDetail == nil {
		return nil
	}
	include := visibleFileIndices(m)
	if m.FilesFlat {
		return buildFlatRows(m.PRDetail.Files, include)
	}
	return buildTreeRows(m.PRDetail.Files, include, m.FilesCollapsed)
}

// clampCursor clamps p into [0, n-1], returning 0 when n <= 0.
func clampCursor(p, n int) int {
	if n <= 0 {
		return 0
	}
	if p >= n {
		return n - 1
	}
	return p
}

// cowToggleCollapsed returns m with path toggled in FilesCollapsed. A fresh map
// is always allocated so that the caller's Model copy never aliases another's.
func cowToggleCollapsed(m Model, path string) Model {
	next := make(map[string]bool, len(m.FilesCollapsed)+1)
	for k, v := range m.FilesCollapsed {
		next[k] = v
	}
	if next[path] {
		delete(next, path)
	} else {
		next[path] = true
	}
	m.FilesCollapsed = next
	return m
}

// FilesView renders the Files panel rows with a cursor prefix on each line.
// width is the full panel width including borders; budget = width-4 (border 2 +
// cursor prefix 2). Pass width=0 for headless rendering (no truncation).
func FilesView(m Model, width int) string {
	rows := visibleFileRows(m)
	var sb strings.Builder
	budget := width - 4
	if width <= 0 {
		budget = 0
	}
	for i, row := range rows {
		cursor := " "
		if i == m.FilesPanel.Cursor {
			cursor = ">"
		}
		sb.WriteString(cursor + " " + renderFileRow(row, budget) + "\n")
	}
	return sb.String()
}

func UpdateFiles(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.PRDetail == nil {
		return m, nil
	}
	ks, bound := m.resolveKey(msg)
	if !bound {
		return m, nil
	}
	switch ks {
	case "`":
		// Preserve the selected file's position across a tree<->flat mode change:
		// remember the FileIndex at the cursor (or -1 for a dir), flip the mode,
		// then relocate that file in the new row layout.
		rows := visibleFileRows(m)
		curFileIndex := -1
		if m.FilesPanel.Cursor >= 0 && m.FilesPanel.Cursor < len(rows) {
			curFileIndex = rows[m.FilesPanel.Cursor].FileIndex
		}
		m.FilesFlat = !m.FilesFlat
		newRows := visibleFileRows(m)
		m.FilesPanel.Cursor = clampCursor(m.FilesPanel.Cursor, len(newRows))
		if curFileIndex >= 0 {
			for i, r := range newRows {
				if r.FileIndex == curFileIndex {
					m.FilesPanel.Cursor = i
					break
				}
			}
		}
		return m, nil
	case "/":
		m = startPanelFilter(m, &m.FilesPanel)
		return m, nil
	case "-":
		include := visibleFileIndices(m)
		dirs := allDirPaths(m.PRDetail.Files, include)
		next := make(map[string]bool, len(dirs))
		for _, d := range dirs {
			next[d] = true
		}
		m.FilesCollapsed = next
		m.FilesPanel.Cursor = clampCursor(m.FilesPanel.Cursor, len(visibleFileRows(m)))
		return m, nil
	case "=":
		m.FilesCollapsed = nil
		m.FilesPanel.Cursor = clampCursor(m.FilesPanel.Cursor, len(visibleFileRows(m)))
		return m, nil
	case "|":
		return toggleSplitView(m)
	}
	rows := visibleFileRows(m)
	if len(rows) == 0 {
		return m, nil
	}
	switch ks {
	case "j", "down":
		if m.FilesPanel.Cursor < len(rows)-1 {
			m.FilesPanel.Cursor++
			m = followFilesSelection(m)
		}
	case "k", "up":
		if m.FilesPanel.Cursor > 0 {
			m.FilesPanel.Cursor--
			m = followFilesSelection(m)
		}
	case "v":
		if m.FilesRangeActive {
			m.FilesRangeActive = false
		} else {
			m.FilesRangeActive = true
			m.FilesRangeStart = m.FilesPanel.Cursor
		}
	case "enter":
		row := rows[min(m.FilesPanel.Cursor, len(rows)-1)]
		if row.IsDir {
			m = cowToggleCollapsed(m, row.Path)
			m.FilesPanel.Cursor = clampCursor(m.FilesPanel.Cursor, len(visibleFileRows(m)))
			return m, nil
		}
		m = openFileInMain(m, row.FileIndex)
		return m, nil
	case "c":
		row := rows[min(m.FilesPanel.Cursor, len(rows)-1)]
		if row.IsDir {
			return m, nil
		}
		path := row.Path
		m = openComposer(m, composeState{
			Kind: composeComment, Required: true, SubjectType: "FILE",
			Title: "Comment on " + path + " (file-level) — ctrl+s submit",
			Path:  path,
		})
		return m, nil
	case "t":
		row := rows[min(m.FilesPanel.Cursor, len(rows)-1)]
		if row.IsDir {
			return m, nil
		}
		idx := firstThreadForFile(m, row.Path)
		if idx < 0 {
			m.Toast = NewToast(ToastNoUnresolvedThreadInFile)
			return m, nil
		}
		m = jumpToAnchoredThread(m, m.UnresolvedThreadIndex[idx], false)
		return m, nil
	case "space":
		row := rows[min(m.FilesPanel.Cursor, len(rows)-1)]
		if row.IsDir {
			indices := filesUnderDir(m, row.Path)
			if len(indices) == 0 {
				return m, nil
			}
			m.FilesRangeActive = false
			return toggleViewedFiles(m, indices)
		}
		indices := selectedFileIndices(m)
		if len(indices) == 0 {
			return m, nil
		}
		m.FilesRangeActive = false
		return toggleViewedFiles(m, indices)
	}
	return m, nil
}

// toggleViewedFiles flips the viewed state of the given PRDetail.Files indices,
// painting optimistically and returning the command that persists it; the result
// handler rolls back any failures. Shared by the Files panel and Main, so both
// entry points behave identically.
func toggleViewedFiles(m Model, indices []int) (Model, tea.Cmd) {
	if m.PRDetail == nil || len(indices) == 0 {
		return m, nil
	}
	targetViewed := m.PRDetail.Files[indices[0]].ViewerViewedState != "VIEWED"
	newState := "UNVIEWED"
	if targetViewed {
		newState = "VIEWED"
	}
	previous := map[int]string{}
	for _, idx := range indices {
		previous[idx] = m.PRDetail.Files[idx].ViewerViewedState
		m.PRDetail.Files[idx].ViewerViewedState = newState
	}
	m.Activity = "Updating viewed state…"
	return m, toggleViewedCmd(m.Forge, m.PRDetail.ID, m.PRDetail.Files, indices, targetViewed, previous)
}

// prFileIndexForMain maps the file Main is currently showing back to its index in
// PRDetail.Files, or -1 when Main isn't on a PR file (overview, commit diff).
func prFileIndexForMain(m Model) int {
	if m.PRDetail == nil || m.MainFileIndex < 0 || m.MainFileIndex >= len(m.DiffFiles) {
		return -1
	}
	path := m.DiffFiles[m.MainFileIndex].Path
	for i, f := range m.PRDetail.Files {
		if f.Path == path {
			return i
		}
	}
	return -1
}

// syncFilesCursorToMain moves the Files row selection onto the file Main shows, so
// the panel and the diff never disagree after [ / ] navigation. A file filtered
// out of the panel leaves the cursor alone.
func syncFilesCursorToMain(m Model) Model {
	idx := prFileIndexForMain(m)
	if idx < 0 {
		return m
	}
	rows := visibleFileRows(m)
	for pos, r := range rows {
		if r.FileIndex == idx {
			m.FilesPanel.Cursor = pos
			break
		}
	}
	return m
}

// setMainFile points Main at a diff-file index, re-rendering it and moving the
// Files row selection to match. Used by every Main-driven file change ([ / ]) so
// the panel never disagrees with the diff on screen.
func setMainFile(m Model, diffIdx int) Model {
	if diffIdx < 0 || diffIdx >= len(m.DiffFiles) {
		return m
	}
	m = renderDiffInMain(m, diffIdx) // honours the sticky side-by-side preference
	m.MainMode = MainDiff
	m.MainCursor = 0
	m.MainScroll = 0
	return syncFilesCursorToMain(m)
}

// selectedFileIndices maps the cursor (or active range) from row positions to
// real PRDetail.Files indices, skipping dir rows (FileIndex < 0).
func selectedFileIndices(m Model) []int {
	rows := visibleFileRows(m)
	if len(rows) == 0 {
		return nil
	}
	clamp := func(p int) int {
		if p < 0 {
			return 0
		}
		if p > len(rows)-1 {
			return len(rows) - 1
		}
		return p
	}
	if !m.FilesRangeActive {
		r := rows[clamp(m.FilesPanel.Cursor)]
		if r.FileIndex < 0 {
			return nil
		}
		return []int{r.FileIndex}
	}
	start, end := clamp(m.FilesRangeStart), clamp(m.FilesPanel.Cursor)
	if start > end {
		start, end = end, start
	}
	out := make([]int, 0, end-start+1)
	for p := start; p <= end; p++ {
		if rows[p].FileIndex >= 0 {
			out = append(out, rows[p].FileIndex)
		}
	}
	return out
}

func toggleViewedCmd(forgeClient forge.Forge, prID string, files []domain.ChangedFile, indices []int, viewed bool, previous map[int]string) tea.Cmd {
	return func() tea.Msg {
		failed := []int{}
		for _, idx := range indices {
			if err := forgeClient.MarkFileViewed(context.Background(), prID, files[idx].Path, viewed); err != nil {
				failed = append(failed, idx)
			}
		}
		return viewedToggleResultMsg{Previous: previous, Failed: failed}
	}
}

func firstThreadForFile(m Model, path string) int {
	for i, at := range m.UnresolvedThreadIndex {
		if at.FilePath == path {
			return i
		}
	}
	return -1
}

// showFileInMain points the Main pane at a PR file's diff WITHOUT touching focus;
// ok reports whether the file resolved to a diff. Re-showing the file already
// displayed is a no-op, so Main keeps its cursor and scroll instead of jolting
// back to the top.
func showFileInMain(m Model, idx int) (Model, bool) {
	if m.PRDetail == nil || idx < 0 || idx >= len(m.PRDetail.Files) {
		return m, false
	}
	diffIdx := findDiffFileIndexByPath(m.DiffFiles, m.PRDetail.Files[idx].Path)
	if diffIdx < 0 {
		return m, false
	}
	// Already showing this file? Both representations count: the unified view records
	// the index, the side-by-side view the path. Without the second check the Files
	// panel would rebuild the same split view on every cursor bounce.
	if m.MainMode == MainDiff && m.MainDiff.Kind == mainDiffFile &&
		m.MainDiff.FileIndex == diffIdx && m.MainDiff.Split == wantSplit(m) {
		return m, true
	}
	m.MainCursor = 0
	m.MainScroll = 0
	m = renderDiffInMain(m, diffIdx) // honours the sticky side-by-side preference
	m.MainMode = MainDiff
	return m, true
}

// followFilesSelection keeps Main showing whatever the Files cursor is on: a
// file's diff, or every diff beneath a directory row.
//
// Skipped when an external diff pager is configured: that render spawns a process
// synchronously (3s cap), so following every keystroke would stall the UI — and a
// directory would spawn one per file. Pager users still get the diff on enter.
func followFilesSelection(m Model) Model {
	if m.Config.GUI.DiffPager != "" {
		return m
	}
	rows := visibleFileRows(m)
	if len(rows) == 0 {
		return m
	}
	row := rows[min(m.FilesPanel.Cursor, len(rows)-1)]
	if row.IsDir {
		m, _ = showDirInMain(m, row.Path)
		return m
	}
	m, _ = showFileInMain(m, row.FileIndex)
	return m
}

// filesUnderDir returns the PRDetail.Files indices beneath dirPath, restricted to
// the panel's visible (filtered) set so a directory acts on exactly the subtree
// its row represents — that row's own +N/-N and ✓ aggregates are computed from
// the same filtered set, so widening here would contradict what is on screen.
//
// The trailing separator is load-bearing: matching the bare path would also pull
// in sibling directories that merely share it as a prefix (pkg/uix for pkg/ui).
func filesUnderDir(m Model, dirPath string) []int {
	if m.PRDetail == nil || dirPath == "" {
		return nil
	}
	prefix := dirPath + "/"
	vis := visibleFileIndices(m)
	out := make([]int, 0, len(vis))
	for _, i := range vis {
		if strings.HasPrefix(m.PRDetail.Files[i].Path, prefix) {
			out = append(out, i)
		}
	}
	return out
}

// renderDirDiff concatenates the diffs of every file beneath dirPath into one
// scrollable view. Files with no parsed diff (binaries, renames without content)
// are skipped; nil, nil means there was nothing to show.
//
// Per-file blocks honour fold state (appendFileBlock) and the side-by-side
// preference (wantSplit). Every body row carries RenderIndex = -1 — the 1:1
// line↔Rendered invariant that enables thread anchors holds only for a single-file
// unified diff, and concatenation breaks it.
func renderDirDiff(m Model, dirPath string, indices []int) ([]string, []mainRow) {
	body := make([]string, 0, 128)
	bodyRows := make([]mainRow, 0, 128)
	add := func(s string, r mainRow) {
		body = append(body, s)
		bodyRows = append(bodyRows, r)
	}
	shown := 0
	width := mainContentWidth(m)
	for _, idx := range indices {
		diffIdx := findDiffFileIndexByPath(m.DiffFiles, m.PRDetail.Files[idx].Path)
		if diffIdx < 0 {
			continue
		}
		if shown > 0 {
			add("", mainRow{Kind: rowMeta, RenderIndex: -1})
		}
		appendFileBlock(m, m.DiffFiles[diffIdx], width, add)
		shown++
	}
	if shown == 0 {
		return nil, nil
	}
	head := fmt.Sprintf("Directory: %s/ — %d file", dirPath, shown)
	if shown != 1 {
		head += "s"
	}
	lines := make([]string, 0, len(body)+2)
	rows := make([]mainRow, 0, len(bodyRows)+2)
	lines = append(lines, diffHeaderStyle.Render(head), "")
	rows = append(rows, mainRow{Kind: rowMeta, RenderIndex: -1}, mainRow{Kind: rowMeta, RenderIndex: -1})
	lines = append(lines, body...)
	rows = append(rows, bodyRows...)
	return lines, rows
}

// appendFileBlock emits one file's header (with a fold caret and +N/-N counts) and,
// when expanded, its diff body. Side-by-side or unified is chosen by wantSplit(m).
//
// Every body row carries RenderIndex = -1: the 1:1 line↔Rendered[i] invariant that
// enables thread anchors and line comments holds ONLY for a single-file unified diff.
// Multi-file views are read-only by contract; a real index here would let thread and
// anchor lookups resolve to the wrong file.
func appendFileBlock(m Model, file diff.File, width int, add func(string, mainRow)) {
	expanded := fileExpanded(m, file.Path)
	caret := "▾"
	if !expanded {
		caret = "▸"
	}
	var adds, dels int
	for _, l := range file.Rendered {
		switch l.Kind {
		case diff.LineKindAdd:
			adds++
		case diff.LineKindDel:
			dels++
		}
	}
	hdr := caret + " " + file.Path
	if adds > 0 || dels > 0 {
		hdr += fmt.Sprintf("  +%d/-%d", adds, dels)
	}
	add(diffHeaderStyle.Render(hdr), mainRow{Kind: rowFileHeader, RenderIndex: -1, Key: fileFoldKey(file.Path)})
	if !expanded {
		return
	}
	if wantSplit(m) {
		for _, line := range buildSplitBodyLines(file, width) {
			add(line, mainRow{Kind: rowCode, RenderIndex: -1})
		}
	} else {
		hlLines := highlightedDiffLines(file)
		for i, line := range file.Rendered {
			if line.Kind == diff.LineKindFileHeader {
				// The caret header above already names the file; the raw `diff --git`
				// line would say it again. Only the single-file UNIFIED view keeps it —
				// its 1:1 line↔Rendered contract is load-bearing, and it never comes
				// through here.
				continue
			}
			add(renderDiffLine(line, hlLines[i], width), mainRow{Kind: rowCode, RenderIndex: -1})
		}
	}
}

// showDirInMain points Main at the aggregate diff of a directory subtree WITHOUT
// touching focus. Re-showing the same subtree is a no-op, so Main keeps its cursor
// and scroll instead of jolting back to the top.
//
// The identity includes the Files filter, not just the path: filesUnderDir is
// restricted to the visible set, so the same directory under a narrower filter is
// a genuinely different view and must re-render.
func showDirInMain(m Model, dirPath string) (Model, bool) {
	if m.MainMode == MainDiff && m.MainDiff.Kind == mainDiffDir &&
		m.MainDiff.DirPath == dirPath && m.MainDiff.DirFilter == m.FilesPanel.Filter &&
		m.MainDiff.Split == wantSplit(m) {
		return m, true
	}
	m, ok := renderDirInMain(m, dirPath)
	if !ok {
		return m, false
	}
	m.MainCursor = 0
	m.MainScroll = 0
	return m, true
}

// renderDirInMain draws a directory subtree's aggregate diff, honouring the sticky
// side-by-side preference and per-file fold state. Separate from showDirInMain so
// rerenderMainDiff can redraw the same subtree without resetting cursor and scroll.
func renderDirInMain(m Model, dirPath string) (Model, bool) {
	if m.PRDetail == nil || dirPath == "" {
		return m, false
	}
	lines, rows := renderDirDiff(m, dirPath, filesUnderDir(m, dirPath))
	if len(lines) == 0 {
		return m, false
	}
	m = setMainLines(m, lines)
	m.MainRows = rows // after setMainLines, which clears it
	m.MainDiff = mainDiffSource{
		Kind:      mainDiffDir,
		DirPath:   dirPath,
		DirFilter: m.FilesPanel.Filter,
		Split:     wantSplit(m),
	}
	m.MainFileIndex = -1 // not a single-PR-file view: no line anchors
	m.MainMode = MainDiff
	return m, true
}

func openFileInMain(m Model, idx int) Model {
	m, ok := showFileInMain(m, idx)
	if !ok {
		return m
	}
	return m.PushFocus(FocusMain)
}

func findDiffFileIndexByPath(files []diff.File, path string) int {
	for i, file := range files {
		if file.Path == path {
			return i
		}
	}
	return -1
}

var (
	// Built-in diff renderer styling, delta-ish: colored change lines with a
	// dim line-number gutter.
	diffHeaderStyle = lipgloss.NewStyle().Bold(true)
	diffHunkStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	diffAddStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	diffDelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	diffMetaStyle   = lipgloss.NewStyle().Faint(true)
)

// renderCommitDiff formats one commit's parsed diff into Main-pane lines plus the
// parallel row model, headed by the short SHA and message. Standalone — it does not
// touch m.DiffFiles or the PR-level thread anchors.
//
// Per-file blocks honour fold state and the side-by-side preference via
// appendFileBlock. Every body row carries RenderIndex = -1 (read-only view).
func renderCommitDiff(m Model, oid, headline string, files []diff.File, width int) ([]string, []mainRow) {
	short := oid
	if len(short) > 7 {
		short = short[:7]
	}
	lines := make([]string, 0, 64)
	rows := make([]mainRow, 0, 64)
	add := func(s string, r mainRow) {
		lines = append(lines, s)
		rows = append(rows, r)
	}
	add(diffHeaderStyle.Render(fmt.Sprintf("Commit %s  %s", short, headline)), mainRow{Kind: rowMeta, RenderIndex: -1})
	if len(files) == 0 {
		add(diffMetaStyle.Render("  (no changes)"), mainRow{Kind: rowMeta, RenderIndex: -1})
		return lines, rows
	}
	for _, f := range files {
		appendFileBlock(m, f, width, add)
	}
	return lines, rows
}

// renderDiffFile renders one diff file into Main-pane lines. With
// gui.diffPager configured the file's raw diff section is piped through the
// external command and shown verbatim; otherwise the built-in renderer draws
// a line-number gutter and colored add/del rows. The built-in layout is a
// hard contract: lines[0] is the file header and lines[1+i] maps 1:1 to
// file.Rendered[i] — thread anchors and line-scoped copy rely on it.
func renderDiffFile(m Model, idx int) []string {
	if idx < 0 || idx >= len(m.DiffFiles) {
		return nil
	}
	if m.Config.GUI.DiffPager != "" {
		if lines := renderDiffFileExternal(m, idx); lines != nil {
			return lines
		}
	}
	file := m.DiffFiles[idx]
	hlLines := highlightedDiffLines(file)
	width := mainContentWidth(m)
	lines := []string{diffHeaderStyle.Render("File: " + file.Path)}
	for i, line := range file.Rendered {
		lines = append(lines, renderDiffLine(line, hlLines[i], width))
	}
	return lines
}

// renderDiffLine draws one diff row: line-number gutter, change marker, then the
// code — syntax-highlighted when code is non-empty, raw otherwise.
//
// Added and deleted rows carry a background band across the FULL row, gutter
// included. Without it the only signal for which side a line is on is the single
// `+`/`-` glyph, which is far too little once the code itself is syntax-coloured;
// banding everything but the gutter would just move that notch one column over.
// The band is independent of highlighting — a file with no lexer still needs it.
func renderDiffLine(line diff.RenderedLine, code string, width int) string {
	switch line.Kind {
	case diff.LineKindHunkHeader:
		return diffHunkStyle.Render(line.Text)
	case diff.LineKindFileHeader:
		return diffMetaStyle.Render(line.Text)
	}
	gutter := diffMetaStyle.Render(fmt.Sprintf("%4s %4s │ ", diffLineNo(line.OldNo), diffLineNo(line.NewNo)))
	body := code
	if body == "" {
		body = line.Text
	}
	switch line.Kind {
	case diff.LineKindAdd:
		return diffBand(diffAddBandStyle, gutter+diffAddStyle.Render("+ ")+body, width)
	case diff.LineKindDel:
		return diffBand(diffDelBandStyle, gutter+diffDelStyle.Render("- ")+body, width)
	default:
		return gutter + "  " + body
	}
}

// diffBand lays a tint under a changed row across the pane width, so the whole line
// reads as changed instead of one coloured glyph. Falls back to the bare row when
// the width is unknown or too small to band (headless render, tiny pane).
func diffBand(style lipgloss.Style, content string, width int) string {
	if width <= 0 {
		return content
	}
	return selectionRow(style, padRight(fitWidth(content, width), width))
}

func diffLineNo(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func buildAnchors(detail *domain.PRDetail, files []diff.File) ([]diff.AnchoredThread, []diff.AnchoredThread) {
	if detail == nil {
		return nil, nil
	}
	return anchorsFromThreads(detail.Threads, files)
}

// anchorsFromThreads anchors an explicit thread set (server threads plus any
// optimistic draft overlay) against the diff and builds the unresolved index.
func anchorsFromThreads(threads []domain.Thread, files []diff.File) ([]diff.AnchoredThread, []diff.AnchoredThread) {
	anchored := diff.AnchorThreads(files, threads)
	index := diff.BuildUnresolvedIndex(files, anchored)
	return anchored, index
}

func applyViewedToggleResult(m Model, msg viewedToggleResultMsg) Model {
	if m.PRDetail == nil {
		return m
	}
	for _, idx := range msg.Failed {
		if prev, ok := msg.Previous[idx]; ok && idx >= 0 && idx < len(m.PRDetail.Files) {
			m.PRDetail.Files[idx].ViewerViewedState = prev
		}
	}
	if len(msg.Failed) == 1 {
		m.Toast = NewToast(ToastViewedToggleFailed)
	} else if len(msg.Failed) > 1 {
		m.Toast = NewToast(ToastViewedToggleFailedN(len(msg.Failed)))
	}
	return m
}

func filePathOrder(detail *domain.PRDetail) []string {
	if detail == nil {
		return nil
	}
	out := make([]string, len(detail.Files))
	for i, f := range detail.Files {
		out[i] = f.Path
	}
	sort.Strings(out)
	return out
}
