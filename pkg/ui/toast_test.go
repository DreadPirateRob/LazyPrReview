package ui

import (
	"testing"
	"time"
)

func TestToastNewDuration(t *testing.T) {
	before := time.Now()
	toast := NewToast("hello")
	after := time.Now()

	if toast.Message != "hello" {
		t.Fatalf("expected message 'hello', got %q", toast.Message)
	}
	lo := before.Add(ToastDuration)
	hi := after.Add(ToastDuration)
	if toast.Until.Before(lo) || toast.Until.After(hi) {
		t.Fatalf("Until %v outside expected window [%v, %v]", toast.Until, lo, hi)
	}
}

func TestToastNewAt(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toast := NewToastAt("msg", anchor)
	if !toast.Until.Equal(anchor.Add(ToastDuration)) {
		t.Fatalf("Until %v != %v", toast.Until, anchor.Add(ToastDuration))
	}
}

func TestToastExpiredNotYet(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toast := NewToastAt("msg", anchor)
	// One millisecond before deadline → not expired.
	now := toast.Until.Add(-time.Millisecond)
	if ToastExpired(toast, now) {
		t.Fatal("toast should not be expired 1ms before deadline")
	}
}

func TestToastExpiredAtDeadline(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toast := NewToastAt("msg", anchor)
	// Exactly at deadline → expired.
	if !ToastExpired(toast, toast.Until) {
		t.Fatal("toast should be expired exactly at its deadline")
	}
}

func TestToastExpiredAfterDeadline(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toast := NewToastAt("msg", anchor)
	now := toast.Until.Add(time.Second)
	if !ToastExpired(toast, now) {
		t.Fatal("toast should be expired 1s after deadline")
	}
}

func TestToastExpiredEmptyToast(t *testing.T) {
	// An empty toast (no message) is never "expired"; it is simply absent.
	empty := Toast{}
	if ToastExpired(empty, time.Now()) {
		t.Fatal("empty toast should not be considered expired")
	}
}

func TestToastClearIfExpiredKeepsLive(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toast := NewToastAt("live", anchor)
	now := toast.Until.Add(-time.Second)
	got := ClearIfExpired(toast, now)
	if got.Message != "live" {
		t.Fatalf("expected live toast to survive, got %q", got.Message)
	}
}

func TestToastClearIfExpiredClearsStale(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toast := NewToastAt("stale", anchor)
	now := toast.Until.Add(time.Second)
	got := ClearIfExpired(toast, now)
	if got.Message != "" {
		t.Fatalf("expected cleared toast, got %q", got.Message)
	}
}

func TestToastStringsDefinedAndNonEmpty(t *testing.T) {
	consts := []string{
		ToastViewedToggleFailed,
		ToastCopiedPRURL,
		ToastCopiedBranchName,
		ToastCopiedHeadSHA,
		ToastCopiedFilePath,
		ToastCopiedThreadPermalink,
		ToastCopiedCommentBody,
		ToastMultiplePendingReviews,
		ToastNoUnresolvedThreads,
		ToastNoUnresolvedThreadInFile,
		ToastSetEditorForConfig,
	}
	for _, s := range consts {
		if s == "" {
			t.Fatal("empty Phase 1 toast constant found")
		}
	}
}

func TestToastViewedToggleFailedN(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "Viewed toggle failed for 1 file(s)"},
		{3, "Viewed toggle failed for 3 file(s)"},
	}
	for _, c := range cases {
		got := ToastViewedToggleFailedN(c.n)
		if got != c.want {
			t.Errorf("ToastViewedToggleFailedN(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
