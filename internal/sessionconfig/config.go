package sessionconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Templates []Template
}

type Template struct {
	ID                string
	Name              string
	SessionName       string
	Focus             string
	Description       string
	Chip              string
	WindowIndicators  []string
	Variables         map[string]string
	Parameters        []Parameter
	Enabled           bool
	Source            string
	Match             []string
	BeforeCreateHooks []Hook
	AfterCreateHooks  []Hook
	Windows           []Window
}

type Parameter struct {
	Name    string
	Prompt  string
	Options []string
	Default string
}

type Hook struct {
	Run   string
	Kinds []string
}

type Window struct {
	Name   string
	Focus  string
	When   string
	Layout Node
}

type Node struct {
	Name        string
	Type        string
	When        string
	Path        string
	Command     string
	Commands    []string
	CommandMode string
	Sizes       []int
	Children    []Node
}

const (
	CommandModeInteractive = "interactive"
	CommandModeWrapper     = "wrapper"
	SourceGlobal           = "global"
	SourceLocal            = "local"
)

type rawTemplate struct {
	ID               string            `toml:"id"`
	Name             string            `toml:"name"`
	SessionName      string            `toml:"session_name"`
	Focus            string            `toml:"focus"`
	Description      string            `toml:"description"`
	Chip             string            `toml:"chip"`
	WindowIndicators interface{}       `toml:"window_indicators"`
	Glyphs           interface{}       `toml:"glyphs"`
	Variables        map[string]string `toml:"variables"`
	Parameters       []rawParameter    `toml:"parameters"`
	Enabled          *bool             `toml:"enabled"`
	Match            interface{}       `toml:"match"`
	Hooks            rawHooks          `toml:"hooks"`
	Windows          []rawWindow       `toml:"windows"`
}

type rawParameter struct {
	Name    string   `toml:"name"`
	Prompt  string   `toml:"prompt"`
	Options []string `toml:"options"`
	Default string   `toml:"default"`
}

type rawHooks struct {
	BeforeCreateCommand []rawHook `toml:"before_create_command"`
	BeforeCreateScript  []rawHook `toml:"before_create_script"`
	AfterCreateCommand  []rawHook `toml:"after_create_command"`
	AfterCreateScript   []rawHook `toml:"after_create_script"`
}

type rawHook struct {
	Run   string   `toml:"run"`
	Kinds []string `toml:"kinds"`
}

type rawWindow struct {
	Name   string                 `toml:"name"`
	Focus  string                 `toml:"focus"`
	When   string                 `toml:"when"`
	Layout map[string]interface{} `toml:"layout"`
}

func Path(mainConfigPath string) (string, error) {
	if override := os.Getenv("TMUX_PARATOR_SESSION_CONFIG"); override != "" {
		return override, nil
	}
	if _, err := os.Stat(filepath.Join(".dev", "templates")); err == nil {
		return ".dev", nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if strings.TrimSpace(mainConfigPath) != "" {
		return filepath.Dir(mainConfigPath), nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tmux-parator"), nil
}

func Load(mainConfigPath string) (Config, string, error) {
	path, err := Path(mainConfigPath)
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := LoadFile(path)
	return cfg, path, err
}

func LoadFile(path string) (Config, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, err
	}
	if info.IsDir() {
		return LoadDir(path)
	}
	var rawTemplate rawTemplate
	if err := decodeTemplateFile(path, &rawTemplate); err != nil {
		return Config{}, err
	}
	if rawTemplateEmpty(rawTemplate) {
		return Config{}, nil
	}
	template, err := normalizeTemplate(rawTemplate, filepath.Dir(path))
	if err != nil {
		return Config{}, fmt.Errorf("template %q: %w", rawTemplateLabel(rawTemplate, 1), err)
	}
	template.Source = SourceGlobal
	return Config{Templates: []Template{template}}, nil
}

func LoadDir(path string) (Config, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	cfg := Config{}
	ids := map[string]bool{}
	names := map[string]bool{}
	matchOwners := map[string]string{}
	templateFiles, err := filepath.Glob(filepath.Join(path, "templates", "*.toml"))
	if err != nil {
		return Config{}, err
	}
	sort.Strings(templateFiles)
	for _, templateFile := range templateFiles {
		if err := appendTemplateFile(&cfg, ids, names, matchOwners, templateFile); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func appendTemplateFile(cfg *Config, ids map[string]bool, names map[string]bool, matchOwners map[string]string, path string) error {
	var raw rawTemplate
	if err := decodeTemplateFile(path, &raw); err != nil {
		return err
	}
	if rawTemplateEmpty(raw) {
		return nil
	}
	template, err := normalizeTemplate(raw, filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("%s: template %q: %w", path, rawTemplateLabel(raw, 1), err)
	}
	template.Source = SourceGlobal
	return appendTemplates(cfg, ids, names, matchOwners, []Template{template})
}

func LoadLocal(path string) (Template, string, bool, error) {
	for _, candidate := range localTemplateCandidates(path) {
		if _, err := os.Stat(candidate); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Template{}, "", false, err
		}
		var raw rawTemplate
		if err := decodeTemplateFile(candidate, &raw); err != nil {
			return Template{}, candidate, true, err
		}
		if rawTemplateEmpty(raw) {
			return Template{}, candidate, false, nil
		}
		template, err := normalizeTemplate(raw, filepath.Dir(candidate))
		if err != nil {
			return Template{}, candidate, true, fmt.Errorf("template %q: %w", rawTemplateLabel(raw, 1), err)
		}
		template.Source = SourceLocal
		return template, candidate, true, nil
	}
	return Template{}, "", false, nil
}

func localTemplateCandidates(path string) []string {
	return []string{
		filepath.Join(path, ".tmux-parator", "template.toml"),
		filepath.Join(path, "tmux-parator", "template.toml"),
	}
}

func decodeTemplateFile(path string, raw *rawTemplate) error {
	_, err := toml.DecodeFile(path, raw)
	return err
}

func appendTemplates(cfg *Config, ids map[string]bool, names map[string]bool, matchOwners map[string]string, templates []Template) error {
	for _, template := range templates {
		if ids[template.ID] {
			return fmt.Errorf("template %q: duplicate id", template.ID)
		}
		if names[template.Name] {
			return fmt.Errorf("template %q: duplicate name", template.ID)
		}
		ids[template.ID] = true
		names[template.Name] = true
		for _, match := range template.Match {
			if owner, ok := matchOwners[match]; ok {
				return fmt.Errorf("template %q: match %q already used by template %q", template.ID, match, owner)
			}
			matchOwners[match] = template.ID
		}
		cfg.Templates = append(cfg.Templates, template)
	}
	return nil
}

func rawTemplateEmpty(template rawTemplate) bool {
	return strings.TrimSpace(template.ID) == "" &&
		strings.TrimSpace(template.Name) == "" &&
		strings.TrimSpace(template.SessionName) == "" &&
		strings.TrimSpace(template.Focus) == "" &&
		strings.TrimSpace(template.Description) == "" &&
		strings.TrimSpace(template.Chip) == "" &&
		template.WindowIndicators == nil &&
		template.Glyphs == nil &&
		len(template.Variables) == 0 &&
		len(template.Parameters) == 0 &&
		template.Enabled == nil &&
		template.Match == nil &&
		len(template.Hooks.BeforeCreateCommand) == 0 &&
		len(template.Hooks.BeforeCreateScript) == 0 &&
		len(template.Hooks.AfterCreateCommand) == 0 &&
		len(template.Hooks.AfterCreateScript) == 0 &&
		len(template.Windows) == 0
}

func normalizeTemplate(raw rawTemplate, configDir string) (Template, error) {
	match, err := optionalStringList(raw.Match, "match")
	if err != nil {
		return Template{}, err
	}
	if raw.WindowIndicators != nil && raw.Glyphs != nil {
		return Template{}, fmt.Errorf("window_indicators and glyphs are mutually exclusive")
	}
	windowIndicatorsValue := raw.WindowIndicators
	windowIndicatorsField := "window_indicators"
	if windowIndicatorsValue == nil {
		windowIndicatorsValue = raw.Glyphs
		windowIndicatorsField = "glyphs"
	}
	windowIndicators, err := optionalStringList(windowIndicatorsValue, windowIndicatorsField)
	if err != nil {
		return Template{}, err
	}
	match, err = normalizeMatchPatterns(match, configDir)
	if err != nil {
		return Template{}, err
	}
	beforeCreateHooks, err := normalizeHooks(raw.Hooks.BeforeCreateCommand, false, configDir, "before_create_command")
	if err != nil {
		return Template{}, err
	}
	beforeCreateScriptHooks, err := normalizeHooks(raw.Hooks.BeforeCreateScript, true, configDir, "before_create_script")
	if err != nil {
		return Template{}, err
	}
	afterCreateHooks, err := normalizeHooks(raw.Hooks.AfterCreateCommand, false, configDir, "after_create_command")
	if err != nil {
		return Template{}, err
	}
	afterCreateScriptHooks, err := normalizeHooks(raw.Hooks.AfterCreateScript, true, configDir, "after_create_script")
	if err != nil {
		return Template{}, err
	}
	beforeCreateHooks = append(beforeCreateHooks, beforeCreateScriptHooks...)
	afterCreateHooks = append(afterCreateHooks, afterCreateScriptHooks...)
	template := Template{
		ID:                strings.TrimSpace(raw.ID),
		Name:              strings.TrimSpace(raw.Name),
		SessionName:       strings.TrimSpace(raw.SessionName),
		Focus:             strings.TrimSpace(raw.Focus),
		Description:       strings.TrimSpace(raw.Description),
		Chip:              strings.TrimSpace(raw.Chip),
		WindowIndicators:  windowIndicators,
		Variables:         cloneVariables(raw.Variables),
		Enabled:           true,
		Source:            SourceGlobal,
		Match:             match,
		BeforeCreateHooks: beforeCreateHooks,
		AfterCreateHooks:  afterCreateHooks,
		Windows:           make([]Window, 0, len(raw.Windows)),
	}
	template.Parameters, err = normalizeParameters(raw.Parameters, template.Variables)
	if err != nil {
		return Template{}, err
	}
	if raw.Enabled != nil {
		template.Enabled = *raw.Enabled
	}
	if template.ID == "" {
		return Template{}, fmt.Errorf("id is required")
	}
	if template.Name == "" {
		return Template{}, fmt.Errorf("name is required")
	}
	if len(raw.Windows) == 0 {
		return Template{}, fmt.Errorf("at least one window is required")
	}
	windowNames := make(map[string]bool, len(raw.Windows))
	for i, rawWindow := range raw.Windows {
		window, err := normalizeWindow(rawWindow, configDir)
		if err != nil {
			return Template{}, fmt.Errorf("window %q: %w", rawWindowLabel(rawWindow, i+1), err)
		}
		if windowNames[window.Name] {
			return Template{}, fmt.Errorf("window %q: duplicate name", window.Name)
		}
		windowNames[window.Name] = true
		template.Windows = append(template.Windows, window)
	}
	if template.Focus == "" {
		return Template{}, fmt.Errorf("focus is required")
	}
	if !templateContainsStructuralInterpolation(template) && !templateFocusResolves(template.Focus, template.Windows) {
		return Template{}, fmt.Errorf("focus %q does not resolve to a pane", template.Focus)
	}
	return template, nil
}

func normalizeParameters(rawParameters []rawParameter, variables map[string]string) ([]Parameter, error) {
	parameters := make([]Parameter, 0, len(rawParameters))
	names := make(map[string]bool, len(rawParameters))
	for i, raw := range rawParameters {
		name := strings.TrimSpace(raw.Name)
		if !variableNamePattern.MatchString(name) {
			return nil, fmt.Errorf("parameter %d: name %q is invalid", i+1, name)
		}
		if isBuiltinVariable(name) {
			return nil, fmt.Errorf("parameter %q: name is reserved", name)
		}
		if _, exists := variables[name]; exists {
			return nil, fmt.Errorf("parameter %q: name conflicts with variables", name)
		}
		if names[name] {
			return nil, fmt.Errorf("parameter %q: duplicate name", name)
		}
		prompt := strings.TrimSpace(raw.Prompt)
		if prompt == "" {
			return nil, fmt.Errorf("parameter %q: prompt is required", name)
		}
		if len(raw.Options) == 0 {
			return nil, fmt.Errorf("parameter %q: options are required", name)
		}
		options := make([]string, 0, len(raw.Options))
		optionSet := make(map[string]bool, len(raw.Options))
		for _, rawOption := range raw.Options {
			option := strings.TrimSpace(rawOption)
			if option == "" {
				return nil, fmt.Errorf("parameter %q: options must not contain empty values", name)
			}
			if optionSet[option] {
				return nil, fmt.Errorf("parameter %q: duplicate option %q", name, option)
			}
			optionSet[option] = true
			options = append(options, option)
		}
		defaultValue := strings.TrimSpace(raw.Default)
		if defaultValue == "" {
			defaultValue = options[0]
		}
		if !optionSet[defaultValue] {
			return nil, fmt.Errorf("parameter %q: default %q is not in options", name, defaultValue)
		}
		names[name] = true
		parameters = append(parameters, Parameter{Name: name, Prompt: prompt, Options: options, Default: defaultValue})
	}
	return parameters, nil
}

func cloneVariables(variables map[string]string) map[string]string {
	if len(variables) == 0 {
		return nil
	}
	result := make(map[string]string, len(variables))
	for name, value := range variables {
		result[strings.TrimSpace(name)] = value
	}
	return result
}

func normalizeHooks(rawHooks []rawHook, scripts bool, configDir string, field string) ([]Hook, error) {
	hooks := make([]Hook, 0, len(rawHooks))
	for i, raw := range rawHooks {
		run := strings.TrimSpace(raw.Run)
		if run == "" {
			return nil, fmt.Errorf("%s entry %d: run is required", field, i+1)
		}
		hook := Hook{Run: run}
		if scripts {
			hook.Run = shellQuote(scriptCommand(configDir, run))
		}
		if len(raw.Kinds) > 0 {
			hook.Kinds = make([]string, 0, len(raw.Kinds))
			for _, kind := range raw.Kinds {
				kind = strings.TrimSpace(kind)
				if kind == "" {
					return nil, fmt.Errorf("%s entry %d: kinds must not contain empty values", field, i+1)
				}
				hook.Kinds = append(hook.Kinds, kind)
			}
		}
		hooks = append(hooks, hook)
	}
	return hooks, nil
}

func normalizeWindow(raw rawWindow, configDir string) (Window, error) {
	windowName := strings.TrimSpace(raw.Name)
	if windowName == "" {
		return Window{}, fmt.Errorf("name is required")
	}
	if len(raw.Layout) == 0 {
		return Window{}, fmt.Errorf("layout is required")
	}
	layout, err := normalizeNode("", raw.Layout, configDir)
	if err != nil {
		return Window{}, err
	}
	window := Window{
		Name:   windowName,
		Focus:  strings.TrimSpace(raw.Focus),
		When:   strings.TrimSpace(raw.When),
		Layout: layout,
	}
	if err := validateCondition(window.When); err != nil {
		return Window{}, fmt.Errorf("when: %w", err)
	}
	if window.Focus != "" && !containsInterpolation(window.Focus) && !nodeContainsInterpolation(window.Layout) && !focusResolves(window.Focus, window.Layout) {
		return Window{}, fmt.Errorf("focus %q does not resolve to a pane", window.Focus)
	}
	return window, nil
}

func normalizeNode(name string, raw map[string]interface{}, configDir string) (Node, error) {
	node := Node{Name: strings.TrimSpace(name)}
	if rawName, ok := stringValue(raw, "name"); ok {
		node.Name = rawName
	}
	node.Type, _ = stringValue(raw, "type")
	node.Type = strings.TrimSpace(node.Type)
	node.When, _ = stringValue(raw, "when")
	if _, exists := raw["when"]; exists && node.When == "" {
		return Node{}, fmt.Errorf("layout %q: when must be a non-empty string", displayNodeName(node.Name))
	}
	if err := validateCondition(node.When); err != nil {
		return Node{}, fmt.Errorf("layout %q: when: %w", displayNodeName(node.Name), err)
	}
	if node.Type == "" {
		return Node{}, fmt.Errorf("layout %q: type is required", displayNodeName(node.Name))
	}
	switch node.Type {
	case "pane":
		if strings.TrimSpace(node.Name) == "" {
			return Node{}, fmt.Errorf("pane name is required")
		}
		node.Path, _ = stringValue(raw, "path")
		node.CommandMode = CommandModeInteractive
		commands, hasCommand, err := optionalStringListValue(raw, "command")
		if err != nil {
			return Node{}, fmt.Errorf("pane %q: %w", displayNodeName(node.Name), err)
		}
		scripts, hasScript, err := optionalStringListValue(raw, "script")
		if err != nil {
			return Node{}, fmt.Errorf("pane %q: %w", displayNodeName(node.Name), err)
		}
		if hasCommand && hasScript {
			return Node{}, fmt.Errorf("pane %q cannot define both command and script", displayNodeName(node.Name))
		}
		if mode, ok := stringValue(raw, "command_mode"); ok {
			switch mode {
			case CommandModeInteractive, CommandModeWrapper:
				node.CommandMode = mode
			default:
				return Node{}, fmt.Errorf("pane %q: command_mode must be %q or %q", displayNodeName(node.Name), CommandModeInteractive, CommandModeWrapper)
			}
		}
		if hasCommand {
			node.Commands = append([]string(nil), commands...)
		}
		if hasScript {
			node.Commands = scriptCommands(configDir, scripts)
		}
		if len(node.Commands) > 0 {
			node.Command = node.Commands[0]
		}
		if _, ok := raw["children"]; ok {
			return Node{}, fmt.Errorf("pane %q cannot define children", displayNodeName(node.Name))
		}
		if _, ok := raw["sizes"]; ok {
			return Node{}, fmt.Errorf("pane %q cannot define sizes", displayNodeName(node.Name))
		}
		return node, nil
	case "columns", "rows":
	default:
		return Node{}, fmt.Errorf("layout %q: invalid type %q", displayNodeName(node.Name), node.Type)
	}

	childNames, err := stringSliceValue(raw, "children")
	if err != nil {
		return Node{}, fmt.Errorf("layout %q: %w", displayNodeName(node.Name), err)
	}
	sizes, err := intSliceValue(raw, "sizes")
	if err != nil {
		return Node{}, fmt.Errorf("layout %q: %w", displayNodeName(node.Name), err)
	}
	if len(childNames) == 0 {
		return Node{}, fmt.Errorf("layout %q: children are required", displayNodeName(node.Name))
	}
	if len(sizes) != len(childNames) {
		return Node{}, fmt.Errorf("layout %q: sizes count %d does not match children count %d", displayNodeName(node.Name), len(sizes), len(childNames))
	}
	for _, size := range sizes {
		if size <= 0 {
			return Node{}, fmt.Errorf("layout %q: sizes must be positive", displayNodeName(node.Name))
		}
	}
	node.Sizes = sizes
	node.Children = make([]Node, 0, len(childNames))
	for _, childName := range childNames {
		childRaw, ok := rawMapValue(raw, childName)
		if !ok {
			return Node{}, fmt.Errorf("pane %q: table is missing", childName)
		}
		child, err := normalizeNode(childName, childRaw, configDir)
		if err != nil {
			return Node{}, err
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

func stringValue(raw map[string]interface{}, key string) (string, bool) {
	value, ok := raw[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return strings.TrimSpace(text), ok
}

func stringSliceValue(raw map[string]interface{}, key string) ([]string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	values, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be a string array", key)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("%s must contain only non-empty strings", key)
		}
		result = append(result, strings.TrimSpace(text))
	}
	return result, nil
}

func optionalStringListValue(raw map[string]interface{}, key string) ([]string, bool, error) {
	value, ok := raw[key]
	if !ok {
		return nil, false, nil
	}
	result, err := optionalStringList(value, key)
	return result, true, err
}

func optionalStringList(value interface{}, key string) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	switch value := value.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s must be non-empty", key)
		}
		return []string{value}, nil
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("%s must contain only non-empty strings", key)
			}
			result = append(result, strings.TrimSpace(text))
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("%s must not be empty", key)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s must be a string or string array", key)
	}
}

func intSliceValue(raw map[string]interface{}, key string) ([]int, error) {
	value, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	values, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an integer array", key)
	}
	result := make([]int, 0, len(values))
	for _, value := range values {
		switch value := value.(type) {
		case int64:
			result = append(result, int(value))
		case int:
			result = append(result, value)
		default:
			return nil, fmt.Errorf("%s must contain only integers", key)
		}
	}
	return result, nil
}

func rawMapValue(raw map[string]interface{}, key string) (map[string]interface{}, bool) {
	value, ok := raw[key]
	if !ok {
		return nil, false
	}
	child, ok := value.(map[string]interface{})
	return child, ok
}

func scriptCommand(configDir string, script string) string {
	script = strings.TrimSpace(script)
	if script == "" {
		return ""
	}
	if filepath.IsAbs(script) {
		return script
	}
	return filepath.Join(configDir, "scripts", script)
}

func scriptCommands(configDir string, scripts []string) []string {
	commands := make([]string, 0, len(scripts))
	for _, script := range scripts {
		commands = append(commands, shellQuote(scriptCommand(configDir, script)))
	}
	return commands
}

func normalizeMatchPatterns(patterns []string, configDir string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		resolved, err := resolveMatchPattern(pattern, configDir)
		if err != nil {
			return nil, err
		}
		if _, err := filepath.Match(resolved, filepath.Clean(resolved)); err != nil {
			return nil, fmt.Errorf("match pattern %q is invalid: %w", pattern, err)
		}
		result = append(result, resolved)
	}
	return result, nil
}

func resolveMatchPattern(pattern string, configDir string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", fmt.Errorf("match must contain only non-empty strings")
	}
	if pattern == "~" || strings.HasPrefix(pattern, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if pattern == "~" {
			return filepath.Clean(home), nil
		}
		return filepath.Clean(filepath.Join(home, pattern[2:])), nil
	}
	if filepath.IsAbs(pattern) {
		return filepath.Clean(pattern), nil
	}
	if strings.TrimSpace(configDir) != "" {
		return filepath.Clean(filepath.Join(configDir, pattern)), nil
	}
	absolute, err := filepath.Abs(pattern)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func focusResolves(focus string, root Node) bool {
	focus = strings.TrimSpace(focus)
	if root.Type == "pane" {
		return focus == root.Name
	}
	parts := strings.Split(focus, ".")
	node := root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		found := false
		for _, child := range node.Children {
			if child.Name == part {
				node = child
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return node.Type == "pane"
}

func templateFocusResolves(focus string, windows []Window) bool {
	windowName, panePath, ok := strings.Cut(strings.TrimSpace(focus), ".")
	if !ok || strings.TrimSpace(windowName) == "" || strings.TrimSpace(panePath) == "" {
		return false
	}
	for _, window := range windows {
		if window.Name == windowName {
			return focusResolves(panePath, window.Layout)
		}
	}
	return false
}

func rawTemplateLabel(template rawTemplate, index int) string {
	if name := strings.TrimSpace(template.Name); name != "" {
		return name
	}
	if id := strings.TrimSpace(template.ID); id != "" {
		return id
	}
	return fmt.Sprintf("%d", index)
}

func rawWindowLabel(window rawWindow, index int) string {
	if name := strings.TrimSpace(window.Name); name != "" {
		return name
	}
	return fmt.Sprintf("%d", index)
}

func displayNodeName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "root"
	}
	return name
}

func EnabledTemplates(templates []Template) []Template {
	enabled := make([]Template, 0, len(templates))
	for _, template := range templates {
		if template.Enabled {
			enabled = append(enabled, template)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		return enabled[i].Name < enabled[j].Name
	})
	return enabled
}

func EnabledTemplatesInOrder(templates []Template) []Template {
	enabled := make([]Template, 0, len(templates))
	for _, template := range templates {
		if template.Enabled {
			enabled = append(enabled, template)
		}
	}
	return enabled
}

func MatchingTemplate(templates []Template, path string) (Template, bool) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return Template{}, false
	}
	bestScore := -1
	var best Template
	for _, template := range templates {
		if !template.Enabled {
			continue
		}
		for _, pattern := range template.Match {
			if matchesTemplatePattern(pattern, path) {
				score := templatePatternSpecificity(pattern)
				if score > bestScore {
					bestScore = score
					best = template
				}
			}
		}
	}
	if bestScore < 0 {
		return Template{}, false
	}
	return best, true
}

func matchesTemplatePattern(pattern string, path string) bool {
	pattern = filepath.Clean(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if containsGlobMeta(pattern) {
		matched, err := filepath.Match(pattern, path)
		return err == nil && matched
	}
	if pattern == path {
		return true
	}
	if pattern == string(filepath.Separator) {
		return strings.HasPrefix(path, string(filepath.Separator))
	}
	return strings.HasPrefix(path, pattern+string(filepath.Separator))
}

func templatePatternSpecificity(pattern string) int {
	pattern = filepath.Clean(strings.TrimSpace(pattern))
	if pattern == "" {
		return -1
	}
	if !containsGlobMeta(pattern) {
		return 1_000_000 + len(pattern)
	}
	return len(literalPrefixBeforeGlob(pattern))
}

func containsGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func literalPrefixBeforeGlob(pattern string) string {
	index := strings.IndexAny(pattern, "*?[")
	if index < 0 {
		return pattern
	}
	return pattern[:index]
}
