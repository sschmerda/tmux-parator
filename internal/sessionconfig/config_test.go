package sessionconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadFileParsesNamedChildLayout(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "zen"
name = "Zen Mode"
focus = "work.main.editor"
description = "Wide middle pane"
chip = "z"
window_indicators = [" editor", " git"]
match = ["/tmp/repos/*", "~/work/*"]

[env]
PROJECT_ROOT = "{workspace_path}"
APP_ENV = "development"

[[hooks.before_create_command]]
run = "git fetch --quiet"

[[hooks.after_create_command]]
run = "echo ready"

[[windows]]
name = "work"
focus = "main.editor"

[windows.layout]
type = "columns"
sizes = [25, 50, 25]
children = ["left", "main", "right"]

[windows.layout.left]
type = "pane"

[windows.layout.main]
type = "rows"
sizes = [80, 20]
children = ["editor", "tests"]

[windows.layout.main.editor]
type = "pane"
path = "."
command = "nvim ."

[windows.layout.main.tests]
type = "pane"

[windows.layout.right]
type = "pane"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	if len(cfg.Templates) != 1 {
		t.Fatalf("templates len = %d, want 1", len(cfg.Templates))
	}
	template := cfg.Templates[0]
	if template.ID != "zen" || template.Name != "Zen Mode" || !template.Enabled {
		t.Fatalf("template = %#v, want zen enabled", template)
	}
	if template.Chip != "z" {
		t.Fatalf("template chip = %q, want z", template.Chip)
	}
	if !reflect.DeepEqual(template.WindowIndicators, []string{" editor", " git"}) {
		t.Fatalf("template window indicators = %#v, want editor and git indicators", template.WindowIndicators)
	}
	if len(template.Match) != 2 || template.Match[0] != filepath.Clean("/tmp/repos/*") {
		t.Fatalf("match = %#v, want normalized patterns", template.Match)
	}
	if !reflect.DeepEqual(template.Env, map[string]string{"APP_ENV": "development", "PROJECT_ROOT": "{workspace_path}"}) {
		t.Fatalf("env = %#v, want parsed template environment", template.Env)
	}
	if len(template.BeforeCreateHooks) != 1 || template.BeforeCreateHooks[0].Run != "git fetch --quiet" {
		t.Fatalf("before_create_hooks = %#v, want git fetch", template.BeforeCreateHooks)
	}
	if len(template.AfterCreateHooks) != 1 || template.AfterCreateHooks[0].Run != "echo ready" {
		t.Fatalf("after_create_hooks = %#v, want echo ready", template.AfterCreateHooks)
	}
	window := template.Windows[0]
	if window.Focus != "main.editor" || window.Layout.Type != "columns" {
		t.Fatalf("window/layout = %#v/%#v, want focus main.editor columns", window, window.Layout)
	}
	if len(window.Layout.Children) != 3 || window.Layout.Children[1].Type != "rows" {
		t.Fatalf("children = %#v, want nested rows in middle", window.Layout.Children)
	}
	editor := window.Layout.Children[1].Children[0]
	if editor.Name != "editor" || editor.Command != "nvim ." {
		t.Fatalf("editor pane = %#v, want editor command", editor)
	}
	if len(editor.Commands) != 1 || editor.Commands[0] != "nvim ." {
		t.Fatalf("editor commands = %#v, want single nvim command", editor.Commands)
	}
	if editor.CommandMode != CommandModeInteractive {
		t.Fatalf("editor command_mode = %q, want %q", editor.CommandMode, CommandModeInteractive)
	}
}

func TestLoadFileRejectsInvalidEnvVariableName(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad-env"
name = "Bad Env"
focus = "work.shell"

[env]
"APP-ENV" = "development"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), `env variable name "APP-ENV" is invalid`) {
		t.Fatalf("LoadFile() error = %v, want invalid env name error", err)
	}
}

func TestLoadFileAllowsInterpolatedStructuralNames(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "dynamic"
name = "Dynamic"
session_name = "{workspace_name}-dev"
focus = "{window_name}.{pane_name}"

[variables]
window_name = "{session_name}-work"
pane_name = "editor"

[[windows]]
name = "{window_name}"
focus = "{pane_name}"

[windows.layout]
type = "pane"
name = "{pane_name}"
command = "nvim {workspace_path}"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	template := cfg.Templates[0]
	if template.Variables["window_name"] != "{session_name}-work" {
		t.Fatalf("window_name variable = %q", template.Variables["window_name"])
	}
	if template.SessionName != "{workspace_name}-dev" {
		t.Fatalf("session_name = %q", template.SessionName)
	}
	rendered, err := Render(template, RenderContext{SessionName: "aoc", WorkspacePath: "/tmp/aoc"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if rendered.Focus != "aoc-work.editor" || rendered.Windows[0].Name != "aoc-work" {
		t.Fatalf("rendered focus/window = %q/%q", rendered.Focus, rendered.Windows[0].Name)
	}
}

func TestLoadFileParsesTemplateParameters(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "parameterized"
name = "Parameterized"
focus = "work.monitor"

[[parameters]]
name = "monitor"
prompt = "System monitor"
options = ["btop", "htop"]
default = "btop"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "monitor"
command = "{monitor}"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	parameter := cfg.Templates[0].Parameters[0]
	if parameter.Name != "monitor" || parameter.Prompt != "System monitor" || parameter.Default != "btop" {
		t.Fatalf("parameter = %#v", parameter)
	}
}

func TestLoadFileRejectsParameterDefaultOutsideOptions(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad-parameter"
name = "Bad Parameter"
focus = "work.monitor"

[[parameters]]
name = "monitor"
prompt = "System monitor"
options = ["btop", "htop"]
default = "top"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "monitor"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), `default "top" is not in options`) {
		t.Fatalf("LoadFile() error = %v, want invalid default error", err)
	}
}

func TestLoadFileAcceptsLegacyGlyphsAsWindowIndicators(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "legacy"
name = "Legacy"
focus = "work.shell"
glyphs = ["", ""]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	if got := cfg.Templates[0].WindowIndicators; !reflect.DeepEqual(got, []string{"", ""}) {
		t.Fatalf("window indicators = %#v, want legacy glyph values", got)
	}
}

func TestLoadFileRejectsWindowIndicatorsWithLegacyGlyphs(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "conflict"
name = "Conflict"
window_indicators = ["editor"]
glyphs = [""]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "window_indicators and glyphs are mutually exclusive") {
		t.Fatalf("LoadFile() err = %v, want conflicting indicator fields error", err)
	}
}

func TestLoadFileRejectsMissingTemplateFocus(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "missing-focus"
name = "Missing Focus"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "focus is required") {
		t.Fatalf("LoadFile() err = %v, want missing focus error", err)
	}
}

func TestLoadFileRejectsTemplateFocusThatDoesNotResolve(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad-focus"
name = "Bad Focus"
focus = "work.missing"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), `focus "work.missing" does not resolve to a pane`) {
		t.Fatalf("LoadFile() err = %v, want unresolved focus error", err)
	}
}

func TestLoadFileParsesSingleTemplateFile(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "repo"
name = "Repository"
focus = "work.shell"
description = "Repository workspace"
match = ["/tmp/repos/*"]

[[hooks.before_create_command]]
run = "git fetch --quiet"

[[hooks.after_create_command]]
run = "echo ready"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
command = "nvim ."
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	if len(cfg.Templates) != 1 {
		t.Fatalf("templates len = %d, want 1", len(cfg.Templates))
	}
	template := cfg.Templates[0]
	if template.ID != "repo" || template.Name != "Repository" || template.Windows[0].Layout.Command != "nvim ." {
		t.Fatalf("template = %#v, want repo template", template)
	}
	if got := template.Windows[0].Layout.Commands; len(got) != 1 || got[0] != "nvim ." {
		t.Fatalf("commands = %#v, want single nvim command", got)
	}
	if len(template.Match) != 1 || template.Match[0] != filepath.Clean("/tmp/repos/*") {
		t.Fatalf("match = %#v, want /tmp/repos/*", template.Match)
	}
	if len(template.BeforeCreateHooks) != 1 || template.BeforeCreateHooks[0].Run != "git fetch --quiet" {
		t.Fatalf("before_create_hooks = %#v, want git fetch", template.BeforeCreateHooks)
	}
	if len(template.AfterCreateHooks) != 1 || template.AfterCreateHooks[0].Run != "echo ready" {
		t.Fatalf("after_create_hooks = %#v, want echo ready", template.AfterCreateHooks)
	}
}

func TestLoadFileResolvesTemplateHooksFromConfigScriptsDir(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "repo"
name = "Repository"
focus = "work.shell"

[[hooks.before_create_script]]
run = "setup.sh"
kinds = ["repo"]

[[hooks.before_create_script]]
run = "prepare.sh"

[[hooks.after_create_script]]
run = "ready.sh"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	beforeWant := []Hook{
		{Run: shellQuote(filepath.Join(filepath.Dir(path), "scripts", "setup.sh")), Kinds: []string{"repo"}},
		{Run: shellQuote(filepath.Join(filepath.Dir(path), "scripts", "prepare.sh"))},
	}
	if got := cfg.Templates[0].BeforeCreateHooks; !reflect.DeepEqual(got, beforeWant) {
		t.Fatalf("before_create_hooks = %#v, want %#v", got, beforeWant)
	}
	afterWant := []Hook{
		{Run: shellQuote(filepath.Join(filepath.Dir(path), "scripts", "ready.sh"))},
	}
	if got := cfg.Templates[0].AfterCreateHooks; !reflect.DeepEqual(got, afterWant) {
		t.Fatalf("after_create_hooks = %#v, want %#v", got, afterWant)
	}
}

func TestLoadDirLoadsTemplateFilesInFilenameOrder(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFile(t, dir, "templates/20-go.toml", `
id = "go"
name = "Go"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)
	writeTemplateFile(t, dir, "templates/10-repo.toml", `
id = "repo"
name = "Repository"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() unexpected error: %v", err)
	}
	if len(cfg.Templates) != 2 || cfg.Templates[0].ID != "repo" || cfg.Templates[1].ID != "go" {
		t.Fatalf("templates = %#v, want repo then go", cfg.Templates)
	}
}

func TestLoadDirResolvesTemplateScriptFromTemplatesScriptsDir(t *testing.T) {
	dir := t.TempDir()
	path := writeTemplateFile(t, dir, "templates/repo.toml", `
id = "repo"
name = "Repository"
focus = "work.notes"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "notes"
script = "notes.sh"
`)

	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() unexpected error: %v", err)
	}
	got := cfg.Templates[0].Windows[0].Layout.Command
	want := shellQuote(filepath.Join(filepath.Dir(path), "scripts", "notes.sh"))
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got := cfg.Templates[0].Windows[0].Layout.Commands; len(got) != 1 || got[0] != want {
		t.Fatalf("commands = %#v, want %#v", got, []string{want})
	}
}

func TestLoadDirResolvesTemplateHooksFromTemplatesScriptsDir(t *testing.T) {
	dir := t.TempDir()
	path := writeTemplateFile(t, dir, "templates/repo.toml", `
id = "repo"
name = "Repository"
focus = "work.shell"

[[hooks.before_create_script]]
run = "setup.sh"

[[hooks.after_create_script]]
run = "ready.sh"

[[hooks.after_create_script]]
run = "notify.sh"
kinds = ["repo", "subdir"]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() unexpected error: %v", err)
	}
	beforeWant := []Hook{{Run: shellQuote(filepath.Join(filepath.Dir(path), "scripts", "setup.sh"))}}
	if got := cfg.Templates[0].BeforeCreateHooks; !reflect.DeepEqual(got, beforeWant) {
		t.Fatalf("before_create_hooks = %#v, want %#v", got, beforeWant)
	}
	afterWant := []Hook{
		{Run: shellQuote(filepath.Join(filepath.Dir(path), "scripts", "ready.sh"))},
		{Run: shellQuote(filepath.Join(filepath.Dir(path), "scripts", "notify.sh")), Kinds: []string{"repo", "subdir"}},
	}
	if got := cfg.Templates[0].AfterCreateHooks; !reflect.DeepEqual(got, afterWant) {
		t.Fatalf("after_create_hooks = %#v, want %#v", got, afterWant)
	}
}

func TestLoadLocalParsesProjectTOMLTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".tmux-parator", "template.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create local template dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`
id = "local"
name = "Local Workspace"
focus = "work.shell"
description = "Local project template"
match = ["/tmp/ignored"]

[[hooks.before_create_command]]
run = "git fetch --quiet"
kinds = ["repo"]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
command = ["git status --short", "git branch --show-current"]
`), 0o644); err != nil {
		t.Fatalf("write local template: %v", err)
	}

	template, gotPath, ok, err := LoadLocal(dir)
	if err != nil {
		t.Fatalf("LoadLocal() unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("LoadLocal() ok = false, want true")
	}
	if gotPath != path {
		t.Fatalf("LoadLocal() path = %q, want %q", gotPath, path)
	}
	if template.Source != SourceLocal || template.ID != "local" || template.Name != "Local Workspace" {
		t.Fatalf("template = %#v, want local source/id/name", template)
	}
	wantHooks := []Hook{{Run: "git fetch --quiet", Kinds: []string{"repo"}}}
	if !reflect.DeepEqual(template.BeforeCreateHooks, wantHooks) {
		t.Fatalf("before hooks = %#v, want %#v", template.BeforeCreateHooks, wantHooks)
	}
	wantCommands := []string{"git status --short", "git branch --show-current"}
	if got := template.Windows[0].Layout.Commands; !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("pane commands = %#v, want %#v", got, wantCommands)
	}
}

func TestLoadDirRejectsDuplicateTemplateIDs(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFile(t, dir, "templates/one.toml", `
id = "repo"
name = "One"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)
	writeTemplateFile(t, dir, "templates/two.toml", `
id = "repo"
name = "Two"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadDir(dir)
	want := `template "repo": duplicate id`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadDir() err = %v, want %q", err, want)
	}
}

func TestLoadDirRejectsDuplicateTemplateNames(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFile(t, dir, "templates/one.toml", `
id = "repo-one"
name = "Repository"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)
	writeTemplateFile(t, dir, "templates/two.toml", `
id = "repo-two"
name = "Repository"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadDir(dir)
	want := `template "repo-two": duplicate name`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadDir() err = %v, want %q", err, want)
	}
}

func TestLoadDirRejectsDuplicateMatchPatterns(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFile(t, dir, "templates/one.toml", `
id = "one"
name = "One"
focus = "work.shell"
match = ["/tmp/repos/*"]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)
	writeTemplateFile(t, dir, "templates/two.toml", `
id = "two"
name = "Two"
focus = "work.shell"
match = ["/tmp/repos/*"]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadDir(dir)
	want := `match "/tmp/repos/*" already used by template "one"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadDir() err = %v, want %q", err, want)
	}
}

func TestMatchingTemplateUsesMostSpecificTemplate(t *testing.T) {
	templates := []Template{
		{ID: "broad", Name: "Broad", Enabled: true, Match: []string{filepath.Clean("/tmp/repos/*")}},
		{ID: "specific", Name: "Specific", Enabled: true, Match: []string{filepath.Clean("/tmp/repos/project")}},
	}

	got, ok := MatchingTemplate(templates, "/tmp/repos/project")
	if !ok || got.ID != "specific" {
		t.Fatalf("MatchingTemplate() = (%#v,%v), want specific true", got, ok)
	}
}

func TestMatchingTemplateIgnoresDisabledTemplates(t *testing.T) {
	templates := []Template{
		{ID: "disabled", Name: "Disabled", Enabled: false, Match: []string{filepath.Clean("/tmp/repos/*")}},
		{ID: "enabled", Name: "Enabled", Enabled: true, Match: []string{filepath.Clean("/tmp/repos/*")}},
	}

	got, ok := MatchingTemplate(templates, "/tmp/repos/project")
	if !ok || got.ID != "enabled" {
		t.Fatalf("MatchingTemplate() = (%#v,%v), want enabled true", got, ok)
	}
}

func TestMatchingTemplatePrefersBroaderTemplateForSubdirectories(t *testing.T) {
	templates := []Template{
		{ID: "code", Name: "Code", Enabled: true, Match: []string{filepath.Clean("/tmp/stefan/code")}},
		{ID: "repos", Name: "Repos", Enabled: true, Match: []string{filepath.Clean("/tmp/stefan/code/repos")}},
	}

	got, ok := MatchingTemplate(templates, "/tmp/stefan/code/repos/project")
	if !ok || got.ID != "repos" {
		t.Fatalf("MatchingTemplate() = (%#v,%v), want repos true", got, ok)
	}
	got, ok = MatchingTemplate(templates, "/tmp/stefan/code/other")
	if !ok || got.ID != "code" {
		t.Fatalf("MatchingTemplate() = (%#v,%v), want code true", got, ok)
	}
}

func TestLoadFileAcceptsRelativeSizesThatDoNotSumTo100(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "thirds"
name = "Thirds"
focus = "work.main"

[[windows]]
name = "work"

[windows.layout]
type = "columns"
sizes = [33, 33, 33]
children = ["left", "main", "right"]

[windows.layout.left]
type = "pane"

[windows.layout.main]
type = "pane"

[windows.layout.right]
type = "pane"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	got := cfg.Templates[0].Windows[0].Layout.Sizes
	want := []int{33, 33, 33}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("sizes = %#v, want %#v", got, want)
	}
}

func TestLoadFileResolvesPaneScriptFromConfigScriptsDir(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "script"
name = "Script"
focus = "work.setup"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "setup"
path = "."
script = "setup.sh"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	got := cfg.Templates[0].Windows[0].Layout.Command
	want := shellQuote(filepath.Join(filepath.Dir(path), "scripts", "setup.sh"))
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}

func TestLoadFilePreservesPaneCommandList(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "commands"
name = "Commands"
focus = "work.setup"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "setup"
command = ["make generate", "go test ./..."]
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	if got := cfg.Templates[0].Windows[0].Layout.Command; got != "make generate" {
		t.Fatalf("command = %q, want %q", got, "make generate")
	}
	want := []string{"make generate", "go test ./..."}
	if got := cfg.Templates[0].Windows[0].Layout.Commands; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestLoadFilePreservesPaneScriptList(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "scripts"
name = "Scripts"
focus = "work.setup"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "setup"
script = ["setup.sh", "start.sh"]
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	first := shellQuote(filepath.Join(filepath.Dir(path), "scripts", "setup.sh"))
	second := shellQuote(filepath.Join(filepath.Dir(path), "scripts", "start.sh"))
	if got := cfg.Templates[0].Windows[0].Layout.Command; got != first {
		t.Fatalf("command = %q, want %q", got, first)
	}
	want := []string{first, second}
	if got := cfg.Templates[0].Windows[0].Layout.Commands; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestLoadFileParsesPaneCommandMode(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "wrapper"
name = "Wrapper"
focus = "work.setup"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "setup"
command = "yazi"
command_mode = "wrapper"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	got := cfg.Templates[0].Windows[0].Layout.CommandMode
	if got != CommandModeWrapper {
		t.Fatalf("command_mode = %q, want %q", got, CommandModeWrapper)
	}
}

func TestLoadFileParsesWindowAndPaneConditions(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "conditional"
name = "Conditional"
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "columns"
sizes = [70, 30]
children = ["shell", "agent"]

[windows.layout.shell]
type = "pane"

[windows.layout.agent]
type = "pane"
when = '{agent} != "none"'

[[windows]]
name = "ci"
when = '{env.CI} == "true"'

[windows.layout]
type = "pane"
name = "logs"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	template := cfg.Templates[0]
	if got := template.Windows[0].Layout.Children[1].When; got != `{agent} != "none"` {
		t.Fatalf("pane when = %q", got)
	}
	if got := template.Windows[1].When; got != `{env.CI} == "true"` {
		t.Fatalf("window when = %q", got)
	}
}

func TestLoadFileRejectsInvalidCondition(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "conditional"
name = "Conditional"
focus = "work.shell"

[[windows]]
name = "work"
when = "{session_kind}"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "must contain an == or != comparison") {
		t.Fatalf("LoadFile() error = %v, want invalid condition", err)
	}
}

func TestLoadFileRejectsInvalidPaneCommandMode(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "setup"
command = "yazi"
command_mode = "silent"
`)

	_, err := LoadFile(path)
	want := `template "Bad": window "work": pane "setup": command_mode must be "interactive" or "wrapper"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileRejectsPaneCommandAndScript(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "setup"
command = "echo command"
script = "setup.sh"
`)

	_, err := LoadFile(path)
	want := `template "Bad": window "work": pane "setup" cannot define both command and script`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileRejectsMissingHookRun(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[hooks.before_create_command]]
kinds = ["repo"]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	want := `template "Bad": before_create_command entry 1: run is required`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileRejectsEmptyHookKind(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[hooks.after_create_script]]
run = "ready.sh"
kinds = ["repo", ""]

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	want := `template "Bad": after_create_script entry 1: kinds must not contain empty values`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileRejectsMissingNamedChild(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[windows]]
name = "work"

[windows.layout]
type = "rows"
sizes = [50, 50]
children = ["top", "bottom"]

[windows.layout.top]
type = "pane"
`)

	_, err := LoadFile(path)
	want := `template "Bad": window "work": pane "bottom": table is missing`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileRejectsUnnamedWindow(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[windows]]

[windows.layout]
type = "pane"
name = "shell"
`)

	_, err := LoadFile(path)
	want := `template "Bad": window "1": name is required`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileRejectsUnnamedRootPane(t *testing.T) {
	path := writeTemplateConfig(t, `
id = "bad"
name = "Bad"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
`)

	_, err := LoadFile(path)
	want := `template "Bad": window "work": pane name is required`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadFile() err = %v, want %q", err, want)
	}
}

func TestLoadFileMissingReturnsEmptyConfig(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}
	if len(cfg.Templates) != 0 {
		t.Fatalf("templates len = %d, want 0", len(cfg.Templates))
	}
}

func TestEnabledTemplatesFiltersAndSorts(t *testing.T) {
	got := EnabledTemplates([]Template{
		{ID: "z", Name: "Zed", Enabled: true},
		{ID: "off", Name: "Off", Enabled: false},
		{ID: "a", Name: "Alpha", Enabled: true},
	})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "z" {
		t.Fatalf("EnabledTemplates() = %#v, want sorted enabled templates", got)
	}
}

func writeTemplateConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write template config: %v", err)
	}
	return path
}

func writeTemplateFile(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create template dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}
	return path
}
