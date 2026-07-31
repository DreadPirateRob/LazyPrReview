package ui

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
)

const bandDiff = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,4 +1,4 @@
 package main
-var a = "one"
+var a = "two"
 var b = 3
`

func bandProbe(t *testing.T) (add, del, ctx diff.RenderedLine) {
	t.Helper()
	file := parseTestDiff(t, bandDiff)
	var got int
	for _, l := range file.Rendered {
		switch l.Kind {
		case diff.LineKindAdd:
			add, got = l, got+1
		case diff.LineKindDel:
			del, got = l, got+1
		case diff.LineKindContext:
			if ctx.Kind == "" {
				ctx, got = l, got+1
			}
		}
	}
	if got < 3 {
		t.Fatalf("fixture needs an add, a del and a context line, found %d", got)
	}
	return add, del, ctx
}

func skipWithoutColour(t *testing.T) (addOpen, delOpen string) {
	t.Helper()
	addOpen, delOpen = bandOpener(diffAddBandStyle), bandOpener(diffDelBandStyle)
	if addOpen == "" || delOpen == "" {
		t.Skip("colour profile emits no styling; there is no band to assert")
	}
	return addOpen, delOpen
}

// The reported problem: with the code syntax-coloured, the only signal for which
// side a line was on was the one-column +/- glyph.
func TestDiffBandMarksChangedRows(t *testing.T) {
	addOpen, delOpen := skipWithoutColour(t)
	add, del, ctx := bandProbe(t)

	// If both sides tinted the same, every other assertion here would still pass
	// while the band told you nothing — which is the bug being fixed.
	if addOpen == delOpen {
		t.Fatalf("added and deleted rows must use distinct tints, both are %q", addOpen)
	}

	if got := renderDiffLine(add, "", 60); !strings.Contains(got, addOpen) {
		t.Errorf("added row must carry the add band, got %q", got)
	}
	if got := renderDiffLine(del, "", 60); !strings.Contains(got, delOpen) {
		t.Errorf("deleted row must carry the del band, got %q", got)
	}
	got := renderDiffLine(ctx, "", 60)
	if strings.Contains(got, addOpen) || strings.Contains(got, delOpen) {
		t.Errorf("context row must stay unbanded, got %q", got)
	}
}

// The band spans the full row, gutter included: leaving the line numbers on the
// neutral background would just move the "one column tells me" problem over.
func TestDiffBandSpansFullRowWidth(t *testing.T) {
	skipWithoutColour(t)
	add, _, _ := bandProbe(t)

	const width = 60
	got := renderDiffLine(add, "", width)
	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("banded row should occupy exactly %d columns, got %d", width, w)
	}
	if !strings.HasPrefix(ansi.Strip(got), "   ") {
		t.Fatalf("row should still open with the line-number gutter, got %q", ansi.Strip(got))
	}
}

// Banding is about diff status, not lexer availability: a file chroma cannot lex
// still has to show which lines changed.
func TestDiffBandAppliesWithoutLexer(t *testing.T) {
	addOpen, delOpen := skipWithoutColour(t)
	file := parseTestDiff(t, `diff --git a/data.xq7z9 b/data.xq7z9
--- a/data.xq7z9
+++ b/data.xq7z9
@@ -1,3 +1,3 @@
 ctx line
-del line
+add line
`)
	hl := highlightedDiffLines(file)

	for i, line := range file.Rendered {
		if hl[i] != "" {
			t.Fatalf("fixture must have no lexer, entry %d was highlighted", i)
		}
		got := renderDiffLine(line, hl[i], 60)
		switch line.Kind {
		case diff.LineKindAdd:
			if !strings.Contains(got, addOpen) {
				t.Errorf("unlexed added row still needs its band, got %q", got)
			}
			if !strings.Contains(ansi.Strip(got), "+ add line") {
				t.Errorf("marker and text must survive the band, got %q", ansi.Strip(got))
			}
		case diff.LineKindDel:
			if !strings.Contains(got, delOpen) {
				t.Errorf("unlexed deleted row still needs its band, got %q", got)
			}
		}
	}
}

// No width means no room to band — the row must degrade to bare content rather than
// emit a zero-width background or pad to a bogus size.
func TestDiffBandSkippedWithoutWidth(t *testing.T) {
	addOpen, _ := skipWithoutColour(t)
	add, _, _ := bandProbe(t)

	for _, width := range []int{0, -1} {
		got := renderDiffLine(add, "", width)
		if strings.Contains(got, addOpen) {
			t.Errorf("width %d cannot be banded, got %q", width, got)
		}
	}
}

// Exactly one background owns a row. A selected changed row reads as selected, with
// the band suppressed — including across the resets chroma emits between tokens.
func TestSelectionSuppressesDiffBand(t *testing.T) {
	addOpen, _ := skipWithoutColour(t)
	selOpen := bandOpener(focusedSelectStyle)
	if selOpen == "" {
		t.Skip("no selection styling to assert")
	}
	add, _, _ := bandProbe(t)

	// A body carrying several resets, as syntax-highlighted code does.
	code := "\x1b[38;5;81mvar\x1b[0m a = \x1b[38;5;186m\"two\"\x1b[0m"
	row := renderDiffLine(add, code, 60)
	if !strings.Contains(row, addOpen) {
		t.Fatalf("precondition: row should be banded, got %q", row)
	}
	if n := strings.Count(row, "\x1b[0m"); n < 2 {
		t.Fatalf("precondition: body should contain several resets, got %d", n)
	}

	selected := highlightSelection("> "+row, 0, 60, true, 1)
	if strings.Contains(selected, addOpen) {
		t.Errorf("selection must suppress the diff band, band opener still present: %q", selected)
	}
	if !strings.Contains(selected, selOpen) {
		t.Errorf("selected row must carry the selection background, got %q", selected)
	}
}

// The band tints come from the chroma style's own diff entries. monokai — the style
// we render with — sets both equal to its page background, which would paint an
// invisible band, so that case MUST take the explicit fallback. Without this test the
// guard reads as redundant and the bands quietly disappear when someone drops it.
func TestDiffBandColourFallsBackWhenStyleHasNoDistinctTint(t *testing.T) {
	sty := styles.Get(highlightStyleName)
	if sty == nil {
		t.Fatalf("style %q must exist", highlightStyleName)
	}
	page := sty.Get(chroma.Background).Background
	inserted := sty.Get(chroma.GenericInserted).Background
	if inserted != page {
		t.Skipf("style %q now defines a distinct inserted background (%s); the fallback branch is no longer reachable",
			highlightStyleName, inserted.String())
	}

	const fallback = "#123456"
	if got := diffBandColour(chroma.GenericInserted, fallback); got != lipgloss.Color(fallback) {
		t.Fatalf("a diff tint equal to the page background must fall back, got %v", got)
	}
}
