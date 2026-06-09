package fuzzy

import "testing"

func TestFilterMatchesMultipleTokens(t *testing.T) {
	candidates := []Candidate{
		{Title: "Split Horizontal", Category: "Panes", Aliases: []string{"sh"}},
		{Title: "New Window", Category: "Windows", Aliases: []string{"nw"}},
	}
	matches := Filter(candidates, "split pane")
	if len(matches) != 1 {
		t.Fatalf("match count = %d, want 1", len(matches))
	}
	if matches[0].Candidate.Title != "Split Horizontal" {
		t.Fatalf("match = %q", matches[0].Candidate.Title)
	}
}

func TestFilterMatchesAliasesAndInitials(t *testing.T) {
	candidates := []Candidate{
		{Title: "Split Horizontal", Category: "Panes", Aliases: []string{"side"}},
	}
	if got := Filter(candidates, "sh"); len(got) != 1 {
		t.Fatalf("initial match count = %d, want 1", len(got))
	}
	if got := Filter(candidates, "side"); len(got) != 1 {
		t.Fatalf("alias match count = %d, want 1", len(got))
	}
}

func TestFilterReturnsTitleMatchIndexes(t *testing.T) {
	candidates := []Candidate{{Title: "Lazygit", Category: "Tools", Aliases: []string{"lg"}}}
	matches := Filter(candidates, "laz")
	if len(matches) != 1 {
		t.Fatalf("match count = %d, want 1", len(matches))
	}
	want := []int{0, 1, 2}
	if !equalIndexes(matches[0].TitleIndexes, want) {
		t.Fatalf("title indexes = %#v, want %#v", matches[0].TitleIndexes, want)
	}
}

func TestFilterMatchesExtraFields(t *testing.T) {
	candidates := []Candidate{
		{Title: "tmux-parator", Fields: []Field{{Name: "path", Value: "/Users/me/code/tmux-parator"}}},
	}
	matches := Filter(candidates, "code")
	if len(matches) != 1 {
		t.Fatalf("match count = %d, want 1", len(matches))
	}
	if len(matches[0].FieldIndexes["path"]) == 0 {
		t.Fatalf("path indexes missing: %#v", matches[0])
	}
}

func TestFilterCarriesFieldIndexesWhenAliasWinsScore(t *testing.T) {
	candidates := []Candidate{
		{
			Title:   "tmux-parator",
			Aliases: []string{"repos"},
			Fields:  []Field{{Name: "path", Value: "/tmp/repos/tmux-parator", Weight: 100}},
		},
	}
	matches := Filter(candidates, "repos")
	if len(matches) != 1 {
		t.Fatalf("match count = %d, want 1", len(matches))
	}
	if len(matches[0].FieldIndexes["path"]) == 0 {
		t.Fatalf("path indexes missing: %#v", matches[0].FieldIndexes)
	}
}

func TestFilterRanksTitleMatchAboveCategoryMatch(t *testing.T) {
	candidates := []Candidate{
		{Title: "Tools", Category: "Misc"},
		{Title: "Btop", Category: "Tools", Aliases: []string{"bt"}},
	}
	matches := Filter(candidates, "tools")
	if len(matches) != 2 {
		t.Fatalf("match count = %d, want 2", len(matches))
	}
	if matches[0].Candidate.Title != "Tools" {
		t.Fatalf("first match = %q, want Tools", matches[0].Candidate.Title)
	}
}

func equalIndexes(got []int, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
