package diff

import "testing"

func splitFile(t *testing.T, raw string) File {
	t.Helper()
	files, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("Parse returned no files")
	}
	return files[0]
}

// A context line is the same code on both sides, so it must appear twice.
func TestSplitRowsPairsContextOnBothSides(t *testing.T) {
	f := splitFile(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
 package main
-var a = 1
+var a = 2
`)
	rows := SplitRows(f)

	var ctx *SplitRow
	for i := range rows {
		if !rows[i].FullWidth && rows[i].Left != nil && rows[i].Left.Kind == LineKindContext {
			ctx = &rows[i]
			break
		}
	}
	if ctx == nil {
		t.Fatal("expected a context row")
	}
	if ctx.Left == nil || ctx.Right == nil {
		t.Fatal("a context row must occupy both sides")
	}
	if ctx.Left.Text != ctx.Right.Text {
		t.Fatalf("context sides must be the same line, got %q and %q", ctx.Left.Text, ctx.Right.Text)
	}
}

// A modification must sit opposite the line it replaces, not below it.
func TestSplitRowsZipsDeletionsAgainstAdditions(t *testing.T) {
	f := splitFile(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
-one
-two
+uno
+dos
 tail
`)
	rows := SplitRows(f)

	var pairs [][2]string
	for _, r := range rows {
		if r.FullWidth || r.Left == nil || r.Left.Kind != LineKindDel {
			continue
		}
		right := ""
		if r.Right != nil {
			right = r.Right.Text
		}
		pairs = append(pairs, [2]string{r.Left.Text, right})
	}
	want := [][2]string{{"one", "uno"}, {"two", "dos"}}
	if len(pairs) != len(want) {
		t.Fatalf("expected %d zipped rows, got %d (%v)", len(want), len(pairs), pairs)
	}
	for i, w := range want {
		if pairs[i] != w {
			t.Errorf("row %d: got %v, want %v", i, pairs[i], w)
		}
	}
}

// Uneven runs leave filler rather than shifting the shorter side out of alignment.
func TestSplitRowsLeavesFillerForUnevenRuns(t *testing.T) {
	f := splitFile(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,1 +1,3 @@
-only
+first
+second
+third
`)
	rows := SplitRows(f)

	var leftFilled, rightOnly int
	for _, r := range rows {
		if r.FullWidth {
			continue
		}
		if r.Left != nil && r.Right != nil {
			leftFilled++
		}
		if r.Left == nil && r.Right != nil {
			rightOnly++
		}
	}
	if leftFilled != 1 {
		t.Errorf("the single deletion should pair with the first addition, got %d paired rows", leftFilled)
	}
	if rightOnly != 2 {
		t.Errorf("the two surplus additions need empty left sides, got %d", rightOnly)
	}
}

// Runs in different hunks are unrelated code and must never be zipped together.
func TestSplitRowsNeverPairsAcrossHunks(t *testing.T) {
	f := splitFile(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,1 +1,1 @@
-alpha
@@ -20,1 +20,1 @@
+omega
`)
	rows := SplitRows(f)

	for _, r := range rows {
		if r.FullWidth || r.Left == nil || r.Right == nil {
			continue
		}
		if r.Left.HunkIndex != r.Right.HunkIndex {
			t.Fatalf("paired lines from hunks %d and %d", r.Left.HunkIndex, r.Right.HunkIndex)
		}
	}
	// The deletion and the addition live in different hunks, so each keeps a side.
	var lone int
	for _, r := range rows {
		if !r.FullWidth && (r.Left == nil) != (r.Right == nil) {
			lone++
		}
	}
	if lone != 2 {
		t.Fatalf("expected the deletion and addition to stay unpaired, got %d lone rows", lone)
	}
}

// Headers describe the whole row, not one revision.
func TestSplitRowsMarksHeadersFullWidth(t *testing.T) {
	f := splitFile(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,1 +1,1 @@
-x
+y
`)
	rows := SplitRows(f)

	for _, r := range rows {
		if r.Left == nil {
			continue
		}
		switch r.Left.Kind {
		case LineKindFileHeader, LineKindHunkHeader:
			if !r.FullWidth {
				t.Errorf("%s must be full width", r.Left.Kind)
			}
			if r.Right != nil {
				t.Errorf("%s must not occupy the right column", r.Left.Kind)
			}
		}
	}
}

// Every rendered line has to survive the transform, or the view silently omits code.
func TestSplitRowsLosesNoLines(t *testing.T) {
	f := splitFile(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,4 +1,4 @@
 keep
-drop
+gain
 tail
@@ -10,2 +10,2 @@
 ctx
-old
+new
`)
	seen := map[int]bool{}
	for _, r := range SplitRows(f) {
		if r.Left != nil {
			seen[r.Left.RenderIndex] = true
		}
		if r.Right != nil {
			seen[r.Right.RenderIndex] = true
		}
	}
	for i, line := range f.Rendered {
		if !seen[line.RenderIndex] {
			t.Errorf("rendered line %d (%s %q) is missing from the split view", i, line.Kind, line.Text)
		}
	}
}
