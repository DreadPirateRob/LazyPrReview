package ui

import (
	"math"
	"strings"
)

// ComputeLayout derives pixel-exact dimensions from the current model state.
//
// Rules (SPEC.md §4):
//   - side panel width = max(20, round(width × sidePanelWidth))
//   - portrait mode when width < 90
//   - command log height = configured size when visible, else 0
//   - hint bar height = 1 when ShowBottomLine, else 0
//
// All pane heights (PanelHeights, MainHeight, CommandLogHeight) are OUTER
// box heights: each pane renders as a bordered box, so 2 rows / 2 cols of
// every pane are border. MainWidth is whatever remains after the side panel
// column is reserved; columns sit border-to-border with no gutter.
func ComputeLayout(m Model) Layout {
	w, h := m.Width, m.Height

	portrait := w < 90

	widthFraction := m.Config.GUI.SidePanelWidth
	if m.ScreenMode == ScreenHalf {
		widthFraction = 0.5
	}
	if m.ScreenMode == ScreenFullscreen {
		widthFraction = 0
	}
	spw := int(math.Round(float64(w) * widthFraction))
	if m.ScreenMode != ScreenFullscreen && spw < 20 {
		spw = 20
	}
	if spw > w {
		spw = w
	}

	mainW := w - spw
	if mainW < 0 {
		mainW = 0
	}

	cmdLogH := 0
	if m.CommandLogOn {
		cmdLogH = m.Config.GUI.CommandLogSize
	}

	hintH := 0
	if m.Config.GUI.ShowBottomLine {
		hintH = 1
	}

	// Reserve rows: header line, hint bar, and the command-log box.
	usableH := h - 1 - hintH - cmdLogH
	if usableH < 0 {
		usableH = 0
	}

	panelHeights := [5]int{}
	mainH := usableH
	if m.ScreenMode != ScreenFullscreen {
		if portrait {
			// Reserve a minimum slice for Main so it never collapses to
			// zero rows; it also receives the flooring leftovers.
			mainMin := 3
			if usableH < mainMin {
				mainMin = usableH
			}
			mainH = mainMin + allocatePanelHeights(&panelHeights, usableH-mainMin, focusedPanelIndex(m.CurrentFocus()), focusedContentHeight(m))
		} else {
			// The three browsers stay EXACTLY equal, and Status/Checks are pinned to
			// their collapsed height unless focused — so a pool that is not divisible
			// by three cannot be fully consumed. Main takes the column's ACTUAL total
			// rather than the whole budget, so both columns end on the same row: the
			// spare rows become a margin above the hint bar instead of a notch under
			// the left column.
			allocatePanelHeights(&panelHeights, usableH, focusedPanelIndex(m.CurrentFocus()), focusedContentHeight(m))
			mainH = 0
			for _, ph := range panelHeights {
				mainH += ph
			}
			if mainH > usableH {
				mainH = usableH // tiny terminals: never overflow the frame
			}
		}
	}

	return Layout{
		Width:            w,
		Height:           h,
		Portrait:         portrait,
		SidePanelWidth:   spw,
		MainWidth:        mainW,
		CommandLogHeight: cmdLogH,
		HintHeight:       hintH,
		MainHeight:       mainH,
		PanelHeights:     panelHeights,
	}
}

// collapsiblePanel reports whether a left-column panel collapses to a boxed
// one-liner when unfocused. Status (0) and Checks (4) are small/ancillary, so
// they shrink to a summary and hand their space to the browser panels.
func collapsiblePanel(i int) bool { return i == 0 || i == 4 }

// focusedContentHeight reports how many content rows the focused collapsible
// panel needs to show all of its data. Zero for every other focus, since only
// Status and Checks grow to fit. Both views are pure functions of the model and
// never consult the layout, so measuring them here cannot recurse.
func focusedContentHeight(m Model) int {
	switch focusedPanelIndex(m.CurrentFocus()) {
	case 0:
		return countContentLines(StatusView(m))
	case 4:
		return countContentLines(ChecksView(m))
	}
	return 0
}

func countContentLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// allocatePanelHeights fills out[] with outer box heights for the five
// left-column panels.
//
// Status and Checks are pinned to a collapsed one-line box unless focused, in
// which case they grow to fit their own content (focusedContentH rows) — capped
// so the browsers never fall below their minimum. The browser panels (PRs,
// Files, Threads) then split what is left EQUALLY: their height never depends on
// focus, so the column does not reflow as focus moves. Returns any leftover row
// for the caller to place.
func allocatePanelHeights(out *[5]int, total int, focused int, focusedContentH int) int {
	if total <= 0 {
		return 0
	}
	const collapsedH = 3 // border + 1 summary line + border
	const browserMin = 3

	fixed := 0
	flexible := make([]int, 0, len(out))
	for i := range out {
		if collapsiblePanel(i) {
			out[i] = collapsedH
			if i == focused {
				// Grow to fit the panel's own rows, but never starve the rest: the
				// other collapsible keeps its box (collapsedH) and each of the three
				// browsers keeps browserMin.
				want := focusedContentH + 2 // borders
				lim := total - collapsedH - browserMin*(len(out)-2)
				if want > lim {
					want = lim
				}
				if want > collapsedH {
					out[i] = want
				}
			}
			fixed += out[i]
		} else {
			out[i] = 0
			flexible = append(flexible, i)
		}
	}

	flex := total - fixed
	// Too tight to honor the fixed collapsibles: fall back to an even split
	// so no panel is starved to zero rows.
	if flex < len(flexible) {
		return allocateEven(out, total)
	}

	minHeight := 3
	if flex < len(flexible)*minHeight {
		minHeight = 1
	}
	share := flex / len(flexible)
	used := 0
	for _, i := range flexible {
		h := share
		if h < minHeight {
			h = minHeight
		}
		out[i] = h
		used += h
	}
	for used > flex {
		progress := false
		for _, i := range flexible {
			if used <= flex {
				break
			}
			if out[i] > minHeight {
				out[i]--
				used--
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return maxInt(0, flex-used)
}

// allocateEven splits total evenly across all five panels, min 1 row each.
// Used only when the terminal is too short for the collapsible layout; heights
// are focus-independent, so the column never reflows as focus moves.
func allocateEven(out *[5]int, total int) int {
	share := total / len(out)
	used := 0
	for i := range out {
		h := share
		if h < 1 {
			h = 1
		}
		out[i] = h
		used += h
	}
	for used > total {
		progress := false
		for i := range out {
			if used <= total {
				break
			}
			if out[i] > 1 {
				out[i]--
				used--
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return maxInt(0, total-used)
}

func focusedPanelIndex(ctx FocusContext) int {
	switch ctx {
	case FocusStatus:
		return 0
	case FocusPRs:
		return 1
	case FocusFiles:
		return 2
	case FocusThreads, FocusThread:
		return 3
	case FocusChecks:
		return 4
	default:
		return -1
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
