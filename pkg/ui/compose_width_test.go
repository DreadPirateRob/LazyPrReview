package ui

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A long file path makes the modal's title its widest line, so the modal is wider
// than the editor. The target-line rows must fill that width instead of stopping
// short of the right edge.
func TestComposerPreviewFillsModalWidth(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.Width, m.Height = 130, 40

	m.MainCursor = addIdx

	const longPath = "frontend2/app/containers/MainDashboard/selectors.ts"
	m = openComposer(m, composeState{
		Kind: composeComment, Required: true, SubjectType: "LINE",
		Title: "Comment " + longPath + ":420 (RIGHT) — ctrl+s submit",
		Path:  longPath, Line: 420, Side: "RIGHT",
		Preview: previewRows(m, m.MainCursor, m.MainCursor),
	})

	body := ComposeView(m)
	inner := lipgloss.Width(body) // the modal sizes to its widest line

	var sawRule bool
	for _, ln := range strings.Split(body, "\n") {
		if !strings.HasPrefix(ansi.Strip(ln), "─") {
			continue
		}
		sawRule = true
		if got := lipgloss.Width(ln); got != inner {
			t.Fatalf("preview rule should span the modal width %d, got %d (%d cols of dead space)",
				inner, got, inner-got)
		}
	}
	if !sawRule {
		t.Fatal("expected a separator rule under the preview")
	}
}

// The budget must never exceed what the modal can actually show: modalDims caps
// the box at frame-4, so content is capped at frame-6.
func TestComposerPreviewRespectsFrameCap(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.Width, m.Height = 60, 20

	m.MainCursor = addIdx
	m = openComposer(m, composeState{
		Kind:    composeComment,
		Title:   "Comment " + strings.Repeat("x/", 60) + "deep.ts:1 (RIGHT)",
		Preview: previewRows(m, m.MainCursor, m.MainCursor),
	})

	for _, ln := range strings.Split(ComposeView(m), "\n") {
		if strings.HasPrefix(ansi.Strip(ln), "─") {
			if got := lipgloss.Width(ln); got > m.Width-6 {
				t.Fatalf("preview budget %d exceeds the frame cap %d", got, m.Width-6)
			}
		}
	}
}
