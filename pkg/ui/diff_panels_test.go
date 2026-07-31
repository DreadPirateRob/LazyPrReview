package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

const panelSampleDiff = `diff --git a/foo.txt b/foo.txt
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

func seededModel(t *testing.T) Model {
	t.Helper()
	forge := fakeforge.New()
	m := New(config.Default(), forge)
	m.Loading = false
	m.PRDetail = &domain.PRDetail{
		ID:      "PR1",
		Title:   "title",
		Body:    "body",
		URL:     "https://github.com/owner/repo/pull/1",
		Files:   []domain.ChangedFile{{Path: "foo.txt", Additions: 2, Deletions: 1, ViewerViewedState: "UNVIEWED"}, {Path: "bar.txt", Additions: 1, Deletions: 1, ViewerViewedState: "UNVIEWED"}},
		Threads: []domain.Thread{{ID: "t1", Path: "foo.txt", DiffSide: "RIGHT", Line: intPtr(3), Comments: []domain.Comment{{Body: "first thread"}}}, {ID: "t2", Path: "bar.txt", DiffSide: "RIGHT", Line: intPtr(1), Comments: []domain.Comment{{Body: "second thread"}}}},
	}
	files, err := diff.Parse(panelSampleDiff + "\ndiff --git a/bar.txt b/bar.txt\n--- a/bar.txt\n+++ b/bar.txt\n@@ -1 +1 @@\n-old\n+new\n")
	if err != nil {
		t.Fatal(err)
	}
	m.DiffFiles = files
	m.Commits = []domain.CommitSummary{
		{OID: "abc123456789", MessageHeadline: "first commit"},
		{OID: "def987654321", MessageHeadline: "second commit"},
	}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, files)
	m.ThreadTab = ThreadsTabUnresolved
	return m
}

func TestFilesOpenDiff(t *testing.T) {
	m := seededModel(t)
	m.FilesPanel.Cursor = 1
	updated, _ := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.MainFileIndex != 1 {
		t.Fatalf("got main file %d", updated.MainFileIndex)
	}
	if updated.CurrentFocus() != FocusMain {
		t.Fatalf("expected main focus, got %s", updated.CurrentFocus())
	}
}

func TestViewedToggleRollback(t *testing.T) {
	forge := fakeforge.New()
	forge.MarkViewedErrs["*"] = errors.New("boom")
	m := New(config.Default(), forge)
	m.Loading = false
	m.PRDetail = &domain.PRDetail{ID: "PR1", Files: []domain.ChangedFile{{Path: "foo.txt", ViewerViewedState: "UNVIEWED"}}}
	msgModel, cmd := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeySpace})
	mid := msgModel
	if mid.PRDetail.Files[0].ViewerViewedState != "VIEWED" {
		t.Fatalf("expected optimistic viewed state")
	}
	result := cmd().(viewedToggleResultMsg)
	final, _ := mid.Update(result)
	got := final.(Model)
	if got.PRDetail.Files[0].ViewerViewedState != "UNVIEWED" {
		t.Fatalf("expected rollback, got %s", got.PRDetail.Files[0].ViewerViewedState)
	}
	if got.Toast.Message == "" {
		t.Fatalf("expected failure toast")
	}
}

func TestViewedToggleSetsAndClearsActivity(t *testing.T) {
	forge := fakeforge.New()
	m := New(config.Default(), forge)
	m.Loading = false
	m.PRDetail = &domain.PRDetail{ID: "PR1", Files: []domain.ChangedFile{{Path: "foo.txt", ViewerViewedState: "UNVIEWED"}}}
	msgModel, cmd := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeySpace})
	mid := msgModel
	if mid.Activity != "Updating viewed state…" {
		t.Fatalf("expected viewed-toggle activity, got %q", mid.Activity)
	}
	result := cmd().(viewedToggleResultMsg)
	final, _ := mid.Update(result)
	if final.(Model).Activity != "" {
		t.Fatalf("expected activity to clear after viewed-toggle result, got %q", final.(Model).Activity)
	}
}

func TestFilesJumpFirstUnresolvedThread(t *testing.T) {
	m := seededModel(t)
	updated, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "t", Code: 't'})
	if updated.CurrentFocus() != FocusMain {
		t.Fatalf("expected main focus, got %s", updated.CurrentFocus())
	}
}

func TestMainThreadNavigationWrap(t *testing.T) {
	m := seededModel(t)
	m = jumpToAnchoredThread(m, m.UnresolvedThreadIndex[0], false)
	updated, _ := UpdateMain(m, tea.KeyPressMsg{Text: "t", Code: 't'})
	if updated.MainFileIndex != 1 {
		t.Fatalf("expected second file after t, got %d", updated.MainFileIndex)
	}
	updated, _ = UpdateMain(updated, tea.KeyPressMsg{Text: "t", Code: 't'})
	if updated.MainFileIndex != 0 {
		t.Fatalf("expected wrap to first file, got %d", updated.MainFileIndex)
	}
}

func TestThreadsCommitsTabShowsCommitRows(t *testing.T) {
	m := seededModel(t)
	m.ThreadTab = ThreadsTabCommits
	view := ThreadsView(m)
	if !strings.Contains(view, "abc1234 first commit") {
		t.Fatalf("expected first commit row, got %q", view)
	}
	updated, _ := UpdateThreads(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if updated.ThreadsPanel.Cursor != 1 {
		t.Fatalf("expected commit cursor to move, got %d", updated.ThreadsPanel.Cursor)
	}
}

func TestThreadsEnterFocusesThread(t *testing.T) {
	m := seededModel(t)
	updated, _ := UpdateThreads(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.CurrentFocus() != FocusThread {
		t.Fatalf("expected thread focus, got %s", updated.CurrentFocus())
	}
}

func TestCurrentFileURLUsesGitHubDiffAnchorAndLine(t *testing.T) {
	m := seededModel(t)
	m = openFileInMain(m, 0)
	if got := currentFileURL(m, false); got != "https://github.com/owner/repo/pull/1/files#diff-ddab29ff2c393ee52855d21a240eb05f775df88e3ce347df759f0c4b80356c35" {
		t.Fatalf("unexpected file url: %s", got)
	}
	for i, line := range m.DiffFiles[m.MainFileIndex].Rendered {
		if line.NewNo != nil && *line.NewNo == 3 {
			m.MainCursor = i + 1
			break
		}
	}
	if got := currentFileURL(m, true); got != "https://github.com/owner/repo/pull/1/files#diff-ddab29ff2c393ee52855d21a240eb05f775df88e3ce347df759f0c4b80356c35R3" {
		t.Fatalf("unexpected line url: %s", got)
	}
	m.FilesPanel.Cursor = 1
	if got := currentSelectedFileURL(m); got != "https://github.com/owner/repo/pull/1/files#diff-08bd2d247cc7aa38b8c4b7fd20ee7edad0b593c3debce92f595c9d016da40bae" {
		t.Fatalf("unexpected selected file url: %s", got)
	}
}

func intPtr(v int) *int { return &v }

func TestMainOverviewRestoreOnEscAndZero(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusMain}
	m = showPROverview(m)
	if m.MainMode != MainOverview {
		t.Fatalf("expected overview baseline, got %s", m.MainMode)
	}
	overview := strings.Join(m.MainLines, "\n")

	// Opening a file switches Main into diff mode.
	opened, _ := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if opened.MainMode != MainDiff {
		t.Fatalf("expected diff mode after opening file, got %s", opened.MainMode)
	}
	if opened.CurrentFocus() != FocusMain {
		t.Fatalf("expected main focus after opening file, got %s", opened.CurrentFocus())
	}

	// esc restores the overview and keeps focus on Main.
	escd, _ := opened.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := escd.(Model)
	if got.MainMode != MainOverview {
		t.Fatalf("expected overview after esc, got %s", got.MainMode)
	}
	if got.CurrentFocus() != FocusMain {
		t.Fatalf("expected main focus after esc, got %s", got.CurrentFocus())
	}
	if strings.Join(got.MainLines, "\n") != overview {
		t.Fatal("expected MainLines restored to overview after esc")
	}

	// From another panel, "0" focuses Main WITHOUT repainting it: a file diff the
	// user was reading stays put (esc is what pops back to the overview).
	diffAgain, _ := UpdateFiles(got, tea.KeyPressMsg{Code: tea.KeyEnter})
	if diffAgain.MainMode != MainDiff {
		t.Fatalf("expected diff mode before zero, got %s", diffAgain.MainMode)
	}
	wantLines := strings.Join(diffAgain.MainLines, "\n")
	fromFiles := diffAgain.PushFocus(FocusFiles)
	zeroed, _ := fromFiles.Update(tea.KeyPressMsg{Text: "0", Code: '0'})
	z := zeroed.(Model)
	if z.MainMode != MainDiff {
		t.Fatalf("zero must not override the current screen, got %s", z.MainMode)
	}
	if strings.Join(z.MainLines, "\n") != wantLines {
		t.Fatal("zero must leave the diff contents untouched")
	}
	if z.CurrentFocus() != FocusMain {
		t.Fatalf("expected main focus after zero, got %s", z.CurrentFocus())
	}
}

func TestCommitsTabEnterScopesDiff(t *testing.T) {
	m := seededModel(t)
	f := m.Forge.(*fakeforge.Fake)
	f.CommitDiffs["abc123456789"] = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-old\n+new\n"
	m.ThreadTab = ThreadsTabCommits
	m.ThreadsPanel.Cursor = 0

	updated, cmd := UpdateThreads(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a commit should fetch its diff")
	}
	lm, ok := cmd().(commitDiffLoadedMsg)
	if !ok {
		t.Fatalf("expected commitDiffLoadedMsg, got %T", cmd())
	}
	final, _ := updated.Update(lm)
	g := final.(Model)
	if g.MainMode != MainDiff {
		t.Fatalf("commit view should set MainMode=diff, got %s", g.MainMode)
	}
	joined := strings.Join(g.MainLines, "\n")
	if !strings.Contains(joined, "Commit abc1234") || !strings.Contains(joined, "first commit") {
		t.Fatalf("Main should show the commit header, got %q", joined)
	}
	if !strings.Contains(joined, "new") {
		t.Fatalf("Main should show the commit's diff content, got %q", joined)
	}
}
