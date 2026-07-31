package ui

import (
	"regexp"
	"sort"
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// mentionRE matches an @handle. GitHub logins are alphanumeric with single
// hyphens, and a team mention carries an org prefix (@org/team), so the slashed
// form is matched too.
//
// RE2 has no lookbehind, so the left boundary is enforced separately in
// mentionSpans; matching this pattern alone would fire inside name@example.com.
var mentionRE = regexp.MustCompile(`@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:/[A-Za-z0-9][A-Za-z0-9-]*)?`)

var (
	mentionStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true) // any @handle
	mentionYouStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("5")).Bold(true)
)

// mentionSpans returns the byte ranges of real @mentions: those at the start of
// the text or preceded by a non-word character. Without that check an email
// address (name@example.com) or a scale suffix (logo@2x.png) reads as a mention,
// which would both mis-highlight and, worse, make m / M navigate to threads that
// never addressed anyone.
func mentionSpans(s string) [][]int {
	locs := mentionRE.FindAllStringIndex(s, -1)
	out := make([][]int, 0, len(locs))
	for _, loc := range locs {
		if loc[0] > 0 {
			switch prev := s[loc[0]-1]; {
			case prev >= 'a' && prev <= 'z', prev >= 'A' && prev <= 'Z',
				prev >= '0' && prev <= '9', prev == '_':
				continue
			}
		}
		out = append(out, loc)
	}
	return out
}

// highlightMentions styles every @handle in a plain-text line, giving the viewer's
// own handle a stronger treatment so "someone is talking to me" is visible without
// reading the words. Input must be unstyled: it is applied to comment bodies before
// any other styling, so there are no escape sequences to corrupt.
func highlightMentions(line, viewerLogin string) string {
	spans := mentionSpans(line)
	if len(spans) == 0 {
		return line
	}
	var sb strings.Builder
	prev := 0
	for _, loc := range spans {
		sb.WriteString(line[prev:loc[0]])
		at := line[loc[0]:loc[1]]
		if viewerLogin != "" && strings.EqualFold(at, "@"+viewerLogin) {
			sb.WriteString(mentionYouStyle.Render(at))
		} else {
			sb.WriteString(mentionStyle.Render(at))
		}
		prev = loc[1]
	}
	sb.WriteString(line[prev:])
	return sb.String()
}

// threadMentionsViewer reports whether any comment in the thread @-mentions the
// viewer. Case-insensitive, since GitHub logins are.
func threadMentionsViewer(th domain.Thread, viewerLogin string) bool {
	if viewerLogin == "" {
		return false
	}
	want := "@" + viewerLogin
	for _, c := range th.Comments {
		for _, loc := range mentionSpans(c.Body) {
			if strings.EqualFold(c.Body[loc[0]:loc[1]], want) {
				return true
			}
		}
	}
	return false
}

// mentionThreads returns the threads that mention the viewer, ordered by file then
// position so m / M cycle them in reading order — the same ordering rule
// BuildUnresolvedIndex applies to t / T.
//
// Unanchored threads (file-level, outdated) are included: they render in the
// file-header block and jumpToAnchoredThread locates them by id, so mention
// navigation means "threads addressing me", not "code-line threads addressing me".
func mentionThreads(m Model) []diff.AnchoredThread {
	if m.ViewerLogin == "" {
		return nil
	}
	fileOrder := make(map[string]int, len(m.DiffFiles))
	for i, f := range m.DiffFiles {
		fileOrder[f.Path] = i
	}
	out := make([]diff.AnchoredThread, 0, 4)
	for _, at := range m.AnchoredThreads {
		if threadMentionsViewer(at.Thread, m.ViewerLogin) {
			out = append(out, at)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := fileOrder[out[i].FilePath], fileOrder[out[j].FilePath]
		if li != lj {
			return li < lj
		}
		return out[i].RenderIndex < out[j].RenderIndex
	})
	return out
}

// cycleMentionJump moves to the next/previous thread mentioning the viewer,
// wrapping. Mirrors cycleThreadJump so the two navigations behave identically.
func cycleMentionJump(m Model, delta int) Model {
	index := mentionThreads(m)
	if len(index) == 0 {
		return m
	}
	current := 0
	if id := threadIDAtMainCursor(m); id != "" {
		for i, at := range index {
			if at.Thread.ID == id {
				current = i
				break
			}
		}
	}
	next := (current + delta + len(index)) % len(index)
	return jumpToAnchoredThread(m, index[next], false)
}
