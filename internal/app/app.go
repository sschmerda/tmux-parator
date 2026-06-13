package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sschmerda/tmux-parator/internal/config"
	"github.com/sschmerda/tmux-parator/internal/discovery"
	"github.com/sschmerda/tmux-parator/internal/theme"
	"github.com/sschmerda/tmux-parator/internal/tmux"
	"github.com/sschmerda/tmux-parator/internal/ui"
)

func Run(args []string) error {
	ctx := context.Background()
	runner := tmux.ExecRunner{}
	cfg, configPath, err := config.Load()
	if err != nil {
		return err
	}
	themeName := cfg.UI.Theme
	if envTheme := os.Getenv("TMUX_PARATOR_THEME"); envTheme != "" {
		themeName = envTheme
	}
	activeTheme := theme.Resolve(themeName)

	if len(args) > 0 {
		switch args[0] {
		case "popup", "--popup":
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable path: %w", err)
			}
			popupCommand := executable
			if configPath != "" {
				absoluteConfigPath, err := filepath.Abs(configPath)
				if err != nil {
					return fmt.Errorf("resolve config path: %w", err)
				}
				popupCommand = "TMUX_PARATOR_CONFIG=" + shellQuote(absoluteConfigPath) + " " + shellQuote(executable)
			}
			if envTheme := os.Getenv("TMUX_PARATOR_THEME"); envTheme != "" {
				popupCommand = "TMUX_PARATOR_THEME=" + shellQuote(envTheme) + " " + popupCommand
			}
			return tmux.OpenPopup(ctx, runner, popupCommand, cfg.UI.PopupWidth, cfg.UI.PopupHeight, activeTheme)
		case "-h", "--help", "help":
			fmt.Fprintln(os.Stdout, `Usage: tmux-parator [popup|--popup|version]

Controls:
  type      filter sessions and roots
  enter     open selected item
  ctrl-g    command overlay
  ctrl-n    rename selected session
  ctrl-s    create named session
  ctrl-t    path search
  ctrl-r    reload
  ctrl-k    kill selected session
  ctrl-_    help
  esc       quit or cancel

Bindings can be changed under [ui.keys] in config.toml.`)
			return nil
		default:
			return fmt.Errorf("unknown command %q", args[0])
		}
	}

	model := ui.NewModelWithKeys(tmux.NewClient(runner), activeTheme, cfg.Roots, discovery.OptionsFromConfig(cfg.Discovery), cfg.PathSearch, cfg.UI.Glyphs, cfg.UI.GlyphColors, cfg.UI.Columns, cfg.UI.Keys, cfg.UI.Dialogs)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
