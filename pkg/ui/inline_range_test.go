package ui

import (
	"strings"
	"testing"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

const rangeSampleDiff = "diff --git a/r.go b/r.go\n--- a/r.go\n+++ b/r.go\n@@ -0,0 +1,3 @@\n+one\n+two\n+three\n"

// rangeModel renders r.go with a thread anchored on the MIDDLE added line, so a
// selection across all three lines necessarily spans an inline thread block.
func rangeModel(t *testing.T) (Model, int, int) {
	t.Helper()
	files, err := diff.Parse(rangeSampleDiff)
	if err != nil {
		t.Fatal(err)
	}
	mid := 2
	m := New(config.Default(), fakeforge.New())
	m.DiffFiles = files
	m.PRDetail = &domain.PRDetail{
		ID:    "PR1",
		Files: []domain.ChangedFile{{Path: "r.go"}},
		Threads: []domain.Thread{{
			ID: "tr", Path: "r.go", Line: &mid, DiffSide: "RIGHT",
			Comments: []domain.Comment{{Author: "reviewer", Body: "THREADTEXT"}},
		}},
	}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, files)
	m.MainMode = MainDiff
	m.MainFileIndex = 0
	m = setMainDiff(m, 0)

	// Main lines of the first and last added rows.
	first, last := -1, -1
	for i, r := range m.MainRows {
		if r.Kind != rowCode {
			continue
		}
		rl := files[0].Rendered[r.RenderIndex]
		if rl.Kind != diff.LineKindAdd {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 || last <= first {
		t.Fatalf("fixture should yield multiple added rows, got first=%d last=%d", first, last)
	}
	if findThreadHeader(m.MainRows, "tr") < 0 {
		t.Fatal("fixture should render the thread inline")
	}
	return m, first, last
}

// A range spanning an inline thread block is still a contiguous run of code, so
// it must be commentable — the block consumes Main rows but no file lines.
func TestRangeSpanningThreadBlockIsAllowed(t *testing.T) {
	m, first, last := rangeModel(t)

	// Sanity: the block really is inside the selection.
	hdr := findThreadHeader(m.MainRows, "tr")
	if hdr <= first || hdr >= last {
		t.Fatalf("thread block should sit inside [%d,%d], got %d", first, last, hdr)
	}

	m.MainCursor = first
	m.MainRangeActive = true
	m.MainRangeStart = first
	m.MainRangeGen = m.MainGen
	m.MainCursor = last

	got, _ := openRangeComment(m)
	if got.CurrentFocus() != FocusCompose {
		t.Fatalf("a range spanning a thread block should open the composer, toast=%q", got.Toast.Message)
	}
	// The anchor must cover the whole code span (lines 1..3), not stop at the block.
	if got.Compose.Line != 3 {
		t.Fatalf("range end line should be 3, got %d", got.Compose.Line)
	}
	if got.Compose.StartLine == nil || *got.Compose.StartLine != 1 {
		t.Fatalf("range start line should be 1, got %v", got.Compose.StartLine)
	}
}

// The composer preview must show exactly what will be submitted: code rows only.
// Thread rows are skipped when resolving the span, so previewing them would lie.
func TestRangePreviewExcludesThreadRows(t *testing.T) {
	m, first, last := rangeModel(t)
	m.MainRangeActive = true
	m.MainRangeStart = first
	m.MainRangeGen = m.MainGen
	m.MainCursor = last

	got, _ := openRangeComment(m)
	preview := strings.Join(got.Compose.Preview, "\n")
	if strings.Contains(preview, "THREADTEXT") {
		t.Fatalf("preview must not include inline thread rows:\n%s", preview)
	}
	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview should contain code line %q:\n%s", want, preview)
		}
	}
}
