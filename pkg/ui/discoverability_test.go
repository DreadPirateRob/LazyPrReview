package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// The authoring keys must be discoverable where they work: visible in the
// context's hint bar and in the scoped `?` help.
func TestHintBarSurfacesAuthoringKeys(t *testing.T) {
	for _, tc := range []struct {
		focus FocusContext
		want  []string
	}{
		{FocusMain, []string{"c Comment", "v Range"}},
		{FocusFiles, []string{"c Comment"}},
		{FocusThreads, []string{"<space> Resolve"}},
		{FocusThread, []string{"r Reply", "e Edit", "d Delete"}},
	} {
		m := seededModel(t)
		m.FocusStack = []FocusContext{tc.focus}
		bar := HintBarView(m)
		for _, want := range tc.want {
			if !strings.Contains(bar, want) {
				t.Errorf("%s hint bar missing %q: %q", tc.focus, want, bar)
			}
		}
	}
}

// The hint bar must fit the terminal so nothing important is clipped away.
func TestHintBarFitsTerminalWidth(t *testing.T) {
	for _, focus := range []FocusContext{FocusMain, FocusFiles, FocusThreads, FocusThread, FocusPRs, FocusStatus, FocusChecks} {
		m := seededModel(t)
		m.Width, m.Height = 120, 36
		m.FocusStack = []FocusContext{focus}
		if bar := HintBarView(m); len(bar) > 0 && lipgloss.Width(bar) > m.Width {
			t.Errorf("%s hint bar overflows %d cols (%d): %q", focus, m.Width, lipgloss.Width(bar), bar)
		}
	}
}

func TestScopedHelpSurfacesAuthoringActions(t *testing.T) {
	m := seededModel(t)
	m.HelpEntries = AllHelpEntries()

	m.FocusStack = []FocusContext{FocusMain}
	help := helpOverlayView(m)
	for _, want := range []string{"submitReview", "commentLine", "rangeSelect"} {
		if !strings.Contains(help, want) {
			t.Errorf("main-scoped help missing %q", want)
		}
	}

	m.FocusStack = []FocusContext{FocusThread}
	help = helpOverlayView(m)
	for _, want := range []string{"replyThread", "editComment", "deleteComment"} {
		if !strings.Contains(help, want) {
			t.Errorf("thread-scoped help missing %q", want)
		}
	}
}

// toggleThreadFold was once advertised in the keymap with no implementation. It is
// real now — z folds the inline thread block under the cursor — so the hint must be
// present AND backed by behavior. The behavior half is what stops it regressing
// into a phantom again.
func TestThreadFoldBindingIsRealAndAdvertised(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusMain}
	if bar := HintBarView(m); !strings.Contains(bar, "Fold") {
		t.Errorf("main hint bar should advertise the fold action: %q", bar)
	}

	// Behavior: park the cursor on a thread row and confirm z collapses it.
	m = setMainDiff(m, 0)
	m.MainMode = MainDiff
	m.MainFileIndex = 0
	row := -1
	for i, r := range m.MainRows {
		if r.Kind == rowThreadHeader {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("fixture should render an inline thread block")
	}
	m.MainCursor = row
	before := len(m.MainLines)

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if got.MainPendingZ {
		t.Fatal("z on a thread row should fold, not arm the zz prefix")
	}
	if len(got.MainLines) >= before {
		t.Fatalf("folding should shrink the diff: %d -> %d", before, len(got.MainLines))
	}
}

// ...but z on a code row must still arm zz, or centering breaks on any line that
// happens to carry a comment.
func TestZOnCodeRowStillArmsCenter(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusMain}
	m = setMainDiff(m, 0)
	m.MainMode = MainDiff
	m.MainFileIndex = 0
	for i, r := range m.MainRows {
		if r.Kind == rowCode {
			m.MainCursor = i
			break
		}
	}
	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if !got.MainPendingZ {
		t.Fatal("z on a code row should arm the zz center prefix")
	}
}
