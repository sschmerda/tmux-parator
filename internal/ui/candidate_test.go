package ui

import (
	"context"
	"errors"
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
	if model.cursor != 0 {
		t.Fatalf("cursor after shift-tab = %d, want first session index 0", model.cursor)
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
	for _, want := range []string{"Show hidden configured paths", "Show gitignored configured paths", "Quit"} {
		if !titles[want] {
			t.Fatalf("command %q missing from main palette: %#v", want, titles)
		}
	}

	model.openCommands(modePathSearch)
	titles = commandTitles(model.commandItems())
	for _, want := range []string{"Show hidden path results", "Show gitignored path results", "Quit"} {
		if !titles[want] {
			t.Fatalf("command %q missing from path palette: %#v", want, titles)
		}
	}
}

func TestCommandPaletteIncludesOpenLastOnlyInBrowse(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{}, config.PathSearch{Enabled: true}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.openCommands(modeBrowse)

	item, ok := commandByID(model.commandItems(), commandOpenLast)
	if !ok {
		t.Fatal("open last session command missing from browse palette")
	}
	if item.Title != "Open last session" || item.Key != "<c-`>" || !item.Enabled {
		t.Fatalf("open last session command = %#v, want title/key/enabled", item)
	}

	model.openCommands(modePathSearch)
	if _, ok := commandByID(model.commandItems(), commandOpenLast); ok {
		t.Fatal("open last session command present in path-search palette")
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
	if !strings.Contains(view, "HIDDEN SKIP") || !strings.Contains(view, "IGNORED SHOW") {
		t.Fatalf("view missing status chips:\n%s", view)
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
	if !strings.HasPrefix(lines[1], "│    ╭") {
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
		if strings.Contains(line, "open sessions") {
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
	kindColumnStart := kindIndex - renderedColumn(renderHeaderColumn("kind", lipgloss.NewStyle(), normalizeUIColumns(config.Columns{}).Chip.Width, lipgloss.Left), "kind")
	footerColumnStart := renderedColumn(footer, "PATH OFF") - ((statusChipWidth - lipgloss.Width("PATH OFF")) / 2)
	if footerColumnStart != kindColumnStart {
		t.Fatalf("footer column start = %d, want kind column start %d: %q", footerColumnStart, kindColumnStart, footer)
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
	if !strings.HasSuffix(section, "    │") {
		t.Fatalf("section divider missing one-cell-longer right inset: %q", section)
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
	sectionTextIndex := renderedColumn(section, "open sessions")
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
	if strings.Contains(ansi.Strip(active), "> open sessions") {
		t.Fatalf("active header should not include cursor marker: %q", ansi.Strip(active))
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

func TestPathSearchUsesBrowseColumnHeader(t *testing.T) {
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
	if !strings.Contains(view, "paths") {
		t.Fatalf("path search view missing paths divider:\n%s", view)
	}
	if strings.Contains(view, "open sessions") {
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

func commandByID(items []commandItem, id commandID) (commandItem, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return commandItem{}, false
}

type fakeSessionClient struct {
	switchLastCalls    int
	killCalls          int
	killedSession      string
	switchedSession    string
	newSessionName     string
	newSessionPath     string
	newSessionMetadata tmux.SessionMetadata
	taggedSession      string
	taggedMetadata     tmux.SessionMetadata
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

func (f *fakeSessionClient) NewSession(_ context.Context, name string, path string, metadata tmux.SessionMetadata) error {
	f.newSessionName = name
	f.newSessionPath = path
	f.newSessionMetadata = metadata
	return nil
}

func (f *fakeSessionClient) TagSession(_ context.Context, session string, metadata tmux.SessionMetadata) error {
	f.taggedSession = session
	f.taggedMetadata = metadata
	return nil
}

func TestPathSearchHelpReturnsToPathSearch(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modePathSearch

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
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

func TestPathSearchHelpRendersModeSpecificContent(t *testing.T) {
	model := NewModel(nil, theme.Default(), nil, discovery.Options{SkipHidden: true}, config.PathSearch{}, config.Glyphs{}, config.GlyphColors{}, config.Columns{})
	model.mode = modeHelp
	model.previousMode = modePathSearch
	model.width = 80
	model.height = 24

	view := model.View()
	if !strings.Contains(view, "Path Search") || !strings.Contains(view, "<c-p>") {
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

func TestOpenCandidateCreatesDuplicateWithNumberedLeafName(t *testing.T) {
	client := &fakeSessionClient{}
	model := NewModel(client, theme.Default(), nil, discovery.Options{}, config.PathSearch{}, config.Glyphs{Manual: "M"}, config.GlyphColors{}, config.Columns{})
	model.sessions = []tmux.Session{{Name: "data"}}
	selected := candidate{kind: candidatePath, fsPath: pathsearch.Candidate{Name: "data", Path: "/tmp/project/data"}}

	cmd := model.openCandidate(selected)
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

	cmd := model.openCandidate(selected)
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

func TestTruncateDots(t *testing.T) {
	if got := truncateDots("repositories-extra", rootColumnWidth); got != "repositor..." {
		t.Fatalf("truncateDots() = %q, want repositor...", got)
	}
	if got := truncateDots("repos", rootColumnWidth); got != "repos" {
		t.Fatalf("truncateDots() = %q, want repos", got)
	}
}
