// Package github implements the forge.Forge interface backed by the gh CLI.
// All GitHub access goes through a ghcli.Runner so every call is logged,
// timeout-guarded, and classified before the UI ever sees the result.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// paginationCap is the maximum number of files or threads to accumulate before
// setting TruncatedFiles/TruncatedThreads on domain.PRDetail.
const paginationCap = 500

// Client implements forge.Forge using the gh CLI runner.
type Client struct {
	runner ghcli.Runner
}

// Compile-time assertion that *Client satisfies forge.Forge.
var _ forge.Forge = (*Client)(nil)

// New creates a Client backed by the given runner.
func New(runner ghcli.Runner) *Client {
	return &Client{runner: runner}
}

// ─── Startup probes (not part of forge.Forge) ─────────────────────────────────

// ProbeAuth runs "gh auth status" to verify the CLI is authenticated.
// Returns ghcli.ErrGHNotFound if gh is absent, ghcli.ErrGHUnauthed if not logged in.
func (c *Client) ProbeAuth(ctx context.Context) error {
	_, err := c.runner.Run(ctx, "auth", "status")
	return err
}

// ViewerLogin returns the authenticated user's login via a tiny GraphQL query,
// confirming both network reachability and API authentication.
func (c *Client) ViewerLogin(ctx context.Context) (string, error) {
	var data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	if err := c.runGQL(ctx, &data, queryViewer); err != nil {
		return "", err
	}
	return data.Viewer.Login, nil
}

// ─── forge.Forge — Phase 1 methods ───────────────────────────────────────────

// ResolveRepo resolves the current repository from the working directory via
// "gh repo view --json nameWithOwner". Returns ghcli.ErrNoRepo if cwd is not
// inside a git repo or no remote matches.
func (c *Client) ResolveRepo(ctx context.Context) (forge.Repo, error) {
	body, err := c.runner.Run(ctx, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return forge.Repo{}, err
	}
	var rv struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(body, &rv); err != nil {
		return forge.Repo{}, fmt.Errorf("decoding repo view: %w", err)
	}
	parts := strings.SplitN(rv.NameWithOwner, "/", 2)
	if len(parts) != 2 {
		return forge.Repo{}, fmt.Errorf("unexpected nameWithOwner: %q", rv.NameWithOwner)
	}
	return forge.Repo{Owner: parts[0], Name: parts[1]}, nil
}

// prListFields is the exact --json field list for gh pr list, matching SPEC §9.
const prListFields = "number,title,author,headRefName,baseRefName,isDraft,state,reviewDecision,statusCheckRollup,updatedAt,labels,additions,deletions,changedFiles,url"

// ListPRs fetches up to 50 PRs for the given filter, sorted by most recently
// updated. The StatusCheckState on each PRSummary is normalized from the
// individual check context array (worst-state precedence: failure>pending>success>none).
func (c *Client) ListPRs(ctx context.Context, repo forge.Repo, filter forge.PRFilter) ([]domain.PRSummary, error) {
	q := buildSearchQuery(filter)
	body, err := c.runner.Run(ctx,
		"pr", "list",
		"--repo", repo.Owner+"/"+repo.Name,
		"--limit", "50",
		"--search", q,
		"--json", prListFields,
	)
	if err != nil {
		return nil, err
	}
	return decodePRList(body)
}

// decodePRList unmarshals a `gh pr list --json` array into PR summaries.
func decodePRList(body []byte) ([]domain.PRSummary, error) {
	var items []prListItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("decoding pr list: %w", err)
	}
	out := make([]domain.PRSummary, 0, len(items))
	for i := range items {
		out = append(out, items[i].toDomain())
	}
	return out, nil
}

// SearchPRs runs a repo-wide PR search across all states (open/closed/merged),
// passing the caller's query straight to gh so GitHub search qualifiers work.
// An empty query returns no results rather than listing the whole repo.
func (c *Client) SearchPRs(ctx context.Context, repo forge.Repo, query string) ([]domain.PRSummary, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	body, err := c.runner.Run(ctx,
		"pr", "list",
		"--repo", repo.Owner+"/"+repo.Name,
		"--state", "all",
		"--limit", "100",
		"--search", q+" sort:updated-desc",
		"--json", prListFields,
	)
	if err != nil {
		return nil, err
	}
	return decodePRList(body)
}

// buildSearchQuery returns the --search argument for gh pr list for the given filter.
// FilterAllOpen uses just "sort:updated-desc"; the other filters prepend a qualifier.
func buildSearchQuery(filter forge.PRFilter) string {
	switch filter {
	case forge.FilterReviewRequested:
		return "review-requested:@me sort:updated-desc"
	case forge.FilterMine:
		return "author:@me sort:updated-desc"
	default: // FilterAllOpen
		return "sort:updated-desc"
	}
}

// PRDetail runs the mega-query from SPEC §9 then follows cursor pages for files
// and reviewThreads until each is exhausted or the paginationCap (500) is reached.
// TruncatedFiles / TruncatedThreads are set when the cap truncates.
func (c *Client) PRDetail(ctx context.Context, repo forge.Repo, number int) (domain.PRDetail, error) {
	var data gqlPRDetailData
	if err := c.runGQL(ctx, &data, queryPRDetail,
		"-f", "owner="+repo.Owner,
		"-f", "name="+repo.Name,
		"-F", "number="+strconv.Itoa(number),
	); err != nil {
		return domain.PRDetail{}, err
	}

	pr := &data.Repository.PullRequest

	// ── Paginate files ────────────────────────────────────────────────────────
	files := append([]fileNode(nil), pr.Files.Nodes...)
	for pr.Files.PageInfo.HasNextPage && len(files) < paginationCap {
		var page struct {
			Repository struct {
				PullRequest struct {
					Files fileConnection `json:"files"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := c.runGQL(ctx, &page, queryPRDetailFilesPage,
			"-f", "owner="+repo.Owner,
			"-f", "name="+repo.Name,
			"-F", "number="+strconv.Itoa(number),
			"-f", "cursor="+pr.Files.PageInfo.EndCursor,
		); err != nil {
			return domain.PRDetail{}, err
		}
		p := page.Repository.PullRequest.Files
		files = append(files, p.Nodes...)
		pr.Files.PageInfo = p.PageInfo
	}
	truncatedFiles := pr.Files.PageInfo.HasNextPage
	if len(files) > paginationCap {
		files = files[:paginationCap]
		truncatedFiles = true
	}

	// ── Paginate threads ──────────────────────────────────────────────────────
	threads := append([]threadNode(nil), pr.ReviewThreads.Nodes...)
	for pr.ReviewThreads.PageInfo.HasNextPage && len(threads) < paginationCap {
		var page struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads threadConnection `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := c.runGQL(ctx, &page, queryPRDetailThreadsPage,
			"-f", "owner="+repo.Owner,
			"-f", "name="+repo.Name,
			"-F", "number="+strconv.Itoa(number),
			"-f", "cursor="+pr.ReviewThreads.PageInfo.EndCursor,
		); err != nil {
			return domain.PRDetail{}, err
		}
		p := page.Repository.PullRequest.ReviewThreads
		threads = append(threads, p.Nodes...)
		pr.ReviewThreads.PageInfo = p.PageInfo
	}
	truncatedThreads := pr.ReviewThreads.PageInfo.HasNextPage
	if len(threads) > paginationCap {
		threads = threads[:paginationCap]
		truncatedThreads = true
	}

	return buildPRDetail(data, files, threads, truncatedFiles, truncatedThreads), nil
}

// Diff returns the unified diff for the PR using "gh pr diff {n} --repo {o/r}".
// NOT --patch (which emits per-commit patches); unified whole-PR diff per SPEC §9.
func (c *Client) Diff(ctx context.Context, repo forge.Repo, number int) (string, error) {
	body, err := c.runner.Run(ctx,
		"pr", "diff", strconv.Itoa(number),
		"--repo", repo.Owner+"/"+repo.Name,
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// CommitDiff returns the unified diff for a single commit via the REST API
// with the diff media type, so the Commits tab can scope Main to one commit.
func (c *Client) CommitDiff(ctx context.Context, repo forge.Repo, sha string) (string, error) {
	if sha == "" {
		return "", nil
	}
	body, err := c.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/commits/%s", repo.Owner, repo.Name, sha),
		"-H", "Accept: application/vnd.github.diff",
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// Commits returns the PR's commit list via `gh pr view --json commits`.
func (c *Client) Commits(ctx context.Context, repo forge.Repo, number int) ([]domain.CommitSummary, error) {
	body, err := c.runner.Run(ctx,
		"pr", "view", strconv.Itoa(number),
		"--repo", repo.Owner+"/"+repo.Name,
		"--json", "commits",
	)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Commits []struct {
			OID             string `json:"oid"`
			MessageHeadline string `json:"messageHeadline"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make([]domain.CommitSummary, 0, len(payload.Commits))
	for _, c := range payload.Commits {
		out = append(out, domain.CommitSummary{OID: c.OID, MessageHeadline: c.MessageHeadline})
	}
	return out, nil
}

// ThreadComments fetches a page of comments for a single review thread (for
// overflow beyond the first 50 loaded in the mega-query). Returns the comments
// and the cursor for the next page (empty if no more pages).
func (c *Client) ThreadComments(ctx context.Context, threadID, after string) ([]domain.Comment, string, error) {
	var data struct {
		Node struct {
			Comments struct {
				PageInfo pageInfo            `json:"pageInfo"`
				Nodes    []threadCommentNode `json:"nodes"`
			} `json:"comments"`
		} `json:"node"`
	}
	vars := []string{"-f", "threadID=" + threadID}
	if after != "" {
		vars = append(vars, "-f", "after="+after)
	}
	if err := c.runGQL(ctx, &data, queryThreadComments, vars...); err != nil {
		return nil, "", err
	}

	nodes := data.Node.Comments.Nodes
	comments := make([]domain.Comment, 0, len(nodes))
	for _, n := range nodes {
		comments = append(comments, domain.Comment{
			ID:              n.ID,
			FullDatabaseID:  int64(n.FullDatabaseID),
			Body:            n.Body,
			State:           n.State,
			Author:          n.Author.Login,
			CreatedAt:       n.CreatedAt,
			LastEditedAt:    n.LastEditedAt,
			URL:             n.URL,
			ViewerDidAuthor: n.ViewerDidAuthor,
		})
	}

	nextCursor := ""
	if data.Node.Comments.PageInfo.HasNextPage {
		nextCursor = data.Node.Comments.PageInfo.EndCursor
	}
	return comments, nextCursor, nil
}

// MarkFileViewed toggles the viewer-viewed state of a changed file in a PR.
// viewed=true → markFileAsViewed mutation; viewed=false → unmarkFileAsViewed.
// This is the only Phase 1 write path.
func (c *Client) MarkFileViewed(ctx context.Context, prID, path string, viewed bool) error {
	mutation := mutationUnmarkFileViewed
	if viewed {
		mutation = mutationMarkFileViewed
	}
	var result json.RawMessage // discard the clientMutationId
	return c.runGQL(ctx, &result, mutation, "-f", "prID="+prID, "-f", "path="+path)
}

// ─── forge.Forge — Phase 2/3 stubs ────────────────────────────────────────────
// Every method below returns ErrUnsupportedInPhase1. No keybindings call these in
// Phase 1, so they are intentionally unreachable from the UI.

func (c *Client) EnsurePendingReview(ctx context.Context, prID, headOID string) (string, error) {
	query := "mutation($pr:ID!,$oid:GitObjectID!){addPullRequestReview(input:{pullRequestId:$pr,commitOID:$oid}){pullRequestReview{id}}}"
	var out struct {
		AddPullRequestReview struct {
			PullRequestReview struct {
				ID string `json:"id"`
			} `json:"pullRequestReview"`
		} `json:"addPullRequestReview"`
	}
	if err := c.runGQL(ctx, &out, query, "-f", "pr="+prID, "-f", "oid="+headOID); err != nil {
		return "", err
	}
	return out.AddPullRequestReview.PullRequestReview.ID, nil
}

func (c *Client) AddReviewThread(ctx context.Context, in forge.AddThreadInput) error {
	vars := []string{"-f", "rev=" + in.ReviewID, "-f", "body=" + in.Body}
	var query string
	if in.SubjectType == "FILE" {
		query = "mutation($rev:ID!,$body:String!){addPullRequestReviewThread(input:{pullRequestReviewId:$rev,body:$body,subjectType:FILE}){thread{id}}}"
	} else {
		vars = append(vars, "-f", "path="+in.Path, "-F", "line="+strconv.Itoa(in.Line))
		if in.StartLine != nil {
			query = "mutation($rev:ID!,$body:String!,$path:String!,$line:Int!,$startLine:Int!){addPullRequestReviewThread(input:{pullRequestReviewId:$rev,body:$body,path:$path,line:$line,side:" + in.Side + ",subjectType:LINE,startLine:$startLine,startSide:" + in.StartSide + "}){thread{id}}}"
			vars = append(vars, "-F", "startLine="+strconv.Itoa(*in.StartLine))
		} else {
			query = "mutation($rev:ID!,$body:String!,$path:String!,$line:Int!){addPullRequestReviewThread(input:{pullRequestReviewId:$rev,body:$body,path:$path,line:$line,side:" + in.Side + ",subjectType:LINE}){thread{id}}}"
		}
	}
	var out struct{}
	return c.runGQL(ctx, &out, query, vars...)
}

func (c *Client) UpdateComment(ctx context.Context, commentID, body string) error {
	query := "mutation($id:ID!,$body:String!){updatePullRequestReviewComment(input:{pullRequestReviewCommentId:$id,body:$body}){pullRequestReviewComment{id}}}"
	var out struct{}
	return c.runGQL(ctx, &out, query, "-f", "id="+commentID, "-f", "body="+body)
}

func (c *Client) DeleteComment(ctx context.Context, repo forge.Repo, ref forge.CommentRef) error {
	if ref.Pending {
		query := "mutation($id:ID!){deletePullRequestReviewComment(input:{id:$id}){pullRequestReview{id}}}"
		var out struct{}
		return c.runGQL(ctx, &out, query, "-f", "id="+ref.ID)
	}
	_, err := c.runner.Run(ctx, "api", "-X", "DELETE",
		fmt.Sprintf("repos/%s/%s/pulls/comments/%d", repo.Owner, repo.Name, ref.FullDatabaseID))
	return err
}

func (c *Client) SubmitReview(ctx context.Context, reviewID string, event forge.ReviewEvent, body string) error {
	switch event {
	case "APPROVE", "REQUEST_CHANGES", "COMMENT":
	default:
		return fmt.Errorf("unknown review event %q", event)
	}
	query := "mutation($rev:ID!,$body:String){submitPullRequestReview(input:{pullRequestReviewId:$rev,event:" + string(event) + ",body:$body}){pullRequestReview{state}}}"
	var out struct{}
	return c.runGQL(ctx, &out, query, "-f", "rev="+reviewID, "-f", "body="+body)
}

func (c *Client) DiscardReview(ctx context.Context, reviewID string) error {
	query := "mutation($rev:ID!){deletePullRequestReview(input:{pullRequestReviewId:$rev}){pullRequestReview{id}}}"
	var out struct{}
	return c.runGQL(ctx, &out, query, "-f", "rev="+reviewID)
}

func (c *Client) ReplyToThread(_ context.Context, _ forge.Repo, _ forge.CommentRef, _ string) error {
	return ghcli.ErrUnsupportedInPhase1
}

// AddThreadReply adds a reply to a review thread, batched onto the pending
// review (verified to produce a PENDING draft reply, SPEC §9).
func (c *Client) AddThreadReply(ctx context.Context, reviewID, threadID, body string) error {
	query := "mutation($rev:ID!,$th:ID!,$body:String!){addPullRequestReviewThreadReply(input:{pullRequestReviewId:$rev,pullRequestReviewThreadId:$th,body:$body}){comment{id}}}"
	var out struct{}
	return c.runGQL(ctx, &out, query, "-f", "rev="+reviewID, "-f", "th="+threadID, "-f", "body="+body)
}

func (c *Client) ResolveThread(ctx context.Context, threadID string, resolved bool) error {
	query := "mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{id}}}"
	if !resolved {
		query = "mutation($id:ID!){unresolveReviewThread(input:{threadId:$id}){thread{id}}}"
	}
	var out struct{}
	return c.runGQL(ctx, &out, query, "-f", "id="+threadID)
}

func (c *Client) AddIssueComment(_ context.Context, _ forge.Repo, _ int, _ string) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (c *Client) Merge(_ context.Context, _ forge.Repo, _ int, _ forge.MergeOpts) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (c *Client) Checkout(_ context.Context, _ int) error {
	return ghcli.ErrUnsupportedInPhase1
}

func (c *Client) FileContent(_ context.Context, _ forge.Repo, _, _ string) ([]byte, error) {
	return nil, ghcli.ErrUnsupportedInPhase1
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// runGQL runs "gh api graphql" with the given query and variable args (e.g.
// "-f owner=foo", "-F number=123"), then decodes the "data" field of the response
// into out. GraphQL-level errors in a 200 response are detected and returned as
// *ghcli.GraphQLError. Uses runner.Run directly to avoid DisallowUnknownFields.
func (c *Client) runGQL(ctx context.Context, out any, query string, vars ...string) error {
	args := make([]string, 0, 4+len(vars))
	args = append(args, "api", "graphql", "-f", "query="+query)
	args = append(args, vars...)

	body, err := c.runner.Run(ctx, args...)
	if err != nil {
		return err
	}

	// Decode the GraphQL envelope to check for errors and extract "data".
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data json.RawMessage `json:"data"`
	}
	if jsonErr := json.Unmarshal(body, &envelope); jsonErr != nil {
		return fmt.Errorf("decoding graphql response: %w", jsonErr)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			if e.Message != "" {
				msgs = append(msgs, e.Message)
			}
		}
		return &ghcli.GraphQLError{Messages: msgs, Raw: string(body)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}
