package tmux

import (
	"context"
	"fmt"
	"strings"

	"github.com/sschmerda/tmux-parator/internal/theme"
)

func OpenPopup(ctx context.Context, runner Runner, command string, width string, height string, activeTheme theme.Theme) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("popup command is empty")
	}
	args := []string{"display-popup", "-E", "-B"}
	if style := PopupStyle(activeTheme); style != "" {
		args = append(args, "-s", style)
	}
	if strings.TrimSpace(width) == "" {
		width = "90%"
	}
	if strings.TrimSpace(height) == "" {
		height = "90%"
	}
	args = append(args, "-w", width, "-h", height, command)
	out, err := runner.Run(ctx, "tmux", args...)
	if err != nil {
		return commandError("open tmux popup", out, err)
	}
	return nil
}

func PopupStyle(activeTheme theme.Theme) string {
	return style(activeTheme.Query, activeTheme.Background)
}

func style(fg string, bg string) string {
	if fg == "" && bg == "" {
		return ""
	}
	if fg == "" {
		return "bg=" + bg
	}
	if bg == "" {
		return "fg=" + fg
	}
	return "fg=" + fg + ",bg=" + bg
}
