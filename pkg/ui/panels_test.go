package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

// ── PR list tests ─────────────────────────────────────────────────────────────

func TestPRListTabNames(t *testing.T) {
	cases := []struct {
		f    forge.PRFilter
		want string
	}{
		{forge.FilterReviewRequested, "Review requested"},
		{forge.FilterMine, "Mine"},
		{forge.FilterAllOpen, "All open"},
	}
	for _, c := range cases {
		if got := PRTabName(c.f); got != c.want {
			t.Errorf("PRTabName(%d) = %q; want %q", c.f, got, c.want)
		}
	}
}

func TestPRListRowContainsRequiredFields(t *testing.T) {
	pr := domain.PRSummary{
		Number:           42,
		Title:            "Fix auth crash",
		Author:           "alice",
		HeadRefName:      "fix/auth",
		BaseRefName:      "main",
		ReviewDecision:   "APPROVED",
		StatusCheckState: "success",
		UpdatedAt:        time.Now().Add(-2 * time.Hour),
		Labels:           []domain.Label{{Name: "bug"}, {Name: "p1"}},
	}
	row := RenderPRRow(pr)

	checks := []string{"#42", "Fix auth crash", "alice", "fix/auth", "✓", "[bug]", "[p1]"}
	for _, want := range checks {
		if !strings.Contains(row, want) {
			t.Errorf("RenderPRRow: row %q missing %q", row, want)
		}
	}
}

func TestPRListRowDraftPrefix(t *testing.T) {
	pr := domain.PRSummary{Number: 99, Title: "WIP", Author: "bob", IsDraft: true}
	row := RenderPRRow(pr)
	if !strings.HasPrefix(row, "[draft]") {
		t.Errorf("draft PR row should start with [draft], got %q", row)
	}
}

func TestPRListRowCIGlyphs(t *testing.T) {
	cases := []struct {
		state string
		glyph string
	}{
		{"success", "✓"},
		{"failure", "✗"},
		{"pending", "◔"},
		{"skipped", "⊘"},
		{"", ""},
	}
	for _, c := range cases {
		pr := domain.PRSummary{Number: 1, StatusCheckState: c.state}
		row := RenderPRRow(pr)
		if c.glyph != "" && !strings.Contains(row, c.glyph) {
			t.Errorf("CI state %q: row %q missing glyph %q", c.state, row, c.glyph)
		}
	}
}

func TestPRListRowDecisionGlyphs(t *testing.T) {
	cases := []struct {
		decision string
		glyph    string
	}{
		{"APPROVED", "✓"},
		{"CHANGES_REQUESTED", "±"},
		{"REVIEW_REQUIRED", "○"},
		{"", ""},
	}
	for _, c := range cases {
		pr := domain.PRSummary{Number: 1, ReviewDecision: c.decision}
		row := RenderPRRow(pr)
		if c.glyph != "" && !strings.Contains(row, c.glyph) {
			t.Errorf("decision %q: row %q missing glyph %q", c.decision, row, c.glyph)
		}
	}
}

// ── PR filter tests ───────────────────────────────────────────────────────────

func TestPRListFilterEmptyQueryReturnsAll(t *testing.T) {
	prs := samplePRs()
	got := FilterPRs(prs, "", "substring")
	if len(got) != len(prs) {
		t.Errorf("empty query should return all %d PRs, got %d", len(prs), len(got))
	}
	// Must be exact same slice (no allocation)
	if len(prs) > 0 && &got[0] != &prs[0] {
		t.Error("empty query must return original slice without allocation")
	}
}

func TestPRListFilterSubstring(t *testing.T) {
	prs := []domain.PRSummary{
		{Number: 1, Title: "Fix authentication bug", Author: "alice"},
		{Number: 2, Title: "Update readme", Author: "bob"},
		{Number: 3, Title: "Auth middleware refactor", Author: "alice"},
	}
	got := FilterPRs(prs, "auth", "substring")
	if len(got) != 2 {
		t.Fatalf("substring 'auth': expected 2 matches, got %d", len(got))
	}
	if got[0].Number != 1 || got[1].Number != 3 {
		t.Errorf("wrong PRs matched: %v", got)
	}
}

func TestPRListFilterSubstringCaseInsensitive(t *testing.T) {
	prs := []domain.PRSummary{
		{Number: 1, Title: "Fix Authentication", Author: "alice"},
		{Number: 2, Title: "update readme", Author: "bob"},
	}
	got := FilterPRs(prs, "AUTHENTICATION", "substring")
	if len(got) != 1 || got[0].Number != 1 {
		t.Errorf("case-insensitive substring failed: got %v", got)
	}
}

func TestPRListFilterFuzzy(t *testing.T) {
	prs := []domain.PRSummary{
		{Number: 1, Title: "Fix authentication bug", Author: "alice"},
		{Number: 2, Title: "Update readme", Author: "bob"},
		{Number: 3, Title: "add unit tests for auth module", Author: "carol"},
	}
	// "atm" should match "authentication" (a..t..m) and "add unit tests for auth module" (a..t..m)
	got := FilterPRs(prs, "atm", "fuzzy")
	if len(got) < 1 {
		t.Fatalf("fuzzy 'atm': expected at least 1 match, got %d", len(got))
	}
}

func TestPRListFilterFuzzyPreservesOrder(t *testing.T) {
	prs := []domain.PRSummary{
		{Number: 3, Title: "third abc", Author: "x"},
		{Number: 1, Title: "first ac", Author: "x"},
		{Number: 2, Title: "second abc match", Author: "x"},
	}
	got := FilterPRs(prs, "ac", "fuzzy")
	// All match; order must be preserved (3, 1, 2)
	if len(got) != 3 {
		t.Fatalf("all should match, got %d", len(got))
	}
	for i, want := range []int{3, 1, 2} {
		if got[i].Number != want {
			t.Errorf("position %d: want PR#%d, got PR#%d", i, want, got[i].Number)
		}
	}
}

func TestPRListFilterNoMatch(t *testing.T) {
	prs := samplePRs()
	got := FilterPRs(prs, "zzznomatch", "substring")
	if len(got) != 0 {
		t.Errorf("expected 0 matches, got %d", len(got))
	}
}

func TestPRListTitleShowsTabs(t *testing.T) {
	m := newPRListModel()
	title := prListTitle(m)
	for _, tab := range []string{"Review requested", "Mine", "All open"} {
		if !strings.Contains(title, tab) {
			t.Errorf("PR panel title missing tab %q: %q", tab, title)
		}
	}
}

func TestPRListTitleActiveTabBracketed(t *testing.T) {
	m := newPRListModel()
	m.PRFilter = forge.FilterMine
	title := prListTitle(m)
	if !strings.Contains(title, "[Mine]") {
		t.Errorf("active tab should be bracketed, title: %q", title)
	}
	// Other tabs should not be bracketed
	if strings.Contains(title, "[Review requested]") {
		t.Errorf("inactive tab should not be bracketed")
	}
}

func TestPRListTitleShowsFilterPrompt(t *testing.T) {
	m := newPRListModel()
	m.FilterActive = true
	m.PRPanel.Filter = "fix"
	title := prListTitle(m)
	if !strings.Contains(title, "/fix█") {
		t.Errorf("live filter query not shown in title: %q", title)
	}
	m.FilterActive = false
	title = prListTitle(m)
	if !strings.Contains(title, "/fix") || strings.Contains(title, "█") {
		t.Errorf("committed filter should show without input cursor: %q", title)
	}
}

func TestPRListViewCursorMarker(t *testing.T) {
	m := newPRListModel()
	view := PRListView(m, 20)
	if !strings.Contains(view, "> ") {
		t.Errorf("cursor marker '> ' not found in view: %q", view)
	}
}

func TestPRListEnterFetchesDetail(t *testing.T) {
	f := fakeforge.New()
	f.Repo = forge.Repo{Owner: "myorg", Name: "myrepo"}
	f.PRDetails[2] = domain.PRDetail{ID: "PR2", Number: 2, Title: "Update readme"}
	f.DiffTexts[2] = panelSampleDiff
	m := newPRListModel()
	m.Forge = f
	m.PRPanel.Cursor = 1
	updated, cmd := UpdatePRs(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to trigger detail fetch command")
	}
	if updated.OpenedPRNumber != 2 {
		t.Fatalf("expected opened PR number 2, got %d", updated.OpenedPRNumber)
	}
}

func TestPRListEnterSetsLoadingActivity(t *testing.T) {
	f := fakeforge.New()
	f.Repo = forge.Repo{Owner: "myorg", Name: "myrepo"}
	f.PRDetails[2] = domain.PRDetail{ID: "PR2", Number: 2, Title: "Update readme"}
	f.DiffTexts[2] = panelSampleDiff
	m := newPRListModel()
	m.Forge = f
	m.PRPanel.Cursor = 1
	updated, cmd := UpdatePRs(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to trigger detail fetch command")
	}
	if updated.Activity != "Loading PR detail…" {
		t.Fatalf("expected loading activity, got %q", updated.Activity)
	}
}

func TestPRFilterClosesOnEscAndEnter(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{name: "esc", msg: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "enter", msg: tea.KeyPressMsg{Code: tea.KeyEnter}},
	}
	for _, tc := range cases {
		m := newPRListModel()
		m.FilterActive = true
		m.PRPanel.Filter = "auth"
		m.PRPanel.Cursor = 2
		updated, _ := m.Update(tc.msg)
		got := updated.(Model)
		if got.FilterActive {
			t.Fatalf("%s should close filter", tc.name)
		}
		if got.PRPanel.Cursor != 0 {
			t.Fatalf("%s should reset cursor, got %d", tc.name, got.PRPanel.Cursor)
		}
	}
}

func TestPRSelectionPopulatesDetailPanels(t *testing.T) {
	f := fakeforge.New()
	f.Repo = forge.Repo{Owner: "myorg", Name: "myrepo"}
	f.PRDetails[2] = domain.PRDetail{
		ID:     "PR2",
		Number: 2,
		Title:  "Update readme",
		Files:  []domain.ChangedFile{{Path: "foo.txt", ViewerViewedState: "UNVIEWED"}},
		Threads: []domain.Thread{
			{ID: "t1", Path: "foo.txt", DiffSide: "RIGHT", Line: intPtr(3), Comments: []domain.Comment{{Body: "thread"}}},
		},
		Checks: []domain.Check{{Kind: "checkRun", Name: "CI", Status: "completed", Conclusion: "success"}},
	}
	f.DiffTexts[2] = panelSampleDiff
	m := newPRListModel()
	m.Forge = f
	m.FocusStack = []FocusContext{FocusPRs}
	m.PRPanel.Cursor = 1

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to trigger detail fetch")
	}
	msg := cmd()
	updated, _ = updated.(Model).Update(msg)
	got := updated.(Model)

	if got.PRDetail == nil || got.PRDetail.Number != 2 {
		t.Fatalf("expected PR detail for #2 to load, got %#v", got.PRDetail)
	}
	if got.CurrentFocus() != FocusMain {
		t.Fatalf("expected focus to land on main overview, got %s", got.CurrentFocus())
	}
	if len(visibleFileRows(got)) == 0 {
		t.Fatal("expected files panel rows to populate")
	}
	if len(got.AnchoredThreads) == 0 {
		t.Fatal("expected anchored threads to populate")
	}
	view := fmt.Sprint(got.View())
	for _, section := range []string{"Files", "[Unresolved]", "Checks"} {
		if !strings.Contains(view, section) {
			t.Fatalf("expected %s section in populated view", section)
		}
	}
}

func TestPRSelectionClearsLoadingActivityAfterDetail(t *testing.T) {
	f := fakeforge.New()
	f.Repo = forge.Repo{Owner: "myorg", Name: "myrepo"}
	f.PRDetails[2] = domain.PRDetail{ID: "PR2", Number: 2, Title: "Update readme", Files: []domain.ChangedFile{{Path: "foo.txt"}}}
	f.DiffTexts[2] = panelSampleDiff
	m := newPRListModel()
	m.Forge = f
	m.FocusStack = []FocusContext{FocusPRs}
	m.PRPanel.Cursor = 1
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.(Model).Activity != "Loading PR detail…" {
		t.Fatalf("expected loading activity before fetch resolves")
	}
	msg := cmd()
	updated, _ = updated.(Model).Update(msg)
	if updated.(Model).Activity != "" {
		t.Fatalf("expected loading activity to clear after detail load, got %q", updated.(Model).Activity)
	}
}

// ── Status panel tests ────────────────────────────────────────────────────────

func TestStatusPanelShowsRepo(t *testing.T) {
	m := newStatusModel()
	view := StatusView(m)
	if !strings.Contains(view, "myorg/myrepo") {
		t.Errorf("StatusView missing repo, got: %q", view)
	}
}

func TestStatusPanelShowsViewer(t *testing.T) {
	m := newStatusModel()
	view := StatusView(m)
	if !strings.Contains(view, "testuser") {
		t.Errorf("StatusView missing viewer login, got: %q", view)
	}
}

func TestStatusPanelShowsReviewDecision(t *testing.T) {
	m := newStatusModel()
	detail := samplePRDetail()
	detail.ReviewDecision = "APPROVED"
	m.PRDetail = &detail
	view := StatusView(m)
	if !strings.Contains(view, "APPROVED") {
		t.Errorf("StatusView missing review decision, got: %q", view)
	}
}

func TestStatusPanelShowsRequestedReviewers(t *testing.T) {
	m := newStatusModel()
	detail := samplePRDetail()
	detail.RequestedReviewers = []domain.RequestedReviewer{
		{Kind: "user", Name: "alice"},
		{Kind: "team", Name: "security"},
	}
	m.PRDetail = &detail
	view := StatusView(m)
	if !strings.Contains(view, "alice") {
		t.Errorf("StatusView missing reviewer alice, got: %q", view)
	}
	if !strings.Contains(view, "@security") {
		t.Errorf("StatusView missing team reviewer @security, got: %q", view)
	}
}

func TestStatusPanelShowsRateLimit(t *testing.T) {
	m := newStatusModel()
	detail := samplePRDetail()
	detail.RateLimitRemaining = 4500
	m.PRDetail = &detail
	view := StatusView(m)
	if !strings.Contains(view, "4500") {
		t.Errorf("StatusView missing rate limit, got: %q", view)
	}
}

func TestStatusBannerPendingReviewShown(t *testing.T) {
	m := newStatusModel()
	detail := samplePRDetail()
	detail.PendingReviewCount = 3
	m.PRDetail = &detail
	view := StatusView(m)
	if !strings.Contains(view, "PENDING review") {
		t.Errorf("StatusView missing pending review banner, got: %q", view)
	}
	if !strings.Contains(view, "3") {
		t.Errorf("StatusView banner missing count, got: %q", view)
	}
}

func TestStatusBannerHiddenWhenNoPending(t *testing.T) {
	m := newStatusModel()
	detail := samplePRDetail()
	detail.PendingReviewCount = 0
	m.PRDetail = &detail
	view := StatusView(m)
	if strings.Contains(view, "PENDING review") {
		t.Errorf("StatusView should not show pending banner when count=0, got: %q", view)
	}
}

func TestStatusScrollAdvancesAndClampsCursor(t *testing.T) {
	m := newStatusModel()
	detail := samplePRDetail()
	m.PRDetail = &detail

	// j advances the scroll cursor.
	m, _ = UpdateStatus(m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if m.Status.Cursor != 1 {
		t.Fatalf("expected status cursor to advance to 1, got %d", m.Status.Cursor)
	}
	// k moves it back and clamps at 0.
	m, _ = UpdateStatus(m, tea.KeyPressMsg{Text: "k", Code: 'k'})
	m, _ = UpdateStatus(m, tea.KeyPressMsg{Text: "k", Code: 'k'})
	if m.Status.Cursor != 0 {
		t.Fatalf("expected status cursor to clamp at 0, got %d", m.Status.Cursor)
	}
}

func TestStatusNoPRDetail(t *testing.T) {
	m := newStatusModel()
	m.PRDetail = nil
	view := StatusView(m)
	// Should still show repo and viewer without crashing
	if !strings.Contains(view, "myorg/myrepo") {
		t.Errorf("StatusView without PRDetail missing repo, got: %q", view)
	}
}

func TestMainViewportScrollsWithCursor(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.ScreenMode = ScreenFullscreen
	m.Width = 40
	m.Height = 12
	m.PRDetail = &domain.PRDetail{Title: "t"}
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	m.MainLines = lines

	m.MainCursor = 0
	top := fmt.Sprint(m.View())
	if !strings.Contains(top, "line-00") {
		t.Fatalf("expected first line visible at top, got:\n%s", top)
	}
	if strings.Contains(top, "line-59") {
		t.Fatalf("did not expect last line visible at top")
	}
	if n := len(strings.Split(strings.TrimRight(top, "\n"), "\n")); n > m.Height {
		t.Fatalf("main viewport overflow: %d lines > height %d", n, m.Height)
	}

	m.MainCursor = 59
	bottom := fmt.Sprint(m.View())
	if !strings.Contains(bottom, "line-59") {
		t.Fatalf("expected last line visible after scrolling to bottom, got:\n%s", bottom)
	}
	if strings.Contains(bottom, "line-00") {
		t.Fatalf("did not expect first line visible after scrolling to bottom")
	}
}

func TestPROverviewShowsMetadataAndConversation(t *testing.T) {
	line := 12
	d := domain.PRDetail{
		Number:         7,
		Title:          "Add retries",
		State:          "OPEN",
		Author:         "alice",
		HeadRefName:    "feat/retries",
		BaseRefName:    "main",
		Additions:      10,
		Deletions:      2,
		ChangedFiles:   3,
		ReviewDecision: "APPROVED",
		Body:           "First line\nSecond line",
		Labels:         []domain.Label{{Name: "bug"}, {Name: "p1"}},
		RequestedReviewers: []domain.RequestedReviewer{
			{Kind: "user", Name: "bob"},
			{Kind: "team", Name: "backend"},
		},
		Assignees:          []string{"carol"},
		PendingReviewCount: 2,
		Timeline: []domain.TimelineItem{
			{Kind: "IssueComment", Author: "dave", Body: "looks good"},
			{Kind: "PullRequestReview", Author: "erin", State: "APPROVED", Body: "ship it"},
		},
		Threads: []domain.Thread{
			{
				Path: "foo.go", Line: &line,
				Comments: []domain.Comment{
					{Author: "bob", Body: "please rename this"},
					{Author: "alice", Body: "done"},
				},
			},
		},
	}
	m := New(config.Default(), nil)
	m.Width, m.Height = 120, 40
	m.PRDetail = &d
	m = setMainOverview(m, d)
	out := ansi.Strip(strings.Join(m.MainLines, "\n"))

	wants := []string{
		"#7  Add retries",
		"● OPEN",              // state badge, not "OPEN · by alice"
		"✓ APPROVED",          // review decision badge
		"alice",               // author moved into the badge row
		"+10 -2 · 3 files",    // change counts
		"✎ 2 draft",           // pending-review signal must survive header compression
		"feat/retries → main", // compact branch row
		"reviewers bob, @backend",
		"assignees carol",
		"labels bug, p1",
		"Description",
		"First line",
		"Second line",
		"Conversation · 2",
		"dave",
		"erin",
		"Review threads · 1",
		"foo.go:12",
		"please rename this", // human threads default expanded
		"done",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("overview missing %q in:\n%s", w, out)
		}
	}

	// Empty fields are dropped rather than printed as an em dash.
	if strings.Contains(out, "—") {
		t.Errorf("empty metadata should be omitted, not dashed:\n%s", out)
	}
}

// Every overview line must fit the pane budget: the old builder emitted raw
// unwrapped bodies that got clipped at the right edge.
func TestPROverviewRespectsWidth(t *testing.T) {
	m := New(config.Default(), nil)
	m.Width, m.Height = 100, 40
	d := domain.PRDetail{
		Number: 1, Title: "wide", State: "OPEN",
		Body: strings.Repeat("lorem ipsum dolor sit amet ", 20),
		Timeline: []domain.TimelineItem{
			{Kind: "IssueComment", Author: "dave", Body: strings.Repeat("chatter ", 60)},
		},
	}
	m.PRDetail = &d
	m = setMainOverview(m, d)

	budget := mainContentWidth(m)
	if budget <= 0 {
		t.Fatal("expected a positive content budget")
	}
	for i, l := range m.MainLines {
		if w := ansi.StringWidth(l); w > budget {
			t.Fatalf("line %d is %d cols, budget %d: %q", i, w, budget, ansi.Strip(l))
		}
	}
}

// ── Checks panel tests ────────────────────────────────────────────────────────

func TestChecksGlyphSuccess(t *testing.T) {
	c := domain.Check{Kind: "checkRun", Status: "completed", Conclusion: "success"}
	if got := CheckGlyph(c); got != "✓" {
		t.Errorf("success glyph: got %q, want ✓", got)
	}
}

func TestChecksGlyphNeutral(t *testing.T) {
	c := domain.Check{Kind: "checkRun", Status: "completed", Conclusion: "neutral"}
	if got := CheckGlyph(c); got != "✓" {
		t.Errorf("neutral glyph: got %q, want ✓", got)
	}
}

func TestChecksGlyphFailure(t *testing.T) {
	for _, conclusion := range []string{"failure", "error", "timed_out", "startup_failure"} {
		c := domain.Check{Kind: "checkRun", Status: "completed", Conclusion: conclusion}
		if got := CheckGlyph(c); got != "✗" {
			t.Errorf("conclusion %q glyph: got %q, want ✗", conclusion, got)
		}
	}
}

func TestChecksGlyphActionRequired(t *testing.T) {
	c := domain.Check{Kind: "checkRun", Status: "completed", Conclusion: "action_required"}
	if got := CheckGlyph(c); got != "±" {
		t.Errorf("action_required glyph: got %q, want ±", got)
	}
}

func TestChecksGlyphCancelled(t *testing.T) {
	for _, conclusion := range []string{"cancelled", "stale", "skipped"} {
		c := domain.Check{Kind: "checkRun", Status: "completed", Conclusion: conclusion}
		if got := CheckGlyph(c); got != "⊘" {
			t.Errorf("conclusion %q glyph: got %q, want ⊘", conclusion, got)
		}
	}
}

func TestChecksGlyphPending(t *testing.T) {
	for _, status := range []string{"in_progress", "queued", "waiting", "pending"} {
		c := domain.Check{Kind: "checkRun", Status: status}
		if got := CheckGlyph(c); got != "◔" {
			t.Errorf("status %q glyph: got %q, want ◔", status, got)
		}
	}
}

func TestChecksGlyphStatusContext(t *testing.T) {
	cases := []struct {
		state string
		want  string
	}{
		{"success", "✓"},
		{"failure", "✗"},
		{"error", "✗"},
		{"pending", "◔"},
		{"", "⊘"},
	}
	for _, c := range cases {
		check := domain.Check{Kind: "status", State: c.state}
		if got := CheckGlyph(check); got != c.want {
			t.Errorf("status context state=%q: got %q, want %q", c.state, got, c.want)
		}
	}
}

func TestChecksTitleShowsTabs(t *testing.T) {
	m := newChecksModel()
	title := checksTitle(m)
	if !strings.Contains(title, "Checks") {
		t.Errorf("Checks panel title missing Checks tab: %q", title)
	}
	if !strings.Contains(title, "Timeline") {
		t.Errorf("Checks panel title missing Timeline tab: %q", title)
	}
}

func TestChecksTitleActiveTabBracketed(t *testing.T) {
	m := newChecksModel()
	m.ChecksPanel.Tab = ChecksTabChecks
	title := checksTitle(m)
	if !strings.Contains(title, "[Checks]") {
		t.Errorf("active Checks tab not bracketed: %q", title)
	}
}

func TestChecksViewShowsCheckRows(t *testing.T) {
	m := newChecksModel()
	view := ChecksView(m)
	if !strings.Contains(view, "CI / build") {
		t.Errorf("ChecksView missing check name: %q", view)
	}
	if !strings.Contains(view, "lint") {
		t.Errorf("ChecksView missing check name: %q", view)
	}
}

func TestChecksViewNoPRDetail(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 80
	m.Height = 24
	view := ChecksView(m)
	if !strings.Contains(view, "no checks") {
		t.Errorf("expected 'no checks' placeholder, got: %q", view)
	}
}

// ── Timeline tests ────────────────────────────────────────────────────────────

func TestTimelineTabView(t *testing.T) {
	m := newChecksModel()
	m.ChecksPanel.Tab = ChecksTabTimeline
	if title := checksTitle(m); !strings.Contains(title, "[Timeline]") {
		t.Errorf("active Timeline tab not bracketed: %q", title)
	}
}

func TestTimelineShowsEvents(t *testing.T) {
	m := newChecksModel()
	m.ChecksPanel.Tab = ChecksTabTimeline
	view := ChecksView(m)
	// Timeline should show the issue comment and review in chrono order
	if !strings.Contains(view, "alice") {
		t.Errorf("Timeline missing author alice: %q", view)
	}
	if !strings.Contains(view, "LGTM") {
		t.Errorf("Timeline missing comment body: %q", view)
	}
}

func TestTimelineChronologicalOrder(t *testing.T) {
	m := newChecksModel()
	m.ChecksPanel.Tab = ChecksTabTimeline
	view := ChecksView(m)
	// Earlier event should appear before later event in the rendered output
	aliceIdx := strings.Index(view, "alice")
	bobIdx := strings.Index(view, "bob")
	if aliceIdx < 0 || bobIdx < 0 {
		t.Fatalf("view missing authors: %q", view)
	}
	if aliceIdx > bobIdx {
		t.Errorf("timeline not chronological: alice appears after bob")
	}
}

func TestTimelineNoEvents(t *testing.T) {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 80
	m.Height = 24
	detail := samplePRDetail()
	detail.Timeline = nil
	m.PRDetail = &detail
	m.ChecksPanel.Tab = ChecksTabTimeline
	m.FocusStack = []FocusContext{FocusChecks}
	view := ChecksView(m)
	if !strings.Contains(view, "no timeline events") {
		t.Errorf("expected 'no timeline events' placeholder, got: %q", view)
	}
}

// ── Hint bar tests ────────────────────────────────────────────────────────────

func TestHintBarPRsContext(t *testing.T) {
	m := newPRListModel()
	m.FocusStack = []FocusContext{FocusPRs}
	view := HintBarView(m)
	// Should show the PR-specific entry (openPR) plus the trimmed universal
	// shortlist, which is now just the help key.
	if !strings.Contains(view, "<enter>") {
		t.Errorf("HintBar for prs missing <enter>: %q", view)
	}
	if !strings.Contains(view, "?") {
		t.Errorf("HintBar for prs missing help key: %q", view)
	}
}

func TestHintBarStatusContext(t *testing.T) {
	m := newStatusModel()
	m.FocusStack = []FocusContext{FocusStatus}
	view := HintBarView(m)
	// Status has 'e' for editConfig
	if !strings.Contains(view, "e") {
		t.Errorf("HintBar for status missing 'e': %q", view)
	}
}

func TestHintBarChecksContext(t *testing.T) {
	m := newChecksModel()
	m.FocusStack = []FocusContext{FocusChecks}
	view := HintBarView(m)
	if view == "" {
		t.Error("HintBar for checks returned empty string")
	}
}

func TestHintBarDisabledWhenShowBottomLineFalse(t *testing.T) {
	m := newPRListModel()
	m.Config.GUI.ShowBottomLine = false
	view := HintBarView(m)
	if view != "" {
		t.Errorf("HintBar should be empty when ShowBottomLine=false, got: %q", view)
	}
}

func TestHintBarDiffersByContext(t *testing.T) {
	base := newPRListModel()

	base.FocusStack = []FocusContext{FocusPRs}
	prView := HintBarView(base)

	base.FocusStack = []FocusContext{FocusChecks}
	checksView := HintBarView(base)

	if prView == checksView {
		t.Error("hint bar should differ between prs and checks contexts")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func samplePRs() []domain.PRSummary {
	return []domain.PRSummary{
		{Number: 1, Title: "Fix auth bug", Author: "alice", HeadRefName: "fix/auth"},
		{Number: 2, Title: "Update readme", Author: "bob", HeadRefName: "docs/readme"},
		{Number: 3, Title: "Refactor cache layer", Author: "carol", HeadRefName: "refactor/cache"},
	}
}

func newPRListModel() Model {
	cfg := config.Default()
	cfg.GUI.ShowBottomLine = true
	m := New(cfg, nil)
	m.Loading = false
	m.Width = 80
	m.Height = 24
	m.Repo = forge.Repo{Owner: "myorg", Name: "myrepo"}
	m.PRs = samplePRs()
	m.FocusStack = []FocusContext{FocusPRs}
	return m
}

func newStatusModel() Model {
	m := newPRListModel()
	m.ViewerLogin = "testuser"
	m.Repo = forge.Repo{Owner: "myorg", Name: "myrepo"}
	m.FocusStack = []FocusContext{FocusStatus}
	return m
}

func samplePRDetail() domain.PRDetail {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	return domain.PRDetail{
		Number:         42,
		Title:          "Sample PR",
		ReviewDecision: "REVIEW_REQUIRED",
		Checks: []domain.Check{
			{Kind: "checkRun", Name: "CI / build", Status: "completed", Conclusion: "success"},
			{Kind: "checkRun", Name: "lint", Status: "completed", Conclusion: "failure"},
		},
		Timeline: []domain.TimelineItem{
			{Kind: "IssueComment", Author: "alice", Body: "LGTM!", SortAt: t1},
			{Kind: "PullRequestReview", Author: "bob", Body: "Looks good to merge", SortAt: t2},
		},
	}
}

func newChecksModel() Model {
	m := newStatusModel()
	m.FocusStack = []FocusContext{FocusChecks}
	detail := samplePRDetail()
	m.PRDetail = &detail
	return m
}
