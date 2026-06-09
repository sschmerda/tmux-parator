package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileMissingUsesDefaults(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.UI.Theme != "shades-of-purple" || cfg.UI.PopupWidth != "90%" || cfg.UI.PopupHeight != "90%" {
		t.Fatalf("defaults not applied: %#v", cfg.UI)
	}
	if !cfg.Discovery.SkipHidden || len(cfg.Discovery.SkipDirs) == 0 {
		t.Fatalf("discovery defaults not applied: %#v", cfg.Discovery)
	}
	if cfg.Discovery.Backend != "auto" {
		t.Fatalf("discovery backend = %q, want auto", cfg.Discovery.Backend)
	}
}

func TestDefaultComesFromEmbeddedTOML(t *testing.T) {
	cfg := Default()
	if cfg.UI.Theme != "shades-of-purple" {
		t.Fatalf("theme = %q, want embedded default", cfg.UI.Theme)
	}
	if cfg.UI.Glyphs.Repo != "\ue702" || cfg.UI.Glyphs.Subdir == "" || cfg.UI.Glyphs.Worktree == "" || cfg.UI.Glyphs.Manual == "" {
		t.Fatalf("glyph defaults not applied: %#v", cfg.UI.Glyphs)
	}
	if cfg.UI.GlyphColors.Repo != "#f14e32" || cfg.UI.GlyphColors.Subdir == "" || cfg.UI.GlyphColors.Worktree == "" || cfg.UI.GlyphColors.Manual == "" {
		t.Fatalf("glyph color defaults not applied: %#v", cfg.UI.GlyphColors)
	}
	if !cfg.UI.Columns.Chip.Show || cfg.UI.Columns.Chip.Width != 12 || cfg.UI.Columns.Chip.MaxWidth != 12 || !cfg.UI.Columns.Root.Show || cfg.UI.Columns.Root.Width != 12 || cfg.UI.Columns.Root.MaxWidth != 20 || !cfg.UI.Columns.Name.Show || cfg.UI.Columns.Name.Width != 28 || cfg.UI.Columns.Name.MaxWidth != 40 || !cfg.UI.Columns.Path.Show || cfg.UI.Columns.Path.MaxWidth != 0 {
		t.Fatalf("column defaults not applied: %#v", cfg.UI.Columns)
	}
	if !cfg.Discovery.SkipHidden {
		t.Fatalf("skip_hidden = false, want embedded default true")
	}
	if len(cfg.Discovery.SkipDirs) != 4 {
		t.Fatalf("skip_dirs = %#v, want embedded defaults", cfg.Discovery.SkipDirs)
	}
	if len(cfg.Roots) != 2 {
		t.Fatalf("roots = %#v, want embedded example roots", cfg.Roots)
	}
	if !cfg.PathSearch.Enabled || cfg.PathSearch.Backend != "auto" || cfg.PathSearch.MaxDepth != 12 || cfg.PathSearch.Limit != 5000 {
		t.Fatalf("path_search defaults not applied: %#v", cfg.PathSearch)
	}
	if cfg.Roots[0].MaxDepth == 0 || len(cfg.Roots[0].SkipDirs) == 0 {
		t.Fatalf("first root does not include full default fields: %#v", cfg.Roots[0])
	}
	if cfg.Roots[0].Glyph != cfg.UI.Glyphs.Repo {
		t.Fatalf("first root glyph = %q, want repo glyph %q", cfg.Roots[0].Glyph, cfg.UI.Glyphs.Repo)
	}
	if cfg.Roots[1].Depth == 0 || !cfg.Roots[1].SkipHidden {
		t.Fatalf("second root does not include full default fields: %#v", cfg.Roots[1])
	}
	if cfg.Roots[1].Glyph != cfg.UI.Glyphs.Subdir {
		t.Fatalf("second root glyph = %q, want subdir glyph %q", cfg.Roots[1].Glyph, cfg.UI.Glyphs.Subdir)
	}
}

func TestLoadFileReadsRoots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[ui]
theme = "catppuccin"
popup_width = "80%"
popup_height = "70%"

[[roots]]
name = "repos"
path = "~/code"
mode = "repo"
glyph = "R"
glyph_color = "#d6a84f"
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.UI.Theme != "catppuccin" || cfg.UI.PopupWidth != "80%" || cfg.UI.PopupHeight != "70%" {
		t.Fatalf("ui config not applied: %#v", cfg.UI)
	}
	if len(cfg.Roots) != 1 || cfg.Roots[0].Name != "repos" || cfg.Roots[0].Mode != "repo" {
		t.Fatalf("roots not loaded: %#v", cfg.Roots)
	}
	if cfg.Roots[0].Glyph != "R" {
		t.Fatalf("root glyph = %q, want R", cfg.Roots[0].Glyph)
	}
	if cfg.Roots[0].GlyphColor != "#d6a84f" {
		t.Fatalf("root glyph color = %q, want #d6a84f", cfg.Roots[0].GlyphColor)
	}
}

func TestLoadFileReadsUIColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[ui.columns.chip]
show = false
width = 8
max_width = 8

[ui.columns.root]
show = true
width = 16
max_width = 18

[ui.columns.name]
show = true
width = 32
max_width = 36

[ui.columns.path]
show = false
width = 40
max_width = 80
include_root = false
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.UI.Columns.Chip.Show || cfg.UI.Columns.Chip.Width != 8 || cfg.UI.Columns.Chip.MaxWidth != 8 {
		t.Fatalf("chip column = %#v, want hidden width/max_width 8", cfg.UI.Columns.Chip)
	}
	if !cfg.UI.Columns.Root.Show || cfg.UI.Columns.Root.Width != 16 || cfg.UI.Columns.Root.MaxWidth != 18 {
		t.Fatalf("root column = %#v, want shown width 16 max 18", cfg.UI.Columns.Root)
	}
	if !cfg.UI.Columns.Name.Show || cfg.UI.Columns.Name.Width != 32 || cfg.UI.Columns.Name.MaxWidth != 36 {
		t.Fatalf("name column = %#v, want shown width 32 max 36", cfg.UI.Columns.Name)
	}
	if cfg.UI.Columns.Path.Show || cfg.UI.Columns.Path.Width != 40 || cfg.UI.Columns.Path.MaxWidth != 80 {
		t.Fatalf("path column = %#v, want hidden width 40 max 80", cfg.UI.Columns.Path)
	}
	if cfg.UI.Columns.Path.IncludeRoot {
		t.Fatalf("path column include_root = true, want false")
	}
}

func TestLoadFileKeepsExplicitZeroColumnWidths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[ui.columns.root]
show = true
width = 0

[ui.columns.name]
show = true
width = 0

[ui.columns.path]
include_root = true
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.UI.Columns.Root.Width != 0 {
		t.Fatalf("root width = %d, want explicit auto width 0", cfg.UI.Columns.Root.Width)
	}
	if cfg.UI.Columns.Name.Width != 0 {
		t.Fatalf("name width = %d, want explicit auto width 0", cfg.UI.Columns.Name.Width)
	}
	if !cfg.UI.Columns.Path.IncludeRoot {
		t.Fatalf("path include_root = false, want true")
	}
}

func TestLoadFileReadsUIGlyphs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[ui.glyphs]
repo = "R"

[ui.glyph_colors]
repo = "#123456"

[[roots]]
name = "repos"
path = "~/code"
mode = "repo"
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.UI.Glyphs.Repo != "R" {
		t.Fatalf("repo glyph = %q, want R", cfg.UI.Glyphs.Repo)
	}
	if cfg.UI.GlyphColors.Repo != "#123456" {
		t.Fatalf("repo glyph color = %q, want #123456", cfg.UI.GlyphColors.Repo)
	}
	if cfg.UI.Glyphs.Subdir == "" || cfg.UI.Glyphs.Worktree == "" || cfg.UI.Glyphs.Manual == "" {
		t.Fatalf("partial glyph config did not keep defaults: %#v", cfg.UI.Glyphs)
	}
	if cfg.UI.GlyphColors.Subdir == "" || cfg.UI.GlyphColors.Worktree == "" || cfg.UI.GlyphColors.Manual == "" {
		t.Fatalf("partial glyph color config did not keep defaults: %#v", cfg.UI.GlyphColors)
	}
	if len(cfg.Roots) != 1 || cfg.Roots[0].Glyph != "R" {
		t.Fatalf("root glyph did not inherit global repo glyph: %#v", cfg.Roots)
	}
}

func TestLoadFileReadsDiscoveryConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[discovery]
backend = "go"
skip_hidden = false
skip_gitignored = false
skip_dirs = ["node_modules", "tmp"]

[[roots]]
name = "repos"
path = "~/code"
mode = "repo"
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Discovery.SkipHidden {
		t.Fatalf("skip_hidden = true, want false")
	}
	if cfg.Discovery.SkipGitignored {
		t.Fatalf("skip_gitignored = true, want false")
	}
	if cfg.Discovery.Backend != "go" {
		t.Fatalf("backend = %q, want go", cfg.Discovery.Backend)
	}
	if len(cfg.Discovery.SkipDirs) != 2 || cfg.Discovery.SkipDirs[1] != "tmp" {
		t.Fatalf("skip_dirs = %#v", cfg.Discovery.SkipDirs)
	}
}

func TestLoadFileReadsPathSearchConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[path_search]
enabled = true
backend = "go"
roots = ["~", "/"]
	max_depth = 20
	skip_hidden = false
	skip_gitignored = false
	skip_dirs = ["tmp"]
limit = 100
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if !cfg.PathSearch.Enabled || cfg.PathSearch.Backend != "go" || cfg.PathSearch.MaxDepth != 20 {
		t.Fatalf("path_search not loaded: %#v", cfg.PathSearch)
	}
	if cfg.PathSearch.SkipHidden {
		t.Fatalf("path_search skip_hidden = true, want false")
	}
	if cfg.PathSearch.SkipGitignored {
		t.Fatalf("path_search skip_gitignored = true, want false")
	}
	if len(cfg.PathSearch.Roots) != 2 || cfg.PathSearch.Roots[1] != "/" {
		t.Fatalf("path_search roots = %#v", cfg.PathSearch.Roots)
	}
	if len(cfg.PathSearch.SkipDirs) != 1 || cfg.PathSearch.SkipDirs[0] != "tmp" {
		t.Fatalf("path_search skip_dirs = %#v", cfg.PathSearch.SkipDirs)
	}
	if cfg.PathSearch.Limit != 100 {
		t.Fatalf("path_search limit = %d, want 100", cfg.PathSearch.Limit)
	}
}

func TestLoadFileKeepsDiscoveryDefaultsWhenOnlySkipDirsConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[discovery]
skip_dirs = ["tmp"]

[[roots]]
name = "repos"
path = "~/code"
mode = "repo"
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if !cfg.Discovery.SkipHidden {
		t.Fatalf("skip_hidden = false, want default true")
	}
	if len(cfg.Discovery.SkipDirs) != 1 || cfg.Discovery.SkipDirs[0] != "tmp" {
		t.Fatalf("skip_dirs = %#v", cfg.Discovery.SkipDirs)
	}
}

func TestLoadFileReadsRootDiscoveryOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
	[discovery]
	skip_hidden = true
	skip_gitignored = true
	skip_dirs = ["node_modules"]

[[roots]]
name = "repos"
path = "~/code"
	mode = "repo"
	skip_hidden = false
	skip_gitignored = false
	skip_dirs = ["tmp"]
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if len(cfg.Roots) != 1 {
		t.Fatalf("roots = %#v", cfg.Roots)
	}
	root := cfg.Roots[0]
	if root.SkipHidden {
		t.Fatalf("root skip_hidden = true, want false")
	}
	if root.SkipGitignored {
		t.Fatalf("root skip_gitignored = true, want false")
	}
	if len(root.SkipDirs) != 1 || root.SkipDirs[0] != "tmp" {
		t.Fatalf("root skip_dirs = %#v", root.SkipDirs)
	}
}

func TestLoadFileRejectsDuplicateRootNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[[roots]]
name = "repos"
path = "~/code"

[[roots]]
name = "repos"
path = "~/other"
`)
	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile() expected duplicate root name error")
	}
}

func TestLoadFileRejectsInvalidRoot(t *testing.T) {
	tests := map[string]string{
		"missing name": `[[roots]]
path = "~/code"
`,
		"missing path": `[[roots]]
name = "repos"
`,
		"invalid mode": `[[roots]]
name = "repos"
path = "~/code"
mode = "invalid"
`,
		"legacy repos mode": `[[roots]]
name = "repos"
path = "~/code"
mode = "repos"
`,
		"legacy subdirs mode": `[[roots]]
name = "work"
path = "~/work"
mode = "subdirs"
`,
		"invalid depth": `[[roots]]
name = "work"
path = "~/work"
mode = "subdir"
depth = -1
`,
		"invalid max depth": `[[roots]]
name = "repos"
path = "~/code"
mode = "repo"
max_depth = -1
`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			writeFile(t, path, content)
			if _, err := LoadFile(path); err == nil {
				t.Fatal("LoadFile() expected validation error")
			}
		})
	}
}

func TestLoadFileRejectsInvalidPathSearch(t *testing.T) {
	tests := map[string]string{
		"invalid backend": `[path_search]
backend = "strict"
`,
		"invalid max depth": `[path_search]
max_depth = -1
`,
		"invalid limit": `[path_search]
limit = -1
`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			writeFile(t, path, content)
			if _, err := LoadFile(path); err == nil {
				t.Fatal("LoadFile() expected validation error")
			}
		})
	}
}

func TestLoadFileRejectsInvalidDiscoveryBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, `
[discovery]
backend = "strict"
`)
	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile() expected validation error")
	}
}

func TestPathUsesConfigOverride(t *testing.T) {
	t.Setenv("TMUX_PARATOR_CONFIG", "/tmp/tmux-parator/config.toml")
	chdir(t, t.TempDir())
	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if got != "/tmp/tmux-parator/config.toml" {
		t.Fatalf("Path() = %q, want override", got)
	}
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("TMUX_PARATOR_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	chdir(t, t.TempDir())
	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join("/tmp/xdg", "tmux-parator", "config.toml")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestPathUsesLocalDevConfigBeforeXDG(t *testing.T) {
	t.Setenv("TMUX_PARATOR_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, filepath.Join(dir, ".dev", "config.toml"), `
[ui]
theme = "catppuccin"
`)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join(".dev", "config.toml")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test dir: %v", err)
	}
	if err := osWriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
