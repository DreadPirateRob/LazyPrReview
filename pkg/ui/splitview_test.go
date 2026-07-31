package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// splitModel returns a diff model wide enough to host the side-by-side view.
//
// The unified render is rebuilt AFTER the terminal size is set: diffModel builds it
// at zero width, and every diff render bakes in the width it was built at, so a
// baseline captured before the resize would never match what a restore produces.
func splitModel(t *testing.T) Model {
	t.Helper()
	m, _, _ := diffModel(t)
	m.PRDetail = &domain.PRDetail{Title: "t"}
	m.FocusStack = []FocusContext{FocusMain}
	m.Width, m.Height = 200, 44
	m = setMainDiff(m, m.MainFileIndex)
	if w := mainContentWidth(m); !splitFits(w) {
		t.Fatalf("fixture must be wide enough for split, content width %d", w)
	}
	return m
}

// addCodeRow returns the Main line index of an added code row. buildDiffRows tags
// every rendered line rowCode — headers included — and a header has no line numbers
// to anchor, so the first rowCode row is not a usable anchor target.
func addCodeRow(t *testing.T, m Model) int {
	t.Helper()
	rendered := m.DiffFiles[m.MainFileIndex].Rendered
	for i, r := range m.MainRows {
		if r.Kind != rowCode || r.RenderIndex < 0 || r.RenderIndex >= len(rendered) {
			continue
		}
		if rendered[r.RenderIndex].Kind == diff.LineKindAdd {
			return i
		}
	}
	t.Fatal("fixture needs an added code row")
	return -1
}

func TestSplitToggleRoundTripsToUnified(t *testing.T) {
	m := splitModel(t)
	unified := strings.Join(m.MainLines, "\n")
	file := m.DiffFiles[m.MainFileIndex].Path

	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if split.MainSplitPath != file {
		t.Fatalf("split view should record its file, got %q want %q", split.MainSplitPath, file)
	}
	if split.MainFileIndex != -1 {
		t.Fatalf("split view is read-only and must drop the file anchor, got %d", split.MainFileIndex)
	}
	if strings.Join(split.MainLines, "\n") == unified {
		t.Fatal("split view must render differently from unified")
	}

	back, _ := UpdateMain(split, tea.KeyPressMsg{Text: "|", Code: '|'})
	if back.MainSplitPath != "" {
		t.Fatalf("leaving split must clear the recorded file, got %q", back.MainSplitPath)
	}
	if back.MainFileIndex != m.MainFileIndex {
		t.Fatalf("leaving split must restore the file anchor, got %d want %d", back.MainFileIndex, m.MainFileIndex)
	}
	if strings.Join(back.MainLines, "\n") != unified {
		t.Fatal("leaving split must restore the unified render of the same file")
	}
}

// Same contract the commit-scoped and directory-aggregate views rely on: with no
// file anchor, line-scoped actions are inert. This is what makes Scope A safe
// without touching the 40-odd RenderIndex consumers.
func TestSplitViewDisablesLineScopedActions(t *testing.T) {
	m := splitModel(t)
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	split.MainCursor = 3

	if afterC, _ := UpdateMain(split, tea.KeyPressMsg{Text: "c", Code: 'c'}); afterC.CurrentFocus() == FocusCompose {
		t.Fatal("c must not open a composer in the read-only split view")
	}
	if afterV, _ := UpdateMain(split, tea.KeyPressMsg{Text: "v", Code: 'v'}); mainRangeActive(afterV) {
		t.Fatal("v must not arm a range in the read-only split view")
	}
	if _, _, _, ok := anchorAt(split, split.MainCursor); ok {
		t.Fatal("split rows must not resolve to a line anchor")
	}
}

// Restoring unified must bring the line actions back, or the toggle is a trapdoor.
func TestLeavingSplitRestoresLineScopedActions(t *testing.T) {
	m := splitModel(t)
	addRow := addCodeRow(t, m)

	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	back, _ := UpdateMain(split, tea.KeyPressMsg{Text: "|", Code: '|'})
	back.MainCursor = addRow

	if _, _, _, ok := anchorAt(back, back.MainCursor); !ok {
		t.Fatal("unified rows must resolve to a line anchor again after leaving split")
	}
}

func TestSplitRefusedWhenTooNarrow(t *testing.T) {
	m := splitModel(t)
	m.Width = 60 // Main content drops below splitMinWidth
	if splitFits(mainContentWidth(m)) {
		t.Fatal("fixture should be too narrow")
	}

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if got.MainSplitPath != "" {
		t.Fatal("split must be refused when the pane cannot host two columns")
	}
	if got.Toast.Message != ToastSplitTooNarrow {
		t.Fatalf("the refusal must say why, got %q", got.Toast.Message)
	}
}

func TestSplitRefusedWithExternalPager(t *testing.T) {
	m := splitModel(t)
	m.Config.GUI.DiffPager = "delta --paging=never"

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if got.MainSplitPath != "" {
		t.Fatal("an external pager owns its own layout; split must be refused")
	}
	if got.Toast.Message != ToastSplitUnavailablePager {
		t.Fatalf("the refusal must say why, got %q", got.Toast.Message)
	}
}

// Every row is exactly the pane width, so the divider forms a straight column and
// the right-hand side starts at the same offset on every line.
func TestSplitRowsShareOneDividerColumn(t *testing.T) {
	m := splitModel(t)
	width := mainContentWidth(m)
	lines := buildSplitLines(m, m.MainFileIndex, width)
	if len(lines) < 3 {
		t.Fatalf("expected rendered split lines, got %d", len(lines))
	}

	sideW := (width - 1) / 2
	for i, line := range lines[1:] { // [0] is the file header
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "@@") || !strings.Contains(plain, "│") {
			continue // full-width header row
		}
		if w := ansi.StringWidth(line); w != sideW*2+1 {
			t.Errorf("row %d spans %d columns, want %d", i+1, w, sideW*2+1)
		}
		if col := strings.Index(plain, "│"); col != sideW {
			t.Errorf("row %d divider at column %d, want %d", i+1, col, sideW)
		}
	}
}
