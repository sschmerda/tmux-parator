package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
	Dialogs     Dialogs     `toml:"dialogs"`
	Keys        KeyBindings `toml:"keys"`
	Glyphs      Glyphs      `toml:"glyphs"`
	GlyphColors GlyphColors `toml:"glyph_colors"`
	Columns     Columns     `toml:"columns"`
}

type KeyBindings struct {
	Browse     BrowseKeys     `toml:"browse"`
	PathSearch PathSearchKeys `toml:"path_search"`
	Commands   CommandKeys    `toml:"commands"`
	Help       HelpKeys       `toml:"help"`
	Confirm    ConfirmKeys    `toml:"confirm"`
}

type BrowseKeys struct {
	Quit                []string `toml:"quit"`
	CommandPalette      []string `toml:"command_palette"`
	Help                []string `toml:"help"`
	OpenSelected        []string `toml:"open_selected"`
	OpenLastSession     []string `toml:"open_last_session"`
	KillSession         []string `toml:"kill_session"`
	RenameSession       []string `toml:"rename_session"`
	NewSession          []string `toml:"new_session"`
	PathSearch          []string `toml:"path_search"`
	Reload              []string `toml:"reload"`
	ToggleHidden        []string `toml:"toggle_hidden"`
	ToggleIgnored       []string `toml:"toggle_ignored"`
	Up                  []string `toml:"up"`
	Down                []string `toml:"down"`
	PageUp              []string `toml:"page_up"`
	PageDown            []string `toml:"page_down"`
	ScrollUp            []string `toml:"scroll_up"`
	ScrollDown          []string `toml:"scroll_down"`
	JumpNextSection     []string `toml:"jump_next_section"`
	JumpPreviousSection []string `toml:"jump_previous_section"`
	DeleteChar          []string `toml:"delete_char"`
	DeleteWord          []string `toml:"delete_word"`
	ClearInput          []string `toml:"clear_input"`
}

type PathSearchKeys struct {
	Close            []string `toml:"close"`
	CommandPalette   []string `toml:"command_palette"`
	Help             []string `toml:"help"`
	OpenSelected     []string `toml:"open_selected"`
	OpenLastSession  []string `toml:"open_last_session"`
	OpenTyped        []string `toml:"open_typed"`
	CreateTyped      []string `toml:"create_typed"`
	CycleRoot        []string `toml:"cycle_root"`
	Reload           []string `toml:"reload"`
	ToggleHidden     []string `toml:"toggle_hidden"`
	ToggleIgnored    []string `toml:"toggle_ignored"`
	Up               []string `toml:"up"`
	Down             []string `toml:"down"`
	PageUp           []string `toml:"page_up"`
	PageDown         []string `toml:"page_down"`
	ScrollUp         []string `toml:"scroll_up"`
	ScrollDown       []string `toml:"scroll_down"`
	CompleteNext     []string `toml:"complete_next"`
	CompletePrevious []string `toml:"complete_previous"`
	AcceptCompletion []string `toml:"accept_completion"`
	DeleteChar       []string `toml:"delete_char"`
	DeleteWord       []string `toml:"delete_word"`
	ClearInput       []string `toml:"clear_input"`
}

type CommandKeys struct {
	Close       []string `toml:"close"`
	Help        []string `toml:"help"`
	RunSelected []string `toml:"run_selected"`
	Up          []string `toml:"up"`
	Down        []string `toml:"down"`
	PageUp      []string `toml:"page_up"`
	PageDown    []string `toml:"page_down"`
	ScrollUp    []string `toml:"scroll_up"`
	ScrollDown  []string `toml:"scroll_down"`
	DeleteChar  []string `toml:"delete_char"`
	DeleteWord  []string `toml:"delete_word"`
	ClearInput  []string `toml:"clear_input"`
}

type HelpKeys struct {
	Close      []string `toml:"close"`
	Up         []string `toml:"up"`
	Down       []string `toml:"down"`
	PageUp     []string `toml:"page_up"`
	PageDown   []string `toml:"page_down"`
	ScrollUp   []string `toml:"scroll_up"`
	ScrollDown []string `toml:"scroll_down"`
}

type ConfirmKeys struct {
	Yes    []string `toml:"yes"`
	No     []string `toml:"no"`
	Left   []string `toml:"left"`
	Right  []string `toml:"right"`
	Submit []string `toml:"submit"`
}

type Dialogs struct {
	Small DialogSize `toml:"small"`
	Panel DialogSize `toml:"panel"`
}

type DialogSize struct {
	Width  int `toml:"width"`
	Height int `toml:"height"`
}

type Glyphs struct {
	Repo     string `toml:"repo"`
	Subdir   string `toml:"subdir"`
	Path     string `toml:"path"`
	Worktree string `toml:"worktree"`
	Manual   string `toml:"manual"`
}

type GlyphColors struct {
	Repo     string `toml:"repo"`
	Subdir   string `toml:"subdir"`
	Path     string `toml:"path"`
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
	Theme       string         `toml:"theme"`
	PopupWidth  string         `toml:"popup_width"`
	PopupHeight string         `toml:"popup_height"`
	Dialogs     rawDialogs     `toml:"dialogs"`
	Keys        rawKeyBindings `toml:"keys"`
	Glyphs      Glyphs         `toml:"glyphs"`
	GlyphColors GlyphColors    `toml:"glyph_colors"`
	Columns     rawColumns     `toml:"columns"`
}

type rawKeyBindings struct {
	Browse     rawBrowseKeys     `toml:"browse"`
	PathSearch rawPathSearchKeys `toml:"path_search"`
	Commands   rawCommandKeys    `toml:"commands"`
	Help       rawHelpKeys       `toml:"help"`
	Confirm    rawConfirmKeys    `toml:"confirm"`
}

type rawBrowseKeys struct {
	Quit                []string `toml:"quit"`
	CommandPalette      []string `toml:"command_palette"`
	Help                []string `toml:"help"`
	OpenSelected        []string `toml:"open_selected"`
	OpenLastSession     []string `toml:"open_last_session"`
	KillSession         []string `toml:"kill_session"`
	RenameSession       []string `toml:"rename_session"`
	NewSession          []string `toml:"new_session"`
	PathSearch          []string `toml:"path_search"`
	Reload              []string `toml:"reload"`
	ToggleHidden        []string `toml:"toggle_hidden"`
	ToggleIgnored       []string `toml:"toggle_ignored"`
	Up                  []string `toml:"up"`
	Down                []string `toml:"down"`
	PageUp              []string `toml:"page_up"`
	PageDown            []string `toml:"page_down"`
	ScrollUp            []string `toml:"scroll_up"`
	ScrollDown          []string `toml:"scroll_down"`
	JumpNextSection     []string `toml:"jump_next_section"`
	JumpPreviousSection []string `toml:"jump_previous_section"`
	DeleteChar          []string `toml:"delete_char"`
	DeleteWord          []string `toml:"delete_word"`
	ClearInput          []string `toml:"clear_input"`
}

type rawPathSearchKeys struct {
	Close            []string `toml:"close"`
	CommandPalette   []string `toml:"command_palette"`
	Help             []string `toml:"help"`
	OpenSelected     []string `toml:"open_selected"`
	OpenLastSession  []string `toml:"open_last_session"`
	OpenTyped        []string `toml:"open_typed"`
	CreateTyped      []string `toml:"create_typed"`
	CycleRoot        []string `toml:"cycle_root"`
	Reload           []string `toml:"reload"`
	ToggleHidden     []string `toml:"toggle_hidden"`
	ToggleIgnored    []string `toml:"toggle_ignored"`
	Up               []string `toml:"up"`
	Down             []string `toml:"down"`
	PageUp           []string `toml:"page_up"`
	PageDown         []string `toml:"page_down"`
	ScrollUp         []string `toml:"scroll_up"`
	ScrollDown       []string `toml:"scroll_down"`
	CompleteNext     []string `toml:"complete_next"`
	CompletePrevious []string `toml:"complete_previous"`
	AcceptCompletion []string `toml:"accept_completion"`
	DeleteChar       []string `toml:"delete_char"`
	DeleteWord       []string `toml:"delete_word"`
	ClearInput       []string `toml:"clear_input"`
}

type rawCommandKeys struct {
	Close       []string `toml:"close"`
	Help        []string `toml:"help"`
	RunSelected []string `toml:"run_selected"`
	Up          []string `toml:"up"`
	Down        []string `toml:"down"`
	PageUp      []string `toml:"page_up"`
	PageDown    []string `toml:"page_down"`
	ScrollUp    []string `toml:"scroll_up"`
	ScrollDown  []string `toml:"scroll_down"`
	DeleteChar  []string `toml:"delete_char"`
	DeleteWord  []string `toml:"delete_word"`
	ClearInput  []string `toml:"clear_input"`
}

type rawHelpKeys struct {
	Close      []string `toml:"close"`
	Up         []string `toml:"up"`
	Down       []string `toml:"down"`
	PageUp     []string `toml:"page_up"`
	PageDown   []string `toml:"page_down"`
	ScrollUp   []string `toml:"scroll_up"`
	ScrollDown []string `toml:"scroll_down"`
}

type rawConfirmKeys struct {
	Yes    []string `toml:"yes"`
	No     []string `toml:"no"`
	Left   []string `toml:"left"`
	Right  []string `toml:"right"`
	Submit []string `toml:"submit"`
}

type rawDialogs struct {
	Small rawDialogSize `toml:"small"`
	Panel rawDialogSize `toml:"panel"`
}

type rawDialogSize struct {
	Width  *int `toml:"width"`
	Height *int `toml:"height"`
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
	cfg.UI.Dialogs = normalizeDialogs(cfg.UI.Dialogs)
	cfg.UI.Keys = normalizeKeyBindings(cfg.UI.Keys)
	cfg.UI.Glyphs = normalizeGlyphs(cfg.UI.Glyphs)
	cfg.UI.GlyphColors = normalizeGlyphColors(cfg.UI.GlyphColors)
	cfg.UI.Columns = normalizeColumns(cfg.UI.Columns)
	if err := validateKeyBindings(cfg.UI.Keys); err != nil {
		return Config{}, err
	}
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
	ui.Dialogs = mergeDialogs(raw.Dialogs, ui.Dialogs)
	ui.Keys = mergeKeyBindings(raw.Keys, ui.Keys)
	ui.Glyphs = mergeGlyphs(raw.Glyphs, ui.Glyphs)
	ui.GlyphColors = mergeGlyphColors(raw.GlyphColors, ui.GlyphColors)
	ui.Columns = mergeColumns(raw.Columns, ui.Columns)
	return ui
}

func normalizeDialogs(dialogs Dialogs) Dialogs {
	if dialogs.Small.Width <= 0 {
		dialogs.Small.Width = 72
	}
	if dialogs.Small.Height < 0 {
		dialogs.Small.Height = 9
	}
	if dialogs.Panel.Width <= 0 {
		dialogs.Panel.Width = 88
	}
	if dialogs.Panel.Height < 0 {
		dialogs.Panel.Height = 0
	}
	return dialogs
}

func mergeDialogs(raw rawDialogs, fallback Dialogs) Dialogs {
	dialogs := fallback
	dialogs.Small = mergeDialogSize(raw.Small, dialogs.Small)
	dialogs.Panel = mergeDialogSize(raw.Panel, dialogs.Panel)
	return dialogs
}

func mergeKeyBindings(raw rawKeyBindings, fallback KeyBindings) KeyBindings {
	return KeyBindings{
		Browse:     mergeBrowseKeys(raw.Browse, fallback.Browse),
		PathSearch: mergePathSearchKeys(raw.PathSearch, fallback.PathSearch),
		Commands:   mergeCommandKeys(raw.Commands, fallback.Commands),
		Help:       mergeHelpKeys(raw.Help, fallback.Help),
		Confirm:    mergeConfirmKeys(raw.Confirm, fallback.Confirm),
	}
}

func mergeBrowseKeys(raw rawBrowseKeys, fallback BrowseKeys) BrowseKeys {
	keys := fallback
	keys.Quit = mergeKeyList(raw.Quit, keys.Quit)
	keys.CommandPalette = mergeKeyList(raw.CommandPalette, keys.CommandPalette)
	keys.Help = mergeKeyList(raw.Help, keys.Help)
	keys.OpenSelected = mergeKeyList(raw.OpenSelected, keys.OpenSelected)
	keys.OpenLastSession = mergeKeyList(raw.OpenLastSession, keys.OpenLastSession)
	keys.KillSession = mergeKeyList(raw.KillSession, keys.KillSession)
	keys.RenameSession = mergeKeyList(raw.RenameSession, keys.RenameSession)
	keys.NewSession = mergeKeyList(raw.NewSession, keys.NewSession)
	keys.PathSearch = mergeKeyList(raw.PathSearch, keys.PathSearch)
	keys.Reload = mergeKeyList(raw.Reload, keys.Reload)
	keys.ToggleHidden = mergeKeyList(raw.ToggleHidden, keys.ToggleHidden)
	keys.ToggleIgnored = mergeKeyList(raw.ToggleIgnored, keys.ToggleIgnored)
	keys.Up = mergeKeyList(raw.Up, keys.Up)
	keys.Down = mergeKeyList(raw.Down, keys.Down)
	keys.PageUp = mergeKeyList(raw.PageUp, keys.PageUp)
	keys.PageDown = mergeKeyList(raw.PageDown, keys.PageDown)
	keys.ScrollUp = mergeKeyList(raw.ScrollUp, keys.ScrollUp)
	keys.ScrollDown = mergeKeyList(raw.ScrollDown, keys.ScrollDown)
	keys.JumpNextSection = mergeKeyList(raw.JumpNextSection, keys.JumpNextSection)
	keys.JumpPreviousSection = mergeKeyList(raw.JumpPreviousSection, keys.JumpPreviousSection)
	keys.DeleteChar = mergeKeyList(raw.DeleteChar, keys.DeleteChar)
	keys.DeleteWord = mergeKeyList(raw.DeleteWord, keys.DeleteWord)
	keys.ClearInput = mergeKeyList(raw.ClearInput, keys.ClearInput)
	return keys
}

func mergePathSearchKeys(raw rawPathSearchKeys, fallback PathSearchKeys) PathSearchKeys {
	keys := fallback
	keys.Close = mergeKeyList(raw.Close, keys.Close)
	keys.CommandPalette = mergeKeyList(raw.CommandPalette, keys.CommandPalette)
	keys.Help = mergeKeyList(raw.Help, keys.Help)
	keys.OpenSelected = mergeKeyList(raw.OpenSelected, keys.OpenSelected)
	keys.OpenLastSession = mergeKeyList(raw.OpenLastSession, keys.OpenLastSession)
	keys.OpenTyped = mergeKeyList(raw.OpenTyped, keys.OpenTyped)
	keys.CreateTyped = mergeKeyList(raw.CreateTyped, keys.CreateTyped)
	keys.CycleRoot = mergeKeyList(raw.CycleRoot, keys.CycleRoot)
	keys.Reload = mergeKeyList(raw.Reload, keys.Reload)
	keys.ToggleHidden = mergeKeyList(raw.ToggleHidden, keys.ToggleHidden)
	keys.ToggleIgnored = mergeKeyList(raw.ToggleIgnored, keys.ToggleIgnored)
	keys.Up = mergeKeyList(raw.Up, keys.Up)
	keys.Down = mergeKeyList(raw.Down, keys.Down)
	keys.PageUp = mergeKeyList(raw.PageUp, keys.PageUp)
	keys.PageDown = mergeKeyList(raw.PageDown, keys.PageDown)
	keys.ScrollUp = mergeKeyList(raw.ScrollUp, keys.ScrollUp)
	keys.ScrollDown = mergeKeyList(raw.ScrollDown, keys.ScrollDown)
	keys.CompleteNext = mergeKeyList(raw.CompleteNext, keys.CompleteNext)
	keys.CompletePrevious = mergeKeyList(raw.CompletePrevious, keys.CompletePrevious)
	keys.AcceptCompletion = mergeKeyList(raw.AcceptCompletion, keys.AcceptCompletion)
	keys.DeleteChar = mergeKeyList(raw.DeleteChar, keys.DeleteChar)
	keys.DeleteWord = mergeKeyList(raw.DeleteWord, keys.DeleteWord)
	keys.ClearInput = mergeKeyList(raw.ClearInput, keys.ClearInput)
	return keys
}

func mergeCommandKeys(raw rawCommandKeys, fallback CommandKeys) CommandKeys {
	keys := fallback
	keys.Close = mergeKeyList(raw.Close, keys.Close)
	keys.Help = mergeKeyList(raw.Help, keys.Help)
	keys.RunSelected = mergeKeyList(raw.RunSelected, keys.RunSelected)
	keys.Up = mergeKeyList(raw.Up, keys.Up)
	keys.Down = mergeKeyList(raw.Down, keys.Down)
	keys.PageUp = mergeKeyList(raw.PageUp, keys.PageUp)
	keys.PageDown = mergeKeyList(raw.PageDown, keys.PageDown)
	keys.ScrollUp = mergeKeyList(raw.ScrollUp, keys.ScrollUp)
	keys.ScrollDown = mergeKeyList(raw.ScrollDown, keys.ScrollDown)
	keys.DeleteChar = mergeKeyList(raw.DeleteChar, keys.DeleteChar)
	keys.DeleteWord = mergeKeyList(raw.DeleteWord, keys.DeleteWord)
	keys.ClearInput = mergeKeyList(raw.ClearInput, keys.ClearInput)
	return keys
}

func mergeHelpKeys(raw rawHelpKeys, fallback HelpKeys) HelpKeys {
	keys := fallback
	keys.Close = mergeKeyList(raw.Close, keys.Close)
	keys.Up = mergeKeyList(raw.Up, keys.Up)
	keys.Down = mergeKeyList(raw.Down, keys.Down)
	keys.PageUp = mergeKeyList(raw.PageUp, keys.PageUp)
	keys.PageDown = mergeKeyList(raw.PageDown, keys.PageDown)
	keys.ScrollUp = mergeKeyList(raw.ScrollUp, keys.ScrollUp)
	keys.ScrollDown = mergeKeyList(raw.ScrollDown, keys.ScrollDown)
	return keys
}

func mergeConfirmKeys(raw rawConfirmKeys, fallback ConfirmKeys) ConfirmKeys {
	keys := fallback
	keys.Yes = mergeKeyList(raw.Yes, keys.Yes)
	keys.No = mergeKeyList(raw.No, keys.No)
	keys.Left = mergeKeyList(raw.Left, keys.Left)
	keys.Right = mergeKeyList(raw.Right, keys.Right)
	keys.Submit = mergeKeyList(raw.Submit, keys.Submit)
	return keys
}

func mergeKeyList(raw []string, fallback []string) []string {
	if raw != nil {
		copied := make([]string, len(raw))
		copy(copied, raw)
		return copied
	}
	return append([]string(nil), fallback...)
}

func normalizeKeyBindings(keys KeyBindings) KeyBindings {
	defaults := defaultKeyBindings()
	return KeyBindings{
		Browse:     mergeBrowseKeys(rawBrowseKeys{}, mergeBrowseKeys(rawBrowseKeysFromKeys(keys.Browse), defaults.Browse)),
		PathSearch: mergePathSearchKeys(rawPathSearchKeys{}, mergePathSearchKeys(rawPathSearchKeysFromKeys(keys.PathSearch), defaults.PathSearch)),
		Commands:   mergeCommandKeys(rawCommandKeys{}, mergeCommandKeys(rawCommandKeysFromKeys(keys.Commands), defaults.Commands)),
		Help:       mergeHelpKeys(rawHelpKeys{}, mergeHelpKeys(rawHelpKeysFromKeys(keys.Help), defaults.Help)),
		Confirm:    mergeConfirmKeys(rawConfirmKeys{}, mergeConfirmKeys(rawConfirmKeysFromKeys(keys.Confirm), defaults.Confirm)),
	}
}

func defaultKeyBindings() KeyBindings {
	return KeyBindings{
		Browse: BrowseKeys{
			Quit:                []string{"ctrl+c", "esc"},
			CommandPalette:      []string{"ctrl+g"},
			Help:                []string{"ctrl+_"},
			OpenSelected:        []string{"enter"},
			OpenLastSession:     []string{"ctrl+@"},
			KillSession:         []string{"ctrl+k"},
			RenameSession:       []string{"ctrl+n"},
			NewSession:          []string{"ctrl+s"},
			PathSearch:          []string{"ctrl+t"},
			Reload:              []string{"ctrl+r"},
			ToggleHidden:        []string{"alt+h", "meta+h"},
			ToggleIgnored:       []string{"alt+i", "meta+i"},
			Up:                  []string{"up"},
			Down:                []string{"down"},
			PageUp:              []string{"pgup", "ctrl+b"},
			PageDown:            []string{"pgdown", "ctrl+d"},
			ScrollUp:            []string{"ctrl+y"},
			ScrollDown:          []string{"ctrl+e"},
			JumpNextSection:     []string{"tab"},
			JumpPreviousSection: []string{"shift+tab"},
			DeleteChar:          []string{"backspace", "ctrl+h"},
			DeleteWord:          []string{"alt+backspace"},
			ClearInput:          []string{"ctrl+u", "meta+backspace"},
		},
		PathSearch: PathSearchKeys{
			Close:            []string{"ctrl+t", "esc"},
			CommandPalette:   []string{"ctrl+g"},
			Help:             []string{"ctrl+_"},
			OpenSelected:     []string{"enter"},
			OpenLastSession:  []string{"ctrl+@"},
			OpenTyped:        []string{"ctrl+p"},
			CreateTyped:      []string{"ctrl+a"},
			CycleRoot:        []string{"ctrl+o"},
			Reload:           []string{"ctrl+r"},
			ToggleHidden:     []string{"alt+h", "meta+h"},
			ToggleIgnored:    []string{"alt+i", "meta+i"},
			Up:               []string{"up"},
			Down:             []string{"down"},
			PageUp:           []string{"pgup", "ctrl+b"},
			PageDown:         []string{"pgdown", "ctrl+d"},
			ScrollUp:         []string{"ctrl+y"},
			ScrollDown:       []string{"ctrl+e"},
			CompleteNext:     []string{"tab"},
			CompletePrevious: []string{"shift+tab"},
			AcceptCompletion: []string{"left", "right"},
			DeleteChar:       []string{"backspace", "ctrl+h"},
			DeleteWord:       []string{"alt+backspace"},
			ClearInput:       []string{"ctrl+u", "meta+backspace"},
		},
		Commands: CommandKeys{
			Close:       []string{"esc", "ctrl+g"},
			Help:        []string{"ctrl+_"},
			RunSelected: []string{"enter"},
			Up:          []string{"up"},
			Down:        []string{"down"},
			PageUp:      []string{"pgup", "ctrl+b"},
			PageDown:    []string{"pgdown", "ctrl+d"},
			ScrollUp:    []string{"ctrl+y"},
			ScrollDown:  []string{"ctrl+e"},
			DeleteChar:  []string{"backspace", "ctrl+h"},
			DeleteWord:  []string{"alt+backspace"},
			ClearInput:  []string{"ctrl+u", "meta+backspace"},
		},
		Help: HelpKeys{
			Close:      []string{"esc", "ctrl+_"},
			Up:         []string{"up", "k"},
			Down:       []string{"down", "j"},
			PageUp:     []string{"pgup", "ctrl+b"},
			PageDown:   []string{"pgdown", "ctrl+d"},
			ScrollUp:   []string{"ctrl+y"},
			ScrollDown: []string{"ctrl+e"},
		},
		Confirm: ConfirmKeys{
			Yes:    []string{"y", "Y"},
			No:     []string{"n", "N", "esc"},
			Left:   []string{"left", "up", "shift+tab"},
			Right:  []string{"right", "down", "tab"},
			Submit: []string{"enter"},
		},
	}
}

func rawBrowseKeysFromKeys(keys BrowseKeys) rawBrowseKeys {
	return rawBrowseKeys(keys)
}

func rawPathSearchKeysFromKeys(keys PathSearchKeys) rawPathSearchKeys {
	return rawPathSearchKeys(keys)
}

func rawCommandKeysFromKeys(keys CommandKeys) rawCommandKeys {
	return rawCommandKeys(keys)
}

func rawHelpKeysFromKeys(keys HelpKeys) rawHelpKeys {
	return rawHelpKeys(keys)
}

func rawConfirmKeysFromKeys(keys ConfirmKeys) rawConfirmKeys {
	return rawConfirmKeys(keys)
}

func mergeDialogSize(raw rawDialogSize, fallback DialogSize) DialogSize {
	size := fallback
	if raw.Width != nil {
		size.Width = *raw.Width
	}
	if raw.Height != nil {
		size.Height = *raw.Height
	}
	return size
}

func mergeGlyphs(raw Glyphs, fallback Glyphs) Glyphs {
	glyphs := fallback
	if raw.Repo != "" {
		glyphs.Repo = raw.Repo
	}
	if raw.Subdir != "" {
		glyphs.Subdir = raw.Subdir
	}
	if raw.Path != "" {
		glyphs.Path = raw.Path
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
	if raw.Path != "" {
		colors.Path = raw.Path
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
	case "path":
		return glyphs.Path
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
	if glyphs.Path == "" {
		glyphs.Path = "\U000f024b"
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
	if colors.Path == "" {
		colors.Path = "#7dcfff"
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

type keyAction struct {
	name string
	keys []string
}

func validateKeyBindings(keys KeyBindings) error {
	contexts := []struct {
		name    string
		actions []keyAction
	}{
		{
			name: "browse",
			actions: []keyAction{
				{name: "quit", keys: keys.Browse.Quit},
				{name: "command_palette", keys: keys.Browse.CommandPalette},
				{name: "help", keys: keys.Browse.Help},
				{name: "open_selected", keys: keys.Browse.OpenSelected},
				{name: "open_last_session", keys: keys.Browse.OpenLastSession},
				{name: "kill_session", keys: keys.Browse.KillSession},
				{name: "rename_session", keys: keys.Browse.RenameSession},
				{name: "new_session", keys: keys.Browse.NewSession},
				{name: "path_search", keys: keys.Browse.PathSearch},
				{name: "reload", keys: keys.Browse.Reload},
				{name: "toggle_hidden", keys: keys.Browse.ToggleHidden},
				{name: "toggle_ignored", keys: keys.Browse.ToggleIgnored},
				{name: "up", keys: keys.Browse.Up},
				{name: "down", keys: keys.Browse.Down},
				{name: "page_up", keys: keys.Browse.PageUp},
				{name: "page_down", keys: keys.Browse.PageDown},
				{name: "scroll_up", keys: keys.Browse.ScrollUp},
				{name: "scroll_down", keys: keys.Browse.ScrollDown},
				{name: "jump_next_section", keys: keys.Browse.JumpNextSection},
				{name: "jump_previous_section", keys: keys.Browse.JumpPreviousSection},
				{name: "delete_char", keys: keys.Browse.DeleteChar},
				{name: "delete_word", keys: keys.Browse.DeleteWord},
				{name: "clear_input", keys: keys.Browse.ClearInput},
			},
		},
		{
			name: "path_search",
			actions: []keyAction{
				{name: "close", keys: keys.PathSearch.Close},
				{name: "command_palette", keys: keys.PathSearch.CommandPalette},
				{name: "help", keys: keys.PathSearch.Help},
				{name: "open_selected", keys: keys.PathSearch.OpenSelected},
				{name: "open_last_session", keys: keys.PathSearch.OpenLastSession},
				{name: "open_typed", keys: keys.PathSearch.OpenTyped},
				{name: "create_typed", keys: keys.PathSearch.CreateTyped},
				{name: "cycle_root", keys: keys.PathSearch.CycleRoot},
				{name: "reload", keys: keys.PathSearch.Reload},
				{name: "toggle_hidden", keys: keys.PathSearch.ToggleHidden},
				{name: "toggle_ignored", keys: keys.PathSearch.ToggleIgnored},
				{name: "up", keys: keys.PathSearch.Up},
				{name: "down", keys: keys.PathSearch.Down},
				{name: "page_up", keys: keys.PathSearch.PageUp},
				{name: "page_down", keys: keys.PathSearch.PageDown},
				{name: "scroll_up", keys: keys.PathSearch.ScrollUp},
				{name: "scroll_down", keys: keys.PathSearch.ScrollDown},
				{name: "complete_next", keys: keys.PathSearch.CompleteNext},
				{name: "complete_previous", keys: keys.PathSearch.CompletePrevious},
				{name: "accept_completion", keys: keys.PathSearch.AcceptCompletion},
				{name: "delete_char", keys: keys.PathSearch.DeleteChar},
				{name: "delete_word", keys: keys.PathSearch.DeleteWord},
				{name: "clear_input", keys: keys.PathSearch.ClearInput},
			},
		},
		{
			name: "commands",
			actions: []keyAction{
				{name: "close", keys: keys.Commands.Close},
				{name: "help", keys: keys.Commands.Help},
				{name: "run_selected", keys: keys.Commands.RunSelected},
				{name: "up", keys: keys.Commands.Up},
				{name: "down", keys: keys.Commands.Down},
				{name: "page_up", keys: keys.Commands.PageUp},
				{name: "page_down", keys: keys.Commands.PageDown},
				{name: "scroll_up", keys: keys.Commands.ScrollUp},
				{name: "scroll_down", keys: keys.Commands.ScrollDown},
				{name: "delete_char", keys: keys.Commands.DeleteChar},
				{name: "delete_word", keys: keys.Commands.DeleteWord},
				{name: "clear_input", keys: keys.Commands.ClearInput},
			},
		},
		{
			name: "help",
			actions: []keyAction{
				{name: "close", keys: keys.Help.Close},
				{name: "up", keys: keys.Help.Up},
				{name: "down", keys: keys.Help.Down},
				{name: "page_up", keys: keys.Help.PageUp},
				{name: "page_down", keys: keys.Help.PageDown},
				{name: "scroll_up", keys: keys.Help.ScrollUp},
				{name: "scroll_down", keys: keys.Help.ScrollDown},
			},
		},
		{
			name: "confirm",
			actions: []keyAction{
				{name: "yes", keys: keys.Confirm.Yes},
				{name: "no", keys: keys.Confirm.No},
				{name: "left", keys: keys.Confirm.Left},
				{name: "right", keys: keys.Confirm.Right},
				{name: "submit", keys: keys.Confirm.Submit},
			},
		},
	}
	for _, context := range contexts {
		seen := map[string]string{}
		for _, action := range context.actions {
			if len(action.keys) == 0 {
				return fmt.Errorf("ui.keys.%s.%s must include at least one key", context.name, action.name)
			}
			for _, key := range action.keys {
				normalized := strings.TrimSpace(key)
				if normalized == "" {
					return fmt.Errorf("ui.keys.%s.%s contains an empty key", context.name, action.name)
				}
				if !validKeyName(normalized) {
					return fmt.Errorf("ui.keys.%s.%s contains invalid key %q", context.name, action.name, key)
				}
				if previous, ok := seen[normalized]; ok {
					return fmt.Errorf("ui.keys.%s maps key %q to both %s and %s", context.name, normalized, previous, action.name)
				}
				seen[normalized] = action.name
			}
		}
	}
	return nil
}

func validKeyName(key string) bool {
	if singleNonSpaceRune(key) {
		return true
	}
	switch key {
	case "enter", "esc", "tab", "shift+tab", "backspace", "delete", "up", "down", "left", "right", "home", "end", "pgup", "pgdown", "space":
		return true
	}
	for _, prefix := range []string{"ctrl+", "alt+", "meta+"} {
		if strings.HasPrefix(key, prefix) {
			return validModifiedKey(strings.TrimPrefix(key, prefix))
		}
	}
	return false
}

func validModifiedKey(key string) bool {
	if singleNonSpaceRune(key) {
		return true
	}
	switch key {
	case "enter", "esc", "tab", "shift+tab", "backspace", "delete", "up", "down", "left", "right", "home", "end", "pgup", "pgdown", "space":
		return true
	}
	return false
}

func singleNonSpaceRune(value string) bool {
	if len([]rune(value)) != 1 {
		return false
	}
	for _, r := range value {
		return !unicode.IsSpace(r)
	}
	return false
}

func validateDeprecatedKeys(meta toml.MetaData) error {
	for _, key := range meta.Undecoded() {
		if len(key) >= 2 && key[0] == "ui" && key[1] == "keys" {
			return fmt.Errorf("unknown key %s", strings.Join(key, "."))
		}
		if len(key) == 2 && key[0] == "roots" && key[1] == "mode" {
			return fmt.Errorf("roots.kind replaced roots.mode")
		}
	}
	return nil
}
