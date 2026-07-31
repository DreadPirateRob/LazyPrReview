package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// An @handle must be detected only at a real boundary. Matching inside an email
// address or an asset name would both mis-highlight and, worse, make m / M jump to
// threads that never addressed anyone.
func TestMentionSpansBoundaries(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"ping @adrian please", []string{"@adrian"}},
		{"@adrian at line start", []string{"@adrian"}},
		{"(@adrian) in parens", []string{"@adrian"}},
		{"mail me at name@example.com", nil},
		{"use logo@2x.png here", nil},
		{"team ping @acme/backend", []string{"@acme/backend"}},
		{"two @a and @b-c", []string{"@a", "@b-c"}},
		{"no handles here", nil},
	} {
		var got []string
		for _, loc := range mentionSpans(tc.in) {
			got = append(got, tc.in[loc[0]:loc[1]])
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("mentionSpans(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Highlighting must preserve the text exactly; only styling is added.
func TestHighlightMentionsPreservesText(t *testing.T) {
	in := "hey @adrian and @other, see name@example.com"
	out := highlightMentions(in, "adrian")
	if ansi.Strip(out) != in {
		t.Fatalf("highlighting must not alter text:\n got %q\nwant %q", ansi.Strip(out), in)
	}
	if out == in {
		t.Fatal("expected styling to be applied")
	}
}

// The viewer's own handle gets the stronger treatment, case-insensitively.
func TestHighlightMentionsDistinguishesViewer(t *testing.T) {
	mine := highlightMentions("ping @Adrian", "adrian")
	theirs := highlightMentions("ping @somebody", "adrian")
	if mine == theirs {
		t.Fatal("the viewer's own mention should style differently from others")
	}
}

func TestThreadMentionsViewer(t *testing.T) {
	th := domain.Thread{Comments: []domain.Comment{
		{Body: "unrelated"},
		{Body: "what do you think @Adrian?"},
	}}
	if !threadMentionsViewer(th, "adrian") {
		t.Fatal("should detect the viewer's mention case-insensitively")
	}
	if threadMentionsViewer(th, "someone-else") {
		t.Fatal("should not match a different login")
	}
	if threadMentionsViewer(th, "") {
		t.Fatal("no viewer login means no mentions")
	}
	email := domain.Thread{Comments: []domain.Comment{{Body: "reach me at adrian@example.com"}}}
	if threadMentionsViewer(email, "example") {
		t.Fatal("an email domain must not count as a mention")
	}
}

// A thread mentioning you is flagged in its summary, so a COLLAPSED thread still
// shows it needs your attention.
func TestMentionBadgeOnCollapsedThread(t *testing.T) {
	m := seededModel(t)
	m.ViewerLogin = "adrian"
	m.PRDetail.Threads[0].IsResolved = true // collapses by default
	m.PRDetail.Threads[0].Comments[0].Body = "@adrian thoughts?"
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	lines, rows := buildDiffRows(m, 0)
	hdr := findThreadHeader(rows, "t1")
	if hdr < 0 {
		t.Fatal("expected the thread summary to render")
	}
	if !strings.Contains(ansi.Strip(lines[hdr]), "@you") {
		t.Fatalf("collapsed thread mentioning the viewer should be badged: %q", ansi.Strip(lines[hdr]))
	}
}

// m cycles only threads that mention the viewer, and toasts when there are none.
func TestMentionNavigation(t *testing.T) {
	m := seededModel(t)
	m.ViewerLogin = "adrian"
	m.FocusStack = []FocusContext{FocusMain}
	m = setMainDiff(m, 0)
	m.MainMode = MainDiff
	m.MainFileIndex = 0

	// No mentions yet -> toast, no movement.
	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "m", Code: 'm'})
	if got.Toast.Message != ToastNoMentions {
		t.Fatalf("expected a no-mentions toast, got %q", got.Toast.Message)
	}

	// Mention the viewer in bar.txt's thread (t2) and jump to it.
	m.PRDetail.Threads[1].Comments[0].Body = "@adrian please look"
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
	if n := len(mentionThreads(m)); n != 1 {
		t.Fatalf("expected exactly one mentioning thread, got %d", n)
	}

	jumped, _ := UpdateMain(m, tea.KeyPressMsg{Text: "m", Code: 'm'})
	if jumped.Toast.Message == ToastNoMentions {
		t.Fatal("should have found the mentioning thread")
	}
	if got := threadIDAtMainCursor(jumped); got != "t2" {
		t.Fatalf("m should land on the mentioning thread t2, got %q", got)
	}
}

// File-level threads have no code line but still render inline, so mention
// navigation must reach them — otherwise "threads mentioning me" silently skips a
// whole class of comment.
func TestMentionNavigationReachesUnanchoredThreads(t *testing.T) {
	m := seededModel(t)
	m.ViewerLogin = "adrian"
	m.PRDetail.Threads = []domain.Thread{{
		ID: "tfile", Path: "foo.txt", DiffSide: "RIGHT", Line: nil,
		Comments: []domain.Comment{{Author: "someone", Body: "@adrian whole-file question"}},
	}}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	index := mentionThreads(m)
	if len(index) != 1 {
		t.Fatalf("an unanchored mentioning thread should be in the index, got %d", len(index))
	}
	if index[0].Anchored {
		t.Fatal("precondition: the fixture thread should be unanchored")
	}

	got := cycleMentionJump(m, +1)
	if got.MainFileIndex < 0 || got.DiffFiles[got.MainFileIndex].Path != "foo.txt" {
		t.Fatal("jumping to an unanchored thread should still open its file")
	}
	if id := threadIDAtMainCursor(got); id != "tfile" {
		t.Fatalf("cursor should land on the unanchored thread's row, got %q", id)
	}
}

// Demoting a binding from the hint bar must NOT remove it from `?` help — that is
// the whole contract of the demotion list.
func TestDemotedBindingsStayInHelp(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusMain}
	bar := HintBarView(m)
	help := helpOverlayView(m)

	for _, tc := range []struct{ action, key string }{
		{"centerCursor", "zz"},
		{"prevUnresolvedThread", "T"},
		{"prevMention", "M"},
	} {
		if !strings.Contains(help, tc.action) {
			t.Errorf("%s must remain documented in ? help", tc.action)
		}
		_ = tc.key
	}
	// And the forward halves stay visible in the bar.
	for _, want := range []string{"Mention", "Next thread", "Fold"} {
		if !strings.Contains(bar, want) {
			t.Errorf("hint bar should advertise %q: %q", want, bar)
		}
	}
}
