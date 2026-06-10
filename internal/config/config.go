package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed default.toml
var defaultConfigTOML string

type Config struct {
	UI         UI         `toml:"ui"`
	Discovery  Discovery  `toml:"discovery"`
	PathSearch PathSearch `toml:"path_search"`
	Roots      []Root     `toml:"roots"`
}

type UI struct {
	Theme       string      `toml:"theme"`
	PopupWidth  string      `toml:"popup_width"`
	PopupHeight string      `toml:"popup_height"`
	Glyphs      Glyphs      `toml:"glyphs"`
	GlyphColors GlyphColors `toml:"glyph_colors"`
	Columns     Columns     `toml:"columns"`
}

type Glyphs struct {
	Repo     string `toml:"repo"`
	Subdir   string `toml:"subdir"`
	Worktree string `toml:"worktree"`
	Manual   string `toml:"manual"`
}

type GlyphColors struct {
	Repo     string `toml:"repo"`
	Subdir   string `toml:"subdir"`
	Worktree string `toml:"worktree"`
	Manual   string `toml:"manual"`
}

type Columns struct {
	Chip Column `toml:"chip"`
	Root Column `toml:"root"`
	Name Column `toml:"name"`
	Path Column `toml:"path"`
}

type Column struct {
	Show        bool `toml:"show"`
	Width       int  `toml:"width"`
	MaxWidth    int  `toml:"max_width"`
	IncludeRoot bool `toml:"include_root"`
}

type Root struct {
	Name           string   `toml:"name"`
	Path           string   `toml:"path"`
	Kind           string   `toml:"kind"`
	Glyph          string   `toml:"glyph"`
	GlyphColor     string   `toml:"glyph_color"`
	Depth          int      `toml:"depth"`
	MaxDepth       int      `toml:"max_depth"`
	SkipHidden     bool     `toml:"skip_hidden"`
	SkipGitignored bool     `toml:"skip_gitignored"`
	SkipDirs       []string `toml:"skip_dirs"`
}

type Discovery struct {
	Backend        string   `toml:"backend"`
	SkipHidden     bool     `toml:"skip_hidden"`
	SkipGitignored bool     `toml:"skip_gitignored"`
	SkipDirs       []string `toml:"skip_dirs"`
}

type PathSearch struct {
	Enabled        bool     `toml:"enabled"`
	Backend        string   `toml:"backend"`
	Roots          []string `toml:"roots"`
	MaxDepth       int      `toml:"max_depth"`
	SkipHidden     bool     `toml:"skip_hidden"`
	SkipGitignored bool     `toml:"skip_gitignored"`
	SkipDirs       []string `toml:"skip_dirs"`
	Limit          int      `toml:"limit"`
}

type rawRoot struct {
	Name           string   `toml:"name"`
	Path           string   `toml:"path"`
	Kind           string   `toml:"kind"`
	Glyph          string   `toml:"glyph"`
	GlyphColor     string   `toml:"glyph_color"`
	Depth          int      `toml:"depth"`
	MaxDepth       int      `toml:"max_depth"`
	SkipHidden     *bool    `toml:"skip_hidden"`
	SkipGitignored *bool    `toml:"skip_gitignored"`
	SkipDirs       []string `toml:"skip_dirs"`
}

type rawUI struct {
	Theme       string      `toml:"theme"`
	PopupWidth  string      `toml:"popup_width"`
	PopupHeight string      `toml:"popup_height"`
	Glyphs      Glyphs      `toml:"glyphs"`
	GlyphColors GlyphColors `toml:"glyph_colors"`
	Columns     rawColumns  `toml:"columns"`
}

type rawColumns struct {
	Chip rawColumn `toml:"chip"`
	Root rawColumn `toml:"root"`
	Name rawColumn `toml:"name"`
	Path rawColumn `toml:"path"`
}

type rawColumn struct {
	Show        *bool `toml:"show"`
	Width       *int  `toml:"width"`
	MaxWidth    *int  `toml:"max_width"`
	IncludeRoot *bool `toml:"include_root"`
}

type rawDiscovery struct {
	Backend        string   `toml:"backend"`
	SkipHidden     *bool    `toml:"skip_hidden"`
	SkipGitignored *bool    `toml:"skip_gitignored"`
	SkipDirs       []string `toml:"skip_dirs"`
}

type rawPathSearch struct {
	Enabled        *bool    `toml:"enabled"`
	Backend        string   `toml:"backend"`
	Roots          []string `toml:"roots"`
	MaxDepth       int      `toml:"max_depth"`
	SkipHidden     *bool    `toml:"skip_hidden"`
	SkipGitignored *bool    `toml:"skip_gitignored"`
	SkipDirs       []string `toml:"skip_dirs"`
	Limit          int      `toml:"limit"`
}

func Default() Config {
	cfg, err := parseConfig(defaultConfigTOML, Config{})
	if err != nil {
		panic(fmt.Sprintf("parse embedded default config: %v", err))
	}
	return cfg
}

func Path() (string, error) {
	if override := os.Getenv("TMUX_PARATOR_CONFIG"); override != "" {
		return override, nil
	}
	if _, err := os.Stat(filepath.Join(".dev", "config.toml")); err == nil {
		return filepath.Join(".dev", "config.toml"), nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tmux-parator", "config.toml"), nil
}

func Load() (Config, string, error) {
	path, err := Path()
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := LoadFile(path)
	return cfg, path, err
}

func LoadFile(path string) (Config, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, err
	}

	var raw rawConfig
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return Config{}, err
	}
	if err := validateDeprecatedKeys(meta); err != nil {
		return Config{}, err
	}
	cfg := Default()
	applyRawConfig(&cfg, raw, meta)
	return finalize(cfg)
}

func parseConfig(content string, fallback Config) (Config, error) {
	var raw rawConfig
	meta, err := toml.Decode(content, &raw)
	if err != nil {
		return Config{}, err
	}
	if err := validateDeprecatedKeys(meta); err != nil {
		return Config{}, err
	}
	cfg := fallback
	applyRawConfig(&cfg, raw, meta)
	return finalize(cfg)
}

func applyRawConfig(cfg *Config, raw rawConfig, meta toml.MetaData) {
	if meta.IsDefined("ui") {
		cfg.UI = normalizeUI(raw.UI, cfg.UI)
	}
	if meta.IsDefined("discovery") {
		cfg.Discovery = normalizeDiscovery(raw.Discovery, cfg.Discovery)
	}
	if meta.IsDefined("path_search") {
		cfg.PathSearch = normalizePathSearch(raw.PathSearch, cfg.PathSearch, cfg.Discovery)
	}
	cfg.Roots = normalizeRoots(raw.Roots, cfg.Discovery)
}

type rawConfig struct {
	UI         rawUI         `toml:"ui"`
	Discovery  rawDiscovery  `toml:"discovery"`
	PathSearch rawPathSearch `toml:"path_search"`
	Roots      []rawRoot     `toml:"roots"`
}

func finalize(cfg Config) (Config, error) {
	if cfg.UI.Theme == "" {
		cfg.UI.Theme = "shades-of-purple"
	}
	if cfg.UI.PopupWidth == "" {
		cfg.UI.PopupWidth = "90%"
	}
	if cfg.UI.PopupHeight == "" {
		cfg.UI.PopupHeight = "90%"
	}
	cfg.UI.Glyphs = normalizeGlyphs(cfg.UI.Glyphs)
	cfg.UI.GlyphColors = normalizeGlyphColors(cfg.UI.GlyphColors)
	cfg.UI.Columns = normalizeColumns(cfg.UI.Columns)
	cfg.Roots = normalizeRootGlyphs(cfg.Roots, cfg.UI.Glyphs)
	if cfg.Discovery.Backend == "" {
		cfg.Discovery.Backend = "auto"
	}
	if cfg.Discovery.SkipDirs == nil {
		cfg.Discovery.SkipDirs = []string{"node_modules", "vendor", "dist", "build"}
	}
	if cfg.PathSearch.Backend == "" {
		cfg.PathSearch.Backend = "auto"
	}
	if cfg.PathSearch.Roots == nil {
		cfg.PathSearch.Roots = []string{"~"}
	}
	if cfg.PathSearch.MaxDepth == 0 {
		cfg.PathSearch.MaxDepth = 12
	}
	if cfg.PathSearch.SkipDirs == nil {
		cfg.PathSearch.SkipDirs = cfg.Discovery.SkipDirs
	}
	if cfg.PathSearch.Limit == 0 {
		cfg.PathSearch.Limit = 5000
	}
	if err := validatePathSearch(cfg.PathSearch); err != nil {
		return Config{}, err
	}
	if err := validateDiscovery(cfg.Discovery); err != nil {
		return Config{}, err
	}
	if err := validateRoots(cfg.Roots); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalizeUI(raw rawUI, fallback UI) UI {
	ui := fallback
	if raw.Theme != "" {
		ui.Theme = raw.Theme
	}
	if raw.PopupWidth != "" {
		ui.PopupWidth = raw.PopupWidth
	}
	if raw.PopupHeight != "" {
		ui.PopupHeight = raw.PopupHeight
	}
	ui.Glyphs = mergeGlyphs(raw.Glyphs, ui.Glyphs)
	ui.GlyphColors = mergeGlyphColors(raw.GlyphColors, ui.GlyphColors)
	ui.Columns = mergeColumns(raw.Columns, ui.Columns)
	return ui
}

func mergeGlyphs(raw Glyphs, fallback Glyphs) Glyphs {
	glyphs := fallback
	if raw.Repo != "" {
		glyphs.Repo = raw.Repo
	}
	if raw.Subdir != "" {
		glyphs.Subdir = raw.Subdir
	}
	if raw.Worktree != "" {
		glyphs.Worktree = raw.Worktree
	}
	if raw.Manual != "" {
		glyphs.Manual = raw.Manual
	}
	return glyphs
}

func mergeGlyphColors(raw GlyphColors, fallback GlyphColors) GlyphColors {
	colors := fallback
	if raw.Repo != "" {
		colors.Repo = raw.Repo
	}
	if raw.Subdir != "" {
		colors.Subdir = raw.Subdir
	}
	if raw.Worktree != "" {
		colors.Worktree = raw.Worktree
	}
	if raw.Manual != "" {
		colors.Manual = raw.Manual
	}
	return colors
}

func mergeColumns(raw rawColumns, fallback Columns) Columns {
	return Columns{
		Chip: mergeColumn(raw.Chip, fallback.Chip),
		Root: mergeColumn(raw.Root, fallback.Root),
		Name: mergeColumn(raw.Name, fallback.Name),
		Path: mergeColumn(raw.Path, fallback.Path),
	}
}

func mergeColumn(raw rawColumn, fallback Column) Column {
	column := fallback
	if raw.Show != nil {
		column.Show = *raw.Show
	}
	if raw.Width != nil {
		column.Width = *raw.Width
	}
	if raw.MaxWidth != nil {
		column.MaxWidth = *raw.MaxWidth
	}
	if raw.IncludeRoot != nil {
		column.IncludeRoot = *raw.IncludeRoot
	}
	return column
}

func normalizeColumns(columns Columns) Columns {
	columns.Chip = normalizeColumn(columns.Chip, 12, 12)
	columns.Root = normalizeColumn(columns.Root, 12, 20)
	columns.Name = normalizeColumn(columns.Name, 28, 40)
	columns.Path = normalizeColumn(columns.Path, 0, 0)
	return columns
}

func normalizeColumn(column Column, defaultWidth int, defaultMaxWidth int) Column {
	if column.Width < 0 {
		column.Width = defaultWidth
	}
	if column.MaxWidth < 0 {
		column.MaxWidth = defaultMaxWidth
	}
	if column.MaxWidth == 0 && defaultMaxWidth > 0 {
		column.MaxWidth = defaultMaxWidth
	}
	return column
}

func normalizeRootGlyphs(roots []Root, glyphs Glyphs) []Root {
	for i := range roots {
		if strings.TrimSpace(roots[i].Glyph) != "" {
			continue
		}
		roots[i].Glyph = glyphForMode(rootKind(roots[i]), glyphs)
	}
	return roots
}

func glyphForMode(mode string, glyphs Glyphs) string {
	switch strings.TrimSpace(mode) {
	case "repo":
		return glyphs.Repo
	case "worktree":
		return glyphs.Worktree
	default:
		return glyphs.Subdir
	}
}

func normalizeGlyphs(glyphs Glyphs) Glyphs {
	if glyphs.Repo == "" {
		glyphs.Repo = "\ue702"
	}
	if glyphs.Subdir == "" {
		glyphs.Subdir = "\uf0c9"
	}
	if glyphs.Worktree == "" {
		glyphs.Worktree = "\U000f0655"
	}
	if glyphs.Manual == "" {
		glyphs.Manual = "\uebc8"
	}
	return glyphs
}

func normalizeGlyphColors(colors GlyphColors) GlyphColors {
	if colors.Repo == "" {
		colors.Repo = "#f14e32"
	}
	if colors.Subdir == "" {
		colors.Subdir = "#7aa2f7"
	}
	if colors.Worktree == "" {
		colors.Worktree = "#9ece6a"
	}
	if colors.Manual == "" {
		colors.Manual = "#a599e9"
	}
	return colors
}

func normalizeDiscovery(raw rawDiscovery, fallback Discovery) Discovery {
	discovery := fallback
	if raw.Backend != "" {
		discovery.Backend = raw.Backend
	}
	if raw.SkipHidden != nil {
		discovery.SkipHidden = *raw.SkipHidden
	}
	if raw.SkipGitignored != nil {
		discovery.SkipGitignored = *raw.SkipGitignored
	}
	if raw.SkipDirs != nil {
		discovery.SkipDirs = raw.SkipDirs
	}
	return discovery
}

func validateDiscovery(discovery Discovery) error {
	switch strings.TrimSpace(discovery.Backend) {
	case "", "auto", "fd", "go":
	default:
		return fmt.Errorf("discovery.backend must be auto, fd, or go")
	}
	return nil
}

func normalizePathSearch(raw rawPathSearch, fallback PathSearch, discovery Discovery) PathSearch {
	pathSearch := fallback
	if raw.Enabled != nil {
		pathSearch.Enabled = *raw.Enabled
	}
	if raw.Backend != "" {
		pathSearch.Backend = raw.Backend
	}
	if raw.Roots != nil {
		pathSearch.Roots = raw.Roots
	}
	if raw.MaxDepth != 0 {
		pathSearch.MaxDepth = raw.MaxDepth
	}
	if raw.SkipHidden != nil {
		pathSearch.SkipHidden = *raw.SkipHidden
	}
	if raw.SkipGitignored != nil {
		pathSearch.SkipGitignored = *raw.SkipGitignored
	}
	if raw.SkipDirs != nil {
		pathSearch.SkipDirs = raw.SkipDirs
	}
	if raw.Limit != 0 {
		pathSearch.Limit = raw.Limit
	}
	if pathSearch.SkipDirs == nil {
		pathSearch.SkipDirs = discovery.SkipDirs
	}
	return pathSearch
}

func normalizeRoots(rawRoots []rawRoot, discovery Discovery) []Root {
	roots := make([]Root, 0, len(rawRoots))
	for _, raw := range rawRoots {
		kind := normalizeRootMode(raw.Kind)
		skipHidden := discovery.SkipHidden
		if raw.SkipHidden != nil {
			skipHidden = *raw.SkipHidden
		}
		skipGitignored := discovery.SkipGitignored
		if raw.SkipGitignored != nil {
			skipGitignored = *raw.SkipGitignored
		}
		skipDirs := discovery.SkipDirs
		if raw.SkipDirs != nil {
			skipDirs = raw.SkipDirs
		}
		roots = append(roots, Root{
			Name:           raw.Name,
			Path:           raw.Path,
			Kind:           kind,
			Glyph:          raw.Glyph,
			GlyphColor:     raw.GlyphColor,
			Depth:          raw.Depth,
			MaxDepth:       raw.MaxDepth,
			SkipHidden:     skipHidden,
			SkipGitignored: skipGitignored,
			SkipDirs:       skipDirs,
		})
	}
	return roots
}

func validateRoots(roots []Root) error {
	seen := map[string]bool{}
	for i, root := range roots {
		name := strings.TrimSpace(root.Name)
		if name == "" {
			return fmt.Errorf("roots[%d].name is required", i)
		}
		if seen[name] {
			return fmt.Errorf("duplicate root name %q", name)
		}
		seen[name] = true
		if strings.TrimSpace(root.Path) == "" {
			return fmt.Errorf("roots[%d].path is required", i)
		}
		mode := rootKind(root)
		if mode == "" {
			mode = "subdir"
		}
		if mode != "subdir" && mode != "repo" {
			return fmt.Errorf("roots[%d].kind must be subdir or repo", i)
		}
		if mode == "subdir" && root.Depth < 0 {
			return fmt.Errorf("roots[%d].depth must be 0 or greater", i)
		}
		if mode == "repo" && root.MaxDepth < 0 {
			return fmt.Errorf("roots[%d].max_depth must be 0 or greater", i)
		}
	}
	return nil
}

func rootKind(root Root) string {
	return normalizeRootMode(root.Kind)
}

func normalizeRootMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "":
		return ""
	case "subdir":
		return "subdir"
	case "repo":
		return "repo"
	default:
		return strings.TrimSpace(strings.ToLower(mode))
	}
}

func validatePathSearch(pathSearch PathSearch) error {
	switch strings.TrimSpace(pathSearch.Backend) {
	case "", "auto", "fd", "go":
	default:
		return fmt.Errorf("path_search.backend must be auto, fd, or go")
	}
	if pathSearch.MaxDepth < 0 {
		return fmt.Errorf("path_search.max_depth must be 0 or greater")
	}
	if pathSearch.Limit < 0 {
		return fmt.Errorf("path_search.limit must be 0 or greater")
	}
	return nil
}

func validateDeprecatedKeys(meta toml.MetaData) error {
	for _, key := range meta.Undecoded() {
		if len(key) == 2 && key[0] == "roots" && key[1] == "mode" {
			return fmt.Errorf("roots.kind replaced roots.mode")
		}
	}
	return nil
}
