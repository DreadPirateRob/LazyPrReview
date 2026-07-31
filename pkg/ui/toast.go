package ui

import (
	"fmt"
	"time"
)

// ToastDuration is the display lifetime of every short toast notification.
const ToastDuration = 3 * time.Second

// Phase 1 toast strings — keep these in sync with docs/SPEC.md §14 and the
// plan; every user-visible message originates here, not from ad-hoc literals
// scattered across panel code.
const (
	ToastViewedToggleFailed       = "Viewed toggle failed"
	ToastCopiedPRURL              = "Copied PR URL"
	ToastCopiedBranchName         = "Copied branch name"
	ToastCopiedHeadSHA            = "Copied head SHA"
	ToastCopiedFilePath           = "Copied file path"
	ToastCopiedThreadPermalink    = "Copied thread permalink"
	ToastCopiedCommentBody        = "Copied comment body"
	ToastMultiplePendingReviews   = "Multiple pending reviews found — using the newest"
	ToastNoUnresolvedThreads      = "No unresolved threads"
	ToastNoUnresolvedThreadInFile = "No unresolved thread in file"
	ToastNoMentions               = "No threads mention you"
	ToastAuthorFlagFailed         = "Could not save author flag"
	ToastSetEditorForConfig       = "Set $EDITOR to edit config"
	ToastCannotResolveThread      = "You can't resolve this thread"
	ToastResolveFailed            = "Resolve/unresolve failed"
	ToastCommentFailed            = "Comment failed — draft not saved"
	ToastLineCommentPager         = "Line comments need the built-in diff (unset gui.diffPager)"
	ToastMixedSideSelection       = "Select lines on one side only"
	ToastReviewSubmitted          = "Review submitted"
	ToastReviewDiscarded          = "Pending review discarded"
	ToastReviewFailed             = "Review action failed"
	ToastNotYourComment           = "You can only edit/delete your own comments"
	ToastEditFailed               = "Edit failed"
	ToastDeleteFailed             = "Delete failed"
	ToastCannotReply              = "You can't reply to this thread"
	ToastReplyFailed              = "Reply failed"
)

// ToastViewedToggleFailedN returns the aggregate failure toast for a
// multi-file viewed-toggle batch where n files failed.
func ToastViewedToggleFailedN(n int) string {
	return fmt.Sprintf("Viewed toggle failed for %d file(s)", n)
}

// NewToast creates a Toast with a 3-second display window starting from now.
func NewToast(msg string) Toast {
	return Toast{
		Message: msg,
		Until:   time.Now().Add(ToastDuration),
	}
}

// NewToastAt creates a Toast with a 3-second window starting from t. Used in
// tests where clock control is needed.
func NewToastAt(msg string, t time.Time) Toast {
	return Toast{
		Message: msg,
		Until:   t.Add(ToastDuration),
	}
}

// ToastExpired reports whether t has passed its display deadline at the given
// wall time. An empty toast (no message) is never considered expired; it is
// simply absent.
func ToastExpired(t Toast, now time.Time) bool {
	return t.Message != "" && !now.Before(t.Until)
}

// ClearIfExpired returns an empty Toast when t has expired at now, otherwise
// returns t unchanged. Intended for use in the Tea Update loop.
func ClearIfExpired(t Toast, now time.Time) Toast {
	if ToastExpired(t, now) {
		return Toast{}
	}
	return t
}
