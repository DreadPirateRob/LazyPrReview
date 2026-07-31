package ui

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestOverlayCenterKeepsBaseDimsAndCenters(t *testing.T) {
	base := strings.TrimRight(strings.Repeat("abcdefghij\n", 6), "\n") // 10x6
	modal := "+--+\n|XX|\n+--+"                                        // 4x3
	out := overlayCenter(base, modal)

	if got, want := lipgloss.Width(out), lipgloss.Width(base); got != want {
		t.Fatalf("overlay changed width: got %d want %d", got, want)
	}
	if got, want := lipgloss.Height(out), lipgloss.Height(base); got != want {
		t.Fatalf("overlay changed height: got %d want %d", got, want)
	}

	lines := strings.Split(out, "\n")
	// 6 base rows, 3 modal rows -> y offset 1, so the modal's middle row is row 2.
	if !strings.Contains(lines[2], "XX") {
		t.Fatalf("modal should be vertically centered (row 2): %q", lines[2])
	}
	if strings.Contains(lines[0], "XX") {
		t.Fatalf("modal must not be drawn at the top row: %q", lines[0])
	}
	if !strings.Contains(lines[0], "abcdefghij") {
		t.Fatalf("base row above the modal must be untouched: %q", lines[0])
	}
	// 10 cols, 4 modal cols -> x offset 3, so base survives left and right of it.
	if !strings.HasPrefix(lines[2], "abc") {
		t.Fatalf("base should show left of the modal: %q", lines[2])
	}
	if !strings.HasSuffix(strings.TrimRight(lines[2], " "), "hij") {
		t.Fatalf("base should show right of the modal: %q", lines[2])
	}
}

func TestOverlayCenterDegradesWhenEmpty(t *testing.T) {
	base := "abc\ndef"
	if got := overlayCenter(base, ""); got != base {
		t.Fatalf("empty modal should leave base untouched, got %q", got)
	}
	if got := overlayCenter("", "x"); got != "" {
		t.Fatalf("empty base should stay empty, got %q", got)
	}
}

func TestHelpFloatsOverLiveUI(t *testing.T) {
	m := seededModel(t)
	m.Width, m.Height = 100, 30
	// fullView is the composed frame string; fmt.Sprint(m.View()) would append
	// tea.View's struct fields and make width math meaningless.
	plain := fullView(m)

	m.HelpVisible = true
	m.HelpEntries = AllHelpEntries()
	m.FocusStack = []FocusContext{FocusPRs, FocusHelp}
	view := fullView(m)

	if !strings.Contains(view, "Help") {
		t.Fatal("expected the help modal content in the frame")
	}
	// The defining property: panels behind the modal are still visible.
	if !strings.Contains(view, "Status") {
		t.Fatalf("panels must stay visible behind a floating overlay:\n%s", view)
	}
	if lipgloss.Width(view) != lipgloss.Width(plain) || lipgloss.Height(view) != lipgloss.Height(plain) {
		t.Fatalf("overlay must not resize the frame: got %dx%d want %dx%d",
			lipgloss.Width(view), lipgloss.Height(view), lipgloss.Width(plain), lipgloss.Height(plain))
	}
	if w := lipgloss.Width(view); w > m.Width {
		t.Fatalf("frame must never exceed the terminal width: %d > %d", w, m.Width)
	}
}

func TestComposerFloatsOverLiveUI(t *testing.T) {
	m := seededModel(t)
	m.Width, m.Height = 120, 40
	m = openComposer(m, composeState{
		Kind: composeComment, Required: true, SubjectType: "FILE",
		Path: "foo.txt", Title: "Comment on foo.txt",
	})
	view := fullView(m)

	if !strings.Contains(view, "ctrl+s") {
		t.Fatalf("expected the composer's submit hint:\n%s", view)
	}
	if !strings.Contains(view, "Status") {
		t.Fatalf("panels must stay visible behind the composer modal:\n%s", view)
	}
	// A modal, not a takeover: the box must be narrower than the frame.
	if got, frame := lipgloss.Width(ComposeView(m))+2, lipgloss.Width(view); got >= frame {
		t.Fatalf("composer box should be narrower than the frame: %d vs %d", got, frame)
	}
}

func TestMenuFloatsOverLiveUI(t *testing.T) {
	m := seededModel(t)
	m.Width, m.Height = 100, 30
	m = openSubmitMenu(m)
	view := fullView(m)

	if !strings.Contains(view, "Approve") {
		t.Fatalf("expected the submit menu content:\n%s", view)
	}
	if !strings.Contains(view, "Status") {
		t.Fatalf("panels must stay visible behind the menu modal:\n%s", view)
	}
}

func TestModalDimsLeavesMarginAndClamps(t *testing.T) {
	// Small content -> box hugs the content plus borders.
	if w, h := modalDims("abc\nde", 80, 24); w != 5 || h != 4 {
		t.Fatalf("content-sized box wrong: got %dx%d want 5x4", w, h)
	}
	// Oversized content -> capped with a margin so the UI still frames it.
	wide := strings.Repeat("x", 200)
	w, h := modalDims(wide, 80, 24)
	if w > 80-4 {
		t.Fatalf("width should be capped with a margin, got %d", w)
	}
	if h > 24-2 {
		t.Fatalf("height should be capped with a margin, got %d", h)
	}
}
