package ui

import (
	"strings"
	"testing"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// rowsFor renders foo.txt (fixture index 0) and returns the parallel lines/rows.
func rowsFor(t *testing.T, m Model) ([]string, []mainRow) {
	t.Helper()
	lines, rows := buildDiffRows(m, 0)
	if len(lines) != len(rows) {
		t.Fatalf("lines and rows must stay parallel: %d vs %d", len(lines), len(rows))
	}
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}
	return lines, rows
}

func findThreadHeader(rows []mainRow, id string) int {
	for i, r := range rows {
		if r.Kind == rowThreadHeader && r.ThreadID == id {
			return i
		}
	}
	return -1
}

// The model must stay exactly parallel to the lines, and the first row is always
// the file header — row 0 is what header-space keys off.
func TestBuildDiffRowsParallelAndHeader(t *testing.T) {
	m := seededModel(t)
	_, rows := rowsFor(t, m)
	if rows[0].Kind != rowFileHeader {
		t.Fatalf("row 0 must be the file header, got kind %d", rows[0].Kind)
	}
	if codeRenderIndex(m, 0) != -1 {
		t.Fatal("the file header is not a code row")
	}
}

// A thread block is injected directly after the code line it anchors to.
func TestThreadBlockFollowsItsAnchorLine(t *testing.T) {
	m := seededModel(t)
	lines, rows := rowsFor(t, m)

	hdr := findThreadHeader(rows, "t1")
	if hdr < 0 {
		t.Fatal("expected t1's thread header to be rendered inline")
	}
	if rows[hdr-1].Kind != rowCode {
		t.Fatalf("a thread block must sit right after a code row, got kind %d", rows[hdr-1].Kind)
	}

	// The anchor must be the render index AnchorThreads resolved for t1.
	var want int = -1
	for _, at := range m.AnchoredThreads {
		if at.Thread.ID == "t1" {
			want = at.RenderIndex
		}
	}
	if want < 0 {
		t.Fatal("fixture t1 should be anchored")
	}
	if got := rows[hdr-1].RenderIndex; got != want {
		t.Fatalf("thread anchored after render index %d, want %d", got, want)
	}

	// Unresolved threads default expanded, so the body is visible.
	if !strings.Contains(strings.Join(lines, "\n"), "first thread") {
		t.Fatal("an unresolved thread should render its comment body inline")
	}
}

// Code rows shift down once a block is injected, so RenderIndex+1 is no longer
// the Main line. mainLineForRenderIndex is what consumers must use.
func TestMainLineForRenderIndexAccountsForBlocks(t *testing.T) {
	m := seededModel(t)
	_, rows := rowsFor(t, m)
	m.MainRows = rows

	hdr := findThreadHeader(rows, "t1")
	anchor := rows[hdr-1].RenderIndex

	// A code row after the injected block must no longer be at RenderIndex+1.
	var shifted bool
	for i, r := range rows {
		if r.Kind == rowCode && r.RenderIndex > anchor {
			if i != r.RenderIndex+1 {
				shifted = true
			}
			break
		}
	}
	if !shifted {
		t.Fatal("expected code rows after a thread block to shift off RenderIndex+1")
	}
	if got := mainLineForRenderIndex(m, anchor); got != hdr-1 {
		t.Fatalf("mainLineForRenderIndex(%d) = %d, want %d", anchor, got, hdr-1)
	}
}

// Resolved and outdated threads collapse to their summary; unresolved stay open.
func TestSettledThreadsCollapseByDefault(t *testing.T) {
	m := seededModel(t)
	m.PRDetail.Threads[0].IsResolved = true
	m.PRDetail.Threads[0].Comments[0].Body = "settled talk"
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	lines, rows := rowsFor(t, m)
	if findThreadHeader(rows, "t1") < 0 {
		t.Fatal("a resolved thread should still show its summary line")
	}
	body := strings.Join(lines, "\n")
	if strings.Contains(body, "settled talk") {
		t.Fatal("a resolved thread should collapse its comments by default")
	}
	if !strings.Contains(body, "resolved") {
		t.Fatal("the summary should report the resolved state")
	}
}

// An explicit fold overrides either default, both directions.
func TestThreadFoldOverridesDefault(t *testing.T) {
	m := seededModel(t)

	// Collapse an unresolved thread.
	m.ThreadFolded = map[string]bool{"t1": true}
	lines, _ := rowsFor(t, m)
	if strings.Contains(strings.Join(lines, "\n"), "first thread") {
		t.Fatal("folding an unresolved thread should hide its comments")
	}

	// Expand a resolved one.
	m2 := seededModel(t)
	m2.PRDetail.Threads[0].IsResolved = true
	m2.AnchoredThreads, m2.UnresolvedThreadIndex = buildAnchors(m2.PRDetail, m2.DiffFiles)
	m2.ThreadFolded = map[string]bool{"t1": false}
	lines2, _ := rowsFor(t, m2)
	if !strings.Contains(strings.Join(lines2, "\n"), "first thread") {
		t.Fatal("unfolding a resolved thread should reveal its comments")
	}
}

// File-level comments and outdated threads have no diff line to attach to. They
// must still appear — right below the file header — or they are invisible here.
func TestUnanchoredThreadsRenderAfterFileHeader(t *testing.T) {
	m := seededModel(t)
	m.PRDetail.Threads = append(m.PRDetail.Threads, domain.Thread{
		ID:       "tfile",
		Path:     "foo.txt",
		DiffSide: "RIGHT",
		Line:     nil, // file-level: never anchors
		Comments: []domain.Comment{{Author: "adrian", Body: "whole-file note"}},
	})
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	lines, rows := rowsFor(t, m)
	hdr := findThreadHeader(rows, "tfile")
	if hdr < 0 {
		t.Fatal("an unanchored thread must still be rendered")
	}
	// It must land in the header block, before any code row.
	for i := 0; i < hdr; i++ {
		if rows[i].Kind == rowCode {
			t.Fatal("unanchored threads must precede all code rows")
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "whole-file note") {
		t.Fatal("expected the unanchored thread's body")
	}
}

// A range thread anchors at its END line (AnchorThreads resolves Thread.Line), so
// placement alone cannot convey the span — the summary must state it.
func TestRangeThreadSummaryShowsSpan(t *testing.T) {
	m := seededModel(t)
	start := 1
	m.PRDetail.Threads[0].StartLine = &start // Line is 3 in the fixture
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	lines, rows := rowsFor(t, m)
	hdr := findThreadHeader(rows, "t1")
	if hdr < 0 {
		t.Fatal("expected the range thread to render")
	}
	if !strings.Contains(lines[hdr], "lines 1-3") {
		t.Fatalf("range summary should state the span, got %q", lines[hdr])
	}
}

// A pending (draft) comment is marked inline, so you can see unsubmitted work in
// the diff before pressing S.
func TestDraftCommentMarkedInline(t *testing.T) {
	m := seededModel(t)
	m.PRDetail.Threads[0].Comments[0].State = "PENDING"
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	lines, _ := rowsFor(t, m)
	if !strings.Contains(strings.Join(lines, "\n"), "[draft]") {
		t.Fatal("a pending comment should be marked [draft] inline")
	}
}

// Thread rows are never code rows: line-scoped actions must not resolve an anchor
// from them.
func TestThreadRowsAreNotCodeRows(t *testing.T) {
	m := seededModel(t)
	_, rows := rowsFor(t, m)
	m.MainRows = rows
	for i, r := range rows {
		if r.Kind == rowThreadHeader || r.Kind == rowComment {
			if codeRenderIndex(m, i) != -1 {
				t.Fatalf("row %d is a thread row but resolved a code anchor", i)
			}
			if r.ThreadID == "" {
				t.Fatalf("row %d is a thread row with no thread id", i)
			}
		}
	}
}
