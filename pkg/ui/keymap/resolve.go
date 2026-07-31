package keymap

import "strings"

// ContextUniversal holds the bindings that apply in every context.
const ContextUniversal = "universal"

// QuitAction is the action whose binding exits the program.
const QuitAction = "quit"

// hardQuitKey is honoured unconditionally by dispatch so a config cannot lock the
// user in. The effective table therefore keeps it attached to `quit` even when the
// action is remapped or disabled — dropping it would let `?` and the hint bar hide
// a binding that still works, which is the same lie this table exists to remove.
const hardQuitKey = "<c-c>"

// TeaKey converts a binding token into the form the TUI compares against.
//
// Two syntaxes exist and they are not the same: the action table and config.yml
// speak in bracketed tokens (`<enter>`, `<space>`, `<c-c>`), while key handlers
// compare the strings Bubble Tea reports (`enter`, `space`, `ctrl+c`). Every
// binding has to cross that boundary exactly once, here, or a remap of a special
// key silently fails to match anything.
//
// `<disabled>` maps to "" — the caller treats that as unbound.
func TeaKey(token string) string {
	if token == "" || token == "<disabled>" {
		return ""
	}
	lower := strings.ToLower(token)
	if !strings.HasPrefix(lower, "<") || !strings.HasSuffix(lower, ">") {
		// A single rune, or a multi-rune sequence like "zz", passes through as-is.
		return token
	}
	name := lower[1 : len(lower)-1]
	if rest, ok := strings.CutPrefix(name, "c-"); ok {
		return "ctrl+" + rest
	}
	switch name {
	case "pgdn":
		return "pgdown"
	case "backtab":
		return "shift+tab"
	default:
		// esc, enter, space, tab, up, down, left, right, pgup, home, end.
		return name
	}
}

// Bindings is an effective key table: the shipped defaults with the user's
// config.yml overrides applied, expressed in Bubble Tea token form.
type Bindings struct {
	// keys[ctx][action] = effective keys in Bubble Tea form, for DISPATCH.
	keys map[string]map[string][]string
	// display[ctx][action] = the same keys as written in config/defaults
	// (`<enter>`, `<c-c>`), for `?` help and the hint bar. The two forms are
	// deliberately separate: dispatch has to match what Bubble Tea reports, while
	// the UI has always shown the bracketed syntax users type into config.yml.
	display map[string]map[string][]string
	// owner[ctx][key] = the action that key currently triggers.
	owner map[string]map[string]string
	// canonical[ctx][action] = the action's FIRST shipped default key. Handlers
	// keep switching on these, so this is what a pressed key is translated to.
	canonical map[string]map[string]string
	// shipped[ctx][key] = an action that shipped with this key by default,
	// whether or not it still holds it.
	shipped map[string]map[string]string
	// sequences[ctx] = multi-rune defaults such as "zz", whose first key must
	// never be swallowed or the sequence becomes unreachable.
	sequences map[string][]string
}

// Resolve builds the effective table. overrides is context → action → keys in
// config token form; an action absent from overrides keeps its shipped keys, and
// a `<disabled>` entry leaves the action with none.
func Resolve(overrides map[string]map[string][]string) *Bindings {
	b := &Bindings{
		keys:      make(map[string]map[string][]string, len(knownContexts)),
		display:   make(map[string]map[string][]string, len(knownContexts)),
		owner:     make(map[string]map[string]string, len(knownContexts)),
		canonical: make(map[string]map[string]string, len(knownContexts)),
		shipped:   make(map[string]map[string]string, len(knownContexts)),
		sequences: make(map[string][]string, len(knownContexts)),
	}
	for ctx := range knownContexts {
		b.keys[ctx] = map[string][]string{}
		b.display[ctx] = map[string][]string{}
		b.owner[ctx] = map[string]string{}
		b.canonical[ctx] = map[string]string{}
		b.shipped[ctx] = map[string]string{}
	}

	for ctx, list := range byContext {
		for _, action := range list {
			for i, token := range action.Keys {
				key := TeaKey(token)
				if key == "" {
					continue
				}
				if i == 0 {
					b.canonical[ctx][action.Name] = key
				}
				if _, taken := b.shipped[ctx][key]; !taken {
					b.shipped[ctx][key] = action.Name
				}
				if isSequence(key) {
					b.sequences[ctx] = append(b.sequences[ctx], key)
				}
			}

			effective := action.Keys
			if override, ok := overrides[ctx][action.Name]; ok {
				effective = override
			}
			if ctx == ContextUniversal && action.Name == QuitAction {
				effective = withHardQuitKey(effective)
			}
			for _, token := range effective {
				key := TeaKey(token)
				if key == "" {
					// `<disabled>`: contributes no dispatch key and nothing to
					// advertise, which is what makes the action disappear from `?`.
					continue
				}
				b.keys[ctx][action.Name] = append(b.keys[ctx][action.Name], key)
				b.display[ctx][action.Name] = append(b.display[ctx][action.Name], DisplayKey(token))
				if _, taken := b.owner[ctx][key]; !taken {
					b.owner[ctx][key] = action.Name
				}
			}
		}
	}
	return b
}

// withHardQuitKey guarantees the escape hatch is part of quit's effective keys.
// `quit: <disabled>` therefore means "stop `q` quitting", not "make the program
// unquittable" — and `?` still shows the key that does.
func withHardQuitKey(keys []string) []string {
	for _, k := range keys {
		if TeaKey(k) == TeaKey(hardQuitKey) {
			return keys
		}
	}
	return append(append([]string(nil), keys...), hardQuitKey)
}

// DisplayKey canonicalizes a binding token for presentation: bracketed names are
// lowercased so a config written `<C-C>` reads the same as `<c-c>` in `?`, while
// single runes keep their case because `S` and `s` are different bindings.
func DisplayKey(token string) string {
	if len([]rune(token)) > 1 && strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">") {
		return strings.ToLower(token)
	}
	return token
}

// Keys returns the effective dispatch keys for an action, in Bubble Tea form.
func (b *Bindings) Keys(ctx, action string) []string {
	if b == nil {
		return nil
	}
	return b.keys[ctx][action]
}

// DisplayKeys returns the effective keys as written in config/defaults, so `?`
// help and the hint bar advertise what actually works rather than what shipped.
func (b *Bindings) DisplayKeys(ctx, action string) []string {
	if b == nil {
		return nil
	}
	return b.display[ctx][action]
}

// Owner reports which action a pressed key triggers and the scope that claims it.
// Callers use the scope to avoid shadowing: a universal handler must not consume
// `h` when the active context binds it to something of its own.
func (b *Bindings) Owner(ctx, pressed string) (action, scope string, ok bool) {
	if b == nil || pressed == "" {
		return "", "", false
	}
	for _, s := range b.scopes(ctx) {
		if a, found := b.owner[s][pressed]; found {
			return a, s, true
		}
	}
	return "", "", false
}

// Canonical translates a pressed key into the shipped default key of whatever
// action currently owns it, letting handlers keep their literal switch cases.
//
// ok=false means swallow the key: it shipped as a default in this scope but
// config moved that action elsewhere, so honouring it would make the remap a lie.
func (b *Bindings) Canonical(ctx, pressed string) (string, bool) {
	if b == nil || pressed == "" {
		return pressed, true
	}
	// Resolve scope by scope, deciding each one fully before falling outward.
	//
	// Several keys are declared twice at two granularities — universal
	// `togglePrimary` and files `toggleViewed` both claim `<space>`, universal
	// `open` and files `openDiff` both claim `<enter>`. They describe ONE
	// behaviour, so the narrower declaration is authoritative: if the context
	// declared this key and config has since unbound it, the key is dead here even
	// though the universal alias still nominally holds it. Checking every scope's
	// owners before any scope's defaults would let that alias resurrect a
	// binding the user explicitly disabled.
	for _, scope := range b.scopes(ctx) {
		if action, found := b.owner[scope][pressed]; found {
			if canonical := b.canonical[scope][action]; canonical != "" {
				return canonical, true
			}
			return pressed, true
		}
		if _, shipped := b.shipped[scope][pressed]; shipped {
			if b.prefixesSequence(scope, pressed) {
				// `z` opens the `zz` sequence as well as owning a binding of its
				// own. Sequences are not remappable (ValidateKey rejects
				// multi-rune keys), so the prefix has to survive regardless.
				return pressed, true
			}
			return "", false
		}
	}
	// Not modeled in this context: aliases the handlers accept directly (`pgdn`,
	// `end`) and modal keys (composer, menus) pass through untouched.
	return pressed, true
}

func (b *Bindings) scopes(ctx string) []string {
	if ctx == ContextUniversal {
		return []string{ContextUniversal}
	}
	return []string{ctx, ContextUniversal}
}

func (b *Bindings) prefixesSequence(ctx, pressed string) bool {
	for _, seq := range b.sequences[ctx] {
		if seq != pressed && strings.HasPrefix(seq, pressed) {
			return true
		}
	}
	return false
}

// isSequence reports whether a normalized token is a multi-key sequence such as
// "zz", rather than a single rune or a named/modified key.
func isSequence(key string) bool {
	if len([]rune(key)) < 2 || strings.Contains(key, "+") {
		return false
	}
	switch key {
	case "esc", "enter", "space", "tab", "up", "down", "left", "right",
		"pgup", "pgdown", "home", "end":
		return false
	}
	return true
}
