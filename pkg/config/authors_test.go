package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadAuthorsFromMissingFile verifies the first-run contract: a file that
// does not yet exist is not an error.
func TestLoadAuthorsFromMissingFile(t *testing.T) {
	dir := t.TempDir()
	roles, err := LoadAuthorsFrom(filepath.Join(dir, "authors.yml"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if roles == nil {
		t.Fatal("expected non-nil map for missing file")
	}
	if len(roles) != 0 {
		t.Fatalf("expected empty map, got %v", roles)
	}
}

// TestSaveLoadRoundTrip verifies that all valid entries survive a save→load
// cycle without mutation.
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authors.yml")

	want := map[string]AuthorRole{
		"github-actions":          RoleBot,
		"dependabot":              RoleBot,
		"chatgpt-codex-connector": RoleBot,
		"alice":                   RoleHuman,
	}

	if err := SaveAuthorsTo(path, want); err != nil {
		t.Fatalf("SaveAuthorsTo: %v", err)
	}

	got, err := LoadAuthorsFrom(path)
	if err != nil {
		t.Fatalf("LoadAuthorsFrom: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("length mismatch: want %d got %d entries (%v)", len(want), len(got), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q: want %q got %q", k, v, got[k])
		}
	}
}

// TestSaveAuthorsCreatesParentDirs verifies that SaveAuthorsTo creates
// intermediate directories on first run, matching the expected user experience.
func TestSaveAuthorsCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "authors.yml")

	if err := SaveAuthorsTo(path, map[string]AuthorRole{"bot-a": RoleBot}); err != nil {
		t.Fatalf("SaveAuthorsTo with nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist after save: %v", err)
	}
}

// TestLoadAuthorsNormalizesKeys verifies that keys are lowercased and trimmed
// both when saved programmatically and when loaded from a hand-edited file.
func TestLoadAuthorsNormalizesKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authors.yml")

	// Pass mixed-case keys via Save; they must come back lowercased.
	if err := SaveAuthorsTo(path, map[string]AuthorRole{"GitHub-Actions": RoleBot}); err != nil {
		t.Fatalf("SaveAuthorsTo: %v", err)
	}
	roles, err := LoadAuthorsFrom(path)
	if err != nil {
		t.Fatalf("LoadAuthorsFrom after save: %v", err)
	}
	if roles["github-actions"] != RoleBot {
		t.Errorf("expected normalized key github-actions=bot, got map %v", roles)
	}
	if _, ok := roles["GitHub-Actions"]; ok {
		t.Error("un-normalized key must not be present after save")
	}

	// Simulate a user hand-editing the file with mixed-case keys.
	const handEdited = "authors:\n  Dependabot: human\n  COPILOT: bot\n"
	if err := os.WriteFile(path, []byte(handEdited), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	roles2, err := LoadAuthorsFrom(path)
	if err != nil {
		t.Fatalf("LoadAuthorsFrom hand-edited: %v", err)
	}
	if roles2["dependabot"] != RoleHuman {
		t.Errorf("expected dependabot=human after hand-edit load, got %v", roles2)
	}
	if roles2["copilot"] != RoleBot {
		t.Errorf("expected copilot=bot after hand-edit load, got %v", roles2)
	}
}

// TestLoadAuthorsUnknownRoleDropped verifies that a single typo in a role
// value is silently discarded while valid sibling entries survive.
func TestLoadAuthorsUnknownRoleDropped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authors.yml")

	const raw = "authors:\n  good-bot: bot\n  typo-entry: robots\n  good-human: human\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	roles, err := LoadAuthorsFrom(path)
	if err != nil {
		t.Fatalf("LoadAuthorsFrom: %v", err)
	}
	if roles["good-bot"] != RoleBot {
		t.Errorf("good-bot should survive unknown-role drop: got %q", roles["good-bot"])
	}
	if roles["good-human"] != RoleHuman {
		t.Errorf("good-human should survive unknown-role drop: got %q", roles["good-human"])
	}
	if _, ok := roles["typo-entry"]; ok {
		t.Error("entry with unknown role value must be dropped")
	}
}

// TestLoadAuthorsMalformedYAML verifies that a hand-edited file containing
// invalid YAML surfaces an error rather than silently returning an empty map.
func TestLoadAuthorsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authors.yml")

	if err := os.WriteFile(path, []byte(":\t: bad yaml {{\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadAuthorsFrom(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

// TestSaveAuthorsEmptyMap verifies that saving an empty map is legal and that
// a subsequent load returns an empty (not nil) map — the inverse of flagging.
func TestSaveAuthorsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authors.yml")

	if err := SaveAuthorsTo(path, map[string]AuthorRole{}); err != nil {
		t.Fatalf("SaveAuthorsTo empty map: %v", err)
	}

	roles, err := LoadAuthorsFrom(path)
	if err != nil {
		t.Fatalf("LoadAuthorsFrom after empty save: %v", err)
	}
	if roles == nil {
		t.Fatal("expected non-nil map after empty save")
	}
	if len(roles) != 0 {
		t.Fatalf("expected empty map after empty save, got %v", roles)
	}
}

// TestSaveAuthorsAtomicNoTempFiles verifies that no partial or temporary files
// are left in the directory after a successful save.
func TestSaveAuthorsAtomicNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authors.yml")

	if err := SaveAuthorsTo(path, map[string]AuthorRole{"bot-x": RoleBot}); err != nil {
		t.Fatalf("SaveAuthorsTo: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "authors.yml" {
			t.Errorf("unexpected file left in dir after save: %q", e.Name())
		}
	}
}

// TestAuthorsPathSuffix verifies that AuthorsPath returns a path rooted under
// the lazypr config directory — the same neighbourhood as config.yml.
func TestAuthorsPathSuffix(t *testing.T) {
	p := AuthorsPath()
	want := filepath.Join("lazypr", "authors.yml")
	if !strings.HasSuffix(p, want) {
		t.Errorf("AuthorsPath() = %q; want suffix %q", p, want)
	}
}
