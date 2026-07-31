package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

func threadFocusModel(t *testing.T, comments []domain.Comment) Model {
	t.Helper()
	m := seededModel(t)
	m.PRDetail.Threads = []domain.Thread{{ID: "th1", Path: "foo.txt", Comments: comments}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.FocusStack = []FocusContext{FocusThreads}
	m.ThreadTab = ThreadsTabAll
	m.ThreadsPanel.Cursor = 0
	m.ThreadCursor = 0
	return m.PushFocus(FocusThread)
}

func TestThreadFocusEditOwnComment(t *testing.T) {
	m := threadFocusModel(t, []domain.Comment{
		{ID: "c1", Author: "me", ViewerDidAuthor: true, Body: "my note", State: "PENDING"},
	})
	updated, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "e", Code: 'e'})
	if updated.CurrentFocus() != FocusCompose {
		t.Fatal("e on own comment should open the edit composer")
	}
	if updated.Compose.Kind != composeEdit || updated.Compose.CommentID != "c1" {
		t.Fatalf("expected an edit of c1, got %+v", updated.Compose)
	}
	if updated.Composer.Value() != "my note" {
		t.Fatalf("composer should prefill the existing body, got %q", updated.Composer.Value())
	}
}

func TestThreadFocusEditBlockedOnOthersComment(t *testing.T) {
	m := threadFocusModel(t, []domain.Comment{
		{ID: "c1", Author: "bob", ViewerDidAuthor: false, Body: "their note"},
	})
	updated, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "e", Code: 'e'})
	if updated.CurrentFocus() == FocusCompose {
		t.Fatal("e must not edit someone else's comment")
	}
	if updated.Toast.Message == "" {
		t.Fatal("expected a not-your-comment toast")
	}
}

func TestThreadFocusDeletePendingRoutesToRef(t *testing.T) {
	m := threadFocusModel(t, []domain.Comment{
		{ID: "c1", ViewerDidAuthor: true, State: "PENDING"},
	})
	f := m.Forge.(*fakeforge.Fake)
	confirm, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "d", Code: 'd'})
	if confirm.CurrentFocus() != FocusMenu {
		t.Fatal("d should open a delete-confirm menu")
	}
	if !confirm.DeleteTarget.Pending || confirm.DeleteTarget.ID != "c1" {
		t.Fatalf("delete target should be the pending comment, got %+v", confirm.DeleteTarget)
	}
	_, cmd := applyMenuItem(confirm, MenuItem{Kind: menuKindDeleteConfirm})
	if cmd == nil {
		t.Fatal("confirming delete should dispatch a delete command")
	}
	if msg := cmd().(deleteResultMsg); msg.Err != nil {
		t.Fatalf("unexpected delete error: %v", msg.Err)
	}
	if len(f.DeleteCommentCalls) != 1 || !f.DeleteCommentCalls[0].Pending {
		t.Fatalf("should delete via the pending ref, got %+v", f.DeleteCommentCalls)
	}
}

func TestEditCmdUpdatesComment(t *testing.T) {
	f := fakeforge.New()
	msg := editCmd(f, "c1", "new body", composeState{})().(editResultMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if len(f.UpdateCommentCalls) != 1 || f.UpdateCommentCalls[0].ID != "c1" || f.UpdateCommentCalls[0].Body != "new body" {
		t.Fatalf("should update the comment body, got %+v", f.UpdateCommentCalls)
	}
}

func TestEditResultReopensComposerOnError(t *testing.T) {
	m := seededModel(t)
	updated, _ := m.Update(editResultMsg{Err: errString("boom"), Body: "fix it", Compose: composeState{Kind: composeEdit, CommentID: "c1"}})
	g := updated.(Model)
	if g.CurrentFocus() != FocusCompose {
		t.Fatal("a failed edit should reopen the composer")
	}
	if g.Composer.Value() != "fix it" {
		t.Fatalf("body should be restored, got %q", g.Composer.Value())
	}
}

func TestDeleteResultClearsTargetOnBothPaths(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"success", nil}, {"failure", errString("boom")}} {
		m := seededModel(t)
		m.DeleteTarget = forge.CommentRef{ID: "c1", Pending: true}
		updated, _ := m.Update(deleteResultMsg{Err: tc.err})
		if g := updated.(Model); g.DeleteTarget != (forge.CommentRef{}) {
			t.Fatalf("%s: delete target must be cleared, got %+v", tc.name, g.DeleteTarget)
		}
	}
}

func TestThreadFocusEditsCursorSelectedComment(t *testing.T) {
	m := threadFocusModel(t, []domain.Comment{
		{ID: "c1", ViewerDidAuthor: false, Body: "theirs"},
		{ID: "c2", ViewerDidAuthor: true, Body: "mine", State: "PENDING"},
	})
	m.ThreadCursor = 1 // select the second comment, not the first
	updated, _ := UpdateThreadFocus(m, tea.KeyPressMsg{Text: "e", Code: 'e'})
	if updated.Compose.CommentID != "c2" {
		t.Fatalf("e should edit the ThreadCursor-selected comment, got %q", updated.Compose.CommentID)
	}
	if updated.Composer.Value() != "mine" {
		t.Fatalf("should prefill the selected comment's body, got %q", updated.Composer.Value())
	}
}
