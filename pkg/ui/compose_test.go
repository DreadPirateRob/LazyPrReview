package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

const anchorSampleDiff = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,2 @@\n-old\n+new\n unchanged\n"

// diffModel renders x.go through the real Main path and returns the MAIN LINE of
// the first add and del rows. It must go through setMainDiff: the row model is
// what resolves anchors now, and inline thread blocks mean a code line is no
// longer at renderIndex+1.
func diffModel(t *testing.T) (Model, int, int) {
	t.Helper()
	files, err := diff.Parse(anchorSampleDiff)
	if err != nil {
		t.Fatal(err)
	}
	m := New(config.Default(), fakeforge.New())
	m.DiffFiles = files
	m.MainMode = MainDiff
	m.MainFileIndex = 0
	m = setMainDiff(m, 0)
	addIdx, delIdx := -1, -1
	for i, rl := range files[0].Rendered {
		switch rl.Kind {
		case diff.LineKindAdd:
			if addIdx < 0 {
				addIdx = i
			}
		case diff.LineKindDel:
			if delIdx < 0 {
				delIdx = i
			}
		}
	}
	if addIdx < 0 || delIdx < 0 {
		t.Fatalf("fixture needs an add and a del line, got add=%d del=%d", addIdx, delIdx)
	}
	addLine, delLine := mainLineForRenderIndex(m, addIdx), mainLineForRenderIndex(m, delIdx)
	if addLine < 0 || delLine < 0 {
		t.Fatalf("add/del rows must be present in the row model: add=%d del=%d", addLine, delLine)
	}
	return m, addLine, delLine
}

func TestMainCommentAnchorSides(t *testing.T) {
	m, addIdx, delIdx := diffModel(t)

	m.MainCursor = delIdx // lines[0] is the file header
	if p, _, side, ok := mainCommentAnchor(m); !ok || side != "LEFT" || p != "x.go" {
		t.Fatalf("del line should anchor LEFT on x.go: path=%q side=%q ok=%v", p, side, ok)
	}
	m.MainCursor = addIdx
	if _, _, side, ok := mainCommentAnchor(m); !ok || side != "RIGHT" {
		t.Fatalf("add line should anchor RIGHT: side=%q ok=%v", side, ok)
	}
}

func TestMainCommentAnchorDisabledUnderPager(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.Config.GUI.DiffPager = "delta --paging=never"
	m.MainCursor = addIdx
	if _, _, _, ok := mainCommentAnchor(m); ok {
		t.Fatal("an external diff pager must disable line-comment anchoring")
	}
}

func TestComposerBodyRequired(t *testing.T) {
	m := seededModel(t)
	m = openComposer(m, composeState{Kind: composeComment, Required: true, SubjectType: "LINE", Path: "foo.txt", Line: 3, Side: "RIGHT"})
	m.Composer.SetValue("   ")
	updated, cmd := submitCompose(m)
	if cmd != nil {
		t.Fatal("empty body must not submit")
	}
	if updated.CurrentFocus() != FocusCompose {
		t.Fatal("composer should stay open on validation failure")
	}
	if updated.Compose.Err == "" {
		t.Fatal("expected a validation error message")
	}
}

func TestCommentSubmitEnsuresThenAdds(t *testing.T) {
	m := seededModel(t)
	f := m.Forge.(*fakeforge.Fake)
	m.PRDetail.PendingReview = nil

	m = openComposer(m, composeState{Kind: composeComment, Required: true, SubjectType: "LINE", Path: "foo.txt", Line: 3, Side: "RIGHT"})
	m.Composer.SetValue("looks good")
	m, cmd := submitCompose(m)
	if cmd == nil {
		t.Fatal("expected a submit command")
	}
	if len(m.OptimisticDrafts) != 1 || optimisticUnconfirmed(m) != 1 {
		t.Fatalf("expected 1 optimistic draft shown, got %d (unconfirmed %d)", len(m.OptimisticDrafts), optimisticUnconfirmed(m))
	}
	msg, ok := cmd().(commentResultMsg)
	if !ok || !msg.OK {
		t.Fatalf("expected OK result, got %+v", msg)
	}
	if len(f.EnsurePendingCalls) != 1 {
		t.Fatalf("expected exactly one EnsurePendingReview, got %d", len(f.EnsurePendingCalls))
	}
	if len(f.AddThreadCalls) != 1 {
		t.Fatalf("expected one AddReviewThread, got %d", len(f.AddThreadCalls))
	}
	at := f.AddThreadCalls[0]
	if at.Path != "foo.txt" || at.Line != 3 || at.Side != "RIGHT" || at.Body != "looks good" {
		t.Fatalf("wrong thread input: %+v", at)
	}
	if at.ReviewID != msg.ReviewID || msg.ReviewID == "" {
		t.Fatalf("thread should attach to the ensured review: at=%q msg=%q", at.ReviewID, msg.ReviewID)
	}

	updated, rc := m.Update(msg)
	g := updated.(Model)
	if rc == nil {
		t.Fatal("a successful comment should trigger a background reconcile")
	}
	if g.PendingReviewID != msg.ReviewID {
		t.Fatalf("review id should be cached: got %q want %q", g.PendingReviewID, msg.ReviewID)
	}
	if !g.OptimisticDrafts[msg.ClientID].Confirmed {
		t.Fatal("the draft should be marked confirmed pending reconcile")
	}
}

func TestCommentReusesExistingPendingReview(t *testing.T) {
	m := seededModel(t)
	f := m.Forge.(*fakeforge.Fake)
	m.PRDetail.PendingReview = &domain.Review{ID: "R_existing"}

	m = openComposer(m, composeState{Kind: composeComment, Required: true, SubjectType: "FILE", Path: "foo.txt"})
	m.Composer.SetValue("file note")
	_, cmd := submitCompose(m)
	msg := cmd().(commentResultMsg)
	if !msg.OK {
		t.Fatalf("expected OK, got %+v", msg)
	}
	if len(f.EnsurePendingCalls) != 0 {
		t.Fatalf("must not create a review when one exists, got %d ensures", len(f.EnsurePendingCalls))
	}
	if f.AddThreadCalls[0].ReviewID != "R_existing" {
		t.Fatalf("should attach to the existing review, got %q", f.AddThreadCalls[0].ReviewID)
	}
}

func TestCommentCachedReviewIDAvoidsSecondEnsure(t *testing.T) {
	m := seededModel(t)
	f := m.Forge.(*fakeforge.Fake)
	m.PRDetail.PendingReview = nil

	m = openComposer(m, composeState{Kind: composeComment, Required: true, SubjectType: "FILE", Path: "foo.txt"})
	m.Composer.SetValue("one")
	m, cmd := submitCompose(m)
	updated, _ := m.Update(cmd().(commentResultMsg))
	g := updated.(Model)

	g = openComposer(g, composeState{Kind: composeComment, Required: true, SubjectType: "FILE", Path: "bar.txt"})
	g.Composer.SetValue("two")
	_, cmd2 := submitCompose(g)
	if _, ok := cmd2().(commentResultMsg); !ok {
		t.Fatal("expected a comment result")
	}
	if len(f.EnsurePendingCalls) != 1 {
		t.Fatalf("cached review id should avoid a second ensure, got %d", len(f.EnsurePendingCalls))
	}
	if len(f.AddThreadCalls) != 2 {
		t.Fatalf("expected two draft threads, got %d", len(f.AddThreadCalls))
	}
}

func TestCommentErrorRollsBackAndReopens(t *testing.T) {
	m := seededModel(t)
	f := m.Forge.(*fakeforge.Fake)
	m.PRDetail.PendingReview = &domain.Review{ID: "R1"}
	f.AddThreadErr = errString("boom")

	m = openComposer(m, composeState{Kind: composeComment, Required: true, SubjectType: "LINE", Path: "foo.txt", Line: 3, Side: "RIGHT"})
	m.Composer.SetValue("will fail")
	m, cmd := submitCompose(m)
	msg := cmd().(commentResultMsg)
	if msg.Err == nil {
		t.Fatal("expected an error result")
	}
	updated, _ := m.Update(msg)
	g := updated.(Model)
	if len(g.OptimisticDrafts) != 0 {
		t.Fatalf("a failed comment must roll back its optimistic draft, got %d", len(g.OptimisticDrafts))
	}
	if g.CurrentFocus() != FocusCompose {
		t.Fatal("the composer should reopen so the text isn't lost")
	}
	if g.Composer.Value() != "will fail" {
		t.Fatalf("body should be restored, got %q", g.Composer.Value())
	}
	if g.Toast.Message == "" {
		t.Fatal("expected a failure toast")
	}
}

func TestFilesPanelCOpensFileLevelComposer(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusFiles}
	m.FilesPanel.Cursor = 0
	updated, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "c", Code: 'c'})
	if updated.CurrentFocus() != FocusCompose {
		t.Fatal("c in Files should open the composer")
	}
	if updated.Compose.SubjectType != "FILE" || updated.Compose.Path == "" {
		t.Fatalf("expected a file-level comment target, got %+v", updated.Compose)
	}
}

func TestMainRangeCommentSingleSide(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.MainCursor = addIdx
	m.MainRangeActive = true
	m.MainRangeStart = m.MainCursor
	m.MainRangeGen = m.MainGen
	m.MainCursor = addIdx + 1 // extend to the following RIGHT (context) row
	updated, _ := openRangeComment(m)
	if updated.CurrentFocus() != FocusCompose {
		t.Fatal("a same-side range should open the composer")
	}
	if updated.Compose.StartLine == nil {
		t.Fatal("a multi-line comment must set StartLine")
	}
	if updated.Compose.Side != "RIGHT" || updated.Compose.StartSide != "RIGHT" {
		t.Fatalf("expected RIGHT/RIGHT, got %q/%q", updated.Compose.Side, updated.Compose.StartSide)
	}
}

func TestMainRangeCommentRejectsMixedSide(t *testing.T) {
	m, addIdx, delIdx := diffModel(t)
	lo, hi := delIdx, addIdx
	if lo > hi {
		lo, hi = hi, lo
	}
	m.MainRangeActive = true
	m.MainRangeStart = lo
	m.MainRangeGen = m.MainGen
	m.MainCursor = hi
	updated, _ := openRangeComment(m)
	if updated.CurrentFocus() == FocusCompose {
		t.Fatal("a selection crossing LEFT and RIGHT must not open a composer")
	}
	if updated.Toast.Message == "" {
		t.Fatal("expected a mixed-side toast")
	}
}

func TestMainVTogglesRange(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.FocusStack = []FocusContext{FocusMain}
	m.MainCursor = addIdx
	on, _ := UpdateMain(m, tea.KeyPressMsg{Text: "v", Code: 'v'})
	if !mainRangeActive(on) {
		t.Fatal("v should start a range")
	}
	off, _ := UpdateMain(on, tea.KeyPressMsg{Text: "v", Code: 'v'})
	if mainRangeActive(off) {
		t.Fatal("v again should cancel the range")
	}
}

func TestMainRangeInvalidatedByContentChange(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.MainCursor = addIdx
	m.MainRangeActive = true
	m.MainRangeStart = m.MainCursor
	m.MainRangeGen = m.MainGen
	if !mainRangeActive(m) {
		t.Fatal("range should be active before a content change")
	}
	m = setMainLines(m, m.MainLines)
	if mainRangeActive(m) {
		t.Fatal("any Main content rewrite must invalidate the stale range")
	}
}

func TestCommitDiffModeDisablesComment(t *testing.T) {
	m, addIdx, _ := diffModel(t)
	m.FocusStack = []FocusContext{FocusMain}
	m.MainFileIndex = -1 // commit-diff sentinel set by commitDiffLoadedMsg
	m.MainCursor = addIdx
	if afterC, _ := UpdateMain(m, tea.KeyPressMsg{Text: "c", Code: 'c'}); afterC.CurrentFocus() == FocusCompose {
		t.Fatal("c must not open a composer in commit-diff mode (no PR-file anchor)")
	}
	if afterV, _ := UpdateMain(m, tea.KeyPressMsg{Text: "v", Code: 'v'}); mainRangeActive(afterV) {
		t.Fatal("v must not arm a range in commit-diff mode")
	}
}

func TestDraftsRowMarkerNotDoubled(t *testing.T) {
	m := seededModel(t)
	// An orphan pending-review comment (present only in the pending review, not
	// surfaced as a thread) — draftThreads synthesizes a row for it.
	m.PRDetail.PendingReview = &domain.Review{ID: "R1", Comments: []domain.Comment{
		{ID: "pc1", State: "PENDING", Path: "foo.txt", Body: "orphan draft"},
	}}
	m.FocusStack = []FocusContext{FocusThreads}
	m.ThreadTab = ThreadsTabDrafts
	view := ThreadsView(m)
	if n := strings.Count(view, "[draft]"); n != 1 {
		t.Fatalf("expected exactly one [draft] marker (from compactThread), got %d in:\n%q", n, view)
	}
	if !strings.Contains(view, "[unanchored]") {
		t.Fatal("orphan draft row should carry the [unanchored] anchoring badge")
	}
}
