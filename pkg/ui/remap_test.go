package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

// remapModel builds a model the way the real program does — through New, from a
// config — so these tests cover the whole path config → keymap → dispatch rather
// than a hand-built binding table.
func remapModel(t *testing.T, binding map[string]map[string]any) Model {
	t.Helper()
	cfg := config.Default()
	cfg.Keybinding = binding
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("fixture config must be valid: %v", err)
	}
	m := New(cfg, fakeforge.New())
	m.Loading = false
	return m
}

// The reported gap: config.Keybinding loaded and validated, then dispatch ignored
// it because every handler switched on hardcoded literals.
func TestRemappedKeyReachesHandler(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"cursorDown": "x"},
	})
	m.FocusStack = []FocusContext{FocusStatus}

	moved, _ := UpdateStatus(m, tea.KeyPressMsg{Text: "x", Code: 'x'})
	if moved.Status.Cursor != 1 {
		t.Fatalf("remapped x should move the cursor, got %d", moved.Status.Cursor)
	}
}

// Remapping an action away from its default must take the default with it, or both
// keys work and the config was a suggestion rather than a setting.
func TestVacatedDefaultStopsWorking(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"cursorDown": "x"},
	})
	m.FocusStack = []FocusContext{FocusStatus}

	still, _ := UpdateStatus(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if still.Status.Cursor != 0 {
		t.Fatalf("j was vacated by the remap and must no longer move the cursor, got %d", still.Status.Cursor)
	}
}

func TestDisabledBindingIsInert(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"cursorDown": "<disabled>"},
	})
	m.FocusStack = []FocusContext{FocusStatus}

	got, _ := UpdateStatus(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if got.Status.Cursor != 0 {
		t.Fatalf("a disabled action must not move the cursor, got %d", got.Status.Cursor)
	}
}

// A context binding must not be disturbed by a universal remap of the same key:
// Main binds `[` to prevFile while universal binds it to prevTab.
func TestContextKeySurvivesUniversalRemapThroughDispatch(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"prevTab": "p"},
	})
	m.PRDetail = &domain.PRDetail{Title: "t"}
	m.FocusStack = []FocusContext{FocusMain}

	if _, ok := m.Keys.Canonical("main", "["); !ok {
		t.Fatal("[ still belongs to main.prevFile and must reach the diff handler")
	}
}

// ctrl+c is the escape hatch: a config that disables quit must not lock the user in.
func TestCtrlCQuitsEvenWhenQuitDisabled(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"quit": "<disabled>"},
	})
	m.FocusStack = []FocusContext{FocusStatus}

	if _, cmd := m.handleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil {
		t.Fatal("ctrl+c must always quit, even with quit disabled")
	}
	if _, cmd := m.handleKey(tea.KeyPressMsg{Text: "q", Code: 'q'}); cmd != nil {
		t.Fatal("q was disabled and must no longer quit")
	}
}

// prevPanel/nextPanel were registered in the keymap with no handler at all, so `?`
// advertised panel cycling that did nothing.
func TestTabCyclesSidePanels(t *testing.T) {
	m := remapModel(t, nil)
	m.FocusStack = []FocusContext{FocusStatus}

	next, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := next.(Model).CurrentFocus(); got != FocusPRs {
		t.Fatalf("tab from Status should focus PRs, got %s", got)
	}
}

// Panel cycling is claimed by ACTION, so Main keeps `h` for prevHunk instead of the
// universal prevPanel stealing it.
func TestPanelCycleDoesNotStealMainHunkKeys(t *testing.T) {
	m := remapModel(t, nil)
	m.PRDetail = &domain.PRDetail{Title: "t"}
	m.FocusStack = []FocusContext{FocusMain}

	got, _ := m.handleKey(tea.KeyPressMsg{Text: "h", Code: 'h'})
	if focus := got.(Model).CurrentFocus(); focus != FocusMain {
		t.Fatalf("h in Main is prevHunk and must not cycle panels, got focus %s", focus)
	}
}

// The other half of the gap: `?` help and the hint bar read the shipped defaults,
// so they kept advertising keys the config had moved or switched off.
func TestHelpAdvertisesRemappedKey(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"showHelp": "<f1>"},
	})

	var found bool
	for _, e := range AllHelpEntries(m.Keys) {
		if e.Action != "showHelp" {
			continue
		}
		found = true
		if len(e.Keys) != 1 || e.Keys[0] != "<f1>" {
			t.Fatalf("? should advertise the remapped key, got %v", e.Keys)
		}
	}
	if !found {
		t.Fatal("showHelp should still be listed in ? after a remap")
	}
}

func TestHelpDropsDisabledAction(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"threads": {"resolveThread": "<disabled>"},
	})

	for _, e := range AllHelpEntries(m.Keys) {
		if e.Action == "resolveThread" {
			t.Fatal("a disabled action must not be advertised in ? at all")
		}
	}
}

func TestHintBarShowsEffectiveKey(t *testing.T) {
	m := remapModel(t, map[string]map[string]any{
		"universal": {"showHelp": "<f1>"},
	})
	m.Width, m.Height = 130, 40
	m.FocusStack = []FocusContext{FocusStatus}

	bar := HintBarView(m)
	if !strings.Contains(bar, "<f1>") {
		t.Fatalf("hint bar should advertise the remapped help key, got %q", bar)
	}
	if strings.Contains(bar, "? Help") {
		t.Fatalf("hint bar must not still advertise the vacated default, got %q", bar)
	}
}
