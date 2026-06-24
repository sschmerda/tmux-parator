package templatememory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRememberUpdatesAssociationWithoutGrowingHistory(t *testing.T) {
	workspace := t.TempDir()
	store, err := Load(filepath.Join(t.TempDir(), "state.json"), []string{"repo"})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	if err := store.Remember(workspace, "repo", map[string]string{"editor": "vim"}); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	if err := store.Remember(workspace, "repo", map[string]string{"editor": "nvim"}); err != nil {
		t.Fatal(err)
	}
	if len(store.associations) != 1 {
		t.Fatalf("associations = %d, want 1", len(store.associations))
	}
	got, ok := store.Lookup(workspace)
	if !ok || got.Parameters["editor"] != "nvim" {
		t.Fatalf("association = %#v, %v", got, ok)
	}
}

func TestRememberNoTemplateOverridesTemplateAssociation(t *testing.T) {
	workspace := t.TempDir()
	store, err := Load(filepath.Join(t.TempDir(), "state.json"), []string{"repo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remember(workspace, "repo", map[string]string{"editor": "vim"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RememberNoTemplate(workspace); err != nil {
		t.Fatal(err)
	}

	got, ok := store.Lookup(workspace)
	if !ok {
		t.Fatal("Lookup() returned no association")
	}
	if !got.WithoutTemplate || got.TemplateID != "" || len(got.Parameters) != 0 {
		t.Fatalf("association = %#v, want explicit no-template association", got)
	}
}

func TestLoadPrunesMissingAndUnknownAssociations(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid")
	noTemplate := filepath.Join(root, "no-template")
	old := filepath.Join(root, "old")
	unknown := filepath.Join(root, "unknown")
	for _, path := range []string{valid, noTemplate, old, unknown} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	statePath := filepath.Join(root, "state.json")
	data := `{"associations":{` +
		`"` + valid + `":{"template_id":"repo","last_used_at":"2026-01-01T00:00:00Z"},` +
		`"` + noTemplate + `":{"without_template":true,"last_used_at":"2026-01-01T00:00:00Z"},` +
		`"` + old + `":{"template_id":"repo","last_used_at":"2020-01-01T00:00:00Z"},` +
		`"` + unknown + `":{"template_id":"removed","last_used_at":"2026-01-01T00:00:00Z"},` +
		`"` + filepath.Join(root, "missing") + `":{"template_id":"repo","last_used_at":"2026-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(statePath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := load(statePath, []string{"repo"}, func() time.Time {
		return time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.associations) != 3 {
		t.Fatalf("associations = %#v, want valid, no-template, and old", store.associations)
	}
	if _, ok := store.associations[valid]; !ok {
		t.Fatalf("valid association was pruned: %#v", store.associations)
	}
	if association, ok := store.associations[noTemplate]; !ok || !association.WithoutTemplate {
		t.Fatalf("no-template association = %#v/%v, want retained", association, ok)
	}
	if _, ok := store.associations[old]; !ok {
		t.Fatalf("old association was pruned: %#v", store.associations)
	}
}

func TestPruneEnforcesLeastRecentlyUsedCap(t *testing.T) {
	root := t.TempDir()
	store := &Store{
		path:         filepath.Join(root, "state.json"),
		associations: make(map[string]Association),
		templateIDs:  map[string]bool{"repo": true},
		now:          func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
	}
	for index := 0; index < MaxEntries+2; index++ {
		path := filepath.Join(root, fmt.Sprintf("workspace-%03d", index))
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		store.associations[path] = Association{
			TemplateID: "repo",
			LastUsedAt: store.now().Add(time.Duration(index) * time.Minute),
		}
	}
	store.prune()
	if len(store.associations) != MaxEntries {
		t.Fatalf("associations = %d, want %d", len(store.associations), MaxEntries)
	}
}

func TestSavePersistsMostRecentlyUsedFirst(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	if err := os.Mkdir(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(second, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := Load(filepath.Join(root, "state.json"), []string{"repo"})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	if err := store.Remember(first, "repo", nil); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	if err := store.Remember(second, "repo", nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted state
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Associations) != 2 {
		t.Fatalf("associations = %#v, want 2", persisted.Associations)
	}
	if persisted.Associations[0].Path != second || persisted.Associations[1].Path != first {
		t.Fatalf("associations order = %#v, want newest first", persisted.Associations)
	}
}
