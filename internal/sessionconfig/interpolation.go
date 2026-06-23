package sessionconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type RenderContext struct {
	SessionName   string
	WorkspacePath string
	RepoRoot      string
	SessionKind   string
	Environment   map[string]string
}

const sessionNameSentinel = "\x00tmux-parator-session-name\x00"

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_.]*)\}`)
var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var builtinVariables = map[string]bool{
	"session_name":   true,
	"workspace_name": true,
	"workspace_path": true,
	"repo_root":      true,
	"session_kind":   true,
	"template_id":    true,
	"template_name":  true,
}

func Render(template Template, context RenderContext) (Template, error) {
	if len(template.Parameters) > 0 {
		return Template{}, fmt.Errorf("template %q has unresolved parameters", template.Name)
	}
	values := map[string]string{
		"session_name":   context.SessionName,
		"workspace_name": filepath.Base(filepath.Clean(context.WorkspacePath)),
		"workspace_path": context.WorkspacePath,
		"repo_root":      context.RepoRoot,
		"session_kind":   context.SessionKind,
		"template_id":    template.ID,
		"template_name":  template.Name,
	}
	resolvedVariables, err := resolveVariables(template.Variables, values, context.Environment)
	if err != nil {
		return Template{}, fmt.Errorf("template %q: %w", template.Name, err)
	}
	for name, value := range resolvedVariables {
		values[name] = value
	}

	rendered := template
	rendered.SessionName = context.SessionName
	rendered.Variables = resolvedVariables
	if rendered.Focus, err = interpolate(template.Focus, values, context.Environment); err != nil {
		return Template{}, interpolationFieldError(template, "focus", err)
	}
	rendered.BeforeCreateHooks, err = renderHooks(template.BeforeCreateHooks, values, context.Environment)
	if err != nil {
		return Template{}, interpolationFieldError(template, "before_create hook", err)
	}
	rendered.AfterCreateHooks, err = renderHooks(template.AfterCreateHooks, values, context.Environment)
	if err != nil {
		return Template{}, interpolationFieldError(template, "after_create hook", err)
	}
	rendered.Windows = make([]Window, 0, len(template.Windows))
	for i, window := range template.Windows {
		renderedWindow, include, err := renderWindow(window, values, context.Environment)
		if err != nil {
			return Template{}, interpolationFieldError(template, fmt.Sprintf("window %d", i+1), err)
		}
		if include {
			rendered.Windows = append(rendered.Windows, renderedWindow)
		}
	}
	if err := validateRenderedTemplate(rendered); err != nil {
		return Template{}, fmt.Errorf("template %q after interpolation: %w", template.Name, err)
	}
	return rendered, nil
}

func ResolveSessionName(template Template, context RenderContext) (string, error) {
	if strings.TrimSpace(template.SessionName) == "" {
		return "", nil
	}
	values := map[string]string{
		"session_name":   sessionNameSentinel,
		"workspace_name": filepath.Base(filepath.Clean(context.WorkspacePath)),
		"workspace_path": context.WorkspacePath,
		"repo_root":      context.RepoRoot,
		"session_kind":   context.SessionKind,
		"template_id":    template.ID,
		"template_name":  template.Name,
	}
	resolvedVariables, err := resolveVariables(template.Variables, values, context.Environment)
	if err != nil {
		return "", fmt.Errorf("template %q session_name: %w", template.Name, err)
	}
	for name, value := range resolvedVariables {
		values[name] = value
	}
	name, err := interpolate(template.SessionName, values, context.Environment)
	if err != nil {
		return "", fmt.Errorf("template %q session_name: %w", template.Name, err)
	}
	if strings.Contains(name, sessionNameSentinel) {
		return "", fmt.Errorf("template %q session_name cannot reference {session_name}", template.Name)
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("template %q session_name expands to an empty value", template.Name)
	}
	return name, nil
}

func WithParameterValues(template Template, values map[string]string) (Template, error) {
	if len(template.Parameters) == 0 {
		return template, nil
	}
	rendered := template
	rendered.Variables = cloneVariables(template.Variables)
	if rendered.Variables == nil {
		rendered.Variables = make(map[string]string, len(template.Parameters))
	}
	for _, parameter := range template.Parameters {
		value, ok := values[parameter.Name]
		if !ok {
			return Template{}, fmt.Errorf("template %q parameter %q has no selected value", template.Name, parameter.Name)
		}
		valid := false
		for _, option := range parameter.Options {
			if value == option {
				valid = true
				break
			}
		}
		if !valid {
			return Template{}, fmt.Errorf("template %q parameter %q has invalid value %q", template.Name, parameter.Name, value)
		}
		rendered.Variables[parameter.Name] = value
	}
	rendered.Parameters = nil
	return rendered, nil
}

func resolveVariables(variables map[string]string, builtins map[string]string, environment map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(variables))
	resolving := make(map[string]bool, len(variables))
	var resolve func(string) (string, error)
	resolve = func(name string) (string, error) {
		if value, ok := resolved[name]; ok {
			return value, nil
		}
		if resolving[name] {
			return "", fmt.Errorf("variable %q contains a cycle", name)
		}
		raw, ok := variables[name]
		if !ok {
			return "", fmt.Errorf("unknown variable %q", name)
		}
		resolving[name] = true
		values := make(map[string]string, len(builtins)+len(resolved))
		for key, value := range builtins {
			values[key] = value
		}
		for key, value := range resolved {
			values[key] = value
		}
		for dependency := range variables {
			if !containsNamedPlaceholder(raw, dependency) {
				continue
			}
			value, err := resolve(dependency)
			if err != nil {
				return "", err
			}
			values[dependency] = value
		}
		value, err := interpolate(raw, values, environment)
		if err != nil {
			return "", fmt.Errorf("variable %q: %w", name, err)
		}
		delete(resolving, name)
		resolved[name] = value
		return value, nil
	}
	for name := range variables {
		if !variableNamePattern.MatchString(name) {
			return nil, fmt.Errorf("variable name %q is invalid", name)
		}
		if isBuiltinVariable(name) {
			return nil, fmt.Errorf("variable name %q is reserved", name)
		}
		if _, err := resolve(name); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func isBuiltinVariable(name string) bool {
	return builtinVariables[name]
}

func renderHooks(hooks []Hook, values map[string]string, environment map[string]string) ([]Hook, error) {
	rendered := make([]Hook, len(hooks))
	for i, hook := range hooks {
		run, err := interpolate(hook.Run, values, environment)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i+1, err)
		}
		rendered[i] = hook
		rendered[i].Run = run
	}
	return rendered, nil
}

func renderWindow(window Window, values map[string]string, environment map[string]string) (Window, bool, error) {
	rendered := window
	var err error
	if rendered.When, err = interpolate(window.When, values, environment); err != nil {
		return Window{}, false, fmt.Errorf("when: %w", err)
	}
	include, err := evaluateCondition(rendered.When)
	if err != nil {
		return Window{}, false, fmt.Errorf("when %q: %w", rendered.When, err)
	}
	if !include {
		return Window{}, false, nil
	}
	if rendered.Name, err = interpolate(window.Name, values, environment); err != nil {
		return Window{}, false, fmt.Errorf("name: %w", err)
	}
	if rendered.Focus, err = interpolate(window.Focus, values, environment); err != nil {
		return Window{}, false, fmt.Errorf("%q focus: %w", rendered.Name, err)
	}
	var layoutIncluded bool
	if rendered.Layout, layoutIncluded, err = renderNode(window.Layout, values, environment); err != nil {
		return Window{}, false, fmt.Errorf("%q layout: %w", rendered.Name, err)
	}
	if !layoutIncluded {
		return Window{}, false, nil
	}
	return rendered, true, nil
}

func renderNode(node Node, values map[string]string, environment map[string]string) (Node, bool, error) {
	rendered := node
	var err error
	if rendered.When, err = interpolate(node.When, values, environment); err != nil {
		return Node{}, false, fmt.Errorf("when: %w", err)
	}
	include, err := evaluateCondition(rendered.When)
	if err != nil {
		return Node{}, false, fmt.Errorf("when %q: %w", rendered.When, err)
	}
	if !include {
		return Node{}, false, nil
	}
	if rendered.Name, err = interpolate(node.Name, values, environment); err != nil {
		return Node{}, false, fmt.Errorf("name: %w", err)
	}
	if rendered.Path, err = interpolate(node.Path, values, environment); err != nil {
		return Node{}, false, fmt.Errorf("pane %q path: %w", rendered.Name, err)
	}
	rendered.Commands = make([]string, len(node.Commands))
	for i, command := range node.Commands {
		rendered.Commands[i], err = interpolate(command, values, environment)
		if err != nil {
			return Node{}, false, fmt.Errorf("pane %q command %d: %w", rendered.Name, i+1, err)
		}
	}
	if len(rendered.Commands) > 0 {
		rendered.Command = rendered.Commands[0]
	} else if rendered.Command, err = interpolate(node.Command, values, environment); err != nil {
		return Node{}, false, fmt.Errorf("pane %q command: %w", rendered.Name, err)
	}
	rendered.Children = make([]Node, 0, len(node.Children))
	rendered.Sizes = make([]int, 0, len(node.Sizes))
	for i, child := range node.Children {
		renderedChild, childIncluded, err := renderNode(child, values, environment)
		if err != nil {
			return Node{}, false, err
		}
		if childIncluded {
			rendered.Children = append(rendered.Children, renderedChild)
			rendered.Sizes = append(rendered.Sizes, node.Sizes[i])
		}
	}
	if node.Type != "pane" && len(rendered.Children) == 0 {
		return Node{}, false, nil
	}
	return rendered, true, nil
}

func interpolate(value string, values map[string]string, environment map[string]string) (string, error) {
	const openBrace = "\x00tmux-parator-open-brace\x00"
	const closeBrace = "\x00tmux-parator-close-brace\x00"
	const shellVariable = "\x00tmux-parator-shell-variable\x00"
	value = strings.ReplaceAll(value, "${", shellVariable)
	value = strings.ReplaceAll(value, "{{", openBrace)
	value = strings.ReplaceAll(value, "}}", closeBrace)
	var interpolationErr error
	value = placeholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		if interpolationErr != nil {
			return match
		}
		name := match[1 : len(match)-1]
		if strings.HasPrefix(name, "env.") {
			envName := strings.TrimPrefix(name, "env.")
			if envName == "" {
				interpolationErr = fmt.Errorf("environment variable name is empty")
				return match
			}
			if environment != nil {
				if envValue, ok := environment[envName]; ok {
					return envValue
				}
			}
			if envValue, ok := os.LookupEnv(envName); ok {
				return envValue
			}
			interpolationErr = fmt.Errorf("environment variable %q is not set", envName)
			return match
		}
		replacement, ok := values[name]
		if !ok {
			interpolationErr = fmt.Errorf("unknown variable %q", name)
			return match
		}
		return replacement
	})
	if interpolationErr != nil {
		return "", interpolationErr
	}
	value = strings.ReplaceAll(value, openBrace, "{")
	value = strings.ReplaceAll(value, closeBrace, "}")
	value = strings.ReplaceAll(value, shellVariable, "${")
	return value, nil
}

func containsInterpolation(value string) bool {
	return placeholderPattern.MatchString(value)
}

func containsNamedPlaceholder(value string, name string) bool {
	return strings.Contains(value, "{"+name+"}")
}

func templateContainsStructuralInterpolation(template Template) bool {
	if containsInterpolation(template.Focus) {
		return true
	}
	for _, window := range template.Windows {
		if containsInterpolation(window.Name) || containsInterpolation(window.Focus) || containsInterpolation(window.When) || nodeContainsInterpolation(window.Layout) {
			return true
		}
	}
	return false
}

func nodeContainsInterpolation(node Node) bool {
	if containsInterpolation(node.Name) || containsInterpolation(node.When) {
		return true
	}
	for _, child := range node.Children {
		if nodeContainsInterpolation(child) {
			return true
		}
	}
	return false
}

func validateRenderedTemplate(template Template) error {
	if strings.TrimSpace(template.Focus) == "" {
		return fmt.Errorf("focus is empty")
	}
	if len(template.Windows) == 0 {
		return fmt.Errorf("conditions removed every window")
	}
	windowNames := make(map[string]bool, len(template.Windows))
	for _, window := range template.Windows {
		if strings.TrimSpace(window.Name) == "" {
			return fmt.Errorf("window name is empty")
		}
		if windowNames[window.Name] {
			return fmt.Errorf("window %q has a duplicate name", window.Name)
		}
		windowNames[window.Name] = true
		if window.Focus != "" && !focusResolves(window.Focus, window.Layout) {
			return fmt.Errorf("window %q focus %q does not resolve to a pane", window.Name, window.Focus)
		}
		if err := validateRenderedNode(window.Layout); err != nil {
			return fmt.Errorf("window %q: %w", window.Name, err)
		}
	}
	if !templateFocusResolves(template.Focus, template.Windows) {
		return fmt.Errorf("focus %q does not resolve to a pane", template.Focus)
	}
	return nil
}

func validateRenderedNode(node Node) error {
	if strings.TrimSpace(node.Name) == "" && node.Type == "pane" {
		return fmt.Errorf("pane name is empty")
	}
	childNames := make(map[string]bool, len(node.Children))
	if node.Type != "pane" && len(node.Children) == 0 {
		return fmt.Errorf("layout %q has no children after conditions", displayNodeName(node.Name))
	}
	if node.Type != "pane" && len(node.Sizes) != len(node.Children) {
		return fmt.Errorf("layout %q sizes count %d does not match children count %d", displayNodeName(node.Name), len(node.Sizes), len(node.Children))
	}
	for _, child := range node.Children {
		if strings.TrimSpace(child.Name) == "" {
			return fmt.Errorf("child name is empty")
		}
		if childNames[child.Name] {
			return fmt.Errorf("child %q has a duplicate name", child.Name)
		}
		childNames[child.Name] = true
		if err := validateRenderedNode(child); err != nil {
			return err
		}
	}
	return nil
}

func interpolationFieldError(template Template, field string, err error) error {
	return fmt.Errorf("template %q %s: %w", template.Name, field, err)
}
