package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// counterText returns the ANSI-stripped counter string for the given adds/dels,
// matching renderFileRow's own format. Reusing the real styles ensures the test
// stays in sync with any future style change.
func counterText(adds, dels int) string {
	a := diffAddStyle.Render(fmt.Sprintf("+%d", adds))
	d := diffDelStyle.Render(fmt.Sprintf("-%d", dels))
	return ansi.Strip(a + " " + d)
}

// fileRowDepth0 is a convenience: depth=0 non-viewed file with fixed adds/dels.
func fileRowDepth0(label string, adds, dels int) fileRow {
	return fileRow{Label: label, Adds: adds, Dels: dels}
}

// ── counters flush-right ──────────────────────────────────────────────────────

func TestRenderFileRowFlushRight(t *testing.T) {
	const width = 40
	r := fileRowDepth0("short.go", 5, 3)
	row := renderFileRow(r, width)

	if got := ansi.StringWidth(row); got != width {
		t.Errorf("row width = %d, want %d", got, width)
	}
	stripped := ansi.Strip(row)
	ct := counterText(5, 3)
	if !strings.HasSuffix(stripped, ct) {
		t.Errorf("row does not end with counters %q: %q", ct, stripped)
	}
}

// ── long path truncates but counters survive ──────────────────────────────────

func TestRenderFileRowLongPathTruncates(t *testing.T) {
	const width = 40
	// label is 46 chars — well over the 32-col labelBudget at this width.
	label := "very/long/path/controllers/files_controller.go"
	r := fileRowDepth0(label, 10, 2)
	row := renderFileRow(r, width)

	if got := ansi.StringWidth(row); got != width {
		t.Errorf("row width = %d, want %d", got, width)
	}
	stripped := ansi.Strip(row)
	if !strings.Contains(stripped, "…") {
		t.Errorf("truncated row should contain ellipsis: %q", stripped)
	}
	// Counter digits must survive — this is the regression guard for the bug.
	ct := counterText(10, 2)
	if !strings.Contains(stripped, ct) {
		t.Errorf("counters %q missing from truncated row: %q", ct, stripped)
	}
	if !strings.HasSuffix(stripped, ct) {
		t.Errorf("counters should be flush-right; row: %q", stripped)
	}
}

// ── left truncation: basename (rightmost path component) survives ─────────────

func TestRenderFileRowLeftTruncationKeepsTail(t *testing.T) {
	// "pkg/controllers/files.go" = 24 chars.
	// At width=20, prefixW=2, counterW=5 ("+1 -1"), labelBudget=12.
	// tailBudget=11 → last 11 chars = "rs/files.go" — "files.go" survives.
	const width = 20
	label := "pkg/controllers/files.go"
	r := fileRowDepth0(label, 1, 1)
	row := renderFileRow(r, width)

	if got := ansi.StringWidth(row); got != width {
		t.Errorf("row width = %d, want %d", got, width)
	}
	stripped := ansi.Strip(row)
	// The basename "files.go" must appear — left truncation preserves the tail.
	if !strings.Contains(stripped, "files.go") {
		t.Errorf("basename 'files.go' should survive left truncation: %q", stripped)
	}
	// The row ends with the counters, not an ellipsis.
	ct := counterText(1, 1)
	if !strings.HasSuffix(stripped, ct) {
		t.Errorf("row should end with counters %q, not ellipsis: %q", ct, stripped)
	}
}

// ── exact-fit boundary ────────────────────────────────────────────────────────

// At width=20, depth=0, non-viewed, +3 -2:
//
//	prefixW = 2 (space-marker + space), counterW = 5 ("+3 -2"), labelBudget = 12.
//
// "exact-twelve" is exactly 12 chars → should NOT be truncated.
func TestRenderFileRowExactFitBoundary(t *testing.T) {
	const width = 20
	label := "exact-twelve" // len=12, equals labelBudget
	r := fileRowDepth0(label, 3, 2)
	row := renderFileRow(r, width)

	if got := ansi.StringWidth(row); got != width {
		t.Errorf("row width = %d, want %d", got, width)
	}
	if strings.Contains(ansi.Strip(row), "…") {
		t.Errorf("label that exactly fits the budget should not be truncated: %q", ansi.Strip(row))
	}
}

// One column narrower than the exact-fit label triggers truncation, and the
// row is still exactly width columns with counters intact.
func TestRenderFileRowOneColumnNarrowerTruncates(t *testing.T) {
	const width = 19 // same label, one col less → labelBudget=11, label=12 → truncate
	label := "exact-twelve"
	r := fileRowDepth0(label, 3, 2)
	row := renderFileRow(r, width)

	if got := ansi.StringWidth(row); got != width {
		t.Errorf("row width = %d, want %d", got, width)
	}
	stripped := ansi.Strip(row)
	if !strings.Contains(stripped, "…") {
		t.Errorf("row should contain ellipsis after truncation: %q", stripped)
	}
	ct := counterText(3, 2)
	if !strings.Contains(stripped, ct) {
		t.Errorf("counters %q missing after truncation: %q", ct, stripped)
	}
}

// ── tiny width degrades to counters and never exceeds budget ──────────────────

func TestRenderFileRowTinyWidth(t *testing.T) {
	for _, width := range []int{1, 4} {
		r := fileRowDepth0("x.go", 5, 3)
		row := renderFileRow(r, width)
		if got := ansi.StringWidth(row); got > width {
			t.Errorf("width=%d: row width %d exceeds budget", width, got)
		}
	}
}

// ── indent grows two columns per depth level ──────────────────────────────────

func TestRenderFileRowIndentGrowsWithDepth(t *testing.T) {
	const width = 40
	for _, depth := range []int{0, 1, 2, 3} {
		r := fileRow{Label: "pkg/", Depth: depth, IsDir: true, FileIndex: -1, Adds: 1, Dels: 1}
		row := renderFileRow(r, width)

		if got := ansi.StringWidth(row); got != width {
			t.Errorf("depth %d: row width %d want %d", depth, got, width)
		}
		// Expanded dir marker is "▾"; it appears immediately after the indent
		// (2*depth spaces), confirming the indent width.
		stripped := ansi.Strip(row)
		wantPrefix := strings.Repeat(" ", 2*depth) + "▾"
		if !strings.HasPrefix(stripped, wantPrefix) {
			t.Errorf("depth %d: stripped row %q does not start with %q", depth, stripped, wantPrefix)
		}
	}
}

// ── directory and file markers ────────────────────────────────────────────────

func TestRenderFileRowMarkers(t *testing.T) {
	tests := []struct {
		name     string
		r        fileRow
		wantMark string
	}{
		{
			"dir collapsed",
			fileRow{Label: "pkg/", IsDir: true, Collapsed: true, FileIndex: -1},
			"▸",
		},
		{
			"dir expanded",
			fileRow{Label: "pkg/", IsDir: true, Collapsed: false, FileIndex: -1},
			"▾",
		},
		{
			"file viewed",
			fileRow{Label: "main.go", Viewed: true},
			"✓",
		},
		{
			"file unviewed",
			fileRow{Label: "main.go", Viewed: false},
			" ",
		},
	}

	for _, tc := range tests {
		row := renderFileRow(tc.r, 40)
		stripped := ansi.Strip(row)
		runes := []rune(stripped)
		if len(runes) == 0 {
			t.Fatalf("%s: empty row", tc.name)
		}
		// depth=0, so the marker is at rune index 0 (no indent).
		got := string(runes[0])
		if got != tc.wantMark {
			t.Errorf("%s: marker = %q, want %q (row: %q)", tc.name, got, tc.wantMark, stripped)
		}
	}
}

// ── headless path (width <= 0) ────────────────────────────────────────────────

// At width=0 (and negative), renderFileRow must return the FULL label without
// truncation or padding. Existing tests render the panel with no width budget
// and assert on substrings, so this contract is load-bearing.
func TestRenderFileRowHeadless(t *testing.T) {
	label := "very/long/path/that/would/be/truncated.go"
	r := fileRowDepth0(label, 7, 2)
	ct := counterText(7, 2)

	for _, width := range []int{0, -1, -100} {
		row := renderFileRow(r, width)
		stripped := ansi.Strip(row)

		// Full label must be present — no truncation.
		if !strings.Contains(stripped, label) {
			t.Errorf("width=%d: full label %q not in row %q", width, label, stripped)
		}
		// Counters must be present.
		if !strings.Contains(stripped, ct) {
			t.Errorf("width=%d: counters %q not in row %q", width, ct, stripped)
		}
		// Row must end with the counters — no trailing padding.
		if !strings.HasSuffix(stripped, ct) {
			t.Errorf("width=%d: row should end with counters %q, got %q", width, ct, stripped)
		}
		// Row must not be padded to any fixed width (should contain no trailing spaces).
		if strings.HasSuffix(stripped, " ") {
			t.Errorf("width=%d: headless row has trailing spaces: %q", width, stripped)
		}
	}
}

// ── table: exact column counts across widths ──────────────────────────────────

// Proves that ansi.StringWidth(row) == width for a range of budgets, including
// widths where the label fits, widths where it truncates, and widths tight
// enough to trigger the degenerate counter-only path.
func TestRenderFileRowExactWidthAcrossRange(t *testing.T) {
	// "very/long/path/controllers/files_controller.go" = 46 chars.
	// prefix=2, "+15 -8"=6 cols, so labelBudget=width-9.
	// label fits when width >= 55; truncates for 10 <= width < 55;
	// degenerate (labelBudget<=0) when width <= 9.
	r := fileRow{Label: "very/long/path/controllers/files_controller.go", Adds: 15, Dels: 8}

	for _, width := range []int{8, 9, 10, 15, 20, 30, 40, 54, 55, 60} {
		row := renderFileRow(r, width)
		got := ansi.StringWidth(row)
		if got != width {
			t.Errorf("width=%d: ansi.StringWidth(row) = %d (row: %q)",
				width, got, ansi.Strip(row))
		}
	}
}
