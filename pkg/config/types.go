package config

type Config struct {
	GUI       GUIConfig                    `yaml:"gui"`
	GitHub    GitHubConfig                 `yaml:"github"`
	OS        OSConfig                     `yaml:"os"`
	Keybinding map[string]map[string]any   `yaml:"keybinding"`
}

type GUIConfig struct {
	SidePanelWidth         float64     `yaml:"sidePanelWidth"`
	ExpandFocusedSidePanel bool        `yaml:"expandFocusedSidePanel"`
	CommandLogSize         int         `yaml:"commandLogSize"`
	ScreenMode             string      `yaml:"screenMode"`
	FilterMode             string      `yaml:"filterMode"`
	NerdFontsVersion       string      `yaml:"nerdFontsVersion"`
	Theme                  ThemeConfig `yaml:"theme"`
	ShowBottomLine         bool        `yaml:"showBottomLine"`
	// DiffPager pipes each file's raw diff through an external renderer
	// (e.g. "delta --paging=never") instead of the built-in diff view.
	DiffPager              string      `yaml:"diffPager"`
}

type ThemeConfig struct {
	ActiveBorderColor   []string `yaml:"activeBorderColor"`
	InactiveBorderColor []string `yaml:"inactiveBorderColor"`
	SelectedLineBgColor []string `yaml:"selectedLineBgColor"`
}

type GitHubConfig struct {
	AutoRefreshInterval int    `yaml:"autoRefreshInterval"`
	DefaultMergeMethod  string `yaml:"defaultMergeMethod"`
	DeleteBranchOnMerge bool   `yaml:"deleteBranchOnMerge"`
}

type OSConfig struct {
	EditPreset string `yaml:"editPreset"`
	Edit       string `yaml:"edit"`
	OpenLink   string `yaml:"openLink"`
}

func Default() Config {
	return Config{
		GUI: GUIConfig{
			SidePanelWidth:         0.3333,
			ExpandFocusedSidePanel: true,
			CommandLogSize:         8,
			ScreenMode:             "normal",
			FilterMode:             "substring",
			NerdFontsVersion:       "3",
			Theme: ThemeConfig{
				ActiveBorderColor:   []string{"green", "bold"},
				InactiveBorderColor: []string{"default"},
				SelectedLineBgColor: []string{"blue"},
			},
			ShowBottomLine: true,
		},
		GitHub: GitHubConfig{
			AutoRefreshInterval: 60,
			DefaultMergeMethod:  "squash",
			DeleteBranchOnMerge: true,
		},
		OS: OSConfig{},
		Keybinding: map[string]map[string]any{
			"universal": {},
			"status":    {},
			"prs":       {},
			"files":     {},
			"threads":   {},
			"thread":    {},
			"checks":    {},
			"main":      {},
		},
	}
}
