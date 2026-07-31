package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
)

// ── fixtures ──────────────────────────────────────────────────────────────────

// goTwoHunkDiff is a two-hunk Go diff used by several tests.
const goTwoHunkDiff = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,5 +1,5 @@
 package main
 
 func hello() {
-	fmt.Println("hello")
+	fmt.Println("world")
 }
@@ -10,4 +10,4 @@
 func bye() {
-	fmt.Println("bye")
+	fmt.Println("goodbye")
 }
`

// parseTestDiff parses a raw diff and returns the first File or fatals.
func parseTestDiff(t *testing.T, raw string) diff.File {
	t.Helper()
	files, err := diff.Parse(raw)
	if err != nil {
		t.Fatalf("diff.Parse: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("diff.Parse: no files")
	}
	return files[0]
}

// ── length invariant ──────────────────────────────────────────────────────────

// TestDiffHighlightLengthInvariant verifies the load-bearing contract:
// len(highlightedDiffLines(file)) == len(file.Rendered) for a diff that mixes
// context, add, del lines across multiple hunks.
func TestDiffHighlightLengthInvariant(t *testing.T) {
	file := parseTestDiff(t, goTwoHunkDiff)
	hl := highlightedDiffLines(file)
	if len(hl) != len(file.Rendered) {
		t.Fatalf("length mismatch: highlightedDiffLines=%d, file.Rendered=%d",
			len(hl), len(file.Rendered))
	}
}

// TestDiffHighlightLengthInvariantAllKinds uses a fixture that exercises every
// LineKind including file headers and hunk headers.
func TestDiffHighlightLengthInvariantAllKinds(t *testing.T) {
	raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
 package a
-var x = 1
+var x = 2
 // end
@@ -10,2 +10,2 @@
-var y = 1
+var y = 2
`
	file := parseTestDiff(t, raw)
	hl := highlightedDiffLines(file)
	if len(hl) != len(file.Rendered) {
		t.Fatalf("length mismatch: got %d, want %d", len(hl), len(file.Rendered))
	}
	// File-header and hunk-header entries must always be "".
	for _, l := range file.Rendered {
		switch l.Kind {
		case diff.LineKindFileHeader, diff.LineKindHunkHeader:
			if hl[l.RenderIndex] != "" {
				t.Errorf("header line %d (kind=%s) must produce \"\", got %q",
					l.RenderIndex, l.Kind, hl[l.RenderIndex])
			}
		}
	}
}

// ── ANSI output for Go source ─────────────────────────────────────────────────

// TestDiffHighlightGoFileProducesANSI verifies that a .go diff produces ANSI
// color sequences in the highlighted code text. Skips if the active color
// profile emits no escapes (matches the precedent in TestHighlightSelectionKeepsFgColor).
func TestDiffHighlightGoFileProducesANSI(t *testing.T) {
	sample := diffAddStyle.Render("+")
	if !strings.Contains(sample, "\x1b[") {
		t.Skip("color profile emits no styling; ANSI test is a no-op")
	}

	file := parseTestDiff(t, goTwoHunkDiff)
	hl := highlightedDiffLines(file)

	sawColor := false
	for _, l := range file.Rendered {
		if l.Kind == diff.LineKindContext || l.Kind == diff.LineKindAdd || l.Kind == diff.LineKindDel {
			if strings.Contains(hl[l.RenderIndex], "\x1b[") {
				sawColor = true
				break
			}
		}
	}
	if !sawColor {
		t.Error("expected at least one code line with ANSI escapes from Go syntax highlighting")
	}
}

// ── unknown-path fallback ──────────────────────────────────────────────────────

// TestDiffHighlightUnknownPathFallback verifies that a file with an
// unrecognised extension returns all-"" highlights, and that renderDiffLine
// with those "" values produces output byte-identical to the pre-change renderer
// (which colored marker + text together with the kind style).
func TestDiffHighlightUnknownPathFallback(t *testing.T) {
	rawDiff := `diff --git a/data.xq7z9 b/data.xq7z9
--- a/data.xq7z9
+++ b/data.xq7z9
@@ -1,3 +1,3 @@
 ctx line
-del line
+add line
`
	file := parseTestDiff(t, rawDiff)
	hl := highlightedDiffLines(file)

	if len(hl) != len(file.Rendered) {
		t.Fatalf("length: got %d, want %d", len(hl), len(file.Rendered))
	}
	for i, s := range hl {
		if s != "" {
			t.Errorf("entry %d: expected \"\" for unknown extension, got %q", i, s)
		}
	}

	// With all-"" highlights renderDiffLine must fall back to exact pre-change output.
	for _, line := range file.Rendered {
		got := renderDiffLine(line, "")
		var want string
		gutter := diffMetaStyle.Render(fmt.Sprintf("%4s %4s │ ", diffLineNo(line.OldNo), diffLineNo(line.NewNo)))
		switch line.Kind {
		case diff.LineKindAdd:
			want = gutter + diffAddStyle.Render("+ "+line.Text)
		case diff.LineKindDel:
			want = gutter + diffDelStyle.Render("- "+line.Text)
		case diff.LineKindContext:
			want = gutter + "  " + line.Text
		case diff.LineKindHunkHeader:
			want = diffHunkStyle.Render(line.Text)
		case diff.LineKindFileHeader:
			want = diffMetaStyle.Render(line.Text)
		default:
			continue
		}
		if got != want {
			t.Errorf("fallback line %d (kind=%s): output mismatch\n  got:  %q\n  want: %q",
				line.RenderIndex, line.Kind, got, want)
		}
	}
}

// ── multi-line state preservation ─────────────────────────────────────────────

// TestDiffHighlightMultiLineBlockComment verifies that lexer state is preserved
// across context lines within the same hunk. The interior lines of a block
// comment (/* ... */) depend on the lexer having seen the opening /* on a
// preceding line; a per-line re-lex would start fresh and classify those lines
// differently.
func TestDiffHighlightMultiLineBlockComment(t *testing.T) {
	rawDiff := `diff --git a/y.go b/y.go
--- a/y.go
+++ b/y.go
@@ -1,6 +1,6 @@
 package p
 /*
  * line one inside comment
  * line two inside comment
  */
-func old() {}
+func new() {}
`
	file := parseTestDiff(t, rawDiff)
	hl := highlightedDiffLines(file)

	// Find the two "* line N inside comment" context lines.
	var commentLines []string
	for _, l := range file.Rendered {
		if l.Kind == diff.LineKindContext && strings.Contains(l.Text, "inside comment") {
			commentLines = append(commentLines, hl[l.RenderIndex])
		}
	}
	if len(commentLines) < 2 {
		t.Fatalf("expected 2 comment-interior context lines, got %d", len(commentLines))
	}
	if commentLines[0] == "" {
		t.Skip("highlighting inactive (no lexer or color profile)")
	}

	// Both interior lines are inside the same /* */ block: they must receive
	// consistent highlighting (same leading ANSI sequence = same token type).
	// Extract the ANSI prefix up to the first visible character.
	leadingEscapes := func(s string) string {
		i := 0
		for i < len(s) {
			if s[i] != '\x1b' {
				break
			}
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
			} else {
				break
			}
		}
		return s[:i]
	}

	p0 := leadingEscapes(commentLines[0])
	p1 := leadingEscapes(commentLines[1])
	if p0 != p1 {
		t.Errorf("block-comment interior lines have different color prefixes — state not preserved across lines:\n  line1: %q\n  line2: %q", commentLines[0], commentLines[1])
	}

	// Additionally verify that the highlighted line differs from lexing the
	// same text completely in isolation (which would not see the opening /*).
	lex := lexers.Match("y.go")
	if lex == nil {
		return // lexer not available in this environment
	}
	lex = chroma.Coalesce(lex)
	sty := styles.Get("monokai")
	if sty == nil {
		sty = styles.Fallback
	}
	isolated := highlightSourceLines(lex, sty, []string{" * line one inside comment"})
	if len(isolated) == 0 {
		return // isolated highlight failed
	}
	if commentLines[0] == isolated[0] {
		t.Errorf("comment interior highlighted identically to isolated lex — state not preserved across lines:\n  in-context: %q\n  isolated:   %q", commentLines[0], isolated[0])
	}
}

// ── hunk-boundary reset ───────────────────────────────────────────────────────

// TestDiffHighlightHunkResetPreventsBleed is the key regression guard for the
// per-hunk segmentation strategy. When hunk 1 ends with an unclosed /* block
// comment, a naive whole-file tokenisation would let the comment color bleed
// into hunk 2. Per-hunk reset prevents this: hunk 2 is tokenised independently
// and its output must match a file that contains only hunk 2.
func TestDiffHighlightHunkResetPreventsBleed(t *testing.T) {
	// Two-hunk file: hunk 1 ends with an unclosed block comment.
	rawTwoHunks := `diff --git a/bleed.go b/bleed.go
--- a/bleed.go
+++ b/bleed.go
@@ -1,3 +1,3 @@
 package p
 /* unclosed block comment
-var old = 1
+var old = 2
@@ -10,3 +10,3 @@
 func target() {
-	return 0
+	return 1
 }
`
	// Isolated file with only hunk 2's code.
	rawHunk2Only := `diff --git a/bleed.go b/bleed.go
--- a/bleed.go
+++ b/bleed.go
@@ -10,3 +10,3 @@
 func target() {
-	return 0
+	return 1
 }
`
	files2, err := diff.Parse(rawTwoHunks)
	if err != nil {
		t.Fatalf("parse two-hunk: %v", err)
	}
	filesIso, err := diff.Parse(rawHunk2Only)
	if err != nil {
		t.Fatalf("parse isolated: %v", err)
	}
	if len(files2) == 0 || len(filesIso) == 0 {
		t.Fatal("expected parsed files")
	}

	hlTwo := highlightedDiffLines(files2[0])
	hlIso := highlightedDiffLines(filesIso[0])

	// Skip if highlighting is off.
	anyHighlighted := false
	for _, s := range hlTwo {
		if s != "" {
			anyHighlighted = true
			break
		}
	}
	if !anyHighlighted {
		t.Skip("highlighting inactive (no lexer or color profile)")
	}

	// Collect hunk-2 content lines from the two-hunk file (HunkIndex == 1).
	var hunk2A []string
	for _, l := range files2[0].Rendered {
		if l.HunkIndex == 1 && isContentKind(l.Kind) {
			hunk2A = append(hunk2A, hlTwo[l.RenderIndex])
		}
	}
	// Collect the equivalent lines from the isolated file (HunkIndex == 0).
	var hunk2B []string
	for _, l := range filesIso[0].Rendered {
		if l.HunkIndex == 0 && isContentKind(l.Kind) {
			hunk2B = append(hunk2B, hlIso[l.RenderIndex])
		}
	}

	if len(hunk2A) != len(hunk2B) {
		t.Fatalf("hunk-2 line count mismatch: two-hunk=%d isolated=%d", len(hunk2A), len(hunk2B))
	}
	for i := range hunk2A {
		if hunk2A[i] != hunk2B[i] {
			t.Errorf("hunk-2 line %d: bleed from hunk 1 detected\n  two-hunk: %q\n  isolated: %q",
				i, hunk2A[i], hunk2B[i])
		}
	}
}

func isContentKind(k diff.LineKind) bool {
	return k == diff.LineKindContext || k == diff.LineKindAdd || k == diff.LineKindDel
}

// ── marker coloring ────────────────────────────────────────────────────────────

// TestDiffHighlightMarkersKindColored verifies that even when syntax
// highlighting is active, the +/- markers remain colored by diffAddStyle /
// diffDelStyle (so add/del lines stay identifiable at a glance).
func TestDiffHighlightMarkersKindColored(t *testing.T) {
	sampleAdd := diffAddStyle.Render("+")
	hasColor := strings.Contains(sampleAdd, "\x1b[")

	file := parseTestDiff(t, goTwoHunkDiff)
	hl := highlightedDiffLines(file)

	for _, line := range file.Rendered {
		switch line.Kind {
		case diff.LineKindAdd:
			rendered := renderDiffLine(line, hl[line.RenderIndex])
			stripped := ansi.Strip(rendered)
			if !strings.Contains(stripped, "+ ") {
				t.Errorf("add line missing '+ ' marker: %q", stripped)
			}
			if hasColor {
				// The standalone marker must carry the add color.
				marker := diffAddStyle.Render("+ ")
				if hl[line.RenderIndex] != "" && !strings.Contains(rendered, marker) {
					t.Errorf("add line: marker not colored with diffAddStyle\n  rendered: %q\n  want to contain: %q",
						rendered, marker)
				}
			}
		case diff.LineKindDel:
			rendered := renderDiffLine(line, hl[line.RenderIndex])
			stripped := ansi.Strip(rendered)
			if !strings.Contains(stripped, "- ") {
				t.Errorf("del line missing '- ' marker: %q", stripped)
			}
		}
	}
}

// ── performance ───────────────────────────────────────────────────────────────

// TestDiffHighlightPerformance measures the wall-clock cost of
// highlightedDiffLines on a synthetic ~3000-line Go diff. The result is
// reported via t.Log so it appears with -v. No hard limit is asserted here;
// the benchmark below is the authoritative measurement.
func TestDiffHighlightPerformance(t *testing.T) {
	file := syntheticGoDiff(3000)
	if len(file.Rendered) == 0 {
		t.Fatal("synthetic diff produced no lines")
	}

	start := time.Now()
	hl := highlightedDiffLines(file)
	elapsed := time.Since(start)

	t.Logf("highlightedDiffLines on %d-line Go diff (rendered=%d): %v",
		3000, len(file.Rendered), elapsed)

	if len(hl) != len(file.Rendered) {
		t.Fatalf("length invariant violated: got %d, want %d", len(hl), len(file.Rendered))
	}
}

// BenchmarkHighlightedDiffLines3kLines is the authoritative per-render
// benchmark used to decide whether a cache is warranted (threshold: ~50ms).
func BenchmarkHighlightedDiffLines3kLines(b *testing.B) {
	file := syntheticGoDiff(3000)
	b.ResetTimer()
	for b.Loop() {
		_ = highlightedDiffLines(file)
	}
}

// syntheticGoDiff builds a diff.File for a synthetic Go source file with
// approximately totalLines rendered content lines. The result has a single hunk
// with roughly (totalLines-2) context lines plus one del and one add.
func syntheticGoDiff(totalLines int) diff.File {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/bench.go b/bench.go\n--- a/bench.go\n+++ b/bench.go\n")

	// Each hunk: (totalLines/chunkSize) chunks so lexer segments are realistic.
	const chunkSize = 300
	lineNum := 1
	remaining := totalLines

	for remaining > 0 {
		size := chunkSize
		if size > remaining {
			size = remaining
		}
		end := lineNum + size - 1
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", lineNum, size, lineNum, size)
		for j := 0; j < size-2; j++ {
			// Realistic Go lines: mix of declarations and comments.
			switch j % 5 {
			case 0:
				fmt.Fprintf(&sb, " func fn%d() int { return %d }\n", lineNum+j, lineNum+j)
			case 1:
				fmt.Fprintf(&sb, " // comment line %d\n", lineNum+j)
			case 2:
				fmt.Fprintf(&sb, " var v%d = \"%d\"\n", lineNum+j, lineNum+j)
			case 3:
				fmt.Fprintf(&sb, " const c%d = %d\n", lineNum+j, lineNum+j)
			default:
				fmt.Fprintf(&sb, " type T%d struct{ X int }\n", lineNum+j)
			}
		}
		fmt.Fprintf(&sb, "-\tfmt.Println(\"old line %d\")\n", end-1)
		fmt.Fprintf(&sb, "+\tfmt.Println(\"new line %d\")\n", end-1)
		lineNum = end + 1
		remaining -= size
	}

	files, err := diff.Parse(sb.String())
	if err != nil || len(files) == 0 {
		panic(fmt.Sprintf("syntheticGoDiff: %v", err))
	}
	return files[0]
}
