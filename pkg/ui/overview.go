package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

var (
	ovTitleStyle   = lipgloss.NewStyle().Bold(true)
	ovLabelStyle   = lipgloss.NewStyle().Faint(true)
	ovSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	ovRuleStyle    = lipgloss.NewStyle().Faint(true)
	ovAuthorStyle  = lipgloss.NewStyle().Bold(true)
	ovMetaStyle    = lipgloss.NewStyle().Faint(true)
	ovBotStyle     = lipgloss.NewStyle().Faint(true)

	ovOpenStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	ovClosedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	ovMergedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	ovDraftStyle  = lipgloss.NewStyle().Faint(true).Bold(true)
	ovWarnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
)

// timelineKey is the stable identity of a timeline item for fold state.
// domain.TimelineItem carries no server ID, so folding by row index would reset
// (or worse, transfer to a different comment) every time the overview is rebuilt —
// which happens on optimistic drafts, refetches, and resizes.
func timelineKey(it domain.TimelineItem) string {
	return fmt.Sprintf("tl|%s|%s|%d|%s", it.Kind, it.Author, it.SortAt.UnixNano(), it.URL)
}

// groupKey identifies a run of consecutive bot comments by its FIRST member, so
// appending a new verdict extends the run without resetting its fold state.
func groupKey(first domain.TimelineItem) string {
	return "grp|" + first.Author + "|" + timelineKey(first)
}

// normalizeAuthor is the lookup form for an author name. GitHub logins are
// case-insensitive, and the persisted override file stores keys the same way.
func normalizeAuthor(author string) string {
	return strings.ToLower(strings.TrimSpace(author))
}

// defaultBotAuthors are treated as machines out of the box. Only a starting point:
// no hardcoded list keeps up with every CI actor a repo installs, so the user's
// persisted overrides win over this.
var defaultBotAuthors = map[string]bool{
	"github-actions": true,
	"dependabot":     true,
	"codecov":        true,
	"renovate":       true,
	"sonarcloud":     true,
	"vercel":         true,
	"netlify":        true,
}

// isBotAuthor reports whether an author is a machine, honoring the user's flags
// first so they can both add an actor the defaults miss and demote one they want
// read as human. Falling back: GitHub suffixes App actors with "[bot]", then the
// built-in defaults.
func isBotAuthor(m Model, author string) bool {
	a := normalizeAuthor(author)
	if a == "" {
		return false
	}
	if role, ok := m.AuthorRoles[a]; ok {
		return role == config.RoleBot
	}
	if strings.HasSuffix(a, "[bot]") {
		return true
	}
	return defaultBotAuthors[a]
}

// authorIsFlagged reports whether the user has an explicit override for an author,
// so the UI can say "flagged" rather than implying it changed a default.
func authorIsFlagged(m Model, author string) bool {
	_, ok := m.AuthorRoles[normalizeAuthor(author)]
	return ok
}

// overviewExpanded reports whether a foldable overview row shows its body. Bot
// comments default collapsed (a CI-heavy PR is mostly repeated verdicts) and human
// comments default expanded; an explicit fold overrides either way.
func overviewExpanded(m Model, key string, bot bool) bool {
	if folded, ok := m.Folded[key]; ok {
		return !folded
	}
	return !bot
}

// verdictGlyph extracts a one-glyph gist from a bot body so a collapsed run still
// says what happened. Bodies commonly lead with a "Verdict:" line.
func verdictGlyph(body string) string {
	low := strings.ToLower(body)
	switch {
	case strings.Contains(low, "needs changes"), strings.Contains(low, "changes requested"):
		return ovWarnStyle.Render("▲ Needs Changes")
	case strings.Contains(low, "approved"):
		return ovOpenStyle.Render("✓ Approved")
	case strings.Contains(low, "failed"), strings.Contains(low, "failure"):
		return ovClosedStyle.Render("✗ Failed")
	}
	return ""
}

// stateBadge styles the PR state so it reads at a glance.
func stateBadge(d domain.PRDetail) string {
	if d.IsDraft {
		return ovDraftStyle.Render("◌ DRAFT")
	}
	switch strings.ToUpper(d.State) {
	case "OPEN":
		return ovOpenStyle.Render("● OPEN")
	case "MERGED":
		return ovMergedStyle.Render("✔ MERGED")
	case "CLOSED":
		return ovClosedStyle.Render("✗ CLOSED")
	}
	return ovLabelStyle.Render(orDash(d.State))
}

// reviewBadge styles the review decision with the SPEC §D2 glyph vocabulary.
func reviewBadge(d domain.PRDetail) string {
	switch strings.ToUpper(d.ReviewDecision) {
	case "APPROVED":
		return ovOpenStyle.Render("✓ APPROVED")
	case "CHANGES_REQUESTED":
		return ovClosedStyle.Render("± CHANGES REQUESTED")
	case "REVIEW_REQUIRED":
		return ovWarnStyle.Render("○ REVIEW REQUIRED")
	case "":
		return ""
	}
	return ovLabelStyle.Render(d.ReviewDecision)
}

// sectionRule renders a titled full-width rule, e.g. "── Description ───────".
func sectionRule(title string, width int) string {
	head := "── " + title + " "
	if width <= 0 {
		return ovSectionStyle.Render(head)
	}
	pad := width - lipgloss.Width(head)
	if pad < 0 {
		return ovSectionStyle.Render(head)
	}
	return ovSectionStyle.Render(head) + ovRuleStyle.Render(strings.Repeat("─", pad))
}

// shortTime formats a timeline timestamp compactly; the year is noise for recent
// activity but matters once a PR is old.
func shortTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.Year() == time.Now().Year() {
		return t.Format("Jan 02 15:04")
	}
	return t.Format("2006-01-02")
}

// packItems lays items across as many lines as needed, breaking only BETWEEN
// items and indenting continuations under the first item.
//
// Generic word wrapping is wrong for a list: ansi.Wordwrap breaks at hyphens (and
// overshoots the limit by a column doing it), so a label like "release-blocker"
// comes out as "release" / "-" / "blocker" — three fragments that read as
// different labels.
func packItems(prefix string, items []string, sep string, width int) []string {
	if len(items) == 0 {
		return nil
	}
	hang := lipgloss.Width(prefix)
	pad := strings.Repeat(" ", hang)
	budget := width - hang
	if width <= 0 || budget < 8 {
		return []string{prefix + strings.Join(items, sep)}
	}

	var out []string
	cur := ""
	flush := func() {
		if cur == "" {
			return
		}
		lead := prefix
		if len(out) > 0 {
			lead = pad
		}
		out = append(out, lead+cur)
		cur = ""
	}
	for _, it := range items {
		candidate := it
		if cur != "" {
			candidate = cur + sep + it
		}
		if lipgloss.Width(candidate) > budget && cur != "" {
			flush()
			candidate = it
		}
		cur = candidate
	}
	flush()
	return out
}

// buildOverview composes the PR overview into Main lines plus the parallel row
// model. It replaces the old flat PRDetail->[]string shape: width is needed to
// wrap prose and size rules, and the Model is needed for fold state, so neither
// can be re-derived at render time.
//
// Consecutive comments from the same bot collapse into one group row — a CI-heavy
// PR is otherwise dominated by repeated verdicts.
func buildOverview(m Model, d domain.PRDetail, width int) ([]string, []mainRow) {
	lines := make([]string, 0, 64)
	rows := make([]mainRow, 0, 64)
	add := func(s string, r mainRow) {
		lines = append(lines, s)
		rows = append(rows, r)
	}
	// Header rows are prose, so they WRAP rather than let the pane clip them. A PR
	// title is the most identifying text on screen; losing its tail to a `…` is
	// never the right trade when another row is free.
	meta := func(s string) {
		for _, l := range wrapToWidth(s, width) {
			add(l, mainRow{Kind: rowMeta, RenderIndex: -1})
		}
	}
	// metaHang wraps with a hanging indent so continuations line up under the text
	// rather than under the label that introduced it.
	metaHang := func(prefix, text string, style lipgloss.Style) {
		hang := lipgloss.Width(prefix)
		budget := width - hang
		if budget < 8 {
			// Too narrow for a hanging indent to help; wrap the whole thing flat.
			meta(style.Render(prefix + text))
			return
		}
		for i, l := range wrapToWidth(text, budget) {
			if i == 0 {
				add(style.Render(prefix+l), mainRow{Kind: rowMeta, RenderIndex: -1})
				continue
			}
			add(style.Render(strings.Repeat(" ", hang)+l), mainRow{Kind: rowMeta, RenderIndex: -1})
		}
	}
	blank := func() { add("", mainRow{Kind: rowMeta, RenderIndex: -1}) }

	// Header: four dense rows instead of one field per line. Empty fields are
	// dropped rather than printed as "—", which is pure noise.
	metaHang(fmt.Sprintf("#%d  ", d.Number), d.Title, ovTitleStyle)

	badges := []string{stateBadge(d)}
	if rb := reviewBadge(d); rb != "" {
		badges = append(badges, rb)
	}
	if d.Author != "" {
		badges = append(badges, ovAuthorStyle.Render(d.Author))
	}
	badges = append(badges, fmt.Sprintf("%s %s · %d files",
		diffAddStyle.Render(fmt.Sprintf("+%d", d.Additions)),
		diffDelStyle.Render(fmt.Sprintf("-%d", d.Deletions)),
		d.ChangedFiles))
	// The pending draft count is the most actionable thing in this header — it is
	// what tells you an unsubmitted review is waiting — so it survives the
	// compression that dropped the old "review:" text line.
	if d.PendingReviewCount > 0 {
		badges = append(badges, draftStyle.Render(fmt.Sprintf("✎ %d draft", d.PendingReviewCount)))
	}
	meta(strings.Join(badges, "   "))

	if d.HeadRefName != "" || d.BaseRefName != "" {
		meta(ovMetaStyle.Render(fmt.Sprintf("%s → %s", orDash(d.HeadRefName), orDash(d.BaseRefName))))
	}

	// These three are comma-separated LISTS, so they pack at item boundaries rather
	// than word-wrap: a hyphenated name like "release-blocker" must stay one token.
	// Each gets its own row only when non-empty, keeping the header dense.
	emitList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		for _, l := range packItems(ovLabelStyle.Render(label+" "), items, ", ", width) {
			add(l, mainRow{Kind: rowMeta, RenderIndex: -1})
		}
	}
	emitList("reviewers", reviewerList(d.RequestedReviewers))
	emitList("assignees", d.Assignees)
	emitList("labels", labelList(d.Labels))

	blank()
	add(sectionRule("Description", width), mainRow{Kind: rowSection, RenderIndex: -1})
	if body := renderMarkdownFor(d.Body, width, m.ViewerLogin); len(body) > 0 {
		for _, l := range body {
			meta(l)
		}
	} else {
		meta(ovMetaStyle.Render("(no description)"))
	}

	blank()
	humans, bots := 0, 0
	for _, it := range d.Timeline {
		if isBotAuthor(m, it.Author) {
			bots++
		} else {
			humans++
		}
	}
	title := fmt.Sprintf("Conversation · %d", len(d.Timeline))
	if bots > 0 && humans > 0 {
		title = fmt.Sprintf("Conversation · %d (%d human · %d bot)", len(d.Timeline), humans, bots)
	}
	add(sectionRule(title, width), mainRow{Kind: rowSection, RenderIndex: -1})
	if len(d.Timeline) == 0 {
		meta(ovMetaStyle.Render("(no comments yet)"))
		return lines, rows
	}

	for i := 0; i < len(d.Timeline); {
		it := d.Timeline[i]
		if !isBotAuthor(m, it.Author) {
			blank()
			appendEventRows(m, it, width, add)
			i++
			continue
		}
		// Gather the whole run of consecutive comments from this same bot.
		j := i
		for j < len(d.Timeline) && d.Timeline[j].Author == it.Author {
			j++
		}
		run := d.Timeline[i:j]
		blank()
		if len(run) == 1 {
			appendEventRows(m, run[0], width, add)
		} else {
			appendGroupRows(m, run, width, add)
		}
		i = j
	}

	// Review threads are part of the conversation, so the overview keeps listing
	// them even though they now also render inline in the diff. They fold by THREAD
	// ID — the same key the inline blocks use — so a thread folded in one place is
	// folded in both, rather than two independent fold states for one conversation.
	if len(d.Threads) > 0 {
		blank()
		add(sectionRule(fmt.Sprintf("Review threads · %d", len(d.Threads)), width),
			mainRow{Kind: rowSection, RenderIndex: -1})
		for _, th := range d.Threads {
			blank()
			appendOverviewThread(m, th, width, add)
		}
	}
	return lines, rows
}

// appendOverviewThread emits one review thread in the overview: a foldable summary
// row plus its comments when expanded.
func appendOverviewThread(m Model, th domain.Thread, width int, add func(string, mainRow)) {
	expanded := threadExpanded(m, th)
	caret := "▸"
	if expanded {
		caret = "▾"
	}
	loc := th.Path
	if th.Line != nil {
		loc = fmt.Sprintf("%s:%d", th.Path, *th.Line)
	}
	head := fmt.Sprintf("%s %s %s", caret,
		ovAuthorStyle.Render(orDash(loc)),
		ovMetaStyle.Render(threadStatus(th)))
	if threadHasDraft(th) {
		head += draftStyle.Render(" [draft]")
	}
	if threadMentionsViewer(th, m.ViewerLogin) {
		head += " " + mentionYouStyle.Render("@you")
	}
	add(head, mainRow{Kind: rowEventHeader, RenderIndex: -1, ThreadID: th.ID, Key: th.ID})
	if !expanded {
		return
	}
	// One row per comment, each carrying its OWN author: a thread is a conversation
	// between different people, so flagging must act on the comment under the cursor.
	// The summary row above deliberately has no Author — it shows a file location, so
	// there is nobody to flag there.
	for _, c := range th.Comments {
		row := mainRow{Kind: rowEventBody, RenderIndex: -1, ThreadID: th.ID, Key: th.ID, Author: c.Author}
		who := ovAuthorStyle.Render(orDash(c.Author))
		if c.State == "PENDING" {
			who += draftStyle.Render(" [draft]")
		}
		add(threadGutter+who+":", row)
		for _, l := range renderMarkdownFor(c.Body, width-4, m.ViewerLogin) {
			add(threadGutter+"  "+l, row)
		}
	}
}

// appendEventRows emits one timeline comment: a summary row (the fold target) and,
// when expanded, its markdown-rendered body under a gutter.
func appendEventRows(m Model, it domain.TimelineItem, width int, add func(string, mainRow)) {
	key := timelineKey(it)
	bot := isBotAuthor(m, it.Author)
	expanded := overviewExpanded(m, key, bot)

	caret := "▸"
	if expanded {
		caret = "▾"
	}
	author := ovAuthorStyle.Render(orDash(it.Author))
	if bot {
		author = ovBotStyle.Render(orDash(it.Author))
	}
	head := fmt.Sprintf("%s %s %s", caret, author,
		ovMetaStyle.Render(timelineVerb(it)+" · "+shortTime(it.SortAt)))
	if g := verdictGlyph(it.Body); g != "" && !expanded {
		head += "  " + g
	}
	add(head, mainRow{Kind: rowEventHeader, RenderIndex: -1, Key: key, Author: it.Author})
	if !expanded {
		return
	}
	row := mainRow{Kind: rowEventBody, RenderIndex: -1, Key: key, Author: it.Author}
	for _, l := range renderMarkdownFor(it.Body, width-2, m.ViewerLogin) {
		add(threadGutter+l, row)
	}
}

// appendGroupRows emits a run of consecutive bot comments as ONE foldable row.
// Collapsed it reports the count, the latest verdict, and the date span; expanded
// it emits each member as its own event block so they stay individually foldable.
func appendGroupRows(m Model, run []domain.TimelineItem, width int, add func(string, mainRow)) {
	key := groupKey(run[0])
	expanded := overviewExpanded(m, key, true)

	caret := "▸"
	if expanded {
		caret = "▾"
	}
	span := shortTime(run[0].SortAt)
	if last := shortTime(run[len(run)-1].SortAt); last != span {
		span += " – " + last
	}
	head := fmt.Sprintf("%s %s %s", caret,
		ovBotStyle.Render(run[0].Author),
		ovMetaStyle.Render(fmt.Sprintf("%d comments · %s", len(run), span)))
	// The latest verdict is the useful gist of a collapsed run.
	for i := len(run) - 1; i >= 0; i-- {
		if g := verdictGlyph(run[i].Body); g != "" {
			head += "  " + ovMetaStyle.Render("latest") + " " + g
			break
		}
	}
	add(head, mainRow{Kind: rowGroupHeader, RenderIndex: -1, Key: key, Author: run[0].Author})
	if !expanded {
		return
	}
	for _, it := range run {
		appendEventRows(m, it, width, add)
	}
}

// mainContentWidth is the column budget for one Main content line: the pane width
// less its border (2) and less the cursor prefix MainView writes on every row (2).
// Getting this wrong is invisible in tests and wraps two columns late in the real
// UI, so it is derived here once rather than at each call site.
func mainContentWidth(m Model) int {
	w := ComputeLayout(m).MainWidth - 4
	if w < 0 {
		return 0
	}
	return w
}

// setMainOverview points Main at the PR overview and owns its row model. It is the
// single entry point for every overview render, so no caller holding only a
// PRDetail can silently skip the row model or the fold state.
func setMainOverview(m Model, d domain.PRDetail) Model {
	lines, rows := buildOverview(m, d, mainContentWidth(m))
	m = setMainLines(m, lines)
	m.MainRows = rows // after setMainLines, which clears it
	m.MainMode = MainOverview
	return m
}

// authorAtCursor returns the comment author of the row under the cursor, or "".
// Read straight off the row model: grouping, folding and markdown all reshape the
// rendered text, so parsing it back would be brittle. Rows with nobody to flag
// (section rules, the PR header, a thread's location summary) carry no Author.
func authorAtCursor(m Model) string {
	r, ok := rowAt(m, m.MainCursor)
	if !ok {
		return ""
	}
	return r.Author
}

// toggleAuthorRole flips the author under the cursor between bot and human and
// persists the choice.
//
// The flip is applied optimistically and the overview rebuilt so the row regroups
// immediately (flagging a bot collapses its run on the spot); the write runs in a
// command and only surfaces on failure, since blocking the UI on disk I/O would be
// worse than a late error.
func toggleAuthorRole(m Model) (Model, tea.Cmd) {
	author := authorAtCursor(m)
	if author == "" || m.PRDetail == nil {
		return m, nil
	}
	key := normalizeAuthor(author)

	next := make(map[string]config.AuthorRole, len(m.AuthorRoles)+1)
	for k, v := range m.AuthorRoles {
		next[k] = v
	}
	// Record the opposite of what is IN EFFECT now, so the first press always
	// visibly flips no matter which default was applying.
	if isBotAuthor(m, author) {
		next[key] = config.RoleHuman
		m.Toast = NewToast(author + " → human")
	} else {
		next[key] = config.RoleBot
		m.Toast = NewToast(author + " → bot")
	}
	m.AuthorRoles = next

	d := *m.PRDetail
	d.Threads = effectiveThreads(m)
	m = setMainOverview(m, d)
	// Grouping just changed, so restore the cursor to whichever row now represents
	// that author rather than to a stale index.
	for i, r := range m.MainRows {
		if normalizeAuthor(r.Author) == key {
			m.MainCursor = i
			break
		}
	}
	if m.MainCursor > len(m.MainLines)-1 {
		m.MainCursor = maxInt(0, len(m.MainLines)-1)
	}
	return m, saveAuthorRolesCmd(next)
}

func saveAuthorRolesCmd(roles map[string]config.AuthorRole) tea.Cmd {
	return func() tea.Msg {
		if err := config.SaveAuthors(roles); err != nil {
			return authorRolesSavedMsg{Err: err}
		}
		return authorRolesSavedMsg{}
	}
}
