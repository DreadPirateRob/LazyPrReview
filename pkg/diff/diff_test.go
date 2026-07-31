package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

const sampleDiff = `diff --git a/foo.txt b/foo.txt
index 1111111..2222222 100644
--- a/foo.txt
+++ b/foo.txt
@@ -2,2 +2,3 @@
 line two
-line three
+line three changed
+line four
 line five
`

func TestParseHunkMath(t *testing.T) {
	files, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	file := files[0]
	if file.Path != "foo.txt" {
		t.Fatalf("expected foo.txt, got %q", file.Path)
	}
	if len(file.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(file.Hunks))
	}
	h := file.Hunks[0]
	if h.OldStart != 2 || h.OldLines != 2 || h.NewStart != 2 || h.NewLines != 3 {
		t.Fatalf("unexpected hunk math: %+v", h)
	}
	if len(file.Rendered) != 7 {
		t.Fatalf("expected 7 rendered lines, got %d", len(file.Rendered))
	}
}

func TestAnchorThreadsRightAndLeft(t *testing.T) {
	files, _ := Parse(sampleDiff)
	rightLine := 3
	leftLine := 3
	threads := []domain.Thread{
		{ID: "r", Path: "foo.txt", Line: &rightLine, DiffSide: "RIGHT"},
		{ID: "l", Path: "foo.txt", Line: &leftLine, DiffSide: "LEFT"},
	}
	anchored := AnchorThreads(files, threads)
	if !anchored[0].Anchored || anchored[0].RenderIndex < 0 {
		t.Fatalf("expected RIGHT thread to anchor: %+v", anchored[0])
	}
	if !anchored[1].Anchored || anchored[1].RenderIndex < 0 {
		t.Fatalf("expected LEFT thread to anchor: %+v", anchored[1])
	}
}

func TestAnchorThreadsOutdatedAndUnanchored(t *testing.T) {
	files, _ := Parse(sampleDiff)
	missing := 99
	threads := []domain.Thread{
		{ID: "o", Path: "foo.txt", Line: nil, DiffSide: "RIGHT", IsOutdated: true},
		{ID: "u", Path: "foo.txt", Line: &missing, DiffSide: "RIGHT"},
	}
	anchored := AnchorThreads(files, threads)
	if anchored[0].Badge != "[outdated]" || anchored[0].Anchored {
		t.Fatalf("expected outdated panel-only thread, got %+v", anchored[0])
	}
	if anchored[1].Badge != "[unanchored]" || anchored[1].Anchored {
		t.Fatalf("expected unanchored panel-only thread, got %+v", anchored[1])
	}
}

func TestBuildUnresolvedIndexOrdering(t *testing.T) {
	files, _ := Parse(sampleDiff + "\ndiff --git a/bar.txt b/bar.txt\n--- a/bar.txt\n+++ b/bar.txt\n@@ -1 +1 @@\n-old\n+new\n")
	r1 := 3
	r2 := 1
	anchored := AnchorThreads(files, []domain.Thread{
		{ID: "two", Path: "bar.txt", Line: &r2, DiffSide: "RIGHT"},
		{ID: "one", Path: "foo.txt", Line: &r1, DiffSide: "RIGHT"},
		{ID: "resolved", Path: "foo.txt", Line: &r1, DiffSide: "RIGHT", IsResolved: true},
	})
	index := BuildUnresolvedIndex(files, anchored)
	if len(index) != 2 {
		t.Fatalf("expected 2 indexed threads, got %d", len(index))
	}
	if index[0].Thread.ID != "one" || index[1].Thread.ID != "two" {
		t.Fatalf("unexpected order: %#v", index)
	}
}

func TestCursorSide(t *testing.T) {
	if got := CursorSide(LineKindDel); got != "LEFT" {
		t.Fatalf("expected LEFT, got %q", got)
	}
	if got := CursorSide(LineKindAdd); got != "RIGHT" {
		t.Fatalf("expected RIGHT, got %q", got)
	}
	if got := CursorSide(LineKindContext); got != "RIGHT" {
		t.Fatalf("expected RIGHT, got %q", got)
	}
}

func TestRealDiffFixturesParse(t *testing.T) {
	for _, path := range []string{"../../testdata/diff/5731.diff", "../../testdata/diff/5732.diff"} {
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		files, err := Parse(string(data))
		if err != nil {
			t.Fatalf("Parse(%s): %v", path, err)
		}
		if len(files) == 0 {
			t.Fatalf("expected parsed files for %s", path)
		}
		hunks := 0
		for _, file := range files {
			hunks += len(file.Hunks)
		}
		if hunks == 0 {
			t.Fatalf("expected parsed hunks for %s", path)
		}
	}
}
