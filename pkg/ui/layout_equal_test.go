package ui

import "testing"

// The browser panels must be focus-invariant: focusing one of them may not
// resize any of them. Regression guard for the accordion behavior that made the
// whole left column reflow on every focus change.
func TestBrowserPanelsShareHeightAcrossFocus(t *testing.T) {
	var first [5]int
	for n, focus := range map[string]FocusContext{
		"PRs":     FocusPRs,
		"Files":   FocusFiles,
		"Threads": FocusThreads,
	} {
		l := ComputeLayout(landscapeModel(focus))
		if l.PanelHeights[1] != l.PanelHeights[2] || l.PanelHeights[2] != l.PanelHeights[3] {
			t.Fatalf("%s focused: browsers must share one height, got PRs=%d Files=%d Threads=%d",
				n, l.PanelHeights[1], l.PanelHeights[2], l.PanelHeights[3])
		}
		if first[1] == 0 {
			first = l.PanelHeights
			continue
		}
		for i := 1; i <= 3; i++ {
			if l.PanelHeights[i] != first[i] {
				t.Fatalf("%s focused: panel %d changed height to %d (want %d) — focus must not resize browsers",
					n, i+1, l.PanelHeights[i], first[i])
			}
		}
	}
}

// Focusing Status or Checks still expands that panel (they collapse to a
// one-liner when unfocused), which shrinks the pool the browsers divide — but
// the browsers must still end up equal to each other.
func TestBrowsersStayEqualWhenCollapsibleFocused(t *testing.T) {
	for _, focus := range []FocusContext{FocusStatus, FocusChecks} {
		l := ComputeLayout(landscapeModel(focus))
		if l.PanelHeights[1] != l.PanelHeights[2] || l.PanelHeights[2] != l.PanelHeights[3] {
			t.Fatalf("%s focused: browsers must stay equal, got PRs=%d Files=%d Threads=%d",
				focus, l.PanelHeights[1], l.PanelHeights[2], l.PanelHeights[3])
		}
	}
}

// Both columns must terminate on the same row. Status/Checks are pinned at 3 and
// the browsers stay exactly equal, so the pool is often not fully consumable —
// the spare rows become a margin above the hint bar rather than a notch under the
// left column, which means Main matches the side column exactly.
func TestColumnsTerminateTogether(t *testing.T) {
	for h := 20; h <= 60; h++ {
		m := landscapeModel(FocusFiles)
		m.Height = h
		l := ComputeLayout(m)
		sum := 0
		for _, ph := range l.PanelHeights {
			sum += ph
		}
		if sum != l.MainHeight {
			t.Fatalf("height %d: side column is %d rows but Main is %d — columns must end on the same row",
				h, sum, l.MainHeight)
		}
	}
}
