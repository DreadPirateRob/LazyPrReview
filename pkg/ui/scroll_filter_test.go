package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// scrollModel: fullscreen 40x12 => usableH 10, interior rows 8, half page 4.
func scrollModel() Model {
	m := New(config.Default(), nil)
	m.Loading = false
	m.ScreenMode = ScreenFullscreen
	m.Width = 40
	m.Height = 12
	m.PRDetail = &domain.PRDetail{Title: "t"}
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	m.MainLines = lines
	m.FocusStack = []FocusContext{FocusMain}
	return m
}

func TestMainHalfPageScroll(t *testing.T) {
	m := scrollModel()
	m, _ = UpdateMain(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.MainCursor != 4 || m.MainScroll != 4 {
		t.Fatalf("ctrl+d: expected cursor/scroll 4/4, got %d/%d", m.MainCursor, m.MainScroll)
	}
	m, _ = UpdateMain(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.MainCursor != 0 || m.MainScroll != 0 {
		t.Fatalf("ctrl+u: expected cursor/scroll 0/0, got %d/%d", m.MainCursor, m.MainScroll)
	}
}

func TestMainHalfPageKeepsScreenRow(t *testing.T) {
	m := scrollModel()
	m.MainCursor = 10
	m = followMainCursor(m) // scroll = 3, cursor on bottom row
	rowBefore := m.MainCursor - m.MainScroll
	m, _ = UpdateMain(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if row := m.MainCursor - m.MainScroll; row != rowBefore {
		t.Fatalf("expected cursor to keep screen row %d, got %d", rowBefore, row)
	}
}

func TestMainHeadlessFallbackPage(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.PRDetail = &domain.PRDetail{Title: "t"}
	m.MainLines = make([]string, 60)
	m, _ = UpdateMain(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.MainCursor != 10 {
		t.Fatalf("headless ctrl+d: expected historical page 10, got %d", m.MainCursor)
	}
}

func TestMainZZCentersCursor(t *testing.T) {
	m := scrollModel()
	m.MainCursor = 30
	m = followMainCursor(m)
	m, _ = UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if !m.MainPendingZ {
		t.Fatal("expected first z to arm pending state")
	}
	m, _ = UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if m.MainPendingZ {
		t.Fatal("expected second z to clear pending state")
	}
	// rows=8 => center puts cursor at scroll+4: scroll = 30-4 = 26.
	if m.MainScroll != 26 {
		t.Fatalf("zz: expected scroll 26 (cursor centered), got %d", m.MainScroll)
	}
	if m.MainCursor != 30 {
		t.Fatalf("zz must not move the cursor, got %d", m.MainCursor)
	}
}

func TestMainPendingZDisarmedByOtherKey(t *testing.T) {
	m := scrollModel()
	m.MainCursor = 30
	m = followMainCursor(m)
	scrollBefore := m.MainScroll
	m, _ = UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	m, _ = UpdateMain(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if m.MainPendingZ {
		t.Fatal("expected pending z disarmed by j")
	}
	if m.MainCursor != 31 {
		t.Fatalf("expected j handled normally after pending z, cursor %d", m.MainCursor)
	}
	if m.MainScroll != scrollBefore+1 {
		t.Fatalf("expected follow scroll %d, got %d", scrollBefore+1, m.MainScroll)
	}
}

func TestFilterCapturesUniversalKeys(t *testing.T) {
	m := newPRListModel()
	m.FilterActive = true

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected q to feed the filter, not quit")
	}
	if got.PRPanel.Filter != "q" {
		t.Fatalf("expected filter query %q, got %q", "q", got.PRPanel.Filter)
	}

	updated, _ = got.Update(tea.KeyPressMsg{Text: "1", Code: '1'})
	got = updated.(Model)
	if got.PRPanel.Filter != "q1" {
		t.Fatalf("expected filter query %q, got %q", "q1", got.PRPanel.Filter)
	}
	if got.CurrentFocus() != FocusPRs {
		t.Fatalf("expected focus to stay on PRs while filtering, got %s", got.CurrentFocus())
	}

	// ctrl+c stays a safety quit even while filtering.
	_, cmd = got.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected ctrl+c to quit while filtering")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected QuitMsg, got %T", msg)
		}
	}
}

func TestFilterEscCancelsEnterCommits(t *testing.T) {
	m := newPRListModel()
	m.FilterActive = true
	m.PRPanel.Filter = "auth"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)
	if got.FilterActive || got.PRPanel.Filter != "" {
		t.Fatalf("esc should cancel filter and clear query, got active=%v query=%q", got.FilterActive, got.PRPanel.Filter)
	}

	m.FilterActive = true
	m.PRPanel.Filter = "auth"
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got = updated.(Model)
	if got.FilterActive {
		t.Fatal("enter should commit and leave filter typing mode")
	}
	if got.PRPanel.Filter != "auth" {
		t.Fatalf("enter should keep the query, got %q", got.PRPanel.Filter)
	}
}
