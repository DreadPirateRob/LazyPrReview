package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// ── Layout tests ─────────────────────────────────────────────────────────────

func TestLayoutPortraitThreshold(t *testing.T) {
	cfg := config.Default()
	m := New(cfg, nil)

	m.Width = 89
	if l := ComputeLayout(m); !l.Portrait {
		t.Errorf("width=89: want portrait=true, got false")
	}

	m.Width = 90
	if l := ComputeLayout(m); l.Portrait {
		t.Errorf("width=90: want portrait=false, got true")
	}
}

func TestLayoutSidePanelWidthNormal(t *testing.T) {
	cfg := config.Default() // SidePanelWidth = 0.3333
	m := New(cfg, nil)
	m.Width = 200

	l := ComputeLayout(m)
	// round(200 × 0.3333) = round(66.66) = 67
	if l.SidePanelWidth != 67 {
		t.Errorf("SidePanelWidth = %d, want 67", l.SidePanelWidth)
	}
	if l.MainWidth != 133 {
		t.Errorf("MainWidth = %d, want 133 (200-67)", l.MainWidth)
	}
}

func TestLayoutSidePanelWidthClampMin(t *testing.T) {
	cfg := config.Default()
	cfg.GUI.SidePanelWidth = 0.01 // very small → clamp to 20
	m := New(cfg, nil)
	m.Width = 100

	l := ComputeLayout(m)
	if l.SidePanelWidth != 20 {
		t.Errorf("SidePanelWidth = %d, want 20 (min clamp)", l.SidePanelWidth)
	}
}

func TestLayoutSidePanelWidthClampMax(t *testing.T) {
	cfg := config.Default()
	cfg.GUI.SidePanelWidth = 2.0 // fraction > 1 → clamp to width
	m := New(cfg, nil)
	m.Width = 50

	l := ComputeLayout(m)
	if l.SidePanelWidth != 50 {
		t.Errorf("SidePanelWidth = %d, want 50 (max clamp to width)", l.SidePanelWidth)
	}
	if l.MainWidth != 0 {
		t.Errorf("MainWidth = %d, want 0 when side panel fills width", l.MainWidth)
	}
}

func TestLayoutCommandLogHeight(t *testing.T) {
	cfg := config.Default() // CommandLogSize = 8
	m := New(cfg, nil)
	m.Width = 120

	m.CommandLogOn = false
	if l := ComputeLayout(m); l.CommandLogHeight != 0 {
		t.Errorf("CommandLogHeight = %d, want 0 when off", l.CommandLogHeight)
	}

	m.CommandLogOn = true
	if l := ComputeLayout(m); l.CommandLogHeight != 8 {
		t.Errorf("CommandLogHeight = %d, want 8 when on", l.CommandLogHeight)
	}
}

func TestLayoutHintHeight(t *testing.T) {
	cfg := config.Default() // ShowBottomLine = true
	m := New(cfg, nil)
	m.Width = 120

	if l := ComputeLayout(m); l.HintHeight != 1 {
		t.Errorf("HintHeight = %d, want 1 (ShowBottomLine=true)", l.HintHeight)
	}

	cfg2 := config.Default()
	cfg2.GUI.ShowBottomLine = false
	m2 := New(cfg2, nil)
	m2.Width = 120
	if l := ComputeLayout(m2); l.HintHeight != 0 {
		t.Errorf("HintHeight = %d, want 0 (ShowBottomLine=false)", l.HintHeight)
	}
}

// ── Fatal screen tests ───────────────────────────────────────────────────────

func TestFatalMissingGH(t *testing.T) {
	m := newFatalModel(FatalMissingGH)
	v := fatalView(m)

	if !strings.Contains(v, "gh not found") {
		t.Errorf("missing 'gh not found' in: %q", v)
	}
	if !strings.Contains(v, "cli.github.com") {
		t.Errorf("missing install URL in: %q", v)
	}
	if !strings.Contains(v, "ctrl+c") {
		t.Errorf("missing exit hint in: %q", v)
	}
}

func TestFatalUnauthed(t *testing.T) {
	m := newFatalModel(FatalUnauthed)
	v := fatalView(m)

	if !strings.Contains(v, "not authenticated") {
		t.Errorf("missing 'not authenticated' in: %q", v)
	}
	if !strings.Contains(v, "gh auth login") {
		t.Errorf("missing 'gh auth login' in: %q", v)
	}
}

func TestFatalNoRepo(t *testing.T) {
	m := newFatalModel(FatalNoRepo)
	v := fatalView(m)

	if !strings.Contains(v, "no GitHub repository") {
		t.Errorf("missing 'no GitHub repository' in: %q", v)
	}
	if !strings.Contains(v, "--repo") {
		t.Errorf("missing '--repo' fix in: %q", v)
	}
}

func TestFatalQuitKeys(t *testing.T) {
	m := newFatalModel(FatalMissingGH)

	keys := []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	}
	for _, k := range keys {
		_, cmd := m.Update(k)
		if cmd == nil {
			t.Errorf("key %q: expected a quit Cmd, got nil", k.Keystroke())
			continue
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("key %q: cmd() = %T, want tea.QuitMsg", k.Keystroke(), msg)
		}
	}
}

func TestFatalNonQuitKeysIgnored(t *testing.T) {
	m := newFatalModel(FatalMissingGH)

	// Every non-quit key in fatal context must be a no-op.
	for _, k := range []tea.KeyPressMsg{
		{Code: 'x', Text: "x"},
		{Code: tea.KeyEscape},
		{Code: tea.KeyEnter},
	} {
		_, cmd := m.Update(k)
		if cmd != nil {
			t.Errorf("key %q in fatal context: expected nil cmd, got non-nil", k.Keystroke())
		}
	}
}

func TestLoadingQuitKeys(t *testing.T) {
	m := New(config.Default(), nil) // starts in Loading=true
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("'q' in loading state: expected quit Cmd, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

func TestMutationFailedMsgCreatesLiveToast(t *testing.T) {
	m := New(config.Default(), nil)
	before := time.Now()
	updated, _ := m.Update(mutationFailedMsg{Action: "toggle", Err: errors.New("boom")})
	got := updated.(Model).Toast
	if got.Message != "boom" {
		t.Fatalf("toast message = %q, want %q", got.Message, "boom")
	}
	if got.Until.Before(before.Add(ToastDuration)) {
		t.Fatalf("toast Until was not initialized: %v", got.Until)
	}
	if ToastExpired(got, before) {
		t.Fatalf("new mutation failure toast should be live immediately")
	}
}

// ── Focus stack tests ────────────────────────────────────────────────────────

func TestFocusStackPushPop(t *testing.T) {
	m := New(config.Default(), nil)
	if m.CurrentFocus() != FocusPRs {
		t.Errorf("initial focus = %q, want %q", m.CurrentFocus(), FocusPRs)
	}

	m2 := m.PushFocus(FocusFiles)
	if m2.CurrentFocus() != FocusFiles {
		t.Errorf("after push: focus = %q, want %q", m2.CurrentFocus(), FocusFiles)
	}
	// original unmodified
	if m.CurrentFocus() != FocusPRs {
		t.Errorf("original focus mutated after PushFocus")
	}

	m3 := m2.PopFocus()
	if m3.CurrentFocus() != FocusPRs {
		t.Errorf("after pop: focus = %q, want %q", m3.CurrentFocus(), FocusPRs)
	}
}

func TestFocusStackPopFloor(t *testing.T) {
	m := New(config.Default(), nil) // stack = [prs]
	m2 := m.PopFocus()
	if m2.CurrentFocus() != FocusPRs {
		t.Errorf("pop from one-entry stack changed focus to %q", m2.CurrentFocus())
	}
	if len(m2.FocusStack) != 1 {
		t.Errorf("FocusStack len = %d, want 1", len(m2.FocusStack))
	}
}

// ── Screen mode cycle tests ───────────────────────────────────────────────────

func TestScreenModeCycle(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false

	// normal → half → fullscreen → normal
	m = m.NextScreenMode()
	if m.ScreenMode != ScreenHalf {
		t.Errorf("NextScreenMode: got %q, want %q", m.ScreenMode, ScreenHalf)
	}
	m = m.NextScreenMode()
	if m.ScreenMode != ScreenFullscreen {
		t.Errorf("NextScreenMode: got %q, want %q", m.ScreenMode, ScreenFullscreen)
	}
	m = m.NextScreenMode()
	if m.ScreenMode != ScreenNormal {
		t.Errorf("NextScreenMode wrap: got %q, want %q", m.ScreenMode, ScreenNormal)
	}

	// fullscreen → half → normal via PrevScreenMode
	m.ScreenMode = ScreenFullscreen
	m = m.PrevScreenMode()
	if m.ScreenMode != ScreenHalf {
		t.Errorf("PrevScreenMode: got %q, want %q", m.ScreenMode, ScreenHalf)
	}
}

func TestViewReflectsScreenMode(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.ScreenMode = ScreenFullscreen
	view := fmt.Sprint(m.View())
	if !strings.Contains(view, "mode: fullscreen") {
		t.Fatalf("expected fullscreen mode in view, got %q", view)
	}
	if strings.Contains(view, "Status") {
		t.Fatalf("fullscreen view should omit side panels")
	}
}

func TestViewReflectsHelpAndCommandLog(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.FocusStack = []FocusContext{FocusPRs}
	m.CommandLogOn = true
	m.CommandLog = []ghcli.CommandLogEntry{{Command: []string{"gh", "pr", "list"}}}
	m.HelpVisible = true
	m.HelpEntries = AllHelpEntries(nil)
	view := fmt.Sprint(m.View())
	if !strings.Contains(view, "Command log") || !strings.Contains(view, "gh pr list") {
		t.Fatalf("expected command log in view, got %q", view)
	}
	if !strings.Contains(view, "Help") {
		t.Fatalf("expected help overlay in view")
	}
	if strings.Contains(view, "main prevHunk") {
		t.Fatalf("help overlay should be scoped to active context plus universal entries")
	}
	if !strings.Contains(view, "prs <enter> openPR") {
		t.Fatalf("expected current-context help entry in view")
	}
}

func TestHelpOverlayScrollsWithCursor(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.HelpVisible = true
	m.HelpEntries = AllHelpEntries(nil)
	m.FocusStack = []FocusContext{FocusPRs, FocusHelp}
	before := fmt.Sprint(m.View())
	m2, _ := UpdateHelp(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	after := fmt.Sprint(m2.View())
	if m2.HelpCursor != 1 {
		t.Fatalf("expected help cursor to move, got %d", m2.HelpCursor)
	}
	if before == after {
		t.Fatalf("expected help overlay render to change after scrolling")
	}
}

func TestEscClosesHelpOverlay(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.HelpVisible = true
	m.HelpEntries = AllHelpEntries(nil)
	m.FocusStack = []FocusContext{FocusPRs, FocusHelp}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)
	if got.HelpVisible {
		t.Fatal("expected help overlay to close on esc")
	}
	if got.CurrentFocus() != FocusPRs {
		t.Fatalf("expected focus to return to PRs, got %s", got.CurrentFocus())
	}
}

func TestViewReflectsPortraitLayout(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 80
	m.Height = 24
	view := fmt.Sprint(m.View())
	if !strings.Contains(view, "portrait=true") {
		t.Fatalf("expected portrait marker in view")
	}
	if strings.Contains(view, "||") {
		t.Fatalf("portrait layout should stack panels, not render two columns")
	}
}

func TestViewReflectsNormalVsHalfLayout(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 120
	m.Height = 24
	normal := fmt.Sprint(m.View())
	m.ScreenMode = ScreenHalf
	half := fmt.Sprint(m.View())
	if !strings.Contains(normal, "╭─[1]─Status") || !strings.Contains(half, "╭─[1]─Status") {
		t.Fatalf("expected bordered side panels in non-portrait layouts")
	}
	if !strings.Contains(normal, "╭─[0]─Main") || !strings.Contains(half, "╭─[0]─Main") {
		t.Fatalf("expected bordered main pane in non-portrait layouts")
	}
	if !strings.Contains(normal, "side=40 main=80") {
		t.Fatalf("expected normal split in view, got %q", normal)
	}
	if !strings.Contains(half, "side=60 main=60") {
		t.Fatalf("expected half split in view, got %q", half)
	}
}

func TestCommandLogMenuFocusOption(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.CommandLog = []ghcli.CommandLogEntry{{Command: []string{"gh", "pr", "list"}}}
	m = openCommandLogMenu(m)
	if len(m.MenuItems) < 2 {
		t.Fatalf("expected command log menu items")
	}
	m, _ = applyMenuItem(m, m.MenuItems[1])
	if !m.CommandLogOn || !m.CommandLogFocused {
		t.Fatalf("expected command log to be shown and focused")
	}
	if !strings.Contains(fmt.Sprint(m.View()), "Command log [focused]") {
		t.Fatalf("expected focused command log marker in view")
	}
}

func TestViewFitsTerminalHeightPortrait(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 80
	m.Height = 18
	view := fmt.Sprint(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) > m.Height {
		t.Fatalf("portrait view overflowed terminal height: got %d lines for height %d", len(lines), m.Height)
	}
}

func TestViewFitsTerminalHeightColumns(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 120
	m.Height = 18
	view := fmt.Sprint(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) > m.Height {
		t.Fatalf("column view overflowed terminal height: got %d lines for height %d", len(lines), m.Height)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newFatalModel(kind FatalKind) Model {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Fatal = kind
	m.FocusStack = []FocusContext{FocusFatal}
	m.Width = 80
	m.Height = 24
	return m
}
