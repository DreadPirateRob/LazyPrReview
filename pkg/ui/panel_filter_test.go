package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestThreadsTitleShowsAllTabs(t *testing.T) {
	m := seededModel(t)
	title := threadsTitle(m)
	if !strings.Contains(title, "[Unresolved]") || !strings.Contains(title, "All") || !strings.Contains(title, "Commits") {
		t.Fatalf("expected full tab bar with active bracketed, got %q", title)
	}
	m.ThreadTab = ThreadsTabCommits
	title = threadsTitle(m)
	if !strings.Contains(title, "[Commits]") || !strings.Contains(title, "Unresolved") {
		t.Fatalf("expected Commits bracketed with other tabs visible, got %q", title)
	}
}

func TestFilesFilterNarrowsAndOpens(t *testing.T) {
	m := seededModel(t)
	m.FilesPanel.Filter = "bar"
	view := FilesView(m, 0)
	if strings.Contains(view, "foo.txt") || !strings.Contains(view, "bar.txt") {
		t.Fatalf("expected only bar.txt visible, got %q", view)
	}
	updated, _ := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.MainFileIndex != 1 {
		t.Fatalf("expected filtered enter to open bar.txt (diff index 1), got %d", updated.MainFileIndex)
	}
}

func TestThreadsFilterNarrowsAndJumps(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabAll
	m.ThreadsPanel.Filter = "second"
	rows := filteredThreadRows(m)
	if len(rows) != 1 || rows[0].FilePath != "bar.txt" {
		t.Fatalf("expected only the bar.txt thread, got %#v", rows)
	}
	view := ThreadsView(m)
	if strings.Contains(view, "foo.txt:3") {
		t.Fatalf("expected foo thread filtered out, got %q", view)
	}
	updated, _ := UpdateThreads(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.MainFileIndex != 1 {
		t.Fatalf("expected jump into bar.txt diff, got file %d", updated.MainFileIndex)
	}
}

func TestChecksFilterNarrowsRows(t *testing.T) {
	m := newChecksModel()
	m.ChecksPanel.Filter = "lint"
	view := ChecksView(m)
	if strings.Contains(view, "CI / build") || !strings.Contains(view, "lint") {
		t.Fatalf("expected only lint check visible, got %q", view)
	}
}

func TestPRLoadResetsDetailPanelFilters(t *testing.T) {
	m := seededModel(t)
	m.FilesPanel.Filter = "foo"
	m.ThreadsPanel.Filter = "bar"
	m.ChecksPanel.Filter = "ci"
	m.PRPanel.Filter = "keep"
	updated, _ := m.Update(prDetailLoadedMsg{Detail: *m.PRDetail})
	got := updated.(Model)
	if got.FilesPanel.Filter != "" || got.ThreadsPanel.Filter != "" || got.ChecksPanel.Filter != "" {
		t.Fatalf("expected detail panel filters reset on PR open, got files=%q threads=%q checks=%q",
			got.FilesPanel.Filter, got.ThreadsPanel.Filter, got.ChecksPanel.Filter)
	}
	if got.PRPanel.Filter != "keep" {
		t.Fatalf("PR list filter must survive PR open, got %q", got.PRPanel.Filter)
	}
}

func TestThreadsTabSwitchResetsFilter(t *testing.T) {
	m := seededModel(t)
	m.ThreadsPanel.Filter = "x"
	m.ThreadsPanel.Cursor = 1
	updated, _ := UpdateThreads(m, tea.KeyPressMsg{Text: "]", Code: ']'})
	if updated.ThreadsPanel.Filter != "" || updated.ThreadsPanel.Cursor != 0 {
		t.Fatalf("expected tab switch to reset filter+cursor, got %q/%d",
			updated.ThreadsPanel.Filter, updated.ThreadsPanel.Cursor)
	}
}

func TestSlashFiltersFilesPanelEndToEnd(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusFiles}
	updated, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	got := updated.(Model)
	if !got.FilterActive {
		t.Fatal("expected / to activate the Files filter")
	}
	updated, _ = got.Update(tea.KeyPressMsg{Text: "b", Code: 'b'})
	got = updated.(Model)
	if got.FilesPanel.Filter != "b" {
		t.Fatalf("expected typed key to land in Files filter, got %q", got.FilesPanel.Filter)
	}
	updated, _ = got.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got = updated.(Model)
	if got.FilterActive || got.FilesPanel.Filter != "b" {
		t.Fatalf("expected enter to commit the Files filter, got active=%v %q", got.FilterActive, got.FilesPanel.Filter)
	}
	if view := FilesView(got, 0); strings.Contains(view, "foo.txt") {
		t.Fatalf("expected committed filter to narrow files, got %q", view)
	}
}
