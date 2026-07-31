package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

// shiftKey builds the KeyPressMsg an enhanced-keyboard terminal actually sends
// for a shifted letter: the base (lowercase) rune plus ModShift, with Text
// carrying the uppercase glyph. This is what tmux/xterm delivered in a live
// capture — msg.Keystroke() returns "shift+s", not "S".
func shiftKey(lower rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: lower, Mod: tea.ModShift, Text: string(lower - 32)}
}

func TestSubmitKeyOpensMenu(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.FocusStack = []FocusContext{FocusMain}
	updated, _ := m.Update(shiftKey('s'))
	g := updated.(Model)
	if g.CurrentFocus() != FocusMenu {
		t.Fatal("shift+s should open the submit menu")
	}
	if len(g.MenuItems) != 4 {
		t.Fatalf("expected 4 submit choices (a/r/c/d), got %d", len(g.MenuItems))
	}
}

// Refresh (shift+r) is universal; the handler returns a cmd that yields a
// refreshMsg. Proves the fix is not special-cased to submit alone.
func TestShiftRRefreshes(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.FocusStack = []FocusContext{FocusMain}
	_, cmd := m.Update(shiftKey('r'))
	if cmd == nil {
		t.Fatal("shift+r should dispatch a refresh command")
	}
	if _, ok := cmd().(refreshMsg); !ok {
		t.Fatalf("shift+r should produce refreshMsg, got %T", cmd())
	}
}

// A panel-local shifted binding: shift+T in Main is prev-unresolved-thread.
// With no unresolved threads it toasts — an observable that only fires if the
// case matched, proving UpdateMain also honors the shifted form.
func TestShiftTInMainDispatches(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.UnresolvedThreadIndex = nil
	m.FocusStack = []FocusContext{FocusMain}
	updated, _ := m.Update(shiftKey('t'))
	g := updated.(Model)
	if g.Toast.Message != ToastNoUnresolvedThreads {
		t.Fatalf("shift+T should reach the prev-thread case (toast), got %q", g.Toast.Message)
	}
}

// Filter input flows through handleFilterInput, which only appends single-rune
// keystrokes. Before the fix a shifted letter arrived as "shift+a" (len 7) and
// was silently dropped — you could not type a capital into any filter. The
// dispatchKey fold at handleKey restores it.
func TestFilterAcceptsCapitalLetters(t *testing.T) {
	m := seededModel(t)
	m.Loading = false
	m.FocusStack = []FocusContext{FocusPRs}

	opened, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = opened.(Model)
	if !m.FilterActive {
		t.Fatal("/ should open the filter")
	}

	typed, _ := m.Update(shiftKey('a'))
	m = typed.(Model)
	if m.PRPanel.Filter != "A" {
		t.Fatalf("filter should accept a capital letter, got %q", m.PRPanel.Filter)
	}
}

func TestSubmitApproveBodyOptional(t *testing.T) {
	m := seededModel(t)
	m = openSubmitMenu(m)
	g, _ := applyMenuItem(m, MenuItem{Kind: menuKindReview, Value: "APPROVE"})
	if g.CurrentFocus() != FocusCompose {
		t.Fatal("approve should open the body composer")
	}
	if g.Compose.Required {
		t.Fatal("approve body must be optional")
	}
	if g.Compose.Event != "APPROVE" {
		t.Fatalf("expected APPROVE event, got %q", g.Compose.Event)
	}
}

func TestSubmitRequestChangesBodyRequired(t *testing.T) {
	m := seededModel(t)
	m = openSubmitMenu(m)
	g, _ := applyMenuItem(m, MenuItem{Kind: menuKindReview, Value: "REQUEST_CHANGES"})
	if !g.Compose.Required {
		t.Fatal("request-changes body must be required")
	}
	g.Composer.SetValue("")
	after, cmd := submitCompose(g)
	if cmd != nil {
		t.Fatal("empty required body must not submit")
	}
	if after.CurrentFocus() != FocusCompose {
		t.Fatal("composer should stay open on empty required body")
	}
}

func TestSubmitReviewCmdEnsuresThenSubmits(t *testing.T) {
	f := fakeforge.New()
	msg := submitReviewCmd(f, "PR1", "OID", "", forge.ReviewEvent("COMMENT"), "looks off")().(reviewDoneMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if len(f.EnsurePendingCalls) != 1 {
		t.Fatalf("should ensure a review when none given, got %d", len(f.EnsurePendingCalls))
	}
	if len(f.SubmitReviewCalls) != 1 || f.SubmitReviewCalls[0].Event != "COMMENT" {
		t.Fatalf("should submit with the event, got %+v", f.SubmitReviewCalls)
	}
	if msg.ReviewID == "" {
		t.Fatal("result should carry the resolved review id")
	}
}

func TestSubmitReviewCmdReusesReviewID(t *testing.T) {
	f := fakeforge.New()
	msg := submitReviewCmd(f, "PR1", "OID", "R9", forge.ReviewEvent("APPROVE"), "")().(reviewDoneMsg)
	if len(f.EnsurePendingCalls) != 0 {
		t.Fatal("should not ensure when a review id is supplied")
	}
	if f.SubmitReviewCalls[0].ReviewID != "R9" {
		t.Fatalf("should submit the given review, got %q", f.SubmitReviewCalls[0].ReviewID)
	}
	if msg.ReviewID != "R9" {
		t.Fatalf("should carry the review id, got %q", msg.ReviewID)
	}
}

func TestReviewDoneSuccessClearsStateAndReturnsToList(t *testing.T) {
	m := seededModel(t)
	m.PendingReviewID = "R1"
	m.OptimisticDrafts = map[string]optimisticDraft{"o1": {}}
	m.FocusStack = []FocusContext{FocusMain}
	updated, cmd := m.Update(reviewDoneMsg{})
	g := updated.(Model)
	if g.PendingReviewID != "" || len(g.OptimisticDrafts) != 0 {
		t.Fatal("a closed review should clear cached review id + optimistic drafts")
	}
	if g.CurrentFocus() != FocusPRs {
		t.Fatal("should return to the PR list after submit")
	}
	if cmd == nil {
		t.Fatal("should refetch detail + list")
	}
}

func TestReviewDoneErrorCachesReviewID(t *testing.T) {
	m := seededModel(t)
	updated, _ := m.Update(reviewDoneMsg{Err: errString("boom"), ReviewID: "R7"})
	g := updated.(Model)
	if g.PendingReviewID != "R7" {
		t.Fatalf("a failed submit should cache the review id for retry, got %q", g.PendingReviewID)
	}
	if g.Toast.Message == "" {
		t.Fatal("expected a failure toast")
	}
}

func TestDiscardOpensConfirmThenDiscards(t *testing.T) {
	m := seededModel(t)
	f := m.Forge.(*fakeforge.Fake)
	m = openSubmitMenu(m)
	confirm, _ := applyMenuItem(m, MenuItem{Kind: menuKindDiscardReview})
	if confirm.CurrentFocus() != FocusMenu || len(confirm.MenuItems) != 2 {
		t.Fatalf("discard should open a yes/no confirm menu, got focus=%v items=%d", confirm.CurrentFocus(), len(confirm.MenuItems))
	}
	confirm.PendingReviewID = "R1"
	_, cmd := applyMenuItem(confirm, MenuItem{Kind: menuKindDiscardConfirm})
	if cmd == nil {
		t.Fatal("confirming discard should dispatch a discard command")
	}
	msg := cmd().(reviewDoneMsg)
	if !msg.Discarded || msg.Err != nil {
		t.Fatalf("expected a clean discard result, got %+v", msg)
	}
	if len(f.DiscardCalls) != 1 || f.DiscardCalls[0] != "R1" {
		t.Fatalf("should discard the resolved review, got %+v", f.DiscardCalls)
	}
}

func TestPRListRefreshUsesSearchOnSearchTab(t *testing.T) {
	m := seededModel(t)
	m.PRFilter = forge.FilterSearch
	m.PRPanel.Filter = "is:open"
	msg := prListRefreshCmd(m)().(prsLoadedMsg)
	if msg.Filter != forge.FilterSearch || msg.Query != "is:open" {
		t.Fatalf("search tab must refresh via search, got filter=%v query=%q", msg.Filter, msg.Query)
	}
}
