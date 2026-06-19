package tmux

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sschmerda/tmux-parator/internal/sessionconfig"
)

func TestExecRunnerRunInDirUsesNonInteractiveHookEnvironment(t *testing.T) {
	out, err := (ExecRunner{}).RunInDir(
		context.Background(),
		t.TempDir(),
		"/bin/sh",
		"-c",
		`printf '%s\n' "$GCM_INTERACTIVE" "$GIT_ASKPASS" "$GIT_SSH_COMMAND" "$GIT_TERMINAL_PROMPT" "$SSH_ASKPASS" "$SSH_ASKPASS_REQUIRE"`,
	)
	if err != nil {
		t.Fatalf("RunInDir() unexpected error: %v", err)
	}

	got := strings.Fields(string(out))
	sort.Strings(got)
	want := []string{"0", "Never", "false", "false", "never", "ssh", "-oBatchMode=yes"}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunInDir() environment = %#v, want %#v", got, want)
	}
}

func TestParseSessions(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []Session
	}{
		{
			name: "one formatted session per line",
			out:  "main\x1f2\x1f1\x1fMon Jun  1 10:00:00 2026\x1f\x1f\x1f\x1f\x1f\x1f\x1f\x1f/Users/me\nwork\x1f1\x1f0\x1fTue Jun  2 11:00:00 2026\x1f\x1f\x1f\x1f\x1f\x1f\x1f\x1f/Users/me/work\n",
			want: []Session{
				{Name: "main", Windows: "2", Attached: true, CreatedTime: "Mon Jun  1 10:00:00 2026", CurrentPath: "/Users/me"},
				{Name: "work", Windows: "1", Attached: false, CreatedTime: "Tue Jun  2 11:00:00 2026", CurrentPath: "/Users/me/work"},
			},
		},
		{
			name: "tagged parator session metadata",
			out:  "parator-dev\x1f1\x1f0\x1fTue Jun  2 11:00:00 2026\x1f1\x1frepo\x1f/Users/me/repos/tmux-parator\x1frepos\x1ftmux-parator\x1fR\x1f#d6a84f\x1f/Users/me/repos/tmux-parator\n",
			want: []Session{
				{
					Name:        "parator-dev",
					Windows:     "1",
					Attached:    false,
					CreatedTime: "Tue Jun  2 11:00:00 2026",
					CurrentPath: "/Users/me/repos/tmux-parator",
					Metadata: SessionMetadata{
						CreatedByParator: true,
						Kind:             "repo",
						Path:             "/Users/me/repos/tmux-parator",
						Root:             "repos",
						BaseName:         "tmux-parator",
						Glyph:            "R",
						GlyphColor:       "#d6a84f",
					},
				},
			},
		},
		{
			name: "ignores empty lines and trims carriage returns",
			out:  "\nmain\r\n\nwork\n",
			want: []Session{{Name: "main"}, {Name: "work"}},
		},
		{
			name: "keeps punctuation in names",
			out:  "repo:feature/test\n",
			want: []Session{{Name: "repo:feature/test"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessions([]byte(tt.out))
			if len(got) != len(tt.want) {
				t.Fatalf("ParseSessions() len = %d, want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseSessions()[%d] = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateSessionName(t *testing.T) {
	if err := validateSessionName("main"); err != nil {
		t.Fatalf("validateSessionName() unexpected error: %v", err)
	}

	if err := validateSessionName("  "); err == nil {
		t.Fatal("validateSessionName() expected error for blank name")
	}
}

func TestIsDuplicateSessionError(t *testing.T) {
	if !IsDuplicateSessionError(errors.New("create tmux session: exit status 1: duplicate session: work")) {
		t.Fatal("IsDuplicateSessionError() = false, want true")
	}
	if IsDuplicateSessionError(errors.New("create tmux session: exit status 1: bad path")) {
		t.Fatal("IsDuplicateSessionError() = true, want false")
	}
}

func TestExactSessionTarget(t *testing.T) {
	got := exactSessionTarget("temp_cpp_test_.vscode")
	if got != "=temp_cpp_test_.vscode:" {
		t.Fatalf("exactSessionTarget() = %q, want %q", got, "=temp_cpp_test_.vscode:")
	}
}

func TestNewSessionTagsParatorMetadata(t *testing.T) {
	runner := &recordingRunner{}
	client := NewClient(runner)

	err := client.NewSession(context.Background(), "repos_tmux-parator", "/Users/me/repos/tmux-parator", SessionMetadata{
		Kind:       "repo",
		Path:       "/Users/me/repos/tmux-parator",
		Root:       "repos",
		BaseName:   "tmux-parator",
		Glyph:      "R",
		GlyphColor: "#d6a84f",
	})
	if err != nil {
		t.Fatalf("NewSession() unexpected error: %v", err)
	}

	want := [][]string{
		{"tmux", "new-session", "-d", "-s", "repos_tmux-parator", "-c", "/Users/me/repos/tmux-parator"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.created", "1"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.kind", "repo"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.path", "/Users/me/repos/tmux-parator"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.root", "repos"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.base_name", "tmux-parator"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.glyph", "R"},
		{"tmux", "set-option", "-t", "=repos_tmux-parator:", "@tmux-parator.glyph_color", "#d6a84f"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
}

func TestSwitchLastSessionUsesTmuxLastSession(t *testing.T) {
	runner := &recordingRunner{}
	client := NewClient(runner)

	if err := client.SwitchLastSession(context.Background()); err != nil {
		t.Fatalf("SwitchLastSession() unexpected error: %v", err)
	}

	want := [][]string{{"tmux", "switch-client", "-l"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
}

func TestRenameSessionUsesExactOldSessionTarget(t *testing.T) {
	runner := &recordingRunner{}
	client := NewClient(runner)

	if err := client.RenameSession(context.Background(), "old:name", "new-name"); err != nil {
		t.Fatalf("RenameSession() unexpected error: %v", err)
	}

	want := [][]string{{"tmux", "rename-session", "-t", "=old:name:", "new-name"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
}

func TestNewSessionWithLayoutCreatesNestedNamedChildLayout(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n", "%2\n", "%3\n", "%4\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "zen",
		Name:  "Zen Mode",
		Focus: "work.main.editor",
		Windows: []sessionconfig.Window{
			{
				Name:  "work",
				Focus: "main.editor",
				Layout: sessionconfig.Node{
					Type:  "columns",
					Sizes: []int{25, 50, 25},
					Children: []sessionconfig.Node{
						{Name: "left", Type: "pane"},
						{
							Name:  "main",
							Type:  "rows",
							Sizes: []int{80, 20},
							Children: []sessionconfig.Node{
								{Name: "editor", Type: "pane", Path: ".", Command: "nvim ."},
								{Name: "tests", Type: "pane"},
							},
						},
						{Name: "right", Type: "pane"},
					},
				},
			},
		},
	}

	err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template)
	if err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	wantPrefix := [][]string{
		{"tmux", "display-message", "-p", "#{window_width} #{window_height}"},
		{"tmux", "new-session", "-d", "-x", "120", "-y", "40", "-P", "-F", "#{window_id} #{pane_id}", "-s", "repo", "-n", "work", "-c", "/Users/me/repo"},
		{"tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-h", "-t", "%1", "-c", "/Users/me/repo"},
	}
	if len(runner.calls) < len(wantPrefix) {
		t.Fatalf("runner calls len = %d, want at least %d: %#v", len(runner.calls), len(wantPrefix), runner.calls)
	}
	if !reflect.DeepEqual(runner.calls[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("runner calls prefix = %#v, want %#v", runner.calls[:len(wantPrefix)], wantPrefix)
	}
	if !hasCall(runner.calls, []string{"tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-h", "-t", "%2", "-c", "/Users/me/repo"}) {
		t.Fatalf("runner calls missing second horizontal split: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-v", "-t", "%2", "-c", "/Users/me/repo"}) {
		t.Fatalf("runner calls missing vertical split: %#v", runner.calls)
	}
	if !hasCallPrefix(runner.calls, []string{"tmux", "respawn-pane", "-k", "-t", "%2", "-c", "/Users/me/repo"}) {
		t.Fatalf("runner calls missing respawn-pane prefix: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%2", "-l", "nvim ."}) {
		t.Fatalf("runner calls missing typed nvim command: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%2", "C-m"}) {
		t.Fatalf("runner calls missing nvim enter key: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "display-message", "-p", "-t", "%1", "#{window_width} #{window_height}"}) {
		t.Fatalf("runner calls missing size probe after respawn: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "resize-pane", "-t", "%1", "-x", "29"}) {
		t.Fatalf("runner calls missing first resize: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "resize-pane", "-t", "%2", "-x", "60"}) {
		t.Fatalf("runner calls missing second resize: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "resize-pane", "-t", "%2", "-y", "31"}) {
		t.Fatalf("runner calls missing vertical resize: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "select-pane", "-t", "%2"}) {
		t.Fatalf("runner calls missing final pane selection: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "select-window", "-t", "@1"}) {
		t.Fatalf("runner calls missing select first window by id: %#v", runner.calls)
	}
	if !callAppearsAfter(runner.calls, []string{"tmux", "select-pane", "-t", "%2"}, []string{"tmux", "select-window", "-t", "@1"}) {
		t.Fatalf("final focus pane was not selected after first window: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutCreatesMultipleWindows(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n", "@2 %2\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "dev",
		Name:  "Development",
		Focus: "git.git",
		Windows: []sessionconfig.Window{
			{Name: "code", Layout: sessionconfig.Node{Name: "editor", Type: "pane", Command: "nvim ."}},
			{Name: "git", Layout: sessionconfig.Node{Name: "git", Type: "pane", Path: "tools", Command: "lazygit"}},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	wantContains := [][]string{
		{"tmux", "new-session", "-d", "-x", "120", "-y", "40", "-P", "-F", "#{window_id} #{pane_id}", "-s", "repo", "-n", "code", "-c", "/Users/me/repo"},
		{"tmux", "new-window", "-d", "-P", "-F", "#{window_id} #{pane_id}", "-t", "=repo:", "-n", "git", "-c", "/Users/me/repo/tools"},
	}
	for _, want := range wantContains {
		if !hasCall(runner.calls, want) {
			t.Fatalf("runner calls missing %#v in %#v", want, runner.calls)
		}
	}
	if !hasCallPrefix(runner.calls, []string{"tmux", "respawn-pane", "-k", "-t", "%1", "-c", "/Users/me/repo"}) {
		t.Fatalf("runner calls missing first respawn-pane prefix: %#v", runner.calls)
	}
	if !hasCallPrefix(runner.calls, []string{"tmux", "respawn-pane", "-k", "-t", "%2", "-c", "/Users/me/repo/tools"}) {
		t.Fatalf("runner calls missing second respawn-pane prefix: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%1", "-l", "nvim ."}) {
		t.Fatalf("runner calls missing typed nvim command: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%1", "C-m"}) {
		t.Fatalf("runner calls missing nvim enter key: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%2", "-l", "lazygit"}) {
		t.Fatalf("runner calls missing typed lazygit command: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%2", "C-m"}) {
		t.Fatalf("runner calls missing lazygit enter key: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "select-window", "-t", "@2"}) ||
		!hasCall(runner.calls, []string{"tmux", "select-pane", "-t", "%2"}) {
		t.Fatalf("runner calls did not select configured final target: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutWrapperCommandModeRespawnsShellCommand(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "wrapper",
		Name:  "Wrapper",
		Focus: "files.files",
		Windows: []sessionconfig.Window{
			{
				Name: "files",
				Layout: sessionconfig.Node{
					Name:        "files",
					Type:        "pane",
					Path:        ".",
					Command:     "yazi",
					CommandMode: sessionconfig.CommandModeWrapper,
				},
			},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	if !hasCallPrefix(runner.calls, []string{"tmux", "respawn-pane", "-k", "-t", "%1", "-c", "/Users/me/repo", "/bin/sh", "-lc"}) {
		t.Fatalf("runner calls missing wrapper respawn-pane command: %#v", runner.calls)
	}
	if !hasCallContaining(runner.calls, "set -e\nyazi\nexec ") {
		t.Fatalf("runner calls missing wrapped yazi command: %#v", runner.calls)
	}
	if hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%1", "-l", "yazi"}) {
		t.Fatalf("runner calls typed yazi interactively in wrapper mode: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutSendsPaneCommandListIndividually(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "commands",
		Name:  "Commands",
		Focus: "work.setup",
		Windows: []sessionconfig.Window{
			{
				Name: "work",
				Layout: sessionconfig.Node{
					Name:     "setup",
					Type:     "pane",
					Command:  "make generate",
					Commands: []string{"make generate", "go test ./..."},
				},
			},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%1", "-l", "make generate"}) {
		t.Fatalf("runner calls missing first typed command: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%1", "-l", "go test ./..."}) {
		t.Fatalf("runner calls missing second typed command: %#v", runner.calls)
	}
	if hasCall(runner.calls, []string{"tmux", "send-keys", "-t", "%1", "-l", "make generate && go test ./..."}) {
		t.Fatalf("runner calls chained pane commands: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutRunsTemplateHooksAroundCreate(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "dev",
		Name:  "Development",
		Focus: "code.editor",
		BeforeCreateHooks: []sessionconfig.Hook{
			{Run: "git fetch --quiet"},
			{Run: "make generate"},
		},
		AfterCreateHooks: []sessionconfig.Hook{
			{Run: "echo ready"},
		},
		Windows: []sessionconfig.Window{
			{Name: "code", Layout: sessionconfig.Node{Name: "editor", Type: "pane", Command: "nvim ."}},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	wantPrefix := [][]string{
		{"dir:/Users/me/repo", "/bin/sh", "-c", "git fetch --quiet"},
		{"dir:/Users/me/repo", "/bin/sh", "-c", "make generate"},
		{"tmux", "display-message", "-p", "#{window_width} #{window_height}"},
		{"tmux", "new-session", "-d", "-x", "120", "-y", "40", "-P", "-F", "#{window_id} #{pane_id}", "-s", "repo", "-n", "code", "-c", "/Users/me/repo"},
	}
	if len(runner.calls) < len(wantPrefix) || !reflect.DeepEqual(runner.calls[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("runner calls prefix = %#v, want %#v", runner.calls, wantPrefix)
	}
	afterCall := []string{"dir:/Users/me/repo", "/bin/sh", "-c", "echo ready"}
	selectCall := []string{"tmux", "select-window", "-t", "@1"}
	if !callAppearsAfter(runner.calls, afterCall, selectCall) {
		t.Fatalf("after_create hook did not run after select-window: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutStopsWhenBeforeCreateHookFails(t *testing.T) {
	runner := &recordingRunner{errors: []error{errors.New("exit status 1")}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "dev",
		Name:  "Development",
		Focus: "code.editor",
		BeforeCreateHooks: []sessionconfig.Hook{
			{Run: "git fetch --quiet"},
		},
		Windows: []sessionconfig.Window{
			{Name: "code", Layout: sessionconfig.Node{Name: "editor", Type: "pane"}},
		},
	}

	err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template)
	if err == nil || !strings.Contains(err.Error(), "run before_create hook") {
		t.Fatalf("NewSessionWithLayout() err = %v, want before_create hook error", err)
	}
	if hasCallStartingWith(runner.calls, "tmux") {
		t.Fatalf("runner calls included tmux after failed before_create: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutRollsBackWhenAfterCreateHookFails(t *testing.T) {
	runner := &recordingRunner{
		outputs: []string{"@1 %1\n"},
		errorsByCall: map[string]error{
			"dir:/Users/me/repo /bin/sh -c echo ready": errors.New("exit status 1"),
		},
	}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "dev",
		Name:  "Development",
		Focus: "code.editor",
		AfterCreateHooks: []sessionconfig.Hook{
			{Run: "echo ready"},
		},
		Windows: []sessionconfig.Window{
			{Name: "code", Layout: sessionconfig.Node{Name: "editor", Type: "pane"}},
		},
	}

	err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template)
	if err == nil || !strings.Contains(err.Error(), "run after_create hook") {
		t.Fatalf("NewSessionWithLayout() err = %v, want after_create hook error", err)
	}
	if !hasCall(runner.calls, []string{"tmux", "kill-session", "-t", "=repo:"}) {
		t.Fatalf("runner calls missing rollback kill-session: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutReportsRollbackFailure(t *testing.T) {
	runner := &recordingRunner{
		outputs: []string{"@1 %1\n"},
		errorsByCall: map[string]error{
			"dir:/Users/me/repo /bin/sh -c echo ready": errors.New("hook failed"),
			"tmux kill-session -t =repo:":              errors.New("kill failed"),
		},
	}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "dev",
		Name:  "Development",
		Focus: "code.editor",
		AfterCreateHooks: []sessionconfig.Hook{
			{Run: "echo ready"},
		},
		Windows: []sessionconfig.Window{
			{Name: "code", Layout: sessionconfig.Node{Name: "editor", Type: "pane"}},
		},
	}

	err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template)
	if err == nil ||
		!strings.Contains(err.Error(), "run after_create hook") ||
		!strings.Contains(err.Error(), "rollback partial tmux session") {
		t.Fatalf("NewSessionWithLayout() err = %v, want creation and rollback errors", err)
	}
}

func TestNewSessionWithLayoutRunsOnlyMatchingKindHooks(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "dev",
		Name:  "Development",
		Focus: "code.editor",
		BeforeCreateHooks: []sessionconfig.Hook{
			{Run: "git fetch --quiet", Kinds: []string{"repo"}},
			{Run: "mkdir -p tmp", Kinds: []string{"subdir"}},
			{Run: "pwd"},
		},
		AfterCreateHooks: []sessionconfig.Hook{
			{Run: "echo ready", Kinds: []string{"repo"}},
			{Run: "echo skip", Kinds: []string{"subdir"}},
		},
		Windows: []sessionconfig.Window{
			{Name: "code", Layout: sessionconfig.Node{Name: "editor", Type: "pane"}},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	if !hasCall(runner.calls, []string{"dir:/Users/me/repo", "/bin/sh", "-c", "git fetch --quiet"}) {
		t.Fatalf("runner calls missing repo before hook: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"dir:/Users/me/repo", "/bin/sh", "-c", "pwd"}) {
		t.Fatalf("runner calls missing unconditional before hook: %#v", runner.calls)
	}
	if !hasCall(runner.calls, []string{"dir:/Users/me/repo", "/bin/sh", "-c", "echo ready"}) {
		t.Fatalf("runner calls missing repo after hook: %#v", runner.calls)
	}
	if hasCall(runner.calls, []string{"dir:/Users/me/repo", "/bin/sh", "-c", "mkdir -p tmp"}) {
		t.Fatalf("runner calls included subdir-only before hook: %#v", runner.calls)
	}
	if hasCall(runner.calls, []string{"dir:/Users/me/repo", "/bin/sh", "-c", "echo skip"}) {
		t.Fatalf("runner calls included subdir-only after hook: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutResizesAllPanesFromConfigOrder(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n", "%2\n", "%3\n", "%4\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "wide",
		Name:  "Wide",
		Focus: "work.one",
		Windows: []sessionconfig.Window{
			{
				Name: "work",
				Layout: sessionconfig.Node{
					Type:  "columns",
					Sizes: []int{10, 20, 30, 40},
					Children: []sessionconfig.Node{
						{Name: "one", Type: "pane"},
						{Name: "two", Type: "pane"},
						{Name: "three", Type: "pane"},
						{Name: "four", Type: "pane"},
					},
				},
			},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	wantCalls := [][]string{
		{"tmux", "resize-pane", "-t", "%1", "-x", "12"},
		{"tmux", "resize-pane", "-t", "%2", "-x", "23"},
		{"tmux", "resize-pane", "-t", "%3", "-x", "35"},
	}
	for _, want := range wantCalls {
		if !hasCall(runner.calls, want) {
			t.Fatalf("runner calls missing %#v in %#v", want, runner.calls)
		}
	}
	if hasCall(runner.calls, []string{"tmux", "resize-pane", "-t", "%4", "-x", "48"}) {
		t.Fatalf("runner calls resized final remainder pane: %#v", runner.calls)
	}
	if hasSplitPercentageArg(runner.calls) {
		t.Fatalf("runner calls used split percentage: %#v", runner.calls)
	}
}

func TestNewSessionWithLayoutResizesRepositoryStyleRightWideLayout(t *testing.T) {
	runner := &recordingRunner{outputs: []string{"@1 %1\n", "%2\n", "%3\n"}}
	client := NewClient(runner)
	template := sessionconfig.Template{
		ID:    "repo",
		Name:  "Repository",
		Focus: "shell.main",
		Windows: []sessionconfig.Window{
			{
				Name:  "shell",
				Focus: "main",
				Layout: sessionconfig.Node{
					Type:  "columns",
					Sizes: []int{25, 25, 50},
					Children: []sessionconfig.Node{
						{Name: "left", Type: "pane"},
						{Name: "main", Type: "pane", Path: "."},
						{Name: "right", Type: "pane"},
					},
				},
			},
		},
	}

	if err := client.NewSessionWithLayout(context.Background(), "repo", "/Users/me/repo", tmuxMetadata(), template); err != nil {
		t.Fatalf("NewSessionWithLayout() unexpected error: %v", err)
	}

	wantCalls := [][]string{
		{"tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-h", "-t", "%1", "-c", "/Users/me/repo"},
		{"tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-h", "-t", "%2", "-c", "/Users/me/repo"},
		{"tmux", "resize-pane", "-t", "%1", "-x", "30"},
		{"tmux", "resize-pane", "-t", "%2", "-x", "29"},
		{"tmux", "select-pane", "-t", "%2"},
	}
	for _, want := range wantCalls {
		if !hasCall(runner.calls, want) {
			t.Fatalf("runner calls missing %#v in %#v", want, runner.calls)
		}
	}
	if hasCall(runner.calls, []string{"tmux", "resize-pane", "-t", "%3", "-x", "59"}) {
		t.Fatalf("runner calls resized final remainder pane: %#v", runner.calls)
	}
	if hasSplitPercentageArg(runner.calls) {
		t.Fatalf("runner calls used split percentage: %#v", runner.calls)
	}
}

func TestDistributeCellsUsesPaneBudgetExcludingSeparators(t *testing.T) {
	budget := paneCellBudget(120, 3)
	if budget != 118 {
		t.Fatalf("paneCellBudget(120, 3) = %d, want 118", budget)
	}
	got := distributeCells(budget, []int{25, 25, 50})
	want := []int{30, 29, 59}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("distributeCells() = %#v, want %#v", got, want)
	}
}

func TestDistributeCellsUsesRelativeSizes(t *testing.T) {
	got := distributeCells(118, []int{33, 33, 33})
	want := []int{39, 40, 39}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("distributeCells() = %#v, want %#v", got, want)
	}
}

func TestDistributeCellsPreservesMirroredSizesWhenRemainderCannotBeSplit(t *testing.T) {
	got := distributeCells(118, []int{25, 50, 25})
	want := []int{29, 60, 29}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("distributeCells() = %#v, want %#v", got, want)
	}
}

func TestListSessionsRequestsPaneCurrentPath(t *testing.T) {
	runner := &recordingRunner{}
	client := NewClient(runner)

	if _, err := client.ListSessions(context.Background()); err != nil {
		t.Fatalf("ListSessions() unexpected error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %#v, want one call", runner.calls)
	}
	if got := runner.calls[0][3]; !strings.Contains(got, "#{pane_current_path}") {
		t.Fatalf("list format = %q, want pane_current_path", got)
	}
}

type recordingRunner struct {
	calls        [][]string
	outputs      []string
	errors       []error
	errorsByCall map[string]error
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if err := r.errorForCall(call); err != nil {
		return nil, err
	}
	if err := r.nextError(); err != nil {
		return nil, err
	}
	if len(r.outputs) > 0 && callRequestsPaneID(args) {
		out := r.outputs[0]
		r.outputs = r.outputs[1:]
		return []byte(out), nil
	}
	if len(args) > 0 && args[0] == "display-message" {
		return []byte("120 40\n"), nil
	}
	return nil, nil
}

func (r *recordingRunner) RunInDir(_ context.Context, dir string, name string, args ...string) ([]byte, error) {
	call := append([]string{"dir:" + dir, name}, args...)
	r.calls = append(r.calls, call)
	if err := r.errorForCall(call); err != nil {
		return nil, err
	}
	if err := r.nextError(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *recordingRunner) errorForCall(call []string) error {
	if len(r.errorsByCall) == 0 {
		return nil
	}
	return r.errorsByCall[strings.Join(call, " ")]
}

func (r *recordingRunner) nextError() error {
	if len(r.errors) == 0 {
		return nil
	}
	err := r.errors[0]
	r.errors = r.errors[1:]
	return err
}

func callRequestsPaneID(args []string) bool {
	for _, arg := range args {
		if arg == "-P" {
			return true
		}
	}
	return false
}

func tmuxMetadata() SessionMetadata {
	return SessionMetadata{Kind: "repo", Path: "/Users/me/repo", BaseName: "repo"}
}

func hasCall(calls [][]string, want []string) bool {
	for _, call := range calls {
		if reflect.DeepEqual(call, want) {
			return true
		}
	}
	return false
}

func callAppearsAfter(calls [][]string, call []string, previous []string) bool {
	seenPrevious := false
	for _, got := range calls {
		if reflect.DeepEqual(got, previous) {
			seenPrevious = true
			continue
		}
		if seenPrevious && reflect.DeepEqual(got, call) {
			return true
		}
	}
	return false
}

func hasCallStartingWith(calls [][]string, prefix string) bool {
	for _, call := range calls {
		if len(call) > 0 && call[0] == prefix {
			return true
		}
	}
	return false
}

func hasCallPrefix(calls [][]string, prefix []string) bool {
	for _, call := range calls {
		if len(call) < len(prefix) {
			continue
		}
		if reflect.DeepEqual(call[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func hasCallContaining(calls [][]string, needle string) bool {
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), needle) {
			return true
		}
	}
	return false
}

func hasSplitPercentageArg(calls [][]string) bool {
	for _, call := range calls {
		if len(call) > 2 && call[0] == "tmux" && call[1] == "split-window" && containsArg(call, "-p") {
			return true
		}
	}
	return false
}

func containsArg(values []string, arg string) bool {
	for _, value := range values {
		if value == arg {
			return true
		}
	}
	return false
}
