# tmux-parator

`tmux-parator` is a standalone Bubble Tea TUI for preparing reproducible tmux
workspaces. The current version focuses on current tmux sessions and configured
workspace roots: list, filter, switch, create, and kill sessions with
confirmation.

It is part of a Latin-themed tmux tool suite:

- `tmux-dux`: command palette / launcher
- `tmux-parator`: workspace preparer
- `tmux-custos`: possible future coding-agent monitor

## Usage

Start the TUI:

```sh
tmux-parator
```

Open in a tmux popup:

```sh
tmux-parator popup
```

or:

```sh
tmux-parator --popup
```

## Requirements

- Go toolchain for development
- `tmux` at runtime

## Controls

- Type to filter tmux sessions and configured roots.
- `Enter` opens the selected item.
- `Tab` jumps to the next main-list section; `Shift-Tab` jumps to the previous
  section.
- `Ctrl-g` opens the fuzzy command palette.
- `Ctrl-n` renames the selected open tmux session.
- `Ctrl-s` creates a named tmux session from the selected row's path and kind.
- `Ctrl-t` starts the create-session-from-path flow.
- `Ctrl-r` reloads sessions and configured roots.
- `Ctrl-k` asks to kill the selected session.
- `Meta-h` toggles hidden-directory skipping for configured roots.
- `Meta-i` toggles gitignored-directory skipping for configured roots.
- `Ctrl-?` shows help.
- `Esc` quits or cancels the current overlay.

Confirmation popups:

- `Left`/`Right`, `Up`/`Down`, or `Tab` select between cancel and confirm.
- `Enter` chooses the selected action.
- `y` confirms immediately; `n` or `Esc` cancels immediately.

## Hotkeys

Default bindings can be changed under `[ui.keys]` in `config.toml`. Command,
template, and help popups reuse navigation and prompt-editing bindings from
`[ui.keys.browse]`; their own key sections contain only popup-specific actions.

Main list:

| Key | Action |
| --- | --- |
| `Up` / `Down` | Move selection one row. |
| `Ctrl-y` / `Ctrl-e` | Scroll the result viewport up/down one row, keeping the selection visible. |
| `PgUp` / `Ctrl-b` | Move selection up by half a page. |
| `PgDown` / `Ctrl-d` | Move selection down by half a page. |
| `Tab` / `Shift-Tab` | Jump to the next/previous section. |
| `Enter` | Open selected item. |
| `Ctrl-g` | Open command palette. |
| `Ctrl-n` | Rename selected open tmux session. |
| `Ctrl-s` | Create a named tmux session from the selected row. |
| `Ctrl-t` | Open path search. |
| `Ctrl-r` | Reload sessions and configured roots. |
| `Ctrl-k` | Confirm killing the selected session. |
| `Meta-h` / `Meta-i` | Toggle hidden/gitignored directory skipping. |
| `Ctrl-?` | Show help. |
| `Esc` | Quit or cancel the current overlay. |

Prompt editing:

| Key | Action |
| --- | --- |
| `Backspace` / `Ctrl-h` | Delete one character. |
| `Alt-Backspace` | Delete one word. |
| `Ctrl-u` / `Meta-Backspace` | Clear the prompt. |

Path search:

| Key | Action |
| --- | --- |
| `Up` / `Down` | Move selection one row. |
| `Ctrl-y` / `Ctrl-e` | Scroll results up/down one row, keeping the selection visible. |
| `PgUp` / `Ctrl-b` | Move selection up by half a page. |
| `PgDown` / `Ctrl-d` | Move selection down by half a page. |
| `Tab` / `Shift-Tab` | Complete or narrow the current path segment forward/backward. |
| `Left` / `Right` | Accept the current completion cycle. |
| `Enter` | Open the selected path result. |
| `Ctrl-l` | Create the selected path result with a chosen session template, or with no template. |
| `Ctrl-p` | Open the exact typed path when it exists. |
| `Ctrl-a` | Create the exact typed path after confirmation. |
| `Ctrl-o` | Cycle the prompt root. |
| `Ctrl-t` / `Esc` | Close path search. |

Command palette, template picker, and help:

| Key | Action |
| --- | --- |
| `Up` / `Down` | Move selection one row. |
| `Ctrl-y` / `Ctrl-e` | Scroll the popup viewport up/down one row. |
| `PgUp` / `Ctrl-b` | Move selection up by half a page. |
| `PgDown` / `Ctrl-d` | Move selection down by half a page. |
| Prompt editing keys | Edit the fuzzy-search query. |
| `Enter` | Run the selected command or create the selected template. |
| `Esc` / `Ctrl-g` | Close the command palette. |
| `Ctrl-?` | Toggle help. |

Filtering uses the same weighted, multi-token fuzzy matching style as
`tmux-dux`. Sessions are shown above configured roots, separated by a divider
in the unfiltered list. Session rows include an origin chip: ` repo`,
` subdir`, `󰉋 path`, `󰙅 worktree` for parator-managed workspaces, and ` manual`
for untagged tmux sessions.
For sessions created by `tmux-parator`, the origin is stored on the tmux
session with `@tmux-parator.*` user options. That metadata stays with the live
session if it is renamed and is removed automatically when the session is
killed. Older or untagged sessions are shown as manual sessions even if
their names match configured candidates.

In the command palette:

- Type to fuzzy-search commands.
- Navigation and prompt editing use `[ui.keys.browse]`.
- `Enter` runs the selected command.
- `Esc` or `Ctrl-g` closes the palette.
- `Ctrl-?` opens help.
- `Open last session` switches to tmux's last active session.
- Toggle commands and `Quit` are available from the palette.

In help:

- Type to fuzzy-search actions, displayed keys, and descriptions.
- Navigation and prompt editing use `[ui.keys.browse]`.
- `Esc` or `Ctrl-?` closes help.

Direct session toggle:

- `Ctrl-\`` switches to tmux's last active session using `tmux switch-client -l`.
- This behaves like tmux's native last-session toggle, so pressing it repeatedly moves between the two most recent sessions.

## Fuzzy Matching

Keep this section updated whenever search behavior changes.

All fuzzy inputs use multi-token matching:

- Input is split on whitespace.
- Every token must match somewhere in the candidate.
- Matching is subsequence-based, so the letters do not need to be contiguous.
  For example, `tpr` can match `tmux-parator`.
- Matching is case-insensitive.
- The shared fuzzy scorer prefers higher-weight fields and better character
  placement, such as matches near the start of a field, matches after
  separators, and adjacent characters.

Shared field weights:

- title/name: highest weight
- aliases: high weight
- initials: medium-high weight
- category: medium weight
- extra fields such as root labels and paths: lower weight unless overridden by
  the caller

Main search:

- The main prompt filters open tmux sessions and configured root candidates.
- Results are displayed in sections:
  - `open sessions`
  - `available workspaces`
- `Tab` and `Shift-Tab` jump between visible section starts, both with and
  without a query.
- The origin chip label is searchable: `repo`, `subdir`, `path`, `worktree`, `manual`.
- The root column is searchable:
  - configured candidates use the root `name`, for example `repos`
  - tagged sessions use `@tmux-parator.root`
- The name column is searchable:
  - sessions use the tmux session name
  - candidates use the discovered directory or repo name
- Configured candidates also match their compact display path, for example
  `repos/tmux-parator`.
- Main search does not match full absolute paths. Use path search for that.
- Main search does not match glyphs, tmux attached/detached state, or window
  counts.

Main search ranking:

- Open tmux sessions always sort before configured root/path candidates when
  both match.
- Each visible section is sorted independently by fuzzy score.
- Equal-score results fall back to candidate name sorting.

Path search:

- `Ctrl-t` opens filesystem path search.
- The prompt is parsed as a path-like string. In `~/stefan/repos`, `~/stefan`
  is the traversal root and `repos` is the fuzzy query.
- If there is text after the last slash, that text is the query. In
  `~/data csedm`, the traversal root is `~` and the query is `data csedm`.
- Path search matches directory names and discovered paths relative to the
  active traversal root.
- Path search is the place to search absolute path components such as
  `stefan/code/repos` by first choosing a broad enough root, for example `/`.
- The root prefix itself is not searched. With root `~`, the
  `/Users/<name>` prefix is ignored so user names do not create noisy matches.
- Path-column highlights merge basename/title matches and full-path matches, so
  a query like `~/data csedm` can highlight both `data` in the path and `csedm`
  in the directory name.
- `Tab` completes or narrows the current path segment from direct children or
  the selected fuzzy result.
- `Ctrl-l` opens the template picker for the selected path result. The picker
  also includes a no-template option.

Path search ranking:

- Literal token coverage across the basename and root-relative path is ranked
  before fuzzy score. This is important for multi-token queries such as
  `~/data csedm`.
- Each query token gets its best literal rank from the candidate basename and
  every component of the root-relative path:
  1. component exactly equals the token
  2. component starts with the token
  3. token appears after a separator such as `_`, `-`, `.`, or space
  4. token is contained somewhere in the component
  5. token is an ordered subsequence from the start of a component, so small
     omissions such as `csdm` still rank well against `CSEDM_...`
  6. token matches only as a generic fuzzy subsequence
- The token ranks are added, so a result where `data` and `csedm` both appear as
  real path/name components is preferred over a result where only `data` is a
  literal component and `csedm` is just a fuzzy subsequence.
- After literal token coverage, basename/title matches sort before
  parent-path-only fuzzy matches. This means a query like `csedm` should prefer
  a directory ending in `/CSEDM_...` over a deeper directory that only has
  `csedm` in a parent path.
- After those path-specific buckets, results sort by fuzzy score.
- Equal-score path results prefer shallower directories relative to the current
  traversal root.
- Remaining ties sort by basename and then by full path.

Command palette:

- `Ctrl-g` opens the fuzzy command palette.
- Commands match their title, key binding, command id, description, and disabled
  reason.

Command palette ranking:

- Commands use the shared fuzzy scorer.
- Command title has the highest weight.
- Key binding, command id, description, and disabled reason are lower-weight
  searchable fields.

Help uses the same popup layout and fuzzy matcher. Action names have the
highest weight, followed by displayed keys and descriptions.

## Themes

`tmux-parator` uses the same built-in color schemes as `tmux-dux`. The default
is `shades-of-purple`.

Available themes:

- `catppuccin`
- `tokyonight`
- `rosepine`
- `kanagawa`
- `shades-of-purple`
- `solarized`
- `gruvbox`

Select one with:

```sh
TMUX_PARATOR_THEME=catppuccin tmux-parator --popup
```

## Configuration

Configuration is TOML and is loaded from:

```text
./.dev/config.toml
```

when present in the current working directory, otherwise from:

```text
$XDG_CONFIG_HOME/tmux-parator/config.toml
```

or, when `XDG_CONFIG_HOME` is unset:

```text
~/.config/tmux-parator/config.toml
```

`TMUX_PARATOR_CONFIG` overrides all of these paths.

Built-in defaults are defined in:

```text
internal/config/default.toml
```

That file is embedded into release binaries. During development, changing it
changes the app defaults after rebuilding.

For one-off local testing, point the app at any config file:

```sh
TMUX_PARATOR_CONFIG=examples/tmux-parator/config.toml ./bin/tmux-parator
```

The configuration is organized in two layers:

- global sections such as `[ui]`, `[discovery]`, and `[path_search]`
- per-root `[[roots]]` entries, which follow the global sections and inherit
  the global discovery defaults unless a root overrides them

Example:

```toml
[ui]
theme = "shades-of-purple"
popup_width = "90%"
popup_height = "90%"

[ui.dialogs.small]
width = 72
height = 9

[ui.dialogs.panel]
width = 88
height = 0

[ui.glyphs]
repo = ""
subdir = ""
path = "󰉋"
worktree = "󰙅"
manual = ""

[ui.glyph_colors]
repo = "#f14e32"
subdir = "#7aa2f7"
path = "#7dcfff"
worktree = "#9ece6a"
manual = "#a599e9"

[ui.columns.chip]
show = true
width = 12
max_width = 12

[ui.columns.root]
show = true
width = 12
max_width = 20

[ui.columns.name]
show = true
width = 28
max_width = 40

[ui.columns.path]
show = true
width = 0
max_width = 0
include_root = true

[discovery]
backend = "auto"
skip_hidden = true
skip_gitignored = true
skip_dirs = ["node_modules", "vendor", "dist", "build"]

[path_search]
enabled = true
backend = "auto"
roots = ["~"]
max_depth = 12
skip_hidden = true
skip_gitignored = true
skip_dirs = ["node_modules", ".git", "vendor", "dist", "build", ".cache", ".venv", "__pycache__"]
limit = 5000

[[roots]]
name = "projects"
path = "~/projects"
kind = "repo"
glyph = ""
glyph_color = "#f14e32"
depth = 1
max_depth = 4
skip_hidden = true
skip_gitignored = true
skip_dirs = ["node_modules", "vendor", "dist", "build"]

[[roots]]
name = "scratch"
path = "~/scratch"
kind = "subdir"
glyph = ""
glyph_color = "#7aa2f7"
depth = 2
max_depth = 0
skip_hidden = true
skip_gitignored = true
skip_dirs = ["node_modules", "vendor", "dist", "build"]
```

Config fields:

| Field | Scope | Description |
| --- | --- | --- |
| `ui.theme` | global | Built-in theme name. Defaults to `shades-of-purple`. |
| `ui.popup_width` | global | Width passed to `tmux display-popup -w`. Defaults to `90%`. |
| `ui.popup_height` | global | Height passed to `tmux display-popup -h`. Defaults to `90%`. |
| `ui.dialogs.small.width` | global | Target width for confirmation, name, notice, and error frames in terminal cells. Defaults to `72`. |
| `ui.dialogs.small.height` | global | Preferred height for small frames. `0` uses content-responsive height. Defaults to `9`. |
| `ui.dialogs.panel.width` | global | Target width for command palette and help frames in terminal cells. Defaults to `88`. |
| `ui.dialogs.panel.height` | global | Preferred height for command palette and help frames. `0` uses viewport-responsive auto height. Defaults to `0`. |
| `ui.glyphs.repo` | global | Glyph used for repo chips. Defaults to ``. |
| `ui.glyphs.subdir` | global | Glyph used for subdir chips. Defaults to ``. |
| `ui.glyphs.path` | global | Glyph used for ad-hoc path session chips. Defaults to `󰉋`. |
| `ui.glyphs.worktree` | global | Glyph used for worktree chips. Defaults to `󰙅`. |
| `ui.glyphs.manual` | global | Glyph used for untagged tmux session chips. Defaults to ``. |
| `ui.glyph_colors.repo` | global | Glyph foreground color used for repo chips. Defaults to `#f14e32`. |
| `ui.glyph_colors.subdir` | global | Glyph foreground color used for subdir chips. Defaults to `#7aa2f7`. |
| `ui.glyph_colors.path` | global | Glyph foreground color used for ad-hoc path session chips. Defaults to `#7dcfff`. |
| `ui.glyph_colors.worktree` | global | Glyph foreground color used for worktree chips. Defaults to `#9ece6a`. |
| `ui.glyph_colors.manual` | global | Glyph foreground color used for untagged tmux session chips. Defaults to `#a599e9`. |
| `ui.columns.chip.show` | global | Shows the origin chip column when `true`. Defaults to `true`. |
| `ui.columns.chip.width` | global | Origin chip column width in terminal cells. `0` uses the built-in chip width. Defaults to `12`. |
| `ui.columns.chip.max_width` | global | Maximum origin chip width when `width = 0`. Defaults to `12`. |
| `ui.columns.root.show` | global | Shows the root-name column when `true`. Defaults to `true`. |
| `ui.columns.root.width` | global | Root-name column width in terminal cells. `0` auto-sizes to visible rows. Defaults to `12`. |
| `ui.columns.root.max_width` | global | Maximum root-name width when `width = 0`. Defaults to `20`. |
| `ui.columns.name.show` | global | Shows the result-name column when `true`. Defaults to `true`. |
| `ui.columns.name.width` | global | Result-name column width in terminal cells. `0` auto-sizes to visible rows. Defaults to `28`. |
| `ui.columns.name.max_width` | global | Maximum result-name width when `width = 0`. Defaults to `40`. |
| `ui.columns.path.show` | global | Shows the compact path/detail column when `true`. Defaults to `true`. |
| `ui.columns.path.width` | global | Compact path/detail column width in terminal cells. `0` means use remaining row width. Defaults to `0`. |
| `ui.columns.path.max_width` | global | Maximum compact path/detail width when `width = 0`. `0` means uncapped. Defaults to `0`. |
| `ui.columns.path.include_root` | global | Includes the configured root prefix in compact root paths. Defaults to `true`. The main list always uses root-prefixed compact paths for configured workspaces so open and available workspace rows use the same path scheme. |
| `discovery.backend` | global | Discovery backend for configured `[[roots]]` entries: `auto`, `fd`, or `go`. `auto` prefers `fd` when it is available and falls back to Go. |
| `discovery.skip_hidden` | global | Whether hidden directories are skipped by default. Defaults to `true`. |
| `discovery.skip_gitignored` | global | Whether gitignored directories are skipped by default. Defaults to `true`. |
| `discovery.skip_dirs` | global | Directory basenames skipped by default during traversal. |
| `path_search.enabled` | global | Enables `Ctrl-t` filesystem path search. Defaults to `true`. |
| `path_search.backend` | global | Path search backend: `auto`, `fd`, or `go`. `auto` prefers `fd` when available and falls back to Go. |
| `path_search.roots` | global | Default path-search roots. The first entry is used when opening path search. |
| `path_search.max_depth` | global | Maximum path-search traversal depth. `0` means unlimited. Defaults to `12`. |
| `path_search.skip_hidden` | global | Whether path search skips hidden directories. Defaults to `true`. |
| `path_search.skip_gitignored` | global | Whether path search skips gitignored directories. Defaults to `true`. |
| `path_search.skip_dirs` | global | Directory basenames skipped by path search. |
| `path_search.limit` | global | Maximum path-search results retained before fuzzy filtering. Defaults to `5000`. |
| `roots.name` | root | Required unique namespace shown in compact root paths. |
| `roots.path` | root | Required filesystem path to discover under. `~` is expanded. |
| `roots.kind` | root | Discovery kind: `subdir` or `repo`. Defaults to `subdir`. |
| `roots.glyph` | root | Optional per-root chip glyph override. Defaults to the global glyph for the root mode. |
| `roots.glyph_color` | root | Optional per-root glyph foreground color override, for example `#d6a84f`. Defaults to the built-in color for the root mode. |
| `roots.depth` | root | `subdir` traversal depth. `0` means default `1`. |
| `roots.max_depth` | root | `repo` traversal limit. `0` means unlimited. |
| `roots.skip_hidden` | root | Per-root override for `discovery.skip_hidden`. |
| `roots.skip_gitignored` | root | Per-root override for `discovery.skip_gitignored`. |
| `roots.skip_dirs` | root | Per-root replacement for `discovery.skip_dirs`. |

For `ui.columns.*.width`, positive values are fixed terminal-cell widths.
`0` means auto/flexible behavior: the chip uses its built-in width, root/name
auto-size from the currently visible rows up to `max_width`, and path/detail
uses the remaining row width. For the path/detail column, `max_width = 0` means
uncapped. The main/root search uses root-prefixed compact paths for configured
workspace rows, for example `repos/tmux-parator`, so open sessions and
available workspaces use the same path scheme.

For `ui.dialogs.*.width`, positive values are target frame widths in terminal
cells; frames shrink to fit narrow terminals. For `ui.dialogs.*.height`,
positive values are preferred heights that can grow for content, while
`small.height = 0` is content-responsive and `panel.height = 0` keeps the
viewport-responsive command/help sizing.

Root modes:

- `subdir`: list child directories. `depth = 1` lists direct children only;
  `depth = 2` lists direct children and their children. Omitted `depth`
  defaults to `1`.
- `repo`: recursively list directories containing `.git`. Omitted
  `max_depth` scans without a depth limit. `max_depth = 4` limits traversal to
  four levels below the configured root.

Discovery options:

- `backend`: discovery backend for configured roots. `auto` prefers `fd`
  when it is installed and falls back to the Go filesystem traversal backend
  when it is not.
- `skip_hidden`: skips hidden directories when `true`. Defaults to `true`.
- `skip_gitignored`: skips directories matched by `.gitignore` when `true`.
  Defaults to `true`.
- `skip_dirs`: directory basenames to skip while traversing. These match the
  last path component only, not the full path. Defaults to
  `["node_modules", "vendor", "dist", "build"]`.
- These options can be overridden per root. A root-level `skip_dirs` replaces
  the global list for that root.

In `repo` mode, `.git` is still detected as a repository marker before skip
rules are applied.

Filesystem path search:

- `Ctrl-t` opens a separate directory search mode.
- The prompt behaves like a path. In `~/stefan/repos`, `~/stefan` is the search
  root and `repos` is the fuzzy query.
- Typing `/` changes the parsed search root and starts a new streamed search
  below that root.
- `Backspace` removes prompt characters and reparses the search root/query.
- `Tab` completes or narrows the current path segment.
- `Enter` opens the selected directory as a tmux session and switches to it.
- `Ctrl-p` opens the exact typed prompt path as a session when it exists as a
  directory.
- `Ctrl-o` cycles the prompt root through `~`, `/`, `.`, and `..`.
- `Meta-h` toggles hidden-directory skipping for the current path search.
- `Meta-i` toggles gitignored-directory skipping for the current path search.
- The search runs asynchronously through Bubble Tea commands, so traversal does
  not block normal UI rendering.
- `backend = "auto"` prefers `fd` when it is installed and falls back to the Go
  filesystem traversal backend when it is not.

Root `name` values are required and must be unique. They are shown in the root
column and stored as `@tmux-parator.root`. The compact path/detail column is
based on the actual configured root path basename, not the `name` label.
Instead of displaying full absolute paths, `tmux-parator` shows root candidates
as:

```text
<basename-of-root-path>/<relative-path-from-root>
```

Examples:

```text
path = "~/code/repos"       -> repos/tmux-parator
path = "~/stefan/documents" -> documents/notes
path = "~/work/client-a"    -> client-a/api
```

Selecting a root candidate creates a detached tmux session in that path and
switches to it.

## Session Templates

Session templates live next to the main config in a `templates/` directory.
The normal config layout is:

```text
~/.config/tmux-parator/
  config.toml
  templates/
    repo.toml
    scripts/
      notes.sh
      setup.sh
```

Each `templates/*.toml` file defines one template. Template files are loaded in
filename order.

A workspace can also define one local template at
`.tmux-parator/template.toml`. Local templates are discovered when opening a
workspace. On Enter, a local template overrides matching global templates after
showing a notice first. On `Ctrl-l`, the local template appears at the top of
the template picker. Local and global templates are grouped under separate
section headings, with the active section highlighted. `Tab` and `Shift-Tab`
jump between sections; the normal configured browse navigation keys move
between template rows.

For local experiments, `./.dev/templates/*.toml` is used when that directory is
present in the current working directory. Otherwise templates are loaded from
the directory that contains `TMUX_PARATOR_CONFIG` or the resolved main config
file. If no main config path is available, they are loaded from:

```text
$XDG_CONFIG_HOME/tmux-parator/templates/*.toml
```

or, when `XDG_CONFIG_HOME` is unset:

```text
~/.config/tmux-parator/templates/*.toml
```

`TMUX_PARATOR_SESSION_CONFIG` overrides all of these paths. It can point to a
config directory or a single template file.

Local template example:

```toml
id = "local"
name = "Local Workspace"
focus = "work.main"
description = "Project-local workspace layout."
chip = "lw"
window_indicators = [" editor", " git"]

[[hooks.before_create_command]]
run = "git fetch --quiet"
kinds = ["repo"]

[[windows]]
name = "work"
focus = "main"

[windows.layout]
type = "columns"
sizes = [70, 30]
children = ["main", "side"]

[windows.layout.main]
type = "pane"
command = "nvim ."

[windows.layout.side]
type = "pane"
command = "git status --short"
```

See `examples/tmux-parator/` for a complete example directory. This is
`examples/tmux-parator/templates/repo.toml`:

```toml
id = "repo"
name = "Repository"
focus = "shell.main.editor"
description = "Three-window repository workspace with nested panes."
chip = "r"
window_indicators = [" shell", " git", "󰇥 files", "󰎚 scratch"]
enabled = true
match = ["~/code/repos/*", "~/work/*"]

[[hooks.before_create_command]]
run = "git fetch --quiet"
kinds = ["repo"]

[[hooks.after_create_script]]
run = "ready.sh"
kinds = ["repo"]

[[windows]]
name = "shell"
focus = "main.editor"

[windows.layout]
type = "columns"
sizes = [25, 50, 25]
children = ["left", "main", "right"]

[windows.layout.left]
type = "pane"
path = "."
command = ["git status --short", "git branch --show-current"]

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
path = "."
command = "go test ./..."

[windows.layout.right]
type = "pane"
path = "."
command = "git log --oneline -5"

[[windows]]
name = "git"

[windows.layout]
type = "pane"
name = "lazygit"
path = "."
command = "lazygit"

[[windows]]
name = "files"

[windows.layout]
type = "pane"
name = "yazi"
path = "."
command = "yazi"
command_mode = "wrapper"

[[windows]]
name = "scratch"

[windows.layout]
type = "pane"
name = "notes"
path = "docs"
script = ["setup.sh", "notes.sh"]
```

Session template fields:

| Field | Scope | Description |
| --- | --- | --- |
| `id` | template | Required stable identifier. It must be unique. |
| `name` | template | Required display name. It must be unique, and enabled templates are sorted by this name. |
| `focus` | template | Required final startup target in `window.pane.path` form, for example `shell.main.editor`. It must resolve to a pane. |
| `description` | template | Optional description shown in the template picker. |
| `chip` | template | Optional short searchable abbreviation rendered before the template name for quick fuzzy selection. |
| `window_indicators` | template | Optional string or string list shown after the template name to summarize the template's windows. Entries may contain glyphs, text, or both and are separated by `·`. Indicator text participates in fuzzy search. |
| `enabled` | template | Optional flag for showing the template. Defaults to `true`. |
| `match` | template | Optional path pattern or pattern list used by Enter to create matching paths with this template automatically. |
| `hooks.before_create_command` | template | Optional array of hook tables. Each `[[hooks.before_create_command]]` entry runs one shell command before the tmux session is created. |
| `hooks.before_create_script` | template | Optional array of hook tables. Each `[[hooks.before_create_script]]` entry resolves `run` under `scripts/` next to the template file, then runs that script before the tmux session is created. |
| `hooks.after_create_command` | template | Optional array of hook tables. Each `[[hooks.after_create_command]]` entry runs one shell command after the tmux session, windows, panes, metadata, and initial selection are created. |
| `hooks.after_create_script` | template | Optional array of hook tables. Each `[[hooks.after_create_script]]` entry resolves `run` under `scripts/` next to the template file, then runs that script after the tmux session is created. |
| `hooks.*.run` | hook table | Required command string or script name for that hook entry. |
| `hooks.*.kinds` | hook table | Optional session kind list. When set, the hook runs only when the created session metadata kind matches one of the listed values such as `repo`, `subdir`, `path`, or `manual`. |
| `windows.name` | window | Required tmux window name. |
| `windows.focus` | window | Optional dot path to the pane made active within that window after layout creation, for example `main.editor`. It must resolve to a pane, not a layout group. Template-level `focus` still controls the final startup window and pane. |
| `windows.layout.type` | layout node | Required node type: `pane`, `columns`, or `rows`. |
| `windows.layout.name` | root pane | Required only when the window root layout is a single `pane`. Child pane names normally come from the parent `children` list. |
| `windows.layout.path` | pane | Optional pane working directory. Relative paths are resolved under the selected workspace path. `.` or an empty value use the selected workspace path. |
| `windows.layout.command` | pane | Optional command or command list sent to the pane followed by Enter. Lists are sent as individual commands in order. |
| `windows.layout.command_mode` | pane | Optional command launch mode. `interactive` sends the command to the pane's shell with `tmux send-keys`. `wrapper` starts the pane with `/bin/sh -lc` and runs pane commands as a small shell script before `exec`-ing the final login shell. Defaults to `interactive`. |
| `windows.layout.script` | pane | Optional script name or script list resolved under `scripts/` next to the template file, then sent to the pane followed by Enter as individual commands in order. Mutually exclusive with `command`. |
| `windows.layout.sizes` | `columns`/`rows` | Required positive integer weights, one per child. Values are relative and do not need to sum to `100`. |
| `windows.layout.children` | `columns`/`rows` | Required child node names. Each child must have a matching nested table. |

Layout rules:

- `glyphs` is accepted as a deprecated alias for `window_indicators`. Do not
  configure both fields in the same template.
- `pane` is a leaf. It cannot define `sizes` or `children`.
- `columns` splits panes horizontally; `rows` splits panes vertically.
- `sizes = [25, 50, 25]` means the middle child receives twice the space of
  either side child. `[1, 1, 1]` and `[33, 33, 33]` both mean equal thirds.
- Terminal cells are integers, so exact percentages are rounded. Remainder
  cells are distributed to preserve relative weights and mirrored panes where
  possible.
- Child pane names come from the parent `children` list. In
  `children = ["editor"]`, `[windows.layout.editor]` defines a pane
  named `editor` unless it explicitly sets `name`.
- A root single-pane window has no parent `children` list, so it must set
  `name`.
- `match` supports exact paths and Go `filepath.Match` globs. Plain paths act
  as subtree matches, so `~/stefan/code` applies to all paths below that
  directory. `~` expands to the home directory; relative patterns are resolved
  from the directory containing the template file.
- When Enter creates a new session for a root or path-search result, the most
  specific enabled template whose `match` pattern matches the selected path is
  used automatically. Deeper paths win over broader parents, and exact paths
  win over globs. Existing path sessions are still switched to instead of
  recreated. `Ctrl-l` always opens the template picker for manual selection,
  including a no-template option. The picker shows chips, names, and window
  indicators, and fuzzy-searches chips, names, ids, descriptions, and window
  indicators.
- `command` and `script` are mutually exclusive. Use `command` for inline
  commands and `script` for dotfile-managed scripts such as
  `~/.config/tmux-parator/templates/scripts/setup.sh`.
- `command_mode = "interactive"` is the default and is the most robust option
  for normal shells and TUIs. It visibly types the command at the prompt before
  pressing Enter.
- `command_mode = "wrapper"` is useful for programs with terminal startup
  quirks such as `yazi`. It avoids `send-keys`, but the command runs in a
  wrapper shell before control is handed to the final interactive shell.
- Template-level hooks are configured as repeated TOML tables such as
  `[[hooks.before_create_command]]`. Each table represents one hook entry with
  a required `run` value and an optional `kinds` filter.
- Session hook entries are run by `tmux-parator` outside tmux panes. Their
  output is hidden on success and included in the error when a hook fails.
- Session hooks are non-interactive. They have no controlling terminal, and
  Git credential prompts and SSH password prompts are disabled. Configure a
  credential helper or SSH key first; otherwise commands such as `git fetch`
  fail and report an error instead of waiting for input.
- Hook entries are evaluated in file order within each hook list.
- If a hook entry has `kinds = ["repo"]`, it runs only for sessions whose
  metadata kind is `repo`. When `kinds` is omitted, the hook runs for all
  session kinds.
- If a `before_create` hook fails, the tmux session is not created.
- After `tmux new-session` succeeds, creation is transactional for the tmux
  session. If creating a window or pane, applying metadata or focus, or running
  an `after_create` hook fails, tmux-parator kills the partial session before
  returning the error. If cleanup also fails, both errors are reported.
- Relative `script` values are resolved from the directory containing the
  template file: in `templates/repo.toml`, `script = "setup.sh"` runs
  `templates/scripts/setup.sh`.
- `[[hooks.before_create_script]]` with `run = "setup.sh"` and
  `[[hooks.after_create_script]]` with `run = "ready.sh"` run those resolved
  scripts outside tmux in the selected workspace.
- `command = ["make generate", "go test ./..."]` sends `make generate` and
  then `go test ./...` as separate pane commands. `script` lists use the same
  ordering after each script path is resolved.
- Pane `command` and `script` entries do not have per-kind conditions. They are
  pane startup commands, intended to be visible in the pane when they run.
- Template `focus` starts with a window name, followed by child names joined by
  dots, and must end at a pane. For example, `shell.main.editor` targets the
  `editor` pane below the `main` group in the `shell` window.
- Window `focus` omits the window prefix. For example, `main.editor` is valid
  within that `shell` window; `main` alone is invalid when it points at a
  layout group.

## Session Names And Metadata

`tmux-parator` keeps tmux session names short and readable:

- The base session name is the selected leaf directory name.
- Unsafe tmux-name characters are converted to `_`.
- If the base name is available, it is used directly.
- If the base name already exists for a different path, the next available
  numeric suffix is used: `_2`, `_3`, and so on.
- Existing sessions are not renamed. For example, if `data` already exists, the
  next duplicate becomes `data_2`; `data` does not become `data_1`.
- If a tmux session already has `@tmux-parator.path` equal to the selected path,
  `tmux-parator` switches to that session instead of creating another duplicate.

Examples:

```text
~/code/repos/tmux-parator          -> tmux-parator
~/work/client-a/tmux-parator       -> tmux-parator_2
~/data/ddia/CSEDM_2021_F19_...     -> CSEDM_2021_F19_...
~/other/project/data               -> data
~/code/repos/qmk_firmware/data     -> data_2
```

Open parator-managed sessions show a compact version of the stored path in the
path/detail column when available. If the path matches a configured root
candidate, the root display path is used, for example `repos/tmux-parator`.
That prefix comes from the root path basename, not from `roots.name`.
Otherwise the path is shortened relative to the home directory when possible,
for example `~/data/ddia/CSEDM_2021...`. Sessions without parator path metadata
leave the path/detail column empty.

For sessions created or opened by `tmux-parator`, the app stores tmux user
options on the session:

| tmux option | Meaning |
| --- | --- |
| `@tmux-parator.created` | Set to `1` when the session was tagged by `tmux-parator`. |
| `@tmux-parator.kind` | Workspace kind shown in the chip: `repo`, `subdir`, `path`, or `worktree`. Untagged tmux sessions are shown as `manual` without stored parator metadata. |
| `@tmux-parator.path` | Absolute workspace path used to create or identify the session. |
| `@tmux-parator.root` | Configured root label for root candidates, for example `repos`; empty for ad-hoc path search sessions. |
| `@tmux-parator.base_name` | Unsuffixed sanitized leaf name used as the base for duplicate numbering. |
| `@tmux-parator.glyph` | Optional per-root glyph override stored on the session. |
| `@tmux-parator.glyph_color` | Optional per-root glyph color override stored on the session. |

These values live on the tmux session. They disappear when the tmux session is
killed.

## Roadmap

`tmux-parator` is built incrementally. The current version already includes
the original v0.1 session workflow plus configured root discovery and path
search. Future milestones should continue to keep the binary self-contained and
avoid required dependencies on fuzzy finders or preview tools.

### v0.1

- Create Go module.
- Add CLI entrypoint.
- Add minimal Bubble Tea TUI.
- List current tmux sessions.
- Fuzzy/simple-filter sessions.
- Switch to selected session.
- Kill selected session with confirmation.
- Add tests for parsing/helper logic.
- Add README.

### v0.2

- Load TOML config.
- Add configured roots.
- Add `subdirs` discovery.
- Add `repos` discovery.
- Implement async Go filesystem traversal.
- Add optional `fd` backend.

### v0.3

- Create tmux sessions from selected path/repo.
- Add native layouts with multiple windows.
- Add layout selection precedence.

### v0.4

- Add pane layouts.
- Support hbox/vbox.
- Support percentage sizes.

### v0.5

- Add worktree creation.
- Add branch prompt.
- Add `{repo}_{branch}` naming.
- Add worktree workspace creation.

### v0.6

- Add layout memory.
- Add state file.
- Add script layout backend.

### v0.7+

- Add devcontainer runtime backend.
- Add optional tmuxifier backend.
- Add more advanced nested pane layouts.
- Add worktree cleanup helpers for merged branches.

## Install

With TPM:

```tmux
set -g @plugin 'sschmerda/tmux-parator'
set -g @tmux-parator-key 'P'
```

Press `prefix` + `I` to install. The TPM plugin downloads the latest release
binary into the plugin directory. `@tmux-parator-key` is optional; no key is
bound unless you set it explicitly. The example above binds `prefix` + `P` to
`tmux-parator popup`.

For a global binding that does not require the tmux prefix, bind it manually
after the TPM plugin declaration:

```tmux
bind-key -n C-p run-shell '"${TMUX_PARATOR_BIN:-tmux-parator}" popup'
```

With the release installer:

```sh
curl -fsSL https://raw.githubusercontent.com/sschmerda/tmux-parator/main/scripts/install.sh | sh
```

Install a specific release:

```sh
TMUX_PARATOR_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/sschmerda/tmux-parator/main/scripts/install.sh | sh
```

The installer supports macOS and Linux on `amd64` and `arm64`, matching the
precompiled GitHub release archives. It installs to `~/.local/bin` by default.
Override that with `TMUX_PARATOR_INSTALL_DIR`.

Build from source:

```sh
go install github.com/sschmerda/tmux-parator/cmd/tmux-parator@latest
```

Local development build:

```sh
go build -o bin/tmux-parator ./cmd/tmux-parator
```

Or use the Makefile:

```sh
make fmt
make test
make build
make run
make popup
make check
```

## Release Builds

Release builds are handled by GoReleaser and GitHub Actions. Pushing a `v*` tag
runs tests, builds Linux/macOS `amd64` and `arm64` archives, generates
`checksums.txt`, and publishes a GitHub release.

Create a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Run a local snapshot with GoReleaser:

```sh
goreleaser release --snapshot --clean
```

Verify a published release archive with GitHub artifact attestations when the
release was built from a public repository:

```sh
gh attestation verify tmux-parator_linux_arm64.tar.gz \
  --repo sschmerda/tmux-parator
```
