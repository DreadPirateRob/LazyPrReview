package github

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// TestLiveAuthoringSmoke exercises the real GraphQL write path against a
// throwaway PR: ensure pending review -> add a draft thread -> read it back ->
// discard the review (cleanup). Gated behind LAZYPR_LIVE=1 so it never runs in
// the normal suite. Leaves the PR clean via the discard defer.
func TestLiveAuthoringSmoke(t *testing.T) {
	if os.Getenv("LAZYPR_LIVE") != "1" {
		t.Skip("set LAZYPR_LIVE=1 to run the live write test")
	}
	const (
		prID    = "PR_kwDOKvQieM73BnQS"
		headOID = "54ac02a08808b1de8b326f9110f2111aef69fd2a"
		path    = "app/index.js"
		line    = 403
	)
	repo := forge.Repo{Owner: "DreadPirateRob", Name: "stratoptic"}
	runner := ghcli.New(30*time.Second, func(ghcli.CommandLogEntry) {})
	c := New(runner)
	ctx := context.Background()

	// Safety: only run against a clean PR so the cleanup discard can never wipe
	// a preexisting pending review's drafts.
	if pre, err := c.PRDetail(ctx, repo, 4); err != nil {
		t.Fatalf("preflight PRDetail: %v", err)
	} else if pre.PendingReview != nil {
		t.Skip("PR already has a pending review; skipping to avoid disturbing it")
	}

	reviewID, err := c.EnsurePendingReview(ctx, prID, headOID)
	if err != nil {
		t.Fatalf("EnsurePendingReview: %v", err)
	}
	if reviewID == "" {
		t.Fatal("EnsurePendingReview returned an empty review id")
	}
	t.Logf("pending review id: %s", reviewID)

	defer func() {
		if err := c.DiscardReview(ctx, reviewID); err != nil {
			t.Errorf("cleanup DiscardReview: %v", err)
		} else {
			t.Log("cleaned up: pending review discarded")
		}
	}()

	body := "lazypr live smoke — safe to ignore"
	in := forge.AddThreadInput{
		Repo:        repo,
		PRID:        prID,
		ReviewID:    reviewID,
		Path:        path,
		Line:        line,
		Side:        "RIGHT",
		Body:        body,
		SubjectType: "LINE",
	}
	if err := c.AddReviewThread(ctx, in); err != nil {
		t.Fatalf("AddReviewThread: %v", err)
	}

	pr, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("PRDetail readback: %v", err)
	}
	found := false
	if pr.PendingReview != nil {
		for _, cm := range pr.PendingReview.Comments {
			if strings.Contains(cm.Body, "live smoke") {
				found = true
			}
		}
	}
	for _, th := range pr.Threads {
		for _, cm := range th.Comments {
			if strings.Contains(cm.Body, "live smoke") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("draft comment not found on server after AddReviewThread")
	}
	t.Logf("draft comment confirmed on server (pending count via readback)")
}

// TestLiveSubmitReview verifies the real submit path: ensure -> add draft ->
// submit (COMMENT, the only event allowed on your own PR). Leaves a published
// review on the throwaway PR (submit can't be undone), so it needs an explicit
// opt-in beyond the normal live suite.
func TestLiveSubmitReview(t *testing.T) {
	if os.Getenv("LAZYPR_LIVE") != "1" || os.Getenv("LAZYPR_LIVE_PUBLISH") != "1" {
		t.Skip("set LAZYPR_LIVE=1 and LAZYPR_LIVE_PUBLISH=1 to run the publishing submit test")
	}
	const (
		prID    = "PR_kwDOKvQieM73BnQS"
		headOID = "54ac02a08808b1de8b326f9110f2111aef69fd2a"
		path    = "app/index.js"
		line    = 403
	)
	repo := forge.Repo{Owner: "DreadPirateRob", Name: "stratoptic"}
	runner := ghcli.New(30*time.Second, func(ghcli.CommandLogEntry) {})
	c := New(runner)
	ctx := context.Background()

	if pre, err := c.PRDetail(ctx, repo, 4); err != nil {
		t.Fatalf("preflight PRDetail: %v", err)
	} else if pre.PendingReview != nil {
		t.Skip("PR already has a pending review; skipping")
	}

	reviewID, err := c.EnsurePendingReview(ctx, prID, headOID)
	if err != nil {
		t.Fatalf("EnsurePendingReview: %v", err)
	}
	in := forge.AddThreadInput{Repo: repo, PRID: prID, ReviewID: reviewID, Path: path, Line: line, Side: "RIGHT", Body: "lazypr submit smoke — safe to ignore", SubjectType: "LINE"}
	if err := c.AddReviewThread(ctx, in); err != nil {
		t.Fatalf("AddReviewThread: %v", err)
	}
	if err := c.SubmitReview(ctx, reviewID, forge.ReviewEvent("COMMENT"), "lazypr review smoke — safe to ignore"); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	after, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if after.PendingReview != nil {
		t.Fatal("review should no longer be PENDING after submit")
	}
	t.Log("review submitted; pending review cleared on server")
}

// TestLiveEditDelete verifies UpdateComment + DeleteComment (pending path) on a
// throwaway draft, then discards. Self-cleaning; gated behind LAZYPR_LIVE=1.
func TestLiveEditDelete(t *testing.T) {
	if os.Getenv("LAZYPR_LIVE") != "1" {
		t.Skip("set LAZYPR_LIVE=1 to run the live write test")
	}
	const (
		prID    = "PR_kwDOKvQieM73BnQS"
		headOID = "54ac02a08808b1de8b326f9110f2111aef69fd2a"
		path    = "app/index.js"
		line    = 403
	)
	repo := forge.Repo{Owner: "DreadPirateRob", Name: "stratoptic"}
	runner := ghcli.New(30*time.Second, func(ghcli.CommandLogEntry) {})
	c := New(runner)
	ctx := context.Background()

	if pre, err := c.PRDetail(ctx, repo, 4); err != nil {
		t.Fatalf("preflight: %v", err)
	} else if pre.PendingReview != nil {
		t.Skip("PR already has a pending review; skipping")
	}

	reviewID, err := c.EnsurePendingReview(ctx, prID, headOID)
	if err != nil {
		t.Fatalf("EnsurePendingReview: %v", err)
	}
	defer func() { _ = c.DiscardReview(ctx, reviewID) }()

	in := forge.AddThreadInput{Repo: repo, PRID: prID, ReviewID: reviewID, Path: path, Line: line, Side: "RIGHT", Body: "editdelete smoke v1", SubjectType: "LINE"}
	if err := c.AddReviewThread(ctx, in); err != nil {
		t.Fatalf("AddReviewThread: %v", err)
	}

	findDraft := func(pr domain.PRDetail, want string) (string, bool) {
		if pr.PendingReview != nil {
			for _, cm := range pr.PendingReview.Comments {
				if strings.Contains(cm.Body, want) {
					return cm.ID, true
				}
			}
		}
		for _, th := range pr.Threads {
			for _, cm := range th.Comments {
				if strings.Contains(cm.Body, want) {
					return cm.ID, true
				}
			}
		}
		return "", false
	}

	pr, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	commentID, ok := findDraft(pr, "editdelete smoke v1")
	if !ok {
		t.Fatal("draft comment not found after add")
	}
	if err := c.UpdateComment(ctx, commentID, "editdelete smoke v2"); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	pr2, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("readback after edit: %v", err)
	}
	if _, ok := findDraft(pr2, "editdelete smoke v2"); !ok {
		t.Fatal("edited body not found after UpdateComment")
	}
	if err := c.DeleteComment(ctx, repo, forge.CommentRef{ID: commentID, Pending: true}); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	pr3, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("readback after delete: %v", err)
	}
	if _, ok := findDraft(pr3, "editdelete smoke"); ok {
		t.Fatal("draft still present after DeleteComment")
	}
	t.Log("edit + delete (pending path) verified on server; discarding review")
}

// TestLiveThreadReply verifies AddThreadReply produces a PENDING (batched) draft
// reply on an existing thread, then discards. Self-cleaning; LAZYPR_LIVE=1.
func TestLiveThreadReply(t *testing.T) {
	if os.Getenv("LAZYPR_LIVE") != "1" {
		t.Skip("set LAZYPR_LIVE=1 to run the live write test")
	}
	const (
		prID    = "PR_kwDOKvQieM73BnQS"
		headOID = "54ac02a08808b1de8b326f9110f2111aef69fd2a"
	)
	repo := forge.Repo{Owner: "DreadPirateRob", Name: "stratoptic"}
	runner := ghcli.New(30*time.Second, func(ghcli.CommandLogEntry) {})
	c := New(runner)
	ctx := context.Background()

	pre, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if pre.PendingReview != nil {
		t.Skip("PR already has a pending review; skipping")
	}
	threadID := ""
	for _, th := range pre.Threads {
		threadID = th.ID
		break
	}
	if threadID == "" {
		t.Skip("no existing thread on PR #4 to reply to")
	}

	reviewID, err := c.EnsurePendingReview(ctx, prID, headOID)
	if err != nil {
		t.Fatalf("EnsurePendingReview: %v", err)
	}
	defer func() { _ = c.DiscardReview(ctx, reviewID) }()

	if err := c.AddThreadReply(ctx, reviewID, threadID, "lazypr reply smoke — safe to ignore"); err != nil {
		t.Fatalf("AddThreadReply: %v", err)
	}
	after, err := c.PRDetail(ctx, repo, 4)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	found := false
	for _, th := range after.Threads {
		if th.ID != threadID {
			continue
		}
		for _, cm := range th.Comments {
			if strings.Contains(cm.Body, "reply smoke") && cm.State == "PENDING" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("batched reply not found as a PENDING draft under the thread")
	}
	t.Log("batched draft reply verified PENDING on server; discarding review")
}
