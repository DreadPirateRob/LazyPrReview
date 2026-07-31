package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

// PRTabCount is the number of PR list tabs.
const PRTabCount = 4

// PRTabName returns the display name for a PR filter tab.
func PRTabName(f forge.PRFilter) string {
	switch f {
	case forge.FilterReviewRequested:
		return "Review requested"
	case forge.FilterMine:
		return "Mine"
	case forge.FilterAllOpen:
		return "All open"
	default:
		return "Search"
	}
}

// RenderPRRow builds the two display lines for a PRSummary:
//
//	line 1: [state] #num title
//	line 2:   author | CI-glyph decision-glyph [labels…] updated | ⎇branch
//
// The lines are joined by a newline; PRListView prefixes the first line with
// the selection marker and the second is indented to align beneath it.
func RenderPRRow(pr domain.PRSummary) string {
	prefix := ""
	switch {
	case pr.State == "MERGED":
		prefix = "[merged] "
	case pr.State == "CLOSED":
		prefix = "[closed] "
	case pr.IsDraft:
		prefix = "[draft] "
	}
	title := fmt.Sprintf("%s#%d %s", prefix, pr.Number, pr.Title)
	meta := fmt.Sprintf("  ╰─ %s | %s%s%s %s | ⎇%s",
		pr.Author,
		ciGlyph(pr.StatusCheckState),
		decisionGlyph(pr.ReviewDecision),
		labelString(pr.Labels),
		timeAgo(pr.UpdatedAt),
		pr.HeadRefName,
	)
	return title + "\n" + meta
}

// FilterPRs filters a PR list by query using the configured mode.
// "fuzzy" uses case-insensitive subsequence matching; anything else is
// case-insensitive substring. Source order is preserved. An empty query
// returns the original slice without allocation.
func FilterPRs(prs []domain.PRSummary, query, mode string) []domain.PRSummary {
	if query == "" {
		return prs
	}
	q := strings.ToLower(query)
	var out []domain.PRSummary
	for _, pr := range prs {
		candidate := strings.ToLower(fmt.Sprintf("#%d %s %s %s", pr.Number, pr.Title, pr.Author, pr.HeadRefName))
		if matchesFilter(candidate, q, mode) {
			out = append(out, pr)
		}
	}
	return out
}

// selectedPRNumber returns the PR number under the cursor in the current
// visible list, or 0 when there is no selection.
func selectedPRNumber(m Model) int {
	vis := VisiblePRs(m)
	if m.PRPanel.Cursor >= 0 && m.PRPanel.Cursor < len(vis) {
		return vis[m.PRPanel.Cursor].Number
	}
	return 0
}

// indexOfPR returns the visible-list index of the given PR number, or 0 when
// absent — so a background refresh keeps the cursor on the same PR, while a
// tab switch (whose prior selection isn't in the new list) resets to the top.
func indexOfPR(m Model, number int) int {
	if number == 0 {
		return 0
	}
	for i, pr := range VisiblePRs(m) {
		if pr.Number == number {
			return i
		}
	}
	return 0
}

// VisiblePRs returns the PRs that are visible under the current filter.
func VisiblePRs(m Model) []domain.PRSummary {
	// The Search tab is already scoped server-side to the committed query;
	// don't re-filter it client-side (that would match only #num/title/author
	// and hide server matches on body/labels).
	if m.PRFilter == forge.FilterSearch {
		return m.PRs
	}
	return FilterPRs(m.PRs, m.PRPanel.Filter, m.Config.GUI.FilterMode)
}

// UpdatePRs handles keystrokes when the PR list is focused.
func UpdatePRs(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	ks, bound := m.resolveKey(msg)
	if !bound {
		return m, nil
	}

	visible := VisiblePRs(m)
	maxIdx := len(visible) - 1
	if maxIdx < 0 {
		maxIdx = 0
	}

	switch ks {
	case "k", "<up>":
		if m.PRPanel.Cursor > 0 {
			m.PRPanel.Cursor--
		}
	case "j", "<down>":
		if m.PRPanel.Cursor < maxIdx {
			m.PRPanel.Cursor++
		}
	case "[":
		if int(m.PRFilter) > 0 {
			m.PRFilter--
			return prsTabSwitch(m)
		}
	case "]":
		if int(m.PRFilter) < PRTabCount-1 {
			m.PRFilter++
			return prsTabSwitch(m)
		}
	case "/":
		m = startPanelFilter(m, &m.PRPanel)
	case "enter":
		if len(visible) > 0 && m.PRPanel.Cursor < len(visible) {
			pr := visible[m.PRPanel.Cursor]
			m.OpenedPRNumber = pr.Number
			m.Activity = "Loading PR detail…"
			m.LoadingDetail = true
			return m, fetchPRDetailCmd(m.Forge, m.Repo, pr.Number)
		}
	case ">":
		if maxIdx > 0 {
			m.PRPanel.Cursor = maxIdx
		}
	case "<":
		m.PRPanel.Cursor = 0
	}
	return m, nil
}

// prListTitle composes the PR panel's border title. Normally it shows all
// tabs with the active one bracketed; but while the query input is open, or on
// the Search tab, it leads with just the active tab + query so a long query is
// never truncated off the border by the full tab list (which already fills the
// side panel at narrow widths).
func prListTitle(m Model) string {
	active := PRTabName(m.PRFilter)
	suffix := filterSuffix(m, FocusPRs, m.PRPanel)
	if (m.FilterActive && m.CurrentFocus() == FocusPRs) || m.PRFilter == forge.FilterSearch {
		return active + suffix
	}
	tabs := []forge.PRFilter{forge.FilterReviewRequested, forge.FilterMine, forge.FilterAllOpen, forge.FilterSearch}
	names := make([]string, len(tabs))
	for i, f := range tabs {
		names[i] = PRTabName(f)
	}
	return tabsTitle(names, active) + suffix
}

// PRListView renders the PR list panel rows; tabs and filter state live in
// the border title (prListTitle).
func PRListView(m Model, height int) string {
	var sb strings.Builder
	visible := VisiblePRs(m)
	if len(visible) == 0 {
		if m.PRFilter == forge.FilterSearch {
			sb.WriteString("  (press / then type a query and Enter to search all PRs)\n")
		} else {
			sb.WriteString("  (no pull requests)\n")
		}
		return sb.String()
	}
	for i, pr := range visible {
		cursor := "  "
		if i == m.PRPanel.Cursor {
			cursor = "> "
		}
		sb.WriteString(cursor + RenderPRRow(pr) + "\n")
	}
	return sb.String()
}

// ── internal helpers ──────────────────────────────────────────────────────────

func ciGlyph(state string) string {
	switch strings.ToLower(state) {
	case "success":
		return "✓"
	case "failure", "error":
		return "✗"
	case "pending":
		return "◔"
	case "skipped":
		return "⊘"
	default:
		return "·"
	}
}

func decisionGlyph(decision string) string {
	switch decision {
	case "APPROVED":
		return "✓"
	case "CHANGES_REQUESTED":
		return "±"
	case "REVIEW_REQUIRED":
		return "○"
	default:
		return ""
	}
}

func labelString(labels []domain.Label) string {
	if len(labels) == 0 {
		return ""
	}
	var parts []string
	for _, l := range labels {
		parts = append(parts, "["+l.Name+"]")
	}
	return " " + strings.Join(parts, "")
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func matchesFilter(s, q, mode string) bool {
	switch mode {
	case "fuzzy":
		return fuzzyMatch(s, q)
	default:
		return strings.Contains(s, q)
	}
}

// fuzzyMatch reports whether every rune in q appears in s in order
// (case-insensitive subsequence match). Source order is preserved.
func fuzzyMatch(s, q string) bool {
	rs := []rune(s)
	rq := []rune(q)
	si := 0
	for _, r := range rq {
		found := false
		for si < len(rs) {
			sr := rs[si]
			si++
			if sr == r {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// fetchPRDetailCmd returns a Tea command that loads PR detail, commits, and diff via Forge.
func fetchPRDetailCmd(f forge.Forge, repo forge.Repo, number int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		detail, err := f.PRDetail(ctx, repo, number)
		if err != nil {
			return mutationFailedMsg{Action: "loadPRDetail", Err: err}
		}
		commits, err := f.Commits(ctx, repo, number)
		if err != nil {
			commits = nil
		}
		rawDiff, err := f.Diff(ctx, repo, number)
		if err != nil {
			return prDetailLoadedMsg{Detail: detail, Commits: commits, Diff: nil}
		}
		files, _ := diff.Parse(rawDiff)
		return prDetailLoadedMsg{Detail: detail, Commits: commits, Diff: files}
	}
}

// applyPRDetailData stores freshly-fetched PR detail + diff and rebuilds the
// derived anchors. It performs NO focus/mode changes —
// callers layer those (full open vs silent reconcile).
func applyPRDetailData(m *Model, detail domain.PRDetail, commits []domain.CommitSummary, files []diff.File) {
	m.PRDetail = &detail
	m.Commits = commits
	m.DiffFiles = files
	// Keep the cached pending-review id in sync with server truth so later
	// comments never attach to a discarded/adopted-away review.
	if detail.PendingReview != nil {
		m.PendingReviewID = detail.PendingReview.ID
	} else {
		m.PendingReviewID = ""
	}
	m.AnchoredThreads, m.UnresolvedThreadIndex = buildAnchors(m.PRDetail, m.DiffFiles)
}

// detailReconciledMsg carries a background refetch used to reconcile optimistic
// state with server truth without disturbing focus/mode/cursor. Err is set when
// the refetch failed so the handler can retry rather than strand drafts.
type detailReconciledMsg struct {
	Detail  domain.PRDetail
	Commits []domain.CommitSummary
	Diff    []diff.File
	Attempt int
	Err     error
}

// reconcileRetryMsg re-triggers a failed background reconcile after a delay.
type reconcileRetryMsg struct{ Attempt int }

// reconcileDetailCmd silently refetches PR detail for a background reconcile.
// A fetch error is reported (not swallowed) so confirmed optimistic drafts get
// reconciled by a retry instead of lingering forever.
func reconcileDetailCmd(f forge.Forge, repo forge.Repo, number, attempt int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		detail, err := f.PRDetail(ctx, repo, number)
		if err != nil {
			return detailReconciledMsg{Attempt: attempt, Err: err}
		}
		commits, _ := f.Commits(ctx, repo, number)
		rawDiff, err := f.Diff(ctx, repo, number)
		if err != nil {
			return detailReconciledMsg{Attempt: attempt, Err: err}
		}
		files, _ := diff.Parse(rawDiff)
		return detailReconciledMsg{Detail: detail, Commits: commits, Diff: files, Attempt: attempt}
	}
}

// prsTabSwitch resets per-tab state after a tab change and returns the command
// that loads the new tab's data. The three server-backed tabs refetch
// immediately; the Search tab clears its list and waits for a query.
func prsTabSwitch(m Model) (Model, tea.Cmd) {
	m.PRPanel.Cursor = 0
	m.PRPanel.Filter = ""
	if m.PRFilter == forge.FilterSearch {
		m.PRs = nil
		m.Activity = ""
		m.LoadingPRs = false
		return m, nil
	}
	// Serve fresh cache synchronously so rapid tab switching doesn't refetch.
	if prs, ok := m.cachedPRs(m.PRFilter); ok {
		m.PRs = prs
		m.Activity = ""
		m.LoadingPRs = false
		return m, nil
	}
	m.Activity = "Loading PRs…"
	m.LoadingPRs = true
	return m, fetchPRsCmd(m.Forge, m.Repo, m.PRFilter)
}

// sectionLoading reports whether the panel owning ctx is currently fetching
// its data (and therefore interaction-blocked). The PR-detail bundle backs
// Files, Threads, Checks, Main and the nested Thread context.
func (m Model) sectionLoading(ctx FocusContext) bool {
	switch ctx {
	case FocusPRs:
		return m.LoadingPRs
	case FocusFiles, FocusThreads, FocusChecks, FocusMain, FocusThread:
		return m.LoadingDetail
	}
	return false
}

// prCacheEntry is a TTL-stamped PR list slice.
type prCacheEntry struct {
	prs []domain.PRSummary
	at  time.Time
}

// prCacheTTL is how long a cached PR list stays fresh: one auto-refresh
// interval (config), min 60s. Manual refresh (R) bypasses the cache.
func prCacheTTL(m Model) time.Duration {
	if s := m.Config.GitHub.AutoRefreshInterval; s > 0 {
		return time.Duration(s) * time.Second
	}
	return 60 * time.Second
}

func (m Model) cachedPRs(filter forge.PRFilter) ([]domain.PRSummary, bool) {
	e, ok := m.PRCache[filter]
	if !ok || time.Since(e.at) >= prCacheTTL(m) {
		return nil, false
	}
	return e.prs, true
}

func (m Model) cachedSearch(query string) ([]domain.PRSummary, bool) {
	e, ok := m.SearchCache[query]
	if !ok || time.Since(e.at) >= prCacheTTL(m) {
		return nil, false
	}
	return e.prs, true
}

func storePRCache(m *Model, filter forge.PRFilter, prs []domain.PRSummary) {
	if m.PRCache == nil {
		m.PRCache = map[forge.PRFilter]prCacheEntry{}
	}
	m.PRCache[filter] = prCacheEntry{prs: prs, at: time.Now()}
}

func storeSearchCache(m *Model, query string, prs []domain.PRSummary) {
	if m.SearchCache == nil {
		m.SearchCache = map[string]prCacheEntry{}
	}
	m.SearchCache[query] = prCacheEntry{prs: prs, at: time.Now()}
}

// fetchPRsCmd loads the PR list for a filter tab (Review requested / Mine /
// All open) via Forge.ListPRs.
func fetchPRsCmd(f forge.Forge, repo forge.Repo, filter forge.PRFilter) tea.Cmd {
	return func() tea.Msg {
		prs, err := f.ListPRs(context.Background(), repo, filter)
		return prsLoadedMsg{Filter: filter, PRs: prs, Err: err}
	}
}

// searchPRsCmd runs a repo-wide, all-states PR search via Forge.SearchPRs.
func searchPRsCmd(f forge.Forge, repo forge.Repo, query string) tea.Cmd {
	return func() tea.Msg {
		prs, err := f.SearchPRs(context.Background(), repo, query)
		return prsLoadedMsg{Filter: forge.FilterSearch, Query: query, PRs: prs, Err: err}
	}
}

// prListRefreshCmd refetches the active PR-list tab, mirroring manual refresh:
// the committed search query on the Search tab, a plain list otherwise.
func prListRefreshCmd(m Model) tea.Cmd {
	if m.PRFilter == forge.FilterSearch {
		return searchPRsCmd(m.Forge, m.Repo, m.PRPanel.Filter)
	}
	return fetchPRsCmd(m.Forge, m.Repo, m.PRFilter)
}

// commitDiffCmd fetches and parses a single commit's diff for the Commits tab.
func commitDiffCmd(f forge.Forge, repo forge.Repo, sha, headline string) tea.Cmd {
	return func() tea.Msg {
		text, err := f.CommitDiff(context.Background(), repo, sha)
		if err != nil {
			return commitDiffLoadedMsg{SHA: sha, Headline: headline, Err: err}
		}
		files, _ := diff.Parse(text)
		return commitDiffLoadedMsg{SHA: sha, Headline: headline, Files: files}
	}
}

// resolveThreadCmd resolves or unresolves a review thread via the forge.
func resolveThreadCmd(f forge.Forge, threadID string, resolved bool) tea.Cmd {
	return func() tea.Msg {
		err := f.ResolveThread(context.Background(), threadID, resolved)
		return resolveResultMsg{ThreadID: threadID, Resolved: resolved, Err: err}
	}
}

// prsMsgActive reports whether a PR-list result matches the tab (and, for
// search, the committed query) the user is currently looking at.
func prsMsgActive(m Model, msg prsLoadedMsg) bool {
	if msg.Filter != m.PRFilter {
		return false
	}
	if msg.Filter == forge.FilterSearch {
		return msg.Query == m.PRPanel.Filter
	}
	return true
}
