package github

// queryViewer fetches the authenticated viewer's login — used as a startup probe.
const queryViewer = `query { viewer { login } }`

// queryPRDetail is the mega-query from SPEC §9, schema-verified against jesseduffield/lazygit.
// All field names below are confirmed present in the GitHub GraphQL schema.
const queryPRDetail = `query PRDetail($owner: String!, $name: String!, $number: Int!) {
  rateLimit { remaining resetAt }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id number title body state isDraft url createdAt updatedAt
      author { login }
      baseRefName headRefName headRefOid
      additions deletions changedFiles
      mergeable reviewDecision
      labels(first: 20) { nodes { name color } }
      files(first: 100) {
        pageInfo { hasNextPage endCursor }
        nodes { path additions deletions changeType viewerViewedState }
      }
      reviewThreads(first: 100) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id isResolved isOutdated path line startLine diffSide startDiffSide
          resolvedBy { login } viewerCanResolve viewerCanReply
          comments(first: 50) {
            pageInfo { hasNextPage endCursor }
            nodes { id fullDatabaseId body state author { login } createdAt lastEditedAt url viewerDidAuthor }
          }
        }
      }
      latestReviews(first: 50) { nodes { author { login } state submittedAt body } }
      reviews(states: [PENDING], first: 5) {
        nodes {
          id state body createdAt
          comments(first: 100) {
            pageInfo { hasNextPage endCursor }
            nodes { id fullDatabaseId body path line startLine state outdated }
          }
        }
      }
      reviewRequests(first: 20) {
        nodes { requestedReviewer { ... on User { login } ... on Team { name } } }
      }
      assignees(first: 20) { nodes { login } }
      commits(last: 1) {
        nodes { commit { oid statusCheckRollup { state
          contexts(first: 100) {
            pageInfo { hasNextPage endCursor }
            nodes {
              __typename
              ... on CheckRun { name status conclusion detailsUrl startedAt completedAt }
              ... on StatusContext { context state targetUrl description createdAt }
            } } } } }
      }
      comments(first: 50) {
        pageInfo { hasNextPage endCursor }
        nodes { id author { login } body createdAt url }
      }
    }
  }
}`

// queryPRDetailFilesPage fetches the next page of changed files via cursor.
const queryPRDetailFilesPage = `query PRDetailFiles($owner: String!, $name: String!, $number: Int!, $cursor: String!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      files(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes { path additions deletions changeType viewerViewedState }
      }
    }
  }
}`

// queryPRDetailThreadsPage fetches the next page of review threads via cursor.
const queryPRDetailThreadsPage = `query PRDetailThreads($owner: String!, $name: String!, $number: Int!, $cursor: String!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id isResolved isOutdated path line startLine diffSide startDiffSide
          resolvedBy { login } viewerCanResolve viewerCanReply
          comments(first: 50) {
            pageInfo { hasNextPage endCursor }
            nodes { id fullDatabaseId body state author { login } createdAt lastEditedAt url viewerDidAuthor }
          }
        }
      }
    }
  }
}`

// queryThreadComments paginates comments inside a single review thread (overflow beyond first 50).
const queryThreadComments = `query ThreadComments($threadID: ID!, $after: String) {
  node(id: $threadID) {
    ... on PullRequestReviewThread {
      comments(first: 50, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes { id fullDatabaseId body state author { login } createdAt lastEditedAt url viewerDidAuthor }
      }
    }
  }
}`

// mutationMarkFileViewed marks a changed file as viewed in a PR.
const mutationMarkFileViewed = `mutation MarkFileViewed($prID: ID!, $path: String!) {
  markFileAsViewed(input: {pullRequestId: $prID, path: $path}) { clientMutationId }
}`

// mutationUnmarkFileViewed removes the viewed mark from a changed file in a PR.
const mutationUnmarkFileViewed = `mutation UnmarkFileViewed($prID: ID!, $path: String!) {
  unmarkFileAsViewed(input: {pullRequestId: $prID, path: $path}) { clientMutationId }
}`
