package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sschmerda/tmux-parator/internal/config"
	"github.com/sschmerda/tmux-parator/internal/discovery"
	"github.com/sschmerda/tmux-parator/internal/fuzzy"
	"github.com/sschmerda/tmux-parator/internal/pathsearch"
	"github.com/sschmerda/tmux-parator/internal/sessionconfig"
	"github.com/sschmerda/tmux-parator/internal/templatememory"
	"github.com/sschmerda/tmux-parator/internal/theme"
	"github.com/sschmerda/tmux-parator/internal/tmux"
)

func renderedColumn(line string, needle string) int {
	index := strings.Index(line, needle)
	if index < 0 {
		return -1
	}
	return lipgloss.Width(line[:index])
}

func headerLine(view string) string {
	for _, line := range strings.Split(ansi.Strip(view), "\n") {
		if strings.Contains(line, "kind") && strings.Contains(line, "root") && strings.Contains(line, "name") {
			return line
		}
	}
	return ""
}

func TestSanitizeSessionName(t *testing.T) {
	tests := map[string]string{
		"tmux-parator":          "tmux-parator",
		"repo feature/test":     "repo_feature_test",
		"  weird:::name  ":      "weird_name",
		"workspace.with-dash":   "workspace_with-dash",
		"temp/cpp_test/.vscode": "temp_cpp_test_vscode",
		"":                      "workspace",
	}
	for input, want := range tests {
		if got := sanitizeSessionName(input); got != want {
			t.Fatalf("sanitizeSessionName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRootCandidateSessionNameUsesRootNamespace(t *testing.T) {
	item := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName:     "work",
			Name:         "api",
			RelativePath: "client-a/api",
			DisplayPath:  "work/client-a/api",
		},
	}
	if got := item.sessionName(); got != "api" {
		t.Fatalf("sessionName() = %q, want api", got)
	}
	if got := item.detail(); got != "work/client-a/api" {
		t.Fatalf("detail() = %q, want compact display path", got)
	}
}

func TestRebuildCandidatesCombinesSessionsAndRoots(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rootItems = []discovery.Candidate{{Name: "tmux-dux", Path: "/tmp/tmux-dux", Mode: "repo"}}

	model.rebuildCandidates()
	model.applyFilter()

	if len(model.filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(model.filtered))
	}
	if model.filtered[0].kind != candidateSession || model.filtered[0].title() != "main" {
		t.Fatalf("first candidate = %#v, want main session", model.filtered[0])
	}
	if model.filtered[1].kind != candidateRoot || model.filtered[1].title() != "tmux-dux" {
		t.Fatalf("second candidate = %#v, want tmux-dux root", model.filtered[1])
	}
}

func TestRebuildCandidatesSortsOpenSessionsByCurrentThenActivity(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{
		{Name: "older", Activity: 100},
		{Name: "current", Current: true, Activity: 50},
		{Name: "recent", Activity: 200},
	}

	model.rebuildCandidates()
	model.applyFilter()

	got := []string{
		model.filtered[0].title(),
		model.filtered[1].title(),
		model.filtered[2].title(),
	}
	want := []string{"current", "recent", "older"}
	if !slices.Equal(got, want) {
		t.Fatalf("session order = %#v, want %#v", got, want)
	}
}

func TestDefaultBrowseCursorStartsOnSecondOpenSession(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{
		{Name: "current", Current: true, Activity: 100},
		{Name: "recent", Activity: 90},
	}
	model.rebuildCandidates()
	model.applyFilter()

	model.selectDefaultBrowseCursor()

	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want second open session index 1", model.cursor)
	}
}

func TestDefaultBrowseCursorStaysOnOnlyOpenSession(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "current", Current: true, Activity: 100}}
	model.rebuildCandidates()
	model.applyFilter()

	model.selectDefaultBrowseCursor()

	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want only open session index 0", model.cursor)
	}
}

func TestSectionHeadersSeparateSessionsAndWorkspacesWithAndWithoutFilter(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rootItems = []discovery.Candidate{{Name: "tmux-dux", Path: "/tmp/tmux-dux", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	if !model.sectionHeaderBefore(0) || !model.sectionHeaderBefore(1) {
		t.Fatal("section headers missing for session/workspace groups")
	}

	model.filter = "tmux"
	model.applyFilter()
	if !model.sectionHeaderBefore(0) {
		t.Fatal("section header missing while filtering")
	}
}

func TestMainFilterGroupsMatchingSessionsBeforeRoots(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "tmux-dux"}, {Name: "other-tmux"}}
	model.rootItems = []discovery.Candidate{
		{RootName: "projects", Name: "tmux-dux", RelativePath: "tmux-dux", DisplayPath: "projects/tmux-dux", Mode: "repo"},
	}
	model.rebuildCandidates()
	model.filter = "tmux"
	model.applyFilter()

	if len(model.filtered) != 3 {
		t.Fatalf("filtered len = %d, want 3", len(model.filtered))
	}
	if model.filtered[0].kind != candidateSession || model.filtered[1].kind != candidateSession {
		t.Fatalf("first two candidates = %#v, want sessions first", model.filtered[:2])
	}
	if model.filtered[2].kind != candidateRoot {
		t.Fatalf("third candidate kind = %v, want root", model.filtered[2].kind)
	}
}

func TestMainFilterMatchesRootPrefixBeforeUnderscore(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 180
	model.height = 40
	model.rootItems = []discovery.Candidate{
		{RootName: "temp", Name: "quarto-examples", RelativePath: "quarto-examples", DisplayPath: "temp/quarto-examples", Mode: "subdir"},
		{RootName: "repos", Name: "zahnoel_analyse", RelativePath: "zahnoel_analyse", DisplayPath: "repos/zahnoel_analyse", Mode: "repo"},
	}
	model.rebuildCandidates()
	model.filter = "zahn"
	model.applyFilter()

	if len(model.filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1: %#v", len(model.filtered), model.filtered)
	}
	if model.filtered[0].title() != "zahnoel_analyse" {
		t.Fatalf("filtered title = %q, want zahnoel_analyse", model.filtered[0].title())
	}
	if view := ansi.Strip(model.View()); !strings.Contains(view, "zahnoel_analyse") {
		t.Fatalf("rendered view does not include match:\n%s", view)
	}
}

func TestBrowseTabJumpsBetweenSections(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "tmux-dux"}, {Name: "other"}}
	model.rootItems = []discovery.Candidate{
		{RootName: "projects", Name: "tmux-dux", RelativePath: "tmux-dux", DisplayPath: "projects/tmux-dux", Mode: "repo"},
	}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.cursor != 2 {
		t.Fatalf("cursor after tab = %d, want first workspace index 2", model.cursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(Model)
	if model.cursor != 1 {
		t.Fatalf("cursor after shift-tab = %d, want second session index 1", model.cursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(Model)
	if model.cursor != 2 {
		t.Fatalf("cursor after shift-tab from sessions = %d, want first workspace index 2", model.cursor)
	}
}

func TestBrowseShiftTabJumpsToPreviousSectionFromWithinCurrentSection(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "tmux-dux"}, {Name: "other"}}
	model.rootItems = []discovery.Candidate{
		{RootName: "projects", Name: "tmux-dux", RelativePath: "tmux-dux", DisplayPath: "projects/tmux-dux", Mode: "repo"},
		{RootName: "projects", Name: "notes", RelativePath: "notes", DisplayPath: "projects/notes", Mode: "repo"},
	}
	model.rebuildCandidates()
	model.applyFilter()
	model.cursor = 3

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(Model)
	if model.cursor != 1 {
		t.Fatalf("cursor after shift-tab from workspace section = %d, want second session index 1", model.cursor)
	}
}

func TestSessionOriginsDoNotInferFromMatchingRootNames(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "repos_tmux-parator"}, {Name: "scratch_notes"}}
	model.rootItems = []discovery.Candidate{
		{RootName: "repos", Name: "tmux-parator", RelativePath: "tmux-parator", DisplayPath: "repos/tmux-parator", Mode: "repo"},
		{RootName: "scratch", Name: "notes", RelativePath: "notes", DisplayPath: "scratch/notes", Mode: "subdir"},
	}

	model.rebuildCandidates()
	model.applyFilter()

	if model.filtered[0].origin != "" {
		t.Fatalf("first origin = %q, want no inferred origin", model.filtered[0].origin)
	}
	if model.filtered[1].origin != "" {
		t.Fatalf("second origin = %q, want no inferred origin", model.filtered[1].origin)
	}
}

func TestSessionOriginsUseTmuxMetadata(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{
		{Name: "renamed-session", Metadata: tmux.SessionMetadata{Kind: "repo", Path: "/tmp/tmux-parator", Root: "repos"}},
		{Name: "scratch_notes", Metadata: tmux.SessionMetadata{Kind: "repo", Path: "/tmp/notes", Root: "repos"}},
	}
	model.rootItems = []discovery.Candidate{
		{RootName: "scratch", Name: "notes", RelativePath: "notes", DisplayPath: "scratch/notes", Mode: "subdir"},
	}

	model.rebuildCandidates()
	model.applyFilter()

	if model.filtered[0].origin != "repo" {
		t.Fatalf("renamed session origin = %q, want repo", model.filtered[0].origin)
	}
	if model.filtered[1].origin != "repo" {
		t.Fatalf("metadata origin = %q, want repo", model.filtered[1].origin)
	}
}

func TestCandidateSessionMetadata(t *testing.T) {
	root := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{Name: "tmux-parator", RootName: "repos", Mode: "repo", Path: "/tmp/tmux-parator", Glyph: "R", GlyphColor: "#d6a84f"},
	}
	if got := root.sessionMetadata(); got.Kind != "repo" || got.Path != "/tmp/tmux-parator" || got.Root != "repos" || got.BaseName != "tmux-parator" || got.Glyph != "R" || got.GlyphColor != "#d6a84f" {
		t.Fatalf("root metadata = %#v, want repo path/root/base/glyph/color", got)
	}

	path := candidate{
		kind:   candidatePath,
		fsPath: pathsearch.Candidate{Name: "notes", Path: "/tmp/notes"},
	}
	if got := path.sessionMetadata(); got.Kind != "path" || got.Path != "/tmp/notes" || got.Root != "" || got.BaseName != "notes" {
		t.Fatalf("path metadata = %#v, want path path/base", got)
	}
}

func TestSessionDetailUsesInferredCurrentPathForUntaggedSession(t *testing.T) {
	item := candidate{
		kind:    candidateSession,
		session: tmux.Session{Name: "main", Windows: "3", Attached: true, CurrentPath: "/tmp/main"},
	}
	if got := item.detail(); got != "/tmp/main" {
		t.Fatalf("detail() = %q, want inferred current path", got)
	}

	item.session.Metadata.Path = "/tmp/main"
	if got := item.detail(); got != "/tmp/main" {
		t.Fatalf("detail() = %q, want metadata path", got)
	}
}

func TestSessionDetailUsesCompactRootPath(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "concepts", Metadata: tmux.SessionMetadata{Path: "/tmp/repos/concepts", Root: "repos"}}}
	model.rootItems = []discovery.Candidate{{RootName: "repos", Name: "concepts", Path: "/tmp/repos/concepts", RelativePath: "concepts", DisplayPath: "repos/concepts"}}

	model.rebuildCandidates()

	if len(model.candidates) == 0 {
		t.Fatal("candidates empty")
	}
	if got := model.candidates[0].detail(); got != "repos/concepts" {
		t.Fatalf("detail() = %q, want compact root path", got)
	}
}

func TestSessionDetailPrefersMetadataPathOverCurrentPath(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{
		Name:        "concepts",
		CurrentPath: "/tmp/other/location",
		Metadata:    tmux.SessionMetadata{Path: "/tmp/repos/concepts", Root: "repos"},
	}}
	model.rootItems = []discovery.Candidate{{RootName: "repos", Name: "concepts", Path: "/tmp/repos/concepts", RelativePath: "concepts", DisplayPath: "repos/concepts"}}

	model.rebuildCandidates()

	if got := model.candidates[0].detail(); got != "repos/concepts" {
		t.Fatalf("detail() = %q, want metadata-derived compact path", got)
	}
}

func TestCandidateRootLabel(t *testing.T) {
	tests := []struct {
		name string
		item candidate
		want string
	}{
		{
			name: "session metadata root",
			item: candidate{kind: candidateSession, session: tmux.Session{Metadata: tmux.SessionMetadata{Root: "repos"}}},
			want: "repos",
		},
		{
			name: "configured root name",
			item: candidate{kind: candidateRoot, root: discovery.Candidate{RootName: "scratch", Path: "/tmp/repos"}},
			want: "scratch",
		},
		{
			name: "root basename fallback",
			item: candidate{kind: candidateRoot, root: discovery.Candidate{Path: "/tmp/repos"}},
			want: "repos",
		},
		{
			name: "path search no root",
			item: candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Path: "/tmp/repos"}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.rootLabel(); got != tt.want {
				t.Fatalf("rootLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCandidateGlyphPrefersRootAndSessionOverrides(t *testing.T) {
	glyphs := config.Glyphs{Repo: "G", Subdir: "S", Path: "P", Worktree: "W", Manual: "M"}

	root := candidate{kind: candidateRoot, root: discovery.Candidate{Mode: "repo", Glyph: "R"}}
	if got := candidateGlyph(root, glyphs); got != "R" {
		t.Fatalf("root glyph = %q, want R", got)
	}

	session := candidate{kind: candidateSession, origin: "repo", session: tmux.Session{Metadata: tmux.SessionMetadata{Glyph: "M"}}}
	if got := candidateGlyph(session, glyphs); got != "M" {
		t.Fatalf("session glyph = %q, want M", got)
	}

	path := candidate{kind: candidatePath}
	if got := candidateGlyph(path, glyphs); got != "P" {
		t.Fatalf("path glyph = %q, want P", got)
	}
}

func TestCandidateGlyphColorPrefersRootAndSessionOverrides(t *testing.T) {
	root := candidate{kind: candidateRoot, root: discovery.Candidate{Mode: "subdir", GlyphColor: "#d6a84f"}}
	if got := candidateGlyphColor(root, false, config.GlyphColors{}); got != lipgloss.Color("#d6a84f") {
		t.Fatalf("root glyph color = %q, want #d6a84f", got)
	}

	session := candidate{kind: candidateSession, origin: "repo", session: tmux.Session{Metadata: tmux.SessionMetadata{GlyphColor: "#abcdef"}}}
	if got := candidateGlyphColor(session, true, config.GlyphColors{}); got != lipgloss.Color("#abcdef") {
		t.Fatalf("session glyph color = %q, want #abcdef", got)
	}
}

func TestCandidateGlyphColorUsesGlobalConfig(t *testing.T) {
	root := candidate{kind: candidateRoot, root: discovery.Candidate{Mode: "subdir"}}
	colors := config.GlyphColors{Subdir: "#112233"}
	if got := candidateGlyphColor(root, false, colors); got != lipgloss.Color("#112233") {
		t.Fatalf("root glyph color = %q, want #112233", got)
	}
}

func TestUnmatchedSessionOriginUsesManualChipMetadata(t *testing.T) {
	if got := originLabel(""); got != "manual" {
		t.Fatalf("originLabel(\"\") = %q, want manual", got)
	}
	tests := map[string]string{
		"repo":     "\ue702",
		"subdir":   "\uf0c9",
		"path":     "\U000f024b",
		"worktree": "\U000f0655",
		"manual":   "\uebc8",
		"":         "\uebc8",
	}
	for origin, want := range tests {
		if got := originGlyph(origin, config.Glyphs{}); got != want {
			t.Fatalf("originGlyph(%q) = %q, want %q", origin, got, want)
		}
	}
}

func TestOriginGlyphUsesConfiguredGlyphs(t *testing.T) {
	glyphs := config.Glyphs{
		Repo:     "R",
		Subdir:   "S",
		Path:     "P",
		Worktree: "W",
		Manual:   "M",
	}
	tests := map[string]string{
		"repo":     "R",
		"subdir":   "S",
		"path":     "P",
		"worktree": "W",
		"manual":   "M",
		"":         "M",
	}
	for origin, want := range tests {
		if got := originGlyph(origin, glyphs); got != want {
			t.Fatalf("originGlyph(%q) = %q, want %q", origin, got, want)
		}
	}
}

func TestCandidateFuzzyMatchesRootVisibleColumnsAndPaths(t *testing.T) {
	root := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName:    "repos",
			Name:        "tmux-parator",
			Path:        "/Users/me/stefan/code/repos/tmux-parator",
			DisplayPath: "repos/tmux-parator",
			Mode:        "repo",
		},
	}
	for _, query := range []string{"repos", "repo", "tmux"} {
		matches := fuzzy.Filter([]fuzzy.Candidate{root.fuzzyCandidate()}, query)
		if len(matches) != 1 {
			t.Fatalf("query %q match count = %d, want 1", query, len(matches))
		}
	}
	if matches := fuzzy.Filter([]fuzzy.Candidate{root.fuzzyCandidate()}, "stefan"); len(matches) != 0 {
		t.Fatalf("absolute path query matched root candidate, want no match: %#v", matches)
	}
}

func TestCandidateFuzzyDoesNotMatchSessionRuntimeDetails(t *testing.T) {
	session := candidate{
		kind: candidateSession,
		session: tmux.Session{
			Name:     "main",
			Windows:  "3",
			Attached: true,
		},
	}
	matches := fuzzy.Filter([]fuzzy.Candidate{session.fuzzyCandidate()}, "attached 3")
	if len(matches) != 0 {
		t.Fatalf("match count = %d, want 0", len(matches))
	}
}

func TestCandidateFuzzyMatchesSessionOrigin(t *testing.T) {
	session := candidate{
		kind:    candidateSession,
		session: tmux.Session{Name: "main"},
		origin:  "repo",
	}
	matches := fuzzy.Filter([]fuzzy.Candidate{session.fuzzyCandidate()}, "repo")
	if len(matches) != 1 {
		t.Fatalf("match count = %d, want 1", len(matches))
	}
}

func TestCandidateFuzzyMatchesTaggedSessionRootButNotStoredAbsolutePath(t *testing.T) {
	session := candidate{
		kind:   candidateSession,
		origin: "repo",
		session: tmux.Session{
			Name: "renamed",
			Metadata: tmux.SessionMetadata{
				Root: "repos",
				Path: "/Users/me/stefan/code/repos/tmux-parator",
			},
		},
	}
	for _, query := range []string{"repos", "repo"} {
		matches := fuzzy.Filter([]fuzzy.Candidate{session.fuzzyCandidate()}, query)
		if len(matches) != 1 {
			t.Fatalf("query %q match count = %d, want 1", query, len(matches))
		}
	}
	if matches := fuzzy.Filter([]fuzzy.Candidate{session.fuzzyCandidate()}, "stefan"); len(matches) != 0 {
		t.Fatalf("stored absolute path query matched session, want no match: %#v", matches)
	}
}

func TestBrowseModeJAndKAppendToFilter(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(Model)

	if model.filter != "jk" {
		t.Fatalf("filter = %q, want jk", model.filter)
	}
}

func TestCursorMovementScrollsCandidateList(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.height = 9
	model.sessions = []tmux.Session{
		{Name: "one"},
		{Name: "two"},
		{Name: "three"},
		{Name: "four"},
	}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)

	if model.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", model.cursor)
	}
	if model.scroll == 0 {
		t.Fatal("scroll = 0, want list to scroll")
	}
}

func TestBrowseScrollAndPageKeys(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.height = 14
	for i := 0; i < 10; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i)})
	}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(Model)
	if model.scroll != 1 || model.cursor != 1 {
		t.Fatalf("after ctrl+e scroll=%d cursor=%d, want 1/1", model.scroll, model.cursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	model = updated.(Model)
	if model.scroll != 0 || model.cursor != 1 {
		t.Fatalf("after ctrl+y scroll=%d cursor=%d, want 0/1", model.scroll, model.cursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(Model)
	if model.cursor != 3 {
		t.Fatalf("after ctrl+d cursor=%d, want 3", model.cursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlB})
	model = updated.(Model)
	if model.cursor != 1 {
		t.Fatalf("after ctrl+b cursor=%d, want 1", model.cursor)
	}
}

func TestPathSearchScrollAndPageKeys(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.height = 14
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("path-%02d", i)
		model.pathResult = append(model.pathResult, candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: name, Path: "/tmp/" + name}})
	}

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(Model)
	if model.pathScroll != 1 || model.pathCursor != 1 {
		t.Fatalf("after ctrl+e pathScroll=%d pathCursor=%d, want 1/1", model.pathScroll, model.pathCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(Model)
	if model.pathCursor != 3 {
		t.Fatalf("after ctrl+d pathCursor=%d, want 3", model.pathCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlB})
	model = updated.(Model)
	if model.pathCursor != 1 {
		t.Fatalf("after ctrl+b pathCursor=%d, want 1", model.pathCursor)
	}
}

func TestCommandPaletteScrollAndPageKeys(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 100
	model.height = 20
	model.openCommands(modeBrowse)

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(Model)
	if model.commandScroll != 1 || model.commandCursor != 1 {
		t.Fatalf("after ctrl+e commandScroll=%d commandCursor=%d, want 1/1", model.commandScroll, model.commandCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	model = updated.(Model)
	if model.commandScroll != 0 || model.commandCursor != 1 {
		t.Fatalf("after ctrl+y commandScroll=%d commandCursor=%d, want 0/1", model.commandScroll, model.commandCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(Model)
	if model.commandCursor <= 1 {
		t.Fatalf("after ctrl+d commandCursor=%d, want movement by more than one row", model.commandCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlB})
	model = updated.(Model)
	if model.commandCursor != 1 {
		t.Fatalf("after ctrl+b commandCursor=%d, want 1", model.commandCursor)
	}
}

func TestHelpScrollAndPageKeys(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 100
	model.height = 18
	model.openHelp(modeBrowse)

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(Model)
	if model.helpScroll != 1 || model.helpCursor != 1 {
		t.Fatalf("after ctrl+e helpScroll=%d helpCursor=%d, want 1/1", model.helpScroll, model.helpCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(Model)
	if model.helpCursor <= 1 {
		t.Fatalf("after ctrl+d helpCursor=%d, want movement by more than one row", model.helpCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlB})
	model = updated.(Model)
	if model.helpCursor != 1 {
		t.Fatalf("after ctrl+b helpCursor=%d, want 1", model.helpCursor)
	}
}

func TestCommandsAndHelpUseBrowseNavigationKeys(t *testing.T) {
	keys := config.Default().UI.Keys
	keys.Browse.Up = []string{"k"}
	keys.Browse.Down = []string{"j"}
	model := NewModelWithKeys(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, keys)

	model.openCommands(modeBrowse)
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(Model)
	if model.commandCursor != 1 {
		t.Fatalf("command cursor = %d, want browse down key to move selection", model.commandCursor)
	}

	model.openHelp(modeBrowse)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(Model)
	if model.helpCursor != 1 {
		t.Fatalf("help cursor = %d, want browse down key to move selection", model.helpCursor)
	}
}

func TestHelpFuzzySearchesActionsKeysAndDescriptions(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.openHelp(modeBrowse)

	for _, query := range []string{"kill selected", "c-k", "confirmation"} {
		model.helpInput = query
		matches := model.helpMatches()
		if len(matches) == 0 || matches[0].item.Action != "Kill selected session" {
			t.Fatalf("help query %q matches = %#v, want kill selected session", query, matches)
		}
	}
}

func TestPopupFiltersSelectBestResultAndRestorePreviousCursor(t *testing.T) {
	t.Run("commands", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{Enabled: true}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.openCommands(modeBrowse)
		model.commandCursor = 4

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("session")})
		model = updated.(Model)
		if model.commandCursor != 0 {
			t.Fatalf("filtered command cursor = %d, want best result at 0", model.commandCursor)
		}

		updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)
		if model.commandCursor != 4 {
			t.Fatalf("cleared command cursor = %d, want previous cursor 4", model.commandCursor)
		}
	})

	t.Run("help", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.openHelp(modeBrowse)
		model.helpCursor = 5

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("kill")})
		model = updated.(Model)
		if model.helpCursor != 0 {
			t.Fatalf("filtered help cursor = %d, want best result at 0", model.helpCursor)
		}

		updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)
		if model.helpCursor != 5 {
			t.Fatalf("cleared help cursor = %d, want previous cursor 5", model.helpCursor)
		}
	})

	t.Run("templates", func(t *testing.T) {
		model := NewModelWithTemplates(
			nil,
			theme.Default(),
			nil,
			discovery.Options{},
			config.PathSearch{},
			config.Glyphs{},
			config.GlyphColors{},
			config.Columns{},
			config.Default().UI.Keys,
			[]sessionconfig.Template{
				testTemplate("repo", "Repository"),
				testTemplate("notes", "Notes"),
				testTemplate("shell", "Shell"),
			},
		)
		model.mode = modeTemplatePicker
		model.templateAvailable = model.templates
		model.templateFiltered = templatePickerItems(model.templateAvailable)
		model.templateCursor = 2

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("repo")})
		model = updated.(Model)
		if model.templateCursor != 0 {
			t.Fatalf("filtered template cursor = %d, want best result at 0", model.templateCursor)
		}

		updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)
		if model.templateCursor != 2 {
			t.Fatalf("cleared template cursor = %d, want previous cursor 2", model.templateCursor)
		}
	})
}

func TestPopupFilterSelectionTransitions(t *testing.T) {
	tests := []struct {
		name        string
		previous    string
		next        string
		cursor      int
		scroll      int
		restore     int
		total       int
		wantCursor  int
		wantScroll  int
		wantRestore int
	}{
		{
			name:        "starts filtering and saves cursor",
			previous:    "",
			next:        "repo",
			cursor:      4,
			scroll:      2,
			total:       3,
			wantCursor:  0,
			wantScroll:  0,
			wantRestore: 4,
		},
		{
			name:        "changes filter and keeps saved cursor",
			previous:    "repo",
			next:        "repository",
			cursor:      2,
			scroll:      1,
			restore:     4,
			total:       2,
			wantCursor:  0,
			wantScroll:  0,
			wantRestore: 4,
		},
		{
			name:        "clears filter and restores cursor",
			previous:    "repo",
			next:        "",
			restore:     4,
			total:       8,
			wantCursor:  4,
			wantScroll:  0,
			wantRestore: 4,
		},
		{
			name:        "clamps restored cursor to available results",
			previous:    "repo",
			next:        "",
			restore:     8,
			total:       3,
			wantCursor:  2,
			wantScroll:  0,
			wantRestore: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor, scroll, restore := popupFilterSelection(
				tt.previous,
				tt.next,
				tt.cursor,
				tt.scroll,
				tt.restore,
				tt.total,
			)
			if cursor != tt.wantCursor || scroll != tt.wantScroll || restore != tt.wantRestore {
				t.Fatalf(
					"popupFilterSelection() = (%d,%d,%d), want (%d,%d,%d)",
					cursor,
					scroll,
					restore,
					tt.wantCursor,
					tt.wantScroll,
					tt.wantRestore,
				)
			}
		})
	}
}

func TestBrowseCursorStaysRenderedWhenMovingBelowViewport(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 100
	model.height = 16
	for i := 0; i < 12; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i)})
	}
	model.rebuildCandidates()
	model.applyFilter()

	for i := 0; i < 6; i++ {
		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}

	view := ansi.Strip(model.View())
	if !strings.Contains(view, "session-06") {
		t.Fatalf("selected session is not rendered after scrolling:\n%s", view)
	}
	if !strings.Contains(view, "▌") {
		t.Fatalf("selection marker is not rendered after scrolling:\n%s", view)
	}
}

func TestPathCursorStaysRenderedWhenMovingBelowViewport(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.width = 100
	model.height = 16
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("path-%02d", i)
		model.pathResult = append(model.pathResult, candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: name, Path: "/tmp/" + name}})
	}

	for i := 0; i < 6; i++ {
		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}

	view := ansi.Strip(model.View())
	if !strings.Contains(view, "path-06") {
		t.Fatalf("selected path is not rendered after scrolling:\n%s", view)
	}
	if !strings.Contains(view, "▌") {
		t.Fatalf("selection marker is not rendered after scrolling:\n%s", view)
	}
}

func TestFilteringResetsScrollWhenResultsShrink(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.height = 10
	model.sessions = []tmux.Session{{Name: "one"}, {Name: "two"}, {Name: "three"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.cursor = 2
	model.scroll = 2

	model.filter = "one"
	model.applyFilter()

	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", model.cursor)
	}
	if model.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", model.scroll)
	}
}

func TestTypingFilterSelectsFirstMatchAndClearingRestoresPreviousCursor(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.height = 10
	model.sessions = []tmux.Session{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.cursor = 2

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)

	if model.cursor != 0 {
		t.Fatalf("cursor after typing = %d, want 0", model.cursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)

	if model.filter != "" {
		t.Fatalf("filter after backspace = %q, want empty", model.filter)
	}
	if model.cursor != 2 {
		t.Fatalf("cursor after clearing filter = %d, want 2", model.cursor)
	}
}

func TestAltBackspaceDeletesShellWordFromBrowseFilter(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.height = 10
	model.sessions = []tmux.Session{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.cursor = 2

	for _, value := range "alpha beta" {
		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}})
		model = updated.(Model)
	}
	var updated tea.Model
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
	model = updated.(Model)

	if model.filter != "alpha " {
		t.Fatalf("filter = %q, want alpha space", model.filter)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
	model = updated.(Model)

	if model.filter != "" {
		t.Fatalf("filter after second alt-backspace = %q, want empty", model.filter)
	}
	if model.cursor != 2 {
		t.Fatalf("cursor after clearing filter = %d, want 2", model.cursor)
	}
}

func TestAltBackspaceDeletesShellWordFromPromptModes(t *testing.T) {
	t.Run("commands", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.openCommands(modeBrowse)
		model.commandInput = "open selected"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
		model = updated.(Model)

		if model.commandInput != "open " {
			t.Fatalf("commandInput = %q, want open space", model.commandInput)
		}
	})

	t.Run("create", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.mode = modeCreateSession
		model.createText = "api-server"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
		model = updated.(Model)

		if model.createText != "api-" {
			t.Fatalf("createText = %q, want api-", model.createText)
		}
	})

	t.Run("rename", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.mode = modeRenameSession
		model.renameText = "docs_v2"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
		model = updated.(Model)

		if model.renameText != "docs_" {
			t.Fatalf("renameText = %q, want docs_", model.renameText)
		}
	})
}

func TestCtrlUClearsPromptModes(t *testing.T) {
	t.Run("browse", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.height = 10
		model.sessions = []tmux.Session{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}
		model.rebuildCandidates()
		model.applyFilter()
		model.cursor = 2
		model.addBrowseFilterText("alpha")

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)

		if model.filter != "" {
			t.Fatalf("filter = %q, want empty", model.filter)
		}
		if model.cursor != 2 {
			t.Fatalf("cursor after clearing filter = %d, want 2", model.cursor)
		}
	})

	t.Run("commands", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.openCommands(modeBrowse)
		model.commandInput = "open selected"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)

		if model.commandInput != "" {
			t.Fatalf("commandInput = %q, want empty", model.commandInput)
		}
	})

	t.Run("create", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.mode = modeCreateSession
		model.createText = "api-server"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)

		if model.createText != "" {
			t.Fatalf("createText = %q, want empty", model.createText)
		}
	})

	t.Run("rename", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.mode = modeRenameSession
		model.renameText = "docs_v2"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)

		if model.renameText != "" {
			t.Fatalf("renameText = %q, want empty", model.renameText)
		}
	})

	t.Run("path", func(t *testing.T) {
		model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
		model.mode = modePathSearch
		model.pathInput = "/tmp/my-project"
		model.pathRoot = "/tmp"
		model.pathCompletions = []candidate{{kind: candidatePath}}
		model.pathCompletionCursor = 0
		model.pathCompletionInput = model.pathInput
		model.pathCompletionRoot = model.pathRoot
		model.pathCompletionQuery = "my-project"

		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlU})
		model = updated.(Model)

		if model.pathInput != "" {
			t.Fatalf("pathInput = %q, want empty", model.pathInput)
		}
		if model.hasPathCompletionCycle() {
			t.Fatalf("path completion cycle still active: %#v", model.pathCompletions)
		}
	})
}

func TestPathSearchAltBackspaceReparsesInputAndClearsCompletion(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = "/tmp/my-project"
	model.pathRoot = "/tmp"
	model.pathCompletions = []candidate{{kind: candidatePath}}
	model.pathCompletionCursor = 0
	model.pathCompletionInput = model.pathInput
	model.pathCompletionRoot = model.pathRoot
	model.pathCompletionQuery = "my-project"

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("alt-backspace with unchanged root returned command")
	}
	if model.pathInput != "/tmp/my-" {
		t.Fatalf("pathInput = %q, want /tmp/my-", model.pathInput)
	}
	if model.pathRoot != "/tmp" {
		t.Fatalf("pathRoot = %q, want /tmp", model.pathRoot)
	}
	if model.hasPathCompletionCycle() {
		t.Fatalf("path completion cycle still active: %#v", model.pathCompletions)
	}
}

func TestTextDeletionHelpersAreRuneSafe(t *testing.T) {
	if got := deleteLastRune("aé"); got != "a" {
		t.Fatalf("deleteLastRune() = %q, want a", got)
	}
	tests := map[string]string{
		"hello world":     "hello ",
		"/tmp/my-project": "/tmp/my-",
		"/tmp/repo/":      "/tmp/repo",
		"docs_v2":         "docs_",
		"alpha  ":         "",
	}
	for input, want := range tests {
		if got := deleteLastShellWord(input); got != want {
			t.Fatalf("deleteLastShellWord(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPathCompletionsKeepAlphabeticalOrderWithoutQuery(t *testing.T) {
	children := []pathsearch.Candidate{
		{Name: "alpha", Path: "/tmp/alpha"},
		{Name: "beta", Path: "/tmp/beta"},
	}
	got := filterPathCompletions(children, "")
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("completions = %#v, want alphabetical input order", got)
	}
}

func TestPathCompletionsUseFuzzyRankWithQuery(t *testing.T) {
	children := []pathsearch.Candidate{
		{Name: "not-repos", Path: "/tmp/not-repos"},
		{Name: "repos", Path: "/tmp/repos"},
	}
	got := filterPathCompletions(children, "repos")
	if len(got) != 2 {
		t.Fatalf("completions len = %d, want 2", len(got))
	}
	if got[0].Name != "repos" {
		t.Fatalf("first completion = %q, want repos", got[0].Name)
	}
}

func TestPathSearchSlashIsTypedInput(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathRoot = "~"

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if model.pathRoot != "/" {
		t.Fatalf("pathRoot = %q, want /", model.pathRoot)
	}
	if model.pathInput != "/" {
		t.Fatalf("pathInput = %q, want /", model.pathInput)
	}
}

func TestPathSearchCtrlOCyclesRoot(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathRoot = "~"

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlO})
	model = updated.(Model)

	if model.pathRoot != "/" {
		t.Fatalf("pathRoot = %q, want /", model.pathRoot)
	}
	if model.pathInput != "/" {
		t.Fatalf("pathInput = %q, want /", model.pathInput)
	}
	model.cyclePathRoot()
	if model.pathRoot != "." || model.pathInput != "./" {
		t.Fatalf("cycled root/input = %q/%q, want ./", model.pathRoot, model.pathInput)
	}
	model.cyclePathRoot()
	if model.pathRoot != ".." || model.pathInput != "../" {
		t.Fatalf("cycled root/input = %q/%q, want ../", model.pathRoot, model.pathInput)
	}
}

func TestMainTogglesDiscoverySkipRules(t *testing.T) {
	model := NewModel(
		nil,
		theme.Default(),
		[]config.Root{{Name: "work", Path: "/tmp", Kind: "subdir", SkipHidden: true, SkipGitignored: true}},
		discovery.Options{SkipHidden: true, SkipGitignored: true},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
	)

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}, Alt: true})
	model = updated.(Model)
	if model.discovery.SkipHidden || model.roots[0].SkipHidden {
		t.Fatalf("skip hidden not toggled off: discovery=%v root=%v", model.discovery.SkipHidden, model.roots[0].SkipHidden)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}, Alt: true})
	model = updated.(Model)
	if model.discovery.SkipGitignored || model.roots[0].SkipGitignored {
		t.Fatalf("skip gitignored not toggled off: discovery=%v root=%v", model.discovery.SkipGitignored, model.roots[0].SkipGitignored)
	}
}

func TestCommandPaletteIncludesToggleAndQuitCommands(t *testing.T) {
	model := NewModel(
		nil,
		theme.Default(),
		nil,
		discovery.Options{SkipHidden: true, SkipGitignored: true},
		config.PathSearch{Enabled: true, SkipHidden: true, SkipGitignored: true},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
	)
	model.openCommands(modeBrowse)

	titles := commandTitles(model.commandItems())
	for _, want := range []string{"Toggle hidden configured paths", "Toggle gitignored configured paths", "Quit"} {
		if !titles[want] {
			t.Fatalf("command %q missing from main palette: %#v", want, titles)
		}
	}

	model.openCommands(modePathSearch)
	titles = commandTitles(model.commandItems())
	for _, want := range []string{"Add typed path", "Toggle hidden path results", "Toggle gitignored path results", "Quit"} {
		if !titles[want] {
			t.Fatalf("command %q missing from path palette: %#v", want, titles)
		}
	}
}

func TestClearTemplateMemoryCommandAvailability(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	item, ok := commandByID(model.commandItems(), commandClearTemplate)
	if !ok {
		t.Fatal("clear template memory command missing")
	}
	if item.Enabled || !strings.Contains(item.DisabledReason, "not available") {
		t.Fatalf("command = %#v, want disabled without memory", item)
	}

	memory := &fakeTemplateMemory{
		associations: map[string]templatememory.Association{
			"/tmp/repo": {TemplateID: "repo"},
		},
	}
	model = NewModelWithTemplateMemory(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, config.Default().UI.Keys, nil, memory)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	item, ok = commandByID(model.commandItems(), commandClearTemplate)
	if !ok || !item.Enabled {
		t.Fatalf("command = %#v/%v, want enabled with memory", item, ok)
	}
	if item.Key != "command palette" {
		t.Fatalf("command key = %q, want command palette", item.Key)
	}
}

func TestClearTemplateMemoryCommandForgetsSelectedWorkspace(t *testing.T) {
	memory := &fakeTemplateMemory{
		associations: map[string]templatememory.Association{
			"/tmp/repo": {TemplateID: "repo"},
		},
	}
	model := NewModelWithTemplateMemory(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, config.Default().UI.Keys, nil, memory)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.pendingTemplatePath = "/tmp/repo"
	model.width = 96
	model.height = 24
	model.openCommands(modeBrowse)

	item, ok := commandByID(model.commandItems(), commandClearTemplate)
	if !ok || !item.Enabled {
		t.Fatalf("command = %#v/%v, want enabled", item, ok)
	}
	updated, cmd := model.runCommand(item)
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("clear template memory command returned nil command")
	}
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("update cmd = %#v, want nil", cmd)
	}
	if memory.forgottenPath != "/tmp/repo" {
		t.Fatalf("forgottenPath = %q, want /tmp/repo", memory.forgottenPath)
	}
	if _, ok := memory.associations["/tmp/repo"]; ok {
		t.Fatalf("association was not removed: %#v", memory.associations)
	}
	if model.mode != modeBrowse || model.notice == nil || model.notice.Error() != "template memory cleared" {
		t.Fatalf("mode/notice = %v/%v, want browse cleared notice", model.mode, model.notice)
	}
	view := ansi.Strip(model.View())
	if strings.Contains(view, "<enter> use template") || !strings.Contains(view, "<enter>/<esc> dismiss") {
		t.Fatalf("clear template notice has wrong actions:\n%s", view)
	}
}

func TestPathCommandPaletteClearTemplateMemoryUsesSelectedResult(t *testing.T) {
	memory := &fakeTemplateMemory{
		associations: map[string]templatememory.Association{
			"/tmp/repo": {WithoutTemplate: true},
		},
	}
	model := NewModelWithTemplateMemory(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, config.Default().UI.Keys, nil, memory)
	model.mode = modePathSearch
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "repo", Path: "/tmp/repo"}}}
	model.openCommands(modePathSearch)

	item, ok := commandByID(model.commandItems(), commandClearTemplate)
	if !ok || !item.Enabled || item.Title != "Clear selected result template memory" {
		t.Fatalf("command = %#v/%v, want path clear enabled", item, ok)
	}
	updated, cmd := model.runCommand(item)
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("path clear template memory command returned nil command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if memory.forgottenPath != "/tmp/repo" {
		t.Fatalf("forgottenPath = %q, want /tmp/repo", memory.forgottenPath)
	}
	if model.mode != modePathSearch || model.pathNotice == nil || model.pathNotice.Error() != "template memory cleared" {
		t.Fatalf("mode/pathNotice = %v/%v, want path-search cleared notice", model.mode, model.pathNotice)
	}
}

func TestPathCommandPaletteCreateTypedPathEnabledOnlyForMissingPath(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = missing
	model.openCommands(modePathSearch)

	item, ok := commandByID(model.commandItems(), commandCreateTyped)
	if !ok {
		t.Fatal("create typed path command missing")
	}
	if item.Title != "Add typed path" || item.Key != "<ctrl-a>" || !item.Enabled {
		t.Fatalf("create typed command = %#v, want title/key/enabled", item)
	}

	model.pathInput = root
	item, ok = commandByID(model.commandItems(), commandCreateTyped)
	if !ok {
		t.Fatal("create typed path command missing after existing path")
	}
	if item.Enabled || !strings.Contains(item.DisabledReason, "already exists") {
		t.Fatalf("existing path command = %#v, want disabled already exists", item)
	}
}

func TestCommandPaletteIncludesOpenLastOnlyInBrowse(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{Enabled: true}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.openCommands(modeBrowse)

	item, ok := commandByID(model.commandItems(), commandOpenLast)
	if !ok {
		t.Fatal("open last session command missing from browse palette")
	}
	if item.Title != "Open last session" || item.Key != "<ctrl-`>" || !item.Enabled {
		t.Fatalf("open last session command = %#v, want title/key/enabled", item)
	}

	model.openCommands(modePathSearch)
	if _, ok := commandByID(model.commandItems(), commandOpenLast); ok {
		t.Fatal("open last session command present in path-search palette")
	}
}

func TestConfiguredBrowseOpenSelectedKeyReplacesDefault(t *testing.T) {
	client := &fakeSessionClient{}
	keys := config.Default().UI.Keys
	keys.Browse.OpenSelected = []string{"x"}
	model := NewModelWithKeys(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, keys)
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("default enter binding returned command after remap")
	}
	if client.switchedSession != "" {
		t.Fatalf("default enter switched session %q after remap", client.switchedSession)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("configured x binding returned nil command")
	}
	msg := cmd()
	if switched, ok := msg.(switchedMsg); !ok || switched.err != nil {
		t.Fatalf("configured x binding command returned %#v", msg)
	}
	if client.switchedSession != "main" {
		t.Fatalf("configured x binding switched session %q, want main", client.switchedSession)
	}
	if model.filter != "" {
		t.Fatalf("configured x binding updated filter to %q", model.filter)
	}
}

func TestQuickSwitchSingleDigitSwitchesVisibleSession(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "one", Activity: 3}, {Name: "two", Activity: 2}, {Name: "three", Activity: 1}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("Alt+2 returned nil command")
	}
	msg := cmd()
	if switched, ok := msg.(switchedMsg); !ok || switched.err != nil {
		t.Fatalf("Alt+2 command returned %#v", msg)
	}
	if client.switchedSession != "two" {
		t.Fatalf("switched session = %q, want two", client.switchedSession)
	}
	if model.quickSwitchInput != "" {
		t.Fatalf("quickSwitchInput = %q, want empty", model.quickSwitchInput)
	}
}

func TestQuickSwitchUsesConfiguredModifiers(t *testing.T) {
	client := &fakeSessionClient{}
	quickSwitch := config.Default().UI.QuickSwitch
	quickSwitch.Modifiers = []string{"meta"}
	model := NewModelWithTemplateMemoryAndQuickSwitch(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, config.Default().UI.Keys, quickSwitch, nil, nil)
	model.sessions = []tmux.Session{{Name: "one", Activity: 2}, {Name: "two", Activity: 1}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("plain 2 returned quick switch command")
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("alt+2 returned quick switch command with meta-only modifier")
	}
}

func TestQuickSwitchTwoDigitSwitchesVisibleSession(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	for i := 1; i <= 12; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i), Activity: int64(100 - i)})
	}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("first Alt+1 returned nil timeout command")
	}
	if model.quickSwitchInput != "1" {
		t.Fatalf("quickSwitchInput = %q, want 1", model.quickSwitchInput)
	}
	view := ansi.Strip(model.View())
	if strings.Contains(view, "Quick switch") || strings.Contains(view, "press next alt+digit") {
		t.Fatalf("quick switch pending state should not render popup:\n%s", view)
	}
	if !strings.Contains(view, "11▌") {
		t.Fatalf("quick switch pending state should keep matching label visible:\n%s", view)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("second Alt+2 returned nil switch command")
	}
	msg := cmd()
	if switched, ok := msg.(switchedMsg); !ok || switched.err != nil {
		t.Fatalf("Alt+1 Alt+2 command returned %#v", msg)
	}
	if client.switchedSession != "session-02" {
		t.Fatalf("switched session = %q, want session-02", client.switchedSession)
	}
	if model.quickSwitchInput != "" {
		t.Fatalf("quickSwitchInput = %q, want empty", model.quickSwitchInput)
	}
}

func TestQuickSwitchLabelsFollowFilteredSessions(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{
		{Name: "alpha", Activity: 3},
		{Name: "beta", Activity: 2},
		{Name: "gamma", Activity: 1},
	}
	model.rootItems = []discovery.Candidate{{Name: "alpha-root", Path: "/tmp/alpha-root", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	if label, ok := model.quickSwitchLabelForIndex(0); !ok || label != "1" {
		t.Fatalf("unfiltered first label = %q/%v, want 1/true", label, ok)
	}

	model.updateBrowseFilter("ga")
	if len(model.filtered) == 0 || model.filtered[0].title() != "gamma" {
		t.Fatalf("filtered results = %#v, want gamma first", candidateTitles(model.filtered))
	}
	if label, ok := model.quickSwitchLabelForIndex(0); !ok || label != "1" {
		t.Fatalf("filtered gamma label = %q/%v, want 1/true", label, ok)
	}
	model.clearBrowseFilter()
	if model.filtered[0].title() != "alpha" {
		t.Fatalf("cleared filter first = %q, want alpha", model.filtered[0].title())
	}
}

func TestQuickSwitchSkipsRootsAndCapsAtEightyOneLabels(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	for i := 1; i <= 82; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i), Activity: int64(1000 - i)})
	}
	model.rootItems = []discovery.Candidate{{Name: "root", Path: "/tmp/root", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	if label, ok := model.quickSwitchLabelForIndex(0); !ok || label != "11" {
		t.Fatalf("first label = %q/%v, want 11/true", label, ok)
	}
	if label, ok := model.quickSwitchLabelForIndex(80); !ok || label != "99" {
		t.Fatalf("81st label = %q/%v, want 99/true", label, ok)
	}
	if label, ok := model.quickSwitchLabelForIndex(81); ok || label != "" {
		t.Fatalf("82nd label = %q/%v, want no label", label, ok)
	}
	if label, ok := model.quickSwitchLabelForIndex(82); ok || label != "" {
		t.Fatalf("root label = %q/%v, want no label", label, ok)
	}
}

func TestQuickSwitchPendingAbortsOnPlainDigitAndTimeout(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	for i := 1; i <= 10; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i), Activity: int64(100 - i)})
	}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true})
	model = updated.(Model)
	if model.quickSwitchInput != "1" {
		t.Fatalf("quickSwitchInput = %q, want 1", model.quickSwitchInput)
	}
	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("plain digit after pending quick switch returned command")
	}
	if model.quickSwitchInput != "" {
		t.Fatalf("quickSwitchInput after plain digit = %q, want empty", model.quickSwitchInput)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true})
	model = updated.(Model)
	token := model.quickSwitchToken
	updated, _ = model.Update(quickSwitchTimeoutMsg{token: token})
	model = updated.(Model)
	if model.quickSwitchInput != "" {
		t.Fatalf("quickSwitchInput after timeout = %q, want empty", model.quickSwitchInput)
	}
}

func TestBrowseQuickSwitchGutterKeepsColumnsAligned(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 120
	model.height = 20
	model.sessions = []tmux.Session{{Name: "main", Metadata: tmux.SessionMetadata{Path: "/tmp/main"}}}
	model.rootItems = []discovery.Candidate{{Name: "root", Path: "/tmp/root", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	view := ansi.Strip(model.View())
	header := headerLine(view)
	sessionLine := ""
	rootLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "main") {
			sessionLine = line
		}
		if strings.Contains(line, "root") && !strings.Contains(line, "kind") {
			rootLine = line
		}
	}
	if header == "" || sessionLine == "" || rootLine == "" {
		t.Fatalf("missing lines:\n%s", view)
	}
	headerKind := renderedColumn(header, "kind")
	sessionKind := renderedColumn(sessionLine, "\uebc8")
	rootKind := renderedColumn(rootLine, "\ue702")
	if headerKind != sessionKind || headerKind != rootKind {
		t.Fatalf("kind columns header/session/root = %d/%d/%d:\n%s\n%s\n%s", headerKind, sessionKind, rootKind, header, sessionLine, rootLine)
	}
	labelIndex := renderedColumn(sessionLine, "1")
	if labelIndex < 0 || labelIndex >= sessionKind {
		t.Fatalf("quick switch label index = %d, want before kind column %d:\n%s", labelIndex, sessionKind, sessionLine)
	}
}

func TestQuickSwitchPendingPrefixHighlightKeepsLabelWidth(t *testing.T) {
	styles := newStyles(theme.Default())
	rendered := renderQuickSwitchLabel("11", "1", styles.muted, styles.popupAccent)
	if got := lipgloss.Width(rendered); got != quickSwitchLabelWidth {
		t.Fatalf("label width = %d, want %d", got, quickSwitchLabelWidth)
	}
	if ansi.Strip(rendered) != "11" {
		t.Fatalf("rendered label text = %q, want 11", ansi.Strip(rendered))
	}
}

func TestRunOpenLastSessionCommandCallsClient(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeCommands
	model.commandPreviousMode = modeBrowse

	item, ok := commandByID(model.commandItems(), commandOpenLast)
	if !ok {
		t.Fatal("open last session command missing")
	}
	updated, cmd := model.runCommand(item)
	model = updated.(Model)
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want browse", model.mode)
	}
	if cmd == nil {
		t.Fatal("runCommand() returned nil command")
	}
	msg := cmd()
	if _, ok := msg.(switchedMsg); !ok {
		t.Fatalf("cmd message = %#v, want switchedMsg", msg)
	}
	if client.switchLastCalls != 1 {
		t.Fatalf("switchLastCalls = %d, want 1", client.switchLastCalls)
	}
}

func TestCtrlBacktickSwitchesToLastSession(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlAt})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("updateKey() returned nil command")
	}
	if _, ok := cmd().(switchedMsg); !ok {
		t.Fatal("command did not return switchedMsg")
	}
	if client.switchLastCalls != 1 {
		t.Fatalf("switchLastCalls = %d, want 1", client.switchLastCalls)
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want browse", model.mode)
	}
}

func TestCtrlBacktickSwitchesToLastSessionFromPathSearch(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathBusy = true

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlAt})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("updateKey() returned nil command")
	}
	if _, ok := cmd().(switchedMsg); !ok {
		t.Fatal("command did not return switchedMsg")
	}
	if client.switchLastCalls != 1 {
		t.Fatalf("switchLastCalls = %d, want 1", client.switchLastCalls)
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want browse", model.mode)
	}
	if model.pathBusy {
		t.Fatal("pathBusy = true, want stopped path search")
	}
}

func TestConfirmKillArrowSelectionAndEnter(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	model = updated.(Model)
	if model.mode != modeConfirmKill {
		t.Fatalf("mode = %v, want confirm kill", model.mode)
	}
	if model.confirmChoice != confirmCancel {
		t.Fatalf("confirmChoice = %v, want cancel", model.confirmChoice)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(Model)
	if model.confirmChoice != confirmYes {
		t.Fatalf("confirmChoice = %v, want yes", model.confirmChoice)
	}

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("enter on confirm returned nil command")
	}
	if _, ok := cmd().(killedMsg); !ok {
		t.Fatal("enter on confirm did not return killedMsg")
	}
	if client.killCalls != 1 || client.killedSession != "main" {
		t.Fatalf("kill = (%d, %q), want (1, main)", client.killCalls, client.killedSession)
	}
}

func TestConfirmKillEnterCancelsByDefault(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.mode = modeConfirmKill
	model.confirmChoice = confirmCancel

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("enter on cancel returned command")
	}
	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want browse", model.mode)
	}
	if client.killCalls != 0 {
		t.Fatalf("killCalls = %d, want 0", client.killCalls)
	}
}

func TestCtrlNOpensRenamePromptPrefilledWithSelectedSession(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}, {Name: "other"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("ctrl-n returned command before rename submit")
	}
	if model.mode != modeRenameSession {
		t.Fatalf("mode = %v, want rename session", model.mode)
	}
	if model.renameOriginal != "main" || model.renameText != "main" {
		t.Fatalf("rename original/text = %q/%q, want main/main", model.renameOriginal, model.renameText)
	}
}

func TestCtrlNOnAvailableWorkspaceShowsNotice(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.rootItems = []discovery.Candidate{{Name: "api", Path: "/tmp/api", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.width = 96
	model.height = 24

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("ctrl-n on workspace returned command")
	}
	if model.mode != modeBrowse || model.notice == nil || !strings.Contains(model.notice.Error(), "not an open tmux session") {
		t.Fatalf("mode/notice = %v/%v, want browse notice", model.mode, model.notice)
	}
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "Notice") || !strings.Contains(view, "not an open tmux session") || !strings.Contains(view, "<enter>/<esc> dismiss") {
		t.Fatalf("workspace rename notice missing:\n%s", view)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("enter while rename notice visible returned command")
	}
	if model.notice != nil {
		t.Fatalf("notice = %v, want dismissed", model.notice)
	}
	if client.newSessionName != "" || client.switchedSession != "" {
		t.Fatalf("client used after rename notice dismiss: %#v", client)
	}
}

func TestRenameCommandOnAvailableWorkspaceShowsNotice(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.rootItems = []discovery.Candidate{{Name: "api", Path: "/tmp/api", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.mode = modeCommands
	model.commandPreviousMode = modeBrowse

	item, ok := commandByID(model.commandItems(), commandRenameSession)
	if !ok {
		t.Fatal("rename command missing")
	}
	updated, cmd := model.runCommand(item)
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("rename command on workspace returned command")
	}
	if model.mode != modeBrowse || model.notice == nil || !strings.Contains(model.notice.Error(), "not an open tmux session") {
		t.Fatalf("mode/notice = %v/%v, want browse notice", model.mode, model.notice)
	}
}

func TestUnavailableBrowseCommandShowsNotice(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeCommands
	model.commandPreviousMode = modeBrowse

	item, ok := commandByID(model.commandItems(), commandOpenSelected)
	if !ok {
		t.Fatal("open selected command missing")
	}
	updated, cmd := model.runCommand(item)
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("unavailable command returned command")
	}
	if model.mode != modeBrowse || model.notice == nil || !strings.Contains(model.notice.Error(), "There is no selected candidate") {
		t.Fatalf("mode/notice = %v/%v, want browse notice", model.mode, model.notice)
	}
}

func TestRenameSessionRejectsDuplicateName(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}, {Name: "other"}}
	model.mode = modeRenameSession
	model.renameOriginal = "main"
	model.renameText = "other"

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("duplicate rename returned command")
	}
	if model.mode != modeRenameSession {
		t.Fatalf("mode = %v, want rename session", model.mode)
	}
	if model.notice == nil || !strings.Contains(model.notice.Error(), "already exists") {
		t.Fatalf("notice = %v, want duplicate notice", model.notice)
	}
	if client.renamedOld != "" || client.renamedNew != "" {
		t.Fatalf("rename client used for duplicate: %#v", client)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("enter while duplicate notice visible returned command")
	}
	if model.notice != nil {
		t.Fatalf("notice = %v, want dismissed", model.notice)
	}
	if model.mode != modeRenameSession {
		t.Fatalf("mode = %v, want rename session after notice dismissal", model.mode)
	}
}

func TestRenameSessionCallsClient(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main"}, {Name: "other"}}
	model.mode = modeRenameSession
	model.renameOriginal = "main"
	model.renameText = "renamed"

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
	if cmd == nil {
		t.Fatal("rename returned nil command")
	}
	if msg := cmd(); msg.(renamedMsg).err != nil {
		t.Fatalf("renamedMsg err = %v", msg.(renamedMsg).err)
	}
	if client.renamedOld != "main" || client.renamedNew != "renamed" {
		t.Fatalf("rename = %q/%q, want main/renamed", client.renamedOld, client.renamedNew)
	}
}

func TestCtrlSOpensCreateSessionPromptPrefilledWithSelectedSession(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main", CurrentPath: "/tmp/main"}, {Name: "other", CurrentPath: "/tmp/other"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("ctrl-s returned command before create submit")
	}
	if model.mode != modeCreateSession {
		t.Fatalf("mode = %v, want create session", model.mode)
	}
	if model.createText != "main" {
		t.Fatalf("createText = %q, want selected session name", model.createText)
	}
	if model.createPath != "/tmp/main" || model.createMetadata.Kind != "manual" || model.createMetadata.Path != "/tmp/main" {
		t.Fatalf("create target = %q/%#v, want selected session path/manual metadata", model.createPath, model.createMetadata)
	}
}

func TestCreateSessionCommandPrefillsSelectedSession(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main", CurrentPath: "/tmp/main"}, {Name: "other", CurrentPath: "/tmp/other"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.mode = modeCommands
	model.commandPreviousMode = modeBrowse

	item, ok := commandByID(model.commandItems(), commandNewSession)
	if !ok {
		t.Fatal("new session command missing")
	}
	updated, cmd := model.runCommand(item)
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("new session command returned command before submit")
	}
	if model.mode != modeCreateSession || model.createText != "main" {
		t.Fatalf("mode/createText = %v/%q, want create/main", model.mode, model.createText)
	}
}

func TestCreateNamedSessionFromOpenSessionCreatesSecondSessionInSamePath(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "main", Metadata: tmux.SessionMetadata{Kind: "repo", Path: "/tmp/project", Root: "repos", Glyph: "R", GlyphColor: "#f00"}}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	model.createText = "main-copy"
	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
	if cmd == nil {
		t.Fatal("create named session returned nil command")
	}
	if msg := cmd(); msg.(createdMsg).err != nil {
		t.Fatalf("createdMsg err = %v", msg.(createdMsg).err)
	}
	if client.newSessionName != "main-copy" || client.newSessionPath != "/tmp/project" {
		t.Fatalf("new session = %q/%q, want main-copy//tmp/project", client.newSessionName, client.newSessionPath)
	}
	if client.newSessionMetadata.Kind != "repo" || client.newSessionMetadata.Path != "/tmp/project" || client.newSessionMetadata.Root != "repos" || client.newSessionMetadata.BaseName != "main-copy" {
		t.Fatalf("metadata = %#v, want copied repo metadata with new basename", client.newSessionMetadata)
	}
	if client.switchedSession != "" || client.taggedSession != "" {
		t.Fatalf("create named session switched/tagged existing session: %#v", client)
	}
}

func TestCreateNamedSessionFromAvailableWorkspaceKeepsWorkspaceKind(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.rootItems = []discovery.Candidate{{Name: "api", Path: "/tmp/repos/api", RootName: "repos", Mode: "repo", Glyph: "R", GlyphColor: "#f00"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(Model)
	if model.mode != modeCreateSession || model.createText != "api" {
		t.Fatalf("mode/createText = %v/%q, want create/api", model.mode, model.createText)
	}
	model.createText = "api-copy"
	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("create named workspace session returned nil command")
	}
	if msg := cmd(); msg.(createdMsg).err != nil {
		t.Fatalf("createdMsg err = %v", msg.(createdMsg).err)
	}
	if client.newSessionName != "api-copy" || client.newSessionPath != "/tmp/repos/api" {
		t.Fatalf("new session = %q/%q, want api-copy//tmp/repos/api", client.newSessionName, client.newSessionPath)
	}
	if client.newSessionMetadata.Kind != "repo" || client.newSessionMetadata.Root != "repos" || client.newSessionMetadata.BaseName != "api-copy" {
		t.Fatalf("metadata = %#v, want repo metadata with new basename", client.newSessionMetadata)
	}
}

func TestMainViewUsesStatusChipsInsteadOfPromptStatusLine(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true, SkipGitignored: false}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 100
	model.height = 20
	model.loading = true
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()

	view := model.View()
	if strings.Contains(view, "loading sessions and roots") {
		t.Fatalf("view contains loading line:\n%s", view)
	}
	if strings.Contains(view, "skip hidden | show ignored") {
		t.Fatalf("view contains old prompt status line:\n%s", view)
	}
	if !strings.Contains(view, "help:  <ctrl-?>") || !strings.Contains(view, "commands:  <ctrl-g>") || !strings.Contains(view, "│") || !strings.Contains(view, "PATH OFF") || !strings.Contains(view, "HIDDEN SKIP") || !strings.Contains(view, "IGNORED SHOW") {
		t.Fatalf("view missing status chips:\n%s", view)
	}
	if strings.Contains(view, "enter open") || strings.Contains(view, "ctrl-? help") {
		t.Fatalf("view contains inline hotkey help:\n%s", view)
	}
}

func TestPathSearchFooterKeepsStatusChipsWithoutHotkeyHelp(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.width = 100
	model.height = 20
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "main", Path: "/tmp/main"}}}

	view := model.View()
	if !strings.Contains(view, "help:  <ctrl-?>") || !strings.Contains(view, "commands:  <ctrl-g>") || !strings.Contains(view, "│") || !strings.Contains(view, "PATH ON") || !strings.Contains(view, "HIDDEN") || !strings.Contains(view, "IGNORED") {
		t.Fatalf("path search view missing status chips:\n%s", view)
	}
	if strings.Contains(view, "enter open") || strings.Contains(view, "ctrl-? help") {
		t.Fatalf("path search view contains inline hotkey help:\n%s", view)
	}
}

func TestFooterUsesConfiguredHelpAndCommandHotkeys(t *testing.T) {
	keys := config.Default().UI.Keys
	keys.Browse.Help = []string{"f1"}
	keys.Browse.CommandPalette = []string{"ctrl+p"}
	keys.PathSearch.Help = []string{"f2"}
	keys.PathSearch.CommandPalette = []string{"ctrl+space"}

	model := NewModelWithKeys(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{}, keys)
	model.width = 120
	model.height = 20
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "help:  f1") || !strings.Contains(view, "commands:  <ctrl-p>") || !strings.Contains(view, "PATH OFF") {
		t.Fatalf("browse footer missing configured hotkeys:\n%s", view)
	}

	model.mode = modePathSearch
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "main", Path: "/tmp/main"}}}
	view = ansi.Strip(model.View())
	if !strings.Contains(view, "help:  f2") || !strings.Contains(view, "commands:  <ctrl-space>") || !strings.Contains(view, "PATH ON") {
		t.Fatalf("path footer missing configured hotkeys:\n%s", view)
	}
}

func TestFooterControlChipsUseActiveStyleWhenOpen(t *testing.T) {
	styles := newStyles(theme.Default())
	helpChip := renderFooterControlChip(footerHelpLabel, "<ctrl-?>", true, styles)
	commandChip := renderFooterControlChip(footerCommandLabel, "<ctrl-g>", true, styles)
	footer := renderStatusFooter(false, false, false, "", "<ctrl-?>", "<ctrl-g>", true, true, 120, styles)
	if !strings.Contains(footer, helpChip) {
		t.Fatal("active help footer chip does not use active status style")
	}
	if !strings.Contains(footer, commandChip) {
		t.Fatal("active command footer chip does not use active status style")
	}
}

func TestFooterControlActiveKeyStyleUsesTintedBox(t *testing.T) {
	styles := newStyles(theme.Default())
	activeKeyStyle := footerControlKeyStyle(true, styles)
	if fmt.Sprint(activeKeyStyle.GetBackground()) == fmt.Sprint(styles.statusShow.GetBackground()) {
		t.Fatal("active footer control key style uses the glyph/status accent background")
	}
	if fmt.Sprint(activeKeyStyle.GetBackground()) == fmt.Sprint(styles.statusSkip.GetBackground()) {
		t.Fatal("active footer control key style uses the inactive chip background")
	}
}

func TestFooterControlLabelStyleUsesAccentOnlyWhenActive(t *testing.T) {
	styles := newStyles(theme.Default())
	if fmt.Sprint(footerControlLabelStyle(false, styles).GetBackground()) != fmt.Sprint(styles.statusSkip.GetBackground()) {
		t.Fatal("inactive footer control label does not use the quieter chip background")
	}
	if fmt.Sprint(footerControlLabelStyle(true, styles).GetBackground()) != fmt.Sprint(styles.statusShow.GetBackground()) {
		t.Fatal("active footer control label does not use the accent background")
	}
}

func TestFooterChipsUseSymmetricPadding(t *testing.T) {
	styles := newStyles(theme.Default())
	for _, chip := range []string{
		renderFooterControlChip(footerHelpLabel, "<ctrl-?>", false, styles),
		renderFooterControlChip(footerCommandLabel, "<ctrl-g>", false, styles),
		renderPathModeChip(false, styles),
		renderStatusChip("HIDDEN", true, styles),
		renderStatusChip("IGNORED", false, styles),
	} {
		stripped := ansi.Strip(chip)
		if !strings.HasPrefix(stripped, " ") || !strings.HasSuffix(stripped, " ") {
			t.Fatalf("footer chip %q does not have symmetric one-cell padding", stripped)
		}
	}
}

func TestFooterContentLeavesSafetyMargin(t *testing.T) {
	styles := newStyles(theme.Default())
	width := 32
	footer := renderStatusFooter(false, false, false, "error: this message is intentionally long", "<ctrl-?>", "<ctrl-g>", false, false, width, styles)
	if got, want := lipgloss.Width(footer), width-footerSafetyMargin; got > want {
		t.Fatalf("footer width = %d, want <= %d: %q", got, want, ansi.Strip(footer))
	}
}

func TestMainViewRendersBrowseColumnHeaders(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 120
	model.height = 20
	model.sessions = []tmux.Session{
		{Name: "main", Metadata: tmux.SessionMetadata{Path: "/tmp/main"}},
		{Name: "other", Metadata: tmux.SessionMetadata{Path: "/tmp/other"}},
	}
	model.rootItems = []discovery.Candidate{{RootName: "repos", Name: "tmux-parator", Path: "/tmp/repos/tmux-parator", RelativePath: "tmux-parator", DisplayPath: "repos/tmux-parator", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	view := ansi.Strip(model.View())
	if !strings.Contains(view, "kind") || !strings.Contains(view, "root") || !strings.Contains(view, "name") || !strings.Contains(view, "path") {
		t.Fatalf("view missing browse column headers:\n%s", view)
	}
}

func TestMainViewRendersRoundedAppFrame(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 80
	model.height = 16
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()

	lines := strings.Split(ansi.Strip(model.View()), "\n")
	if len(lines) < 2 {
		t.Fatalf("view has too few lines:\n%s", model.View())
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Fatalf("top frame = %q, want rounded corners", lines[0])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
		t.Fatalf("bottom frame = %q, want rounded corners", lines[len(lines)-1])
	}
}

func TestMainSearchBoxIsInsetFromAppFrame(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 80
	model.height = 16
	model.sessions = []tmux.Session{{Name: "main"}}
	model.rebuildCandidates()
	model.applyFilter()

	lines := strings.Split(ansi.Strip(model.View()), "\n")
	if len(lines) < 2 {
		t.Fatalf("view has too few lines:\n%s", model.View())
	}
	if !strings.HasPrefix(lines[1], "│"+strings.Repeat(" ", searchBoxOuterInset())+"╭") {
		t.Fatalf("search box not inset from frame: %q", lines[1])
	}
}

func TestMainColumnsAndFooterAlignWithSearchInset(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 96
	model.height = 18
	model.loading = false
	model.sessions = []tmux.Session{{Name: "main", Metadata: tmux.SessionMetadata{Path: "/tmp/main"}}}
	model.rebuildCandidates()
	model.applyFilter()

	lines := strings.Split(ansi.Strip(model.View()), "\n")
	var search string
	var header string
	var section string
	var footer string
	for _, line := range lines {
		if strings.Contains(line, "❯") {
			search = line
		}
		if strings.Contains(line, "kind") && strings.Contains(line, "root") && strings.Contains(line, "name") {
			header = line
		}
		if strings.Contains(line, "OPEN SESSIONS") {
			section = line
		}
		if strings.Contains(line, "PATH OFF") && strings.Contains(line, "HIDDEN SKIP") {
			footer = line
		}
	}
	if !strings.HasPrefix(header, "│    ") {
		t.Fatalf("header not aligned with inset: %q", header)
	}
	if !strings.HasPrefix(footer, "│    ") {
		t.Fatalf("footer not aligned with inset: %q", footer)
	}
	if !strings.HasPrefix(search, "│    ") {
		t.Fatalf("search not aligned with column gutter: %q", search)
	}
	kindIndex := renderedColumn(header, "kind")
	if kindIndex < 0 {
		t.Fatalf("header missing kind column: %q", header)
	}
	if promptIndex := renderedColumn(search, "❯"); promptIndex != kindIndex {
		t.Fatalf("search prompt index = %d, want kind index %d:\nsearch: %q\nheader: %q", promptIndex, kindIndex, search, header)
	}
	footerColumnStart := renderedColumn(footer, "help:  <ctrl-?>") - footerChipHorizontalPadding
	wantFooterColumnStart := renderedColumn(search, "❯") - footerChipHorizontalPadding
	if footerColumnStart != wantFooterColumnStart {
		t.Fatalf("footer column start = %d, want %d: %q", footerColumnStart, wantFooterColumnStart, footer)
	}
	for name, line := range map[string]string{"search": search, "header": header, "footer": footer} {
		if got := lipgloss.Width(line); got != model.width {
			t.Fatalf("%s width = %d, want %d: %q", name, got, model.width, line)
		}
	}
	if !strings.HasSuffix(search, "    │") {
		t.Fatalf("search missing matching right inset: %q", search)
	}
	if !strings.HasSuffix(header, "      │") {
		t.Fatalf("header missing row right inset: %q", header)
	}
	if !strings.HasSuffix(footer, "     │") {
		t.Fatalf("footer missing row right inset: %q", footer)
	}
	openPadding := renderedColumn(section, "OPEN SESSIONS") - renderedColumn(section, "│") - 1
	if openPadding < searchBoxOuterInset() {
		t.Fatalf("section divider left padding = %d, want at least search box outer inset %d: %q", openPadding, searchBoxOuterInset(), section)
	}
	if !strings.HasSuffix(section, strings.Repeat(" ", openPadding)+"│") {
		t.Fatalf("section divider right padding does not match left padding %d: %q", openPadding, section)
	}
}

func TestMainViewKeepsTopFrameAndFooterSpacingWhenResultsFillWindow(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 100
	model.height = 12
	for i := 0; i < 20; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i)})
	}
	model.rebuildCandidates()
	model.applyFilter()

	lines := strings.Split(ansi.Strip(model.View()), "\n")
	if len(lines) != model.height {
		t.Fatalf("view height = %d, want %d:\n%s", len(lines), model.height, ansi.Strip(model.View()))
	}
	if !strings.HasPrefix(lines[0], "╭") {
		t.Fatalf("top frame missing from filled view: %q\n%s", lines[0], ansi.Strip(model.View()))
	}
	footerIndex := -1
	for i, line := range lines {
		if strings.Contains(line, "PATH OFF") && strings.Contains(line, "HIDDEN SHOW") {
			footerIndex = i
			break
		}
	}
	if footerIndex < 1 {
		t.Fatalf("footer missing from filled view:\n%s", ansi.Strip(model.View()))
	}
	if strings.Trim(lines[footerIndex-1], " │") != "" {
		t.Fatalf("footer is not separated from results by a blank row: %q\n%s", lines[footerIndex-1], ansi.Strip(model.View()))
	}
}

func TestCommandPaletteDoesNotDuplicateFooterAndKeepsFullFrame(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 100
	model.height = 12
	for i := 0; i < 20; i++ {
		model.sessions = append(model.sessions, tmux.Session{Name: fmt.Sprintf("session-%02d", i)})
	}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlG})
	model = updated.(Model)

	view := ansi.Strip(model.View())
	lines := strings.Split(view, "\n")
	if len(lines) != model.height {
		t.Fatalf("command view height = %d, want %d:\n%s", len(lines), model.height, view)
	}
	if !strings.HasPrefix(lines[0], "╭") {
		t.Fatalf("top frame missing from command view: %q\n%s", lines[0], view)
	}
	footerCount := 0
	for _, line := range lines {
		if strings.Contains(line, "help:  <ctrl-?>") && strings.Contains(line, "commands:  <ctrl-g>") && strings.Contains(line, "PATH OFF") {
			footerCount++
		}
	}
	if footerCount > 1 {
		t.Fatalf("command view footer count = %d, want <= 1:\n%s", footerCount, view)
	}
}

func TestSelectedRowMarkerAlignsWithActiveSectionText(t *testing.T) {
	styles := newStyles(theme.Default())
	item := candidate{kind: candidateSession, session: tmux.Session{Name: "main", Metadata: tmux.SessionMetadata{Path: "/tmp/main"}}}
	columns := normalizeUIColumns(config.Columns{})
	contentWidth := 80
	innerWidth := contentWidth + rowLeftInset() + rowRightInset()
	section := ansi.Strip(renderInsetDividerRow(renderBrowseSectionHeader("open sessions", true, styles, contentWidth+1), innerWidth, styles))
	row := ansi.Strip(renderInsetRow(renderCandidateRow(item, true, styles, contentWidth, config.Glyphs{}, config.GlyphColors{}, columns), innerWidth, styles))

	markerIndex := renderedColumn(row, "▌")
	sectionTextIndex := renderedColumn(section, "OPEN SESSIONS")
	if markerIndex < 0 || sectionTextIndex < 0 {
		t.Fatalf("missing marker or section text:\nsection: %q\nrow: %q", section, row)
	}
	if markerIndex != sectionTextIndex {
		t.Fatalf("marker index = %d, want section text index %d:\nsection: %q\nrow: %q", markerIndex, sectionTextIndex, section, row)
	}
}

func TestRenderBrowseSectionHeaderHighlightsActiveSection(t *testing.T) {
	styles := newStyles(theme.Default())
	active := renderBrowseSectionHeader("open sessions", true, styles, 80)
	inactive := renderBrowseSectionHeader("open sessions", false, styles, 80)
	if strings.Contains(ansi.Strip(active), "> OPEN SESSIONS") {
		t.Fatalf("active header should not include cursor marker: %q", ansi.Strip(active))
	}
	if !strings.Contains(ansi.Strip(active), "OPEN SESSIONS") {
		t.Fatalf("active header is not capitalized: %q", ansi.Strip(active))
	}
	if ansi.Strip(active) != ansi.Strip(inactive) {
		t.Fatalf("active header text = %q, want inactive header text %q", ansi.Strip(active), ansi.Strip(inactive))
	}
}

func TestRenderBrowseColumnHeaderUsesFixedWidth(t *testing.T) {
	styles := newStyles(theme.Default())
	columns := config.Columns{
		Chip: config.Column{Show: true, Width: 12},
		Root: config.Column{Show: true, Width: 12},
		Name: config.Column{Show: true, Width: 20},
		Path: config.Column{Show: true, Width: 24},
	}
	rendered := renderBrowseColumnHeader(styles, 80, columns)
	if got := lipgloss.Width(rendered); got > 80 {
		t.Fatalf("header width = %d, want <= 80", got)
	}
}

func TestRenderBrowseColumnHeaderFlexiblePathFillsWidth(t *testing.T) {
	styles := newStyles(theme.Default())
	columns := config.Columns{
		Chip: config.Column{Show: true, Width: 12},
		Root: config.Column{Show: true, Width: 12},
		Name: config.Column{Show: true, Width: 20},
		Path: config.Column{Show: true, Width: 0, MaxWidth: 0},
	}
	rendered := renderBrowseColumnHeader(styles, 80, columns)
	if got := lipgloss.Width(rendered); got != 80 {
		t.Fatalf("header width = %d, want 80", got)
	}
}

func TestPathSearchCtrlTTogglesBackToBrowse(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathBusy = true

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlT})
	model = updated.(Model)

	if model.mode != modeBrowse {
		t.Fatalf("mode = %v, want browse", model.mode)
	}
	if model.pathBusy {
		t.Fatal("pathBusy = true, want stopped path search")
	}
}

func TestPathSearchUsesSameVisibleRowBudgetAsBrowse(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.height = 20
	for i := 0; i < 30; i++ {
		model.filtered = append(model.filtered, candidate{kind: candidateSession, session: tmux.Session{Name: "session"}})
		model.pathResult = append(model.pathResult, candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "path", Path: "/tmp/path"}})
	}

	if got, want := model.pathListLimit(), model.listLimit(); got != want {
		t.Fatalf("pathListLimit = %d, want %d", got, want)
	}
}

func TestPathSearchUsesUnnumberedColumnHeader(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.width = 96
	model.height = 18
	model.loading = false
	model.sessions = []tmux.Session{{Name: "main", Metadata: tmux.SessionMetadata{Path: "/tmp/main"}}}
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "main", Path: "/tmp/main"}}}
	model.rebuildCandidates()
	model.applyFilter()

	browseHeader := headerLine(model.View())
	model.mode = modePathSearch
	pathHeader := headerLine(model.View())

	if browseHeader == "" || pathHeader == "" {
		t.Fatalf("missing header lines:\nbrowse: %q\npath: %q", browseHeader, pathHeader)
	}
	if pathHeader != browseHeader {
		t.Fatalf("path header = %q, want browse header %q", pathHeader, browseHeader)
	}
}

func TestPathSearchUsesPathsSectionDivider(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.width = 96
	model.height = 18
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "main", Path: "/tmp/main"}}}

	view := ansi.Strip(model.View())
	if !strings.Contains(view, "PATHS") {
		t.Fatalf("path search view missing paths divider:\n%s", view)
	}
	if strings.Contains(view, "OPEN SESSIONS") {
		t.Fatalf("path search view should not use browse session divider:\n%s", view)
	}
}

func TestAnchoredFooterTrimsTrailingSpacerBeforeAnchoring(t *testing.T) {
	var b strings.Builder
	b.WriteString("prompt\n\n")

	appendAnchoredFooter(&b, "footer", 6)

	if got := lipgloss.Height(b.String()); got != 5 {
		t.Fatalf("height = %d, want 5", got)
	}
	if strings.Contains(b.String(), "prompt\n\n\n\n\nfooter") {
		t.Fatalf("footer was anchored with stale trailing spacer:\n%q", b.String())
	}
}

func commandTitles(items []commandItem) map[string]bool {
	titles := make(map[string]bool, len(items))
	for _, item := range items {
		titles[item.Title] = true
	}
	return titles
}

func candidateTitles(items []candidate) []string {
	titles := make([]string, 0, len(items))
	for _, item := range items {
		titles = append(titles, item.title())
	}
	return titles
}

func commandByID(items []commandItem, id commandID) (commandItem, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return commandItem{}, false
}

func helpByKey(items []helpItem, key string) (helpItem, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return helpItem{}, false
}

type fakeSessionClient struct {
	switchLastCalls     int
	killCalls           int
	killedSession       string
	renamedOld          string
	renamedNew          string
	switchedSession     string
	newSessionName      string
	newSessionPath      string
	newSessionMetadata  tmux.SessionMetadata
	templateSessionName string
	templateSessionPath string
	templateMetadata    tmux.SessionMetadata
	templateID          string
	templateVariables   map[string]string
	taggedSession       string
	taggedMetadata      tmux.SessionMetadata
}

type fakeTemplateMemory struct {
	associations         map[string]templatememory.Association
	rememberedPath       string
	rememberedTemplateID string
	rememberedParameters map[string]string
	rememberedNoTemplate string
	forgottenPath        string
}

func (f *fakeTemplateMemory) Lookup(path string) (templatememory.Association, bool) {
	association, ok := f.associations[path]
	return association, ok
}

func (f *fakeTemplateMemory) Remember(path string, templateID string, parameters map[string]string) error {
	f.rememberedPath = path
	f.rememberedTemplateID = templateID
	f.rememberedParameters = cloneStringMap(parameters)
	if f.associations == nil {
		f.associations = make(map[string]templatememory.Association)
	}
	f.associations[path] = templatememory.Association{
		TemplateID: templateID,
		Parameters: cloneStringMap(parameters),
	}
	return nil
}

func (f *fakeTemplateMemory) RememberNoTemplate(path string) error {
	f.rememberedNoTemplate = path
	if f.associations == nil {
		f.associations = make(map[string]templatememory.Association)
	}
	f.associations[path] = templatememory.Association{WithoutTemplate: true}
	return nil
}

func (f *fakeTemplateMemory) Forget(path string) error {
	f.forgottenPath = path
	delete(f.associations, path)
	return nil
}

func (f *fakeSessionClient) ListSessions(context.Context) ([]tmux.Session, error) {
	return nil, nil
}

func (f *fakeSessionClient) SwitchSession(_ context.Context, session string) error {
	f.switchedSession = session
	return nil
}

func (f *fakeSessionClient) SwitchLastSession(context.Context) error {
	f.switchLastCalls++
	return nil
}

func (f *fakeSessionClient) KillSession(_ context.Context, session string) error {
	f.killCalls++
	f.killedSession = session
	return nil
}

func (f *fakeSessionClient) RenameSession(_ context.Context, oldName string, newName string) error {
	f.renamedOld = oldName
	f.renamedNew = newName
	return nil
}

func (f *fakeSessionClient) NewSession(_ context.Context, name string, path string, metadata tmux.SessionMetadata) error {
	f.newSessionName = name
	f.newSessionPath = path
	f.newSessionMetadata = metadata
	return nil
}

func (f *fakeSessionClient) NewSessionWithLayout(_ context.Context, name string, path string, metadata tmux.SessionMetadata, template sessionconfig.Template) error {
	f.templateSessionName = name
	f.templateSessionPath = path
	f.templateMetadata = metadata
	f.templateID = template.ID
	f.templateVariables = template.Variables
	return nil
}

func (f *fakeSessionClient) NewSessionWithLayoutAndSwitch(ctx context.Context, name string, path string, metadata tmux.SessionMetadata, template sessionconfig.Template) error {
	if err := f.NewSessionWithLayout(ctx, name, path, metadata, template); err != nil {
		return err
	}
	f.switchedSession = name
	return nil
}

func (f *fakeSessionClient) TagSession(_ context.Context, session string, metadata tmux.SessionMetadata) error {
	f.taggedSession = session
	f.taggedMetadata = metadata
	return nil
}

func testTemplate(id string, name string) sessionconfig.Template {
	return sessionconfig.Template{
		ID:      id,
		Name:    name,
		Enabled: true,
		Windows: []sessionconfig.Window{
			{Name: "work", Layout: sessionconfig.Node{Name: "shell", Type: "pane"}},
		},
	}
}

func templateWithMatch(id string, name string, patterns ...string) sessionconfig.Template {
	template := testTemplate(id, name)
	template.Match = patterns
	return template
}

func writeLocalTemplate(t *testing.T, dir string, id string, name string) {
	t.Helper()
	path := filepath.Join(dir, ".tmux-parator", "template.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create local template dir: %v", err)
	}
	content := fmt.Sprintf(`
id = %q
name = %q
focus = "work.shell"

[[windows]]
name = "work"

[windows.layout]
type = "pane"
name = "shell"
`, id, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write local template: %v", err)
	}
}

func TestBrowseCommandTextMatchesHelp(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{Enabled: true}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.openCommands(modeBrowse)
	helpItems := helpItemsForMode(modeBrowse)

	for _, command := range model.commandItems() {
		help, ok := helpByKey(helpItems, command.Key)
		if !ok {
			t.Fatalf("help for key %q missing", command.Key)
		}
		if help.Action != command.Title {
			t.Fatalf("action for %q = %q, want %q", command.Key, help.Action, command.Title)
		}
		if help.Description != command.Description {
			t.Fatalf("description for %q = %q, want %q", command.Key, help.Description, command.Description)
		}
	}
}

func TestPathSearchCommandTextMatchesHelp(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.openCommands(modePathSearch)
	helpItems := helpItemsForMode(modePathSearch)

	for _, command := range model.commandItems() {
		help, ok := helpByKey(helpItems, command.Key)
		if !ok {
			t.Fatalf("help for key %q missing", command.Key)
		}
		if help.Action != command.Title {
			t.Fatalf("action for %q = %q, want %q", command.Key, help.Action, command.Title)
		}
		if help.Description != command.Description {
			t.Fatalf("description for %q = %q, want %q", command.Key, help.Description, command.Description)
		}
	}
}

func TestPathSearchHelpReturnsToPathSearch(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlUnderscore})
	model = updated.(Model)
	if model.mode != modeHelp || model.previousMode != modePathSearch {
		t.Fatalf("mode/previous = %v/%v, want help/path search", model.mode, model.previousMode)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.mode != modePathSearch {
		t.Fatalf("mode = %v, want path search", model.mode)
	}
}

func TestPathSearchQuestionMarkIsTypedIntoPrompt(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = "/tmp/name"
	model.pathRoot = "/tmp"

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("question mark returned command")
	}
	if model.mode != modePathSearch {
		t.Fatalf("mode = %v, want path search", model.mode)
	}
	if model.pathInput != "/tmp/name?" {
		t.Fatalf("pathInput = %q, want question mark appended", model.pathInput)
	}
}

func TestPathSearchHelpRendersModeSpecificContent(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeHelp
	model.previousMode = modePathSearch
	model.width = 80
	model.height = 34

	view := model.View()
	if !strings.Contains(view, "Path Search") || !strings.Contains(view, "<ctrl-p>") || !strings.Contains(view, "<ctrl-a>") {
		t.Fatalf("path search help missing expected content:\n%s", view)
	}
	if strings.HasPrefix(view, "Path Search") {
		t.Fatalf("help rendered inline instead of centered panel:\n%s", view)
	}
}

func TestPathCompletionFallbackWritesSelectedFuzzyResultToPrompt(t *testing.T) {
	target := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathRoot = "~"
	model.pathInput = "docu"
	model.pathResult = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "documents", Path: target},
	})

	updated, _ := model.Update(pathCompletionsLoadedMsg{
		root:     "~",
		query:    "docu",
		input:    "docu",
		fallback: pathsearch.Candidate{Name: "documents", Path: target},
	})
	model = updated.(Model)

	if model.pathRoot != target {
		t.Fatalf("pathRoot = %q, want %q", model.pathRoot, target)
	}
	if model.pathInput != displayPathInput(target)+"/" {
		t.Fatalf("pathInput = %q, want locked path", model.pathInput)
	}
}

func TestPathCompletionWritesDirectChildToPrompt(t *testing.T) {
	target := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathRoot = "~"
	model.pathInput = "doc"

	updated, _ := model.Update(pathCompletionsLoadedMsg{
		root:        "~",
		query:       "doc",
		input:       "doc",
		completions: []pathsearch.Candidate{{Name: "documents", Path: target}},
		direction:   1,
	})
	model = updated.(Model)

	if model.pathRoot != target {
		t.Fatalf("pathRoot = %q, want %q", model.pathRoot, target)
	}
	if model.pathInput != displayPathInput(target)+"/" {
		t.Fatalf("pathInput = %q, want completed path", model.pathInput)
	}
}

func TestParsePathInputUsesLastSegmentAsQuery(t *testing.T) {
	root, query := parsePathInput("~/stefan/repos", "~")
	if root != "~/stefan" || query != "repos" {
		t.Fatalf("parsePathInput = (%q, %q), want (~/stefan, repos)", root, query)
	}
	root, query = parsePathInput("~/stefan/", "~")
	if root != "~/stefan" || query != "" {
		t.Fatalf("parsePathInput trailing slash = (%q, %q), want (~/stefan, empty)", root, query)
	}
	root, query = parsePathInput("~/", "~")
	if root != "~" || query != "" {
		t.Fatalf("parsePathInput ~/ = (%q, %q), want (~, empty)", root, query)
	}
	root, query = parsePathInput("./", "~")
	if root != "." || query != "" {
		t.Fatalf("parsePathInput ./ = (%q, %q), want (., empty)", root, query)
	}
	root, query = parsePathInput("../", "~")
	if root != ".." || query != "" {
		t.Fatalf("parsePathInput ../ = (%q, %q), want (.., empty)", root, query)
	}
}

func TestTypedPathCandidateRequiresDirectory(t *testing.T) {
	dir := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathInput = dir

	candidate, ok := model.typedPathCandidate()
	if !ok {
		t.Fatal("typedPathCandidate() ok = false, want true")
	}
	if candidate.path() != dir {
		t.Fatalf("candidate path = %q, want %q", candidate.path(), dir)
	}
}

func TestOpenTypedPathMissingShowsNoticeAndDoesNotOpen(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "missing")
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = target
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "other", Path: root}}}
	model.width = 96
	model.height = 24

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("ctrl-p on missing typed path returned command")
	}
	if model.mode != modePathSearch || model.pathErr == nil || !strings.Contains(model.pathErr.Error(), "not an available directory") {
		t.Fatalf("mode/pathErr = %v/%v, want path search unavailable directory", model.mode, model.pathErr)
	}
	if model.pathNotice == nil || !strings.Contains(model.pathNotice.Error(), "not an available directory") {
		t.Fatalf("pathNotice = %v, want unavailable directory notice", model.pathNotice)
	}
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "Notice") || !strings.Contains(view, "not an available directory") || !strings.Contains(view, "<enter>/<esc> dismiss") {
		t.Fatalf("missing typed path notice missing:\n%s", view)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("enter while open-typed notice visible returned command")
	}
	if model.pathNotice != nil {
		t.Fatalf("pathNotice = %v, want dismissed", model.pathNotice)
	}
	if client.newSessionName != "" || client.switchedSession != "" {
		t.Fatalf("client used after open-typed notice enter: %#v", client)
	}
}

func TestPathSearchCtrlAOpensCreatePathConfirmationForMissingPath(t *testing.T) {
	root := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = filepath.Join(root, "missing")

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("ctrl-a returned command before confirmation")
	}
	if model.mode != modeConfirmCreatePath || model.confirmChoice != confirmCancel {
		t.Fatalf("mode/choice = %v/%v, want confirm-create/cancel", model.mode, model.confirmChoice)
	}
}

func TestPathSearchEnterWithoutSelectionDoesNotCreateTypedPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "missing")
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = target

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("enter without selection returned command")
	}
	if model.mode != modePathSearch {
		t.Fatalf("mode = %v, want path search", model.mode)
	}
}

func TestCreateTypedPathConfirmationCancelDoesNotCreate(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "missing")
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeConfirmCreatePath
	model.createPathInput = target

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("cancel returned command")
	}
	if model.mode != modePathSearch {
		t.Fatalf("mode = %v, want path search", model.mode)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target stat err = %v, want not exist", err)
	}
}

func TestCreateTypedPathConfirmationYesCreatesDirectoryAndSession(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "missing")
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeConfirmCreatePath
	model.createPathInput = target

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)

	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("created path stat = (%v, %v), want directory", info, err)
	}
	if cmd == nil {
		t.Fatal("confirm create returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.newSessionPath != target || client.newSessionMetadata.Kind != "path" || client.newSessionMetadata.Path != target {
		t.Fatalf("new session path/metadata = %q/%#v, want path metadata", client.newSessionPath, client.newSessionMetadata)
	}
}

func TestCreateTypedPathExistingDirectoryWarnsAndDoesNotOpen(t *testing.T) {
	root := t.TempDir()
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = root
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: filepath.Base(root), Path: root}}}
	model.sessions = []tmux.Session{{Name: "existing", Metadata: tmux.SessionMetadata{Path: root}}}

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("existing directory returned command")
	}
	if model.mode != modePathSearch || model.pathErr == nil || !strings.Contains(model.pathErr.Error(), "already exists") {
		t.Fatalf("mode/pathErr = %v/%v, want path search already exists warning", model.mode, model.pathErr)
	}
	if model.pathNotice == nil || !strings.Contains(model.pathNotice.Error(), "already exists") {
		t.Fatalf("pathNotice = %v, want already exists notice", model.pathNotice)
	}
	if client.newSessionName != "" || client.switchedSession != "" {
		t.Fatalf("client used for existing directory: %#v", client)
	}
	model.width = 96
	model.height = 24
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "Notice") || !strings.Contains(view, "already exists") || !strings.Contains(view, "<enter>/<esc> dismiss") {
		t.Fatalf("existing directory notice missing:\n%s", view)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("enter while notice is visible returned command")
	}
	if model.pathNotice != nil {
		t.Fatalf("pathNotice = %v, want dismissed", model.pathNotice)
	}
	if client.taggedSession != "" || client.switchedSession != "" || client.newSessionName != "" {
		t.Fatalf("client used after notice enter: %#v", client)
	}
}

func TestCreateTypedPathExistingFileWarnsAndDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "file")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = target

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("existing file returned command")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "keep" {
		t.Fatalf("file content = %q, want keep", content)
	}
	if model.pathErr == nil || !strings.Contains(model.pathErr.Error(), "not a directory") {
		t.Fatalf("pathErr = %v, want not a directory warning", model.pathErr)
	}
	if model.pathNotice == nil || !strings.Contains(model.pathNotice.Error(), "not a directory") {
		t.Fatalf("pathNotice = %v, want not a directory notice", model.pathNotice)
	}
	model.width = 96
	model.height = 24
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "Notice") || !strings.Contains(view, "not a directory") {
		t.Fatalf("existing file notice missing:\n%s", view)
	}
}

func TestCreateTypedPathNoticeDismissesWithEsc(t *testing.T) {
	root := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathInput = root

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	model = updated.(Model)
	if model.pathNotice == nil {
		t.Fatal("pathNotice = nil, want notice")
	}

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("dismiss returned command")
	}
	if model.mode != modePathSearch || model.pathNotice != nil {
		t.Fatalf("mode/pathNotice = %v/%v, want path search/no notice", model.mode, model.pathNotice)
	}
	if model.pathErr == nil {
		t.Fatal("pathErr = nil, want footer warning retained after notice dismissal")
	}
}

func TestCreateTypedPathNestedCreatesParents(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "one", "two", "three")
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeConfirmCreatePath
	model.createPathInput = target

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("confirm create returned nil command")
	}
	if _, ok := cmd().(createdMsg); !ok {
		t.Fatal("command did not return createdMsg")
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("nested path stat = (%v, %v), want directory", info, err)
	}
	if client.newSessionPath != target {
		t.Fatalf("newSessionPath = %q, want %q", client.newSessionPath, target)
	}
}

func TestCreateTypedPathParentFileReportsError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(blocker, "child")
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeConfirmCreatePath
	model.createPathInput = target

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("blocked parent returned command")
	}
	if model.mode != modePathSearch || model.pathErr == nil || !strings.Contains(model.pathErr.Error(), "not a directory") {
		t.Fatalf("mode/pathErr = %v/%v, want path search not a directory error", model.mode, model.pathErr)
	}
	content, err := os.ReadFile(blocker)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "keep" {
		t.Fatalf("blocker content = %q, want keep", content)
	}
}

func TestPathCompletionRepeatedTabCyclesOriginalCandidates(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathRoot = "~"
	model.pathInput = "doc"

	updated, _ := model.Update(pathCompletionsLoadedMsg{
		root:  "~",
		query: "doc",
		input: "doc",
		completions: []pathsearch.Candidate{
			{Name: "documents", Path: first},
			{Name: "downloads", Path: second},
		},
		direction: 1,
	})
	model = updated.(Model)
	if model.pathRoot != first {
		t.Fatalf("first pathRoot = %q, want %q", model.pathRoot, first)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.pathRoot != second {
		t.Fatalf("second pathRoot = %q, want %q", model.pathRoot, second)
	}
}

func TestPathCompletionRightAcceptsCurrentCompletionContext(t *testing.T) {
	first := t.TempDir()
	next := t.TempDir()
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.pathRoot = "~"
	model.pathInput = "ste"

	updated, _ := model.Update(pathCompletionsLoadedMsg{
		root:        "~",
		query:       "ste",
		input:       "ste",
		completions: []pathsearch.Candidate{{Name: "stefan", Path: first}},
		direction:   1,
	})
	model = updated.(Model)
	if !model.hasPathCompletionCycle() {
		t.Fatal("completion cycle inactive after first completion")
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(Model)
	if model.hasPathCompletionCycle() {
		t.Fatal("completion cycle active after right arrow")
	}

	updated, _ = model.Update(pathCompletionsLoadedMsg{
		root:        first,
		query:       "",
		input:       displayPathInput(first) + "/",
		completions: []pathsearch.Candidate{{Name: "code", Path: next}},
		direction:   1,
	})
	model = updated.(Model)
	if model.pathRoot != next {
		t.Fatalf("pathRoot = %q, want next-level completion %q", model.pathRoot, next)
	}
}

func TestPathSearchFuzzyPrefersShallowerEqualScoreMatches(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/tmp"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "data", Path: "/tmp/a/b/data"},
		{Name: "data", Path: "/tmp/data"},
	})
	model.pathInput = "/tmp/data"

	model.applyPathFilter()

	if len(model.pathResult) != 2 {
		t.Fatalf("pathResult len = %d, want 2", len(model.pathResult))
	}
	if got := model.pathResult[0].path(); got != "/tmp/data" {
		t.Fatalf("first path = %q, want /tmp/data", got)
	}
}

func TestPathSearchFuzzyPrefersBasenameMatches(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/tmp"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "nested", Path: "/tmp/csedm/nested"},
		{Name: "csedm", Path: "/tmp/a/b/csedm"},
	})
	model.pathInput = "/tmp/csedm"

	model.applyPathFilter()

	if len(model.pathResult) != 2 {
		t.Fatalf("pathResult len = %d, want 2", len(model.pathResult))
	}
	if got := model.pathResult[0].path(); got != "/tmp/a/b/csedm" {
		t.Fatalf("first path = %q, want basename match /tmp/a/b/csedm", got)
	}
}

func TestPathSearchFuzzyPrefersLiteralBasenameOverWeakSubsequence(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/Users/stefan"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "sequence_distance_analytics_distance_square_matrix", Path: "/Users/stefan/code/temp/ddia_dataset_analysis/dataset_analysis/pickled_objects/sequence_distance_analytics_distance_square_matrix"},
		{Name: "CSEDM_2021_F19_Release_All_05_23_22", Path: "/Users/stefan/data/ddia/CSEDM_2021_F19_Release_All_05_23_22"},
	})
	model.pathInput = "/Users/stefan/data csedm"

	model.applyPathFilter()

	if len(model.pathResult) != 2 {
		t.Fatalf("pathResult len = %d, want 2", len(model.pathResult))
	}
	if got := model.pathResult[0].path(); got != "/Users/stefan/data/ddia/CSEDM_2021_F19_Release_All_05_23_22" {
		t.Fatalf("first path = %q, want literal CSEDM basename", got)
	}
}

func TestPathSearchFuzzyPrefersMultiTokenPathComponentCoverage(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/Users/stefan"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "sequence_distance_analytics_distance_square_matrix", Path: "/Users/stefan/code/temp/ddia_dataset_analysis/dataset_analysis/pickled_objects/sequence_distance_analytics_distance_square_matrix"},
		{Name: "CSEDM_2021_F19_Release_All_05_23_22", Path: "/Users/stefan/data/ddia/CSEDM_2021_F19_Release_All_05_23_22"},
	})
	model.pathInput = "/Users/stefan/data csedm"

	model.applyPathFilter()

	if len(model.pathResult) != 2 {
		t.Fatalf("pathResult len = %d, want 2", len(model.pathResult))
	}
	if got := model.pathResult[0].path(); got != "/Users/stefan/data/ddia/CSEDM_2021_F19_Release_All_05_23_22" {
		t.Fatalf("first path = %q, want path covering data and csedm as components", got)
	}
}

func TestPathSearchFuzzyRanksPrefixSubsequenceTypos(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/Users/stefan"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "sequence_distance_analytics_distance_square_matrix", Path: "/Users/stefan/code/temp/ddia_dataset_analysis/dataset_analysis/pickled_objects/sequence_distance_analytics_distance_square_matrix"},
		{Name: "CSEDM_2021_F19_Release_All_05_23_22", Path: "/Users/stefan/data/ddia/CSEDM_2021_F19_Release_All_05_23_22"},
	})
	model.pathInput = "/Users/stefan/data csdm"

	model.applyPathFilter()

	if len(model.pathResult) != 2 {
		t.Fatalf("pathResult len = %d, want 2", len(model.pathResult))
	}
	if got := model.pathResult[0].path(); got != "/Users/stefan/data/ddia/CSEDM_2021_F19_Release_All_05_23_22" {
		t.Fatalf("first path = %q, want CSEDM path for csdm typo", got)
	}
}

func TestPathSearchDoesNotMatchRootPrefixAsPathContent(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/Users/stefan"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "data", Path: "/Users/stefan/data"},
	})
	model.pathInput = "/Users/stefan/stefan data"

	model.applyPathFilter()

	if len(model.pathResult) != 0 {
		t.Fatalf("pathResult len = %d, want 0; matched root prefix as path content", len(model.pathResult))
	}
}

func TestPathSearchCarriesPathMatchIndexes(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.pathRoot = "/tmp"
	model.pathItems = candidatesFromPaths([]pathsearch.Candidate{
		{Name: "tmux-parator", Path: "/tmp/stefan/code/repos/tmux-parator"},
	})
	model.pathInput = "/tmp/repos"

	model.applyPathFilter()

	if len(model.pathResult) != 1 {
		t.Fatalf("pathResult len = %d, want 1", len(model.pathResult))
	}
	if len(model.pathResult[0].fieldIndexes[fieldPath]) == 0 && len(model.pathResult[0].fieldIndexes[fieldDetail]) == 0 && len(model.pathResult[0].fieldIndexes[fieldCompactPath]) == 0 {
		t.Fatalf("field indexes empty, want detail/path match indexes: %#v", model.pathResult[0].fieldIndexes)
	}
}

func TestAvailableSessionNameUsesLeafAndNumberedSuffix(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "data"}, {Name: "data_2"}}

	if got := model.availableSessionName("data"); got != "data_3" {
		t.Fatalf("availableSessionName() = %q, want data_3", got)
	}
	if got := model.availableSessionName("notes"); got != "notes" {
		t.Fatalf("availableSessionName() = %q, want notes", got)
	}
}

func TestAvailableTemplateSessionNameInterpolatesSanitizesAndNumbers(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "aoc_dev"}}
	template := sessionconfig.Template{
		Name:        "Development",
		SessionName: "{workspace_name} dev",
	}

	name, baseName, err := model.availableTemplateSessionName(
		template,
		"fallback",
		"/tmp/repos/aoc",
		tmux.SessionMetadata{Kind: "repo"},
	)
	if err != nil {
		t.Fatalf("availableTemplateSessionName() error = %v", err)
	}
	if name != "aoc_dev_2" || baseName != "aoc_dev" {
		t.Fatalf("name/baseName = %q/%q, want aoc_dev_2/aoc_dev", name, baseName)
	}
}

func TestTemplateParametersStartAtDefaultAndResolveSelection(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	template := testTemplate("monitor", "Monitor")
	template.Parameters = []sessionconfig.Parameter{{
		Name:    "monitor",
		Prompt:  "System monitor",
		Options: []string{"htop", "btop"},
		Default: "btop",
	}}

	updated, cmd := model.beginTemplateCreation(template, "repo", "/tmp/repo", tmux.SessionMetadata{Kind: "repo"}, modeBrowse)
	model = updated.(Model)
	if cmd != nil || model.mode != modeTemplateParameter || model.parameterCursor != 1 {
		t.Fatalf("parameter state = mode %v cursor %d cmd %#v, want parameter cursor 1 and nil cmd", model.mode, model.parameterCursor, cmd)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("parameter enter returned nil command")
	}
	if msg := cmd(); msg.(createdMsg).err != nil {
		t.Fatalf("createdMsg error = %v", msg.(createdMsg).err)
	}
	if client.templateVariables["monitor"] != "btop" {
		t.Fatalf("selected monitor = %q, want btop", client.templateVariables["monitor"])
	}
}

func TestSuccessfulTemplateCreationQuitsWithoutSecondSwitch(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})

	updated, cmd := model.Update(createdMsg{name: "repo", switched: true})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("successful switched template creation returned nil command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("command message = %#v, want tea.QuitMsg", msg)
	}
}

func TestTemplateParametersAdvanceGoBackAndCanBeCancelled(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	template := testTemplate("tools", "Tools")
	template.Parameters = []sessionconfig.Parameter{
		{Name: "monitor", Prompt: "Monitor", Options: []string{"btop", "htop"}, Default: "btop"},
		{Name: "agent", Prompt: "Agent", Options: []string{"codex", "claude"}, Default: "codex"},
	}

	updated, _ := model.beginTemplateCreation(template, "repo", "/tmp/repo", tmux.SessionMetadata{}, modeBrowse)
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil || model.parameterIndex != 1 || model.parameterCursor != 0 {
		t.Fatalf("parameter state = index %d cursor %d cmd %#v, want second parameter default", model.parameterIndex, model.parameterCursor, cmd)
	}
	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil || model.mode != modeTemplateParameter || model.parameterIndex != 0 || model.parameterCursor != 1 {
		t.Fatalf("back state = mode %v index %d cursor %d cmd %#v, want first parameter selection restored", model.mode, model.parameterIndex, model.parameterCursor, cmd)
	}
	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil || model.mode != modeBrowse || len(model.parameterTemplate.Parameters) != 0 {
		t.Fatalf("cancel state = mode %v template %#v cmd %#v", model.mode, model.parameterTemplate, cmd)
	}
}

func TestTemplateParameterCancelReturnsToTemplatePicker(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	template := testTemplate("tools", "Tools")
	template.Parameters = []sessionconfig.Parameter{{
		Name:    "agent",
		Prompt:  "Agent",
		Options: []string{"codex", "opencode"},
		Default: "codex",
	}}
	model.mode = modeTemplatePicker
	model.templateFiltered = []sessionconfig.Template{template}
	model.templateAvailable = []sessionconfig.Template{template}
	model.templatePath = "/tmp/repo"
	model.templateName = "repo"
	model.templatePreviousMode = modeBrowse

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil || model.mode != modeTemplateParameter {
		t.Fatalf("template selection state = mode %v cmd %#v, want parameter picker", model.mode, cmd)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil || model.mode != modeTemplatePicker {
		t.Fatalf("parameter cancel state = mode %v cmd %#v, want template picker", model.mode, cmd)
	}
	if model.templatePath != "/tmp/repo" || len(model.templateFiltered) != 1 || model.templateFiltered[0].ID != "tools" {
		t.Fatalf("template picker state was not preserved: %#v", model)
	}
}

func TestRenderTemplateParameterPickerUsesRadioBulletsAndBackLabel(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	parameter := sessionconfig.Parameter{
		Prompt:  "Coding agent",
		Options: []string{"codex", "opencode", "none"},
	}

	view := renderTemplateParameterPicker(model.styles, model.dialogs, parameter, 1, 2, 1, "enter", "esc", true, 100, 35)
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "○   codex") || !strings.Contains(plain, "●   opencode") || !strings.Contains(plain, "○   none") {
		t.Fatalf("parameter picker missing radio bullets:\n%s", plain)
	}
	if !strings.Contains(plain, "enter select") || !strings.Contains(plain, "esc back") {
		t.Fatalf("parameter picker missing navigation footer:\n%s", plain)
	}
	if !strings.Contains(plain, "▌ ●   opencode") {
		t.Fatalf("selected parameter option does not use the popup selection marker:\n%s", plain)
	}
	selectedChip := renderPopupChip("●", model.styles.selected, model.styles.selectedMatch, nil)
	if !strings.Contains(view, selectedChip) {
		t.Fatal("selected parameter chip does not use the selection bar style")
	}
}

func TestOpenCandidateCreatesDuplicateWithNumberedLeafName(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{Manual: "M"}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "data"}}
	selected := candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "data", Path: "/tmp/project/data"}}

	updated, cmd := model.openCandidate(selected)
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("openCandidate() returned nil command")
	}
	if msg := cmd(); msg.(createdMsg).err != nil {
		t.Fatalf("createdMsg err = %v", msg.(createdMsg).err)
	}
	if client.newSessionName != "data_2" || client.newSessionPath != "/tmp/project/data" {
		t.Fatalf("new session = (%q, %q), want (data_2, /tmp/project/data)", client.newSessionName, client.newSessionPath)
	}
	if client.newSessionMetadata.BaseName != "data" || client.newSessionMetadata.Kind != "path" || client.newSessionMetadata.Path != "/tmp/project/data" {
		t.Fatalf("metadata = %#v, want path path/base", client.newSessionMetadata)
	}
}

func TestOpenCandidateSwitchesExistingSessionForSamePath(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "data", Metadata: tmux.SessionMetadata{Path: "/tmp/project/data"}}}
	selected := candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "data", Path: "/tmp/project/data"}}

	updated, cmd := model.openCandidate(selected)
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("openCandidate() returned nil command")
	}
	if msg := cmd(); msg.(switchedMsg).err != nil {
		t.Fatalf("switchedMsg err = %v", msg.(switchedMsg).err)
	}
	if client.taggedSession != "data" || client.switchedSession != "data" {
		t.Fatalf("tag/switch = (%q, %q), want data/data", client.taggedSession, client.switchedSession)
	}
	if client.newSessionName != "" {
		t.Fatalf("newSessionName = %q, want empty", client.newSessionName)
	}
}

func TestTemplateHotkeyOpensPickerForUncreatedWorkspace(t *testing.T) {
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if model.mode != modeTemplatePicker || model.templatePath != "/tmp/repo" || model.templateName != "repo" {
		t.Fatalf("template state = mode %v path %q name %q, want picker /tmp/repo repo", model.mode, model.templatePath, model.templateName)
	}
}

func TestTemplateHotkeyRejectsExistingPathSession(t *testing.T) {
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.sessions = []tmux.Session{{Name: "repo", Metadata: tmux.SessionMetadata{Path: "/tmp/repo"}}}
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.cursor = 1

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if model.mode != modeBrowse || model.notice == nil || !strings.Contains(model.notice.Error(), "already exists") {
		t.Fatalf("mode/notice = %v/%v, want browse already exists notice", model.mode, model.notice)
	}
}

func TestPathSearchTemplateHotkeyOpensPickerForSelectedResult(t *testing.T) {
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.mode = modePathSearch
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "repo", Path: "/tmp/repo"}}}

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if model.mode != modeTemplatePicker || model.templatePath != "/tmp/repo" || model.templateName != "repo" || model.templatePreviousMode != modePathSearch {
		t.Fatalf("template state = mode %v path %q name %q previous %v, want picker /tmp/repo repo path-search", model.mode, model.templatePath, model.templateName, model.templatePreviousMode)
	}
}

func TestPathSearchTemplatePickerCancelReturnsToPathSearch(t *testing.T) {
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.mode = modeTemplatePicker
	model.templatePreviousMode = modePathSearch
	model.templatePath = "/tmp/repo"

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if model.mode != modePathSearch || model.templatePath != "" {
		t.Fatalf("mode/templatePath = %v/%q, want path-search empty", model.mode, model.templatePath)
	}
}

func TestPathSearchTemplatePickerCreatesSelectedTemplate(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.mode = modePathSearch
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "repo", Path: "/tmp/repo"}}}
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("template enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.templateSessionName != "repo" || client.templateSessionPath != "/tmp/repo" || client.templateID != "zen" {
		t.Fatalf("template create = (%q,%q,%q), want repo /tmp/repo zen", client.templateSessionName, client.templateSessionPath, client.templateID)
	}
	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
}

func TestBrowseEnterUsesFirstMatchingTemplate(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{
			templateWithMatch("specific", "Specific", "/tmp/repos/repo"),
			templateWithMatch("broad", "Broad", "/tmp/repos/*"),
		},
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repos/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.templateSessionName != "repo" || client.templateSessionPath != "/tmp/repos/repo" || client.templateID != "specific" {
		t.Fatalf("template create = (%q,%q,%q), want repo /tmp/repos/repo specific", client.templateSessionName, client.templateSessionPath, client.templateID)
	}
	if client.newSessionName != "" {
		t.Fatalf("new session fallback used: %#v", client)
	}
	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
}

func TestBrowseEnterUsesTemplateSessionName(t *testing.T) {
	client := &fakeSessionClient{}
	template := templateWithMatch("repo", "Repository", "/tmp/repos/*")
	template.SessionName = "{workspace_name}-dev"
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{template},
	)
	model.sessions = []tmux.Session{{Name: "repo-dev"}}
	selected := candidate{kind: candidateRoot, root: discovery.Candidate{Name: "repo", Path: "/tmp/repos/repo", Mode: "repo"}}

	updated, cmd := model.openCandidate(selected)
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("browse enter returned nil command")
	}
	if msg := cmd(); msg.(createdMsg).err != nil {
		t.Fatalf("createdMsg error = %v", msg.(createdMsg).err)
	}
	if client.templateSessionName != "repo-dev_2" {
		t.Fatalf("template session name = %q, want repo-dev_2", client.templateSessionName)
	}
	if client.templateMetadata.BaseName != "repo-dev" {
		t.Fatalf("template base name = %q, want repo-dev", client.templateMetadata.BaseName)
	}
}

func TestBrowseEnterUsesLocalTemplateAfterNotice(t *testing.T) {
	client := &fakeSessionClient{}
	dir := t.TempDir()
	writeLocalTemplate(t, dir, "local", "Local Workspace")
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{templateWithMatch("global", "Global Workspace", dir)},
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: dir, Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("first enter cmd = %#v, want nil notice pause", cmd)
	}
	if model.notice == nil || !strings.Contains(model.notice.Error(), "local tmux-parator template found") {
		t.Fatalf("notice = %v, want local template notice", model.notice)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("second enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.templateID != "local" {
		t.Fatalf("templateID = %q, want local", client.templateID)
	}
	if model.notice != nil || !model.loading {
		t.Fatalf("notice/loading = %v/%v, want nil/true", model.notice, model.loading)
	}
}

func TestBrowseEnterConfirmsRememberedTemplateBeforeConfiguredMatch(t *testing.T) {
	client := &fakeSessionClient{}
	memory := &fakeTemplateMemory{
		associations: map[string]templatememory.Association{
			"/tmp/repos/repo": {
				TemplateID: "remembered",
				Parameters: map[string]string{"editor": "vim"},
			},
		},
	}
	remembered := testTemplate("remembered", "Remembered Workspace")
	remembered.Source = sessionconfig.SourceGlobal
	remembered.Parameters = []sessionconfig.Parameter{{
		Name:    "editor",
		Prompt:  "Editor",
		Options: []string{"nvim", "vim"},
		Default: "nvim",
	}}
	matched := templateWithMatch("matched", "Matched Workspace", "/tmp/repos/*")
	matched.Source = sessionconfig.SourceGlobal
	model := NewModelWithTemplateMemory(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{remembered, matched},
		memory,
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repos/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil || model.notice == nil || !strings.Contains(model.notice.Error(), `template "Remembered Workspace" is associated`) {
		t.Fatalf("cmd/notice = %#v/%v, want remembered-template notice", cmd, model.notice)
	}
	model.width = 96
	model.height = 24
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "<enter> use template") || !strings.Contains(view, "<esc> dismiss") {
		t.Fatalf("remembered-template notice actions missing:\n%s", view)
	}

	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("remembered template confirmation returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
	if client.templateID != "remembered" || client.templateVariables["editor"] != "vim" {
		t.Fatalf("template = %q variables %#v, want remembered editor vim", client.templateID, client.templateVariables)
	}
}

func TestSuccessfulTemplateCreationRemembersTemplateAndParameters(t *testing.T) {
	client := &fakeSessionClient{}
	memory := &fakeTemplateMemory{}
	template := testTemplate("repo", "Repository")
	template.Source = sessionconfig.SourceGlobal
	model := NewModelWithTemplateMemory(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{template},
		memory,
	)

	_, cmd := model.finishTemplateCreation(
		template,
		"repo",
		"/tmp/repos/repo",
		tmux.SessionMetadata{Kind: "repo"},
		modeBrowse,
		map[string]string{"editor": "nvim"},
	)
	if cmd == nil {
		t.Fatal("finishTemplateCreation returned nil command")
	}
	if msg := cmd(); msg.(createdMsg).err != nil {
		t.Fatalf("createdMsg error = %v", msg.(createdMsg).err)
	}
	if memory.rememberedPath != "/tmp/repos/repo" || memory.rememberedTemplateID != "repo" || memory.rememberedParameters["editor"] != "nvim" {
		t.Fatalf("remembered = %q/%q/%#v", memory.rememberedPath, memory.rememberedTemplateID, memory.rememberedParameters)
	}
}

func TestPathSearchEnterUsesMatchingTemplate(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{templateWithMatch("go", "Go Project", "/tmp/repos/*")},
	)
	model.mode = modePathSearch
	model.pathResult = []candidate{{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "repo", Path: "/tmp/repos/repo"}}}

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.templateSessionName != "repo" || client.templateSessionPath != "/tmp/repos/repo" || client.templateID != "go" {
		t.Fatalf("template create = (%q,%q,%q), want repo /tmp/repos/repo go", client.templateSessionName, client.templateSessionPath, client.templateID)
	}
	if client.newSessionName != "" {
		t.Fatalf("new session fallback used: %#v", client)
	}
	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
}

func TestTemplatePickerIncludesLocalTemplateBeforeGlobalTemplates(t *testing.T) {
	client := &fakeSessionClient{}
	dir := t.TempDir()
	writeLocalTemplate(t, dir, "local", "Local Workspace")
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("global", "Global Workspace")},
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: dir, Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("ctrl-l cmd = %#v, want nil", cmd)
	}
	if model.mode != modeTemplatePicker {
		t.Fatalf("mode = %v, want template picker", model.mode)
	}
	if len(model.templateFiltered) != 3 {
		t.Fatalf("templateFiltered = %#v, want local, global, no-template", model.templateFiltered)
	}
	if model.templateFiltered[0].ID != "local" || model.templateFiltered[0].Source != sessionconfig.SourceLocal {
		t.Fatalf("first template = %#v, want local template", model.templateFiltered[0])
	}
	if model.templateFiltered[1].ID != "global" {
		t.Fatalf("second template = %#v, want global template", model.templateFiltered[1])
	}
	rendered := ansi.Strip(renderTemplatePicker(
		model.styles,
		model.dialogs,
		model.keys,
		model.templateFiltered,
		"",
		0,
		0,
		100,
		40,
	))
	if !strings.Contains(rendered, "LOCAL ─") || !strings.Contains(rendered, "GLOBAL ─") {
		t.Fatalf("template picker missing local/global headings:\n%s", rendered)
	}
}

func TestBrowseEnterSwitchesExistingPathSessionBeforeTemplateMatch(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{templateWithMatch("go", "Go Project", "/tmp/repos/*")},
	)
	model.sessions = []tmux.Session{{Name: "existing", Metadata: tmux.SessionMetadata{Path: "/tmp/repos/repo"}}}
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repos/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	model.cursor = 1

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter returned nil command")
	}
	msg := cmd()
	if switched, ok := msg.(switchedMsg); !ok || switched.err != nil {
		t.Fatalf("cmd msg = %#v, want successful switchedMsg", msg)
	}
	if client.taggedSession != "existing" || client.templateID != "" || client.newSessionName != "" {
		t.Fatalf("client = %#v, want tagged existing only", client)
	}
}

func TestTemplatePickerCreatesSelectedTemplate(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("template enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.templateSessionName != "repo" || client.templateSessionPath != "/tmp/repo" || client.templateID != "zen" {
		t.Fatalf("template create = (%q,%q,%q), want repo /tmp/repo zen", client.templateSessionName, client.templateSessionPath, client.templateID)
	}
	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}
}

func TestTemplatePickerCanCreateWithoutTemplate(t *testing.T) {
	client := &fakeSessionClient{}
	memory := &fakeTemplateMemory{
		associations: map[string]templatememory.Association{
			"/tmp/repo": {
				TemplateID: "zen",
				Parameters: map[string]string{"agent": "codex"},
			},
		},
	}
	model := NewModelWithTemplateMemory(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
		memory,
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("no-template enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.newSessionName != "repo" || client.newSessionPath != "/tmp/repo" {
		t.Fatalf("plain create = (%q,%q), want repo /tmp/repo", client.newSessionName, client.newSessionPath)
	}
	if client.templateID != "" || client.templateSessionName != "" {
		t.Fatalf("template create used: %#v", client)
	}
	if memory.rememberedNoTemplate != "/tmp/repo" {
		t.Fatalf("rememberedNoTemplate = %q, want /tmp/repo", memory.rememberedNoTemplate)
	}
	if model.mode != modeBrowse || !model.loading {
		t.Fatalf("mode/loading = %v/%v, want browse/loading", model.mode, model.loading)
	}

	client = &fakeSessionClient{}
	model.client = client
	model.loading = false
	model.sessions = nil
	updated, cmd = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("enter after remembered no-template returned nil command")
	}
	msg = cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.newSessionName != "repo" || client.newSessionPath != "/tmp/repo" {
		t.Fatalf("plain recreate = (%q,%q), want repo /tmp/repo", client.newSessionName, client.newSessionPath)
	}
	if client.templateID != "" || model.mode == modeTemplateParameter {
		t.Fatalf("remembered no-template reopened template flow: mode %v client %#v", model.mode, client)
	}
}

func TestTemplateHotkeyOpensNoTemplatePickerWhenNoTemplatesConfigured(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModelWithTemplates(
		client,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		nil,
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	if model.mode != modeTemplatePicker || len(model.templateFiltered) != 1 || !isNoTemplatePickerItem(model.templateFiltered[0]) {
		t.Fatalf("template picker state = mode %v items %#v, want no-template picker", model.mode, model.templateFiltered)
	}

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("no-template-only enter returned nil command")
	}
	msg := cmd()
	if created, ok := msg.(createdMsg); !ok || created.err != nil {
		t.Fatalf("cmd msg = %#v, want successful createdMsg", msg)
	}
	if client.newSessionName != "repo" || client.templateID != "" {
		t.Fatalf("client = %#v, want plain repo session", client)
	}
}

func TestTemplatePickerFuzzyFiltersByNameIDAndChip(t *testing.T) {
	repository := testTemplate("repo", "Repository")
	repository.Chip = "rr"
	workspace := testTemplate("notes", "Workspace")
	workspace.Chip = "ww"
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{
			repository,
			workspace,
		},
	)
	model.rootItems = []discovery.Candidate{{Name: "repo", Path: "/tmp/repo", Mode: "repo"}}
	model.rebuildCandidates()
	model.applyFilter()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rep")})
	model = updated.(Model)

	if model.templateFilter != "rep" {
		t.Fatalf("templateFilter = %q, want rep", model.templateFilter)
	}
	if len(model.templateFiltered) != 1 || model.templateFiltered[0].ID != "repo" {
		t.Fatalf("templateFiltered = %#v, want repo only", model.templateFiltered)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("notes")})
	model = updated.(Model)
	if len(model.templateFiltered) != 1 || model.templateFiltered[0].ID != "notes" {
		t.Fatalf("id search templateFiltered = %#v, want notes only", model.templateFiltered)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	model = updated.(Model)
	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ww")})
	model = updated.(Model)
	if len(model.templateFiltered) != 1 || model.templateFiltered[0].ID != "notes" {
		t.Fatalf("chip search templateFiltered = %#v, want notes only", model.templateFiltered)
	}
}

func TestTemplateRowShowsNameWithoutSourceChip(t *testing.T) {
	row := ansi.Strip(renderTemplateRow(testTemplate("repo", "Workspace"), "", false, newStyles(theme.Default()), 40))
	if strings.Contains(row, "repo") {
		t.Fatalf("template row = %q, want template id hidden", row)
	}
	if !strings.Contains(row, "Workspace") {
		t.Fatalf("template row = %q, want workspace name", row)
	}
	if strings.Contains(row, "GLOBAL") || strings.Contains(row, "LOCAL") {
		t.Fatalf("template row = %q, want no source chip", row)
	}
}

func TestTemplateRowShowsConfiguredChipAndFuzzyHighlights(t *testing.T) {
	template := testTemplate("repo", "Repository")
	template.Chip = "rp"
	template.WindowIndicators = []string{" editor", " git"}
	template.Description = "Editor and Git workspace"
	match := templateFuzzyMatch(template, "rp")
	if len(match.AliasIndexes["rp"]) != 2 {
		t.Fatalf("chip match indexes = %#v, want both chip characters", match.AliasIndexes["rp"])
	}
	match = templateFuzzyMatch(template, "sit")
	if len(match.TitleIndexes) != 3 {
		t.Fatalf("title match indexes = %#v, want three highlighted title characters", match.TitleIndexes)
	}
	row := ansi.Strip(renderTemplateRow(template, "rp", false, newStyles(theme.Default()), 100))
	if !strings.Contains(row, "rp") || strings.Index(row, "rp") > strings.Index(row, "Repository") {
		t.Fatalf("template row = %q, want chip before template name", row)
	}
	if strings.Index(row, "Repository") >= strings.Index(row, " editor") {
		t.Fatalf("template row columns are not chip, name, window indicators: %q", row)
	}
	if !strings.Contains(row, " editor ·  git") {
		t.Fatalf("template row does not separate window indicators: %q", row)
	}
	if strings.Contains(row, template.Description) {
		t.Fatalf("template row contains inline description: %q", row)
	}
	descriptionMatch := templateFuzzyMatch(template, "workspace")
	if len(descriptionMatch.FieldIndexes["description"]) == 0 {
		t.Fatalf("description match indexes missing: %#v", descriptionMatch.FieldIndexes)
	}
	indicatorMatch := templateFuzzyMatch(template, "editor")
	if len(indicatorMatch.FieldIndexes["window_indicators"]) == 0 {
		t.Fatalf("window indicator match indexes missing: %#v", indicatorMatch.FieldIndexes)
	}
}

func TestSelectedTemplateChipIsAdjacentToSelectionMarker(t *testing.T) {
	template := testTemplate("repo", "Repository")
	template.Chip = "rp"
	styles := newStyles(theme.Default())

	row := ansi.Strip(
		renderPopupSelectionMarker(true, styles) +
			renderTemplateRow(template, "", true, styles, 80),
	)
	if !strings.HasPrefix(row, "▌ rp ") {
		t.Fatalf("selected template row = %q, want chip adjacent to selection marker", row)
	}
	renderedChip := renderPopupChip("rp", styles.selected, styles.selectedMatch, nil)
	if !strings.Contains(renderTemplateRow(template, "", true, styles, 80), renderedChip) {
		t.Fatal("selected template chip does not use the selection bar style")
	}
}

func TestCommandAndHelpHotkeysUseSelectionBarChips(t *testing.T) {
	styles := newStyles(theme.Default())

	commandKey := "<enter>"
	commandRow := renderCommandRow(commandMatch{
		item: commandItem{Title: "Open", Key: commandKey, Enabled: true},
	}, true, styles, 80)
	selectedCommandChip := renderPopupChip(commandKey, styles.selected, styles.selectedMatch, nil)
	if !strings.Contains(commandRow, selectedCommandChip) {
		t.Fatal("selected command hotkey does not use the selection bar style")
	}
	if got := ansi.Strip(renderPopupSelectionMarker(true, styles) + commandRow); !strings.HasPrefix(got, "▌ <enter> ") {
		t.Fatalf("selected command row = %q, want hotkey chip adjacent to selection marker", got)
	}

	helpKey := "<ctrl-?>"
	helpRow := renderHelpRow(helpMatch{
		item: helpItem{Key: helpKey, Action: "show help"},
	}, true, styles, 80)
	selectedHelpChip := renderPopupChip(helpKey, styles.selected, styles.selectedMatch, nil)
	if !strings.Contains(helpRow, selectedHelpChip) {
		t.Fatal("selected help hotkey does not use the selection bar style")
	}
	if got := ansi.Strip(renderPopupSelectionMarker(true, styles) + helpRow); !strings.HasPrefix(got, "▌ <ctrl-?> ") {
		t.Fatalf("selected help row = %q, want hotkey chip adjacent to selection marker", got)
	}
}

func TestCommandAndHelpHotkeysUseChipStyleWhenNotSelected(t *testing.T) {
	styles := newStyles(theme.Default())

	commandKey := "<enter>"
	commandRow := renderCommandRow(commandMatch{
		item: commandItem{Title: "Open", Key: commandKey, Enabled: true},
	}, false, styles, 80)
	commandChip := renderPopupChip(
		commandKey,
		styles.chip,
		styles.match.Copy().Background(styles.chip.GetBackground()),
		nil,
	)
	if !strings.Contains(commandRow, commandChip) {
		t.Fatal("command hotkey does not use the chip style")
	}

	helpKey := "<ctrl-?>"
	helpRow := renderHelpRow(helpMatch{
		item: helpItem{Key: helpKey, Action: "show help"},
	}, false, styles, 80)
	helpChip := renderPopupChip(
		helpKey,
		styles.chip,
		styles.match.Copy().Background(styles.chip.GetBackground()),
		nil,
	)
	if !strings.Contains(helpRow, helpChip) {
		t.Fatal("help hotkey does not use the chip style")
	}
}

func TestCommandAndHelpHotkeyChipsShowCombinedBindingsInFull(t *testing.T) {
	styles := newStyles(theme.Default())

	commandKey := "<esc> / <ctrl-g>"
	commandRow := ansi.Strip(renderCommandRow(commandMatch{
		item: commandItem{Title: "Close commands", Key: commandKey, Enabled: true},
	}, false, styles, 80))
	if !strings.Contains(commandRow, commandKey) {
		t.Fatalf("command row = %q, want full hotkey %q", commandRow, commandKey)
	}

	helpKey := "<esc> / <ctrl-?>"
	helpRow := ansi.Strip(renderHelpRow(helpMatch{
		item: helpItem{Key: helpKey, Action: "close help"},
	}, false, styles, 80))
	if !strings.Contains(helpRow, helpKey) {
		t.Fatalf("help row = %q, want full hotkey %q", helpRow, helpKey)
	}
}

func TestCommandAndHelpPopupsShowHelpHotkey(t *testing.T) {
	styles := newStyles(theme.Default())
	dialogs := config.Dialogs{Panel: config.DialogSize{Width: 88, Height: 0}}
	keys := config.Default().UI.Keys
	const helpKey = "<ctrl-?>"
	commandMatches := make([]commandMatch, 0, len(browseCommandSpecsFor(keys)))
	for _, spec := range browseCommandSpecsFor(keys) {
		commandMatches = append(commandMatches, commandMatch{item: commandItemFromSpec(spec, true, "")})
	}

	for _, test := range []struct {
		name  string
		panel string
	}{
		{
			name: "browse commands",
			panel: renderCommandPalette(
				styles,
				dialogs,
				commandMatches,
				"",
				0,
				0,
				120,
				40,
			),
		},
		{
			name:  "browse help",
			panel: renderHelpPanelWithKeys(styles, dialogs, keys, modeBrowse, "", 0, 0, 120, 40),
		},
		{
			name:  "command help",
			panel: renderHelpPanelWithKeys(styles, dialogs, keys, modeCommands, "", 0, 0, 120, 40),
		},
		{
			name:  "path help",
			panel: renderHelpPanelWithKeys(styles, dialogs, keys, modePathSearch, "", 0, 0, 120, 40),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(ansi.Strip(test.panel), helpKey) {
				t.Fatalf("popup does not show help hotkey %q:\n%s", helpKey, ansi.Strip(test.panel))
			}
		})
	}
}

func TestCommandAndHelpRowsShowConfiguredHotkeysInFull(t *testing.T) {
	styles := newStyles(theme.Default())
	keys := config.Default().UI.Keys

	for _, spec := range append(browseCommandSpecsFor(keys), pathSearchCommandSpecsFor(keys)...) {
		row := ansi.Strip(renderCommandRow(commandMatch{
			item: commandItemFromSpec(spec, true, ""),
		}, false, styles, 120))
		if !strings.Contains(row, spec.Key) {
			t.Fatalf("command row = %q, want full hotkey %q", row, spec.Key)
		}
	}

	for _, currentMode := range []mode{modeBrowse, modeCommands, modePathSearch} {
		for _, item := range helpItemsForModeWithKeys(currentMode, keys) {
			row := ansi.Strip(renderHelpRow(helpMatch{item: item}, false, styles, 120))
			if !strings.Contains(row, item.Key) {
				t.Fatalf("help row for mode %v = %q, want full hotkey %q", currentMode, row, item.Key)
			}
		}
	}
}

func TestNoTemplatePickerItemUsesNTChip(t *testing.T) {
	template := noTemplatePickerItem()
	if template.Chip != "nt" {
		t.Fatalf("no-template chip = %q, want nt", template.Chip)
	}
	row := ansi.Strip(renderTemplateRow(template, "", false, newStyles(theme.Default()), 80))
	if !strings.Contains(row, "nt") || !strings.Contains(row, noTemplatePickerName) {
		t.Fatalf("no-template row missing chip or name: %q", row)
	}
	if strings.Contains(row, "Create a normal tmux session") {
		t.Fatalf("no-template row contains inline description: %q", row)
	}
}

func TestTemplatePickerGroupsIndentedLocalAndGlobalTemplates(t *testing.T) {
	local := testTemplate("local", "Local Workspace")
	local.Source = sessionconfig.SourceLocal
	global := testTemplate("global", "Global Workspace")
	global.Source = sessionconfig.SourceGlobal

	rendered := ansi.Strip(renderTemplatePicker(
		newStyles(theme.Default()),
		config.Default().UI.Dialogs,
		config.Default().UI.Keys,
		[]sessionconfig.Template{local, global, noTemplatePickerItem()},
		"",
		0,
		0,
		100,
		40,
	))
	localHeadingIndex := strings.Index(rendered, "LOCAL ─")
	localIndex := strings.Index(rendered, "Local Workspace")
	globalHeadingIndex := strings.Index(rendered, "GLOBAL ─")
	globalIndex := strings.Index(rendered, "Global Workspace")
	if localHeadingIndex < 0 || localIndex < 0 || globalHeadingIndex < 0 || globalIndex < 0 {
		t.Fatalf("template picker missing local/global sections:\n%s", rendered)
	}
	if !(localHeadingIndex < localIndex && localIndex < globalHeadingIndex && globalHeadingIndex < globalIndex) {
		t.Fatalf("template picker order is local heading, local result, global heading, global result:\n%s", rendered)
	}
	headingIndent := -1
	localIndented := false
	globalIndented := false
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "LOCAL ─") || strings.Contains(line, "GLOBAL ─") {
			indent := strings.Index(line, "LOCAL ─")
			if indent < 0 {
				indent = strings.Index(line, "GLOBAL ─")
			}
			if headingIndent < 0 {
				headingIndent = indent
			} else if indent != headingIndent {
				t.Fatalf("template headings have different indentation: %q", line)
			}
		}
		if headingIndent >= 0 && strings.Index(line, "Local Workspace") >= headingIndent+2 {
			localIndented = true
		}
		if headingIndent >= 0 && strings.Index(line, "Global Workspace") >= headingIndent+2 {
			globalIndented = true
		}
	}
	if headingIndent < 0 || !localIndented || !globalIndented {
		t.Fatalf("template results are not indented beneath their headings:\n%s", rendered)
	}
}

func TestPopupRowsAlignWithSearchInterior(t *testing.T) {
	styles := newStyles(theme.Default())
	dialogs := config.Dialogs{
		Panel: config.DialogSize{Width: 70, Height: 20},
	}
	commandPanel := renderCommandPalette(
		styles,
		dialogs,
		[]commandMatch{{item: commandItem{Title: "Open", Key: "<enter>", Enabled: true}}},
		"",
		0,
		0,
		120,
		24,
	)
	templatePanel := renderTemplatePicker(
		styles,
		dialogs,
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("repo", "Repository")},
		"",
		0,
		0,
		120,
		24,
	)
	helpPanel := renderHelpPanel(
		styles,
		dialogs,
		modeBrowse,
		0,
		0,
		120,
		24,
	)

	commandSearch, commandRow := panelSearchAndResultLines(t, commandPanel, "Open")
	templateSearch, templateRow := panelSearchAndResultLines(t, templatePanel, "Repository")
	helpSearch, helpRow := panelSearchAndResultLines(t, helpPanel, "filter sessions")
	if lipgloss.Width(commandSearch) != lipgloss.Width(templateSearch) {
		t.Fatalf("search widths differ: command=%d template=%d", lipgloss.Width(commandSearch), lipgloss.Width(templateSearch))
	}
	if lipgloss.Width(commandSearch) != lipgloss.Width(helpSearch) {
		t.Fatalf("search widths differ: command=%d help=%d", lipgloss.Width(commandSearch), lipgloss.Width(helpSearch))
	}
	if lipgloss.Width(commandRow) != lipgloss.Width(templateRow) {
		t.Fatalf("result row widths differ: command=%d template=%d", lipgloss.Width(commandRow), lipgloss.Width(templateRow))
	}
	if lipgloss.Width(commandRow) != lipgloss.Width(helpRow) {
		t.Fatalf("result row widths differ: command=%d help=%d", lipgloss.Width(commandRow), lipgloss.Width(helpRow))
	}
	if got, want := lipgloss.Width(commandRow), lipgloss.Width(commandSearch); got != want {
		t.Fatalf("command result width = %d, want search width %d", got, want)
	}
	if got, want := lipgloss.Width(templateRow), lipgloss.Width(templateSearch); got != want {
		t.Fatalf("template result width = %d, want search width %d", got, want)
	}
	if got, want := lipgloss.Width(helpRow), lipgloss.Width(helpSearch); got != want {
		t.Fatalf("help result width = %d, want search width %d", got, want)
	}
}

func TestTemplateSectionDividerAlignsWithSearchOuterBox(t *testing.T) {
	panel := renderTemplatePicker(
		newStyles(theme.Default()),
		config.Dialogs{Panel: config.DialogSize{Width: 70, Height: 20}},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("repo", "Repository")},
		"",
		0,
		0,
		120,
		24,
	)

	searchLine, _ := panelSearchAndResultLines(t, panel, "Repository")
	var dividerLine string
	for _, line := range strings.Split(panel, "\n") {
		if strings.Contains(ansi.Strip(line), "GLOBAL ─") {
			dividerLine = line
			break
		}
	}
	if dividerLine == "" {
		t.Fatalf("template panel missing divider:\n%s", ansi.Strip(panel))
	}
	if got, want := lipgloss.Width(dividerLine), lipgloss.Width(searchLine); got != want {
		t.Fatalf("divider width = %d, want search outer width %d", got, want)
	}
}

func TestPopupsShowSelectionMarker(t *testing.T) {
	styles := newStyles(theme.Default())
	dialogs := config.Dialogs{Panel: config.DialogSize{Width: 70, Height: 20}}
	commandPanel := renderCommandPalette(
		styles,
		dialogs,
		[]commandMatch{{item: commandItem{Title: "Open", Key: "<enter>", Enabled: true}}},
		"",
		0,
		0,
		120,
		24,
	)
	templatePanel := renderTemplatePicker(
		styles,
		dialogs,
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("repo", "Repository")},
		"",
		0,
		0,
		120,
		24,
	)
	helpPanel := renderHelpPanel(styles, dialogs, modeBrowse, 0, 0, 120, 24)

	_, commandRow := panelSearchAndResultLines(t, commandPanel, "Open")
	_, templateRow := panelSearchAndResultLines(t, templatePanel, "Repository")
	_, helpRow := panelSearchAndResultLines(t, helpPanel, "filter sessions")
	if !strings.Contains(ansi.Strip(commandRow), "▌") {
		t.Fatalf("command selection row missing marker: %q", ansi.Strip(commandRow))
	}
	if !strings.Contains(ansi.Strip(templateRow), "▌") {
		t.Fatalf("template selection row missing marker: %q", ansi.Strip(templateRow))
	}
	if !strings.Contains(ansi.Strip(helpRow), "▌") {
		t.Fatalf("help selection row missing marker: %q", ansi.Strip(helpRow))
	}
}

func panelSearchAndResultLines(t *testing.T, panel string, resultText string) (string, string) {
	t.Helper()
	var searchLine string
	var resultLine string
	for _, line := range strings.Split(panel, "\n") {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "❯") {
			searchLine = line
		}
		if resultLine == "" && strings.Contains(stripped, resultText) {
			resultLine = line
		}
	}
	if searchLine == "" || resultLine == "" {
		t.Fatalf("panel missing search or result line:\n%s", ansi.Strip(panel))
	}
	return searchLine, resultLine
}

func TestTemplatePickerTabJumpsBetweenSections(t *testing.T) {
	local := testTemplate("local", "Local Workspace")
	local.Source = sessionconfig.SourceLocal
	global := testTemplate("global", "Global Workspace")
	global.Source = sessionconfig.SourceGlobal
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{local, global},
	)
	model.mode = modeTemplatePicker
	model.templateFiltered = templatePickerItems([]sessionconfig.Template{local, global})

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.templateCursor != 1 {
		t.Fatalf("template cursor after tab = %d, want first global index 1", model.templateCursor)
	}

	updated, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(Model)
	if model.templateCursor != 0 {
		t.Fatalf("template cursor after shift-tab = %d, want first local index 0", model.templateCursor)
	}
}

func TestRenderTemplateSectionHeaderHighlightsActiveSection(t *testing.T) {
	styles := newStyles(theme.Default())
	active := renderTemplateSectionHeader(sessionconfig.SourceLocal, true, styles, 40)
	inactive := renderTemplateSectionHeader(sessionconfig.SourceLocal, false, styles, 40)
	if ansi.Strip(active) != ansi.Strip(inactive) {
		t.Fatalf("active header text = %q, want inactive header text %q", ansi.Strip(active), ansi.Strip(inactive))
	}
	activeLabel, activeLine := templateSectionStyles(true, styles)
	inactiveLabel, inactiveLine := templateSectionStyles(false, styles)
	if activeLabel.GetForeground() != styles.popupAccent.GetForeground() || activeLine.GetForeground() != styles.filterLabel.GetForeground() {
		t.Fatal("active template section does not use theme accent styles")
	}
	if inactiveLabel.GetForeground() != styles.muted.GetForeground() || inactiveLine.GetForeground() != styles.muted.GetForeground() {
		t.Fatal("inactive template section does not use muted styles")
	}
}

func TestTemplatePickerCancelReturnsToBrowse(t *testing.T) {
	model := NewModelWithTemplates(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{},
		config.Default().UI.Keys,
		[]sessionconfig.Template{testTemplate("zen", "Zen")},
	)
	model.mode = modeTemplatePicker
	model.templatePath = "/tmp/repo"

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if model.mode != modeBrowse || model.templatePath != "" {
		t.Fatalf("mode/templatePath = %v/%q, want browse empty", model.mode, model.templatePath)
	}
}

func TestPathSearchPathColumnPrefersBasenameHighlights(t *testing.T) {
	item := candidate{
		kind:         candidatePath,
		fsPath:       pathsearch.Candidate{Name: "csedm", Path: "/tmp/code/somewhere/csedm"},
		matchIndexes: []int{0, 1, 2, 3, 4},
		fieldIndexes: map[string][]int{
			fieldPath: {5, 6, 9, 12, 25},
		},
	}

	indexes := pathTitleDetailIndexes(item, item.detail())
	want := []int{20, 21, 22, 23, 24}
	if !slices.Equal(indexes, want) {
		t.Fatalf("path title detail indexes = %#v, want %#v", indexes, want)
	}
}

func TestPathSearchPathColumnMergesBasenameAndPathHighlights(t *testing.T) {
	item := candidate{
		kind:         candidatePath,
		fsPath:       pathsearch.Candidate{Name: "csedm", Path: "/Users/stefan/data/csedm"},
		matchIndexes: []int{0, 1, 2, 3, 4},
		fieldIndexes: map[string][]int{
			fieldPath: {14, 15, 16, 17},
		},
	}

	indexes := detailMatchIndexes(item, item.detail())
	want := []int{14, 15, 16, 17, 19, 20, 21, 22, 23}
	if !slices.Equal(indexes, want) {
		t.Fatalf("detail match indexes = %#v, want %#v", indexes, want)
	}
}

func TestSelectedCandidateRowPadsToWidth(t *testing.T) {
	row := renderCandidateRow(candidate{kind: candidateSession, session: tmux.Session{Name: "main"}}, true, newStyles(theme.Default()), 40, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	if got := lipgloss.Width(row); got != 40 {
		t.Fatalf("row width = %d, want 40", got)
	}
}

func TestSelectedCandidateRowDoesNotShiftContent(t *testing.T) {
	item := candidate{kind: candidateSession, session: tmux.Session{Name: "main", Metadata: tmux.SessionMetadata{Path: "/tmp/main"}}}
	styles := newStyles(theme.Default())

	selected := ansi.Strip(renderCandidateRow(item, true, styles, 80, config.Glyphs{}, config.GlyphColors{}, config.Columns{}))
	unselected := ansi.Strip(renderCandidateRow(item, false, styles, 80, config.Glyphs{}, config.GlyphColors{}, config.Columns{}))

	selectedNameIndex := renderedColumn(selected, "main")
	unselectedNameIndex := renderedColumn(unselected, "main")
	if selectedNameIndex < 0 || unselectedNameIndex < 0 {
		t.Fatalf("missing candidate name:\nselected: %q\nunselected: %q", selected, unselected)
	}
	if selectedNameIndex != unselectedNameIndex {
		t.Fatalf("selected name index = %d, want unselected name index %d:\nselected: %q\nunselected: %q", selectedNameIndex, unselectedNameIndex, selected, unselected)
	}
}

func TestUnselectedCandidateRowClipsLongMusicPathToWidth(t *testing.T) {
	item := candidate{
		kind: candidatePath,
		fsPath: pathsearch.Candidate{
			Name: "06 Happy Holidays (Beef Wellington Remix).movpkg",
			Path: "/Users/stefanschmerda/Music/Music/Media.localized/Music/Compilations/Surviving Christmas (Original Motion Picture Soundtrack)/06 Happy Holidays (Beef Wellington Remix).movpkg/0-1026321-UIAIWLMGCUM7BK3ZAPSLJHKR42PWO4L5",
		},
	}
	row := renderCandidateRow(item, false, newStyles(theme.Default()), 80, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	if got := lipgloss.Width(row); got > 80 {
		t.Fatalf("row width = %d, want <= 80:\n%s", got, ansi.Strip(row))
	}
}

func TestLongPathColumnUsesEllipsisAndFitsWidth(t *testing.T) {
	item := candidate{
		kind: candidatePath,
		fsPath: pathsearch.Candidate{
			Name: "deep",
			Path: "/Users/stefanschmerda/Music/Music/Media.localized/Music/Compilations/Surviving Christmas (Original Motion Picture Soundtrack)/06 Happy Holidays (Beef Wellington Remix).movpkg/Data",
		},
	}
	row := renderCandidateRow(item, false, newStyles(theme.Default()), 80, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	stripped := ansi.Strip(row)
	if got := lipgloss.Width(row); got > 80 {
		t.Fatalf("row width = %d, want <= 80:\n%s", got, stripped)
	}
	if !strings.Contains(stripped, "...") {
		t.Fatalf("row missing ellipsis:\n%s", stripped)
	}
}

func TestPathSearchErrorDoesNotCoverPromptWithModal(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch
	model.width = 100
	model.height = 20
	model.pathInput = "~/Music/"
	model.pathErr = errors.New("permission denied")

	view := model.View()
	if !strings.Contains(view, "path") || !strings.Contains(view, "Music") {
		t.Fatalf("path prompt missing from error view:\n%s", view)
	}
	if strings.Contains(view, " Error ") {
		t.Fatalf("path search error rendered modal popup:\n%s", view)
	}
	if !strings.Contains(view, "error: permission denied") {
		t.Fatalf("path error missing from footer:\n%s", view)
	}
}

func TestCandidateRowShowsFixedRootColumn(t *testing.T) {
	item := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName: "repositories-extra",
			Name:     "tmux-parator",
			Mode:     "repo",
			Glyph:    "R",
		},
	}
	row := renderCandidateRow(item, false, newStyles(theme.Default()), 80, config.Glyphs{}, config.GlyphColors{}, config.Columns{})

	if !strings.Contains(row, "R") {
		t.Fatalf("row missing root glyph override:\n%s", row)
	}
	if !strings.Contains(row, "repositor...") {
		t.Fatalf("row missing truncated root column:\n%s", row)
	}
}

func TestCandidateRowAlignsDetailsAfterFixedNameColumn(t *testing.T) {
	styles := newStyles(theme.Default())
	short := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName:    "repos",
			Name:        "api",
			DisplayPath: "repos/api-path",
			Mode:        "repo",
		},
	}
	long := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName:    "repos",
			Name:        "very-long-candidate-name-that-overflows",
			DisplayPath: "repos/very-long-path",
			Mode:        "repo",
		},
	}

	shortRow := ansi.Strip(renderCandidateRow(short, false, styles, 120, config.Glyphs{}, config.GlyphColors{}, config.Columns{}))
	longRow := ansi.Strip(renderCandidateRow(long, false, styles, 120, config.Glyphs{}, config.GlyphColors{}, config.Columns{}))

	shortDetailIndex := strings.Index(shortRow, "api-path")
	longDetailIndex := strings.Index(longRow, "very-long-path")
	if shortDetailIndex < 0 || longDetailIndex < 0 {
		t.Fatalf("detail not found:\nshort=%q\nlong=%q", shortRow, longRow)
	}
	shortDetail := lipgloss.Width(shortRow[:shortDetailIndex])
	longDetail := lipgloss.Width(longRow[:longDetailIndex])
	if shortDetail != longDetail {
		t.Fatalf("detail columns = %d/%d, want aligned:\nshort=%q\nlong=%q", shortDetail, longDetail, shortRow, longRow)
	}
}

func TestRenderNameColumnHasFixedWidth(t *testing.T) {
	rendered := renderNameColumn("very-long-candidate-name-that-overflows", newStyles(theme.Default()).session, newStyles(theme.Default()).match, nil, nameColumnWidth)
	if got := lipgloss.Width(rendered); got != nameColumnWidth {
		t.Fatalf("name column width = %d, want %d", got, nameColumnWidth)
	}
	if !strings.Contains(ansi.Strip(rendered), "…") {
		t.Fatalf("name column did not truncate with ellipsis: %q", ansi.Strip(rendered))
	}
}

func TestCandidateRowRespectsColumnVisibilityAndWidths(t *testing.T) {
	item := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName:    "repos",
			Name:        "tmux-parator",
			DisplayPath: "repos/tmux-parator",
			Mode:        "repo",
		},
	}
	columns := config.Columns{
		Chip: config.Column{Show: false, Width: 12},
		Root: config.Column{Show: true, Width: 6},
		Name: config.Column{Show: true, Width: 10},
		Path: config.Column{Show: false, Width: 0},
	}
	row := ansi.Strip(renderCandidateRow(item, false, newStyles(theme.Default()), 80, config.Glyphs{}, config.GlyphColors{}, columns))

	if strings.Contains(row, " repo ") {
		t.Fatalf("row contains hidden chip label:\n%s", row)
	}
	if strings.Contains(row, "repos/tmux-parator") {
		t.Fatalf("row contains hidden path column:\n%s", row)
	}
	if !strings.Contains(row, "tmux-para…") {
		t.Fatalf("row missing truncated custom-width name column:\n%s", row)
	}
}

func TestCandidateRowCanIncludeRootPrefixInPathColumn(t *testing.T) {
	item := candidate{
		kind: candidateRoot,
		root: discovery.Candidate{
			RootName:     "repos",
			Name:         "tmux-parator",
			RelativePath: "tmux-parator",
			DisplayPath:  "repos/tmux-parator",
			Mode:         "repo",
		},
		pathDetail: "repos/tmux-parator",
	}
	row := ansi.Strip(renderCandidateRow(item, false, newStyles(theme.Default()), 80, config.Glyphs{}, config.GlyphColors{}, config.Columns{Path: config.Column{Show: true, IncludeRoot: true}}))

	if !strings.Contains(row, "repos/tmux-parator") {
		t.Fatalf("row missing root-prefixed path column:\n%s", row)
	}
}

func TestMainListAvailableWorkspacesUseRootPrefixedPaths(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{
		Path: config.Column{Show: true, IncludeRoot: false},
	})
	model.rootItems = []discovery.Candidate{{
		RootName:     "repos",
		Name:         "tmux-parator",
		Path:         "/tmp/repos/tmux-parator",
		RelativePath: "tmux-parator",
		DisplayPath:  "repos/tmux-parator",
		Mode:         "repo",
	}}

	model.rebuildCandidates()

	if len(model.candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(model.candidates))
	}
	if got := model.candidates[0].detail(); got != "repos/tmux-parator" {
		t.Fatalf("candidate detail = %q, want root-prefixed path", got)
	}
}

func TestRenderColumnsAutoSizesRootAndName(t *testing.T) {
	model := NewModel(
		nil,
		theme.Default(),
		nil,
		discovery.Options{},
		config.PathSearch{},
		config.Glyphs{},
		config.GlyphColors{},
		config.Columns{
			Chip: config.Column{Show: true, Width: 0, MaxWidth: 12},
			Root: config.Column{Show: true, Width: 0, MaxWidth: 6},
			Name: config.Column{Show: true, Width: 0, MaxWidth: 12},
			Path: config.Column{Show: true, Width: 0},
		},
	)
	items := []candidate{
		{kind: candidateRoot, root: discovery.Candidate{RootName: "repos", Name: "api"}},
		{kind: candidateRoot, root: discovery.Candidate{RootName: "long-root-name", Name: "long-service-name"}},
	}

	columns := model.renderColumns(items)
	if columns.Chip.Width != originChipWidth() {
		t.Fatalf("chip width = %d, want default %d", columns.Chip.Width, originChipWidth())
	}
	if columns.Root.Width != 6 {
		t.Fatalf("root width = %d, want capped auto width 6", columns.Root.Width)
	}
	if columns.Name.Width != 12 {
		t.Fatalf("name width = %d, want capped auto width 12", columns.Name.Width)
	}
	if columns.Path.Width != 0 {
		t.Fatalf("path width = %d, want flexible 0", columns.Path.Width)
	}
}

func TestSmallDialogUsesConfiguredFrameSize(t *testing.T) {
	dialogs := config.Dialogs{
		Small: config.DialogSize{Width: 54, Height: 11},
		Panel: config.DialogSize{Width: 88, Height: 0},
	}
	rendered := renderConfirmKill(newStyles(theme.Default()), dialogs, "api", confirmCancel, 120, 40)
	lines := strings.Split(rendered, "\n")

	if got := lipgloss.Width(lines[0]); got != 54 {
		t.Fatalf("small dialog width = %d, want 54:\n%s", got, rendered)
	}
	if got := len(lines); got != 11 {
		t.Fatalf("small dialog height = %d, want 11:\n%s", got, rendered)
	}
}

func TestSmallDialogShrinksOnNarrowTerminal(t *testing.T) {
	dialogs := config.Dialogs{
		Small: config.DialogSize{Width: 72, Height: 9},
		Panel: config.DialogSize{Width: 88, Height: 0},
	}
	rendered := renderPathNoticePopup(newStyles(theme.Default()), dialogs, "path already exists", 30, 20)
	lines := strings.Split(rendered, "\n")

	if got := lipgloss.Width(lines[0]); got != 26 {
		t.Fatalf("small dialog width = %d, want shrink-to-fit width 26:\n%s", got, rendered)
	}
}

func TestPanelDialogsShareConfiguredWidthAndAutoHeight(t *testing.T) {
	dialogs := config.Dialogs{
		Small: config.DialogSize{Width: 72, Height: 9},
		Panel: config.DialogSize{Width: 70, Height: 0},
	}
	matches := []commandMatch{{
		item: commandItem{
			ID:          commandHelp,
			Title:       "Show help",
			Key:         "<ctrl-?>",
			Description: "Show help for the command palette.",
			Enabled:     true,
		},
	}}
	commandPanel := renderCommandPalette(newStyles(theme.Default()), dialogs, matches, "", 0, 0, 120, 24)
	helpPanel := renderHelpPanel(newStyles(theme.Default()), dialogs, modeBrowse, 0, 0, 120, 24)

	if got := lipgloss.Width(strings.Split(commandPanel, "\n")[0]); got != 70 {
		t.Fatalf("command panel width = %d, want 70:\n%s", got, commandPanel)
	}
	if got := lipgloss.Width(strings.Split(helpPanel, "\n")[0]); got != 70 {
		t.Fatalf("help panel width = %d, want 70:\n%s", got, helpPanel)
	}

	explicitAutoDialogs := config.Dialogs{
		Small: config.DialogSize{Width: 72, Height: 9},
		Panel: config.DialogSize{Width: 70, Height: 20},
	}
	explicitAutoPanel := renderHelpPanel(newStyles(theme.Default()), explicitAutoDialogs, modeBrowse, 0, 0, 120, 24)
	if got, want := len(strings.Split(helpPanel, "\n")), len(strings.Split(explicitAutoPanel, "\n")); got != want {
		t.Fatalf("auto panel rendered height = %d, want explicit auto-equivalent height %d", got, want)
	}
}

func TestTruncateDots(t *testing.T) {
	if got := truncateDots("repositories-extra", rootColumnWidth); got != "repositor..." {
		t.Fatalf("truncateDots() = %q, want repositor...", got)
	}
	if got := truncateDots("repos", rootColumnWidth); got != "repos" {
		t.Fatalf("truncateDots() = %q, want repos", got)
	}
}
