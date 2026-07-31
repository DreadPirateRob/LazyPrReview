package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPrecedenceRepoWins(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.yml")
	local := filepath.Join(dir, ".lazypr.yml")
	mustWrite(t, global, "gui:\n  screenMode: half\n")
	mustWrite(t, local, "gui:\n  screenMode: fullscreen\n")

	cfg, err := LoadFrom(global, func() (string, bool) { return local, true })
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if cfg.GUI.ScreenMode != ScreenModeFullscreen {
		t.Fatalf("expected repo-local screenMode to win, got %q", cfg.GUI.ScreenMode)
	}
}

func TestLoadExplicitEmptyOverride(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.yml")
	local := filepath.Join(dir, ".lazypr.yml")
	mustWrite(t, global, "os:\n  openLink: open\n")
	mustWrite(t, local, "os:\n  openLink: \"\"\n")

	cfg, err := LoadFrom(global, func() (string, bool) { return local, true })
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if cfg.OS.OpenLink != "" {
		t.Fatalf("expected explicit empty override, got %q", cfg.OS.OpenLink)
	}
}

func TestLoadInvalidKeySyntax(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.yml")
	mustWrite(t, global, "keybinding:\n  universal:\n    quit: ctrl+c\n")

	_, err := LoadFrom(global, func() (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "invalid key syntax") {
		t.Fatalf("expected invalid key syntax error, got %v", err)
	}
}

func TestLoadDuplicateKeyConflict(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.yml")
	mustWrite(t, global, "keybinding:\n  universal:\n    quit: q\n    showHelp: q\n")

	_, err := LoadFrom(global, func() (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "mapped to both") {
		t.Fatalf("expected duplicate mapping error, got %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
