package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// threadInDiffModel renders a two-comment thread inline in a diff and returns the
// model plus the first Main row of each comment.
func threadInDiffModel(t *testing.T) (m Model, c0, c1 int) {
	t.Helper()
	m, addIdx, _ := diffModel(t)
	line := 1
	m.PRDetail = &domain.PRDetail{Threads: []domain.Thread{{
		ID: "tA", Path: "x.go", Line: &line, DiffSide: "RIGHT",
		Comments: []domain.Comment{
			{ID: "c1", Author: "bob", Body: "first"},
			{ID: "c2", Author: "alice", Body: "second"},
		},
	}}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.ThreadTab = ThreadsTabAll
	m.MainCursor = addIdx
	m.FocusStack = []FocusContext{FocusMain}
	m = refreshMainDiff(m) // emit the inline comment rows

	c0, n0 := mainLineSpanForComment(m, "tA", 0)
	c1, n1 := mainLineSpanForComment(m, "tA", 1)
	if n0 == 0 || n1 == 0 {
		t.Fatalf("fixture must render both comments inline, got spans %d and %d", n0, n1)
	}
	return m, c0, c1
}

// The reported bug: enter swapped the hint bar to the thread bindings and changed
// nothing else on screen. Assert the selection the renderer actually consumes —
// comparing whole frames would pass even against the broken build, because the
// hint bar was precisely the one thing that DID change.
func TestThreadFocusSelectionCoversWholeComment(t *testing.T) {
	m, c0, _ := threadInDiffModel(t)
	_, want := mainLineSpanForComment(m, "tA", 0)
	if want < 2 {
		t.Fatalf("fixture comment should span several rows to be worth covering, got %d", want)
	}

	// Before: the Threads panel holds focus, so Main marks a single cursor row.
	if _, span := mainSelection(m.PushFocus(FocusThreads), FocusThreads); span != 1 {
		t.Fatalf("unfocused Main should select one row, got %d", span)
	}

	got := enterThreadFocus(m, "tA")
	anchor, span := mainSelection(got, FocusThread)
	if anchor != 1+c0 {
		t.Fatalf("selection should start at the comment's first row %d, got %d", 1+c0, anchor)
	}
	if span != want {
		t.Fatalf("selection should cover all %d rows of the comment, got %d", want, span)
	}
}

// A range selection (v) must keep precedence over the thread-focus span.
func TestThreadFocusSelectionYieldsToRange(t *testing.T) {
	m, _, _ := threadInDiffModel(t)
	m = enterThreadFocus(m, "tA")
	m.MainRangeStart = m.MainCursor
	m.MainRangeActive = true
	m.MainRangeGen = m.MainGen
	m.MainCursor += 2

	if _, span := mainSelection(m, FocusThread); span != 3 {
		t.Fatalf("an active range should win over the comment span, got %d", span)
	}
}

func TestThreadFocusPutsCursorOnComment(t *testing.T) {
	m, c0, _ := threadInDiffModel(t)

	got := enterThreadFocus(m, "tA")
	if got.MainCursor != c0 {
		t.Fatalf("cursor should land on the focused comment row %d, got %d", c0, got.MainCursor)
	}
	if got.ThreadCursor != 0 {
		t.Fatalf("entering from the code anchor starts at the first comment, got %d", got.ThreadCursor)
	}
}

// Entering from the second comment must focus the second, not reset to the top.
func TestThreadFocusKeepsCommentUnderCursor(t *testing.T) {
	m, _, c1 := threadInDiffModel(t)
	m.MainCursor = c1

	got, _ := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got.CurrentFocus() != FocusThread {
		t.Fatalf("enter on a comment row should focus the thread, got %s", got.CurrentFocus())
	}
	if got.ThreadCursor != 1 {
		t.Fatalf("should focus the comment under the cursor (index 1), got %d", got.ThreadCursor)
	}
	if got.MainCursor != c1 {
		t.Fatalf("cursor should stay on comment 1 at row %d, got %d", c1, got.MainCursor)
	}
}

// j/k move an index that used to be invisible; they must move the Main cursor too.
func TestThreadFocusJKWalksComments(t *testing.T) {
	m, c0, c1 := threadInDiffModel(t)
	m = enterThreadFocus(m, "tA")

	down, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if down.ThreadCursor != 1 {
		t.Fatalf("j should advance to comment 1, got %d", down.ThreadCursor)
	}
	if down.MainCursor != c1 {
		t.Fatalf("j should move the Main cursor to row %d, got %d", c1, down.MainCursor)
	}

	up, _ := UpdateThreadFocus(down, tea.KeyPressMsg{Text: "k", Code: 'k'})
	if up.ThreadCursor != 0 || up.MainCursor != c0 {
		t.Fatalf("k should return to comment 0 at row %d, got cursor %d row %d", c0, up.ThreadCursor, up.MainCursor)
	}
}

// A resolved thread renders collapsed, so focusing it would otherwise have no
// comment rows to point the cursor at. The enter tests above use an unresolved
// thread, which is expanded by default and never exercises this path.
func TestThreadFocusExpandsCollapsedThread(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	line := 1
	m.PRDetail = &domain.PRDetail{Threads: []domain.Thread{{
		ID: "tR", Path: "x.go", Line: &line, DiffSide: "RIGHT", IsResolved: true,
		Comments: []domain.Comment{{ID: "r1", Author: "bob", Body: "settled"}},
	}}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.MainCursor = addIdx
	m.FocusStack = []FocusContext{FocusMain}
	m = refreshMainDiff(m)

	if _, n := mainLineSpanForComment(m, "tR", 0); n != 0 {
		t.Fatal("a resolved thread should start collapsed, with no comment rows")
	}

	got := enterThreadFocus(m, "tR")
	start, n := mainLineSpanForComment(got, "tR", 0)
	if n == 0 {
		t.Fatal("focusing a collapsed thread must expand it")
	}
	if got.MainCursor != start {
		t.Fatalf("cursor should land on the revealed comment at row %d, got %d", start, got.MainCursor)
	}
}

// The render path hides the Threads panel's focus border during thread focus, but
// focusedPanelIndex must keep reporting 3: layout.go allocates panel heights with
// it, so "simplifying" it to -1 would silently resize the pane stack.
func TestThreadFocusKeepsLayoutPanelIndex(t *testing.T) {
	if got := focusedPanelIndex(FocusThread); got != 3 {
		t.Fatalf("layout sizes the Threads panel from index 3, got %d", got)
	}
}
