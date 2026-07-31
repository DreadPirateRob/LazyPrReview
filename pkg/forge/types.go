package forge

import (
	"context"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

type Repo struct {
	Owner string
	Name  string
}

type PRFilter int

const (
	FilterReviewRequested PRFilter = iota
	FilterMine
	FilterAllOpen
	FilterSearch
)

type ReviewEvent string

type MergeMethod string

type CommentRef struct {
	ID             string
	FullDatabaseID int64
	Pending        bool
}

type AddThreadInput struct {
	Repo        Repo
	PRID        string
	ReviewID    string
	Path        string
	Line        int
	Side        string
	StartLine   *int
	StartSide   string
	Body        string
	SubjectType string
}

type MergeOpts struct {
	Method       MergeMethod
	Auto         bool
	DeleteBranch bool
}

type Forge interface {
	ResolveRepo(ctx context.Context) (Repo, error)
	ListPRs(ctx context.Context, repo Repo, filter PRFilter) ([]domain.PRSummary, error)
	SearchPRs(ctx context.Context, repo Repo, query string) ([]domain.PRSummary, error)
	PRDetail(ctx context.Context, repo Repo, number int) (domain.PRDetail, error)
	Diff(ctx context.Context, repo Repo, number int) (string, error)
	Commits(ctx context.Context, repo Repo, number int) ([]domain.CommitSummary, error)
	CommitDiff(ctx context.Context, repo Repo, sha string) (string, error)
	ThreadComments(ctx context.Context, threadID, after string) ([]domain.Comment, string, error)
	EnsurePendingReview(ctx context.Context, prID, headOID string) (reviewID string, err error)
	AddReviewThread(ctx context.Context, in AddThreadInput) error
	UpdateComment(ctx context.Context, commentID, body string) error
	DeleteComment(ctx context.Context, repo Repo, ref CommentRef) error
	SubmitReview(ctx context.Context, reviewID string, event ReviewEvent, body string) error
	DiscardReview(ctx context.Context, reviewID string) error
	ReplyToThread(ctx context.Context, repo Repo, rootComment CommentRef, body string) error
	AddThreadReply(ctx context.Context, reviewID, threadID, body string) error
	ResolveThread(ctx context.Context, threadID string, resolved bool) error
	MarkFileViewed(ctx context.Context, prID, path string, viewed bool) error
	AddIssueComment(ctx context.Context, repo Repo, number int, body string) error
	Merge(ctx context.Context, repo Repo, number int, opts MergeOpts) error
	Checkout(ctx context.Context, number int) error
	FileContent(ctx context.Context, repo Repo, path, ref string) ([]byte, error)
}
