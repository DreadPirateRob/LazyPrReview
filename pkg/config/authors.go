package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

// AuthorRole classifies a comment author as a bot or a human.
type AuthorRole string

const (
	RoleBot   AuthorRole = "bot"
	RoleHuman AuthorRole = "human"
)

// authorsFile is the on-disk representation. A named top-level key makes the
// YAML self-documenting and leaves room for future metadata fields without
// breaking existing readers.
type authorsFile struct {
	Authors map[string]string `yaml:"authors"`
}

// AuthorsPath returns the canonical path for the author-role override file.
// It sits beside config.yml so the user finds both in one directory.
func AuthorsPath() string {
	return filepath.Join(xdg.ConfigHome, "lazypr", "authors.yml")
}

// LoadAuthors reads the role map from AuthorsPath.
func LoadAuthors() (map[string]AuthorRole, error) {
	return LoadAuthorsFrom(AuthorsPath())
}

// LoadAuthorsFrom reads the role map from an explicit path.
//
// A missing file is normal first-run state — callers never need to
// special-case os.IsNotExist; an empty non-nil map is returned instead.
// Malformed YAML is surfaced as an error. Unknown role values are silently
// dropped so a single typo does not discard the rest of the file.
func LoadAuthorsFrom(path string) (map[string]AuthorRole, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]AuthorRole{}, nil
		}
		return nil, fmt.Errorf("authors: read %s: %w", path, err)
	}

	var raw authorsFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("authors: parse %s: %w", path, err)
	}

	out := make(map[string]AuthorRole, len(raw.Authors))
	for k, v := range raw.Authors {
		// GitHub logins are case-insensitive; normalize so TUI lookups are
		// consistent regardless of how the server returned the login string.
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		role := AuthorRole(v)
		if role != RoleBot && role != RoleHuman {
			continue
		}
		out[key] = role
	}
	return out, nil
}

// SaveAuthors writes the role map to AuthorsPath.
func SaveAuthors(roles map[string]AuthorRole) error {
	return SaveAuthorsTo(AuthorsPath(), roles)
}

// SaveAuthorsTo writes the role map to an explicit path.
//
// The write is atomic: content is staged to a sibling temp file and then
// renamed, so an interrupted write never truncates the user's existing flags.
// Parent directories are created as needed.
func SaveAuthorsTo(path string, roles map[string]AuthorRole) error {
	dir := filepath.Dir(path)
	// 0o755: config dirs need to be searchable by the owning user and readable
	// by any tooling that inspects the XDG config hierarchy.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("authors: create dir %s: %w", dir, err)
	}

	raw := authorsFile{Authors: make(map[string]string, len(roles))}
	for k, v := range roles {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		raw.Authors[key] = string(v)
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("authors: marshal: %w", err)
	}

	// Prepend a comment so a user who discovers the file knows what it is and
	// that it is safe to hand-edit.
	const header = "# Managed by lazypr. Safe to hand-edit.\n"
	content := make([]byte, 0, len(header)+len(data))
	content = append(content, header...)
	content = append(content, data...)

	tmp, err := os.CreateTemp(dir, ".authors-*.yml.tmp")
	if err != nil {
		return fmt.Errorf("authors: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("authors: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("authors: close temp: %w", err)
	}
	// os.CreateTemp creates files with mode 0600; promote to 0644 so the file
	// is consistent with other user config files and readable by group/other.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("authors: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("authors: rename temp: %w", err)
	}
	return nil
}
