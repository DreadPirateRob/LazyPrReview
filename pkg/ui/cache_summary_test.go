package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

func TestPRTabSwitchServesFreshCache(t *testing.T) {
	m, _ := prSearchModel() // active = Review requested
	m.PRCache[forge.FilterMine] = prCacheEntry{prs: []domain.PRSummary{{Number: 42}}, at: time.Now()}
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'}) // → Mine
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("a fresh cache hit must not refetch")
	}
	if len(got.PRs) != 1 || got.PRs[0].Number != 42 {
		t.Fatalf("expected cached PRs served, got %+v", got.PRs)
	}
	if got.Activity != "" {
		t.Fatalf("cache hit must not show a loading banner, got %q", got.Activity)
	}
}

func TestPRTabSwitchStaleCacheRefetches(t *testing.T) {
	m, _ := prSearchModel()
	m.PRCache[forge.FilterMine] = prCacheEntry{prs: []domain.PRSummary{{Number: 42}}, at: time.Now().Add(-2 * time.Hour)}
	_, cmd := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
	if cmd == nil {
		t.Fatal("a stale cache entry must trigger a refetch")
	}
}

func TestPRLoadStoresCache(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterMine
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterMine, PRs: []domain.PRSummary{{Number: 7}}})
	got := updated.(Model)
	if cached, ok := got.cachedPRs(forge.FilterMine); !ok || len(cached) != 1 || cached[0].Number != 7 {
		t.Fatalf("expected result cached, got ok=%v %+v", ok, cached)
	}
}

func TestPRErrorNotCached(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterMine
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterMine, Err: errString("boom")})
	got := updated.(Model)
	if _, ok := got.cachedPRs(forge.FilterMine); ok {
		t.Fatal("errors must not populate the cache")
	}
}

func TestStartupWarmsReviewRequestedCache(t *testing.T) {
	m, _ := prSearchModel()
	updated, _ := m.Update(probesLoadedMsg{Repo: m.Repo, PRs: []domain.PRSummary{{Number: 3}}})
	got := updated.(Model)
	if cached, ok := got.cachedPRs(forge.FilterReviewRequested); !ok || len(cached) != 1 {
		t.Fatalf("startup PRs should warm the Review-requested cache, ok=%v %+v", ok, cached)
	}
}

func TestSearchServesFreshCache(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterSearch
	m.SearchCache["is:open"] = prCacheEntry{prs: []domain.PRSummary{{Number: 5}}, at: time.Now()}
	u, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = u.(Model)
	for _, r := range "is:open" {
		u, _ = m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		m = u.(Model)
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("a fresh search cache hit must not hit the server")
	}
	if len(got.PRs) != 1 || got.PRs[0].Number != 5 {
		t.Fatalf("expected cached search results, got %+v", got.PRs)
	}
}

func TestChecksSummaryFailing(t *testing.T) {
	m := newChecksModel() // samplePRDetail: CI/build success + lint failure
	m.FocusStack = []FocusContext{FocusPRs}
	s := checksSummary(m)
	if !strings.Contains(s, "✗") || !strings.Contains(s, "1 check failing") {
		t.Fatalf("expected '✗ 1 check failing', got %q", s)
	}
}

func TestChecksSummaryPassing(t *testing.T) {
	m := newChecksModel()
	detail := *m.PRDetail
	detail.Checks = []domain.Check{{Kind: "checkRun", Status: "completed", Conclusion: "success"}}
	m.PRDetail = &detail
	if s := checksSummary(m); !strings.Contains(s, "CI passing") {
		t.Fatalf("expected passing summary, got %q", s)
	}
}

func TestChecksViewCollapsesWhenUnfocused(t *testing.T) {
	m := newChecksModel()
	m.FocusStack = []FocusContext{FocusPRs}
	if v := ChecksView(m); strings.Contains(v, "CI / build") {
		t.Fatalf("unfocused checks must show the summary, not rows: %q", v)
	}
	m.FocusStack = []FocusContext{FocusChecks}
	if v := ChecksView(m); !strings.Contains(v, "CI / build") {
		t.Fatalf("focused checks must show full rows: %q", v)
	}
}

func TestChecksTitleCollapsesWhenUnfocused(t *testing.T) {
	m := newChecksModel()
	m.FocusStack = []FocusContext{FocusPRs}
	if title := checksTitle(m); title != "Checks" {
		t.Fatalf("unfocused checks title should be plain 'Checks', got %q", title)
	}
}
