package ui

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A target line longer than the modal must WRAP, not get cut: every character of
// the selected code stays visible.
func TestPreviewWrapsInsteadOfTruncating(t *testing.T) {
	row := "      96 │ + - Official docs reviewed date: 2026-06-10 (https://docs.example.com/very/long/path/to/thing)"
	const w = 60

	got := previewBlock([]string{row}, w)
	if len(got) < 2 {
		t.Fatalf("a row wider than %d should wrap onto multiple lines, got %d", w, len(got))
	}
	joined := ""
	for _, seg := range got {
		if width := lipgloss.Width(seg); width > w {
			t.Fatalf("wrapped segment exceeds the budget %d: %d %q", w, width, seg)
		}
		if strings.Contains(ansi.Strip(seg), "…") {
			t.Fatalf("wrapping must not elide content: %q", seg)
		}
		joined += strings.TrimSpace(ansi.Strip(seg)) + " "
	}
	// The tail of the original line survives somewhere in the wrapped output.
	if !strings.Contains(strings.ReplaceAll(joined, " ", ""), "very/long/path/to/thing)") {
		t.Fatalf("the end of the line should still be visible, got %q", joined)
	}
}

// Continuation lines align under the code column so the gutter doesn't restart.
func TestPreviewWrapAlignsContinuations(t *testing.T) {
	row := "      96 │ + " + strings.Repeat("abcde ", 30)
	indent := codeColumn(row)
	if indent != 13 {
		t.Fatalf("expected the code column past the gutter and change marker, got %d", indent)
	}

	got := wrapPreviewRow(row, 50)
	if len(got) < 2 {
		t.Fatal("expected the row to wrap")
	}
	for _, seg := range got[1:] {
		if !strings.HasPrefix(seg, strings.Repeat(" ", indent)) {
			t.Fatalf("continuation should be indented to the code column: %q", seg)
		}
	}
}

// Rows with no gutter (file/hunk headers) still wrap without blowing the budget.
func TestPreviewWrapHandlesGutterlessRows(t *testing.T) {
	for _, seg := range wrapPreviewRow(strings.Repeat("x", 200), 40) {
		if lipgloss.Width(seg) > 40 {
			t.Fatalf("gutterless row exceeded the budget: %d", lipgloss.Width(seg))
		}
	}
}

// A big selection can't grow the modal without bound.
func TestPreviewBlockCapsHeight(t *testing.T) {
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = "      96 │ + " + strings.Repeat("y", 300)
	}
	if got := previewBlock(rows, 50); len(got) > 13 {
		t.Fatalf("preview should cap its rendered height, got %d rows", len(got))
	}
}
