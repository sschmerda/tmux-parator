package tmux

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestParseSessions(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []Session
	}{
		{
			name: "one formatted session per line",
			out:  "main\x1f2\x1f1\x1fMon Jun  1 10:00:00 2026\x1f\x1f\x1f\x1f\x1f\x1f\x1f\nwork\x1f1\x1f0\x1fTue Jun  2 11:00:00 2026\x1f\x1f\x1f\x1f\x1f\x1f\x1f\n",
			want: []Session{
				{Name: "main", Windows: "2", Attached: true, CreatedTime: "Mon Jun  1 10:00:00 2026"},
				{Name: "work", Windows: "1", Attached: false, CreatedTime: "Tue Jun  2 11:00:00 2026"},
			},
		},
		{
			name: "tagged parator session metadata",
			out:  "parator-dev\x1f1\x1f0\x1fTue Jun  2 11:00:00 2026\x1f1\x1frepo\x1f/Users/me/repos/tmux-parator\x1frepos\x1ftmux-parator\x1fR\x1f#d6a84f\n",
			want: []Session{
				{
					Name:        "parator-dev",
					Windows:     "1",
					Attached:    false,
					CreatedTime: "Tue Jun  2 11:00:00 2026",
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

type recordingRunner struct {
	calls [][]string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	return nil, nil
}
