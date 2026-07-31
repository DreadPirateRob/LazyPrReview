package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// The round trip the user described: open a PR (Main shows its description),
// focus Files so Main shows a file diff, then return to the PR list — Main must
// show the PR description again, and without any refetch.
func TestFocusingPRListRestoresOverview(t *testing.T) {
	m := seededModel(t)
	m = showPROverview(m) // as if enter had just opened the PR
	m, _ = showFileInMain(m, 0)
	if m.MainMode != MainDiff {
		t.Fatalf("precondition: Main should be on a file diff, got %q", m.MainMode)
	}
	m.FocusStack = []FocusContext{FocusFiles}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	got := updated.(Model)

	if got.CurrentFocus() != FocusPRs {
		t.Fatalf("2 should focus the PR list, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainOverview {
		t.Fatalf("returning to the PR list should restore the PR description, got %q", got.MainMode)
	}
	if len(got.MainLines) == 0 {
		t.Fatal("overview lines should be populated")
	}
	if cmd != nil {
		t.Fatal("restoring the overview must not dispatch any command (no refetch)")
	}
}

// Returning to the list when Main is already on the overview must not jolt the
// reader back to the top.
func TestFocusingPRListKeepsOverviewPosition(t *testing.T) {
	m := seededModel(t)
	m = showPROverview(m)
	m.MainCursor, m.MainScroll = 7, 4
	m.FocusStack = []FocusContext{FocusFiles}

	updated, _ := m.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	got := updated.(Model)

	if got.MainCursor != 7 || got.MainScroll != 4 {
		t.Fatalf("already on the overview must preserve position, got cursor=%d scroll=%d", got.MainCursor, got.MainScroll)
	}
}

// With no PR opened yet there is nothing to restore, and it must not blow up.
func TestFocusingPRListNoopWithoutPR(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false

	updated, _ := m.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	got := updated.(Model)

	if got.CurrentFocus() != FocusPRs {
		t.Fatalf("2 should still focus the PR list, got %s", got.CurrentFocus())
	}
	if got.MainMode == MainOverview && len(got.MainLines) > 0 {
		t.Fatal("no PR is open, so there is no overview to show")
	}
}

// Navigating the list must never fetch: the PR opens only on enter.
func TestPRListNavigationDoesNotFetch(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusPRs}
	m.PRs = []domain.PRSummary{{Number: 1, Title: "one"}, {Number: 2, Title: "two"}}
	m.PRPanel.Cursor = 0
	m, _ = showFileInMain(m, 0) // Main is on a file diff

	for _, key := range []tea.KeyPressMsg{
		{Text: "j", Code: 'j'},
		{Text: "k", Code: 'k'},
	} {
		got, cmd := UpdatePRs(m, key)
		if cmd != nil {
			t.Fatalf("%q in the PR list must not dispatch a fetch", key.Keystroke())
		}
		if got.LoadingDetail {
			t.Fatalf("%q must not enter the detail-loading state", key.Keystroke())
		}
		if got.OpenedPRNumber != m.OpenedPRNumber {
			t.Fatalf("%q must not open a PR (opened #%d)", key.Keystroke(), got.OpenedPRNumber)
		}
		if got.MainMode != MainDiff {
			t.Fatalf("%q must not repaint Main, got mode %q", key.Keystroke(), got.MainMode)
		}
	}
}
