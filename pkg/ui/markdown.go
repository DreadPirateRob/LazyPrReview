package ui

import (
	"bytes"
	"strings"
	"unicode/utf8"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// Markdown-specific styles. Heading brightness decreases with depth, matching
// the visual hierarchy cue readers expect.
var (
	mdHeadingStyles = [6]lipgloss.Style{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")), // h1 bright white
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")), // h2 bright cyan
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")), // h3 bright yellow
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")), // h4 bright green
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")), // h5 bright magenta
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")), // h6 bright blue
	}
	mdBoldStyle     = lipgloss.NewStyle().Bold(true)
	mdItalicStyle   = lipgloss.NewStyle().Italic(true)
	mdCodeSpanStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	mdStrikeStyle   = lipgloss.NewStyle().Strikethrough(true).Faint(true)
	mdLinkTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Underline(true)
	mdLinkURLStyle  = lipgloss.NewStyle().Faint(true)

	// mdBQGutter matches threadGutter ("▎ ") from mainrows.go — same annotation-gutter
	// idiom so blockquotes and inline thread annotations read as the same visual class.
	mdBQGutter = "▎ "
	mdBQStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	mdHRStyle = lipgloss.NewStyle().Faint(true)
)

// codeBlockIndent is the 2-space prefix applied to every code block line so
// the block visually sits inside the surrounding text.
const codeBlockIndent = "  "

// renderMarkdown renders a markdown body into terminal lines wrapped to width,
// with no viewer context (so every @mention styles the same).
func renderMarkdown(body string, width int) []string {
	return renderMarkdownFor(body, width, "")
}

// renderMarkdownFor renders a markdown body into terminal lines wrapped to width.
// Each returned element is one visual line; the caller tags each with fold metadata.
//
// viewer is the logged-in user, so their own @handle can be styled distinctly from
// other mentions. Mentions are handled here rather than by a separate pass because
// they are an inline span like emphasis: styling them afterwards would mean parsing
// text that already carries ANSI escapes.
//
// width <= 0 disables wrapping/truncation (headless/test path); styling still applies.
// Empty or whitespace-only body returns nil.
func renderMarkdownFor(body string, width int, viewer string) []string {
	body = strings.TrimRight(body, "\n\r")
	if strings.TrimSpace(body) == "" {
		return nil
	}

	rawLines := strings.Split(body, "\n")
	var out []string

	// Accumulated block lines, flushed when the block type changes.
	var paraLines []string
	var bqLines []string

	// Fenced code block state.
	var fenceLines []string
	var fenceLang, fenceDelim string
	inFence := false

	flushPara := func() {
		if len(paraLines) == 0 {
			return
		}
		text := strings.Join(paraLines, " ")
		paraLines = nil
		styled := renderInline(text, viewer)
		out = append(out, wrapToWidth(styled, width)...)
	}

	flushBQ := func() {
		if len(bqLines) == 0 {
			return
		}
		text := strings.Join(bqLines, " ")
		bqLines = nil
		out = append(out, renderBlockquote(text, width, viewer)...)
	}

	for _, line := range rawLines {
		// ── fenced code block (consumes lines until the closing fence) ──────────
		if inFence {
			if isFenceClose(line, fenceDelim) {
				out = append(out, highlightCode(fenceLang, fenceLines, width)...)
				fenceLines = nil
				fenceLang, fenceDelim = "", ""
				inFence = false
			} else {
				fenceLines = append(fenceLines, line)
			}
			continue
		}

		// ── fence open ───────────────────────────────────────────────────────────
		if lang, delim, ok := parseFenceOpen(line); ok {
			flushPara()
			flushBQ()
			fenceLang, fenceDelim = lang, delim
			fenceLines = nil
			inFence = true
			continue
		}

		// ── blank line ───────────────────────────────────────────────────────────
		if strings.TrimSpace(line) == "" {
			flushPara()
			flushBQ()
			if len(out) > 0 {
				out = append(out, "")
			}
			continue
		}

		// ── ATX heading (#..######) ───────────────────────────────────────────────
		if lvl, text := parseATXHeading(line); lvl > 0 {
			flushPara()
			flushBQ()
			out = append(out, renderHeading(lvl, text, width, viewer))
			continue
		}

		// ── horizontal rule (---, ***, ___) ─────────────────────────────────────
		if isHR(line) {
			flushPara()
			flushBQ()
			out = append(out, renderHR(width))
			continue
		}

		// ── blockquote ───────────────────────────────────────────────────────────
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, ">") {
			flushPara()
			content := trimmed[1:]
			if len(content) > 0 && content[0] == ' ' {
				content = content[1:]
			}
			bqLines = append(bqLines, content)
			continue
		}
		// A non-blockquote line ends any accumulated blockquote.
		flushBQ()

		// ── bullet list item (-, *, +) ────────────────────────────────────────────
		if indent, text, ok := parseBulletItem(line); ok {
			flushPara()
			prefix := strings.Repeat(" ", indent) + "• "
			out = append(out, renderListItem(prefix, text, width, viewer)...)
			continue
		}

		// ── ordered list item (1., 2., …) ────────────────────────────────────────
		if indent, numStr, text, ok := parseOrderedItem(line); ok {
			flushPara()
			prefix := strings.Repeat(" ", indent) + numStr + ". "
			out = append(out, renderListItem(prefix, text, width, viewer)...)
			continue
		}

		// ── paragraph ────────────────────────────────────────────────────────────
		paraLines = append(paraLines, strings.TrimSpace(line))
	}

	flushPara()
	flushBQ()

	// Unclosed fence (malformed input): render whatever was collected.
	if inFence && len(fenceLines) > 0 {
		out = append(out, highlightCode(fenceLang, fenceLines, width)...)
	}

	// Strip trailing blank lines that contribute no visible content.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	return out
}

// ── block helpers ─────────────────────────────────────────────────────────────

func renderHeading(level int, text string, width int, viewer string) string {
	st := mdHeadingStyles[level-1]
	result := st.Render(renderInline(text, viewer))
	// Headings are not wrapped; truncate if they exceed the budget.
	if width > 0 && ansi.StringWidth(result) > width {
		result = ansi.Truncate(result, width, "")
	}
	return result
}

func renderHR(width int) string {
	if width <= 0 {
		return mdHRStyle.Render("─────────")
	}
	// "─" (U+2500) is 1 column wide, so Repeat(width) fills exactly width columns.
	return mdHRStyle.Render(strings.Repeat("─", width))
}

// renderBlockquote renders one blockquote block (already stripped of "> ") with
// the same "▎ " gutter idiom used by threadGutter in mainrows.go.
func renderBlockquote(text string, width int, viewer string) []string {
	gutterW := ansi.StringWidth(mdBQGutter) // "▎ " = 2 columns
	styled := mdBQStyle.Render(renderInline(text, viewer))
	if width <= 0 {
		return []string{mdBQGutter + styled}
	}
	budget := width - gutterW
	if budget < 1 {
		budget = 1
	}
	segs := wrapToWidth(styled, budget)
	if len(segs) == 0 {
		return []string{mdBQGutter}
	}
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		out = append(out, mdBQGutter+seg)
	}
	return out
}

// renderListItem renders a bullet or ordered list item. fullPrefix includes the
// leading indent, marker, and trailing space. Continuation lines are padded to
// the same width as fullPrefix so the text column stays aligned across wraps.
func renderListItem(fullPrefix string, text string, width int, viewer string) []string {
	prefixW := ansi.StringWidth(fullPrefix)
	styledText := renderInline(text, viewer)
	if width <= 0 {
		return []string{fullPrefix + styledText}
	}
	budget := width - prefixW
	if budget < 1 {
		budget = 1
	}
	segs := wrapToWidth(styledText, budget)
	if len(segs) == 0 {
		return []string{fullPrefix}
	}
	cont := strings.Repeat(" ", prefixW)
	out := make([]string, 0, len(segs))
	for i, seg := range segs {
		if i == 0 {
			out = append(out, fullPrefix+seg)
		} else {
			out = append(out, cont+seg)
		}
	}
	return out
}

// highlightCode syntax-highlights code lines with chroma (terminal256 / monokai)
// and hard-wraps any line that would otherwise exceed the width budget.
// Falls back to plain text on any chroma error — a renderer must never fail a body.
func highlightCode(lang string, lines []string, width int) []string {
	const indentW = len(codeBlockIndent)
	code := strings.Join(lines, "\n")

	// Attempt chroma highlighting. language="" falls back to analyser then Fallback lexer.
	var buf bytes.Buffer
	highlighted := false
	func() {
		lex := lexers.Get(lang)
		if lex == nil {
			lex = lexers.Analyse(code)
		}
		if lex == nil {
			lex = lexers.Fallback
		}
		lex = chroma.Coalesce(lex)

		sty := styles.Get("monokai")
		if sty == nil {
			sty = styles.Fallback
		}

		it, err := lex.Tokenise(nil, code)
		if err != nil {
			return
		}
		f := formatters.Get("terminal256")
		if err := f.Format(&buf, sty, it); err != nil {
			buf.Reset()
			return
		}
		highlighted = true
	}()

	var raw string
	if highlighted {
		raw = strings.TrimRight(buf.String(), "\n")
	} else {
		raw = code
	}

	budget := 0
	if width > 0 {
		budget = width - indentW
		if budget < 1 {
			budget = 1
		}
	}

	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if budget > 0 && ansi.StringWidth(line) > budget {
			// Hard-wrap (not word-wrap) to avoid corrupting code structure.
			for _, hs := range strings.Split(ansi.Hardwrap(line, budget, false), "\n") {
				out = append(out, codeBlockIndent+hs)
			}
		} else {
			out = append(out, codeBlockIndent+line)
		}
	}
	return out
}

// wrapToWidth word-wraps s to at most width visible columns, using Wordwrap first
// then Hardwrap to guarantee the invariant even for unbreakable long tokens.
// Returns the input as a single-element slice when width <= 0.
func wrapToWidth(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	wrapped := ansi.Wordwrap(s, width, "")
	segs := strings.Split(wrapped, "\n")
	// Remove trailing empty element that Split produces when wrapped ends with "\n".
	for len(segs) > 0 && segs[len(segs)-1] == "" {
		segs = segs[:len(segs)-1]
	}
	var out []string
	for _, seg := range segs {
		if ansi.StringWidth(seg) <= width {
			out = append(out, seg)
		} else {
			// Unbreakable token (e.g., long URL): hard-wrap as last resort.
			for _, hs := range strings.Split(ansi.Hardwrap(seg, width, false), "\n") {
				out = append(out, hs)
			}
		}
	}
	return out
}

// ── block parsers ─────────────────────────────────────────────────────────────

func parseATXHeading(line string) (level int, text string) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return 0, ""
	}
	// Must be followed by a space (or end of line for an empty heading).
	if i < len(line) && line[i] != ' ' {
		return 0, ""
	}
	text = strings.TrimSpace(line[i:])
	// Strip optional closing # sequence (e.g. "## Title ##").
	text = strings.TrimRight(text, " #")
	return i, text
}

// isHR reports whether line is a thematic break (3+ identical -, *, or _ with optional spaces).
func isHR(line string) bool {
	s := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

func parseFenceOpen(line string) (lang, delim string, ok bool) {
	t := strings.TrimLeft(line, " \t")
	for _, d := range []string{"```", "~~~"} {
		if strings.HasPrefix(t, d) {
			return strings.TrimSpace(t[len(d):]), d, true
		}
	}
	return "", "", false
}

// isFenceClose reports whether line closes a fence started with delim.
// The closing fence must consist entirely of the delimiter character.
func isFenceClose(line, delim string) bool {
	s := strings.TrimSpace(line)
	if len(s) < len(delim) {
		return false
	}
	ch := delim[0]
	for i := range len(s) {
		if s[i] != ch {
			return false
		}
	}
	return true
}

// parseBulletItem parses a line of the form "[spaces][-*+] text".
// Returns the leading-space indent count and the item text on success.
func parseBulletItem(line string) (indent int, text string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, "", false
	}
	c := line[i]
	if c != '-' && c != '*' && c != '+' {
		return 0, "", false
	}
	if i+1 >= len(line) || line[i+1] != ' ' {
		return 0, "", false
	}
	return i, strings.TrimSpace(line[i+2:]), true
}

// parseOrderedItem parses a line of the form "[spaces]N. text".
// Returns the indent count, the raw number string, and the item text.
func parseOrderedItem(line string) (indent int, numStr string, text string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	j := i
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
	}
	if j == i || j >= len(line) || line[j] != '.' {
		return 0, "", "", false
	}
	if j+1 >= len(line) || line[j+1] != ' ' {
		return 0, "", "", false
	}
	return i, line[i:j], strings.TrimSpace(line[j+2:]), true
}

// ── inline renderer ───────────────────────────────────────────────────────────

// renderInline processes inline markdown spans within a single text run.
// It is hand-rolled (no library) and intentionally conservative: unmatched or
// ambiguous delimiters pass through as literal text rather than consuming the
// rest of the line.
func renderInline(s string, viewer string) string {
	if s == "" {
		return ""
	}
	var buf strings.Builder
	buf.Grow(len(s) + 16)
	var prev rune // last visible rune emitted; 0 = start of string

	for len(s) > 0 {
		// (1) Code span — highest priority; content is never re-interpreted.
		if s[0] == '`' {
			if j := strings.IndexByte(s[1:], '`'); j >= 0 {
				buf.WriteString(mdCodeSpanStyle.Render(s[1 : j+1]))
				s = s[j+2:]
				prev = '`'
				continue
			}
		}

		// (1b) @mention — a self-contained span, so it is handled here rather than
		// by a post-pass over already-styled text. `prev` carries the previous
		// visible rune, which is exactly the left-boundary check that stops
		// name@example.com and logo@2x.png from reading as mentions.
		if s[0] == '@' && !isWordRune(prev) {
			if loc := mentionRE.FindStringIndex(s); loc != nil && loc[0] == 0 {
				at := s[:loc[1]]
				if viewer != "" && strings.EqualFold(at, "@"+viewer) {
					buf.WriteString(mentionYouStyle.Render(at))
				} else {
					buf.WriteString(mentionStyle.Render(at))
				}
				s = s[loc[1]:]
				prev = '@'
				continue
			}
		}

		// (2) Link [text](url)
		if s[0] == '[' {
			if lnkText, lnkURL, n := parseInlineLink(s); n > 0 {
				styledText := mdLinkTextStyle.Render(renderInline(lnkText, viewer))
				if lnkText == lnkURL {
					buf.WriteString(styledText)
				} else {
					buf.WriteString(styledText)
					buf.WriteString(" (")
					buf.WriteString(mdLinkURLStyle.Render(lnkURL))
					buf.WriteByte(')')
				}
				s = s[n:]
				prev = ')'
				continue
			}
		}

		// (3) Strikethrough ~~...~~
		if strings.HasPrefix(s, "~~") {
			if j := strings.Index(s[2:], "~~"); j >= 0 {
				buf.WriteString(mdStrikeStyle.Render(renderInline(s[2:j+2], viewer)))
				s = s[j+4:]
				prev = '~'
				continue
			}
		}

		// (4) Bold **...** — checked before single * to avoid greedy collision.
		if strings.HasPrefix(s, "**") {
			if j := strings.Index(s[2:], "**"); j >= 0 && (len(s) < 3 || s[2] != ' ') {
				buf.WriteString(mdBoldStyle.Render(renderInline(s[2:j+2], viewer)))
				s = s[j+4:]
				prev = '*'
				continue
			}
			// No closing **: emit one literal '*' and try again on the remaining '*...'.
			buf.WriteByte('*')
			s = s[1:]
			prev = '*'
			continue
		}

		// (5) Bold __...__ — opening __ must not be inside a word.
		if strings.HasPrefix(s, "__") && !isWordRune(prev) {
			if j := strings.Index(s[2:], "__"); j >= 0 && (len(s) < 3 || s[2] != ' ') {
				buf.WriteString(mdBoldStyle.Render(renderInline(s[2:j+2], viewer)))
				s = s[j+4:]
				prev = '_'
				continue
			}
			buf.WriteByte('_')
			s = s[1:]
			prev = '_'
			continue
		}

		// (6) Italic *...* — opening * not followed by space, to preserve "2 * 3".
		if s[0] == '*' && len(s) > 2 && s[1] != ' ' && s[1] != '*' {
			if j := findClosingStar(s[1:]); j >= 0 {
				buf.WriteString(mdItalicStyle.Render(renderInline(s[1:j+1], viewer)))
				s = s[j+2:]
				prev = '*'
				continue
			}
		}

		// (7) Italic _..._ — opening _ must not be inside a word (handles snake_case).
		if s[0] == '_' && !isWordRune(prev) && len(s) > 2 && s[1] != ' ' && s[1] != '_' {
			if j := findClosingUnder(s[1:]); j >= 0 {
				buf.WriteString(mdItalicStyle.Render(renderInline(s[1:j+1], viewer)))
				s = s[j+2:]
				prev = '_'
				continue
			}
		}

		// Literal rune (handles multibyte UTF-8 correctly).
		r, size := utf8.DecodeRuneInString(s)
		buf.WriteRune(r)
		s = s[size:]
		prev = r
	}
	return buf.String()
}

// isWordRune reports whether r is a letter, digit, or underscore — characters
// that suppress delimiter recognition when adjacent to _ markers.
func isWordRune(r rune) bool {
	if r == 0 {
		return false // start of string
	}
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// parseInlineLink tries to match "[text](url)" starting at s[0]=='['.
// Returns the text, URL, and byte count consumed; n==0 means no match.
func parseInlineLink(s string) (text, url string, n int) {
	if s[0] != '[' {
		return "", "", 0
	}
	bracket := strings.IndexByte(s, ']')
	if bracket < 0 || bracket+1 >= len(s) || s[bracket+1] != '(' {
		return "", "", 0
	}
	paren := strings.IndexByte(s[bracket+2:], ')')
	if paren < 0 {
		return "", "", 0
	}
	return s[1:bracket], s[bracket+2 : bracket+2+paren], bracket + 2 + paren + 1
}

// findClosingStar finds the index of a closing '*' in s (s is the text after an
// opening '*'). Skips over code spans to avoid false matches inside backtick pairs.
// Returns -1 when no valid closer exists.
func findClosingStar(s string) int {
	i := 0
	for i < len(s) {
		if s[i] == '`' {
			// Skip code span so closing * inside backticks isn't a closer.
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				i += j + 2
				continue
			}
		}
		if s[i] == '*' {
			// A closer must not be preceded by a space.
			if i > 0 && s[i-1] == ' ' {
				i++
				continue
			}
			return i
		}
		i++
	}
	return -1
}

// findClosingUnder finds the index of a closing '_' in s. The closer must not
// be followed by a word character, preventing a match inside snake_case tokens.
func findClosingUnder(s string) int {
	i := 0
	for i < len(s) {
		if s[i] == '`' {
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				i += j + 2
				continue
			}
		}
		if s[i] == '_' {
			// Closer must not be followed by a word character.
			if i+1 < len(s) && isWordByte(s[i+1]) {
				i++
				continue
			}
			return i
		}
		i++
	}
	return -1
}

// isWordByte is the ASCII-only fast path for isWordRune, used in findClosingUnder
// where the post-delimiter char is a raw byte from the scanner.
func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
