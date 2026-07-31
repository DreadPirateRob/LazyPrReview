package ui

import "fmt"

// fatalView renders the full-screen error surface used for the three startup
// failure modes. The only valid actions from this screen are q and ctrl+c.
func fatalView(m Model) string {
	return fmt.Sprintf(
		"\n\n  %s\n\n  %s\n\n  Press q or ctrl+c to exit.\n",
		fatalTitle(m.Fatal),
		fatalFix(m.Fatal),
	)
}

func fatalTitle(kind FatalKind) string {
	switch kind {
	case FatalMissingGH:
		return "Error: gh not found"
	case FatalUnauthed:
		return "Error: gh is not authenticated"
	case FatalNoRepo:
		return "Error: no GitHub repository resolvable"
	default:
		return "Error: unknown fatal condition"
	}
}

func fatalFix(kind FatalKind) string {
	switch kind {
	case FatalMissingGH:
		return "Fix: install gh — https://cli.github.com"
	case FatalUnauthed:
		return "Fix: run  gh auth login"
	case FatalNoRepo:
		return "Fix: pass  --repo owner/name  or run lazypr from a GitHub repo directory"
	default:
		return ""
	}
}
