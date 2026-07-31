package keymap

import "testing"

func TestTeaKeyCrossesTheTokenBoundary(t *testing.T) {
	for token, want := range map[string]string{
		"<enter>":    "enter",
		"<space>":    "space",
		"<esc>":      "esc",
		"<up>":       "up",
		"<c-c>":      "ctrl+c",
		"<C-D>":      "ctrl+d",
		"<pgdn>":     "pgdown", // handlers compare "pgdown", not "pgdn"
		"<pgup>":     "pgup",
		"<backtab>":  "shift+tab",
		"j":          "j",
		"S":          "S",
		"zz":         "zz",
		"<disabled>": "",
	} {
		if got := TeaKey(token); got != want {
			t.Errorf("TeaKey(%q) = %q, want %q", token, got, want)
		}
	}
}

func TestCanonicalIsIdentityWithoutOverrides(t *testing.T) {
	b := Resolve(nil)
	for _, tc := range []struct{ ctx, key string }{
		{"files", "j"}, {"files", "space"}, {"main", "["}, {"main", "z"},
		{"prs", "enter"}, {"threads", "enter"}, {"thread", "r"},
	} {
		got, ok := b.Canonical(tc.ctx, tc.key)
		if !ok || got != tc.key {
			t.Errorf("default %s/%s should dispatch unchanged, got %q ok=%v", tc.ctx, tc.key, got, ok)
		}
	}
}

// The whole point of the change: a remapped key must reach the handler as the
// key the handler switches on, and the vacated default must stop working.
func TestCanonicalTranslatesRemappedKey(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		"universal": {"cursorDown": {"x"}},
	})

	if got, ok := b.Canonical("files", "x"); !ok || got != "j" {
		t.Fatalf("remapped x should arrive as j, got %q ok=%v", got, ok)
	}
	if _, ok := b.Canonical("files", "j"); ok {
		t.Fatal("j was vacated by the remap and must be swallowed, not still work")
	}
	// <down> was a second default of the same action, so it goes too.
	if _, ok := b.Canonical("files", "down"); ok {
		t.Fatal("down was a default of cursorDown and must be swallowed as well")
	}
}

func TestDisabledUnbindsTheAction(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		"files": {"toggleViewed": {"<disabled>"}},
	})

	if _, ok := b.Canonical("files", "space"); ok {
		t.Fatal("a disabled action must not leave its key working")
	}
	if keys := b.DisplayKeys("files", "toggleViewed"); len(keys) != 0 {
		t.Fatalf("a disabled action must advertise no keys in ?, got %v", keys)
	}
}

// `[`, `]`, `enter`, `space`, `h`, `l`, `v`, `t`, `c`, `-` and `=` are declared in
// universal AND in a specific context. Moving the universal one must not disturb
// the context one.
func TestContextBindingSurvivesUniversalRemap(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		"universal": {"prevTab": {"p"}},
	})

	if got, ok := b.Canonical("main", "["); !ok || got != "[" {
		t.Fatalf("main binds [ to prevFile; a universal prevTab remap must not touch it, got %q ok=%v", got, ok)
	}
	if _, ok := b.Canonical("threads", "["); ok {
		t.Fatal("outside main, [ belonged to universal prevTab and should now be swallowed")
	}
}

func TestOwnerReportsClaimingScope(t *testing.T) {
	b := Resolve(nil)

	if action, scope, ok := b.Owner("main", "h"); !ok || action != "prevHunk" || scope != "main" {
		t.Fatalf("h in main is prevHunk/main, got %q/%q ok=%v", action, scope, ok)
	}
	if action, scope, ok := b.Owner("files", "h"); !ok || action != "prevPanel" || scope != ContextUniversal {
		t.Fatalf("h outside main falls to universal prevPanel, got %q/%q ok=%v", action, scope, ok)
	}
}

// centerCursor is `zz`, which ValidateKey cannot express, so it is deliberately
// not remappable. Remapping the single-key `z` action off z must NOT make the
// sequence unreachable.
func TestSequencePrefixSurvivesRemap(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		"main": {"toggleThreadFold": {"f"}},
	})

	if got, ok := b.Canonical("main", "f"); !ok || got != "z" {
		t.Fatalf("remapped fold key should arrive as z, got %q ok=%v", got, ok)
	}
	if got, ok := b.Canonical("main", "z"); !ok || got != "z" {
		t.Fatalf("z still opens the zz sequence and must not be swallowed, got %q ok=%v", got, ok)
	}
}

// Keys the handlers accept directly but the keymap never declared (aliases and
// modal keys) must pass through untouched.
func TestUnmodeledKeysPassThrough(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		"universal": {"cursorDown": {"x"}},
	})
	for _, key := range []string{"pgdn", "end", "ctrl+s"} {
		if got, ok := b.Canonical("files", key); !ok || got != key {
			t.Errorf("%q is not modeled in files and must pass through, got %q ok=%v", key, got, ok)
		}
	}
}

// Uses `refresh` rather than `quit`: quit always retains the ctrl+c escape hatch, so
// it is the wrong vehicle for asserting an exact one-key list.
func TestDisplayKeysAreCanonicalAndEffective(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		"universal": {"refresh": {"<C-X>"}},
	})

	got := b.DisplayKeys(ContextUniversal, "refresh")
	if len(got) != 1 || got[0] != "<c-x>" {
		t.Fatalf("? should advertise the remapped key in canonical bracket form, got %v", got)
	}
	if keys := b.Keys(ContextUniversal, "refresh"); len(keys) != 1 || keys[0] != "ctrl+x" {
		t.Fatalf("dispatch needs the tea form, got %v", keys)
	}
}

// ctrl+c is honoured unconditionally by dispatch, so the table must keep advertising
// it: `?` hiding a binding that still works is the same lie the effective table
// exists to remove. `quit: <disabled>` means "stop q quitting", not "unquittable".
func TestDisabledQuitKeepsEscapeHatchVisible(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		ContextUniversal: {QuitAction: {"<disabled>"}},
	})

	display := b.DisplayKeys(ContextUniversal, QuitAction)
	if len(display) != 1 || display[0] != "<c-c>" {
		t.Fatalf("? must still advertise ctrl+c for quit, got %v", display)
	}
	if got, ok := b.Canonical("files", "ctrl+c"); !ok || got != "q" {
		t.Fatalf("ctrl+c must still resolve to the quit handler, got %q ok=%v", got, ok)
	}
	if _, ok := b.Canonical("files", "q"); ok {
		t.Fatal("q was disabled and must be swallowed")
	}
}

// Remapping quit keeps the hatch too, alongside the new key.
func TestRemappedQuitKeepsEscapeHatch(t *testing.T) {
	b := Resolve(map[string]map[string][]string{
		ContextUniversal: {QuitAction: {"Z"}},
	})

	if keys := b.Keys(ContextUniversal, QuitAction); len(keys) != 2 || keys[0] != "Z" || keys[1] != "ctrl+c" {
		t.Fatalf("quit should dispatch on the remap plus the hatch, got %v", keys)
	}
}
