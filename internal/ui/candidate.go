package ui

import (
	"path/filepath"
	"strings"

	"github.com/sschmerda/tmux-parator/internal/discovery"
	"github.com/sschmerda/tmux-parator/internal/fuzzy"
	"github.com/sschmerda/tmux-parator/internal/pathsearch"
	"github.com/sschmerda/tmux-parator/internal/tmux"
)

func rootMode(root discovery.Candidate) string {
	if kind := strings.TrimSpace(root.Kind); kind != "" {
		return kind
	}
	return strings.TrimSpace(root.Mode)
}

type candidateKind int

const (
	candidateSession candidateKind = iota
	candidateRoot
	candidatePath
)

type candidate struct {
	kind         candidateKind
	session      tmux.Session
	root         discovery.Candidate
	fsPath       pathsearch.Candidate
	pathDetail   string
	origin       string
	matchIndexes []int
	fieldIndexes map[string][]int
}

const (
	fieldRoot        = "root"
	fieldPath        = "path"
	fieldCompactPath = "compact_path"
	fieldDetail      = "detail"
)

func candidatesFromSessions(sessions []tmux.Session, origins map[string]string, roots []discovery.Candidate) []candidate {
	items := make([]candidate, 0, len(sessions))
	for _, session := range sessions {
		item := candidate{kind: candidateSession, session: session, origin: origins[session.Name]}
		item.pathDetail = compactSessionPath(session.Metadata.Path, roots)
		items = append(items, item)
	}
	return items
}

func candidatesFromRoots(roots []discovery.Candidate) []candidate {
	return candidatesFromRootsWithPathDetail(roots, true)
}

func candidatesFromRootsWithPathDetail(roots []discovery.Candidate, includeRootInPath bool) []candidate {
	items := make([]candidate, 0, len(roots))
	for _, root := range roots {
		item := candidate{kind: candidateRoot, root: root}
		if includeRootInPath {
			item.pathDetail = root.DisplayPath
		} else {
			item.pathDetail = root.RelativePath
		}
		if item.pathDetail == "" {
			item.pathDetail = root.DisplayPath
		}
		items = append(items, item)
	}
	return items
}

func candidatesFromPaths(paths []pathsearch.Candidate) []candidate {
	items := make([]candidate, 0, len(paths))
	for _, path := range paths {
		items = append(items, candidate{kind: candidatePath, fsPath: path})
	}
	return items
}

func (c candidate) title() string {
	switch c.kind {
	case candidatePath:
		return c.fsPath.Name
	case candidateRoot:
		return c.root.Name
	default:
		return c.session.Name
	}
}

func (c candidate) detail() string {
	switch c.kind {
	case candidatePath:
		return c.fsPath.Path
	case candidateRoot:
		if c.pathDetail != "" {
			return c.pathDetail
		}
		if c.root.DisplayPath != "" {
			return c.root.DisplayPath
		}
		return c.root.Path
	default:
		if c.pathDetail != "" {
			return c.pathDetail
		}
		if strings.TrimSpace(c.session.Metadata.Path) != "" {
			return displaySessionPath(c.session.Metadata.Path)
		}
		return ""
	}
}

func compactSessionPath(path string, roots []discovery.Candidate) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	for _, root := range roots {
		if root.Path == path {
			if root.DisplayPath != "" {
				return root.DisplayPath
			}
			if root.RelativePath != "" && root.RootName != "" {
				return root.RootName + "/" + root.RelativePath
			}
			if root.RelativePath != "" {
				return root.RelativePath
			}
		}
	}
	return displaySessionPath(path)
}

func displaySessionPath(path string) string {
	home, err := pathsearch.ExpandRoot("~")
	if err == nil {
		cleanPath := filepath.Clean(path)
		cleanHome := filepath.Clean(home)
		if cleanPath == cleanHome {
			return "~"
		}
		if strings.HasPrefix(cleanPath, cleanHome+string(filepath.Separator)) {
			return "~/" + filepath.ToSlash(strings.TrimPrefix(cleanPath, cleanHome+string(filepath.Separator)))
		}
	}
	return filepath.ToSlash(path)
}

func (c candidate) fuzzyCandidate() fuzzy.Candidate {
	switch c.kind {
	case candidatePath:
		searchPath := c.fsPath.Path
		if c.pathDetail != "" {
			searchPath = c.pathDetail
		}
		return fuzzy.Candidate{
			Title:    c.fsPath.Name,
			Category: "Paths",
			Fields: []fuzzy.Field{
				{Name: fieldPath, Value: searchPath, Weight: 250},
			},
			Value: c,
		}
	case candidateRoot:
		pathDetail := c.pathDetail
		if pathDetail == "" {
			pathDetail = c.root.DisplayPath
		}
		return fuzzy.Candidate{
			Title:    c.root.Name,
			Category: "Roots",
			Aliases:  []string{originLabel(rootMode(c.root))},
			Fields: []fuzzy.Field{
				{Name: fieldRoot, Value: c.root.RootName, Weight: 900},
				{Name: fieldCompactPath, Value: pathDetail, Weight: 300},
			},
			Value: c,
		}
	default:
		aliases := []string{}
		if c.origin != "" {
			aliases = append(aliases, originLabel(c.origin))
		} else {
			aliases = append(aliases, originLabel(""))
		}
		return fuzzy.Candidate{
			Title:    c.session.Name,
			Category: "Sessions",
			Aliases:  aliases,
			Fields: []fuzzy.Field{
				{Name: fieldRoot, Value: c.session.Metadata.Root, Weight: 900},
			},
			Value: c,
		}
	}
}

func (c candidate) sessionName() string {
	switch c.kind {
	case candidatePath:
		return sanitizeSessionName(c.title())
	case candidateRoot:
		return sanitizeSessionName(c.title())
	default:
		return c.session.Name
	}
}

func (c candidate) path() string {
	if c.kind == candidatePath {
		return c.fsPath.Path
	}
	if c.kind == candidateRoot {
		return c.root.Path
	}
	return ""
}

func (c candidate) rootLabel() string {
	switch c.kind {
	case candidateSession:
		return strings.TrimSpace(c.session.Metadata.Root)
	case candidateRoot:
		if rootName := strings.TrimSpace(c.root.RootName); rootName != "" {
			return rootName
		}
		if rootPath := strings.TrimSpace(c.root.Path); rootPath != "" {
			return filepath.Base(rootPath)
		}
		return ""
	default:
		return ""
	}
}

func (c candidate) sessionMetadata() tmux.SessionMetadata {
	switch c.kind {
	case candidatePath:
		return tmux.SessionMetadata{
			CreatedByParator: true,
			Kind:             "manual",
			Path:             c.fsPath.Path,
			BaseName:         c.sessionName(),
		}
	case candidateRoot:
		return tmux.SessionMetadata{
			CreatedByParator: true,
			Kind:             originLabel(rootMode(c.root)),
			Path:             c.root.Path,
			Root:             c.root.RootName,
			BaseName:         c.sessionName(),
			Glyph:            c.root.Glyph,
			GlyphColor:       c.root.GlyphColor,
		}
	default:
		return tmux.SessionMetadata{}
	}
}

func sanitizeSessionName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "workspace"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "workspace"
	}
	return result
}
