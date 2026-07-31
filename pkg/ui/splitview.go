package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
)

// splitMinWidth is the narrowest Main content width that can host two code columns.
// Below it each side would hold barely twenty columns of code, where wrapping and
// clipping cost more than the alignment gains, so the toggle refuses and says why
// rather than rendering something unreadable.
const splitMinWidth = 80

// Column furniture, in cells: a right-aligned line number plus a space, then the
// change marker. Subtracted from each side to get its code budget.
const (
	splitGutterWidth = 5
	splitMarkerWidth = 2
)

// buildSplitLines renders one file as side-by-side columns: the old revision on the
// left, the new on the right.
//
// This is a READ-ONLY view. Callers pair it with MainFileIndex = -1, the existing
// contract for Main content that is not a single addressable PR file (commit-scoped
// and directory-aggregate diffs use the same sentinel), which makes every
// line-scoped action inert through the guards those views already rely on. Inline
// thread blocks are deliberately absent: refreshMainDiff bails on that sentinel, so
// folding and thread focus would only half-work here. Threads stay in the unified
// view and panel 4.
func buildSplitLines(m Model, idx, width int) []string {
	if idx < 0 || idx >= len(m.DiffFiles) {
		return nil
	}
	file := m.DiffFiles[idx]
	hl := highlightedDiffLines(file)
	sideW := (width - 1) / 2 // one column for the separator
	sep := diffMetaStyle.Render("│")

	rows := diff.SplitRows(file)
	out := make([]string, 0, len(rows)+1)
	out = append(out, diffHeaderStyle.Render("File: "+file.Path+"  (side-by-side)"))

	for _, row := range rows {
		if row.FullWidth {
			if row.Left == nil {
				continue
			}
			if row.Left.Kind == diff.LineKindHunkHeader {
				out = append(out, diffHunkStyle.Render(row.Left.Text))
			} else {
				out = append(out, diffMetaStyle.Render(row.Left.Text))
			}
			continue
		}
		left := renderSplitSide(row.Left, splitCode(hl, row.Left), sideW, true)
		right := renderSplitSide(row.Right, splitCode(hl, row.Right), sideW, false)
		out = append(out, left+sep+right)
	}
	return out
}

// splitCode returns the pre-highlighted code for one side, or "" when the side is
// filler or the file had no lexer.
func splitCode(hl []string, line *diff.RenderedLine) string {
	if line == nil || line.RenderIndex < 0 || line.RenderIndex >= len(hl) {
		return ""
	}
	return hl[line.RenderIndex]
}

// renderSplitSide draws one column: line number, change marker, then the code, tinted
// by the band when the line is an addition or a deletion. A nil line is filler — the
// revisions have unequal numbers of changed lines there — and renders as blank space
// with no band, so absence reads as absence rather than as unchanged code.
func renderSplitSide(line *diff.RenderedLine, code string, sideW int, old bool) string {
	if sideW <= 0 {
		return ""
	}
	if line == nil {
		return strings.Repeat(" ", sideW)
	}

	no := line.NewNo
	if old {
		no = line.OldNo
	}
	gutter := diffMetaStyle.Render(fmt.Sprintf("%4s ", diffLineNo(no)))

	body := code
	if body == "" {
		body = line.Text
	}
	budget := sideW - splitGutterWidth - splitMarkerWidth
	if budget < 1 {
		return padRight(fitWidth(gutter, sideW), sideW)
	}
	body = padRight(fitWidth(body, budget), budget)

	switch line.Kind {
	case diff.LineKindAdd:
		return diffBand(diffAddBandStyle, gutter+diffAddStyle.Render("+ ")+body, sideW)
	case diff.LineKindDel:
		return diffBand(diffDelBandStyle, gutter+diffDelStyle.Render("- ")+body, sideW)
	default:
		return gutter + "  " + body
	}
}

// splitFits reports whether the pane can host the side-by-side view.
func splitFits(width int) bool { return width >= splitMinWidth }

// renderDiffInMain draws one file into Main, honouring the sticky side-by-side
// preference. Every file-BROWSING path routes through here — opening a file, `[`/`]`,
// and the Files panel following its cursor — which is what makes the mode survive a
// focus change instead of snapping back to unified on the next render.
//
// Thread positioning deliberately does NOT route through here: showThreadInMain
// calls setMainDiff directly, because a thread has no side-by-side representation
// and landing the cursor on a row that does not exist would be worse than the mode
// briefly giving way.
//
// diffIdx indexes DiffFiles, the same space MainFileIndex uses.
func renderDiffInMain(m Model, diffIdx int) Model {
	if diffIdx < 0 || diffIdx >= len(m.DiffFiles) {
		return m
	}
	// A pane too narrow for two columns silently renders unified: this runs on every
	// file change, so a toast here would fire repeatedly. The explicit toggle is
	// where the reason gets reported.
	if m.DiffSplit && m.Config.GUI.DiffPager == "" && splitFits(mainContentWidth(m)) {
		width := mainContentWidth(m)
		path := m.DiffFiles[diffIdx].Path
		m = setMainLines(m, buildSplitLines(m, diffIdx, width))
		m.MainSplitPath = path // after setMainLines, which clears it
		m.MainFileIndex = -1   // read-only view: no line anchors, per the shared contract
		return m
	}
	m.MainFileIndex = diffIdx
	return setMainDiff(m, diffIdx)
}

// toggleSplitView flips the sticky side-by-side preference and re-renders the file
// on screen. Turning it off restores the unified render, and with it MainFileIndex
// and every line-scoped action.
func toggleSplitView(m Model) (Model, tea.Cmd) {
	if m.MainMode != MainDiff || m.PRDetail == nil {
		return m, nil
	}
	if m.Config.GUI.DiffPager != "" {
		// The pager owns its own layout; there is nothing to pair up.
		m.Toast = NewToast(ToastSplitUnavailablePager)
		return m, nil
	}

	// Which file is on screen: the split view records its path, the unified view its
	// index. Toggling acts on that file whichever way round it is.
	idx := m.MainFileIndex
	if m.MainSplitPath != "" {
		idx = findDiffFileIndexByPath(m.DiffFiles, m.MainSplitPath)
	}
	if idx < 0 || idx >= len(m.DiffFiles) {
		m.Toast = NewToast(ToastSplitNeedsFile)
		return m, nil
	}

	if m.MainSplitPath == "" && !splitFits(mainContentWidth(m)) {
		m.Toast = NewToast(ToastSplitTooNarrow)
		return m, nil
	}

	// The toggle acts on WHAT IS ON SCREEN, not on the preference. After a thread
	// jump has forced unified, `|` therefore puts side-by-side back rather than
	// switching the preference off and changing nothing visible — a toggle that
	// leaves the frame identical is indistinguishable from a broken key.
	showingSplit := m.MainSplitPath != ""
	m.DiffSplit = !showingSplit
	m.MainCursor = 0
	m.MainScroll = 0
	m = renderDiffInMain(m, idx)
	m.MainMode = MainDiff
	return m, nil
}
