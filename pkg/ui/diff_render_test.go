package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
)

// The built-in renderer's layout is a hard contract: lines[0] is the file
// header and lines[1+i] maps 1:1 to Rendered[i]; thread anchors depend on it.
func TestRenderDiffFileBuiltinStructure(t *testing.T) {
	m := seededModel(t)
	lines := renderDiffFile(m, 0)
	want := 1 + len(m.DiffFiles[0].Rendered)
	if len(lines) != want {
		t.Fatalf("expected %d lines (header + rendered), got %d", want, len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "foo.txt") {
		t.Fatalf("expected file path in header, got %q", got)
	}
	for i, rl := range m.DiffFiles[0].Rendered {
		if !strings.Contains(ansi.Strip(lines[1+i]), rl.Text) {
			t.Fatalf("line %d lost text %q: %q", 1+i, rl.Text, ansi.Strip(lines[1+i]))
		}
	}
}

func TestRenderDiffFileBuiltinColorsAndGutter(t *testing.T) {
	m := seededModel(t)
	file := m.DiffFiles[0]
	lines := renderDiffFile(m, 0)
	sawAdd := false
	sawGutter := false
	for i, rl := range file.Rendered {
		line := lines[1+i]
		if rl.Kind == diff.LineKindAdd && !sawAdd {
			sawAdd = true
			if !strings.Contains(line, "\x1b[") {
				t.Fatalf("expected SGR styling on add line, got %q", line)
			}
		}
		if rl.Kind == diff.LineKindContext && rl.OldNo != nil && rl.NewNo != nil && !sawGutter {
			sawGutter = true
			stripped := ansi.Strip(line)
			if !strings.Contains(stripped, strconv.Itoa(*rl.OldNo)) || !strings.Contains(stripped, strconv.Itoa(*rl.NewNo)) {
				t.Fatalf("expected gutter numbers %d/%d, got %q", *rl.OldNo, *rl.NewNo, stripped)
			}
		}
	}
	if !sawAdd || !sawGutter {
		t.Fatalf("fixture missing add/context lines: add=%v gutter=%v", sawAdd, sawGutter)
	}
}

func TestRenderDiffFilePagerMode(t *testing.T) {
	m := seededModel(t)
	m.Config.GUI.DiffPager = `sed 's/^/X/'`
	lines := renderDiffFile(m, 0)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "Xdiff --git") {
		t.Fatalf("expected sed-transformed raw diff, got %q", lines[0])
	}
}

func TestRenderDiffFilePagerFallbackOnFailure(t *testing.T) {
	m := seededModel(t)
	m.Config.GUI.DiffPager = "false"
	lines := renderDiffFile(m, 0)
	if len(lines) == 0 {
		t.Fatal("expected builtin fallback output")
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "File: foo.txt") {
		t.Fatalf("expected builtin fallback header, got %q", got)
	}
}

func TestPagerModeThreadJumpLandsAtTop(t *testing.T) {
	m := seededModel(t)
	m.Config.GUI.DiffPager = `sed 's/^/X/'`
	if len(m.UnresolvedThreadIndex) == 0 {
		t.Fatal("expected seeded unresolved threads")
	}
	got := jumpToAnchoredThread(m, m.UnresolvedThreadIndex[0], false)
	if got.MainCursor != 0 {
		t.Fatalf("expected cursor at top in pager mode, got %d", got.MainCursor)
	}
	if got.MainMode != MainDiff {
		t.Fatalf("expected diff mode, got %s", got.MainMode)
	}
}

func TestHighlightSelectionSpansRows(t *testing.T) {
	body := "> #7 title\n  ╰─ alice | x | ⎇b\nnext"
	out := highlightSelection(body, 0, 40, true, 2)
	lines := strings.Split(out, "\n")
	if strings.HasPrefix(lines[0], "> ") {
		t.Fatalf("selected row 0 should drop the '> ' marker: %q", lines[0])
	}
	if ansi.StringWidth(lines[0]) != 40 || ansi.StringWidth(lines[1]) != 40 {
		t.Fatalf("both selection rows should span full width, got %d/%d",
			ansi.StringWidth(lines[0]), ansi.StringWidth(lines[1]))
	}
	if ansi.StringWidth(lines[2]) == 40 {
		t.Fatalf("row outside the selection span must be untouched: %q", lines[2])
	}
}

func TestHighlightSelectionKeepsFgColor(t *testing.T) {
	green := diffAddStyle.Render("+5")
	if !strings.Contains(green, "\x1b[") {
		t.Skip("color profile emits no styling; nothing to preserve")
	}
	body := "> " + green + " file.go\nnext"
	out := highlightSelection(body, 0, 40, true, 1)
	line0 := strings.Split(out, "\n")[0]
	opener := green[:strings.IndexByte(green, '+')] // the fg SGR before the token
	if !strings.Contains(line0, opener) {
		t.Fatalf("selected row lost its fg color under the highlight: %q", line0)
	}
	if ansi.StringWidth(line0) != 40 {
		t.Fatalf("selected row should span full width, got %d", ansi.StringWidth(line0))
	}
}
