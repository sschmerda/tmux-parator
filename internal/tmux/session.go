package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type Session struct {
	Name        string
	Windows     string
	Attached    bool
	CreatedTime string
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
	runner Runner
}

func NewClient(runner Runner) Client {
	return Client{runner: runner}
}

func (c Client) ListSessions(ctx context.Context) ([]Session, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{session_windows}",
		"#{session_attached}",
		"#{session_created_string}",
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
			session.CreatedTime = strings.TrimSpace(parts[3])
		}
		if len(parts) > 4 {
			session.Metadata.CreatedByParator = strings.TrimSpace(parts[4]) != ""
		}
		if len(parts) > 5 {
			session.Metadata.Kind = strings.TrimSpace(parts[5])
		}
		if len(parts) > 6 {
			session.Metadata.Path = strings.TrimSpace(parts[6])
		}
		if len(parts) > 7 {
			session.Metadata.Root = strings.TrimSpace(parts[7])
		}
		if len(parts) > 8 {
			session.Metadata.BaseName = strings.TrimSpace(parts[8])
		}
		if len(parts) > 9 {
			session.Metadata.Glyph = strings.TrimSpace(parts[9])
		}
		if len(parts) > 10 {
			session.Metadata.GlyphColor = strings.TrimSpace(parts[10])
		}
		if len(parts) > 11 {
			session.CurrentPath = strings.TrimSpace(parts[11])
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
