package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"

	"github.com/DreadPirateRob/LazyPrReview/pkg/ui/keymap"
)

const (
	ScreenModeNormal     = "normal"
	ScreenModeHalf       = "half"
	ScreenModeFullscreen = "fullscreen"
	FilterModeSubstring  = "substring"
	FilterModeFuzzy      = "fuzzy"
)

func GlobalPath() string {
	return filepath.Join(xdg.ConfigHome, "lazypr", "config.yml")
}

func Load() (Config, error) {
	return LoadFrom(GlobalPath(), repoLocalPath)
}

func LoadFrom(globalPath string, repoLocal func() (string, bool)) (Config, error) {
	merged, err := marshalDefaultMap()
	if err != nil {
		return Config{}, err
	}
	if fileMap, err := readConfigMap(globalPath); err != nil {
		return Config{}, err
	} else if fileMap != nil {
		merged, err = mergeMaps(merged, fileMap)
		if err != nil {
			return Config{}, err
		}
	}
	if localPath, ok := repoLocal(); ok {
		fileMap, err := readConfigMap(localPath)
		if err != nil {
			return Config{}, err
		}
		if fileMap != nil {
			merged, err = mergeMaps(merged, fileMap)
			if err != nil {
				return Config{}, err
			}
		}
	}
	cfg, err := decodeConfigMap(merged)
	if err != nil {
		return Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Validate(cfg Config) error {
	switch cfg.GUI.ScreenMode {
	case ScreenModeNormal, ScreenModeHalf, ScreenModeFullscreen:
	default:
		return fmt.Errorf("unknown screenMode %q", cfg.GUI.ScreenMode)
	}
	switch cfg.GUI.FilterMode {
	case FilterModeSubstring, FilterModeFuzzy:
	default:
		return fmt.Errorf("unknown filterMode %q", cfg.GUI.FilterMode)
	}
	for name, arr := range map[string][]string{
		"activeBorderColor":   cfg.GUI.Theme.ActiveBorderColor,
		"inactiveBorderColor": cfg.GUI.Theme.InactiveBorderColor,
		"selectedLineBgColor": cfg.GUI.Theme.SelectedLineBgColor,
	} {
		if len(arr) == 0 {
			return fmt.Errorf("invalid color array %s", name)
		}
		for _, item := range arr {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("invalid color array %s", name)
			}
		}
	}
	for ctx, mapping := range cfg.Keybinding {
		if !keymap.KnownContext(ctx) {
			return fmt.Errorf("unknown keybinding context %q", ctx)
		}
		seen := map[string]string{}
		for action, raw := range mapping {
			if !keymap.KnownAction(ctx, action) {
				return fmt.Errorf("unknown keybinding action %s.%s", ctx, action)
			}
			keys, err := normalizeBinding(raw)
			if err != nil {
				return fmt.Errorf("invalid keybinding %s.%s: %w", ctx, action, err)
			}
			for _, key := range keys {
				if key == "<disabled>" {
					continue
				}
				if prior, ok := seen[strings.ToLower(key)]; ok {
					return fmt.Errorf("key %q mapped to both %s and %s in %s", key, prior, action, ctx)
				}
				seen[strings.ToLower(key)] = action
			}
		}
	}
	return nil
}

// KeybindingOverrides returns the user's remaps as context → action → keys, with
// the `string | []string` YAML shapes flattened. Validate has already rejected
// unknown contexts/actions and duplicate keys before this runs, so a binding that
// still fails to normalize is dropped rather than reported again — a malformed
// entry must not take down startup.
//
// keymap owns the token→Bubble Tea translation; this only normalizes YAML shape.
func KeybindingOverrides(cfg Config) map[string]map[string][]string {
	if len(cfg.Keybinding) == 0 {
		return nil
	}
	out := make(map[string]map[string][]string, len(cfg.Keybinding))
	for ctx, mapping := range cfg.Keybinding {
		for action, raw := range mapping {
			keys, err := normalizeBinding(raw)
			if err != nil {
				continue
			}
			if out[ctx] == nil {
				out[ctx] = make(map[string][]string, len(mapping))
			}
			out[ctx][action] = keys
		}
	}
	return out
}

func FilterMatch(mode, value, query string) bool {
	query = strings.ToLower(query)
	value = strings.ToLower(value)
	if query == "" {
		return true
	}
	switch mode {
	case FilterModeFuzzy:
		return fuzzyMatch(value, query)
	default:
		return strings.Contains(value, query)
	}
}

func repoLocalPath() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = wd
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}
	return filepath.Join(root, ".lazypr.yml"), true
}

func marshalDefaultMap() (map[string]any, error) {
	b, err := yaml.Marshal(Default())
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func readConfigMap(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out map[string]any
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func mergeMaps(base, overlay map[string]any) (map[string]any, error) {
	result := map[string]any{}
	if err := mergo.Merge(&result, base, mergo.WithOverride, mergo.WithOverwriteWithEmptyValue); err != nil {
		return nil, err
	}
	for key, value := range overlay {
		if baseChild, ok := result[key].(map[string]any); ok {
			if overlayChild, ok := value.(map[string]any); ok {
				mergedChild, err := mergeMaps(baseChild, overlayChild)
				if err != nil {
					return nil, err
				}
				result[key] = mergedChild
				continue
			}
		}
		result[key] = value
	}
	return result, nil
}

func decodeConfigMap(raw map[string]any) (Config, error) {
	b, err := yaml.Marshal(raw)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalizeBinding(raw any) ([]string, error) {
	switch v := raw.(type) {
	case string:
		if err := keymap.ValidateKey(v); err != nil {
			return nil, err
		}
		return []string{v}, nil
	case []any:
		keys := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("binding entries must be strings")
			}
			if err := keymap.ValidateKey(s); err != nil {
				return nil, err
			}
			keys = append(keys, s)
		}
		return keys, nil
	case []string:
		keys := append([]string(nil), v...)
		for _, item := range keys {
			if err := keymap.ValidateKey(item); err != nil {
				return nil, err
			}
		}
		return keys, nil
	default:
		return nil, fmt.Errorf("binding must be string or list of strings")
	}
}

func fuzzyMatch(value, query string) bool {
	if query == "" {
		return true
	}
	runes := []rune(value)
	needle := []rune(query)
	idx := 0
	for _, r := range runes {
		if r == needle[idx] {
			idx++
			if idx == len(needle) {
				return true
			}
		}
	}
	return false
}
