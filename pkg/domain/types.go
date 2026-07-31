package domain

import "time"

type Label struct {
	Name  string
	Color string
}

type RequestedReviewer struct {
	Kind string
	Name string
}

type ChangedFile struct {
	Path              string
	Additions         int
	Deletions         int
	ChangeType        string
	ViewerViewedState string
}

type Comment struct {
	ID             string
	FullDatabaseID int64
	Body           string
	State          string
	Author         string
	CreatedAt      time.Time
	LastEditedAt   *time.Time
	URL            string
	ViewerDidAuthor bool
	Path           string
	Line           *int
	StartLine      *int
	Outdated       bool
}

type Thread struct {
	ID              string
	IsResolved      bool
	IsOutdated      bool
	Path            string
	Line            *int
	StartLine       *int
	DiffSide        string
	StartDiffSide   string
	ResolvedBy      string
	ViewerCanResolve bool
	ViewerCanReply  bool
	Comments        []Comment
}

type Review struct {
	ID          string
	State       string
	Body        string
	CreatedAt   time.Time
	SubmittedAt *time.Time
	Author      string
	Comments    []Comment
}

type Check struct {
	Kind        string
	Name        string
	Status      string
	Conclusion  string
	State       string
	DetailsURL  string
	TargetURL   string
	Description string
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   *time.Time
}

type TimelineItem struct {
	Kind   string
	SortAt time.Time
	Author string
	Body   string
	State  string
	URL    string
}

type CommitSummary struct {
	OID             string
	MessageHeadline string
}

type PRSummary struct {
	Number           int
	Title            string
	Author           string
	HeadRefName      string
	BaseRefName      string
	IsDraft          bool
	State            string
	ReviewDecision   string
	StatusCheckState string
	UpdatedAt        time.Time
	Labels           []Label
	Additions        int
	Deletions        int
	ChangedFiles     int
	URL              string
}

type PRDetail struct {
	ID                        string
	Number                    int
	Title                     string
	Body                      string
	State                     string
	IsDraft                   bool
	URL                       string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	Author                    string
	BaseRefName               string
	HeadRefName               string
	HeadRefOID                string
	Additions                 int
	Deletions                 int
	ChangedFiles              int
	Mergeable                 string
	ReviewDecision            string
	Labels                    []Label
	Files                     []ChangedFile
	Threads                   []Thread
	LatestReviews             []Review
	PendingReview             *Review
	PendingReviewCount        int
	HasMultiplePendingReviews bool
	RequestedReviewers        []RequestedReviewer
	Assignees                 []string
	Checks                    []Check
	ChecksRollupState         string
	IssueComments             []Comment
	Timeline                  []TimelineItem
	RateLimitRemaining        int
	RateLimitResetAt          time.Time
	TruncatedFiles            bool
	TruncatedThreads          bool
}
