package ui

import (
	"strings"
	"testing"
)

// A draft written while reading a file diff must appear inline immediately.
// rebuildWithOptimistic used to rebuild Main only in overview mode, so the new
// comment stayed invisible until the reader left and re-entered the file.
func TestOptimisticDraftAppearsInlineOnDiff(t *testing.T) {
	m := seededModel(t)
	m = setMainDiff(m, 0) // foo.txt
	m.MainMode = MainDiff
	m.MainFileIndex = 0
	before := strings.Join(m.MainLines, "\n")
	if strings.Contains(before, "inline draft body") {
		t.Fatal("precondition: body should not be present yet")
	}

	cs := composeState{
		Kind: composeComment, SubjectType: "LINE", ClientID: "c1",
		Path: "foo.txt", Line: 3, Side: "RIGHT",
	}
	got := insertOptimisticDraft(m, cs, "inline draft body")

	body := strings.Join(got.MainLines, "\n")
	if !strings.Contains(body, "inline draft body") {
		t.Fatal("a new draft should render inline without leaving the file")
	}
	if !strings.Contains(body, "[draft]") {
		t.Fatal("an optimistic draft should be marked [draft] inline")
	}
	if len(got.MainRows) != len(got.MainLines) {
		t.Fatalf("row model must stay parallel after refresh: %d vs %d", len(got.MainRows), len(got.MainLines))
	}
}

// Rolling the draft back (the add failed) must remove it from the diff again,
// via the same rebuild path.
func TestFailedDraftRemovedFromDiff(t *testing.T) {
	m := seededModel(t)
	m = setMainDiff(m, 0)
	m.MainMode = MainDiff
	m.MainFileIndex = 0

	cs := composeState{
		Kind: composeComment, SubjectType: "LINE", ClientID: "c1",
		Path: "foo.txt", Line: 3, Side: "RIGHT",
	}
	m = insertOptimisticDraft(m, cs, "doomed draft")
	if !strings.Contains(strings.Join(m.MainLines, "\n"), "doomed draft") {
		t.Fatal("precondition: draft should be inline")
	}

	// This is what the failure branch does.
	delete(m.OptimisticDrafts, "c1")
	m = rebuildWithOptimistic(m)

	if strings.Contains(strings.Join(m.MainLines, "\n"), "doomed draft") {
		t.Fatal("a rolled-back draft must disappear from the inline diff")
	}
}

// Refreshing must keep the reader in place: the cursor stays on the same code
// line even though a thread block inserted above it shifts every row down.
func TestRefreshKeepsCursorOnSameCodeLine(t *testing.T) {
	m := seededModel(t)
	m = setMainDiff(m, 0)
	m.MainMode = MainDiff
	m.MainFileIndex = 0

	// Park the cursor on the last code row of the file.
	target := -1
	for i, r := range m.MainRows {
		if r.Kind == rowCode {
			target = i
		}
	}
	if target < 0 {
		t.Fatal("expected code rows")
	}
	m.MainCursor = target
	wantRender := m.MainRows[target].RenderIndex

	// Add a draft anchored ABOVE the cursor so rows below it shift.
	cs := composeState{
		Kind: composeComment, SubjectType: "LINE", ClientID: "c2",
		Path: "foo.txt", Line: 3, Side: "RIGHT",
	}
	got := insertOptimisticDraft(m, cs, "shifting note")

	if gotRender := codeRenderIndex(got, got.MainCursor); gotRender != wantRender {
		t.Fatalf("cursor should stay on render index %d, got %d", wantRender, gotRender)
	}
}
