package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// ── width invariant (primary regression guard) ────────────────────────────────

// TestRenderMarkdownWidthInvariant is the regression guard for the clipping bug:
// every line returned must have ansi.StringWidth <= width across a range of body
// shapes and budgets, including widths that force wrapping in every construct.
func TestRenderMarkdownWidthInvariant(t *testing.T) {
	bodies := []struct {
		name string
		body string
	}{
		{
			"long paragraph",
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam.",
		},
		{
			"long code line",
			"```go\nfunc veryLongFunctionName(argument1 string, argument2 int, argument3 bool) (string, error) { return \"\", nil }\n```",
		},
		{
			"long bullet",
			"- This is a very long bullet point that should wrap at the width boundary and the continuation line must align under the text column, not the bullet",
		},
		{
			"heading",
			"## This is a heading that might be quite long and needs to be truncated or at least not overflow the terminal",
		},
		{
			"table-ish",
			"| Column One | Column Two | Column Three | Column Four | Column Five | Column Six |",
		},
		{
			"blockquote long",
			"> This blockquote body is intentionally long so that it wraps inside the gutter prefix and we can verify each line stays within budget",
		},
		{
			"ordered list long",
			"1. This ordered list item has enough text to force wrapping at a narrow width, and its continuation must not exceed the budget",
		},
	}

	widths := []int{20, 40, 80}

	for _, tc := range bodies {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s/w=%d", tc.name, w), func(t *testing.T) {
				lines := renderMarkdown(tc.body, w)
				if len(lines) == 0 {
					t.Fatal("expected at least one line, got none")
				}
				for i, line := range lines {
					got := ansi.StringWidth(line)
					if got > w {
						t.Errorf("line %d: ansi.StringWidth=%d exceeds budget %d: %q",
							i, got, w, ansi.Strip(line))
					}
				}
			})
		}
	}
}

// ── inline spans ──────────────────────────────────────────────────────────────

func TestRenderMarkdownInlineBold(t *testing.T) {
	lines := renderMarkdown("**Verdict:**", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	// The asterisk delimiters must be gone.
	if strings.Contains(stripped, "*") {
		t.Errorf("bold delimiters should be stripped, got %q", stripped)
	}
	// The visible text must be present.
	if !strings.Contains(stripped, "Verdict:") {
		t.Errorf("bold text should be present, got %q", stripped)
	}
	// Styling must be applied (ANSI SGR sequence).
	if !strings.Contains(lines[0], "\x1b[") {
		t.Errorf("bold should have ANSI styling, got %q", lines[0])
	}
}

func TestRenderMarkdownInlineCodeSpan(t *testing.T) {
	lines := renderMarkdown("`code`", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	// Backtick delimiters must be gone.
	if strings.Contains(stripped, "`") {
		t.Errorf("code span backticks should be stripped, got %q", stripped)
	}
	if !strings.Contains(stripped, "code") {
		t.Errorf("code content should be present, got %q", stripped)
	}
	if !strings.Contains(lines[0], "\x1b[") {
		t.Errorf("code span should have ANSI styling, got %q", lines[0])
	}
}

func TestRenderMarkdownInlineItalic(t *testing.T) {
	lines := renderMarkdown("*italic*", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	if strings.Contains(stripped, "*") {
		t.Errorf("italic delimiters should be stripped, got %q", stripped)
	}
	if !strings.Contains(stripped, "italic") {
		t.Errorf("italic text should be present, got %q", stripped)
	}
	if !strings.Contains(lines[0], "\x1b[") {
		t.Errorf("italic should have ANSI styling, got %q", lines[0])
	}
}

func TestRenderMarkdownInlineStrikethrough(t *testing.T) {
	lines := renderMarkdown("~~strike~~", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	if strings.Contains(stripped, "~") {
		t.Errorf("strikethrough delimiters should be stripped, got %q", stripped)
	}
	if !strings.Contains(stripped, "strike") {
		t.Errorf("strike text should be present, got %q", stripped)
	}
	if !strings.Contains(lines[0], "\x1b[") {
		t.Errorf("strikethrough should have ANSI styling, got %q", lines[0])
	}
}

func TestRenderMarkdownInlineLinkDistinct(t *testing.T) {
	// Text and URL are different: both should appear; URL in parens.
	lines := renderMarkdown("[Click here](https://example.com)", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	if !strings.Contains(stripped, "Click here") {
		t.Errorf("link text should be present, got %q", stripped)
	}
	if !strings.Contains(stripped, "(https://example.com)") {
		t.Errorf("link URL should appear in parens, got %q", stripped)
	}
	if !strings.Contains(lines[0], "\x1b[") {
		t.Errorf("link should have ANSI styling, got %q", lines[0])
	}
}

func TestRenderMarkdownInlineLinkSameTextURL(t *testing.T) {
	// When text == URL, the URL appears exactly once (no duplication).
	lines := renderMarkdown("[https://example.com](https://example.com)", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	count := strings.Count(stripped, "https://example.com")
	if count != 1 {
		t.Errorf("URL should appear exactly once when text==URL, got %d occurrences: %q", count, stripped)
	}
}

// ── stray punctuation ─────────────────────────────────────────────────────────

func TestRenderMarkdownStrayAsterisk(t *testing.T) {
	// Asterisks surrounded by spaces are not italic delimiters.
	lines := renderMarkdown("2 * 3 = 6", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	if stripped != "2 * 3 = 6" {
		t.Errorf("stray asterisk should pass through unmodified, got %q", stripped)
	}
}

func TestRenderMarkdownStrayUnderscore(t *testing.T) {
	// Underscores inside a word (snake_case) must not trigger italic.
	lines := renderMarkdown("snake_case_word", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	if stripped != "snake_case_word" {
		t.Errorf("snake_case should pass through unmodified, got %q", stripped)
	}
}

// ── code span content is literal ──────────────────────────────────────────────

func TestRenderMarkdownCodeSpanNoNestedMarkdown(t *testing.T) {
	// Asterisks inside a code span must remain as literal characters.
	lines := renderMarkdown("`**not bold**`", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	stripped := ansi.Strip(lines[0])
	if stripped != "**not bold**" {
		t.Errorf("code span should preserve literal asterisks, got %q", stripped)
	}
}

// ── block constructs ──────────────────────────────────────────────────────────

func TestRenderMarkdownHeadings(t *testing.T) {
	for lvl := range 6 {
		hashes := strings.Repeat("#", lvl+1)
		body := fmt.Sprintf("%s Heading level %d", hashes, lvl+1)
		lines := renderMarkdown(body, 80)
		if len(lines) == 0 {
			t.Fatalf("h%d: expected output", lvl+1)
		}
		stripped := ansi.Strip(lines[0])
		want := fmt.Sprintf("Heading level %d", lvl+1)
		if !strings.Contains(stripped, want) {
			t.Errorf("h%d: expected text %q in %q", lvl+1, want, stripped)
		}
		// Heading marks must not appear in stripped output.
		if strings.HasPrefix(stripped, "#") {
			t.Errorf("h%d: heading marks should be stripped, got %q", lvl+1, stripped)
		}
		if !strings.Contains(lines[0], "\x1b[") {
			t.Errorf("h%d: heading should have ANSI styling, got %q", lvl+1, lines[0])
		}
	}
}

func TestRenderMarkdownBulletList(t *testing.T) {
	body := "- first\n- second\n- third"
	lines := renderMarkdown(body, 80)
	if len(lines) < 3 {
		t.Fatalf("expected 3 bullet lines, got %d", len(lines))
	}
	for i, want := range []string{"first", "second", "third"} {
		stripped := ansi.Strip(lines[i])
		if !strings.Contains(stripped, want) {
			t.Errorf("bullet %d: expected %q in %q", i, want, stripped)
		}
		// Each line must have the bullet glyph.
		if !strings.Contains(stripped, "•") {
			t.Errorf("bullet %d: expected '•' glyph in %q", i, stripped)
		}
	}
}

func TestRenderMarkdownNestedBullet(t *testing.T) {
	body := "- top level\n  - nested item"
	lines := renderMarkdown(body, 80)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	topStripped := ansi.Strip(lines[0])
	nestedStripped := ansi.Strip(lines[1])
	if !strings.Contains(topStripped, "top level") {
		t.Errorf("top-level bullet missing text: %q", topStripped)
	}
	if !strings.Contains(nestedStripped, "nested item") {
		t.Errorf("nested bullet missing text: %q", nestedStripped)
	}
	// Nested item must be indented further than the top-level item.
	// Nesting is verified directly by prefix: top-level bullet has no indent,
	// the nested one has 2-space indent before the bullet glyph.
	if !strings.HasPrefix(topStripped, "• ") {
		t.Errorf("top-level bullet should start with '• ', got %q", topStripped)
	}
	if !strings.HasPrefix(nestedStripped, "  • ") {
		t.Errorf("nested bullet should start with '  • ', got %q", nestedStripped)
	}
}

func TestRenderMarkdownOrderedList(t *testing.T) {
	body := "1. first item\n2. second item\n3. third item"
	lines := renderMarkdown(body, 80)
	if len(lines) < 3 {
		t.Fatalf("expected 3 ordered-list lines, got %d", len(lines))
	}
	for i, want := range []string{"first item", "second item", "third item"} {
		stripped := ansi.Strip(lines[i])
		if !strings.Contains(stripped, want) {
			t.Errorf("item %d: expected %q in %q", i, want, stripped)
		}
		numPrefix := fmt.Sprintf("%d. ", i+1)
		if !strings.HasPrefix(stripped, numPrefix) {
			t.Errorf("item %d: expected prefix %q, got %q", i, numPrefix, stripped)
		}
	}
}

func TestRenderMarkdownBlockquoteGutter(t *testing.T) {
	lines := renderMarkdown("> This is a quoted passage", 80)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	for i, line := range lines {
		stripped := ansi.Strip(line)
		// Every blockquote line must start with the "▎ " gutter (matches threadGutter).
		if !strings.HasPrefix(stripped, "▎ ") {
			t.Errorf("blockquote line %d: expected '▎ ' gutter, got %q", i, stripped)
		}
	}
	stripped := ansi.Strip(lines[0])
	if !strings.Contains(stripped, "This is a quoted passage") {
		t.Errorf("blockquote content missing, got %q", stripped)
	}
}

func TestRenderMarkdownHorizontalRule(t *testing.T) {
	for _, hr := range []string{"---", "***", "___"} {
		lines := renderMarkdown(hr, 20)
		if len(lines) == 0 {
			t.Fatalf("%s: expected output", hr)
		}
		stripped := ansi.Strip(lines[0])
		// HR at width 20 should be 20 columns wide.
		if got := ansi.StringWidth(lines[0]); got > 20 {
			t.Errorf("%s: HR width %d exceeds budget 20", hr, got)
		}
		// Stripped content should not contain the original delimiter characters.
		// (It should be a line of "─" box-drawing chars, not the input dashes.)
		if stripped == hr {
			t.Errorf("%s: HR should render as a rule, not the raw delimiter, got %q", hr, stripped)
		}
	}
}

// ── bullet wrap: continuation alignment ──────────────────────────────────────

// TestRenderMarkdownBulletWrapContinuation asserts that when a bullet item wraps,
// the continuation lines are indented under the TEXT column, not the bullet glyph.
// At width 20, "• " prefix = 2 cols, so the text budget is 18 cols.
func TestRenderMarkdownBulletWrapContinuation(t *testing.T) {
	// This text is longer than 18 chars so it must wrap.
	body := "- Short bullet item that wraps at narrow"
	lines := renderMarkdown(body, 20)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping at width 20, got %d line(s): %v",
			len(lines), ansi.Strip(strings.Join(lines, "|")))
	}
	first := ansi.Strip(lines[0])
	second := ansi.Strip(lines[1])

	// First line begins with "• " (bullet + space).
	if !strings.HasPrefix(first, "• ") {
		t.Errorf("first line should start with '• ', got %q", first)
	}
	// Continuation begins with 2 spaces (same width as "• "), NOT with "• ".
	if !strings.HasPrefix(second, "  ") {
		t.Errorf("continuation should be indented 2 spaces, got %q", second)
	}
	if strings.HasPrefix(second, "• ") {
		t.Errorf("continuation must not have the bullet glyph, got %q", second)
	}
	// Width invariant on both lines.
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 20 {
			t.Errorf("line %d: width %d > 20: %q", i, got, ansi.Strip(line))
		}
	}
}

// ── fenced code blocks ────────────────────────────────────────────────────────

func TestRenderMarkdownFencedCodeGoHighlighted(t *testing.T) {
	body := "```go\nfunc hello() {}\n```"
	lines := renderMarkdown(body, 80)
	if len(lines) == 0 {
		t.Fatal("expected output from fenced code block")
	}
	// Every non-blank output line must start with the code-block indent.
	// (Line starts with "  " raw bytes; ANSI sequences follow, not precede, the indent.)
	for i, line := range lines {
		if stripped := ansi.Strip(line); stripped != "" && !strings.HasPrefix(line, codeBlockIndent) {
			t.Errorf("code line %d: expected indent %q prefix, got %q", i, codeBlockIndent, line)
		}
	}
	// Join stripped lines and check source text is present.
	allStripped := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(allStripped, "func hello()") {
		t.Errorf("source text should survive highlighting: %q", allStripped)
	}
	// Highlighting produces ANSI codes; if not, the raw line equals its stripped form.
	// Not a hard failure: some CI environments run with no-color profiles.
	if lines[0] == ansi.Strip(lines[0]) {
		t.Logf("note: no ANSI highlighting on go code line (headless/no-color env?): %q", lines[0])
	}
}

func TestRenderMarkdownFencedCodeUnknownLanguage(t *testing.T) {
	// Unknown languages must not produce an error or empty output.
	body := "```xyzzy_unknownlang\nsome code here\n```"
	lines := renderMarkdown(body, 80)
	if len(lines) == 0 {
		t.Fatal("expected output even for unknown language")
	}
	allStripped := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(allStripped, "some code here") {
		t.Errorf("code text should survive unknown language: %q", allStripped)
	}
}

func TestRenderMarkdownFencedCodeLongLineHardWrapped(t *testing.T) {
	// A code line longer than the budget must be hard-wrapped (not dropped or
	// word-wrapped), and each segment must satisfy the width invariant.
	longLine := strings.Repeat("x", 60)
	body := "```\n" + longLine + "\n```"
	const w = 20
	lines := renderMarkdown(body, w)
	if len(lines) == 0 {
		t.Fatal("expected output")
	}
	// Verify: no line exceeds the budget.
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > w {
			t.Errorf("code line %d: width %d > %d: %q", i, got, w, ansi.Strip(line))
		}
	}
	// All 60 x-chars must be present across the segments (nothing dropped).
	allStripped := ansi.Strip(strings.Join(lines, ""))
	// The 60 x's will be in multiple segments, concatenated without spaces.
	xCount := strings.Count(allStripped, "x")
	if xCount < 60 {
		t.Errorf("hard-wrap must preserve all %d chars; found only %d x's in %q", 60, xCount, allStripped)
	}
}

func TestRenderMarkdownFencedCodePreservesBlankLines(t *testing.T) {
	body := "```\nline one\n\nline three\n```"
	lines := renderMarkdown(body, 80)
	// There must be an empty (blank) line between line one and line three.
	allStripped := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(allStripped, "line one") || !strings.Contains(allStripped, "line three") {
		t.Fatalf("expected both code lines, got: %q", allStripped)
	}
	hasBlank := false
	for _, l := range lines {
		if ansi.Strip(l) == codeBlockIndent || l == codeBlockIndent || strings.TrimSpace(ansi.Strip(l)) == "" {
			hasBlank = true
			break
		}
	}
	if !hasBlank {
		t.Errorf("blank line inside code block should be preserved; lines: %q", lines)
	}
}

// ── edge cases ────────────────────────────────────────────────────────────────

func TestRenderMarkdownWidthZeroNoWrap(t *testing.T) {
	// A long body at width=0 must be returned as-is (no wrapping, no truncation)
	// but styling is still applied.
	long := "This is a fairly long paragraph that would normally be word wrapped at a reasonable terminal width."
	lines := renderMarkdown(long, 0)
	if len(lines) == 0 {
		t.Fatal("expected output at width=0")
	}
	// All content fits on a single line (no wrapping occurred).
	if len(lines) != 1 {
		t.Errorf("width=0 should produce one line for a single-paragraph body, got %d", len(lines))
	}
	stripped := ansi.Strip(lines[0])
	if !strings.Contains(stripped, "word wrapped") {
		t.Errorf("full content should be present at width=0, got %q", stripped)
	}
}

func TestRenderMarkdownEmptyBodyReturnsNil(t *testing.T) {
	for _, body := range []string{"", "   ", "\n", "\n\n", " \t ", "\r\n"} {
		if got := renderMarkdown(body, 80); got != nil {
			t.Errorf("body %q: expected nil, got %v", body, got)
		}
	}
}

// ── mixed body ────────────────────────────────────────────────────────────────

// TestRenderMarkdownMixedBody exercises the real-world case that triggered this
// feature: a PR review comment with headings, bullets, bold verdict, and a code
// block — all in one body.
func TestRenderMarkdownMixedBody(t *testing.T) {
	body := "## Summary\n\n**Verdict:** looks good\n\n- point one\n- point two\n\n```go\nfmt.Println(\"hello\")\n```"
	lines := renderMarkdown(body, 80)
	if len(lines) == 0 {
		t.Fatal("expected output for mixed body")
	}
	all := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Summary", "Verdict:", "looks good", "point one", "point two", "fmt.Println"} {
		if !strings.Contains(all, want) {
			t.Errorf("mixed body: expected %q in output, got:\n%s", want, all)
		}
	}
	// Width invariant must hold across all lines.
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 80 {
			t.Errorf("mixed body line %d: width %d > 80: %q", i, got, ansi.Strip(line))
		}
	}
}
