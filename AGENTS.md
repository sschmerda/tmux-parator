# AGENTS.md

## Project

`tmux-parator` is a Go CLI/TUI for preparing and switching tmux workspaces.

The binary is `tmux-parator`. The UI is Bubble Tea-based. The tool should stay
self-contained and should interact with external tools only where needed, mainly
`tmux` and, later, `git`.

## Current Direction

Build incrementally. Prefer small, working changes over implementing the full
roadmap.

Core behavior to preserve:

- List tmux sessions.
- Filter/search candidates in the TUI.
- Switch to the selected session.
- Kill a selected session only after explicit confirmation.
- Display errors cleanly; do not panic for normal tmux failures.
- Keep logic testable without requiring a live tmux server.

Do not add large future features unless explicitly requested, including
devcontainers, layout engines, worktree creation, preview panes, zoxide/fzf
integration, or agent monitoring.

## Engineering Rules

- Write idiomatic Go.
- Prefer standard library functionality where practical.
- Use Bubble Tea/Bubbles/Lip Gloss for the TUI.
- Avoid global mutable state.
- Keep packages small and testable.
- Inject command execution behind a small runner interface where practical.
- Avoid shell-heavy logic except for required external tool calls.
- Do not create empty future packages.
- Keep destructive actions explicit and confirmed.

## Existing Structure

Important areas:

- `cmd/tmux-parator`: CLI entrypoint.
- `internal/app`: application wiring.
- `internal/tmux`: tmux command wrappers and parsing.
- `internal/ui`: Bubble Tea model and candidate UI.
- `internal/config`, `internal/discovery`, `internal/pathsearch`,
  `internal/gitignore`: discovery/config support.

Follow existing package style before introducing new abstractions.

## Verification

After code changes, run:

```sh
go fmt ./...
go test ./...
go build ./cmd/tmux-parator
```

`make check` is also available, but note that it runs `go fmt` and
`go mod tidy`.

Tests should avoid requiring a live tmux server unless clearly marked as
integration tests.
