package ui

import (
	"bytes"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
)

// diffHighlightMaxLines is the performance floor: above this many rendered lines a
// file is shown without syntax highlighting. SPEC §10 states this bound.
const diffHighlightMaxLines = 5000

// diffHighlightCache memoises highlightedDiffLines results. The Model is
// copied by value throughout bubbletea dispatch so it cannot hold the cache;
// a package-level map guarded by a mutex is the correct location.
//
// The key fingerprints the rendered CONTENT, not just the path and line count.
// Path plus count collides in two ordinary situations: switching to another PR
// that touches the same file with the same number of rendered lines, and
// refetching after the author edited a line in place — where the count is
// identical and only the text moved. Either would serve colours computed from
// code that is no longer on screen.
var (
	diffHighlightMu    sync.Mutex
	diffHighlightCache = map[string][]string{}
)

// diffHighlightKey fingerprints a file's rendered lines. FNV-1a over
// kind+text+hunk is orders of magnitude cheaper than the chroma pass it guards, so
// correctness here costs nothing measurable.
//
// HunkIndex is part of the fingerprint because the highlight depends on it: lexer
// state resets at every hunk boundary, so two diffs with byte-identical lines split
// into different hunks legitimately highlight differently.
func diffHighlightKey(file diff.File) string {
	h := fnv.New64a()
	for _, line := range file.Rendered {
		_, _ = h.Write([]byte(line.Kind))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(strconv.Itoa(line.HunkIndex)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(line.Text))
		_, _ = h.Write([]byte{0})
	}
	return file.Path + "\x00" + strconv.FormatUint(h.Sum64(), 16)
}

// highlightedDiffLines returns one syntax-highlighted code string per entry in
// file.Rendered. Entries for hunk/file header lines are always "". Entries for
// which no lexer was resolved, or for which chroma returned an unexpected line
// count, are also "". Callers MUST treat "" as "use the original line.Text" —
// it is not an error.
//
// Lexer state is reset at every hunk boundary. An unterminated string or open
// block comment in hunk N cannot bleed its token colour into hunk N+1, which
// is unrelated code that chroma would otherwise mis-classify.
//
// Files above diffHighlightMaxLines are returned unhighlighted; see that constant.
//
// The returned slice length always equals len(file.Rendered); this is a
// load-bearing invariant that buildDiffRows and renderDiffFile depend on.
func highlightedDiffLines(file diff.File) []string {
	if len(file.Rendered) == 0 {
		return []string{}
	}
	if len(file.Rendered) > diffHighlightMaxLines {
		// Cold-pass cost is roughly linear at ~200ms per 3000 rendered lines, and it
		// runs on the render that first shows the file — so a generated lockfile or
		// vendored blob would stall the paint. Falling back to flat rendering keeps
		// the diff readable instead of making the reader wait for colour.
		return make([]string, len(file.Rendered))
	}

	key := diffHighlightKey(file)
	diffHighlightMu.Lock()
	cached, ok := diffHighlightCache[key]
	diffHighlightMu.Unlock()
	if ok {
		return cached
	}

	result := computeHighlightedDiffLines(file)

	diffHighlightMu.Lock()
	diffHighlightCache[key] = result
	diffHighlightMu.Unlock()

	return result
}

// computeHighlightedDiffLines is the cache-miss implementation of
// highlightedDiffLines. Call only when the cache does not have an entry.
func computeHighlightedDiffLines(file diff.File) []string {
	result := make([]string, len(file.Rendered))

	// Resolve lexer from the file path. Fall back to content analysis on a
	// small sample from the first content lines when the path alone is
	// insufficient.
	lex := lexers.Match(file.Path)
	if lex == nil {
		sample := diffSample(file.Rendered, 20)
		if sample != "" {
			lex = lexers.Analyse(sample)
		}
	}
	if lex == nil {
		return result // highlighting off; all ""
	}
	lex = chroma.Coalesce(lex)

	sty := styles.Get("monokai")
	if sty == nil {
		sty = styles.Fallback
	}

	// Walk file.Rendered grouped by HunkIndex. File-header lines (HunkIndex<0)
	// are left as "". Each hunk is processed independently with a fresh lexer
	// state so that unclosed constructs in one hunk cannot bleed into the next.
	i := 0
	n := len(file.Rendered)
	for i < n {
		line := file.Rendered[i]
		if line.HunkIndex < 0 {
			i++
			continue
		}
		hunkIdx := line.HunkIndex
		start := i
		for i < n && file.Rendered[i].HunkIndex == hunkIdx {
			i++
		}
		highlightHunk(file.Rendered[start:i], lex, sty, result)
	}
	return result
}

// highlightHunk tokenises the old side and new side of one hunk with a fresh
// lexer state for each side, then writes highlighted strings into result at
// each content line's RenderIndex.
//
// Context lines appear on both sides; we use the new-side highlighted string
// for them since it is always available.
func highlightHunk(lines []diff.RenderedLine, lex chroma.Lexer, sty *chroma.Style, result []string) {
	var oldLines, newLines []string
	for _, l := range lines {
		switch l.Kind {
		case diff.LineKindContext:
			oldLines = append(oldLines, l.Text)
			newLines = append(newLines, l.Text)
		case diff.LineKindDel:
			oldLines = append(oldLines, l.Text)
		case diff.LineKindAdd:
			newLines = append(newLines, l.Text)
		}
	}

	hlOld := highlightSourceLines(lex, sty, oldLines)
	hlNew := highlightSourceLines(lex, sty, newLines)

	oldIdx := 0
	newIdx := 0
	for _, l := range lines {
		switch l.Kind {
		case diff.LineKindContext:
			if newIdx < len(hlNew) {
				result[l.RenderIndex] = hlNew[newIdx]
			}
			oldIdx++
			newIdx++
		case diff.LineKindAdd:
			if newIdx < len(hlNew) {
				result[l.RenderIndex] = hlNew[newIdx]
			}
			newIdx++
		case diff.LineKindDel:
			if oldIdx < len(hlOld) {
				result[l.RenderIndex] = hlOld[oldIdx]
			}
			oldIdx++
			// LineKindHunkHeader: leave "" (already zero value in result).
		}
	}
}

// highlightSourceLines tokenises src (the given lines joined by "\n") with a
// fresh lexer state and returns one highlighted string per input line. Returns
// nil on any chroma error or if the formatted output produces a different line
// count than the input (e.g. some lexers inject extra blank lines). Callers
// treat nil the same as a slice of empty strings.
func highlightSourceLines(lex chroma.Lexer, sty *chroma.Style, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	src := strings.Join(lines, "\n")

	var buf bytes.Buffer
	// Tokenise(nil, ...) always begins from fresh state (no carry-over from
	// prior calls with the same lexer object).
	it, err := lex.Tokenise(nil, src)
	if err != nil {
		return nil
	}
	f := formatters.Get("terminal256")
	if f == nil {
		return nil
	}
	if err := f.Format(&buf, sty, it); err != nil {
		return nil
	}

	// terminal256 may append a trailing "\x1b[0m\n" reset after the last
	// newline. TrimRight removes bare newlines; the loop below strips any
	// remaining trailing elements whose visible content is empty.
	raw := strings.TrimRight(buf.String(), "\n")
	split := strings.Split(raw, "\n")
	for len(split) > len(lines) {
		last := split[len(split)-1]
		if ansi.Strip(last) != "" {
			// Visible content — do not discard.
			break
		}
		split = split[:len(split)-1]
	}
	if len(split) != len(lines) {
		return nil
	}
	return split
}

// diffSample collects the first n content-line texts from a rendered file for
// use by lexers.Analyse when the file path alone does not resolve a lexer.
func diffSample(rendered []diff.RenderedLine, n int) string {
	var sb strings.Builder
	count := 0
	for _, l := range rendered {
		switch l.Kind {
		case diff.LineKindContext, diff.LineKindAdd, diff.LineKindDel:
			sb.WriteString(l.Text)
			sb.WriteByte('\n')
			count++
			if count >= n {
				return sb.String()
			}
		}
	}
	return sb.String()
}
