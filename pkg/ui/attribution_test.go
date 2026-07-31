package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

// The reported gap: an approved PR showed "✓ APPROVED" with nobody attached, and a
// human approval without a comment body was invisible in the conversation — bots
// showed because their verdicts live in the body text; a human's lives in the
// review STATE, which nothing rendered.
func attributionModel(t *testing.T, d domain.PRDetail) string {
	t.Helper()
	m := New(config.Default(), fakeforge.New())
	m.Width, m.Height = 160, 44
	m = setMainOverview(m, d)
	return ansi.Strip(strings.Join(m.MainLines, "\n"))
}

func TestOverviewNamesApprovers(t *testing.T) {
	out := attributionModel(t, domain.PRDetail{
		Title: "t", State: "OPEN", ReviewDecision: "APPROVED",
		LatestReviews: []domain.Review{
			{Author: "alice", State: "APPROVED"},
			{Author: "carol", State: "CHANGES_REQUESTED"},
			{Author: "dan", State: "APPROVED"},
		},
	})
	if !strings.Contains(out, "APPROVED by alice, dan") {
		t.Fatalf("badge must name the approvers, got:\n%s", firstLines(out, 6))
	}
	if strings.Contains(out, "APPROVED by alice, carol") {
		t.Fatal("a reviewer whose latest review is not an approval must not be listed")
	}
}

func TestOverviewNamesMerger(t *testing.T) {
	out := attributionModel(t, domain.PRDetail{
		Title: "t", State: "MERGED", MergedBy: "bob",
	})
	if !strings.Contains(out, "MERGED by bob") {
		t.Fatalf("state badge must name who merged, got:\n%s", firstLines(out, 6))
	}
}

// A human approval with no body must still read as an approval in the conversation.
func TestConversationShowsBodylessHumanApproval(t *testing.T) {
	at := time.Date(2026, 4, 7, 11, 0, 0, 0, time.UTC)
	out := attributionModel(t, domain.PRDetail{
		Title: "t", State: "OPEN", ReviewDecision: "APPROVED",
		Timeline: []domain.TimelineItem{
			{Kind: "review", Author: "alice", State: "APPROVED", SortAt: at},
			{Kind: "review", Author: "carol", State: "CHANGES_REQUESTED", SortAt: at.Add(time.Hour)},
			{Kind: "merged", Author: "bob", SortAt: at.Add(2 * time.Hour)},
		},
	})
	if !strings.Contains(out, "alice approved") {
		t.Fatalf("a bodyless approval must render its state, got:\n%s", out)
	}
	if !strings.Contains(out, "carol requested changes") {
		t.Fatalf("changes-requested must render its state, got:\n%s", out)
	}
	if !strings.Contains(out, "bob merged this pull request") {
		t.Fatalf("the merge event must appear in the conversation, got:\n%s", out)
	}
	if !strings.Contains(out, "· alice approved") {
		t.Fatalf("a bodyless event has nothing to unfold and must not wear a fold caret, got:\n%s", out)
	}
}

// A review WITH a body keeps its fold caret and its body rows.
func TestConversationReviewWithBodyStillFolds(t *testing.T) {
	at := time.Date(2026, 4, 7, 11, 0, 0, 0, time.UTC)
	out := attributionModel(t, domain.PRDetail{
		Title: "t", State: "OPEN",
		Timeline: []domain.TimelineItem{
			{Kind: "review", Author: "erin", State: "APPROVED", Body: "ship it", SortAt: at},
		},
	})
	if !strings.Contains(out, "▾ erin approved") {
		t.Fatalf("a review with a body stays foldable, got:\n%s", out)
	}
	if !strings.Contains(out, "ship it") {
		t.Fatalf("expanded review body must render, got:\n%s", out)
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
