package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

func threadResolved(m Model, id string) bool {
	for _, th := range m.PRDetail.Threads {
		if th.ID == id {
			return th.IsResolved
		}
	}
	return false
}

func TestResolveThreadCmdCallsForge(t *testing.T) {
	f := fakeforge.New()
	msg, ok := resolveThreadCmd(f, "t9", true)().(resolveResultMsg)
	if !ok || msg.ThreadID != "t9" || !msg.Resolved || msg.Err != nil {
		t.Fatalf("bad resolve result: %+v", msg)
	}
	if len(f.ResolveCalls) != 1 || f.ResolveCalls[0].ThreadID != "t9" || !f.ResolveCalls[0].Resolved {
		t.Fatalf("forge not called correctly: %+v", f.ResolveCalls)
	}
}

func TestResolveOptimisticRollbackOnError(t *testing.T) {
	m := seededModel(t)
	m = setThreadResolved(m, "t1", true)
	if !threadResolved(m, "t1") {
		t.Fatal("optimistic set should mark t1 resolved")
	}
	updated, _ := m.Update(resolveResultMsg{ThreadID: "t1", Resolved: true, Err: errString("boom")})
	g := updated.(Model)
	if threadResolved(g, "t1") {
		t.Fatal("a failed resolve must roll back to unresolved")
	}
	if g.Toast.Message == "" {
		t.Fatal("expected a failure toast")
	}
}

func TestResolveSuccessKeepsState(t *testing.T) {
	m := seededModel(t)
	m = setThreadResolved(m, "t1", true)
	updated, _ := m.Update(resolveResultMsg{ThreadID: "t1", Resolved: true})
	if !threadResolved(updated.(Model), "t1") {
		t.Fatal("a successful resolve should keep t1 resolved")
	}
}

func TestThreadPanelSpaceResolves(t *testing.T) {
	m := seededModel(t)
	for i := range m.PRDetail.Threads {
		m.PRDetail.Threads[i].ViewerCanResolve = true
	}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.FocusStack = []FocusContext{FocusThreads}
	m.ThreadTab = ThreadsTabAll
	m.ThreadsPanel.Cursor = 0

	rows := filteredThreadRows(m)
	if len(rows) == 0 {
		t.Fatal("expected anchored threads in the All tab")
	}
	id := rows[0].Thread.ID
	before := rows[0].Thread.IsResolved

	updated, cmd := UpdateThreads(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space on a resolvable thread should dispatch a resolve cmd")
	}
	if threadResolved(updated, id) == before {
		t.Fatal("space should optimistically flip the thread's resolved state")
	}
}

func TestThreadPanelSpaceBlockedWhenCannotResolve(t *testing.T) {
	m := seededModel(t)
	for i := range m.PRDetail.Threads {
		m.PRDetail.Threads[i].ViewerCanResolve = false
	}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	m.FocusStack = []FocusContext{FocusThreads}
	m.ThreadTab = ThreadsTabAll
	updated, cmd := UpdateThreads(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd != nil {
		t.Fatal("space must not resolve a thread the viewer can't resolve")
	}
	if updated.Toast.Message == "" {
		t.Fatal("expected a can't-resolve toast")
	}
}
