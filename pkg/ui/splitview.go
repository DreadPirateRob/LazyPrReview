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

// toggleSplitView flips the file on screen between unified and side-by-side.
//
// Leaving the view restores the unified render of the same file, and with it
// MainFileIndex and every line-scoped action.
func toggleSplitView(m Model) (Model, tea.Cmd) {
	if m.MainMode != MainDiff || m.PRDetail == nil {
		return m, nil
	}
	if m.Config.GUI.DiffPager != "" {
		// The pager owns its own layout; there is nothing to pair up.
		m.Toast = NewToast(ToastSplitUnavailablePager)
		return m, nil
	}

	if m.MainSplitPath != "" {
		// Back to unified. setMainDiff takes a DiffFiles index, the same space
		// MainFileIndex and findDiffFileIndexByPath use, so the restore stays in one
		// index space — showFileInMain would have wanted a PRDetail.Files index.
		// Restoring MainFileIndex is what switches every line-scoped action back on.
		if idx := findDiffFileIndexByPath(m.DiffFiles, m.MainSplitPath); idx >= 0 {
			m.MainFileIndex = idx
			m.MainCursor = 0
			m.MainScroll = 0
			m = setMainDiff(m, idx) // clears MainSplitPath through setMainLines
			m.MainMode = MainDiff
		}
		return m, nil
	}

	idx := m.MainFileIndex
	if idx < 0 || idx >= len(m.DiffFiles) {
		m.Toast = NewToast(ToastSplitNeedsFile)
		return m, nil
	}
	width := mainContentWidth(m)
	if !splitFits(width) {
		m.Toast = NewToast(ToastSplitTooNarrow)
		return m, nil
	}

	path := m.DiffFiles[idx].Path
	m = setMainLines(m, buildSplitLines(m, idx, width))
	m.MainSplitPath = path // after setMainLines, which clears it
	m.MainFileIndex = -1   // read-only view: no line anchors, per the shared contract
	m.MainMode = MainDiff
	m.MainCursor = 0
	m.MainScroll = 0
	return m, nil
}
