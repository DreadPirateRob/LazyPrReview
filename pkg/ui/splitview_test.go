package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// splitModel returns a diff model wide enough to host the side-by-side view.
//
// The unified render is rebuilt AFTER the terminal size is set: diffModel builds it
// at zero width, and every diff render bakes in the width it was built at, so a
// baseline captured before the resize would never match what a restore produces.
func splitModel(t *testing.T) Model {
	t.Helper()
	m, _, _ := diffModel(t)
	m.PRDetail = &domain.PRDetail{Title: "t"}
	m.FocusStack = []FocusContext{FocusMain}
	m.Width, m.Height = 200, 44
	m = setMainDiff(m, m.MainFileIndex)
	if w := mainContentWidth(m); !splitFits(w) {
		t.Fatalf("fixture must be wide enough for split, content width %d", w)
	}
	return m
}

// addCodeRow returns the Main line index of an added code row. buildDiffRows tags
// every rendered line rowCode — headers included — and a header has no line numbers
// to anchor, so the first rowCode row is not a usable anchor target.
func addCodeRow(t *testing.T, m Model) int {
	t.Helper()
	rendered := m.DiffFiles[m.MainFileIndex].Rendered
	for i, r := range m.MainRows {
		if r.Kind != rowCode || r.RenderIndex < 0 || r.RenderIndex >= len(rendered) {
			continue
		}
		if rendered[r.RenderIndex].Kind == diff.LineKindAdd {
			return i
		}
	}
	t.Fatal("fixture needs an added code row")
	return -1
}

func TestSplitToggleRoundTripsToUnified(t *testing.T) {
	m := splitModel(t)
	unified := strings.Join(m.MainLines, "\n")
	file := m.DiffFiles[m.MainFileIndex].Path

	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if split.DiffFiles[split.MainDiff.FileIndex].Path != file {
		t.Fatalf("split view should record its file, got %q want %q", split.DiffFiles[split.MainDiff.FileIndex].Path, file)
	}
	if split.MainFileIndex != -1 {
		t.Fatalf("split view is read-only and must drop the file anchor, got %d", split.MainFileIndex)
	}
	if strings.Join(split.MainLines, "\n") == unified {
		t.Fatal("split view must render differently from unified")
	}

	back, _ := UpdateMain(split, tea.KeyPressMsg{Text: "|", Code: '|'})
	if back.MainDiff.Split {
		t.Fatalf("leaving split must clear the recorded file, got %q", back.DiffFiles[back.MainDiff.FileIndex].Path)
	}
	if back.MainFileIndex != m.MainFileIndex {
		t.Fatalf("leaving split must restore the file anchor, got %d want %d", back.MainFileIndex, m.MainFileIndex)
	}
	if strings.Join(back.MainLines, "\n") != unified {
		t.Fatal("leaving split must restore the unified render of the same file")
	}
}

// Same contract the commit-scoped and directory-aggregate views rely on: with no
// file anchor, line-scoped actions are inert. This is what makes Scope A safe
// without touching the 40-odd RenderIndex consumers.
func TestSplitViewDisablesLineScopedActions(t *testing.T) {
	m := splitModel(t)
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	split.MainCursor = 3

	if afterC, _ := UpdateMain(split, tea.KeyPressMsg{Text: "c", Code: 'c'}); afterC.CurrentFocus() == FocusCompose {
		t.Fatal("c must not open a composer in the read-only split view")
	}
	if afterV, _ := UpdateMain(split, tea.KeyPressMsg{Text: "v", Code: 'v'}); mainRangeActive(afterV) {
		t.Fatal("v must not arm a range in the read-only split view")
	}
	if _, _, _, ok := anchorAt(split, split.MainCursor); ok {
		t.Fatal("split rows must not resolve to a line anchor")
	}
}

// Restoring unified must bring the line actions back, or the toggle is a trapdoor.
func TestLeavingSplitRestoresLineScopedActions(t *testing.T) {
	m := splitModel(t)
	addRow := addCodeRow(t, m)

	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	back, _ := UpdateMain(split, tea.KeyPressMsg{Text: "|", Code: '|'})
	back.MainCursor = addRow

	if _, _, _, ok := anchorAt(back, back.MainCursor); !ok {
		t.Fatal("unified rows must resolve to a line anchor again after leaving split")
	}
}

func TestSplitRefusedWhenTooNarrow(t *testing.T) {
	m := splitModel(t)
	m.Width = 60 // Main content drops below splitMinWidth
	if splitFits(mainContentWidth(m)) {
		t.Fatal("fixture should be too narrow")
	}

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if got.MainDiff.Split {
		t.Fatal("split must be refused when the pane cannot host two columns")
	}
	if got.Toast.Message != ToastSplitTooNarrow {
		t.Fatalf("the refusal must say why, got %q", got.Toast.Message)
	}
}

func TestSplitRefusedWithExternalPager(t *testing.T) {
	m := splitModel(t)
	m.Config.GUI.DiffPager = "delta --paging=never"

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if got.MainDiff.Split {
		t.Fatal("an external pager owns its own layout; split must be refused")
	}
	if got.Toast.Message != ToastSplitUnavailablePager {
		t.Fatalf("the refusal must say why, got %q", got.Toast.Message)
	}
}

// Every row is exactly the pane width, so the divider forms a straight column and
// the right-hand side starts at the same offset on every line.
func TestSplitRowsShareOneDividerColumn(t *testing.T) {
	m := splitModel(t)
	width := mainContentWidth(m)
	lines := buildSplitLines(m, m.MainFileIndex, width)
	if len(lines) < 3 {
		t.Fatalf("expected rendered split lines, got %d", len(lines))
	}

	sideW := (width - 1) / 2
	for i, line := range lines[1:] { // [0] is the file header
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "@@") || !strings.Contains(plain, "│") {
			continue // full-width header row
		}
		if w := ansi.StringWidth(line); w != sideW*2+1 {
			t.Errorf("row %d spans %d columns, want %d", i+1, w, sideW*2+1)
		}
		if col := strings.Index(plain, "│"); col != sideW {
			t.Errorf("row %d divider at column %d, want %d", i+1, col, sideW)
		}
	}
}

// The reported bug: switch to side-by-side, focus the Files panel, and the panel's
// cursor-follow rebuilt Main as unified — the mode was a property of the content
// rather than a preference.
func TestSplitSurvivesFilesPanelFollow(t *testing.T) {
	m := splitModel(t)
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if !split.MainDiff.Split {
		t.Fatal("precondition: should be in side-by-side")
	}
	splitLines := strings.Join(split.MainLines, "\n")

	// Focus Files, which follows its selection into Main.
	followed := followFilesSelection(split.PushFocus(FocusFiles))
	if !followed.DiffSplit {
		t.Fatal("the side-by-side preference must survive a focus change")
	}
	if !followed.MainDiff.Split {
		t.Fatal("following the Files cursor must keep rendering side-by-side")
	}
	if strings.Join(followed.MainLines, "\n") != splitLines {
		t.Fatal("the same file should still be rendered side-by-side after the focus bounce")
	}
}

// twoFileSplitModel extends splitModel with a second diff file. diffModel's fixture
// is a single file, and carrying the mode across files needs two.
func twoFileSplitModel(t *testing.T) Model {
	t.Helper()
	m := splitModel(t)
	files, err := diff.Parse(anchorSampleDiff + "\ndiff --git a/second.go b/second.go\n--- a/second.go\n+++ b/second.go\n@@ -1 +1 @@\n-was\n+now\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("fixture must parse two files, got %d", len(files))
	}
	m.DiffFiles = files
	m.MainFileIndex = 0
	return setMainDiff(m, 0)
}

// Same for moving between files: the mode belongs to the reader, not the file.
func TestSplitSurvivesFileSwitch(t *testing.T) {
	m := twoFileSplitModel(t)
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	first := split.DiffFiles[split.MainDiff.FileIndex].Path
	if first == "" {
		t.Fatal("precondition: should be in side-by-side")
	}

	next := setMainFile(split, 1)
	if !next.MainDiff.Split {
		t.Fatal("switching files must keep side-by-side")
	}
	if next.DiffFiles[next.MainDiff.FileIndex].Path != m.DiffFiles[1].Path {
		t.Fatalf("should be showing %q, got %q", m.DiffFiles[1].Path, next.DiffFiles[next.MainDiff.FileIndex].Path)
	}
}

// The regression that slipped: `[`/`]` gated on MainFileIndex, which is -1 in split
// mode — `[` was dead and `]` jumped to file 0. This drives the actual KEYS through
// UpdateMain; TestSplitSurvivesFileSwitch calls the helper directly and would keep
// passing if the keypath regressed again.
func TestFileNavKeysWorkInSplit(t *testing.T) {
	m := twoFileSplitModel(t)
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	if !split.MainDiff.Split || split.MainDiff.FileIndex != 0 {
		t.Fatalf("precondition: split on file 0, got split=%v idx=%d", split.MainDiff.Split, split.MainDiff.FileIndex)
	}

	fwd, _ := UpdateMain(split, tea.KeyPressMsg{Text: "]", Code: ']'})
	if fwd.MainDiff.FileIndex != 1 {
		t.Fatalf("] should advance to file 1, got %d", fwd.MainDiff.FileIndex)
	}
	if !fwd.MainDiff.Split {
		t.Fatal("] must keep side-by-side")
	}

	back, _ := UpdateMain(fwd, tea.KeyPressMsg{Text: "[", Code: '['})
	if back.MainDiff.FileIndex != 0 {
		t.Fatalf("[ should return to file 0, got %d", back.MainDiff.FileIndex)
	}
	if !back.MainDiff.Split {
		t.Fatal("[ must keep side-by-side")
	}

	// At the first file, [ is a no-op — not a wrap, and never a jump to a bogus index.
	still, _ := UpdateMain(back, tea.KeyPressMsg{Text: "[", Code: '['})
	if still.MainDiff.FileIndex != 0 || !still.MainDiff.Split {
		t.Fatalf("[ at the first file must stay put, got idx=%d split=%v", still.MainDiff.FileIndex, still.MainDiff.Split)
	}
}

// In a directory or commit view "next file" has no meaning — and with the old
// MainFileIndex gate, `]` there jumped to file 0. It must be inert.
func TestFileNavKeysInertInMultiFileViews(t *testing.T) {
	m := twoFileSplitModel(t)
	m = renderCommitInMain(m, "abc123def", "test commit", m.DiffFiles)
	if m.MainDiff.Kind != mainDiffCommit {
		t.Fatalf("precondition: commit view, got kind %d", m.MainDiff.Kind)
	}
	beforeLines := strings.Join(m.MainLines, "\n")

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "]", Code: ']'})
	if got.MainDiff.Kind != mainDiffCommit {
		t.Fatalf("] in a commit view must stay a commit view, got kind %d", got.MainDiff.Kind)
	}
	if strings.Join(got.MainLines, "\n") != beforeLines {
		t.Fatal("] in a commit view must be inert, but the content changed")
	}
}

// Turning it off has to stick too, or the toggle is one-way.
func TestUnifiedSurvivesAfterToggleOff(t *testing.T) {
	m := splitModel(t)
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})
	back, _ := UpdateMain(split, tea.KeyPressMsg{Text: "|", Code: '|'})
	if back.DiffSplit {
		t.Fatal("toggling off must clear the preference")
	}

	followed := followFilesSelection(back.PushFocus(FocusFiles))
	if followed.MainDiff.Split {
		t.Fatal("with the preference off, following must render unified")
	}
}

// A thread has no side-by-side representation, so jumping to one renders unified —
// but it must not silently rewrite the preference.
func TestThreadJumpRendersUnifiedWithoutClearingPreference(t *testing.T) {
	m := splitModel(t)
	m.PRDetail.Threads = []domain.Thread{{
		ID: "tS", Path: m.DiffFiles[0].Path, DiffSide: "RIGHT", Line: intPtr(1),
		Comments: []domain.Comment{{Body: "hi"}},
	}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	if len(m.AnchoredThreads) == 0 {
		t.Skip("fixture thread did not anchor")
	}
	split, _ := UpdateMain(m, tea.KeyPressMsg{Text: "|", Code: '|'})

	jumped := showThreadInMain(split, m.AnchoredThreads[0])
	if jumped.MainDiff.Split {
		t.Fatal("a thread jump must render unified — threads are not drawn side-by-side")
	}
	if !jumped.DiffSplit {
		t.Fatal("a thread jump must not silently turn the preference off")
	}
	if jumped.MainFileIndex < 0 {
		t.Fatal("the unified render must restore the file anchor so the thread is addressable")
	}
}
