package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Moving the Threads cursor must update Main to that thread, the way the Files
// panel follows its selection — and must NOT steal focus from the panel.
func TestThreadsCursorFollowsIntoMain(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabAll
	m.FocusStack = []FocusContext{FocusThreads}
	rows := filteredThreadRows(m)
	if len(rows) < 2 {
		t.Fatalf("fixture needs at least two threads, got %d", len(rows))
	}

	got, _ := UpdateThreads(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if got.CurrentFocus() != FocusThreads {
		t.Fatalf("following must not steal focus, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainDiff {
		t.Fatalf("Main should show the thread's diff, got %q", got.MainMode)
	}
	want := rows[1].FilePath
	if got.MainFileIndex < 0 || got.DiffFiles[got.MainFileIndex].Path != want {
		t.Fatalf("Main should follow to %q, got index %d", want, got.MainFileIndex)
	}
	if id := threadIDAtMainCursor(got); id != rows[1].Thread.ID {
		t.Fatalf("Main cursor should address thread %q, got %q", rows[1].Thread.ID, id)
	}
}

func TestThreadsCursorFollowsBackUp(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabAll
	m.FocusStack = []FocusContext{FocusThreads}
	rows := filteredThreadRows(m)
	if len(rows) < 2 {
		t.Skip("need two threads")
	}
	m.ThreadsPanel.Cursor = 1
	m = followThreadsSelection(m)

	got, _ := UpdateThreads(m, tea.KeyPressMsg{Text: "k", Code: 'k'})
	if id := threadIDAtMainCursor(got); id != rows[0].Thread.ID {
		t.Fatalf("k should follow back to thread %q, got %q", rows[0].Thread.ID, id)
	}
}

// Switching tabs changes which threads are visible, so Main must follow the new
// selection rather than keep showing one that is no longer listed.
func TestThreadsTabSwitchFollows(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabUnresolved
	m.FocusStack = []FocusContext{FocusThreads}
	m.MainMode = MainOverview

	got, _ := UpdateThreads(m, tea.KeyPressMsg{Text: "]", Code: ']'})
	if got.ThreadTab == ThreadsTabUnresolved {
		t.Fatal("precondition: ] should have switched tabs")
	}
	if got.ThreadTab != ThreadsTabCommits && got.MainMode != MainDiff {
		t.Fatalf("switching to %q should follow into Main, got mode %q", got.ThreadTab, got.MainMode)
	}
}

// The Commits tab's enter FETCHES a commit diff, so following the cursor there
// would fire a network request per keystroke. It stays opt-in via enter.
func TestThreadsCommitsTabDoesNotFollow(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabCommits
	m.FocusStack = []FocusContext{FocusThreads}
	m.MainMode = MainOverview
	before := strings.Join(m.MainLines, "\n")

	got, cmd := UpdateThreads(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if cmd != nil {
		t.Fatal("moving the cursor on the Commits tab must not dispatch a fetch")
	}
	if got.MainMode != MainOverview || strings.Join(got.MainLines, "\n") != before {
		t.Fatal("the Commits tab must not follow into Main")
	}
	if got.ThreadsPanel.Cursor == m.ThreadsPanel.Cursor {
		t.Fatal("the cursor should still move on the Commits tab")
	}
}

// An external pager renders by spawning a process per file (3s cap), so following
// every keystroke would stall the UI — same degradation the Files panel takes.
func TestThreadsFollowSkippedUnderPager(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabAll
	m.Config.GUI.DiffPager = "delta --paging=never"
	m.FocusStack = []FocusContext{FocusThreads}
	m.MainMode = MainOverview

	got, _ := UpdateThreads(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if got.MainMode != MainOverview {
		t.Fatal("pager mode must not follow the Threads cursor")
	}
	if got.ThreadsPanel.Cursor != 1 {
		t.Fatalf("the cursor should still move under a pager, got %d", got.ThreadsPanel.Cursor)
	}
}

// Focusing the panel with 4 follows too, mirroring 3 for Files.
func TestFocusingThreadsFollows(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.ThreadTab = ThreadsTabAll
	m.FocusStack = []FocusContext{FocusPRs}
	m.MainMode = MainOverview

	updated, _ := m.Update(tea.KeyPressMsg{Text: "4", Code: '4'})
	got := updated.(Model)
	if got.CurrentFocus() != FocusThreads {
		t.Fatalf("4 should focus Threads, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainDiff {
		t.Fatalf("focusing Threads should follow into Main, got %q", got.MainMode)
	}
}

// enter still focuses the thread — following must not have replaced that.
func TestThreadsEnterStillFocusesThread(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabAll
	m.FocusStack = []FocusContext{FocusThreads}

	got, _ := UpdateThreads(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got.CurrentFocus() != FocusThread {
		t.Fatalf("enter should focus the thread, got %s", got.CurrentFocus())
	}
}

// Filtering resets the cursor to 0, so Main must re-follow. Otherwise the panel
// highlights one thread while Main still shows the previously selected one.
func TestThreadsFilterCommitFollows(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.ThreadTab = ThreadsTabAll
	m.FocusStack = []FocusContext{FocusThreads}
	rows := filteredThreadRows(m)
	if len(rows) < 2 {
		t.Skip("need two threads")
	}
	// Start with Main on the SECOND thread.
	m.ThreadsPanel.Cursor = 1
	m = followThreadsSelection(m)
	if id := threadIDAtMainCursor(m); id != rows[1].Thread.ID {
		t.Fatalf("precondition: Main should be on thread %q, got %q", rows[1].Thread.ID, id)
	}

	// Filter to the FIRST thread's file; the cursor resets to 0.
	m = startPanelFilter(m, &m.ThreadsPanel)
	for _, ch := range rows[0].FilePath {
		next, _ := handleFilterInput(m, string(ch))
		m = next
	}
	m, _ = handleFilterInput(m, "enter")

	if got := threadIDAtMainCursor(m); got != rows[0].Thread.ID {
		t.Fatalf("Main should re-follow the narrowed selection %q, got %q", rows[0].Thread.ID, got)
	}
}

// The same staleness existed for Files: filtering reset its cursor without
// re-pointing Main.
func TestFilesFilterCommitFollows(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.FocusStack = []FocusContext{FocusFiles}
	m.FilesFlat = true // flat rows map 1:1 to files, keeping the test about filtering
	m = followFilesSelection(m)

	m = startPanelFilter(m, &m.FilesPanel)
	for _, ch := range "bar" {
		next, _ := handleFilterInput(m, string(ch))
		m = next
	}
	m, _ = handleFilterInput(m, "enter")

	if m.MainFileIndex < 0 || !strings.Contains(m.DiffFiles[m.MainFileIndex].Path, "bar") {
		t.Fatalf("Main should follow the filtered Files selection, got index %d", m.MainFileIndex)
	}
}
