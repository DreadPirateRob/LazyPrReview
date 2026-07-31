package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

func TestLoadingBlocksActionsAllowsNav(t *testing.T) {
	m, _ := prSearchModel()
	m.LoadingPRs = true

	// A data-consuming key (enter to open a PR) is swallowed → no fetch cmd.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter must be blocked while the PR list is loading")
	}

	// Tab navigation stays live (switching supersedes the load).
	updated, _ := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
	if updated.(Model).PRFilter != forge.FilterMine {
		t.Fatal("] tab switch must stay live while loading")
	}

	// Focus navigation stays live.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "3", Code: '3'})
	if updated.(Model).CurrentFocus() != FocusFiles {
		t.Fatal("1-5 focus must stay live while loading")
	}
}

func TestLoadingDetailBlocksFileActions(t *testing.T) {
	m := seededModel(t)
	m.FocusStack = []FocusContext{FocusFiles}
	m.LoadingDetail = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open file
	if updated.(Model).MainMode == MainDiff {
		t.Fatal("opening a file must be blocked during detail load")
	}
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	if updated.(Model).FilesPanel.Cursor != 0 {
		t.Fatal("cursor movement must be blocked during detail load")
	}
}

func TestTabSwitchSetsAndResultClearsLoading(t *testing.T) {
	m, _ := prSearchModel()
	updated, _ := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'}) // → Mine (uncached)
	got := updated.(Model)
	if !got.LoadingPRs {
		t.Fatal("switching to an uncached tab should set LoadingPRs")
	}
	updated, _ = got.Update(prsLoadedMsg{Filter: forge.FilterMine, PRs: []domain.PRSummary{{Number: 1}}})
	if updated.(Model).LoadingPRs {
		t.Fatal("the matching result should clear LoadingPRs")
	}
}

func TestDetailStaleGuardIgnoresWrongPR(t *testing.T) {
	m := seededModel(t)
	m.OpenedPRNumber = 5
	m.LoadingDetail = true

	updated, _ := m.Update(prDetailLoadedMsg{Detail: domain.PRDetail{Number: 2, Title: "other"}})
	got := updated.(Model)
	if got.PRDetail != nil && got.PRDetail.Number == 2 {
		t.Fatal("detail for a PR the user didn't open must be ignored")
	}
	if !got.LoadingDetail {
		t.Fatal("loading should persist until the requested PR arrives")
	}

	updated, _ = got.Update(prDetailLoadedMsg{Detail: domain.PRDetail{Number: 5, Title: "wanted"}})
	got = updated.(Model)
	if got.PRDetail == nil || got.PRDetail.Number != 5 {
		t.Fatal("the requested PR's detail must apply")
	}
	if got.LoadingDetail {
		t.Fatal("loading must clear when the requested PR lands")
	}
}

func landscapeModel(focus FocusContext) Model {
	m := New(config.Default(), nil)
	m.Loading = false
	m.Width = 120
	m.Height = 40
	m.FocusStack = []FocusContext{focus}
	return m
}

func TestCollapsibleUnfocusedGetsOneLiner(t *testing.T) {
	l := ComputeLayout(landscapeModel(FocusPRs)) // Status(0) & Checks(4) unfocused
	if l.PanelHeights[0] != 3 {
		t.Fatalf("unfocused Status should collapse to a 3-row box, got %d", l.PanelHeights[0])
	}
	if l.PanelHeights[4] != 3 {
		t.Fatalf("unfocused Checks should collapse to a 3-row box, got %d", l.PanelHeights[4])
	}
	if l.PanelHeights[1] <= 3 {
		t.Fatalf("focused PRs browser should expand, got %d", l.PanelHeights[1])
	}
}

func TestCollapsibleFocusedExpands(t *testing.T) {
	m := landscapeModel(FocusChecks)
	// Focused collapsibles grow to fit their DATA, so there must be some: with
	// no checks at all, staying at the collapsed height is the correct outcome.
	m.PRDetail = &domain.PRDetail{Checks: []domain.Check{
		{Name: "build", Status: "completed", Conclusion: "success"},
		{Name: "lint", Status: "completed", Conclusion: "failure"},
		{Name: "test", Status: "completed", Conclusion: "success"},
	}}
	l := ComputeLayout(m)
	if l.PanelHeights[4] <= 3 {
		t.Fatalf("focused Checks should expand past the one-liner, got %d", l.PanelHeights[4])
	}
	if l.PanelHeights[0] != 3 {
		t.Fatalf("unfocused Status should stay collapsed, got %d", l.PanelHeights[0])
	}
}

func TestFocusedCollapsibleStaysSmallWithNoData(t *testing.T) {
	l := ComputeLayout(landscapeModel(FocusChecks)) // no PRDetail -> "(no checks)"
	if l.PanelHeights[4] != 3 {
		t.Fatalf("focused Checks with no data has nothing to fit, want 3 rows, got %d", l.PanelHeights[4])
	}
}

func TestLoadingPaneShowsMarker(t *testing.T) {
	m := landscapeModel(FocusPRs)
	m.PRs = []domain.PRSummary{{Number: 1, Title: "x"}}
	m.LoadingPRs = true
	if !strings.Contains(fullView(m), "⟳") {
		t.Fatal("a loading pane should render the ⟳ marker in its title")
	}
}
