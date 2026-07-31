// Package fake provides an in-memory Forge implementation for tests.
// Phase 1-supported methods (ResolveRepo, ListPRs, PRDetail, Diff,
// ThreadComments, MarkFileViewed) return fixture data pre-loaded by the
// caller. Every later-phase method returns ghcli.ErrUnsupportedInPhase1.
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// ViewedToggle records a single MarkFileViewed call for later assertions.
type ViewedToggle struct {
	PRID   string
	Path   string
	Viewed bool
}

// ResolveCall records a single ResolveThread call for later assertions.
type ResolveCall struct {
	ThreadID string
	Resolved bool
}

// EnsurePendingCall records a single EnsurePendingReview call.
type EnsurePendingCall struct {
	PRID    string
	HeadOID string
}

// UpdateCommentCall records a single UpdateComment call.
type UpdateCommentCall struct {
	ID   string
	Body string
}

// SubmitReviewCall records a single SubmitReview call.
type SubmitReviewCall struct {
	ReviewID string
	Event    forge.ReviewEvent
	Body     string
}

// AddThreadReplyCall records a single AddThreadReply call.
type AddThreadReplyCall struct {
	ReviewID string
	ThreadID string
	Body     string
}

// threadPage is one page of ThreadComments results.
type threadPage struct {
	Comments []domain.Comment
	// NextCursor is the cursor to return; empty means no more pages.
	NextCursor string
}

// Fake is a fully controllable, thread-safe Forge implementation.
// Zero value is usable; all maps are lazily initialised on first write.
//
// Callers drive it by assigning to the exported fields before tests run.
// Forced-failure fields accept pre-configured errors that the corresponding
// methods return verbatim, letting callers test rollback paths.
type Fake struct {
	mu sync.Mutex

	// Fixture data — set these before the test.
	Repo            forge.Repo
	PRSummaries     map[forge.PRFilter][]domain.PRSummary // keyed by filter
	PRDetails       map[int]domain.PRDetail               // keyed by PR number
	DiffTexts       map[int]string                        // keyed by PR number
	PRCommits       map[int][]domain.CommitSummary        // keyed by PR number
	SearchPRResults map[string][]domain.PRSummary         // keyed by search query
	CommitDiffs     map[string]string                     // keyed by commit SHA

	// ThreadComments pages: outer key = threadID, inner key = after-cursor.
	// An empty after-cursor string represents the first page.
	ThreadPages map[string]map[string]threadPage

	// Forced failures — set to a non-nil error to make that method fail.
	ResolveRepoErr error
	ListPRsErrs    map[forge.PRFilter]error
	PRDetailErrs   map[int]error
	DiffErrs       map[int]error
	CommitErrs     map[int]error
	SearchPRErr    error
	CommitDiffErr  error
	// MarkViewedErrs keys are "<prID>:<path>"; use "*" to force all.
	MarkViewedErrs map[string]error

	// Recorded calls — inspected by tests after actions complete.
	ViewedToggles    []ViewedToggle
	ResolveCalls     []ResolveCall
	ResolveThreadErr error

	// Phase 2 authoring — recorded calls and configurable errors.
	EnsurePendingCalls []EnsurePendingCall
	EnsuredReviewID    string
	EnsurePendingErr   error

	AddThreadCalls []forge.AddThreadInput
	AddThreadErr   error

	UpdateCommentCalls []UpdateCommentCall
	UpdateCommentErr   error

	DeleteCommentCalls []forge.CommentRef
	DeleteCommentErr   error

	SubmitReviewCalls []SubmitReviewCall
	SubmitReviewErr   error

	DiscardCalls []string
	DiscardErr   error

	AddThreadReplyCalls []AddThreadReplyCall
	AddThreadReplyErr   error
}

// New returns a Fake with all maps initialised.
func New() *Fake {
	return &Fake{
		PRSummaries:     make(map[forge.PRFilter][]domain.PRSummary),
		PRDetails:       make(map[int]domain.PRDetail),
		DiffTexts:       make(map[int]string),
		PRCommits:       make(map[int][]domain.CommitSummary),
		SearchPRResults: make(map[string][]domain.PRSummary),
		CommitDiffs:     make(map[string]string),
		ThreadPages:     make(map[string]map[string]threadPage),
		ListPRsErrs:     make(map[forge.PRFilter]error),
		PRDetailErrs:    make(map[int]error),
		DiffErrs:        make(map[int]error),
		CommitErrs:      make(map[int]error),
		MarkViewedErrs:  make(map[string]error),
	}
}

// AddThreadPage registers one page of comments for a thread.
// Pass an empty after string for the first page.
// Pass a non-empty nextCursor to indicate there is another page.
func (f *Fake) AddThreadPage(threadID, after string, comments []domain.Comment, nextCursor string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ThreadPages == nil {
		f.ThreadPages = make(map[string]map[string]threadPage)
	}
	if f.ThreadPages[threadID] == nil {
		f.ThreadPages[threadID] = make(map[string]threadPage)
	}
	f.ThreadPages[threadID][after] = threadPage{Comments: comments, NextCursor: nextCursor}
}

// ─── Phase 1 methods ────────────────────────────────────────────────────────

// ResolveRepo returns f.Repo, or f.ResolveRepoErr when set.
func (f *Fake) ResolveRepo(_ context.Context) (forge.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ResolveRepoErr != nil {
		return forge.Repo{}, f.ResolveRepoErr
	}
	return f.Repo, nil
}

// ListPRs returns the summaries registered for the given filter, or the
// forced error for that filter when set.
func (f *Fake) ListPRs(_ context.Context, _ forge.Repo, filter forge.PRFilter) ([]domain.PRSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListPRsErrs != nil {
		if err := f.ListPRsErrs[filter]; err != nil {
			return nil, err
		}
	}
	if f.PRSummaries != nil {
		if summaries, ok := f.PRSummaries[filter]; ok {
			return summaries, nil
		}
	}
	return nil, nil
}

// SearchPRs returns the summaries registered for the given query, or the
// forced SearchPRErr when set.
func (f *Fake) SearchPRs(_ context.Context, _ forge.Repo, query string) ([]domain.PRSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SearchPRErr != nil {
		return nil, f.SearchPRErr
	}
	if f.SearchPRResults != nil {
		if summaries, ok := f.SearchPRResults[query]; ok {
			return summaries, nil
		}
	}
	return nil, nil
}

// PRDetail returns the detail registered for the given PR number, or the
// forced error for that number when set.
func (f *Fake) PRDetail(_ context.Context, _ forge.Repo, number int) (domain.PRDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.PRDetailErrs != nil {
		if err := f.PRDetailErrs[number]; err != nil {
			return domain.PRDetail{}, err
		}
	}
	if f.PRDetails != nil {
		if detail, ok := f.PRDetails[number]; ok {
			return detail, nil
		}
	}
	return domain.PRDetail{}, fmt.Errorf("fake: no PRDetail registered for PR #%d", number)
}

// Diff returns the diff text registered for the given PR number, or the
// forced error for that number when set.
func (f *Fake) Diff(_ context.Context, _ forge.Repo, number int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DiffErrs != nil {
		if err := f.DiffErrs[number]; err != nil {
			return "", err
		}
	}
	if f.DiffTexts != nil {
		if text, ok := f.DiffTexts[number]; ok {
			return text, nil
		}
	}
	return "", fmt.Errorf("fake: no Diff registered for PR #%d", number)
}

// CommitDiff returns the diff registered for the given SHA, or the forced
// CommitDiffErr when set.
func (f *Fake) CommitDiff(_ context.Context, _ forge.Repo, sha string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CommitDiffErr != nil {
		return "", f.CommitDiffErr
	}
	if f.CommitDiffs != nil {
		if text, ok := f.CommitDiffs[sha]; ok {
			return text, nil
		}
	}
	return "", nil
}

// Commits returns the commits registered for the given PR number, or the
// forced error for that number when set.
func (f *Fake) Commits(_ context.Context, _ forge.Repo, number int) ([]domain.CommitSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CommitErrs != nil {
		if err := f.CommitErrs[number]; err != nil {
			return nil, err
		}
	}
	if f.PRCommits != nil {
		if commits, ok := f.PRCommits[number]; ok {
			return commits, nil
		}
	}
	return nil, nil
}

// ThreadComments returns the page of comments registered for threadID + after
// cursor. An unknown threadID/after pair returns an empty result with no error
// (matches real behaviour when a thread has only one page).
func (f *Fake) ThreadComments(_ context.Context, threadID, after string) ([]domain.Comment, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ThreadPages != nil {
		if pages, ok := f.ThreadPages[threadID]; ok {
			if page, ok := pages[after]; ok {
				return page.Comments, page.NextCursor, nil
			}
		}
	}
	return nil, "", nil
}

// MarkFileViewed records the call and returns a forced error when configured.
// Key lookup order: "<prID>:<path>", then "*" (wildcard for all).
func (f *Fake) MarkFileViewed(_ context.Context, prID, path string, viewed bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Record call before returning any error (mirrors optimistic-UI ordering).
	f.ViewedToggles = append(f.ViewedToggles, ViewedToggle{PRID: prID, Path: path, Viewed: viewed})
	if f.MarkViewedErrs != nil {
		key := prID + ":" + path
		if err := f.MarkViewedErrs[key]; err != nil {
			return err
		}
		if err := f.MarkViewedErrs["*"]; err != nil {
			return err
		}
	}
	return nil
}

// ─── Phase 2 authoring methods ───────────────────────────────────────────────

func (f *Fake) EnsurePendingReview(_ context.Context, prID, headOID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EnsurePendingCalls = append(f.EnsurePendingCalls, EnsurePendingCall{PRID: prID, HeadOID: headOID})
	if f.EnsurePendingErr != nil {
		return "", f.EnsurePendingErr
	}
	id := f.EnsuredReviewID
	if id == "" {
		id = "R_fake"
	}
	return id, nil
}

func (f *Fake) AddReviewThread(_ context.Context, in forge.AddThreadInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AddThreadCalls = append(f.AddThreadCalls, in)
	return f.AddThreadErr
}

func (f *Fake) UpdateComment(_ context.Context, commentID, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.UpdateCommentCalls = append(f.UpdateCommentCalls, UpdateCommentCall{ID: commentID, Body: body})
	return f.UpdateCommentErr
}

func (f *Fake) DeleteComment(_ context.Context, _ forge.Repo, ref forge.CommentRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DeleteCommentCalls = append(f.DeleteCommentCalls, ref)
	return f.DeleteCommentErr
}

func (f *Fake) SubmitReview(_ context.Context, reviewID string, event forge.ReviewEvent, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SubmitReviewCalls = append(f.SubmitReviewCalls, SubmitReviewCall{ReviewID: reviewID, Event: event, Body: body})
	return f.SubmitReviewErr
}

func (f *Fake) DiscardReview(_ context.Context, reviewID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DiscardCalls = append(f.DiscardCalls, reviewID)
	return f.DiscardErr
}

func (f *Fake) ReplyToThread(_ context.Context, _ forge.Repo, _ forge.CommentRef, _ string) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (f *Fake) AddThreadReply(_ context.Context, reviewID, threadID, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AddThreadReplyCalls = append(f.AddThreadReplyCalls, AddThreadReplyCall{ReviewID: reviewID, ThreadID: threadID, Body: body})
	return f.AddThreadReplyErr
}

func (f *Fake) ResolveThread(_ context.Context, threadID string, resolved bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResolveCalls = append(f.ResolveCalls, ResolveCall{ThreadID: threadID, Resolved: resolved})
	return f.ResolveThreadErr
}

func (f *Fake) AddIssueComment(_ context.Context, _ forge.Repo, _ int, _ string) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (f *Fake) Merge(_ context.Context, _ forge.Repo, _ int, _ forge.MergeOpts) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (f *Fake) Checkout(_ context.Context, _ int) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (f *Fake) FileContent(_ context.Context, _ forge.Repo, _, _ string) ([]byte, error) {
	return nil, ghcli.ErrUnsupportedInPhase1
}
