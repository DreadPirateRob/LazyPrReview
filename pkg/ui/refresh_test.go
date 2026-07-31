package ui

import (
	"strings"
	"testing"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

func TestRateLimited(t *testing.T) {
	m := Model{}
	if m.rateLimited() {
		t.Fatal("no PR detail → not rate limited")
	}
	m.PRDetail = &domain.PRDetail{RateLimitRemaining: 42}
	if !m.rateLimited() {
		t.Fatal("42 < 100 → rate limited")
	}
	m.PRDetail.RateLimitRemaining = 4000
	if m.rateLimited() {
		t.Fatal("4000 → not rate limited")
	}
}

func TestShouldAutoRefresh(t *testing.T) {
	m, _ := prSearchModel() // active = Review requested
	m.Config.GitHub.AutoRefreshInterval = 60
	if !m.shouldAutoRefresh() {
		t.Fatal("default active tab should auto-refresh")
	}

	m.Config.GitHub.AutoRefreshInterval = 0
	if m.shouldAutoRefresh() {
		t.Fatal("interval 0 disables auto-refresh")
	}
	m.Config.GitHub.AutoRefreshInterval = 60

	m.PRFilter = forge.FilterSearch
	if m.shouldAutoRefresh() {
		t.Fatal("Search tab is user-driven; no auto-refresh")
	}
	m.PRFilter = forge.FilterAllOpen

	m.LoadingPRs = true
	if m.shouldAutoRefresh() {
		t.Fatal("in-flight fetch → no auto-refresh")
	}
	m.LoadingPRs = false

	m.PRDetail = &domain.PRDetail{RateLimitRemaining: 10}
	if m.shouldAutoRefresh() {
		t.Fatal("rate-limited → auto-refresh pauses")
	}
}

func TestAutoRefreshTickIsSilent(t *testing.T) {
	m, _ := prSearchModel()
	m.Config.GitHub.AutoRefreshInterval = 60
	updated, cmd := m.Update(autoRefreshTickMsg{})
	if cmd == nil {
		t.Fatal("a tick on an active tab should return a refetch+reschedule cmd")
	}
	if updated.(Model).LoadingPRs {
		t.Fatal("auto-refresh must be silent — no loading dim/block")
	}
}

func TestRefreshPreservesSelectionByNumber(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterMine
	m.PRs = []domain.PRSummary{{Number: 1}, {Number: 2}, {Number: 3}}
	m.PRPanel.Cursor = 2 // PR #3
	// A refresh returns the same set reordered; the cursor should track #3.
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterMine, PRs: []domain.PRSummary{{Number: 3}, {Number: 1}}})
	if got := updated.(Model).PRPanel.Cursor; got != 0 {
		t.Fatalf("cursor should follow PR #3 to its new index 0, got %d", got)
	}
}

func TestTabResultResetsSelectionWhenAbsent(t *testing.T) {
	m, _ := prSearchModel()
	m.PRFilter = forge.FilterMine
	m.PRs = []domain.PRSummary{{Number: 1}}
	m.PRPanel.Cursor = 0 // PR #1
	// The new tab's list doesn't contain the prior selection → reset to top.
	updated, _ := m.Update(prsLoadedMsg{Filter: forge.FilterMine, PRs: []domain.PRSummary{{Number: 9}, {Number: 8}}})
	if got := updated.(Model).PRPanel.Cursor; got != 0 {
		t.Fatalf("absent prior selection should reset cursor to 0, got %d", got)
	}
}

func TestStatusRateLimitWarn(t *testing.T) {
	m := newStatusModel()
	m.PRDetail = &domain.PRDetail{RateLimitRemaining: 42}
	if !strings.Contains(StatusView(m), "⚠") {
		t.Fatalf("low rate limit should warn, got %q", StatusView(m))
	}
	m.PRDetail.RateLimitRemaining = 4000
	if strings.Contains(StatusView(m), "⚠") {
		t.Fatalf("healthy rate limit should not warn, got %q", StatusView(m))
	}
}
