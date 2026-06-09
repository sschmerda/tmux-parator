package gitignore

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Matcher struct {
	root    string
	enabled bool
	cache   map[string][]pattern
}

type pattern struct {
	baseRel  string
	value    string
	negated  bool
	dirOnly  bool
	anchored bool
}

func New(root string, enabled bool) Matcher {
	return Matcher{
		root:    filepath.Clean(root),
		enabled: enabled,
		cache:   map[string][]pattern{},
	}
}

func (m *Matcher) Ignored(candidatePath string, isDir bool) bool {
	if !m.enabled {
		return false
	}
	rel, err := filepath.Rel(m.root, candidatePath)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	dirs := parentDirs(rel, isDir)
	ignored := false
	for _, dir := range dirs {
		for _, p := range m.patternsFor(dir) {
			if p.matches(rel, isDir) {
				ignored = !p.negated
			}
		}
	}
	return ignored
}

func parentDirs(rel string, isDir bool) []string {
	dir := path.Dir(rel)
	if isDir {
		dir = rel
	}
	if dir == "." {
		return []string{""}
	}
	parts := strings.Split(dir, "/")
	dirs := []string{""}
	current := ""
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		dirs = append(dirs, current)
	}
	return dirs
}

func (m *Matcher) patternsFor(dirRel string) []pattern {
	if patterns, ok := m.cache[dirRel]; ok {
		return patterns
	}
	gitignorePath := filepath.Join(m.root, filepath.FromSlash(dirRel), ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		m.cache[dirRel] = nil
		return nil
	}
	patterns := parsePatterns(string(content), dirRel)
	m.cache[dirRel] = patterns
	return patterns
}

func parsePatterns(content string, baseRel string) []pattern {
	lines := strings.Split(content, "\n")
	patterns := make([]pattern, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := pattern{baseRel: baseRel}
		if strings.HasPrefix(line, "!") {
			p.negated = true
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "/") {
			p.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if strings.HasPrefix(line, "/") {
			p.anchored = true
			line = strings.TrimPrefix(line, "/")
		}
		line = filepath.ToSlash(filepath.Clean(line))
		if line == "." {
			continue
		}
		if strings.Contains(line, "/") {
			p.anchored = true
		}
		p.value = line
		patterns = append(patterns, p)
	}
	return patterns
}

func (p pattern) matches(rel string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}
	if p.baseRel != "" {
		if rel != p.baseRel && !strings.HasPrefix(rel, p.baseRel+"/") {
			return false
		}
		rel = strings.TrimPrefix(rel, p.baseRel+"/")
	}
	if p.anchored {
		return matchPath(p.value, rel)
	}
	for _, part := range strings.Split(rel, "/") {
		if matchPath(p.value, part) {
			return true
		}
	}
	return false
}

func matchPath(pattern string, value string) bool {
	ok, err := path.Match(pattern, value)
	if err == nil && ok {
		return true
	}
	return pattern == value || strings.HasPrefix(value, pattern+"/")
}
