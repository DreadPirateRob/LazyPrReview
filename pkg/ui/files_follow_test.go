package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Moving the Files cursor must update the Main pane (section 0) to that file's
// diff, without stealing focus away from the Files panel.
func TestFilesCursorFollowsIntoMain(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusFiles}
	m.FilesPanel.Cursor = 0

	got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "j", Code: 'j'})

	if got.FilesPanel.Cursor != 1 {
		t.Fatalf("j should move the Files cursor, got %d", got.FilesPanel.Cursor)
	}
	if got.MainMode != MainDiff {
		t.Fatalf("Main should show the selected file's diff, got mode %q", got.MainMode)
	}
	want := findDiffFileIndexByPath(got.DiffFiles, got.PRDetail.Files[1].Path)
	if got.MainFileIndex != want {
		t.Fatalf("Main should follow the cursor to bar.txt: got index %d want %d", got.MainFileIndex, want)
	}
	if len(got.MainLines) == 0 {
		t.Fatal("Main lines should be populated for the followed file")
	}
	if got.CurrentFocus() != FocusFiles {
		t.Fatalf("following must not steal focus from Files, got %s", got.CurrentFocus())
	}
}

func TestFilesCursorFollowsBackUp(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusFiles}
	m.FilesPanel.Cursor = 1
	m = followFilesSelection(m) // showing bar.txt

	got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "k", Code: 'k'})

	want := findDiffFileIndexByPath(got.DiffFiles, got.PRDetail.Files[0].Path)
	if got.MainFileIndex != want {
		t.Fatalf("k should follow back to foo.txt: got index %d want %d", got.MainFileIndex, want)
	}
}

// Focusing the Files panel should immediately show the selected file in Main.
func TestFocusingFilesSyncsMain(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusPRs}
	m.FilesPanel.Cursor = 1

	updated, _ := m.Update(tea.KeyPressMsg{Text: "3", Code: '3'})
	got := updated.(Model)

	if got.CurrentFocus() != FocusFiles {
		t.Fatalf("3 should focus Files, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainDiff {
		t.Fatalf("focusing Files should sync Main to the selection, got mode %q", got.MainMode)
	}
	want := findDiffFileIndexByPath(got.DiffFiles, got.PRDetail.Files[1].Path)
	if got.MainFileIndex != want {
		t.Fatalf("Main should show the selected file: got index %d want %d", got.MainFileIndex, want)
	}
}

// An external diff pager renders synchronously (3s cap), so auto-follow is
// deliberately skipped there — it would stall the UI on every keystroke.
func TestFilesFollowSkippedUnderPager(t *testing.T) {
	m := seededModel(t)
	m.Config.GUI.DiffPager = "delta --paging=never"
	m.FocusStack = []FocusContext{FocusFiles}
	m.FilesPanel.Cursor = 0

	got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "j", Code: 'j'})

	if got.FilesPanel.Cursor != 1 {
		t.Fatalf("the cursor must still move under a pager, got %d", got.FilesPanel.Cursor)
	}
	if got.MainMode == MainDiff {
		t.Fatal("auto-follow must be skipped when an external diff pager is configured")
	}
}

// Re-showing the file already displayed must not jolt Main back to the top.
func TestShowFileInMainKeepsPositionWhenUnchanged(t *testing.T) {
	m := seededModel(t)
	m, ok := showFileInMain(m, 0)
	if !ok {
		t.Fatal("expected foo.txt to resolve to a diff")
	}
	m.MainCursor, m.MainScroll = 5, 3

	got, ok := showFileInMain(m, 0)
	if !ok {
		t.Fatal("re-showing the same file should still report ok")
	}
	if got.MainCursor != 5 || got.MainScroll != 3 {
		t.Fatalf("re-showing the same file must preserve position, got cursor=%d scroll=%d", got.MainCursor, got.MainScroll)
	}
}

// The refactor split focus out of the Main-population logic; enter must still
// move focus into Main.
func TestEnterStillFocusesMain(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusFiles}
	got, _ := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got.CurrentFocus() != FocusMain {
		t.Fatalf("enter should focus Main, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainDiff {
		t.Fatalf("enter should show the file diff, got mode %q", got.MainMode)
	}
}
