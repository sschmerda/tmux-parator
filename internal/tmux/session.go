package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sschmerda/tmux-parator/internal/sessionconfig"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type dirRunner interface {
	RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func (ExecRunner) RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	cmd.Env = nonInteractiveHookEnv(os.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.CombinedOutput()
}

func nonInteractiveHookEnv(environ []string) []string {
	values := map[string]string{
		"GCM_INTERACTIVE":     "Never",
		"GIT_ASKPASS":         "false",
		"GIT_SSH_COMMAND":     "ssh -oBatchMode=yes",
		"GIT_TERMINAL_PROMPT": "0",
		"SSH_ASKPASS":         "false",
		"SSH_ASKPASS_REQUIRE": "never",
	}
	result := make([]string, 0, len(environ)+len(values))
	for _, entry := range environ {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := values[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

type Session struct {
	Name        string
	Windows     string
	Attached    bool
	Current     bool
	CreatedTime string
	Activity    int64
	CurrentPath string
	Metadata    SessionMetadata
}

type SessionMetadata struct {
	CreatedByParator bool
	Kind             string
	Path             string
	Root             string
	BaseName         string
	Glyph            string
	GlyphColor       string
}

type Client struct {
	runner           Runner
	panePollInterval time.Duration
}

func NewClient(runner Runner) Client {
	return Client{
		runner:           runner,
		panePollInterval: 10 * time.Millisecond,
	}
}

func (c Client) ListSessions(ctx context.Context) ([]Session, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{session_windows}",
		"#{session_attached}",
		"#{==:#{session_name},#{client_session}}",
		"#{session_created_string}",
		"#{session_activity}",
		"#{@tmux-parator.created}",
		"#{@tmux-parator.kind}",
		"#{@tmux-parator.path}",
		"#{@tmux-parator.root}",
		"#{@tmux-parator.base_name}",
		"#{@tmux-parator.glyph}",
		"#{@tmux-parator.glyph_color}",
		"#{pane_current_path}",
	}, sessionFieldSeparator)
	out, err := c.runner.Run(ctx, "tmux", "list-sessions", "-F", format)
	if err != nil {
		return nil, commandError("list tmux sessions", out, err)
	}
	return ParseSessions(out), nil
}

func (c Client) SwitchSession(ctx context.Context, name string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	out, err := c.runner.Run(ctx, "tmux", "switch-client", "-t", exactSessionTarget(name))
	if err != nil {
		return commandError("switch tmux session", out, err)
	}
	return nil
}

func (c Client) SwitchLastSession(ctx context.Context) error {
	out, err := c.runner.Run(ctx, "tmux", "switch-client", "-l")
	if err != nil {
		return commandError("switch to last tmux session", out, err)
	}
	return nil
}

func (c Client) KillSession(ctx context.Context, name string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	out, err := c.runner.Run(ctx, "tmux", "kill-session", "-t", exactSessionTarget(name))
	if err != nil {
		return commandError("kill tmux session", out, err)
	}
	return nil
}

func (c Client) RenameSession(ctx context.Context, oldName string, newName string) error {
	if err := validateSessionName(oldName); err != nil {
		return err
	}
	if err := validateSessionName(newName); err != nil {
		return err
	}
	out, err := c.runner.Run(ctx, "tmux", "rename-session", "-t", exactSessionTarget(oldName), newName)
	if err != nil {
		return commandError("rename tmux session", out, err)
	}
	return nil
}

func (c Client) NewSession(ctx context.Context, name string, path string, metadata SessionMetadata) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	args := []string{"new-session", "-d", "-s", name}
	if strings.TrimSpace(path) != "" {
		args = append(args, "-c", path)
	}
	out, err := c.runner.Run(ctx, "tmux", args...)
	if err != nil {
		return commandError("create tmux session", out, err)
	}
	if err := c.setSessionMetadata(ctx, name, metadata); err != nil {
		return err
	}
	return nil
}

func (c Client) NewSessionWithLayout(ctx context.Context, name string, path string, metadata SessionMetadata, template sessionconfig.Template) (err error) {
	return c.newSessionWithLayout(ctx, name, path, metadata, template, false)
}

func (c Client) NewSessionWithLayoutAndSwitch(ctx context.Context, name string, path string, metadata SessionMetadata, template sessionconfig.Template) (err error) {
	return c.newSessionWithLayout(ctx, name, path, metadata, template, true)
}

func (c Client) newSessionWithLayout(ctx context.Context, name string, path string, metadata SessionMetadata, template sessionconfig.Template, switchAfterCreation bool) (err error) {
	if err := validateSessionName(name); err != nil {
		return err
	}
	if len(template.Windows) == 0 {
		return fmt.Errorf("template %q has no windows", template.Name)
	}
	if strings.TrimSpace(template.Focus) == "" {
		return fmt.Errorf("template %q has no focus", template.Name)
	}
	template, err = sessionconfig.Render(template, sessionconfig.RenderContext{
		SessionName:   name,
		WorkspacePath: path,
		RepoRoot:      metadata.Root,
		SessionKind:   metadata.Kind,
	})
	if err != nil {
		return err
	}
	if err := c.runTemplateHooks(ctx, path, "before_create", metadata.Kind, template.BeforeCreateHooks); err != nil {
		return err
	}
	firstWindow := template.Windows[0]
	args := []string{"new-session", "-d"}
	if width, height, ok := c.currentWindowSize(ctx); ok {
		args = append(args, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height))
	}
	args = appendTemplateEnvArgs(args, template.Env)
	args = append(args, "-P", "-F", "#{window_id} #{pane_id}", "-s", name, "-n", firstWindow.Name)
	if createPath := nodeCreatePath(path, firstWindow.Layout); strings.TrimSpace(createPath) != "" {
		args = append(args, "-c", createPath)
	}
	out, err := c.runner.Run(ctx, "tmux", args...)
	if err != nil {
		return commandError("create tmux session", out, err)
	}
	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := c.KillSession(context.WithoutCancel(ctx), name); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback partial tmux session: %w", cleanupErr))
		}
	}()
	firstWindowID, firstPane, err := parseWindowPaneIDs(out)
	if err != nil {
		return fmt.Errorf("create tmux session: %w", err)
	}
	if err := c.setSessionEnvironment(ctx, name, template.Env); err != nil {
		return err
	}
	windowTargets := map[string]windowTarget{}
	var paneStartups []paneStartup
	firstResult, err := c.applyWindowLayout(ctx, path, firstWindow, firstPane)
	if err != nil {
		return err
	}
	if firstWindow.Focus != "" {
		if err := c.selectPane(ctx, firstResult.focusPane); err != nil {
			return err
		}
	}
	paneStartups = append(paneStartups, firstResult.startups...)
	windowTargets[firstWindow.Name] = windowTarget{id: firstWindowID, panes: firstResult.panes}
	for _, window := range template.Windows[1:] {
		args := []string{"new-window", "-d", "-P", "-F", "#{window_id} #{pane_id}", "-t", exactSessionTarget(name), "-n", window.Name}
		if createPath := nodeCreatePath(path, window.Layout); strings.TrimSpace(createPath) != "" {
			args = append(args, "-c", createPath)
		}
		out, err := c.runner.Run(ctx, "tmux", args...)
		if err != nil {
			return commandError("create tmux window", out, err)
		}
		windowID, paneID, err := parseWindowPaneIDs(out)
		if err != nil {
			return fmt.Errorf("create tmux window %q: %w", window.Name, err)
		}
		result, err := c.applyWindowLayout(ctx, path, window, paneID)
		if err != nil {
			return err
		}
		if window.Focus != "" {
			if err := c.selectPane(ctx, result.focusPane); err != nil {
				return err
			}
		}
		paneStartups = append(paneStartups, result.startups...)
		windowTargets[window.Name] = windowTarget{id: windowID, panes: result.panes}
	}
	if err := c.setSessionMetadata(ctx, name, metadata); err != nil {
		return err
	}
	focusWindow, focusPane, err := resolveTemplateFocus(template.Focus, windowTargets)
	if err != nil {
		return fmt.Errorf("template %q: %w", template.Name, err)
	}
	if err := c.runTemplateHooks(ctx, path, "after_create", metadata.Kind, template.AfterCreateHooks); err != nil {
		return err
	}
	if err := c.startStrictPaneCommands(ctx, paneStartups); err != nil {
		return err
	}
	out, err = c.runner.Run(ctx, "tmux", "select-window", "-t", focusWindow)
	if err != nil {
		return commandError("select tmux window", out, err)
	}
	if err := c.selectPane(ctx, focusPane); err != nil {
		return err
	}
	if err := c.queueInteractivePaneCommands(ctx, paneStartups); err != nil {
		return err
	}
	if switchAfterCreation {
		if err := c.SwitchSession(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func appendTemplateEnvArgs(args []string, env map[string]string) []string {
	for _, name := range sortedMapKeys(env) {
		args = append(args, "-e", name+"="+env[name])
	}
	return args
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c Client) setSessionEnvironment(ctx context.Context, name string, env map[string]string) error {
	for _, key := range sortedMapKeys(env) {
		out, err := c.runner.Run(ctx, "tmux", "set-environment", "-t", exactSessionTarget(name), key, env[key])
		if err != nil {
			return commandError("set tmux session environment", out, err)
		}
	}
	return nil
}

func (c Client) runTemplateHooks(ctx context.Context, path string, name string, sessionKind string, hooks []sessionconfig.Hook) error {
	for _, hook := range hooks {
		if !hookMatchesKind(hook, sessionKind) {
			continue
		}
		command := strings.TrimSpace(hook.Run)
		if command == "" {
			continue
		}
		out, err := c.runShellCommand(ctx, path, command)
		if err != nil {
			return commandError("run "+name+" hook", out, err)
		}
	}
	return nil
}

func hookMatchesKind(hook sessionconfig.Hook, sessionKind string) bool {
	if len(hook.Kinds) == 0 {
		return true
	}
	sessionKind = strings.TrimSpace(sessionKind)
	for _, kind := range hook.Kinds {
		if strings.TrimSpace(kind) == sessionKind {
			return true
		}
	}
	return false
}

func (c Client) runShellCommand(ctx context.Context, path string, command string) ([]byte, error) {
	if runner, ok := c.runner.(dirRunner); ok {
		return runner.RunInDir(ctx, path, "/bin/sh", "-c", command)
	}
	if strings.TrimSpace(path) != "" {
		command = "cd " + shellQuote(path) + " && " + command
	}
	return c.runner.Run(ctx, "/bin/sh", "-c", command)
}

func parseWindowPaneIDs(out []byte) (string, string, error) {
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return "", "", fmt.Errorf("missing window or pane id")
	}
	return fields[0], fields[1], nil
}

type layoutResult struct {
	focusPane string
	panes     map[string]string
	nodes     map[string]string
	startups  []paneStartup
}

type paneStartup struct {
	paneID string
	path   string
	node   sessionconfig.Node
}

type windowTarget struct {
	id    string
	panes map[string]string
}

func (c Client) applyWindowLayout(ctx context.Context, basePath string, window sessionconfig.Window, paneID string) (layoutResult, error) {
	result, err := c.applyLayoutNode(ctx, basePath, paneID, window.Layout, "")
	if err != nil {
		return layoutResult{}, err
	}
	width, height, err := c.windowSize(ctx, result.focusPane)
	if err != nil {
		return layoutResult{}, err
	}
	if err := c.enforceLayoutSizes(ctx, window.Layout, result, width, height, ""); err != nil {
		return layoutResult{}, err
	}
	if window.Focus != "" {
		if pane, ok := result.panes[window.Focus]; ok {
			result.focusPane = pane
		}
	}
	return result, nil
}

func resolveTemplateFocus(focus string, windows map[string]windowTarget) (string, string, error) {
	windowName, panePath, ok := strings.Cut(strings.TrimSpace(focus), ".")
	if !ok || strings.TrimSpace(windowName) == "" || strings.TrimSpace(panePath) == "" {
		return "", "", fmt.Errorf("focus %q must use window.pane syntax", focus)
	}
	window, ok := windows[windowName]
	if !ok {
		return "", "", fmt.Errorf("focus %q references unknown window %q", focus, windowName)
	}
	pane, ok := window.panes[panePath]
	if !ok {
		return "", "", fmt.Errorf("focus %q references unknown pane %q", focus, panePath)
	}
	return window.id, pane, nil
}

func (c Client) selectPane(ctx context.Context, focusPane string) error {
	out, err := c.runner.Run(ctx, "tmux", "select-pane", "-t", focusPane)
	if err != nil {
		return commandError("select tmux pane", out, err)
	}
	return nil
}

func (c Client) applyLayoutNode(ctx context.Context, basePath string, paneID string, node sessionconfig.Node, prefix string) (layoutResult, error) {
	if node.Type == "pane" {
		panes := map[string]string{}
		nodes := map[string]string{}
		if node.Name != "" {
			path := nodePath(prefix, node.Name)
			panes[path] = paneID
			nodes[path] = paneID
		}
		startups := []paneStartup{{paneID: paneID, path: resolvePanePath(basePath, node.Path), node: node}}
		return layoutResult{focusPane: paneID, panes: panes, nodes: nodes, startups: startups}, nil
	}
	if len(node.Children) == 0 {
		return layoutResult{}, fmt.Errorf("layout node %q has no children", node.Name)
	}
	childPane := paneID
	focusPane := paneID
	panes := map[string]string{}
	nodes := map[string]string{}
	var startups []paneStartup
	childPrefix := prefix
	if node.Name != "" {
		childPrefix = nodePath(prefix, node.Name)
		nodes[childPrefix] = paneID
	}
	for i := range node.Children {
		if i < len(node.Children)-1 {
			args := []string{"split-window", "-d", "-P", "-F", "#{pane_id}", splitFlag(node.Type), "-t", childPane}
			if path := nodeCreatePath(basePath, node.Children[i+1]); strings.TrimSpace(path) != "" {
				args = append(args, "-c", path)
			}
			out, err := c.runner.Run(ctx, "tmux", args...)
			if err != nil {
				return layoutResult{}, commandError("split tmux pane", out, err)
			}
			nextPane := strings.TrimSpace(string(out))
			if nextPane == "" {
				return layoutResult{}, fmt.Errorf("split tmux pane: missing pane id")
			}
			result, err := c.applyLayoutNode(ctx, basePath, childPane, node.Children[i], childPrefix)
			if err != nil {
				return layoutResult{}, err
			}
			if i == 0 {
				focusPane = result.focusPane
			}
			mergePaneMaps(panes, result.panes)
			mergePaneMaps(nodes, result.nodes)
			startups = append(startups, result.startups...)
			childPane = nextPane
			continue
		}
		result, err := c.applyLayoutNode(ctx, basePath, childPane, node.Children[i], childPrefix)
		if err != nil {
			return layoutResult{}, err
		}
		if i == 0 {
			focusPane = result.focusPane
		}
		mergePaneMaps(panes, result.panes)
		mergePaneMaps(nodes, result.nodes)
		startups = append(startups, result.startups...)
	}
	if node.Name != "" {
		nodes[childPrefix] = focusPane
	}
	return layoutResult{focusPane: focusPane, panes: panes, nodes: nodes, startups: startups}, nil
}

func (c Client) windowSize(ctx context.Context, paneID string) (int, int, error) {
	out, err := c.runner.Run(ctx, "tmux", "display-message", "-p", "-t", paneID, "#{window_width} #{window_height}")
	if err != nil {
		return 0, 0, commandError("read tmux window size", out, err)
	}
	width, height, err := parseWindowSize(out)
	if err != nil {
		return 0, 0, fmt.Errorf("read tmux window size: %w", err)
	}
	return width, height, nil
}

func (c Client) currentWindowSize(ctx context.Context) (int, int, bool) {
	out, err := c.runner.Run(ctx, "tmux", "display-message", "-p", "#{window_width} #{window_height}")
	if err != nil {
		return 0, 0, false
	}
	width, height, err := parseWindowSize(out)
	if err != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func parseWindowSize(out []byte) (int, int, error) {
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected output %q", strings.TrimSpace(string(out)))
	}
	width, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("window width: %w", err)
	}
	height, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("window height: %w", err)
	}
	return width, height, nil
}

func (c Client) enforceLayoutSizes(ctx context.Context, node sessionconfig.Node, result layoutResult, width int, height int, prefix string) error {
	if node.Type == "pane" {
		return nil
	}
	childPrefix := prefix
	if node.Name != "" {
		childPrefix = nodePath(prefix, node.Name)
	}
	var allocations []int
	if node.Type == "columns" {
		allocations = distributeCells(paneCellBudget(width, len(node.Children)), node.Sizes)
	} else {
		allocations = distributeCells(paneCellBudget(height, len(node.Children)), node.Sizes)
	}
	for i, child := range node.Children {
		childPath := nodePath(childPrefix, child.Name)
		target := result.nodes[childPath]
		if target == "" {
			continue
		}
		if i < len(node.Children)-1 {
			flag := "-x"
			if node.Type == "rows" {
				flag = "-y"
			}
			out, err := c.runner.Run(ctx, "tmux", "resize-pane", "-t", target, flag, strconv.Itoa(allocations[i]))
			if err != nil {
				return commandError("resize tmux pane", out, err)
			}
		}
		childWidth := width
		childHeight := height
		if node.Type == "columns" {
			childWidth = allocations[i]
		} else {
			childHeight = allocations[i]
		}
		if err := c.enforceLayoutSizes(ctx, child, result, childWidth, childHeight, childPrefix); err != nil {
			return err
		}
	}
	return nil
}

func paneCellBudget(total int, panes int) int {
	if panes > 1 {
		total -= panes - 1
	}
	if total < panes {
		return panes
	}
	return total
}

func distributeCells(total int, sizes []int) []int {
	allocations := make([]int, len(sizes))
	if len(sizes) == 0 {
		return allocations
	}
	sizeTotal := 0
	for _, size := range sizes {
		if size > 0 {
			sizeTotal += size
		}
	}
	if sizeTotal == 0 {
		return allocations
	}
	remainders := make([]int, len(sizes))
	used := 0
	for i, size := range sizes {
		cells := total * size
		allocations[i] = cells / sizeTotal
		remainders[i] = cells % sizeTotal
		if allocations[i] < 1 {
			allocations[i] = 1
		}
		used += allocations[i]
	}
	for used < total {
		index := nextAllocationIndex(remainders, allocations, sizes, used, total)
		allocations[index]++
		remainders[index] = -1
		used++
	}
	for used > total {
		index := largestAllocationIndex(allocations)
		if index < 0 {
			break
		}
		allocations[index]--
		used--
	}
	return allocations
}

func nextAllocationIndex(remainders []int, allocations []int, sizes []int, used int, total int) int {
	index := largestRemainderIndex(remainders)
	mirror := len(sizes) - 1 - index
	if mirror == index || mirror < 0 || mirror >= len(sizes) {
		return index
	}
	if sizes[index] != sizes[mirror] || remainders[index] != remainders[mirror] || allocations[index] != allocations[mirror] {
		return index
	}
	if used+2 <= total {
		return index
	}
	center := len(sizes) / 2
	if len(sizes)%2 == 1 {
		return center
	}
	if mirror > index {
		return mirror
	}
	return index
}

func largestRemainderIndex(remainders []int) int {
	index := 0
	for i := 1; i < len(remainders); i++ {
		if remainders[i] > remainders[index] {
			index = i
		}
	}
	return index
}

func largestAllocationIndex(allocations []int) int {
	index := -1
	for i, allocation := range allocations {
		if allocation <= 1 {
			continue
		}
		if index < 0 || allocation > allocations[index] {
			index = i
		}
	}
	return index
}

func (c Client) startStrictPaneCommands(ctx context.Context, startups []paneStartup) error {
	for _, startup := range startups {
		commands := paneCommands(startup.node)
		if len(commands) == 0 || startup.node.CommandMode != sessionconfig.CommandModeWrapper {
			continue
		}
		if err := c.restrictInvisiblePassthrough(ctx, startup.paneID); err != nil {
			return err
		}
		reportStatus := len(commands) > 1
		if reportStatus {
			if err := c.preparePaneCommandStatus(ctx, startup.paneID); err != nil {
				return err
			}
		}
		args := []string{"respawn-pane", "-k", "-t", startup.paneID}
		if strings.TrimSpace(startup.path) != "" {
			args = append(args, "-c", startup.path)
		}
		args = append(args, "/bin/sh", "-lc", paneCommandWrapper(commands, startup.paneID, reportStatus))
		out, err := c.runner.Run(ctx, "tmux", args...)
		if err != nil {
			return commandError("start tmux pane command", out, err)
		}
		if reportStatus {
			status, err := c.waitForPaneCommandStatus(ctx, startup.paneID)
			if err != nil {
				return err
			}
			if status != "ok" {
				command, exitStatus, err := parsePaneCommandFailure(status, commands)
				if err != nil {
					return err
				}
				return fmt.Errorf("run tmux pane command %q: exit status %d", command, exitStatus)
			}
			if err := c.clearPaneCommandStatus(ctx, startup.paneID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c Client) queueInteractivePaneCommands(ctx context.Context, startups []paneStartup) error {
	for _, startup := range startups {
		commands := paneCommands(startup.node)
		if len(commands) == 0 || startup.node.CommandMode == sessionconfig.CommandModeWrapper {
			continue
		}
		if err := c.restrictInvisiblePassthrough(ctx, startup.paneID); err != nil {
			return err
		}
		out, err := c.runner.Run(ctx, "tmux", "run-shell", "-b", interactivePaneCommandQueue(startup.paneID, commands))
		if err != nil {
			return commandError("queue tmux pane commands", out, err)
		}
	}
	return nil
}

func interactivePaneCommandQueue(paneID string, commands []string) string {
	lines := []string{
		"pane=" + shellQuote(paneID),
		`pane_pid=$(tmux display-message -p -t "$pane" '##{pane_pid}') || exit 0`,
		`shell_pgid=$(ps -o pgid= -p "$pane_pid" | tr -d ' ') || exit 0`,
		`[ -n "$shell_pgid" ] || exit 0`,
		"wait_for_shell() {",
		"  stable=0",
		"  while [ \"$stable\" -lt 3 ]; do",
		`    foreground=$(ps -o tpgid= -p "$pane_pid" | tr -d ' ') || return 1`,
		`    if [ "$foreground" = "$shell_pgid" ]; then stable=$((stable + 1)); else stable=0; fi`,
		"    sleep 0.01",
		"  done",
		"}",
		"wait_for_command() {",
		"  stable=0",
		"  saw_command=0",
		"  while :; do",
		`    foreground=$(ps -o tpgid= -p "$pane_pid" | tr -d ' ') || return 1`,
		`    if [ "$foreground" != "$shell_pgid" ]; then`,
		"      saw_command=1",
		"      stable=0",
		"    else",
		"      stable=$((stable + 1))",
		`      if [ "$saw_command" -eq 1 ] && [ "$stable" -ge 3 ]; then return 0; fi`,
		`      if [ "$saw_command" -eq 0 ] && [ "$stable" -ge 100 ]; then return 0; fi`,
		"    fi",
		"    sleep 0.01",
		"  done",
		"}",
		"wait_for_shell || exit 0",
	}
	for index, command := range commands {
		lines = append(lines,
			"tmux send-keys -t \"$pane\" -l "+shellQuote(command)+" || exit 0",
			`tmux send-keys -t "$pane" C-m || exit 0`,
		)
		if index < len(commands)-1 {
			lines = append(lines, "wait_for_command || exit 0")
		}
	}
	return strings.Join(lines, "\n")
}

const paneCommandStatusOption = "@tmux-parator.command-status"

func (c Client) preparePaneCommandStatus(ctx context.Context, paneID string) error {
	out, err := c.runner.Run(ctx, "tmux", "set-option", "-p", "-t", paneID, paneCommandStatusOption, "pending")
	if err != nil {
		return commandError("prepare tmux pane command status", out, err)
	}
	return nil
}

func (c Client) clearPaneCommandStatus(ctx context.Context, paneID string) error {
	out, err := c.runner.Run(ctx, "tmux", "set-option", "-p", "-u", "-t", paneID, paneCommandStatusOption)
	if err != nil {
		return commandError("clear tmux pane command status", out, err)
	}
	return nil
}

func (c Client) restrictInvisiblePassthrough(ctx context.Context, paneID string) error {
	value := "#{?#{==:#{allow-passthrough},all},on,#{allow-passthrough}}"
	out, err := c.runner.Run(ctx, "tmux", "set-option", "-p", "-F", "-t", paneID, "allow-passthrough", value)
	if err != nil {
		return commandError("restrict tmux pane passthrough", out, err)
	}
	return nil
}

func (c Client) waitForPaneCommandStatus(ctx context.Context, paneID string) (string, error) {
	format := "#{pane_dead}\t#{@" + strings.TrimPrefix(paneCommandStatusOption, "@") + "}"
	for {
		out, err := c.runner.Run(ctx, "tmux", "display-message", "-p", "-t", paneID, format)
		if err != nil {
			return "", commandError("read tmux pane command status", out, err)
		}
		dead, status, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
		if !ok {
			return "", fmt.Errorf("read tmux pane command status: unexpected output %q", strings.TrimSpace(string(out)))
		}
		if status != "" && status != "pending" {
			return status, nil
		}
		if dead == "1" {
			return "", fmt.Errorf("read tmux pane command status: pane %s exited during startup", paneID)
		}
		if err := c.waitPanePoll(ctx); err != nil {
			return "", err
		}
	}
}

func (c Client) waitPanePoll(ctx context.Context) error {
	if c.panePollInterval <= 0 {
		return nil
	}
	timer := time.NewTimer(c.panePollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func paneCommands(node sessionconfig.Node) []string {
	if len(node.Commands) > 0 {
		commands := make([]string, 0, len(node.Commands))
		for _, command := range node.Commands {
			command = strings.TrimSpace(command)
			if command != "" {
				commands = append(commands, command)
			}
		}
		return commands
	}
	command := strings.TrimSpace(node.Command)
	if command == "" {
		return nil
	}
	return []string{command}
}

func paneCommandWrapper(commands []string, paneID string, reportStatus bool) string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}
	lines := make([]string, 0, len(commands)*3+2)
	if !reportStatus {
		lines = append(lines, "set -e")
		lines = append(lines, commands...)
		lines = append(lines, "exec "+shellQuote(shell)+" -l")
		return strings.Join(lines, "\n")
	}
	for index, command := range commands[:len(commands)-1] {
		lines = append(lines, "eval "+shellQuote(command))
		lines = append(lines, "__tmux_parator_status=$?")
		lines = append(lines,
			`if [ "$__tmux_parator_status" -ne 0 ]; then tmux set-option -p -t `+
				shellQuote(paneID)+" "+paneCommandStatusOption+fmt.Sprintf(` "error:%d:$__tmux_parator_status"`, index)+
				`; exit "$__tmux_parator_status"; fi`,
		)
	}
	lines = append(lines, "tmux set-option -p -t "+shellQuote(paneID)+" "+paneCommandStatusOption+" ok")
	lines = append(lines, commands[len(commands)-1])
	lines = append(lines, "exec "+shellQuote(shell)+" -l")
	return strings.Join(lines, "\n")
}

func parsePaneCommandFailure(status string, commands []string) (string, int, error) {
	fields := strings.Split(status, ":")
	if len(fields) != 3 || fields[0] != "error" {
		return "", 0, fmt.Errorf("read tmux pane command status: unexpected status %q", status)
	}
	index, err := strconv.Atoi(fields[1])
	if err != nil || index < 0 || index >= len(commands)-1 {
		return "", 0, fmt.Errorf("read tmux pane command status: unexpected command index %q", fields[1])
	}
	exitStatus, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", 0, fmt.Errorf("read tmux pane command status: unexpected exit status %q", fields[2])
	}
	return commands[index], exitStatus, nil
}

func nodeCreatePath(basePath string, node sessionconfig.Node) string {
	if node.Type == "pane" {
		return resolvePanePath(basePath, node.Path)
	}
	if len(node.Children) == 0 {
		return strings.TrimSpace(basePath)
	}
	return nodeCreatePath(basePath, node.Children[0])
}

func splitFlag(layoutType string) string {
	if layoutType == "rows" {
		return "-v"
	}
	return "-h"
}

func resolvePanePath(basePath string, panePath string) string {
	panePath = strings.TrimSpace(panePath)
	if panePath == "" || panePath == "." {
		return strings.TrimSpace(basePath)
	}
	if filepath.IsAbs(panePath) || strings.TrimSpace(basePath) == "" {
		return panePath
	}
	return filepath.Join(basePath, panePath)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func nodePath(prefix string, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func mergePaneMaps(dst map[string]string, src map[string]string) {
	for key, value := range src {
		dst[key] = value
	}
}

func (c Client) TagSession(ctx context.Context, name string, metadata SessionMetadata) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	return c.setSessionMetadata(ctx, name, metadata)
}

func (c Client) setSessionMetadata(ctx context.Context, name string, metadata SessionMetadata) error {
	type optionValue struct {
		option string
		value  string
	}
	values := []optionValue{
		{option: "@tmux-parator.created", value: "1"},
	}
	if strings.TrimSpace(metadata.Kind) != "" {
		values = append(values, optionValue{option: "@tmux-parator.kind", value: strings.TrimSpace(metadata.Kind)})
	}
	if strings.TrimSpace(metadata.Path) != "" {
		values = append(values, optionValue{option: "@tmux-parator.path", value: strings.TrimSpace(metadata.Path)})
	}
	if strings.TrimSpace(metadata.Root) != "" {
		values = append(values, optionValue{option: "@tmux-parator.root", value: strings.TrimSpace(metadata.Root)})
	}
	if strings.TrimSpace(metadata.BaseName) != "" {
		values = append(values, optionValue{option: "@tmux-parator.base_name", value: strings.TrimSpace(metadata.BaseName)})
	}
	if strings.TrimSpace(metadata.Glyph) != "" {
		values = append(values, optionValue{option: "@tmux-parator.glyph", value: strings.TrimSpace(metadata.Glyph)})
	}
	if strings.TrimSpace(metadata.GlyphColor) != "" {
		values = append(values, optionValue{option: "@tmux-parator.glyph_color", value: strings.TrimSpace(metadata.GlyphColor)})
	}
	for _, item := range values {
		out, err := c.runner.Run(ctx, "tmux", "set-option", "-t", exactSessionTarget(name), item.option, item.value)
		if err != nil {
			return commandError("tag tmux session", out, err)
		}
	}
	return nil
}

const sessionFieldSeparator = "\x1f"

func ParseSessions(out []byte) []Session {
	lines := bytes.Split(out, []byte{'\n'})
	sessions := make([]Session, 0, len(lines))
	for _, line := range lines {
		raw := strings.TrimRight(string(line), "\r")
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		parts := strings.Split(raw, sessionFieldSeparator)
		if len(parts) == 1 {
			parts = strings.Split(raw, "\t")
		}
		session := Session{Name: strings.TrimSpace(parts[0])}
		if len(parts) > 1 {
			session.Windows = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			session.Attached = strings.TrimSpace(parts[2]) != "0"
		}
		if len(parts) > 3 {
			session.Current = strings.TrimSpace(parts[3]) == "1"
		}
		if len(parts) > 4 {
			session.CreatedTime = strings.TrimSpace(parts[4])
		}
		if len(parts) > 5 {
			session.Activity, _ = strconv.ParseInt(strings.TrimSpace(parts[5]), 10, 64)
		}
		if len(parts) > 6 {
			session.Metadata.CreatedByParator = strings.TrimSpace(parts[6]) != ""
		}
		if len(parts) > 7 {
			session.Metadata.Kind = strings.TrimSpace(parts[7])
		}
		if len(parts) > 8 {
			session.Metadata.Path = strings.TrimSpace(parts[8])
		}
		if len(parts) > 9 {
			session.Metadata.Root = strings.TrimSpace(parts[9])
		}
		if len(parts) > 10 {
			session.Metadata.BaseName = strings.TrimSpace(parts[10])
		}
		if len(parts) > 11 {
			session.Metadata.Glyph = strings.TrimSpace(parts[11])
		}
		if len(parts) > 12 {
			session.Metadata.GlyphColor = strings.TrimSpace(parts[12])
		}
		if len(parts) > 13 {
			session.CurrentPath = strings.TrimSpace(parts[13])
		}
		sessions = append(sessions, session)
	}
	return sessions
}

func validateSessionName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("session name is empty")
	}
	return nil
}

func exactSessionTarget(name string) string {
	return "=" + name + ":"
}

func commandError(action string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, msg)
}

func IsDuplicateSessionError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate session")
}
