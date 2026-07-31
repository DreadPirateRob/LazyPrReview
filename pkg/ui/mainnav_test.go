package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
)

// ── 0 focuses Main without repainting it ──────────────────────────────────────

func TestZeroFallsBackToOverviewWhenMainEmpty(t *testing.T) {
	m := seededModel(t)
	m.MainLines = nil // never populated
	m.FocusStack = []FocusContext{FocusFiles}

	updated, _ := m.Update(tea.KeyPressMsg{Text: "0", Code: '0'})
	got := updated.(Model)

	if got.CurrentFocus() != FocusMain {
		t.Fatalf("0 should focus Main, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainOverview || len(got.MainLines) == 0 {
		t.Fatalf("with nothing to show, 0 should fall back to the overview, got mode %q", got.MainMode)
	}
}

func TestZeroKeepsMainCursorAndScroll(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0)
	m.MainCursor, m.MainScroll = 4, 2
	m = m.PushFocus(FocusFiles)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "0", Code: '0'})
	got := updated.(Model)

	if got.MainCursor != 4 || got.MainScroll != 2 {
		t.Fatalf("0 must not move the reader: cursor=%d scroll=%d", got.MainCursor, got.MainScroll)
	}
}

// ── [ / ] in Main move the Files row selection ────────────────────────────────

func TestBracketNavSyncsFilesCursor(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0) // foo.txt
	if m.FilesPanel.Cursor != 0 {
		t.Fatalf("precondition: expected Files cursor on the first file, got %d", m.FilesPanel.Cursor)
	}

	next, _ := UpdateMain(m, tea.KeyPressMsg{Text: "]", Code: ']'})
	if next.MainFileIndex != 1 {
		t.Fatalf("] should advance the Main file, got %d", next.MainFileIndex)
	}
	if next.FilesPanel.Cursor != 1 {
		t.Fatalf("] should move the Files row selection to match, got %d", next.FilesPanel.Cursor)
	}

	back, _ := UpdateMain(next, tea.KeyPressMsg{Text: "[", Code: '['})
	if back.MainFileIndex != 0 {
		t.Fatalf("[ should go back a file, got %d", back.MainFileIndex)
	}
	if back.FilesPanel.Cursor != 0 {
		t.Fatalf("[ should move the Files row selection back, got %d", back.FilesPanel.Cursor)
	}
}

func TestThreadJumpSyncsFilesCursor(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0) // foo.txt, Files cursor 0

	target := -1
	for i, at := range m.UnresolvedThreadIndex {
		if at.Anchored && at.FilePath == "bar.txt" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Skip("fixture has no anchored thread in bar.txt")
	}

	got := jumpToAnchoredThread(m, m.UnresolvedThreadIndex[target], false)
	if got.FilesPanel.Cursor != 1 {
		t.Fatalf("jumping to a thread in another file should move the Files row, got %d", got.FilesPanel.Cursor)
	}
}

// ── space on Main's File: header toggles viewed ───────────────────────────────

func TestSpaceOnFileHeaderTogglesViewed(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0) // the cursor lands on the File: header row
	if m.MainCursor != 0 {
		t.Fatalf("precondition: expected the cursor on the header row, got %d", m.MainCursor)
	}
	if m.PRDetail.Files[0].ViewerViewedState == "VIEWED" {
		t.Fatal("precondition: fixture file should start unviewed")
	}

	got, cmd := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space on the header row should dispatch a viewed toggle")
	}
	if got.PRDetail.Files[0].ViewerViewedState != "VIEWED" {
		t.Fatalf("viewed state should flip optimistically, got %q", got.PRDetail.Files[0].ViewerViewedState)
	}
	if got.Activity == "" {
		t.Fatal("expected activity while the toggle is in flight")
	}
}

func TestSpaceOffFileHeaderDoesNothing(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0)
	m.MainCursor = 2 // a code row, not the header

	got, cmd := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd != nil {
		t.Fatal("space off the header row must not toggle viewed")
	}
	if got.PRDetail.Files[0].ViewerViewedState == "VIEWED" {
		t.Fatal("viewed state must not change")
	}
}

func TestSpaceOnHeaderSkippedUnderPager(t *testing.T) {
	m := seededModel(t)
	m.Config.GUI.DiffPager = "delta --paging=never"
	m = openFileInMain(m, 0)
	m.MainCursor = 0

	if _, cmd := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeySpace}); cmd != nil {
		t.Fatal("an external pager has no header-row contract, so space must not toggle")
	}
}

func TestSpaceInOverviewDoesNothing(t *testing.T) {
	m := seededModel(t)
	m = showPROverview(m)
	m.MainCursor = 0

	if _, cmd := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeySpace}); cmd != nil {
		t.Fatal("space in the overview must not toggle viewed")
	}
}

func TestSpaceInCommitDiffDoesNothing(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0)
	m.MainFileIndex = -1 // commit-diff sentinel
	m.MainCursor = 0

	if _, cmd := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeySpace}); cmd != nil {
		t.Fatal("a commit-scoped diff is not a PR file, so space must not toggle")
	}
}

func TestSpaceWithoutPRDetailDoesNothing(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.MainMode = MainDiff
	if _, cmd := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeySpace}); cmd != nil {
		t.Fatal("no PR detail means nothing to toggle")
	}
}
