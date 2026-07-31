package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

var ctx = context.Background()

// ─── compile-time interface check ────────────────────────────────────────────

var _ forge.Forge = (*fake.Fake)(nil)

// ─── ResolveRepo ─────────────────────────────────────────────────────────────

func TestResolveRepo_Success(t *testing.T) {
	f := fake.New()
	f.Repo = forge.Repo{Owner: "acme", Name: "widget"}

	got, err := f.ResolveRepo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != f.Repo {
		t.Fatalf("got %+v, want %+v", got, f.Repo)
	}
}

func TestResolveRepo_ForcedError(t *testing.T) {
	f := fake.New()
	want := errors.New("no remote")
	f.ResolveRepoErr = want

	_, err := f.ResolveRepo(ctx)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// ─── ListPRs ─────────────────────────────────────────────────────────────────

func TestListPRs_PerFilter(t *testing.T) {
	f := fake.New()
	mine := []domain.PRSummary{{Number: 1, Title: "mine"}}
	all := []domain.PRSummary{{Number: 2, Title: "all-a"}, {Number: 3, Title: "all-b"}}
	f.PRSummaries[forge.FilterMine] = mine
	f.PRSummaries[forge.FilterAllOpen] = all

	repo := forge.Repo{Owner: "o", Name: "r"}

	got, err := f.ListPRs(ctx, repo, forge.FilterMine)
	if err != nil || len(got) != 1 || got[0].Number != 1 {
		t.Fatalf("FilterMine: got %+v err %v", got, err)
	}

	got, err = f.ListPRs(ctx, repo, forge.FilterAllOpen)
	if err != nil || len(got) != 2 {
		t.Fatalf("FilterAllOpen: got %+v err %v", got, err)
	}

	// Unregistered filter → nil slice, no error.
	got, err = f.ListPRs(ctx, repo, forge.FilterReviewRequested)
	if err != nil || got != nil {
		t.Fatalf("FilterReviewRequested (empty): got %+v err %v", got, err)
	}
}

func TestListPRs_ForcedError(t *testing.T) {
	f := fake.New()
	want := errors.New("rate limit")
	f.ListPRsErrs[forge.FilterAllOpen] = want

	_, err := f.ListPRs(ctx, forge.Repo{}, forge.FilterAllOpen)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// ─── PRDetail ────────────────────────────────────────────────────────────────

func TestPRDetail_Success(t *testing.T) {
	f := fake.New()
	detail := domain.PRDetail{
		Number: 42,
		Title:  "great PR",
		Author: "alice",
		Files: []domain.ChangedFile{
			{Path: "main.go", Additions: 10, ViewerViewedState: "UNVIEWED"},
		},
	}
	f.PRDetails[42] = detail

	got, err := f.PRDetail(ctx, forge.Repo{}, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Number != 42 || got.Title != "great PR" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "main.go" {
		t.Fatalf("files: got %+v", got.Files)
	}
}

func TestPRDetail_UnregisteredNumber(t *testing.T) {
	f := fake.New()
	_, err := f.PRDetail(ctx, forge.Repo{}, 99)
	if err == nil {
		t.Fatal("expected error for unregistered PR, got nil")
	}
}

func TestPRDetail_ForcedError(t *testing.T) {
	f := fake.New()
	want := errors.New("api error")
	f.PRDetailErrs[7] = want

	_, err := f.PRDetail(ctx, forge.Repo{}, 7)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// ─── Diff ─────────────────────────────────────────────────────────────────────

func TestDiff_Success(t *testing.T) {
	f := fake.New()
	f.DiffTexts[5] = "--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n-old\n+new\n"

	got, err := f.Diff(ctx, forge.Repo{}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty diff")
	}
}

func TestDiff_UnregisteredNumber(t *testing.T) {
	f := fake.New()
	_, err := f.Diff(ctx, forge.Repo{}, 99)
	if err == nil {
		t.Fatal("expected error for unregistered PR diff, got nil")
	}
}

func TestDiff_ForcedError(t *testing.T) {
	f := fake.New()
	want := errors.New("network timeout")
	f.DiffErrs[3] = want

	_, err := f.Diff(ctx, forge.Repo{}, 3)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// ─── ThreadComments ───────────────────────────────────────────────────────────

func TestThreadComments_SinglePage(t *testing.T) {
	f := fake.New()
	now := time.Now()
	comments := []domain.Comment{
		{ID: "c1", Body: "nice", Author: "bob", CreatedAt: now},
		{ID: "c2", Body: "agreed", Author: "carol", CreatedAt: now},
	}
	f.AddThreadPage("thread-1", "", comments, "")

	got, cursor, err := f.ThreadComments(ctx, "thread-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor != "" {
		t.Fatalf("expected empty cursor, got %q", cursor)
	}
	if len(got) != 2 {
		t.Fatalf("got %d comments, want 2", len(got))
	}
}

func TestThreadComments_MultiPage(t *testing.T) {
	f := fake.New()
	now := time.Now()
	page1 := []domain.Comment{{ID: "c1", CreatedAt: now}}
	page2 := []domain.Comment{{ID: "c2", CreatedAt: now}, {ID: "c3", CreatedAt: now}}

	f.AddThreadPage("thread-x", "", page1, "cursor-abc")
	f.AddThreadPage("thread-x", "cursor-abc", page2, "")

	// First page
	got1, cur1, err := f.ThreadComments(ctx, "thread-x", "")
	if err != nil || len(got1) != 1 || cur1 != "cursor-abc" {
		t.Fatalf("page 1: got %v cursor %q err %v", got1, cur1, err)
	}

	// Second page
	got2, cur2, err := f.ThreadComments(ctx, "thread-x", "cursor-abc")
	if err != nil || len(got2) != 2 || cur2 != "" {
		t.Fatalf("page 2: got %v cursor %q err %v", got2, cur2, err)
	}
}

func TestThreadComments_UnknownThread(t *testing.T) {
	f := fake.New()
	got, cursor, err := f.ThreadComments(ctx, "does-not-exist", "")
	if err != nil || got != nil || cursor != "" {
		t.Fatalf("unknown thread: got %v cursor %q err %v", got, cursor, err)
	}
}

// ─── MarkFileViewed ──────────────────────────────────────────────────────────

func TestMarkFileViewed_RecordsCall(t *testing.T) {
	f := fake.New()

	if err := f.MarkFileViewed(ctx, "PR_01", "main.go", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.MarkFileViewed(ctx, "PR_01", "lib.go", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(f.ViewedToggles) != 2 {
		t.Fatalf("got %d toggles, want 2", len(f.ViewedToggles))
	}
	if f.ViewedToggles[0] != (fake.ViewedToggle{PRID: "PR_01", Path: "main.go", Viewed: true}) {
		t.Errorf("toggle[0]: %+v", f.ViewedToggles[0])
	}
	if f.ViewedToggles[1] != (fake.ViewedToggle{PRID: "PR_01", Path: "lib.go", Viewed: false}) {
		t.Errorf("toggle[1]: %+v", f.ViewedToggles[1])
	}
}

func TestMarkFileViewed_ForcedErrorPerFile(t *testing.T) {
	f := fake.New()
	want := errors.New("mutation failed")
	f.MarkViewedErrs["PR_01:main.go"] = want

	err := f.MarkFileViewed(ctx, "PR_01", "main.go", true)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	// Call was still recorded before the error was returned (mirrors optimistic UI).
	if len(f.ViewedToggles) != 1 {
		t.Fatalf("expected 1 recorded toggle even on error, got %d", len(f.ViewedToggles))
	}

	// A different file on the same PR must succeed.
	if err := f.MarkFileViewed(ctx, "PR_01", "other.go", true); err != nil {
		t.Fatalf("other.go should succeed: %v", err)
	}
}

func TestMarkFileViewed_WildcardError(t *testing.T) {
	f := fake.New()
	want := errors.New("service unavailable")
	f.MarkViewedErrs["*"] = want

	for _, path := range []string{"a.go", "b.go", "c.go"} {
		if err := f.MarkFileViewed(ctx, "PR_02", path, true); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", path, err, want)
		}
	}
}

// ─── Later-phase methods all return ErrUnsupportedInPhase1 ───────────────────

func TestLaterPhaseMethods_AllReturnUnsupported(t *testing.T) {
	f := fake.New()
	repo := forge.Repo{Owner: "o", Name: "r"}

	cases := []struct {
		name string
		fn   func() error
	}{
		{"ReplyToThread", func() error {
			return f.ReplyToThread(ctx, repo, forge.CommentRef{}, "body")
		}},
		{"AddIssueComment", func() error {
			return f.AddIssueComment(ctx, repo, 1, "body")
		}},
		{"Merge", func() error {
			return f.Merge(ctx, repo, 1, forge.MergeOpts{})
		}},
		{"Checkout", func() error {
			return f.Checkout(ctx, 1)
		}},
		{"FileContent", func() error {
			_, err := f.FileContent(ctx, repo, "main.go", "HEAD")
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if !errors.Is(err, ghcli.ErrUnsupportedInPhase1) {
				t.Fatalf("%s: got %v, want ErrUnsupportedInPhase1", tc.name, err)
			}
		})
	}
}

// ─── Phase-2 authoring fake records calls ────────────────────────────────────

func TestFakeAuthoringRecordsCalls(t *testing.T) {
	t.Run("EnsurePendingReview_default_id", func(t *testing.T) {
		f := fake.New()
		id, err := f.EnsurePendingReview(ctx, "PR_1", "sha1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "R_fake" {
			t.Errorf("got id %q, want R_fake", id)
		}
		if len(f.EnsurePendingCalls) != 1 || f.EnsurePendingCalls[0].PRID != "PR_1" {
			t.Errorf("call not recorded: %+v", f.EnsurePendingCalls)
		}
	})

	t.Run("EnsurePendingReview_custom_id", func(t *testing.T) {
		f := fake.New()
		f.EnsuredReviewID = "R_custom"
		id, err := f.EnsurePendingReview(ctx, "PR_2", "sha2")
		if err != nil || id != "R_custom" {
			t.Fatalf("got id=%q err=%v, want id=R_custom err=nil", id, err)
		}
	})

	t.Run("EnsurePendingReview_forced_error", func(t *testing.T) {
		f := fake.New()
		want := errors.New("ensure failed")
		f.EnsurePendingErr = want
		_, err := f.EnsurePendingReview(ctx, "PR_3", "sha3")
		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
	})

	t.Run("AddReviewThread_records_and_errors", func(t *testing.T) {
		f := fake.New()
		in := forge.AddThreadInput{ReviewID: "rev1", Body: "hello", SubjectType: "LINE"}
		if err := f.AddReviewThread(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.AddThreadCalls) != 1 || f.AddThreadCalls[0].ReviewID != "rev1" {
			t.Errorf("call not recorded: %+v", f.AddThreadCalls)
		}
		want := errors.New("thread err")
		f.AddThreadErr = want
		if err := f.AddReviewThread(ctx, in); !errors.Is(err, want) {
			t.Errorf("forced error: got %v", err)
		}
	})

	t.Run("UpdateComment_records_and_errors", func(t *testing.T) {
		f := fake.New()
		if err := f.UpdateComment(ctx, "C_1", "new body"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.UpdateCommentCalls) != 1 || f.UpdateCommentCalls[0].ID != "C_1" || f.UpdateCommentCalls[0].Body != "new body" {
			t.Errorf("call not recorded: %+v", f.UpdateCommentCalls)
		}
		want := errors.New("update err")
		f.UpdateCommentErr = want
		if err := f.UpdateComment(ctx, "C_1", "x"); !errors.Is(err, want) {
			t.Errorf("forced error: got %v", err)
		}
	})

	t.Run("DeleteComment_records_and_errors", func(t *testing.T) {
		f := fake.New()
		ref := forge.CommentRef{ID: "C_2", Pending: true}
		if err := f.DeleteComment(ctx, forge.Repo{}, ref); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.DeleteCommentCalls) != 1 || f.DeleteCommentCalls[0].ID != "C_2" {
			t.Errorf("call not recorded: %+v", f.DeleteCommentCalls)
		}
		want := errors.New("delete err")
		f.DeleteCommentErr = want
		if err := f.DeleteComment(ctx, forge.Repo{}, ref); !errors.Is(err, want) {
			t.Errorf("forced error: got %v", err)
		}
	})

	t.Run("SubmitReview_records_and_errors", func(t *testing.T) {
		f := fake.New()
		if err := f.SubmitReview(ctx, "rev1", forge.ReviewEvent("APPROVE"), "lgtm"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.SubmitReviewCalls) != 1 || f.SubmitReviewCalls[0].ReviewID != "rev1" || f.SubmitReviewCalls[0].Event != "APPROVE" {
			t.Errorf("call not recorded: %+v", f.SubmitReviewCalls)
		}
		want := errors.New("submit err")
		f.SubmitReviewErr = want
		if err := f.SubmitReview(ctx, "rev1", "APPROVE", ""); !errors.Is(err, want) {
			t.Errorf("forced error: got %v", err)
		}
	})

	t.Run("DiscardReview_records_and_errors", func(t *testing.T) {
		f := fake.New()
		if err := f.DiscardReview(ctx, "rev2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.DiscardCalls) != 1 || f.DiscardCalls[0] != "rev2" {
			t.Errorf("call not recorded: %+v", f.DiscardCalls)
		}
		want := errors.New("discard err")
		f.DiscardErr = want
		if err := f.DiscardReview(ctx, "rev2"); !errors.Is(err, want) {
			t.Errorf("forced error: got %v", err)
		}
	})
}

func TestSearchPRs(t *testing.T) {
	f := fake.New()
	f.SearchPRResults["is:merged"] = []domain.PRSummary{{Number: 9, Title: "found", State: "MERGED"}}

	got, err := f.SearchPRs(ctx, forge.Repo{}, "is:merged")
	if err != nil || len(got) != 1 || got[0].Number != 9 {
		t.Fatalf("registered query: got %+v err %v", got, err)
	}

	got, err = f.SearchPRs(ctx, forge.Repo{}, "nope")
	if err != nil || got != nil {
		t.Fatalf("unregistered query: got %+v err %v", got, err)
	}

	f.SearchPRErr = errors.New("rate limit")
	if _, err := f.SearchPRs(ctx, forge.Repo{}, "is:merged"); !errors.Is(err, f.SearchPRErr) {
		t.Fatalf("forced error: got %v", err)
	}
}
