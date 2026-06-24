package sessionconfig

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderInterpolatesTemplateCreationFields(t *testing.T) {
	template := Template{
		ID:        "repo",
		Name:      "Repository",
		Focus:     "{window_prefix}.{editor}",
		Variables: map[string]string{"window_prefix": "{session_name}-work", "editor": "editor"},
		Env:       map[string]string{"APP_ENV": "{session_kind}", "PROJECT_ROOT": "{workspace_path}"},
		BeforeCreateHooks: []Hook{
			{Run: "printf '%s' {workspace_path}"},
		},
		AfterCreateHooks: []Hook{
			{Run: "printf '%s' {env.READY_MESSAGE}"},
		},
		Windows: []Window{
			{
				Name:  "{window_prefix}",
				Focus: "{editor}",
				Layout: Node{
					Type:     "columns",
					Name:     "main",
					Sizes:    []int{1},
					Children: []Node{{Type: "pane", Name: "{editor}", Path: "{repo_root}", Commands: []string{"nvim {workspace_path}", "echo ${HOME}", "echo {{session_name}}"}}},
				},
			},
		},
	}

	rendered, err := Render(template, RenderContext{
		SessionName:   "aoc",
		WorkspacePath: "/tmp/repos/aoc",
		RepoRoot:      "/tmp/repos/aoc",
		SessionKind:   "repo",
		Environment:   map[string]string{"READY_MESSAGE": "ready"},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if rendered.Focus != "aoc-work.editor" {
		t.Fatalf("focus = %q, want aoc-work.editor", rendered.Focus)
	}
	if !reflect.DeepEqual(rendered.Env, map[string]string{"APP_ENV": "repo", "PROJECT_ROOT": "/tmp/repos/aoc"}) {
		t.Fatalf("env = %#v, want interpolated session environment", rendered.Env)
	}
	window := rendered.Windows[0]
	if window.Name != "aoc-work" || window.Focus != "editor" {
		t.Fatalf("window name/focus = %q/%q, want aoc-work/editor", window.Name, window.Focus)
	}
	pane := window.Layout.Children[0]
	if pane.Name != "editor" || pane.Path != "/tmp/repos/aoc" {
		t.Fatalf("pane name/path = %q/%q, want editor//tmp/repos/aoc", pane.Name, pane.Path)
	}
	wantCommands := []string{"nvim /tmp/repos/aoc", "echo ${HOME}", "echo {session_name}"}
	for i, want := range wantCommands {
		if pane.Commands[i] != want {
			t.Fatalf("command %d = %q, want %q", i, pane.Commands[i], want)
		}
	}
	if rendered.BeforeCreateHooks[0].Run != "printf '%s' /tmp/repos/aoc" {
		t.Fatalf("before hook = %q", rendered.BeforeCreateHooks[0].Run)
	}
	if rendered.AfterCreateHooks[0].Run != "printf '%s' ready" {
		t.Fatalf("after hook = %q", rendered.AfterCreateHooks[0].Run)
	}
}

func TestRenderRejectsUnknownVariableInEnv(t *testing.T) {
	template := Template{
		ID:    "repo",
		Name:  "Repository",
		Focus: "work.shell",
		Env:   map[string]string{"PROJECT_ROOT": "{missing}"},
		Windows: []Window{{
			Name:   "work",
			Layout: Node{Type: "pane", Name: "shell"},
		}},
	}

	_, err := Render(template, RenderContext{SessionName: "aoc", WorkspacePath: "/tmp/aoc"})
	if err == nil || !strings.Contains(err.Error(), `env: PROJECT_ROOT: unknown variable "missing"`) {
		t.Fatalf("Render() error = %v, want env interpolation error", err)
	}
}

func TestRenderRejectsUnknownVariablesBeforeCreation(t *testing.T) {
	template := Template{
		ID:    "repo",
		Name:  "Repository",
		Focus: "work.shell",
		Windows: []Window{{
			Name:   "work",
			Layout: Node{Type: "pane", Name: "shell", Command: "echo {missing}"},
		}},
	}

	_, err := Render(template, RenderContext{SessionName: "aoc", WorkspacePath: "/tmp/aoc"})
	if err == nil || !strings.Contains(err.Error(), `unknown variable "missing"`) {
		t.Fatalf("Render() error = %v, want unknown variable error", err)
	}
}

func TestRenderRejectsVariableCycles(t *testing.T) {
	template := Template{
		ID:        "repo",
		Name:      "Repository",
		Focus:     "work.shell",
		Variables: map[string]string{"first": "{second}", "second": "{first}"},
		Windows: []Window{{
			Name:   "work",
			Layout: Node{Type: "pane", Name: "shell"},
		}},
	}

	_, err := Render(template, RenderContext{SessionName: "aoc", WorkspacePath: "/tmp/aoc"})
	if err == nil || !strings.Contains(err.Error(), "contains a cycle") {
		t.Fatalf("Render() error = %v, want cycle error", err)
	}
}

func TestRenderRejectsDuplicateExpandedWindowNames(t *testing.T) {
	template := Template{
		ID:    "repo",
		Name:  "Repository",
		Focus: "{session_name}.one",
		Windows: []Window{
			{Name: "{session_name}", Layout: Node{Type: "pane", Name: "one"}},
			{Name: "aoc", Layout: Node{Type: "pane", Name: "two"}},
		},
	}

	_, err := Render(template, RenderContext{SessionName: "aoc", WorkspacePath: "/tmp/aoc"})
	if err == nil || !strings.Contains(err.Error(), `window "aoc" has a duplicate name`) {
		t.Fatalf("Render() error = %v, want duplicate window error", err)
	}
}

func TestRenderFiltersConditionalWindowsAndPanes(t *testing.T) {
	template := Template{
		ID:    "conditional",
		Name:  "Conditional",
		Focus: "work.editor",
		Variables: map[string]string{
			"agent": "none",
		},
		Windows: []Window{
			{
				Name: "work",
				Layout: Node{
					Name:  "main",
					Type:  "columns",
					Sizes: []int{70, 20, 10},
					Children: []Node{
						{Name: "editor", Type: "pane"},
						{Name: "agent", Type: "pane", When: `{agent} != "none"`},
						{Name: "tests", Type: "pane", When: `{session_kind} == "repo"`},
					},
				},
			},
			{
				Name:   "ci",
				When:   `{env.CI} == "true"`,
				Layout: Node{Name: "logs", Type: "pane"},
			},
		},
	}

	rendered, err := Render(template, RenderContext{
		SessionName:   "aoc",
		WorkspacePath: "/tmp/aoc",
		SessionKind:   "repo",
		Environment:   map[string]string{"CI": "false"},
	})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}
	if len(rendered.Windows) != 1 {
		t.Fatalf("windows = %#v, want only work", rendered.Windows)
	}
	layout := rendered.Windows[0].Layout
	gotNames := []string{layout.Children[0].Name, layout.Children[1].Name}
	if !reflect.DeepEqual(gotNames, []string{"editor", "tests"}) {
		t.Fatalf("children = %#v, want editor and tests", gotNames)
	}
	if !reflect.DeepEqual(layout.Sizes, []int{70, 10}) {
		t.Fatalf("sizes = %#v, want matching retained weights", layout.Sizes)
	}
}

func TestRenderRejectsFocusRemovedByCondition(t *testing.T) {
	template := Template{
		ID:    "conditional",
		Name:  "Conditional",
		Focus: "agent.agent",
		Variables: map[string]string{
			"agent": "none",
		},
		Windows: []Window{
			{Name: "work", Layout: Node{Name: "shell", Type: "pane"}},
			{Name: "agent", When: `{agent} != "none"`, Layout: Node{Name: "agent", Type: "pane"}},
		},
	}

	_, err := Render(template, RenderContext{SessionName: "aoc", WorkspacePath: "/tmp/aoc"})
	if err == nil || !strings.Contains(err.Error(), `focus "agent.agent" does not resolve to a pane`) {
		t.Fatalf("Render() error = %v, want removed focus error", err)
	}
}

func TestResolveSessionNameUsesWorkspaceAndVariables(t *testing.T) {
	template := Template{
		ID:          "repo",
		Name:        "Repository",
		SessionName: "{name_prefix}-{session_kind}",
		Variables:   map[string]string{"name_prefix": "{workspace_name}-dev"},
	}

	name, err := ResolveSessionName(template, RenderContext{
		WorkspacePath: "/tmp/repos/aoc",
		SessionKind:   "repo",
	})
	if err != nil {
		t.Fatalf("ResolveSessionName() error = %v", err)
	}
	if name != "aoc-dev-repo" {
		t.Fatalf("ResolveSessionName() = %q, want aoc-dev-repo", name)
	}
}

func TestResolveSessionNameRejectsFinalSessionNameReference(t *testing.T) {
	tests := []Template{
		{Name: "Direct", SessionName: "{session_name}-dev"},
		{Name: "Indirect", SessionName: "{prefix}-dev", Variables: map[string]string{"prefix": "{session_name}"}},
	}
	for _, template := range tests {
		t.Run(template.Name, func(t *testing.T) {
			_, err := ResolveSessionName(template, RenderContext{WorkspacePath: "/tmp/aoc"})
			if err == nil || !strings.Contains(err.Error(), "cannot reference {session_name}") {
				t.Fatalf("ResolveSessionName() error = %v, want recursion error", err)
			}
		})
	}
}

func TestWithParameterValuesMakesSelectionsAvailableToInterpolation(t *testing.T) {
	template := Template{
		Name: "Monitor",
		Parameters: []Parameter{{
			Name:    "monitor",
			Prompt:  "System monitor",
			Options: []string{"btop", "htop"},
			Default: "btop",
		}},
	}

	resolved, err := WithParameterValues(template, map[string]string{"monitor": "htop"})
	if err != nil {
		t.Fatalf("WithParameterValues() error = %v", err)
	}
	if resolved.Variables["monitor"] != "htop" || len(resolved.Parameters) != 0 {
		t.Fatalf("resolved template = %#v", resolved)
	}
}

func TestRenderRejectsUnresolvedParameters(t *testing.T) {
	template := Template{
		Name: "Monitor",
		Parameters: []Parameter{{
			Name:    "monitor",
			Prompt:  "System monitor",
			Options: []string{"btop", "htop"},
			Default: "btop",
		}},
	}

	_, err := Render(template, RenderContext{})
	if err == nil || !strings.Contains(err.Error(), "has unresolved parameters") {
		t.Fatalf("Render() error = %v, want unresolved parameters error", err)
	}
}
