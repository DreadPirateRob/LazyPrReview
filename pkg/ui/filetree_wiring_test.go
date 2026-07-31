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

// treeModel creates a Model with files under two directories so that
// buildTreeRows produces dir rows interleaved with file rows.
// DiffFiles are wired to match PRDetail.Files so enter-on-file-row works.
func treeModel(t *testing.T) Model {
	t.Helper()
	f := fakeforge.New()
	m := New(config.Default(), f)
	m.Loading = false
	files, err := diff.Parse(
		"diff --git a/pkg/foo.go b/pkg/foo.go\n" +
			"--- a/pkg/foo.go\n+++ b/pkg/foo.go\n" +
			"@@ -1 +1 @@\n-old\n+new\n" +
			"diff --git a/pkg/bar.go b/pkg/bar.go\n" +
			"--- a/pkg/bar.go\n+++ b/pkg/bar.go\n" +
			"@@ -1,2 +1,3 @@\n-old1\n-old2\n+new1\n+new2\n+new3\n" +
			"diff --git a/docs/README.md b/docs/README.md\n" +
			"--- a/docs/README.md\n+++ b/docs/README.md\n" +
			"@@ -1 +1 @@\n-# old\n+# new\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	m.PRDetail = &domain.PRDetail{
		ID:    "PR1",
		Title: "title",
		Files: []domain.ChangedFile{
			{Path: "pkg/foo.go", Additions: 1, Deletions: 0, ViewerViewedState: "UNVIEWED"},
			{Path: "pkg/bar.go", Additions: 2, Deletions: 1, ViewerViewedState: "UNVIEWED"},
			{Path: "docs/README.md", Additions: 5, Deletions: 0, ViewerViewedState: "UNVIEWED"},
		},
	}
	m.DiffFiles = files
	m.AnchoredThreads = nil
	m.UnresolvedThreadIndex = nil
	m.FilesCollapsed = nil
	m.FilesFlat = false
	m.FocusStack = []FocusContext{FocusFiles}
	return m
}

// firstDirPos returns the row index of the first directory row, or -1 if none.
func firstDirPos(m Model) int {
	for i, r := range visibleFileRows(m) {
		if r.IsDir {
			return i
		}
	}
	return -1
}

// space on a directory row acts on its VISIBLE subtree: with a Files filter
// active, files hidden from the panel are not toggled. That matches the ✓ and
// +N/-N the dir row itself shows, which are computed from the filtered set — and
// it matches the subtree the dir diff renders into Main.
func TestSpaceDirRowRespectsFilter(t *testing.T) {
	m := treeModel(t)
	m.FilesPanel.Filter = "foo" // hides pkg/bar.go from the panel
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("expected a dir row to survive the filter")
	}
	m.FilesPanel.Cursor = dirPos

	got, cmd := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space on a dir row should dispatch a viewed toggle")
	}
	state := map[string]string{}
	for _, f := range got.PRDetail.Files {
		state[f.Path] = f.ViewerViewedState
	}
	if state["pkg/foo.go"] != "VIEWED" {
		t.Fatalf("visible file under the dir should be toggled, got %q", state["pkg/foo.go"])
	}
	if state["pkg/bar.go"] == "VIEWED" {
		t.Fatal("a file hidden by the filter must not be toggled")
	}
}

// TestJKTraverseDirRows asserts that j/k move through dir rows as well as file rows.
func TestJKTraverseDirRows(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must produce at least one dir row")
	}
	m.FilesPanel.Cursor = dirPos

	if dirPos < len(rows)-1 {
		got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
		if got.FilesPanel.Cursor != dirPos+1 {
			t.Fatalf("j from dir row %d: want cursor %d, got %d", dirPos, dirPos+1, got.FilesPanel.Cursor)
		}
		back, _ := UpdateFiles(got, tea.KeyPressMsg{Text: "k", Code: 'k'})
		if back.FilesPanel.Cursor != dirPos {
			t.Fatalf("k back to dir row: want %d, got %d", dirPos, back.FilesPanel.Cursor)
		}
	}
}

// TestEnterDirRowTogglesCollapse asserts that enter on a dir row collapses it,
// does not change MainMode/MainLines, and does not focus Main.
func TestEnterDirRowTogglesCollapse(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must produce at least one dir row")
	}
	dirPath := rows[dirPos].Path
	m.FilesPanel.Cursor = dirPos
	savedMode := m.MainMode
	savedLines := len(m.MainLines)

	got, _ := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if !got.FilesCollapsed[dirPath] {
		t.Fatalf("enter on dir %q must collapse it; FilesCollapsed=%v", dirPath, got.FilesCollapsed)
	}
	if got.MainMode != savedMode {
		t.Fatalf("enter on dir must not change MainMode: got %q", got.MainMode)
	}
	if len(got.MainLines) != savedLines {
		t.Fatalf("enter on dir must not change MainLines: before=%d after=%d", savedLines, len(got.MainLines))
	}
	if got.CurrentFocus() == FocusMain {
		t.Fatal("enter on dir must not focus Main")
	}

	// Second enter on the (now-visible, collapsed) dir row should expand it.
	got2, _ := UpdateFiles(got, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got2.FilesCollapsed[dirPath] {
		t.Fatalf("second enter on dir %q must expand it; still collapsed", dirPath)
	}
}

// TestEnterFileRowInTreeOpensMain asserts that enter on a file row within a
// tree still opens it in Main (the dir-vs-file dispatch must not break file rows).
func TestEnterFileRowInTreeOpensMain(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)

	// Find the first file row.
	filePos := -1
	for i, r := range rows {
		if !r.IsDir {
			filePos = i
			break
		}
	}
	if filePos < 0 {
		t.Fatal("treeModel must produce at least one file row")
	}
	m.FilesPanel.Cursor = filePos

	got, _ := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if got.CurrentFocus() != FocusMain {
		t.Fatalf("enter on file row must focus Main, got %s", got.CurrentFocus())
	}
	if got.MainMode != MainDiff {
		t.Fatalf("enter on file row must show diff, got mode %q", got.MainMode)
	}
}

// TestCollapseAllAndExpandAll asserts that - collapses every directory (shrinking
// the row count) and = restores the full row count.
func TestCollapseAllAndExpandAll(t *testing.T) {
	m := treeModel(t)
	originalLen := len(visibleFileRows(m))
	if originalLen == 0 {
		t.Fatal("treeModel must have rows")
	}
	if firstDirPos(m) < 0 {
		t.Fatal("treeModel must have dir rows for collapse/expand to be meaningful")
	}

	// Collapse all.
	got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "-", Code: '-'})
	if len(got.FilesCollapsed) == 0 {
		t.Fatal("- must populate FilesCollapsed")
	}
	collapsedLen := len(visibleFileRows(got))
	if collapsedLen >= originalLen {
		t.Fatalf("- must shrink row count: before=%d after=%d", originalLen, collapsedLen)
	}

	// Expand all.
	got2, _ := UpdateFiles(got, tea.KeyPressMsg{Text: "=", Code: '='})
	if len(got2.FilesCollapsed) != 0 {
		t.Fatalf("= must clear FilesCollapsed, got %v", got2.FilesCollapsed)
	}
	expandedLen := len(visibleFileRows(got2))
	if expandedLen != originalLen {
		t.Fatalf("= must restore row count: want %d, got %d", originalLen, expandedLen)
	}
}

// TestBacktickPreservesFileAcrossFlip asserts that flipping tree<->flat keeps
// the cursor on the same file (same FileIndex).
func TestBacktickPreservesFileAcrossFlip(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)

	// Find the first file row in tree mode.
	filePos := -1
	wantIdx := -1
	for i, r := range rows {
		if !r.IsDir {
			filePos = i
			wantIdx = r.FileIndex
			break
		}
	}
	if filePos < 0 {
		t.Fatal("treeModel must have file rows")
	}
	m.FilesPanel.Cursor = filePos

	// Flip to flat.
	got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "`", Code: '`'})
	if !got.FilesFlat {
		t.Fatal("backtick must flip FilesFlat")
	}
	newRows := visibleFileRows(got)
	if got.FilesPanel.Cursor >= len(newRows) {
		t.Fatalf("cursor out of range after backtick: cursor=%d len=%d", got.FilesPanel.Cursor, len(newRows))
	}
	if newRows[got.FilesPanel.Cursor].FileIndex != wantIdx {
		t.Fatalf("backtick must preserve file at cursor: want FileIndex %d, got %d",
			wantIdx, newRows[got.FilesPanel.Cursor].FileIndex)
	}

	// Flip back to tree: file should still be preserved.
	got2, _ := UpdateFiles(got, tea.KeyPressMsg{Text: "`", Code: '`'})
	if got2.FilesFlat {
		t.Fatal("second backtick must restore tree mode")
	}
	treeRows := visibleFileRows(got2)
	if got2.FilesPanel.Cursor >= len(treeRows) {
		t.Fatalf("cursor out of range after second backtick: cursor=%d len=%d", got2.FilesPanel.Cursor, len(treeRows))
	}
	if treeRows[got2.FilesPanel.Cursor].FileIndex != wantIdx {
		t.Fatalf("second backtick must preserve file: want FileIndex %d, got %d",
			wantIdx, treeRows[got2.FilesPanel.Cursor].FileIndex)
	}
}

// TestSpaceDirRowTogglesSubtreeViewed asserts that space on a dir row toggles
// viewed for every file under that directory and returns a command, while files
// outside the subtree are not touched.
func TestSpaceDirRowTogglesSubtreeViewed(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must produce at least one dir row")
	}
	m.FilesPanel.Cursor = dirPos
	dirPath := rows[dirPos].Path
	prefix := dirPath + "/"

	// Partition files into under-dir and outside-dir.
	var underDir, outsideDir []int
	for i, f := range m.PRDetail.Files {
		if strings.HasPrefix(f.Path, prefix) {
			underDir = append(underDir, i)
		} else {
			outsideDir = append(outsideDir, i)
		}
	}
	if len(underDir) == 0 {
		t.Fatalf("dir %q must have files beneath it", dirPath)
	}

	got, cmd := UpdateFiles(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space on dir row must return a command")
	}
	for _, idx := range underDir {
		if got.PRDetail.Files[idx].ViewerViewedState != "VIEWED" {
			t.Fatalf("file index %d (%s) under %s must be optimistically VIEWED",
				idx, m.PRDetail.Files[idx].Path, dirPath)
		}
	}
	for _, idx := range outsideDir {
		if got.PRDetail.Files[idx].ViewerViewedState == "VIEWED" {
			t.Fatalf("file index %d (%s) outside %s must not be touched",
				idx, m.PRDetail.Files[idx].Path, dirPath)
		}
	}
}

// TestCursorClampedAfterCollapse asserts that the cursor stays in range after
// collapsing directories removes rows that were previously under the cursor.
func TestCursorClampedAfterCollapse(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)
	if len(rows) == 0 {
		t.Fatal("treeModel must have rows")
	}
	// Place cursor at the last row — it will disappear after collapseAll.
	m.FilesPanel.Cursor = len(rows) - 1

	got, _ := UpdateFiles(m, tea.KeyPressMsg{Text: "-", Code: '-'})
	newRows := visibleFileRows(got)
	if len(newRows) == 0 {
		t.Skip("no rows after collapse")
	}
	if got.FilesPanel.Cursor >= len(newRows) {
		t.Fatalf("cursor out of range after collapseAll: cursor=%d len=%d",
			got.FilesPanel.Cursor, len(newRows))
	}

	// Cursor must also be in range after a mode flip.
	m2 := treeModel(t)
	m2.FilesPanel.Cursor = len(rows) - 1
	got2, _ := UpdateFiles(m2, tea.KeyPressMsg{Text: "`", Code: '`'})
	flatRows := visibleFileRows(got2)
	if len(flatRows) == 0 {
		t.Skip("no rows after flip")
	}
	if got2.FilesPanel.Cursor >= len(flatRows) {
		t.Fatalf("cursor out of range after backtick: cursor=%d len=%d",
			got2.FilesPanel.Cursor, len(flatRows))
	}
}

// TestFollowFilesSelectionShowsDirDiff asserts that landing on a dir row fills
// Main with the aggregate diff of that subtree rather than leaving the previously
// opened file on screen.
func TestFollowFilesSelectionShowsDirDiff(t *testing.T) {
	m := treeModel(t)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must have dir rows")
	}
	m.FilesPanel.Cursor = dirPos
	m.MainLines = []string{"stale previous file"}
	m.MainMode = MainOverview

	got := followFilesSelection(m)

	if got.MainMode != MainDiff {
		t.Fatalf("dir row should put Main in diff mode, got %q", got.MainMode)
	}
	if len(got.MainLines) < 2 || !strings.Contains(got.MainLines[0], "Directory:") {
		t.Fatalf("expected a Directory: header, got %v", got.MainLines)
	}
	dir := visibleFileRows(m)[dirPos].Path
	if got.MainDirPath != dir {
		t.Fatalf("MainDirPath = %q, want %q", got.MainDirPath, dir)
	}
	// A concatenated view has no 1:1 line mapping, so it must not masquerade as a
	// single PR file — that sentinel is what disables line comments and anchors.
	if got.MainFileIndex >= 0 {
		t.Fatalf("dir view must set MainFileIndex = -1, got %d", got.MainFileIndex)
	}
	// Every file beneath the dir must appear.
	body := strings.Join(got.MainLines, "\n")
	for _, idx := range filesUnderDir(m, dir) {
		if p := m.PRDetail.Files[idx].Path; !strings.Contains(body, p) {
			t.Fatalf("dir diff is missing %q", p)
		}
	}
}

// Re-showing the same directory must not jolt the reader back to the top.
func TestDirFollowIsIdempotent(t *testing.T) {
	m := treeModel(t)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must have dir rows")
	}
	m.FilesPanel.Cursor = dirPos
	m = followFilesSelection(m)
	m.MainCursor, m.MainScroll = 7, 4

	got := followFilesSelection(m)
	if got.MainCursor != 7 || got.MainScroll != 4 {
		t.Fatalf("re-showing the same dir must keep cursor/scroll, got %d/%d", got.MainCursor, got.MainScroll)
	}
}

// Moving from a dir row onto a file row must hand Main back to the single-file
// view, restoring line anchors.
func TestDirThenFileRestoresFileView(t *testing.T) {
	m := treeModel(t)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must have dir rows")
	}
	m.FilesPanel.Cursor = dirPos
	m = followFilesSelection(m)

	rows := visibleFileRows(m)
	filePos := -1
	for i, r := range rows {
		if !r.IsDir {
			filePos = i
			break
		}
	}
	if filePos < 0 {
		t.Fatal("treeModel must have file rows")
	}
	m.FilesPanel.Cursor = filePos
	got := followFilesSelection(m)

	if got.MainDirPath != "" {
		t.Fatalf("moving to a file must clear MainDirPath, got %q", got.MainDirPath)
	}
	if got.MainFileIndex < 0 {
		t.Fatal("moving to a file must restore a real MainFileIndex (line anchors)")
	}
}

// The dir no-op keys on the Files filter as well as the path: the same directory
// under a narrower filter is a different subtree and must re-render rather than
// leave the wider body (and a stale cursor) on screen.
func TestDirFollowRerendersWhenFilterChanges(t *testing.T) {
	m := treeModel(t)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must have dir rows")
	}
	m.FilesPanel.Cursor = dirPos
	m = followFilesSelection(m)
	dir := m.MainDirPath
	wide := strings.Join(m.MainLines, "\n")
	m.MainCursor, m.MainScroll = 9, 5

	// Narrow to a single file that still lives under the same directory.
	under := filesUnderDir(m, dir)
	if len(under) < 2 {
		t.Skip("need a dir with at least two visible files")
	}
	m.FilesPanel.Filter = m.PRDetail.Files[under[0]].Path

	rows := visibleFileRows(m)
	pos := -1
	for i, r := range rows {
		if r.IsDir && r.Path == dir {
			pos = i
			break
		}
	}
	if pos < 0 {
		t.Skip("filter removed the dir row itself")
	}
	m.FilesPanel.Cursor = pos
	got := followFilesSelection(m)

	if strings.Join(got.MainLines, "\n") == wide {
		t.Fatal("narrowing the filter must re-render the dir diff, not reuse the wider body")
	}
	if got.MainCursor != 0 || got.MainScroll != 0 {
		t.Fatalf("a re-rendered dir view must reset the cursor, got %d/%d", got.MainCursor, got.MainScroll)
	}
}

// TestSelectedFileIndicesSkipsDirRows asserts that a range spanning dir rows
// returns only non-negative FileIndex values (no dir rows in the result).
func TestSelectedFileIndicesSkipsDirRows(t *testing.T) {
	m := treeModel(t)
	rows := visibleFileRows(m)
	dirPos := firstDirPos(m)
	if dirPos < 0 {
		t.Fatal("treeModel must have dir rows")
	}
	// Find the first file row after the dir.
	fileAfter := -1
	for i := dirPos + 1; i < len(rows); i++ {
		if !rows[i].IsDir {
			fileAfter = i
			break
		}
	}
	if fileAfter < 0 {
		t.Skip("need a file row after the first dir row")
	}

	m.FilesRangeActive = true
	m.FilesRangeStart = dirPos
	m.FilesPanel.Cursor = fileAfter

	indices := selectedFileIndices(m)

	for _, idx := range indices {
		if idx < 0 {
			t.Fatalf("selectedFileIndices must not include dir rows (FileIndex < 0): %v", indices)
		}
	}
	found := false
	for _, idx := range indices {
		if idx == rows[fileAfter].FileIndex {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("selectedFileIndices must include file at cursor (FileIndex %d): got %v",
			rows[fileAfter].FileIndex, indices)
	}
}

// TestCollapseToggleCopyOnWrite asserts that toggling collapse on one Model copy
// does not mutate another's FilesCollapsed map (copy-on-write contract).
func TestCollapseToggleCopyOnWrite(t *testing.T) {
	m1 := treeModel(t)
	m2 := m1 // value copy — shares no heap state

	dirPos := firstDirPos(m1)
	if dirPos < 0 {
		t.Fatal("treeModel must have dir rows")
	}
	m1.FilesPanel.Cursor = dirPos

	// Collapse the dir on m1.
	m1, _ = UpdateFiles(m1, tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(m1.FilesCollapsed) == 0 {
		t.Fatal("m1 must have collapsed state after enter on dir")
	}
	// m2 must still have an empty/nil map — the maps must not alias.
	if len(m2.FilesCollapsed) != 0 {
		t.Fatalf("collapsing on m1 must not affect m2 (copy-on-write violated): m2.FilesCollapsed=%v", m2.FilesCollapsed)
	}
}
