package github

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// ─── Primitive helpers ────────────────────────────────────────────────────────

// loginWrapper wraps a GitHub actor node that exposes a login field.
type loginWrapper struct {
	Login string `json:"login"`
}

// pageInfo is the standard GraphQL pagination cursor object.
type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// labelNode is a single GitHub label.
type labelNode struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// labelConnection wraps a paginated label list (we always fetch all ≤20 at once).
type labelConnection struct {
	Nodes []labelNode `json:"nodes"`
}

// stringInt64 handles GitHub's fullDatabaseId which arrives as a JSON string-encoded
// integer (e.g. "3509772848"), not a bare number. Falls back to a bare number for
// robustness.
type stringInt64 int64

func (n *stringInt64) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("stringInt64: parsing %q: %w", s, err)
		}
		*n = stringInt64(i)
		return nil
	}
	var i int64
	if err := json.Unmarshal(b, &i); err != nil {
		return err
	}
	*n = stringInt64(i)
	return nil
}

// ─── PR list types ────────────────────────────────────────────────────────────

// rollupContext is one entry in the statusCheckRollup array returned by gh pr list.
// It can be either a CheckRun or a StatusContext; __typename distinguishes them.
// Extra fields (workflowName, etc.) from gh are silently ignored by json.Unmarshal.
type rollupContext struct {
	Typename   string `json:"__typename"`
	State      string `json:"state"`      // StatusContext: SUCCESS|FAILURE|ERROR|PENDING|EXPECTED
	Status     string `json:"status"`     // CheckRun: COMPLETED|IN_PROGRESS|QUEUED|…
	Conclusion string `json:"conclusion"` // CheckRun: SUCCESS|FAILURE|CANCELLED|TIMED_OUT|…
}

// prListItem is one element in the JSON array returned by gh pr list --json.
type prListItem struct {
	Number            int             `json:"number"`
	Title             string          `json:"title"`
	Author            loginWrapper    `json:"author"`
	HeadRefName       string          `json:"headRefName"`
	BaseRefName       string          `json:"baseRefName"`
	IsDraft           bool            `json:"isDraft"`
	State             string          `json:"state"`
	ReviewDecision    string          `json:"reviewDecision"`
	StatusCheckRollup []rollupContext `json:"statusCheckRollup"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	Labels            []labelNode     `json:"labels"`
	Additions         int             `json:"additions"`
	Deletions         int             `json:"deletions"`
	ChangedFiles      int             `json:"changedFiles"`
	URL               string          `json:"url"`
}

func (it *prListItem) toDomain() domain.PRSummary {
	labels := make([]domain.Label, 0, len(it.Labels))
	for _, l := range it.Labels {
		labels = append(labels, domain.Label{Name: l.Name, Color: l.Color})
	}
	return domain.PRSummary{
		Number:           it.Number,
		Title:            it.Title,
		Author:           it.Author.Login,
		HeadRefName:      it.HeadRefName,
		BaseRefName:      it.BaseRefName,
		IsDraft:          it.IsDraft,
		State:            it.State,
		ReviewDecision:   it.ReviewDecision,
		StatusCheckState: normalizeRollupFromContexts(it.StatusCheckRollup),
		UpdatedAt:        it.UpdatedAt,
		Labels:           labels,
		Additions:        it.Additions,
		Deletions:        it.Deletions,
		ChangedFiles:     it.ChangedFiles,
		URL:              it.URL,
	}
}

// normalizeRollupFromContexts computes worst-case rollup state across all rollup
// contexts returned by gh pr list. Precedence: failure > pending > success > none.
func normalizeRollupFromContexts(ctxs []rollupContext) string {
	worst := "none"
	for _, ctx := range ctxs {
		s := contextEffectiveState(ctx)
		worst = maxRollupState(worst, s)
		if worst == "failure" {
			return "failure" // can't get worse
		}
	}
	return worst
}

func contextEffectiveState(ctx rollupContext) string {
	switch ctx.Typename {
	case "CheckRun":
		if ctx.Status != "COMPLETED" {
			return "pending"
		}
		switch strings.ToUpper(ctx.Conclusion) {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			return "success"
		case "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED":
			return "failure"
		default:
			return "pending"
		}
	case "StatusContext":
		switch strings.ToUpper(ctx.State) {
		case "SUCCESS":
			return "success"
		case "FAILURE", "ERROR":
			return "failure"
		case "PENDING", "EXPECTED":
			return "pending"
		}
	}
	return "none"
}

var rollupRank = map[string]int{"none": 0, "success": 1, "pending": 2, "failure": 3}

func maxRollupState(a, b string) string {
	if rollupRank[b] > rollupRank[a] {
		return b
	}
	return a
}

// normalizeRollupStateName maps the GraphQL CommitStatusState enum (from the mega-query's
// statusCheckRollup.state) to a lowercase normalized form used in domain.PRDetail.
func normalizeRollupStateName(state string) string {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return "success"
	case "FAILURE", "ERROR":
		return "failure"
	case "PENDING", "EXPECTED":
		return "pending"
	default:
		return "none"
	}
}

// ─── Mega-query JSON types ────────────────────────────────────────────────────

type fileNode struct {
	Path              string `json:"path"`
	Additions         int    `json:"additions"`
	Deletions         int    `json:"deletions"`
	ChangeType        string `json:"changeType"`
	ViewerViewedState string `json:"viewerViewedState"`
}

type fileConnection struct {
	PageInfo pageInfo   `json:"pageInfo"`
	Nodes    []fileNode `json:"nodes"`
}

// threadCommentNode is a comment inside a reviewThread.
type threadCommentNode struct {
	ID              string       `json:"id"`
	FullDatabaseID  stringInt64  `json:"fullDatabaseId"`
	Body            string       `json:"body"`
	State           string       `json:"state"`
	Author          loginWrapper `json:"author"`
	CreatedAt       time.Time    `json:"createdAt"`
	LastEditedAt    *time.Time   `json:"lastEditedAt"`
	URL             string       `json:"url"`
	ViewerDidAuthor bool         `json:"viewerDidAuthor"`
}

type threadCommentConn struct {
	PageInfo pageInfo            `json:"pageInfo"`
	Nodes    []threadCommentNode `json:"nodes"`
}

type threadNode struct {
	ID               string            `json:"id"`
	IsResolved       bool              `json:"isResolved"`
	IsOutdated       bool              `json:"isOutdated"`
	Path             string            `json:"path"`
	Line             *int              `json:"line"`
	StartLine        *int              `json:"startLine"`
	DiffSide         string            `json:"diffSide"`
	StartDiffSide    string            `json:"startDiffSide"`
	ResolvedBy       *loginWrapper     `json:"resolvedBy"`
	ViewerCanResolve bool              `json:"viewerCanResolve"`
	ViewerCanReply   bool              `json:"viewerCanReply"`
	Comments         threadCommentConn `json:"comments"`
}

type threadConnection struct {
	PageInfo pageInfo     `json:"pageInfo"`
	Nodes    []threadNode `json:"nodes"`
}

type latestReviewNode struct {
	Author      loginWrapper `json:"author"`
	State       string       `json:"state"`
	SubmittedAt *time.Time   `json:"submittedAt"`
	Body        string       `json:"body"`
}

type latestReviewConnection struct {
	Nodes []latestReviewNode `json:"nodes"`
}

// pendingCommentNode is a comment inside a PENDING review's own comment list
// (not inside a reviewThread). These have different fields from threadCommentNode.
type pendingCommentNode struct {
	ID             string      `json:"id"`
	FullDatabaseID stringInt64 `json:"fullDatabaseId"`
	Body           string      `json:"body"`
	Path           string      `json:"path"`
	Line           *int        `json:"line"`
	StartLine      *int        `json:"startLine"`
	State          string      `json:"state"`
	Outdated       bool        `json:"outdated"`
}

type pendingCommentConn struct {
	PageInfo pageInfo             `json:"pageInfo"`
	Nodes    []pendingCommentNode `json:"nodes"`
}

type pendingReviewNode struct {
	ID        string             `json:"id"`
	State     string             `json:"state"`
	Body      string             `json:"body"`
	CreatedAt time.Time          `json:"createdAt"`
	Comments  pendingCommentConn `json:"comments"`
}

type pendingReviewConnection struct {
	Nodes []pendingReviewNode `json:"nodes"`
}

// reviewerUnion decodes the requestedReviewer inline fragment.
// login is present for User; name is present for Team.
type reviewerUnion struct {
	Login string `json:"login"` // User
	Name  string `json:"name"`  // Team
}

type reviewRequestNode struct {
	RequestedReviewer reviewerUnion `json:"requestedReviewer"`
}

type reviewRequestConnection struct {
	Nodes []reviewRequestNode `json:"nodes"`
}

type assigneeConnection struct {
	Nodes []loginWrapper `json:"nodes"`
}

// checkContextNode decodes both CheckRun and StatusContext inline fragments.
// __typename distinguishes them.
type checkContextNode struct {
	Typename string `json:"__typename"`
	// CheckRun fields
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	DetailsURL  string     `json:"detailsUrl"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	// StatusContext fields
	Context     string     `json:"context"` // the name for StatusContext
	State       string     `json:"state"`
	TargetURL   string     `json:"targetUrl"`
	Description string     `json:"description"`
	CreatedAt   *time.Time `json:"createdAt"`
}

type checkContextConnection struct {
	PageInfo pageInfo           `json:"pageInfo"`
	Nodes    []checkContextNode `json:"nodes"`
}

type statusCheckRollupData struct {
	State    string                 `json:"state"`
	Contexts checkContextConnection `json:"contexts"`
}

type commitData struct {
	Oid               string                 `json:"oid"`
	StatusCheckRollup *statusCheckRollupData `json:"statusCheckRollup"`
}

type commitWrapper struct {
	Commit commitData `json:"commit"`
}

type commitConnection struct {
	Nodes []commitWrapper `json:"nodes"`
}

type issueCommentNode struct {
	ID        string       `json:"id"`
	Author    loginWrapper `json:"author"`
	Body      string       `json:"body"`
	CreatedAt time.Time    `json:"createdAt"`
	URL       string       `json:"url"`
}

type issueCommentConnection struct {
	PageInfo pageInfo           `json:"pageInfo"`
	Nodes    []issueCommentNode `json:"nodes"`
}

type rateLimitData struct {
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"resetAt"`
}

type pullRequestData struct {
	ID             string                  `json:"id"`
	Number         int                     `json:"number"`
	Title          string                  `json:"title"`
	Body           string                  `json:"body"`
	State          string                  `json:"state"`
	IsDraft        bool                    `json:"isDraft"`
	URL            string                  `json:"url"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
	Author         loginWrapper            `json:"author"`
	BaseRefName    string                  `json:"baseRefName"`
	HeadRefName    string                  `json:"headRefName"`
	HeadRefOid     string                  `json:"headRefOid"`
	Additions      int                     `json:"additions"`
	Deletions      int                     `json:"deletions"`
	ChangedFiles   int                     `json:"changedFiles"`
	Mergeable      string                  `json:"mergeable"`
	ReviewDecision string                  `json:"reviewDecision"`
	Labels         labelConnection         `json:"labels"`
	Files          fileConnection          `json:"files"`
	ReviewThreads  threadConnection        `json:"reviewThreads"`
	LatestReviews  latestReviewConnection  `json:"latestReviews"`
	Reviews        pendingReviewConnection `json:"reviews"`
	ReviewRequests reviewRequestConnection `json:"reviewRequests"`
	Assignees      assigneeConnection      `json:"assignees"`
	Commits        commitConnection        `json:"commits"`
	Comments       issueCommentConnection  `json:"comments"`
}

type repositoryData struct {
	PullRequest pullRequestData `json:"pullRequest"`
}

type gqlPRDetailData struct {
	RateLimit  rateLimitData  `json:"rateLimit"`
	Repository repositoryData `json:"repository"`
}

// ─── Domain conversion ────────────────────────────────────────────────────────

// buildPRDetail converts the decoded mega-query data plus paginated files/threads
// into a domain.PRDetail value.
func buildPRDetail(
	data gqlPRDetailData,
	files []fileNode,
	threads []threadNode,
	truncatedFiles bool,
	truncatedThreads bool,
) domain.PRDetail {
	pr := data.Repository.PullRequest
	rl := data.RateLimit

	// Files
	domainFiles := make([]domain.ChangedFile, 0, len(files))
	for _, f := range files {
		domainFiles = append(domainFiles, domain.ChangedFile{
			Path:              f.Path,
			Additions:         f.Additions,
			Deletions:         f.Deletions,
			ChangeType:        f.ChangeType,
			ViewerViewedState: f.ViewerViewedState,
		})
	}

	// Threads
	domainThreads := make([]domain.Thread, 0, len(threads))
	for _, t := range threads {
		resolvedBy := ""
		if t.ResolvedBy != nil {
			resolvedBy = t.ResolvedBy.Login
		}
		comments := make([]domain.Comment, 0, len(t.Comments.Nodes))
		for _, c := range t.Comments.Nodes {
			comments = append(comments, domain.Comment{
				ID:              c.ID,
				FullDatabaseID:  int64(c.FullDatabaseID),
				Body:            c.Body,
				State:           c.State,
				Author:          c.Author.Login,
				CreatedAt:       c.CreatedAt,
				LastEditedAt:    c.LastEditedAt,
				URL:             c.URL,
				ViewerDidAuthor: c.ViewerDidAuthor,
			})
		}
		domainThreads = append(domainThreads, domain.Thread{
			ID:               t.ID,
			IsResolved:       t.IsResolved,
			IsOutdated:       t.IsOutdated,
			Path:             t.Path,
			Line:             t.Line,
			StartLine:        t.StartLine,
			DiffSide:         t.DiffSide,
			StartDiffSide:    t.StartDiffSide,
			ResolvedBy:       resolvedBy,
			ViewerCanResolve: t.ViewerCanResolve,
			ViewerCanReply:   t.ViewerCanReply,
			Comments:         comments,
		})
	}

	// Latest reviews
	latestReviews := make([]domain.Review, 0, len(pr.LatestReviews.Nodes))
	for _, r := range pr.LatestReviews.Nodes {
		latestReviews = append(latestReviews, domain.Review{
			State:       r.State,
			Body:        r.Body,
			Author:      r.Author.Login,
			SubmittedAt: r.SubmittedAt,
		})
	}

	// Pending review adoption: choose newest by createdAt.
	var pendingReview *domain.Review
	hasMultiple := len(pr.Reviews.Nodes) > 1
	if len(pr.Reviews.Nodes) > 0 {
		newest := pr.Reviews.Nodes[0]
		for _, rv := range pr.Reviews.Nodes[1:] {
			if rv.CreatedAt.After(newest.CreatedAt) {
				newest = rv
			}
		}
		pendingComments := make([]domain.Comment, 0, len(newest.Comments.Nodes))
		for _, c := range newest.Comments.Nodes {
			pendingComments = append(pendingComments, domain.Comment{
				ID:             c.ID,
				FullDatabaseID: int64(c.FullDatabaseID),
				Body:           c.Body,
				State:          c.State,
				Path:           c.Path,
				Line:           c.Line,
				StartLine:      c.StartLine,
				Outdated:       c.Outdated,
			})
		}
		pendingReview = &domain.Review{
			ID:        newest.ID,
			State:     newest.State,
			Body:      newest.Body,
			CreatedAt: newest.CreatedAt,
			Comments:  pendingComments,
		}
	}

	// Requested reviewers: User ↔ non-empty Login; Team ↔ non-empty Name.
	requestedReviewers := make([]domain.RequestedReviewer, 0, len(pr.ReviewRequests.Nodes))
	for _, rr := range pr.ReviewRequests.Nodes {
		rv := rr.RequestedReviewer
		if rv.Login != "" {
			requestedReviewers = append(requestedReviewers, domain.RequestedReviewer{Kind: "user", Name: rv.Login})
		} else if rv.Name != "" {
			requestedReviewers = append(requestedReviewers, domain.RequestedReviewer{Kind: "team", Name: rv.Name})
		}
	}

	// Assignees
	assignees := make([]string, 0, len(pr.Assignees.Nodes))
	for _, a := range pr.Assignees.Nodes {
		if a.Login != "" {
			assignees = append(assignees, a.Login)
		}
	}

	// Checks from the most recent commit's statusCheckRollup.
	var checks []domain.Check
	checksRollup := "none"
	if len(pr.Commits.Nodes) > 0 {
		commit := pr.Commits.Nodes[0].Commit
		if commit.StatusCheckRollup != nil {
			checksRollup = normalizeRollupStateName(commit.StatusCheckRollup.State)
			for _, ctx := range commit.StatusCheckRollup.Contexts.Nodes {
				checks = append(checks, contextToCheck(ctx))
			}
		}
	}

	// Labels
	labels := make([]domain.Label, 0, len(pr.Labels.Nodes))
	for _, l := range pr.Labels.Nodes {
		labels = append(labels, domain.Label{Name: l.Name, Color: l.Color})
	}

	// Issue comments (as domain.Comment for IssueComments field)
	issueComments := make([]domain.Comment, 0, len(pr.Comments.Nodes))
	for _, c := range pr.Comments.Nodes {
		issueComments = append(issueComments, domain.Comment{
			ID:        c.ID,
			Author:    c.Author.Login,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			URL:       c.URL,
		})
	}

	// Timeline: merge issue comments + submitted latest reviews, sorted by time.
	timeline := buildTimeline(pr.Comments.Nodes, pr.LatestReviews.Nodes)

	pendingReviewCount := 0
	if pendingReview != nil {
		pendingReviewCount = len(pendingReview.Comments)
	}

	return domain.PRDetail{
		ID:                        pr.ID,
		Number:                    pr.Number,
		Title:                     pr.Title,
		Body:                      pr.Body,
		State:                     pr.State,
		IsDraft:                   pr.IsDraft,
		URL:                       pr.URL,
		CreatedAt:                 pr.CreatedAt,
		UpdatedAt:                 pr.UpdatedAt,
		Author:                    pr.Author.Login,
		BaseRefName:               pr.BaseRefName,
		HeadRefName:               pr.HeadRefName,
		HeadRefOID:                pr.HeadRefOid,
		Additions:                 pr.Additions,
		Deletions:                 pr.Deletions,
		ChangedFiles:              pr.ChangedFiles,
		Mergeable:                 pr.Mergeable,
		ReviewDecision:            pr.ReviewDecision,
		Labels:                    labels,
		Files:                     domainFiles,
		Threads:                   domainThreads,
		LatestReviews:             latestReviews,
		PendingReview:             pendingReview,
		PendingReviewCount:        pendingReviewCount,
		HasMultiplePendingReviews: hasMultiple,
		RequestedReviewers:        requestedReviewers,
		Assignees:                 assignees,
		Checks:                    checks,
		ChecksRollupState:         checksRollup,
		IssueComments:             issueComments,
		Timeline:                  timeline,
		RateLimitRemaining:        rl.Remaining,
		RateLimitResetAt:          rl.ResetAt,
		TruncatedFiles:            truncatedFiles,
		TruncatedThreads:          truncatedThreads,
	}
}

// contextToCheck converts a checkContextNode to a domain.Check.
func contextToCheck(ctx checkContextNode) domain.Check {
	switch ctx.Typename {
	case "CheckRun":
		return domain.Check{
			Kind:        "CheckRun",
			Name:        ctx.Name,
			Status:      ctx.Status,
			Conclusion:  ctx.Conclusion,
			DetailsURL:  ctx.DetailsURL,
			StartedAt:   ctx.StartedAt,
			CompletedAt: ctx.CompletedAt,
		}
	case "StatusContext":
		return domain.Check{
			Kind:        "StatusContext",
			Name:        ctx.Context, // StatusContext uses "context" as its display name
			State:       ctx.State,
			TargetURL:   ctx.TargetURL,
			Description: ctx.Description,
			CreatedAt:   ctx.CreatedAt,
		}
	default:
		return domain.Check{Kind: ctx.Typename, Name: ctx.Name}
	}
}

// buildTimeline merges issue comments and submitted latest reviews into one
// chronological list, as defined in SPEC §9.
func buildTimeline(comments []issueCommentNode, reviews []latestReviewNode) []domain.TimelineItem {
	items := make([]domain.TimelineItem, 0, len(comments)+len(reviews))
	for _, c := range comments {
		items = append(items, domain.TimelineItem{
			Kind:   "comment",
			SortAt: c.CreatedAt,
			Author: c.Author.Login,
			Body:   c.Body,
			URL:    c.URL,
		})
	}
	for _, r := range reviews {
		if r.SubmittedAt == nil {
			continue // PENDING reviews are not in the timeline
		}
		items = append(items, domain.TimelineItem{
			Kind:   "review",
			SortAt: *r.SubmittedAt,
			Author: r.Author.Login,
			Body:   r.Body,
			State:  r.State,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].SortAt.Before(items[j].SortAt)
	})
	return items
}
