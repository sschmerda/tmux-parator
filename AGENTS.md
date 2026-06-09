# AGENTS.md

## Project Overview

This repository contains `tmux-parator`.

`tmux-parator` is a standalone Go binary and Bubble Tea TUI for preparing reproducible tmux workspaces from existing tmux sessions, configured paths, git repositories, git worktrees, declarative layouts, and later devcontainer runtimes.

It is part of a Latin-themed tmux tool suite:

- `tmux-dux`: command palette / central launcher
- `tmux-parator`: workspace preparer / session, repo, worktree, layout, and runtime manager
- `tmux-custos`: possible future coding-agent monitor / watcher

The name `parator` comes from Latin `parare`, meaning to prepare or make ready. The tool should prepare a workspace before the user enters it.

Core idea:

> `tmux-parator` turns paths, repositories, worktrees, and layouts into reproducible tmux workspaces.

## Current Priority

Build the project incrementally.

Do not attempt to implement the complete long-term vision in the first pass.

The first useful version should:

1. Start as a standalone binary called `tmux-parator`.
2. Open a Bubble Tea TUI.
3. List current tmux sessions.
4. Filter/search those sessions.
5. Switch to the selected session.
6. Kill a selected session with confirmation.
7. Handle errors cleanly.
8. Include a small README.
9. Include tests for parsing and pure helper logic.

Only after this works should the project expand into configured paths, repos, worktrees, layouts, and devcontainer runtimes.

## Non-Goals for v0.1

Do not implement these in the first version:

- git worktree creation
- devcontainer support
- TOML layout engine
- complex pane layouts
- layout memory
- zoxide integration
- preview panes
- fzf integration
- required `fd`
- required `find`
- agent monitoring

These are future milestones.

## Core Principles

- Keep the binary self-contained.
- Prefer Go standard library functionality where practical.
- Keep the TUI fully controlled by Bubble Tea.
- Do not delegate the primary interface to external fuzzy finders.
- Do not add preview panes; prioritize clear candidate names and paths.
- Make destructive actions explicit and confirm them.
- Keep config user-authored and state/cache machine-generated.
- Keep packages testable.
- Avoid global mutable state.
- Avoid shell-heavy architecture except where interacting with required external tools such as `tmux` and `git`.

## Technology

Use:

- Go
- Bubble Tea
- Bubbles where useful
- Lip Gloss where useful
- TOML for future config
- Go standard library filesystem traversal for future discovery
- `os/exec` for `tmux`, `git`, and optional `fd`

Required runtime tools:

- `tmux`

Future required runtime tools:

- `git`, once repo/worktree support is added

Optional future runtime tools:

- `fd`, only as an acceleration backend if available and configured

Avoid required dependencies on:

- `fzf`
- `find`
- `zoxide`
- `bat`
- preview tools

## Binary and Commands

The main binary should be:

```sh
tmux-parator
```

Default behavior:

```sh
tmux-parator
```

Starts the TUI in the current terminal, pane, or popup.

Popup behavior:

```sh
tmux-parator popup
tmux-parator --popup
```

Should eventually wrap the same TUI with a tmux popup, for example:

```sh
tmux display-popup -E -w 90% -h 90% "tmux-parator"
```

Popup mode is a convenience wrapper. The core TUI must not depend on popup mode.

## Suggested Project Structure

Use a clean, idiomatic Go structure. A good starting point:

```text
cmd/tmux-parator/main.go

internal/app/
  app.go

internal/tmux/
  session.go
  popup.go

internal/ui/
  model.go
  update.go
  view.go
  keymap.go
  candidate.go
  confirm.go

internal/config/
  config.go

internal/discovery/
  discoverer.go

internal/git/
  repo.go
  worktree.go

internal/layout/
  layout.go

internal/state/
  state.go
```

Do not create empty packages unless they are useful soon. It is acceptable to start with only:

```text
cmd/tmux-parator/main.go
internal/app/app.go
internal/tmux/session.go
internal/ui/model.go
```

and add the other packages when features require them.

## Tmux Session Behavior

For v0.1, implement:

- list sessions
- switch session
- kill session

Use `tmux list-sessions` in a parseable format if possible, for example:

```sh
tmux list-sessions -F '#{session_name}'
```

Switch:

```sh
tmux switch-client -t <session>
```

Kill:

```sh
tmux kill-session -t <session>
```

Errors should be returned and displayed in the UI. Do not panic for normal tmux command failures.

Command execution should be testable. Prefer a small runner interface, for example:

```go
type Runner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
}
```

Then tmux functions can be tested without spawning real tmux processes.

## TUI Behavior

The v0.1 TUI should:

- show a list of sessions
- allow filtering/searching
- allow selecting an item
- Enter: switch to selected session
- Ctrl-k: ask to kill selected session
- y: confirm kill when confirmation is active
- n or Esc: cancel confirmation
- q: quit
- show useful error messages
- avoid crashing on empty session lists

No preview pane.

Use the available width for the session list and status/help text.

## Candidate Model

The UI should eventually show different candidate kinds. Introduce the concept early if useful.

Future candidate kinds:

- `session`
- `path`
- `repo`
- `worktree`
- `action`

Suggested type:

```go
type CandidateKind string

const (
    CandidateSession  CandidateKind = "session"
    CandidatePath     CandidateKind = "path"
    CandidateRepo     CandidateKind = "repo"
    CandidateWorktree CandidateKind = "worktree"
)

type Candidate struct {
    Kind        CandidateKind
    Name        string
    Path        string
    SessionName string
    RepoName    string
    RepoPath    string
    Branch      string
    LayoutName  string
}
```

For v0.1, only `CandidateSession` is required.

## Future Config

Future config should live at:

```text
~/.config/tmux-parator/config.toml
```

Future generated state should live at:

```text
~/.local/state/tmux-parator/state.json
```

Config and state must remain separate:

- config: user-authored defaults
- state: remembered runtime choices

Possible future config shape:

```toml
[general]
default_layout = "default"
layout_selection = "auto" # auto | prompt | none
session_name_template = "{{ .Name }}"

[popup]
width = "90%"
height = "90%"

[discovery]
backend = "auto" # auto | go | fd
include_hidden = false
follow_symlinks = false
ignore_dirs = [
  ".git",
  "node_modules",
  "target",
  "vendor",
  ".cache",
  ".venv",
  "__pycache__",
  "dist",
  "build"
]

[[roots]]
name = "Projects"
path = "~/repos"
mode = "repos" # repos | subdirs
max_depth = 4
default_layout = "go-dev"
default_runtime = "host"

[[roots]]
name = "Scratch"
path = "~/scratch"
mode = "subdirs"
max_depth = 2
default_layout = "default"
default_runtime = "host"

[worktrees]
base_dir = "~/worktrees"
name_template = "{{ .Repo }}_{{ .Branch }}"
default_layout = "agent"
default_runtime = "host"
base_branch = "main"
```

## Future Discovery

Discovery should support configured roots.

Root modes:

- `subdirs`: every subdirectory under the root up to max depth is a possible session
- `repos`: only git repositories under the root are possible sessions

Default discovery backend should be pure Go traversal using the standard library.

Use:

- `filepath.WalkDir`
- `os.ReadDir`

Discovery should be asynchronous so the UI remains responsive.

Skip heavy directories by default:

- `.git`
- `node_modules`
- `target`
- `vendor`
- `.cache`
- `.venv`
- `__pycache__`
- `dist`
- `build`

Do not follow symlinks by default.

## Future Optional fd Backend

`fd` may be used only as an optional acceleration backend.

Config should support:

```toml
[discovery]
backend = "auto" # auto | go | fd
```

Behavior:

- `go`: always use built-in Go traversal
- `fd`: use `fd` if available; fall back to Go if unavailable unless strict behavior is later added
- `auto`: use `fd` if available, otherwise Go

The TUI must not care which backend produced the candidates.

## Future Git Repository Detection

A directory is a git repository if it contains `.git`.

Important:

`.git` may be either a directory or a file. Worktrees often use a `.git` file.

Do not require `.git` to be a directory.

Suggested logic:

```go
func IsGitRepo(path string) bool {
    _, err := os.Stat(filepath.Join(path, ".git"))
    return err == nil
}
```

In `repos` discovery mode, once a repo is found, add it as a candidate and usually skip deeper traversal inside it.

## Future Worktree Support

Worktree support is a core future feature.

From a selected repo candidate, the user should be able to create a worktree.

Flow:

1. Select repo.
2. Press the worktree creation key.
3. Prompt for branch name.
4. Derive worktree name.
5. Create worktree.
6. Create or connect to the workspace.

Naming rule:

```text
{original_repo_name}_{branch_name}
```

Example:

```text
repo:   tmux-parator
branch: feature/layouts

worktree/session name:
tmux-parator_feature_layouts
```

Branch names must be sanitized. Replace unsafe characters such as:

- `/`
- spaces
- `:`
- repeated separators where useful

If branch does not exist:

```sh
git -C <repo> worktree add <worktree_path> -b <branch>
```

If branch exists:

```sh
git -C <repo> worktree add <worktree_path> <branch>
```

Worktree-created workspaces should use the configured default worktree layout and runtime.

## Future Layouts

Layouts are first-class.

A layout defines how a tmux workspace is created.

Initial layout backend:

- `native`: built-in TOML layout compiled to tmux commands

Future backends:

- `script`: user-provided shell script
- `tmuxifier`: optional tmuxifier-style layout support

Use a backend abstraction so additional layout syntaxes can be added later.

Suggested interface:

```go
type LayoutBackend interface {
    CreateSession(ctx context.Context, spec CreateSessionSpec) error
}
```

Native layouts should eventually support:

- multiple windows
- one command per window
- pane layouts
- horizontal pane groups
- vertical pane groups
- percentage sizing
- per-window or per-pane start directory overrides

Example future layout:

```toml
[[layouts]]
name = "agent"
backend = "native"
runtime = "host"

[[layouts.windows]]
name = "dev"

[layouts.windows.layout]
type = "hbox"
children = [
  { name = "editor", size = "50%", command = "nvim ." },
  { name = "agent",  size = "30%", command = "codex" },
  { name = "shell",  size = "20%", command = "$SHELL" }
]

[[layouts.windows]]
name = "test"
command = "go test ./..."
```

This should produce a window similar to:

```text
| editor 50% | agent 30% | shell 20% |
```

Nested layouts can be a later extension.

## Future Layout Selection

Config should support:

```toml
[general]
default_layout = "default"
layout_selection = "auto" # auto | prompt | none
```

Meaning:

- `auto`: use path-specific layout, remembered layout, root default, or global default
- `prompt`: ask before creating a new session
- `none`: create a plain session without layout

## Future Layout Memory

The app may remember which layout was used for a path.

Do not write this into the user config file.

Use a generated state file, for example:

```text
~/.local/state/tmux-parator/state.json
```

Example state:

```json
{
  "path_layouts": {
    "/Users/stefan/repos/tmux-parator": "go-dev"
  },
  "recent_paths": ["/Users/stefan/repos/tmux-parator"]
}
```

## Future Devcontainer Runtime

Devcontainer support is important, but it must be implemented only after basic session creation, repo discovery, worktree creation, and layouts are working.

Some repos are generated from a devcontainer template and contain a top-level `devcontainer/` directory with build instructions and lockfiles. Worktrees copy this directory and its lockfiles, so each worktree contains the data required to rebuild its own container.

For a repo or worktree with devcontainer data, the runtime lifecycle should be:

```text
select repo/worktree
derive workspace name
detect devcontainer runtime
build or build from lockfile first
if build succeeds:
    call the configured command that connects host tmux to the container
if build fails:
    show error and do not create/switch a normal host session
```

Important:

For devcontainer workspaces, `tmux-parator` should not first create a normal host tmux session. The devcontainer runtime should prepare the container first, then call the correct host-tmux connection command.

The app should not hard-code the devcontainer internals. It should call configured commands from the workspace path.

Possible future config:

```toml
[[runtimes]]
name = "devcontainer"
type = "commands"
detect_paths = ["devcontainer"]
working_dir = "."

prefer_lockfile = true
lockfile_paths = [
  "devcontainer/devcontainer.lock",
  "devcontainer/compose.lock.yml",
  "devcontainer/lock.json"
]

build = "make devcontainer-build NAME={{ .WorkspaceName }}"
build_from_lockfile = "make devcontainer-build-lock NAME={{ .WorkspaceName }}"
connect = "make devcontainer-tmux NAME={{ .WorkspaceName }}"
```

Commands should run with:

```text
working directory = workspace path
```

For a worktree:

```text
~/worktrees/myrepo_feature_x
```

the build/connect commands must execute from that worktree so they use the worktree’s own `devcontainer/` directory and lockfiles.

Suggested future runtime interface:

```go
type Runtime interface {
    Detect(path string) bool
    Prepare(ctx context.Context, ws Workspace) error
    Connect(ctx context.Context, ws Workspace) error
}
```

Host runtime:

```text
Prepare:
  no-op

Connect:
  create tmux session from layout
  switch-client
```

Devcontainer runtime:

```text
Prepare:
  run build/build-from-lockfile command

Connect:
  run configured host-tmux-to-container command
```

## Suggested Future Workspace Model

```go
type Workspace struct {
    Name       string
    Path       string
    RepoName   string
    BranchName string
    Runtime    string
    Layout     string
}
```

Command templates may receive:

```text
{{ .WorkspaceName }}
{{ .Path }}
{{ .RepoName }}
{{ .BranchName }}
{{ .LayoutName }}
{{ .RuntimeName }}
```

## Keybindings

Suggested eventual keybindings:

```text
Enter      switch/create/connect selected workspace
Ctrl-k     kill selected tmux session, with confirmation
Ctrl-w     create worktree from selected repo
Ctrl-d     remove selected worktree, with confirmation
Ctrl-l     choose/change layout for selected path/repo/worktree
Tab        cycle source filter
Esc        cancel current prompt or go back
q          quit
```

For v0.1, only implement what is needed for session switching and safe session killing.

## Testing Expectations

Prefer testable packages over logic embedded directly in the Bubble Tea model.

Write tests for:

- tmux session parsing
- session name sanitization
- branch name sanitization
- worktree name generation
- config loading
- root path expansion
- git repo detection
- discovery ignore rules
- layout selection precedence
- layout command generation where feasible

Avoid tests that require a live tmux server unless they are explicitly marked integration tests.

## Style

- Write idiomatic Go.
- Keep functions small.
- Prefer explicit types over clever abstractions.
- Avoid global mutable state.
- Make shell command execution injectable where practical.
- Return useful errors.
- Keep UI state transitions clear and simple.
- Do not over-engineer before v0.1 works.
- Use `gofmt`.

## Commands to Run

After changes, run:

```sh
go fmt ./...
go test ./...
go build ./cmd/tmux-parator
```

If the project structure changes, adjust the build command accordingly.

## Implementation Milestones

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

## Definition of Done for v0.1

A useful v0.1 should let the user:

1. Open `tmux-parator`.
2. See current tmux sessions.
3. Filter/search them.
4. Switch to a selected session.
5. Kill a session with confirmation.
6. See errors cleanly.
7. Run tests successfully.

Do not expand scope before this works.
