package templatememory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MaxEntries = 2000
)

type Association struct {
	TemplateID      string            `json:"template_id,omitempty"`
	WithoutTemplate bool              `json:"without_template,omitempty"`
	Parameters      map[string]string `json:"parameters,omitempty"`
	LastUsedAt      time.Time         `json:"last_used_at"`
}

type state struct {
	Associations []persistedAssociation `json:"associations"`
}

type persistedAssociation struct {
	Path string `json:"path"`
	Association
}

type legacyState struct {
	Associations map[string]Association `json:"associations"`
}

type Store struct {
	path         string
	associations map[string]Association
	templateIDs  map[string]bool
	now          func() time.Time
}

func Path() (string, error) {
	if override := strings.TrimSpace(os.Getenv("TMUX_PARATOR_STATE")); override != "" {
		return override, nil
	}
	base := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "tmux-parator", "state.json"), nil
}

func Load(path string, templateIDs []string) (*Store, error) {
	return load(path, templateIDs, time.Now)
}

func load(path string, templateIDs []string, now func() time.Time) (*Store, error) {
	store := &Store{
		path:         path,
		associations: make(map[string]Association),
		templateIDs:  make(map[string]bool, len(templateIDs)),
		now:          now,
	}
	for _, id := range templateIDs {
		if id = strings.TrimSpace(id); id != "" {
			store.templateIDs[id] = true
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("read template memory: %w", err)
	}
	associations, err := decodeAssociations(data)
	if err != nil {
		return nil, fmt.Errorf("decode template memory: %w", err)
	}
	for path, association := range associations {
		store.associations[path] = association
	}
	if store.prune() {
		if err := store.save(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func decodeAssociations(data []byte) (map[string]Association, error) {
	var persisted state
	if err := json.Unmarshal(data, &persisted); err == nil {
		associations := make(map[string]Association, len(persisted.Associations))
		for _, entry := range persisted.Associations {
			path := normalizePath(entry.Path)
			if path != "" {
				associations[path] = Association{
					TemplateID:      entry.TemplateID,
					WithoutTemplate: entry.WithoutTemplate,
					Parameters:      cloneParameters(entry.Parameters),
					LastUsedAt:      entry.LastUsedAt,
				}
			}
		}
		return associations, nil
	}

	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	associations := make(map[string]Association, len(legacy.Associations))
	for path, association := range legacy.Associations {
		path = normalizePath(path)
		if path != "" {
			association.Parameters = cloneParameters(association.Parameters)
			associations[path] = association
		}
	}
	return associations, nil
}

func (s *Store) Lookup(path string) (Association, bool) {
	path = normalizePath(path)
	association, ok := s.associations[path]
	if !ok {
		return Association{}, false
	}
	association.Parameters = cloneParameters(association.Parameters)
	return association, true
}

func (s *Store) Remember(path string, templateID string, parameters map[string]string) error {
	path = normalizePath(path)
	templateID = strings.TrimSpace(templateID)
	if path == "" || templateID == "" || !s.templateIDs[templateID] {
		return nil
	}
	s.associations[path] = Association{
		TemplateID: templateID,
		Parameters: cloneParameters(parameters),
		LastUsedAt: s.now().UTC(),
	}
	s.prune()
	return s.save()
}

func (s *Store) RememberNoTemplate(path string) error {
	path = normalizePath(path)
	if path == "" {
		return nil
	}
	s.associations[path] = Association{
		WithoutTemplate: true,
		LastUsedAt:      s.now().UTC(),
	}
	s.prune()
	return s.save()
}

func (s *Store) Forget(path string) error {
	path = normalizePath(path)
	if _, ok := s.associations[path]; !ok {
		return nil
	}
	delete(s.associations, path)
	return s.save()
}

func (s *Store) prune() bool {
	changed := false
	for path, association := range s.associations {
		if !association.WithoutTemplate && !s.templateIDs[association.TemplateID] {
			delete(s.associations, path)
			changed = true
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			delete(s.associations, path)
			changed = true
		}
	}
	if len(s.associations) <= MaxEntries {
		return changed
	}
	type entry struct {
		path string
		used time.Time
	}
	entries := make([]entry, 0, len(s.associations))
	for path, association := range s.associations {
		entries = append(entries, entry{path: path, used: association.LastUsedAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].used.Equal(entries[j].used) {
			return entries[i].path < entries[j].path
		}
		return entries[i].used.Before(entries[j].used)
	})
	for _, entry := range entries[:len(entries)-MaxEntries] {
		delete(s.associations, entry.path)
	}
	return true
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create template memory directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return fmt.Errorf("create template memory file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("secure template memory file: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state{Associations: s.orderedAssociations()}); err != nil {
		file.Close()
		return fmt.Errorf("encode template memory: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync template memory: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close template memory: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("replace template memory: %w", err)
	}
	return nil
}

func (s *Store) orderedAssociations() []persistedAssociation {
	entries := make([]persistedAssociation, 0, len(s.associations))
	for path, association := range s.associations {
		entries = append(entries, persistedAssociation{
			Path:        path,
			Association: association,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastUsedAt.Equal(entries[j].LastUsedAt) {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].LastUsedAt.After(entries[j].LastUsedAt)
	})
	return entries
}

func normalizePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func cloneParameters(parameters map[string]string) map[string]string {
	if len(parameters) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(parameters))
	for name, value := range parameters {
		cloned[name] = value
	}
	return cloned
}
