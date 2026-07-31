package github

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// ─── Fake runner ──────────────────────────────────────────────────────────────

type fakeRunner struct {
	calls [][]string
	fn    func(args []string) ([]byte, error)
}

func newFake(fn func(args []string) ([]byte, error)) *fakeRunner {
	return &fakeRunner{fn: fn}
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{}, args...))
	return f.fn(args)
}

func (f *fakeRunner) RunJSON(_ context.Context, out any, args ...string) error {
	b, err := f.Run(context.Background(), args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// containsArg reports whether any element of args contains substr.
func containsArg(args []string, substr string) bool {
	for _, a := range args {
		if strings.Contains(a, substr) {
			return true
		}
	}
	return false
}

// argValue returns the value after "prefix" in the args slice, or "".
func argValue(args []string, prefix string) string {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
	}
	return ""
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

// gqlWrap wraps a value in {"data": ...} as returned by gh api graphql.
func gqlWrap(data any) []byte {
	b, _ := json.Marshal(map[string]any{"data": data})
	return b
}

// minimalPR returns a map[string]any representing the minimum valid
// pullRequest data for a mega-query response.
func minimalPR() map[string]any {
	return map[string]any{
		"id": "PR_001", "number": 1, "title": "Test PR", "body": "body",
		"state": "OPEN", "isDraft": false,
		"url":         "https://github.com/o/r/pull/1",
		"createdAt":   "2024-01-01T00:00:00Z",
		"updatedAt":   "2024-01-01T00:00:00Z",
		"author":      map[string]any{"login": "alice"},
		"baseRefName": "main", "headRefName": "fix", "headRefOid": "abc",
		"additions": 1, "deletions": 0, "changedFiles": 1,
		"mergeable": "MERGEABLE", "reviewDecision": "",
		"labels":         map[string]any{"nodes": []any{}},
		"latestReviews":  map[string]any{"nodes": []any{}},
		"reviews":        map[string]any{"nodes": []any{}},
		"reviewRequests": map[string]any{"nodes": []any{}},
		"commits":        map[string]any{"nodes": []any{}},
		"files": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			"nodes":    []any{},
		},
		"reviewThreads": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			"nodes":    []any{},
		},
		"comments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			"nodes":    []any{},
		},
	}
}

// megaQueryResp builds a gqlWrap'd mega-query response with controllable files pagination.
func megaQueryResp(numFiles int, hasNextFilePage bool, filesCursor string) []byte {
	files := make([]any, numFiles)
	for i := range files {
		files[i] = map[string]any{
			"path":      "src/file" + string(rune('a'+i%26)) + ".go",
			"additions": 1, "deletions": 0, "changeType": "MODIFIED",
			"viewerViewedState": "UNVIEWED",
		}
	}
	pr := minimalPR()
	pr["files"] = map[string]any{
		"pageInfo": map[string]any{"hasNextPage": hasNextFilePage, "endCursor": filesCursor},
		"nodes":    files,
	}
	return gqlWrap(map[string]any{
		"rateLimit":  map[string]any{"remaining": 5000, "resetAt": "2024-01-01T01:00:00Z"},
		"repository": map[string]any{"pullRequest": pr},
	})
}

// filesPageResp builds a gqlWrap'd files-pagination response.
func filesPageResp(numFiles int, hasNext bool, cursor string) []byte {
	files := make([]any, numFiles)
	for i := range files {
		files[i] = map[string]any{
			"path":      "page/file" + string(rune('a'+i%26)) + ".go",
			"additions": 1, "deletions": 0, "changeType": "MODIFIED",
			"viewerViewedState": "UNVIEWED",
		}
	}
	return gqlWrap(map[string]any{
		"repository": map[string]any{
			"pullRequest": map[string]any{
				"files": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": hasNext, "endCursor": cursor},
					"nodes":    files,
				},
			},
		},
	})
}

// prListResp builds a JSON array response for gh pr list.
func prListResp(items []map[string]any) []byte {
	b, _ := json.Marshal(items)
	return b
}

func emptyPRList() []byte { return []byte("[]") }

// ─── Test 1: filter selection ─────────────────────────────────────────────────

func TestListPRsFilterSearchTerm(t *testing.T) {
	cases := []struct {
		filter     forge.PRFilter
		wantSearch string
	}{
		{forge.FilterReviewRequested, "review-requested:@me sort:updated-desc"},
		{forge.FilterMine, "author:@me sort:updated-desc"},
		{forge.FilterAllOpen, "sort:updated-desc"},
	}
	for _, tc := range cases {
		var gotSearch string
		fr := newFake(func(args []string) ([]byte, error) {
			for i, a := range args {
				if a == "--search" && i+1 < len(args) {
					gotSearch = args[i+1]
				}
			}
			return emptyPRList(), nil
		})
		c := New(fr)
		_, err := c.ListPRs(context.Background(), forge.Repo{Owner: "o", Name: "n"}, tc.filter)
		if err != nil {
			t.Fatalf("filter %d: ListPRs error: %v", tc.filter, err)
		}
		if gotSearch != tc.wantSearch {
			t.Errorf("filter %d: --search = %q, want %q", tc.filter, gotSearch, tc.wantSearch)
		}
	}
}

// ─── Test 2: multi-pending review adopts newest ───────────────────────────────

func TestPRDetailAdoptsNewestPendingReview(t *testing.T) {
	older := "2024-01-01T10:00:00Z"
	newer := "2024-01-02T10:00:00Z"

	pr := minimalPR()
	pr["reviews"] = map[string]any{
		"nodes": []any{
			map[string]any{
				"id": "rev_old", "state": "PENDING", "body": "old body",
				"createdAt": older,
				"comments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
					"nodes":    []any{},
				},
			},
			map[string]any{
				"id": "rev_new", "state": "PENDING", "body": "new body",
				"createdAt": newer,
				"comments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
					"nodes": []any{
						map[string]any{
							"id": "c1", "fullDatabaseId": "999",
							"body": "draft comment", "path": "foo.go",
							"line": nil, "startLine": nil,
							"state": "PENDING", "outdated": false,
						},
					},
				},
			},
		},
	}

	fr := newFake(func(args []string) ([]byte, error) {
		return gqlWrap(map[string]any{
			"rateLimit":  map[string]any{"remaining": 5000, "resetAt": "2024-01-01T01:00:00Z"},
			"repository": map[string]any{"pullRequest": pr},
		}), nil
	})
	c := New(fr)
	detail, err := c.PRDetail(context.Background(), forge.Repo{Owner: "o", Name: "n"}, 1)
	if err != nil {
		t.Fatalf("PRDetail error: %v", err)
	}
	if !detail.HasMultiplePendingReviews {
		t.Error("HasMultiplePendingReviews should be true with 2 pending reviews")
	}
	if detail.PendingReview == nil {
		t.Fatal("PendingReview is nil")
	}
	if detail.PendingReview.ID != "rev_new" {
		t.Errorf("adopted review ID = %q, want %q", detail.PendingReview.ID, "rev_new")
	}
	if detail.PendingReviewCount != 1 {
		t.Errorf("PendingReviewCount = %d, want 1", detail.PendingReviewCount)
	}
}

// ─── Test 3: pagination caps at 500 and sets TruncatedFiles ──────────────────

func TestPRDetailFilesPaginationCap(t *testing.T) {
	// mega-query → 100 files, hasNextPage=true, cursor="c1"
	// page queries (c1→c2, c2→c3, c3→c4, c4→c5): each 100 files, hasNextPage=true
	// After 5×100=500 files loop exits (500 is not < 500).
	// pr.Files.PageInfo.HasNextPage is true from the last page → TruncatedFiles=true.

	pageCallCount := 0
	fr := newFake(func(args []string) ([]byte, error) {
		if containsArg(args, "PRDetailFiles") {
			pageCallCount++
			cursor := "c" + string(rune('1'+pageCallCount))
			return filesPageResp(100, true, cursor), nil
		}
		// Initial mega-query
		return megaQueryResp(100, true, "c1"), nil
	})

	c := New(fr)
	detail, err := c.PRDetail(context.Background(), forge.Repo{Owner: "o", Name: "n"}, 42)
	if err != nil {
		t.Fatalf("PRDetail error: %v", err)
	}
	if len(detail.Files) != paginationCap {
		t.Errorf("Files count = %d, want %d", len(detail.Files), paginationCap)
	}
	if !detail.TruncatedFiles {
		t.Error("TruncatedFiles should be true at cap")
	}
	// Should have made exactly 4 extra page calls (100+100+100+100+100=500 then stop)
	if pageCallCount != 4 {
		t.Errorf("page calls = %d, want 4", pageCallCount)
	}
}

// ─── Test 4: timeline merge is chronologically sorted ─────────────────────────

func TestPRDetailTimelineSorted(t *testing.T) {
	t1 := "2024-01-01T08:00:00Z"
	t2 := "2024-01-01T10:00:00Z"
	t3 := "2024-01-01T12:00:00Z"

	pr := minimalPR()
	// Issue comments at t1 and t3 (out of order relative to review at t2)
	pr["comments"] = map[string]any{
		"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
		"nodes": []any{
			map[string]any{
				"id": "ic1", "author": map[string]any{"login": "alice"},
				"body": "comment at t3", "createdAt": t3, "url": "u1",
			},
			map[string]any{
				"id": "ic2", "author": map[string]any{"login": "bob"},
				"body": "comment at t1", "createdAt": t1, "url": "u2",
			},
		},
	}
	// Review at t2
	pr["latestReviews"] = map[string]any{
		"nodes": []any{
			map[string]any{
				"author":      map[string]any{"login": "charlie"},
				"state":       "APPROVED",
				"submittedAt": t2,
				"body":        "lgtm",
			},
		},
	}

	fr := newFake(func(args []string) ([]byte, error) {
		return gqlWrap(map[string]any{
			"rateLimit":  map[string]any{"remaining": 4999, "resetAt": "2024-01-01T01:00:00Z"},
			"repository": map[string]any{"pullRequest": pr},
		}), nil
	})

	c := New(fr)
	detail, err := c.PRDetail(context.Background(), forge.Repo{Owner: "o", Name: "n"}, 1)
	if err != nil {
		t.Fatalf("PRDetail error: %v", err)
	}
	if len(detail.Timeline) != 3 {
		t.Fatalf("timeline length = %d, want 3", len(detail.Timeline))
	}
	for i := 1; i < len(detail.Timeline); i++ {
		if detail.Timeline[i].SortAt.Before(detail.Timeline[i-1].SortAt) {
			t.Errorf("timeline[%d].SortAt (%v) is before timeline[%d].SortAt (%v)",
				i, detail.Timeline[i].SortAt, i-1, detail.Timeline[i-1].SortAt)
		}
	}
	// Verify the expected order: t1(comment) t2(review) t3(comment)
	wantKinds := []string{"comment", "review", "comment"}
	wantAuthors := []string{"bob", "charlie", "alice"}
	for i, item := range detail.Timeline {
		if item.Kind != wantKinds[i] {
			t.Errorf("timeline[%d].Kind = %q, want %q", i, item.Kind, wantKinds[i])
		}
		if item.Author != wantAuthors[i] {
			t.Errorf("timeline[%d].Author = %q, want %q", i, item.Author, wantAuthors[i])
		}
	}
}

// ─── Test 5: MarkFileViewed chooses the correct mutation ─────────────────────

func TestMarkFileViewedMutationChoice(t *testing.T) {
	cases := []struct {
		viewed       bool
		wantFragment string
	}{
		{true, "markFileAsViewed"},
		{false, "unmarkFileAsViewed"},
	}
	for _, tc := range cases {
		var gotQuery string
		fr := newFake(func(args []string) ([]byte, error) {
			gotQuery = argValue(args, "query=")
			return gqlWrap(map[string]any{
				"markFileAsViewed":   map[string]any{"clientMutationId": nil},
				"unmarkFileAsViewed": map[string]any{"clientMutationId": nil},
			}), nil
		})
		c := New(fr)
		err := c.MarkFileViewed(context.Background(), "PR_001", "src/foo.go", tc.viewed)
		if err != nil {
			t.Fatalf("viewed=%v: MarkFileViewed error: %v", tc.viewed, err)
		}
		if !strings.Contains(gotQuery, tc.wantFragment) {
			t.Errorf("viewed=%v: mutation query does not contain %q:\n%s",
				tc.viewed, tc.wantFragment, gotQuery)
		}
	}
}

// ─── Test 6: rollup normalization ─────────────────────────────────────────────

func TestNormalizeRollupFromContexts(t *testing.T) {
	cases := []struct {
		name string
		ctxs []rollupContext
		want string
	}{
		{
			"empty → none",
			nil, "none",
		},
		{
			"all success → success",
			[]rollupContext{
				{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Typename: "StatusContext", State: "SUCCESS"},
			}, "success",
		},
		{
			"failure beats pending",
			[]rollupContext{
				{Typename: "CheckRun", Status: "IN_PROGRESS"},
				{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"},
			}, "failure",
		},
		{
			"pending when not completed",
			[]rollupContext{
				{Typename: "CheckRun", Status: "IN_PROGRESS"},
				{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			}, "pending",
		},
		{
			"SKIPPED counts as success",
			[]rollupContext{
				{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"},
			}, "success",
		},
		{
			"TIMED_OUT is failure",
			[]rollupContext{
				{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "TIMED_OUT"},
			}, "failure",
		},
		{
			"StatusContext ERROR is failure",
			[]rollupContext{
				{Typename: "StatusContext", State: "ERROR"},
			}, "failure",
		},
	}
	for _, tc := range cases {
		got := normalizeRollupFromContexts(tc.ctxs)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ─── Test 7: Phase 2/3 stubs return ErrUnsupportedInPhase1 ──────────────────

func TestPhase2StubsReturnUnsupported(t *testing.T) {
	fr := newFake(func(args []string) ([]byte, error) {
		return nil, nil
	})
	c := New(fr)
	ctx := context.Background()

	errs := []error{
		c.ReplyToThread(ctx, forge.Repo{}, forge.CommentRef{}, "body"),
		c.AddIssueComment(ctx, forge.Repo{}, 1, "body"),
		c.Merge(ctx, forge.Repo{}, 1, forge.MergeOpts{}),
		c.Checkout(ctx, 1),
		func() error { _, e := c.FileContent(ctx, forge.Repo{}, "f", "r"); return e }(),
	}
	for i, err := range errs {
		if err != ghcli.ErrUnsupportedInPhase1 {
			t.Errorf("stub[%d]: got %v, want ErrUnsupportedInPhase1", i, err)
		}
	}
}

// ─── Test 8: GraphQL error in 200 response surfaces as GraphQLError ──────────

func TestRunGQLHandlesGraphQLErrors(t *testing.T) {
	fr := newFake(func(args []string) ([]byte, error) {
		return []byte(`{"errors":[{"message":"something went wrong"}],"data":null}`), nil
	})
	c := New(fr)
	var out any
	err := c.runGQL(context.Background(), &out, queryViewer)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	gqlErr, ok := err.(*ghcli.GraphQLError)
	if !ok {
		t.Fatalf("error type = %T, want *ghcli.GraphQLError", err)
	}
	if len(gqlErr.Messages) == 0 || gqlErr.Messages[0] != "something went wrong" {
		t.Errorf("unexpected messages: %v", gqlErr.Messages)
	}
}

// ─── Test 9: ResolveRepo parses nameWithOwner ─────────────────────────────────

func TestResolveRepo(t *testing.T) {
	fr := newFake(func(args []string) ([]byte, error) {
		return []byte(`{"nameWithOwner":"jesseduffield/lazygit"}`), nil
	})
	c := New(fr)
	repo, err := c.ResolveRepo(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepo: %v", err)
	}
	if repo.Owner != "jesseduffield" || repo.Name != "lazygit" {
		t.Errorf("got %+v, want {jesseduffield lazygit}", repo)
	}
	// Verify the args sent to the runner
	if len(fr.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.calls))
	}
	call := fr.calls[0]
	if !containsArg(call, "nameWithOwner") {
		t.Errorf("call missing nameWithOwner: %v", call)
	}
}

// ─── Test 10: PRDetail decodes fullDatabaseId string integer ─────────────────

func TestPRDetailFullDatabaseIDStringDecode(t *testing.T) {
	line := 42
	pr := minimalPR()
	pr["reviewThreads"] = map[string]any{
		"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
		"nodes": []any{
			map[string]any{
				"id": "TH_001", "isResolved": false, "isOutdated": false,
				"path": "src/main.go", "line": line, "startLine": nil,
				"diffSide": "RIGHT", "startDiffSide": "RIGHT",
				"resolvedBy": nil, "viewerCanResolve": true, "viewerCanReply": true,
				"comments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
					"nodes": []any{
						map[string]any{
							"id": "IC_001", "fullDatabaseId": "3509772848",
							"body": "nice", "state": "SUBMITTED",
							"author":       map[string]any{"login": "reviewer"},
							"createdAt":    "2024-01-01T09:00:00Z",
							"lastEditedAt": nil, "url": "https://github.com/...",
							"viewerDidAuthor": false,
						},
					},
				},
			},
		},
	}
	fr := newFake(func(args []string) ([]byte, error) {
		return gqlWrap(map[string]any{
			"rateLimit":  map[string]any{"remaining": 5000, "resetAt": "2024-01-01T01:00:00Z"},
			"repository": map[string]any{"pullRequest": pr},
		}), nil
	})
	c := New(fr)
	detail, err := c.PRDetail(context.Background(), forge.Repo{Owner: "o", Name: "n"}, 1)
	if err != nil {
		t.Fatalf("PRDetail: %v", err)
	}
	if len(detail.Threads) != 1 {
		t.Fatalf("thread count = %d, want 1", len(detail.Threads))
	}
	th := detail.Threads[0]
	if len(th.Comments) != 1 {
		t.Fatalf("comment count = %d, want 1", len(th.Comments))
	}
	wantID := int64(3509772848)
	if th.Comments[0].FullDatabaseID != wantID {
		t.Errorf("FullDatabaseID = %d, want %d", th.Comments[0].FullDatabaseID, wantID)
	}
}

// ─── Test 11: normalizeRollupStateName ───────────────────────────────────────

func TestNormalizeRollupStateName(t *testing.T) {
	cases := map[string]string{
		"SUCCESS":  "success",
		"FAILURE":  "failure",
		"ERROR":    "failure",
		"PENDING":  "pending",
		"EXPECTED": "pending",
		"":         "none",
		"UNKNOWN":  "none",
	}
	for in, want := range cases {
		if got := normalizeRollupStateName(in); got != want {
			t.Errorf("normalizeRollupStateName(%q) = %q, want %q", in, got, want)
		}
	}
}

// ─── Test 12: buildTimeline skips PENDING reviews ────────────────────────────

func TestBuildTimelineSkipsPendingReviews(t *testing.T) {
	// latestReviews: one submitted (submittedAt set), one PENDING (submittedAt nil)
	submitted := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
	reviews := []latestReviewNode{
		{Author: loginWrapper{"alice"}, State: "APPROVED", SubmittedAt: &submitted, Body: "lgtm"},
		{Author: loginWrapper{"bob"}, State: "PENDING", SubmittedAt: nil, Body: "draft"},
	}
	timeline := buildTimeline(nil, reviews)
	if len(timeline) != 1 {
		t.Fatalf("timeline length = %d, want 1 (PENDING review skipped)", len(timeline))
	}
	if timeline[0].Author != "alice" {
		t.Errorf("unexpected author: %q", timeline[0].Author)
	}
}

// ─── Test 13: Commits command + decode ───────────────────────────────────────

func TestCommits(t *testing.T) {
	fr := newFake(func(args []string) ([]byte, error) {
		return []byte(`{"commits":[{"oid":"abc123","messageHeadline":"first"},{"oid":"def456","messageHeadline":"second"}]}`), nil
	})
	c := New(fr)
	commits, err := c.Commits(context.Background(), forge.Repo{Owner: "o", Name: "n"}, 7)
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("commit count = %d, want 2", len(commits))
	}
	if commits[0].OID != "abc123" || commits[0].MessageHeadline != "first" {
		t.Fatalf("unexpected first commit: %+v", commits[0])
	}
	call := fr.calls[0]
	if !containsArg(call, "pr") || !containsArg(call, "view") || !containsArg(call, "commits") {
		t.Fatalf("unexpected args: %v", call)
	}
}

func TestSearchPRsArgs(t *testing.T) {
	var got []string
	fr := newFake(func(args []string) ([]byte, error) {
		got = append([]string{}, args...)
		return emptyPRList(), nil
	})
	c := New(fr)
	if _, err := c.SearchPRs(context.Background(), forge.Repo{Owner: "o", Name: "n"}, "is:merged label:bug"); err != nil {
		t.Fatalf("SearchPRs error: %v", err)
	}
	adjacent := func(flag, val string) bool {
		for i, a := range got {
			if a == flag && i+1 < len(got) && got[i+1] == val {
				return true
			}
		}
		return false
	}
	if !adjacent("--state", "all") {
		t.Errorf("expected --state all, got %v", got)
	}
	if !adjacent("--limit", "100") {
		t.Errorf("expected --limit 100, got %v", got)
	}
	if !adjacent("--search", "is:merged label:bug sort:updated-desc") {
		t.Errorf("--search wrong, got %v", got)
	}
}

func TestSearchPRsEmptyQuerySkipsGH(t *testing.T) {
	called := false
	fr := newFake(func(args []string) ([]byte, error) {
		called = true
		return emptyPRList(), nil
	})
	c := New(fr)
	prs, err := c.SearchPRs(context.Background(), forge.Repo{Owner: "o", Name: "n"}, "   ")
	if err != nil || prs != nil {
		t.Fatalf("empty query: got %v err %v", prs, err)
	}
	if called {
		t.Error("empty query must not shell out to gh")
	}
}

func TestResolveThreadMutation(t *testing.T) {
	for _, tc := range []struct {
		resolved bool
		want     string
	}{
		{true, "resolveReviewThread"},
		{false, "unresolveReviewThread"},
	} {
		var gotQuery string
		fr := newFake(func(args []string) ([]byte, error) {
			for _, a := range args {
				if strings.HasPrefix(a, "query=") {
					gotQuery = a
				}
			}
			return []byte(`{"data":{}}`), nil
		})
		c := New(fr)
		if err := c.ResolveThread(context.Background(), "t1", tc.resolved); err != nil {
			t.Fatalf("resolved=%v: %v", tc.resolved, err)
		}
		if !strings.Contains(gotQuery, tc.want) {
			t.Errorf("resolved=%v: query %q missing %q", tc.resolved, gotQuery, tc.want)
		}
		if !strings.Contains(gotQuery, "unresolveReviewThread") && !tc.resolved {
			t.Errorf("unresolve should not use resolve mutation")
		}
	}
}

// ─── Test 14: EnsurePendingReview mutation and id extraction ─────────────────

func TestEnsurePendingReviewMutation(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"R1"}}}}`), nil
	})
	c := New(fr)
	id, err := c.EnsurePendingReview(context.Background(), "PR_123", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "R1" {
		t.Errorf("got id %q, want %q", id, "R1")
	}
	if !containsArg(gotArgs, "addPullRequestReview") {
		t.Errorf("query missing addPullRequestReview; args: %v", gotArgs)
	}
}

// ─── Test 15: AddReviewThread builds correct mutation literals ────────────────

func TestAddReviewThreadLineMutation(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	err := c.AddReviewThread(context.Background(), forge.AddThreadInput{
		ReviewID:    "rev1",
		Path:        "main.go",
		Line:        10,
		Side:        "RIGHT",
		Body:        "looks good",
		SubjectType: "LINE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "side:RIGHT") {
		t.Errorf("query missing side:RIGHT; args: %v", gotArgs)
	}
	if !containsArg(gotArgs, "subjectType:LINE") {
		t.Errorf("query missing subjectType:LINE; args: %v", gotArgs)
	}
	if !containsArg(gotArgs, "addPullRequestReviewThread") {
		t.Errorf("query missing addPullRequestReviewThread; args: %v", gotArgs)
	}
}

func TestAddReviewThreadWithStartLine(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	start := 5
	err := c.AddReviewThread(context.Background(), forge.AddThreadInput{
		ReviewID:    "rev1",
		Path:        "main.go",
		Line:        10,
		Side:        "RIGHT",
		StartLine:   &start,
		StartSide:   "LEFT",
		Body:        "multiline",
		SubjectType: "LINE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "startSide:LEFT") {
		t.Errorf("query missing startSide:LEFT; args: %v", gotArgs)
	}
	if !containsArg(gotArgs, "startLine=5") {
		t.Errorf("missing startLine var; args: %v", gotArgs)
	}
}

func TestAddReviewThreadFileMutation(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	err := c.AddReviewThread(context.Background(), forge.AddThreadInput{
		ReviewID:    "rev1",
		Body:        "file comment",
		SubjectType: "FILE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "subjectType:FILE") {
		t.Errorf("query missing subjectType:FILE; args: %v", gotArgs)
	}
}

// ─── Test 16: UpdateComment sends correct mutation ────────────────────────────

func TestUpdateCommentMutation(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	if err := c.UpdateComment(context.Background(), "C_abc", "new body"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "updatePullRequestReviewComment") {
		t.Errorf("query missing updatePullRequestReviewComment; args: %v", gotArgs)
	}
}

// ─── Test 17: DeleteComment — pending uses GraphQL, non-pending uses REST ─────

func TestDeleteCommentPendingGraphQL(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	err := c.DeleteComment(context.Background(),
		forge.Repo{Owner: "owner", Name: "repo"},
		forge.CommentRef{ID: "C_123", Pending: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "deletePullRequestReviewComment") {
		t.Errorf("expected GraphQL deletePullRequestReviewComment; args: %v", gotArgs)
	}
}

func TestDeleteCommentNonPendingREST(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return nil, nil
	})
	c := New(fr)
	err := c.DeleteComment(context.Background(),
		forge.Repo{Owner: "owner", Name: "repo"},
		forge.CommentRef{FullDatabaseID: 42, Pending: false},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "-X") {
		t.Errorf("expected REST -X arg; args: %v", gotArgs)
	}
	if !containsArg(gotArgs, "repos/owner/repo/pulls/comments/42") {
		t.Errorf("expected correct REST path; args: %v", gotArgs)
	}
}

// ─── Test 18: SubmitReview inlines event literal, rejects unknown events ──────

func TestSubmitReviewMutation(t *testing.T) {
	for _, event := range []forge.ReviewEvent{"APPROVE", "REQUEST_CHANGES", "COMMENT"} {
		event := event
		var gotArgs []string
		fr := newFake(func(args []string) ([]byte, error) {
			gotArgs = append([]string{}, args...)
			return []byte(`{"data":{}}`), nil
		})
		c := New(fr)
		if err := c.SubmitReview(context.Background(), "rev1", event, ""); err != nil {
			t.Fatalf("event %s: unexpected error: %v", event, err)
		}
		want := "event:" + string(event)
		if !containsArg(gotArgs, want) {
			t.Errorf("event %s: query missing %q; args: %v", event, want, gotArgs)
		}
		if !containsArg(gotArgs, "submitPullRequestReview") {
			t.Errorf("event %s: query missing submitPullRequestReview", event)
		}
	}
}

func TestSubmitReviewRejectsInvalidEvent(t *testing.T) {
	fr := newFake(func(args []string) ([]byte, error) { return nil, nil })
	c := New(fr)
	err := c.SubmitReview(context.Background(), "rev1", forge.ReviewEvent("INVALID"), "")
	if err == nil {
		t.Fatal("expected error for unknown event, got nil")
	}
}

// ─── Test 19: DiscardReview sends correct mutation ────────────────────────────

func TestDiscardReviewMutation(t *testing.T) {
	var gotArgs []string
	fr := newFake(func(args []string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	if err := c.DiscardReview(context.Background(), "rev_x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsArg(gotArgs, "deletePullRequestReview") {
		t.Errorf("query missing deletePullRequestReview; args: %v", gotArgs)
	}
}

func TestAddThreadReplyBatchedMutation(t *testing.T) {
	var gotQuery string
	fr := newFake(func(args []string) ([]byte, error) {
		for _, a := range args {
			if strings.HasPrefix(a, "query=") {
				gotQuery = a
			}
		}
		return []byte(`{"data":{}}`), nil
	})
	c := New(fr)
	if err := c.AddThreadReply(context.Background(), "R1", "TH1", "reply body"); err != nil {
		t.Fatalf("AddThreadReply: %v", err)
	}
	for _, want := range []string{"addPullRequestReviewThreadReply", "pullRequestReviewId", "pullRequestReviewThreadId"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("reply mutation missing %q in %q", want, gotQuery)
		}
	}
}
