package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

func TestThreadFocusReplyOpensComposer(t *testing.T) {
	m := threadFocusModel(t, []domain.Comment{{ID: "c1", Body: "hi"}})
	m.PRDetail.Threads[0].ViewerCanReply = true
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	updated, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "r", Code: 'r'})
	if updated.CurrentFocus() != FocusCompose {
		t.Fatal("r should open the reply composer")
	}
	if updated.Compose.Kind != composeReply || updated.Compose.ThreadID != "th1" {
		t.Fatalf("expected a reply targeting th1, got %+v", updated.Compose)
	}
}

func TestThreadFocusReplyBlockedWhenCannotReply(t *testing.T) {
	m := threadFocusModel(t, []domain.Comment{{ID: "c1", Body: "hi"}})
	// ViewerCanReply defaults false
	updated, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "r", Code: 'r'})
	if updated.CurrentFocus() == FocusCompose {
		t.Fatal("r must not reply to a thread the viewer can't reply to")
	}
	if updated.Toast.Message == "" {
		t.Fatal("expected a cannot-reply toast")
	}
}

func TestThreadReplyCmdEnsuresThenReplies(t *testing.T) {
	f := fakeforge.New()
	msg := threadReplyCmd(f, "PR1", "OID", "", "TH1", "my reply", composeState{})().(replyResultMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if len(f.EnsurePendingCalls) != 1 {
		t.Fatalf("should ensure a review when none given, got %d", len(f.EnsurePendingCalls))
	}
	if len(f.AddThreadReplyCalls) != 1 || f.AddThreadReplyCalls[0].ThreadID != "TH1" || f.AddThreadReplyCalls[0].Body != "my reply" {
		t.Fatalf("should reply to the thread, got %+v", f.AddThreadReplyCalls)
	}
	if msg.ReviewID == "" {
		t.Fatal("result should carry the resolved review id")
	}
}

func TestThreadReplyCmdReusesReviewID(t *testing.T) {
	f := fakeforge.New()
	msg := threadReplyCmd(f, "PR1", "OID", "R5", "TH1", "reply", composeState{})().(replyResultMsg)
	if len(f.EnsurePendingCalls) != 0 {
		t.Fatal("should not ensure when a review id is supplied")
	}
	if f.AddThreadReplyCalls[0].ReviewID != "R5" {
		t.Fatalf("should reply on the given review, got %q", f.AddThreadReplyCalls[0].ReviewID)
	}
	if msg.ReviewID != "R5" {
		t.Fatalf("should carry the review id, got %q", msg.ReviewID)
	}
}

func TestReplyResultErrorReopensWithThreadIntact(t *testing.T) {
	m := seededModel(t)
	updated, _ := m.Update(replyResultMsg{
		Err: errString("boom"), Body: "retry me", ReviewID: "R7",
		Compose: composeState{Kind: composeReply, ThreadID: "th9"},
	})
	g := updated.(Model)
	if g.CurrentFocus() != FocusCompose {
		t.Fatal("a failed reply should reopen the composer")
	}
	if g.Composer.Value() != "retry me" {
		t.Fatalf("body should be restored, got %q", g.Composer.Value())
	}
	if g.Compose.ThreadID != "th9" {
		t.Fatalf("thread target must survive the failure, got %q", g.Compose.ThreadID)
	}
	if g.PendingReviewID != "R7" {
		t.Fatal("should cache the review id even on failure (avoid re-ensuring)")
	}
}

func TestMainEnterFocusesThreadUnderCursor(t *testing.T) {
	m, addIdx, _ := diffModel(t) // x.go diff, MainMode=MainDiff, MainFileIndex=0
	l1, l2 := 1, 2
	m.PRDetail = &domain.PRDetail{Threads: []domain.Thread{
		{ID: "tA", Path: "x.go", Line: &l1, DiffSide: "RIGHT", Comments: []domain.Comment{{ID: "a1", Body: "A"}}},
		{ID: "tB", Path: "x.go", Line: &l2, DiffSide: "RIGHT", Comments: []domain.Comment{{ID: "b1", Body: "B"}}},
	}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.ThreadTab = ThreadsTabAll
	m.ThreadsPanel.Cursor = 1 // stale panel selection points elsewhere
	m.MainCursor = addIdx     // Main cursor sits on tA's anchor (the add line)
	m.FocusStack = []FocusContext{FocusMain}
	g, _ := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if g.CurrentFocus() != FocusThread {
		t.Fatal("enter on a thread anchor should focus the thread")
	}
	if at, ok := focusedThread(g); !ok || at.Thread.ID != "tA" {
		t.Fatalf("should focus the thread under the Main cursor (tA), got %q ok=%v", at.Thread.ID, ok)
	}
}

func TestMainEnterOffThreadDoesNotFocus(t *testing.T) {
	m, _, _ := diffModel(t)
	l := 1
	m.PRDetail = &domain.PRDetail{Threads: []domain.Thread{
		{ID: "tA", Path: "x.go", Line: &l, DiffSide: "RIGHT", Comments: []domain.Comment{{Body: "A"}}},
	}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.MainCursor = 0 // file-header line, not a thread anchor
	m.FocusStack = []FocusContext{FocusMain}
	updated, _ := UpdateMain(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.CurrentFocus() == FocusThread {
		t.Fatal("enter off a thread anchor must not focus a stale thread")
	}
}
