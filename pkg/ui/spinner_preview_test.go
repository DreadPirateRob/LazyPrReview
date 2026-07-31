package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
)

// ── Composer target-line preview ──────────────────────────────────────────────

func TestComposerPreviewShowsTargetLine(t *testing.T) {
	m, addIdx, _ := diffModel(t)

	m.FocusStack = []FocusContext{FocusMain}
	m.MainCursor = addIdx

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "c", Code: 'c'})
	if got.CurrentFocus() != FocusCompose {
		t.Fatal("c should open the composer")
	}
	if len(got.Compose.Preview) != 1 {
		t.Fatalf("expected exactly the target line in the preview, got %d rows", len(got.Compose.Preview))
	}
	if !strings.Contains(ansi.Strip(got.Compose.Preview[0]), "new") {
		t.Fatalf("preview should show the line being commented on, got %q", got.Compose.Preview[0])
	}
	if !strings.Contains(ansi.Strip(ComposeView(got)), "new") {
		t.Fatalf("composer body should render the preview:\n%s", ComposeView(got))
	}
}

func TestComposerPreviewSpansRange(t *testing.T) {
	m, addIdx, _ := diffModel(t)

	m.MainRangeActive = true
	m.MainRangeStart = addIdx
	m.MainRangeGen = m.MainGen
	m.MainCursor = addIdx + 1 // extend one row down (same side)

	got, _ := openRangeComment(m)
	if got.CurrentFocus() != FocusCompose {
		t.Fatal("a same-side range should open the composer")
	}
	if len(got.Compose.Preview) != 2 {
		t.Fatalf("expected 2 preview rows for a 2-line range, got %d", len(got.Compose.Preview))
	}
}

func TestPreviewRowsElidesLongSelections(t *testing.T) {
	m := New(config.Default(), nil)
	m.MainMode = MainDiff
	m = setMainLines(m, make([]string, 30))

	rows := previewRows(m, 0, 19) // 20-row selection
	if len(rows) != 9 {
		t.Fatalf("expected 8 rows plus an elision marker, got %d", len(rows))
	}
	if !strings.Contains(rows[8], "more lines") {
		t.Fatalf("expected an elision marker, got %q", rows[8])
	}
}

func TestPreviewRowsIgnoresNonDiffModes(t *testing.T) {
	m := New(config.Default(), nil)
	m.MainMode = MainOverview
	m = setMainLines(m, []string{"overview", "text"})
	if rows := previewRows(m, 0, 1); rows != nil {
		t.Fatalf("overview text must never be shown as a comment target: %q", rows)
	}
}

// ── Loading spinner ───────────────────────────────────────────────────────────

func TestLoadingSpinnerFillsAndCenters(t *testing.T) {
	m := New(config.Default(), nil) // starts in the loading state
	m.Width, m.Height = 40, 9

	view := loadingView(m)
	if w, h := lipgloss.Width(view), lipgloss.Height(view); w != 40 || h != 9 {
		t.Fatalf("spinner should fill the terminal so it can center: got %dx%d", w, h)
	}
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[4], "Loading") {
		t.Fatalf("spinner should sit on the middle row (4 of 9): %q", lines[4])
	}
	// Horizontally centered: leading padding before the glyph.
	if !strings.HasPrefix(lines[4], " ") {
		t.Fatalf("spinner should be horizontally centered: %q", lines[4])
	}
}

func TestLoadingSpinnerAnimatesThenStops(t *testing.T) {
	m := New(config.Default(), nil)
	m.Width, m.Height = 40, 9
	first := ansi.Strip(loadingView(m))

	updated, cmd := m.Update(spinnerTickMsg{})
	g := updated.(Model)
	if g.SpinnerFrame != 1 {
		t.Fatalf("tick should advance the frame, got %d", g.SpinnerFrame)
	}
	if cmd == nil {
		t.Fatal("should keep ticking while still loading")
	}
	if ansi.Strip(loadingView(g)) == first {
		t.Fatal("spinner glyph should change between frames")
	}

	g.Loading = false
	if _, stop := g.Update(spinnerTickMsg{}); stop != nil {
		t.Fatal("should stop ticking once loading completes")
	}
}

func TestLoadingSpinnerUsesActivityLabel(t *testing.T) {
	m := New(config.Default(), nil)
	m.Width, m.Height = 60, 7
	m.Activity = "Loading PR detail…"
	if !strings.Contains(ansi.Strip(loadingView(m)), "Loading PR detail…") {
		t.Fatalf("spinner should surface the current activity:\n%s", loadingView(m))
	}
}

func TestLoadingSpinnerHeadlessFallback(t *testing.T) {
	m := New(config.Default(), nil) // no dimensions
	if got := loadingView(m); !strings.Contains(got, "Loading") {
		t.Fatalf("headless loading view should still say what it is: %q", got)
	}
}
