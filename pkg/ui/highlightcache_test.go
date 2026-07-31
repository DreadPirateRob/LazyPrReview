package ui

import (
	"fmt"
	"strings"
	"testing"
)

// The cache originally keyed on path + rendered-line count, which collides in two
// ordinary situations: another PR touching the same file with the same line count,
// and a refetch after the author edited a line in place. Either served colours
// computed from code no longer on screen.
func TestDiffHighlightCacheKeyTracksLineText(t *testing.T) {
	const before = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-var x = "one"
+var x = "two"
`
	// Same path, same rendered-line count, one changed line.
	after := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-var y = 41
+var y = 42
`
	a := parseTestDiff(t, before)
	b := parseTestDiff(t, after)

	if len(a.Rendered) != len(b.Rendered) {
		t.Fatalf("fixtures must have equal rendered counts to test the collision, got %d and %d",
			len(a.Rendered), len(b.Rendered))
	}
	if diffHighlightKey(a) == diffHighlightKey(b) {
		t.Fatal("different code at the same path and line count must not share a cache key")
	}
}

// Highlighting resets lexer state at hunk boundaries, so a different split must not
// reuse a cached result. It happens that hunk headers are themselves rendered lines
// whose text encodes the ranges, so the split is already visible to the fingerprint
// through them — HunkIndex in the key is belt-and-braces for the day diff.Parse
// stops emitting header lines. Either way, this is the property that must hold.
func TestDiffHighlightCacheKeyTracksHunkBoundaries(t *testing.T) {
	const oneHunk = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,4 +1,4 @@
 package main
-var a = 1
+var a = 2
 var b = 3
`
	const twoHunks = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-var a = 1
+var a = 2
@@ -9,1 +9,1 @@
 var b = 3
`
	a := parseTestDiff(t, oneHunk)
	b := parseTestDiff(t, twoHunks)

	if got, want := a.Rendered[len(a.Rendered)-1].HunkIndex, b.Rendered[len(b.Rendered)-1].HunkIndex; got == want {
		t.Fatalf("fixtures must actually differ in hunk split, both ended at hunk %d", got)
	}
	if diffHighlightKey(a) == diffHighlightKey(b) {
		t.Fatal("a different hunk split changes the highlight and must not share a cache key")
	}
}

// Above the floor, highlighting is skipped — but the length invariant that the row
// model depends on must still hold, or every cursor and thread anchor shifts.
func TestDiffHighlightSkippedAboveLineFloor(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n")
	sb.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", diffHighlightMaxLines+10, diffHighlightMaxLines+10))
	for i := 0; i < diffHighlightMaxLines+10; i++ {
		sb.WriteString(fmt.Sprintf(" var x%d = %d\n", i, i))
	}
	file := parseTestDiff(t, sb.String())

	if len(file.Rendered) <= diffHighlightMaxLines {
		t.Fatalf("fixture must exceed the floor, got %d rendered lines", len(file.Rendered))
	}
	hl := highlightedDiffLines(file)
	if len(hl) != len(file.Rendered) {
		t.Fatalf("length invariant must hold even when skipping, got %d want %d", len(hl), len(file.Rendered))
	}
	for i, h := range hl {
		if h != "" {
			t.Fatalf("line %d should be unhighlighted above the floor, got %q", i, h)
		}
	}
}
