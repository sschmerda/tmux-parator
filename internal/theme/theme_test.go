package theme

import "testing"

func TestResolveDefaultsToShadesOfPurple(t *testing.T) {
	got := Resolve("")
	if got.Name != "shades-of-purple" {
		t.Fatalf("theme = %q, want shades-of-purple", got.Name)
	}
}

func TestResolveKnownThemes(t *testing.T) {
	for _, name := range Names() {
		got := Resolve(name)
		if got.Name == "" || got.Background == "" || got.Title == "" || got.PaletteBorder == "" || got.PromptBorder == "" || got.Prompt == "" || got.Query == "" || got.SearchBG == "" || got.SearchFG == "" || got.Empty == "" || got.ChipBG == "" || got.SelectedChip == "" || got.SelectedChipBG == "" || got.Glyph == "" || got.MatchFG == "" || got.SelectedMatchFG == "" || got.SelectedBG == "" {
			t.Fatalf("theme %q resolved incompletely: %#v", name, got)
		}
	}
}

func TestResolveUnknownThemeUsesDefault(t *testing.T) {
	got := Resolve("tokyo-night")
	if got.Name != "shades-of-purple" {
		t.Fatalf("theme = %q, want shades-of-purple", got.Name)
	}
}
