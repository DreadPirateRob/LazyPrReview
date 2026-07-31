package keymap

import "testing"

func TestAuthoringActionsRegistered(t *testing.T) {
	bindings := DefaultBindings()
	if _, ok := bindings["thread"]; !ok {
		t.Fatalf("expected thread context to exist")
	}
	// Phase 2 authoring must be registered under the contexts where it actually
	// works, so `?` help and the hint bar surface it (scoped help shows the
	// active context plus universal).
	for _, tc := range []struct{ ctx, action string }{
		{"universal", "submitReview"},
		{"main", "commentLine"},
		{"main", "rangeSelect"},
		{"files", "commentFile"},
		{"threads", "resolveThread"},
		{"thread", "replyThread"},
		{"thread", "editComment"},
		{"thread", "deleteComment"},
	} {
		if !KnownAction(tc.ctx, tc.action) {
			t.Errorf("authoring action %s/%s not registered", tc.ctx, tc.action)
		}
	}
	if !KnownAction("main", "nextUnresolvedThread") {
		t.Fatalf("expected phase 1 main action to be registered")
	}
}

func TestValidateKey(t *testing.T) {
	valid := []string{"q", "?", "<c-c>", "<enter>", "<disabled>"}
	for _, key := range valid {
		if err := ValidateKey(key); err != nil {
			t.Fatalf("expected %q to be valid: %v", key, err)
		}
	}
	if err := ValidateKey("ctrl+c"); err == nil {
		t.Fatalf("expected invalid syntax to fail")
	}
}
