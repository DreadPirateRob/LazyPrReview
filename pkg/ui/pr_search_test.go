package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

func prSearchModel() (Model, *fakeforge.Fake) {
	f := fakeforge.New()
	f.Repo = forge.Repo{Owner: "o", Name: "r"}
	f.PRSummaries[forge.FilterMine] = []domain.PRSummary{{Number: 2, Title: "mine pr"}}
	f.SearchPRResults["is:merged"] = []domain.PRSummary{{Number: 9, Title: "old one", State: "MERGED"}}
	m := New(config.Default(), f)
	m.Loading = false
	m.Repo = f.Repo
	m.FocusStack = []FocusContext{FocusPRs}
	m.PRs = []domain.PRSummary{{Number: 1, Title: "requested"}}
	return m, f
}

func TestPRTabSwitchFetches(t *testing.T) {
	m, _ := prSearchModel()
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
	got := updated.(Model)
	if got.PRFilter != forge.FilterMine {
		t.Fatalf("expected Mine tab, got %d", got.PRFilter)
	}
	if cmd == nil {
		t.Fatal("expected a fetch cmd on tab switch")
	}
	msg := cmd()
	lm, ok := msg.(prsLoadedMsg)
	if !ok || lm.Filter != forge.FilterMine {
		t.Fatalf("expected prsLoadedMsg for Mine, got %#v", msg)
	}
	updated, _ = got.Update(lm)
	got = updated.(Model)
	if len(got.PRs) != 1 || got.PRs[0].Number != 2 {
		t.Fatalf("expected Mine PRs applied, got %+v", got.PRs)
	}
}

func TestPRStaleFetchIgnored(t *testing.T) {
	m, _ := prSearchModel() // active tab = Review requested
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterMine, PRs: []domain.PRSummary{{Number: 99}}})
	got := updated.(Model)
	if len(got.PRs) != 1 || got.PRs[0].Number != 1 {
		t.Fatalf("stale (non-active-tab) result must be ignored, got %+v", got.PRs)
	}
}

func TestPRFetchErrorKeepsListShowsToast(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterMine
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterMine, Err: errString("rate limited")})
	got := updated.(Model)
	if got.Toast.Message == "" {
		t.Fatal("expected an error toast")
	}
	if len(got.PRs) != 1 {
		t.Fatalf("error must not wipe the current list, got %+v", got.PRs)
	}
}

func TestPRSearchTabFlow(t *testing.T) {
	m, _ := prSearchModel()
	// Walk to the Search tab (Requested→Mine→All→Search).
	for range 3 {
		u, _ := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
		m = u.(Model)
	}
	if m.PRFilter != forge.FilterSearch {
		t.Fatalf("expected Search tab, got %d", m.PRFilter)
	}
	if m.PRs != nil {
		t.Fatal("entering Search should clear the list until a query runs")
	}
	if m.Activity != "" {
		t.Fatalf("Search tab must not show a loading banner, got %q", m.Activity)
	}
	// Open the query input and type.
	u, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = u.(Model)
	if !m.FilterActive {
		t.Fatal("expected / to open the search input")
	}
	for _, r := range "is:merged" {
		u, _ = m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		m = u.(Model)
	}
	if m.PRPanel.Filter != "is:merged" {
		t.Fatalf("query build failed, got %q", m.PRPanel.Filter)
	}
	// Enter runs the server search.
	u, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = u.(Model)
	if cmd == nil {
		t.Fatal("expected a search cmd on enter")
	}
	lm, ok := cmd().(prsLoadedMsg)
	if !ok || lm.Filter != forge.FilterSearch || lm.Query != "is:merged" {
		t.Fatalf("expected search prsLoadedMsg, got %#v", lm)
	}
	u, _ = m.Update(lm)
	m = u.(Model)
	if len(m.PRs) != 1 || m.PRs[0].Number != 9 {
		t.Fatalf("expected search results applied, got %+v", m.PRs)
	}
	// The Search tab must not client-filter its server results.
	if got := VisiblePRs(m); len(got) != 1 {
		t.Fatalf("Search tab should show server results unfiltered, got %d", len(got))
	}
}

func TestPRSearchStaleQueryIgnored(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterSearch
	m.PRPanel.Filter = "current" // the committed query
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterSearch, Query: "outdated", PRs: []domain.PRSummary{{Number: 7}}})
	got := updated.(Model)
	if len(got.PRs) != 1 && got.PRs != nil {
		t.Fatalf("a result for a superseded query must be ignored, got %+v", got.PRs)
	}
	if len(got.PRs) == 1 && got.PRs[0].Number == 7 {
		t.Fatal("stale search result was wrongly applied")
	}
}

func TestRenderPRRowStateTags(t *testing.T) {
	if got := RenderPRRow(domain.PRSummary{Number: 1, Title: "t", State: "MERGED"}); !strings.Contains(got, "[merged]") {
		t.Errorf("merged tag missing: %q", got)
	}
	if got := RenderPRRow(domain.PRSummary{Number: 2, Title: "t", State: "CLOSED"}); !strings.Contains(got, "[closed]") {
		t.Errorf("closed tag missing: %q", got)
	}
	if got := RenderPRRow(domain.PRSummary{Number: 3, Title: "t", State: "OPEN", IsDraft: true}); !strings.Contains(got, "[draft]") {
		t.Errorf("draft tag missing: %q", got)
	}
}

func TestPRSearchTabTitleShowsQuery(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterSearch
	m.PRPanel.Filter = "is:merged label:bug"
	title := prListTitle(m)
	if !strings.Contains(title, "Search") || !strings.Contains(title, "/is:merged label:bug") {
		t.Fatalf("search title must show active tab + full query, got %q", title)
	}
	// The full tab list is what truncated the query off the border; on the
	// Search tab it must lead with the active tab instead.
	if strings.Contains(title, "Review requested - Mine") {
		t.Fatalf("search title should not cram the full tab list, got %q", title)
	}
}

func TestPRTitleWhileTypingLeadsWithQuery(t *testing.T) {
	m, _ := prSearchModel() // focus PRs, tab = Review requested
	m.FilterActive = true
	m.PRPanel.Filter = "auth"
	title := prListTitle(m)
	if !strings.Contains(title, "/auth█") {
		t.Fatalf("typing must keep the query visible in the title, got %q", title)
	}
	if strings.Contains(title, "Mine - All open") {
		t.Fatalf("while typing, title should lead with active tab, not full list: %q", title)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestRenderPRRowTwoLines(t *testing.T) {
	pr := domain.PRSummary{
		Number: 7719, Title: "allow OHLC charting", Author: "DreadPirateRob",
		HeadRefName: "bugfix/CJ-11877", StatusCheckState: "failure",
		Labels: []domain.Label{{Name: "bug"}, {Name: "Frontend"}},
	}
	lines := strings.Split(RenderPRRow(pr), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "#7719") || !strings.Contains(lines[0], "allow OHLC charting") {
		t.Fatalf("line 1 should hold #num + title, got %q", lines[0])
	}
	if strings.Contains(lines[0], "DreadPirateRob") || strings.Contains(lines[0], "⎇") {
		t.Fatalf("author/branch must be on line 2, not line 1: %q", lines[0])
	}
	if !strings.Contains(lines[1], "DreadPirateRob") || !strings.Contains(lines[1], "| ⎇bugfix/CJ-11877") {
		t.Fatalf("line 2 should hold author + branch, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "[bug][Frontend]") {
		t.Fatalf("line 2 should hold labels, got %q", lines[1])
	}
	if strings.Count(lines[1], " | ") != 2 {
		t.Fatalf("line 2 should have two ' | ' separators, got %q", lines[1])
	}
}

func TestPRListViewTwoLinesPerPR(t *testing.T) {
	m := newPRListModel()
	lines := strings.Split(strings.TrimRight(PRListView(m, 20), "\n"), "\n")
	if n := len(VisiblePRs(m)); len(lines) != 2*n {
		t.Fatalf("expected 2 lines per PR (%d rows), got %d", 2*n, len(lines))
	}
}

func TestPRRowLine2Connector(t *testing.T) {
	pr := domain.PRSummary{Number: 7, Title: "t", Author: "alice", HeadRefName: "b"}
	lines := strings.Split(RenderPRRow(pr), "\n")
	if !strings.HasPrefix(lines[1], "  ╰─ ") {
		t.Fatalf("line 2 should start with the ╰─ tree connector, got %q", lines[1])
	}
}
