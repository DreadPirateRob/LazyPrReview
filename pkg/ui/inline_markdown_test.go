package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Inline diff comments are markdown, same as the overview. They used to be emitted
// as raw split lines, so `**Verdict:**` showed its asterisks.
func TestInlineDiffCommentRendersMarkdown(t *testing.T) {
	m := seededModel(t)
	m.PRDetail.Threads[0].Comments[0].Body = "**Verdict:** looks `fine`"
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	lines, _ := buildDiffRows(m, 0, 100)
	body := ansi.Strip(strings.Join(lines, "\n"))

	if !strings.Contains(body, "Verdict:") {
		t.Fatalf("expected the comment text:\n%s", body)
	}
	if strings.Contains(body, "**Verdict:**") {
		t.Fatalf("inline diff comments must render markdown, not raw:\n%s", body)
	}
	if strings.Contains(body, "`fine`") {
		t.Fatalf("code span backticks should be consumed:\n%s", body)
	}
}

// And they must respect the width budget. buildDiffRows took no width at all
// before, so a long comment line ran past the pane edge and got clipped.
func TestInlineDiffCommentWrapsToWidth(t *testing.T) {
	m := seededModel(t)
	m.PRDetail.Threads[0].Comments[0].Body = strings.Repeat("chatter ", 60)
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)

	const budget = 60
	lines, _ := buildDiffRows(m, 0, budget)
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > budget {
			t.Fatalf("diff line %d is %d cols (budget %d): %q", i, w, budget, ansi.Strip(l))
		}
	}
	if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "chatter") {
		t.Fatal("expected the wrapped comment body")
	}
}

// Mentions are styled inside markdown bodies, and the viewer's own handle stands
// out. Previously the diff had mentions but no markdown while the overview had
// markdown but no mentions — the two paths disagreed.
func TestMarkdownStylesMentions(t *testing.T) {
	out := renderMarkdownFor("ping @someone", 80, "adrian")
	if len(out) == 0 {
		t.Fatal("expected output")
	}
	if ansi.Strip(out[0]) != "ping @someone" {
		t.Fatalf("text must survive styling, got %q", ansi.Strip(out[0]))
	}
	if out[0] == ansi.Strip(out[0]) {
		t.Fatal("a mention should be styled")
	}

	mine := renderMarkdownFor("ping @adrian", 80, "adrian")
	theirs := renderMarkdownFor("ping @someone", 80, "adrian")
	if mine[0] == theirs[0] {
		t.Fatal("the viewer's own mention should style differently from others")
	}
}

// The boundary rule survives the move into the inline renderer: an email address
// is not a mention.
func TestMarkdownMentionBoundaryInBody(t *testing.T) {
	out := renderMarkdownFor("mail me at name@example.com", 80, "adrian")
	if len(out) == 0 {
		t.Fatal("expected output")
	}
	if out[0] != ansi.Strip(out[0]) {
		t.Fatalf("an email address must not be styled as a mention: %q", out[0])
	}
}

// A mention inside a code span stays literal — code spans are never reinterpreted.
func TestMarkdownMentionInCodeSpanIsLiteral(t *testing.T) {
	out := renderMarkdownFor("use `@handle` here", 80, "adrian")
	if len(out) == 0 {
		t.Fatal("expected output")
	}
	if !strings.Contains(ansi.Strip(out[0]), "@handle") {
		t.Fatalf("code span content should be preserved verbatim: %q", ansi.Strip(out[0]))
	}
}

// Overview bodies go through the same viewer-aware renderer, so mention styling
// can't drift back to being diff-only.
func TestOverviewBodyStylesMentions(t *testing.T) {
	m := overviewModel(t)
	m.ViewerLogin = "adrian"
	m.PRDetail.Timeline[3].Body = "@adrian please look"
	m = setMainOverview(m, *m.PRDetail)

	var found bool
	for _, l := range m.MainLines {
		if strings.Contains(ansi.Strip(l), "@adrian") {
			found = true
			if l == ansi.Strip(l) {
				t.Fatalf("overview mention should be styled: %q", l)
			}
		}
	}
	if !found {
		t.Fatal("expected the mention to render in the overview")
	}
}

// The no-viewer wrapper still works (unit tests and non-UI callers use it).
func TestRenderMarkdownWrapperHasNoViewer(t *testing.T) {
	out := renderMarkdown("ping @adrian", 80)
	if len(out) == 0 {
		t.Fatal("expected output")
	}
	if ansi.Strip(out[0]) != "ping @adrian" {
		t.Fatalf("text must survive, got %q", ansi.Strip(out[0]))
	}
}
