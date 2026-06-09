package discovery

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sschmerda/tmux-parator/internal/config"
	"github.com/sschmerda/tmux-parator/internal/gitignore"
)

type Candidate struct {
	RootName     string
	Path         string
	RelativePath string
	DisplayPath  string
	Name         string
	Mode         string
	Glyph        string
	GlyphColor   string
}

type Options struct {
	Backend        string
	SkipHidden     bool
	SkipGitignored bool
	SkipDirs       []string
}

func OptionsFromConfig(cfg config.Discovery) Options {
	return Options{
		Backend:        cfg.Backend,
		SkipHidden:     cfg.SkipHidden,
		SkipGitignored: cfg.SkipGitignored,
		SkipDirs:       cfg.SkipDirs,
	}
}

func Discover(ctx context.Context, roots []config.Root, options Options) ([]Candidate, error) {
	var all []Candidate
	for _, root := range roots {
		candidates, err := discoverRoot(ctx, root, options)
		if err != nil {
			return nil, err
		}
		all = append(all, candidates...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name)
	})
	return all, nil
}

func discoverRoot(ctx context.Context, root config.Root, options Options) ([]Candidate, error) {
	rootPath := expandHome(root.Path)
	if strings.TrimSpace(rootPath) == "" {
		return nil, nil
	}
	mode := strings.TrimSpace(root.Mode)
	if mode == "" {
		mode = "subdir"
	}
	rootOptions := Options{SkipHidden: root.SkipHidden, SkipGitignored: root.SkipGitignored, SkipDirs: root.SkipDirs}
	if root.SkipDirs == nil {
		rootOptions = options
	}
	backend := resolveBackend(options.Backend)

	switch mode {
	case "subdir":
		if backend == "fd" {
			if candidates, err := discoverSubdirsFD(ctx, root.Name, rootPath, mode, root.Glyph, root.GlyphColor, root.Depth, rootOptions); err == nil {
				return candidates, nil
			}
		}
		return discoverSubdirsGo(ctx, root.Name, rootPath, mode, root.Glyph, root.GlyphColor, root.Depth, rootOptions)
	case "repo":
		if backend == "fd" {
			if candidates, err := discoverReposFD(ctx, root.Name, rootPath, mode, root.Glyph, root.GlyphColor, root.MaxDepth, rootOptions); err == nil {
				return candidates, nil
			}
		}
		return discoverReposGo(ctx, root.Name, rootPath, mode, root.Glyph, root.GlyphColor, root.MaxDepth, rootOptions)
	default:
		return nil, nil
	}
}

func resolveBackend(name string) string {
	switch strings.TrimSpace(name) {
	case "fd":
		if _, err := exec.LookPath("fd"); err == nil {
			return "fd"
		}
	case "auto":
		if _, err := exec.LookPath("fd"); err == nil {
			return "fd"
		}
	}
	return "go"
}

func discoverSubdirsGo(ctx context.Context, rootName string, rootPath string, mode string, glyph string, glyphColor string, depth int, options Options) ([]Candidate, error) {
	if depth == 0 {
		depth = 1
	}
	var candidates []Candidate
	ignores := gitignore.New(rootPath, options.SkipGitignored)
	err := scanSubdirs(ctx, rootPath, 0, depth, options, ignores, func(path string, dirDepth int, entry os.DirEntry) {
		_ = dirDepth
		candidates = append(candidates, newCandidate(rootName, rootPath, path, entry.Name(), mode, glyph, glyphColor))
	})
	return candidates, err
}

func discoverSubdirsFD(ctx context.Context, rootName string, rootPath string, mode string, glyph string, glyphColor string, depth int, options Options) ([]Candidate, error) {
	if depth == 0 {
		depth = 1
	}
	paths, err := fdDirectories(ctx, rootPath, depth, options)
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(paths))
	for _, path := range paths {
		if path == rootPath {
			continue
		}
		if currentDepth := pathDepth(rootPath, path); currentDepth > 0 && currentDepth <= depth {
			candidates = append(candidates, newCandidate(rootName, rootPath, path, filepath.Base(path), mode, glyph, glyphColor))
		}
	}
	return candidates, nil
}

func discoverReposGo(ctx context.Context, rootName string, rootPath string, mode string, glyph string, glyphColor string, maxDepth int, options Options) ([]Candidate, error) {
	var candidates []Candidate
	ignores := gitignore.New(rootPath, options.SkipGitignored)
	err := scanRepos(ctx, rootPath, rootPath, 0, maxDepth, options, ignores, func(path string, entry os.DirEntry) {
		candidates = append(candidates, newCandidate(rootName, rootPath, path, filepath.Base(path), mode, glyph, glyphColor))
	})
	return candidates, err
}

func discoverReposFD(ctx context.Context, rootName string, rootPath string, mode string, glyph string, glyphColor string, maxDepth int, options Options) ([]Candidate, error) {
	paths, err := fdDirectories(ctx, rootPath, maxDepth, options)
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(paths))
	for _, path := range paths {
		if path == rootPath {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if hasGitMetadata(path) {
			candidates = append(candidates, newCandidate(rootName, rootPath, path, filepath.Base(path), mode, glyph, glyphColor))
		}
	}
	return candidates, nil
}

func shouldSkipDir(name string, options Options) bool {
	if options.SkipHidden && strings.HasPrefix(name, ".") {
		return true
	}
	for _, skipped := range options.SkipDirs {
		if skipped == name {
			return true
		}
	}
	return false
}

func pathDepth(rootPath string, path string) int {
	relativePath, err := filepath.Rel(rootPath, path)
	if err != nil || relativePath == "." {
		return 0
	}
	relativePath = filepath.Clean(relativePath)
	depth := 0
	for _, part := range strings.Split(relativePath, string(filepath.Separator)) {
		if part != "" && part != "." {
			depth++
		}
	}
	return depth
}

func scanSubdirs(ctx context.Context, currentPath string, currentDepth int, maxDepth int, options Options, ignores gitignore.Matcher, visit func(path string, dirDepth int, entry os.DirEntry)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if shouldSkipDir(name, options) {
			continue
		}
		path := filepath.Join(currentPath, name)
		if ignores.Ignored(path, true) {
			continue
		}
		dirDepth := currentDepth + 1
		if dirDepth > maxDepth {
			continue
		}
		visit(path, dirDepth, entry)
		if dirDepth == maxDepth {
			continue
		}
		if err := scanSubdirs(ctx, path, dirDepth, maxDepth, options, ignores, visit); err != nil {
			return err
		}
	}
	return nil
}

func scanRepos(ctx context.Context, rootPath string, currentPath string, currentDepth int, maxDepth int, options Options, ignores gitignore.Matcher, visit func(path string, entry os.DirEntry)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if maxDepth > 0 && currentDepth > maxDepth {
		return nil
	}
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if name == ".git" {
			visit(currentPath, entry)
			return nil
		}
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(currentPath, name)
		if currentPath != rootPath && shouldSkipDir(name, options) {
			continue
		}
		if currentPath != rootPath && ignores.Ignored(path, true) {
			continue
		}
		nextDepth := currentDepth + 1
		if maxDepth > 0 && nextDepth > maxDepth {
			continue
		}
		if err := scanRepos(ctx, rootPath, path, nextDepth, maxDepth, options, ignores, visit); err != nil {
			return err
		}
	}
	return nil
}

func fdDirectories(ctx context.Context, rootPath string, maxDepth int, options Options) ([]string, error) {
	args := []string{".", rootPath, "--type", "d", "--color", "never", "--absolute-path"}
	if maxDepth > 0 {
		args = append(args, "--max-depth", strconv.Itoa(maxDepth))
	}
	if !options.SkipHidden {
		args = append(args, "--hidden")
	}
	if !options.SkipGitignored {
		args = append(args, "--no-ignore")
	}
	for _, skipped := range options.SkipDirs {
		if strings.TrimSpace(skipped) != "" {
			args = append(args, "--exclude", skipped)
		}
	}
	output, err := exec.CommandContext(ctx, "fd", args...).Output()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	paths := make([]string, 0, 128)
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}

func hasGitMetadata(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func newCandidate(rootName string, rootPath string, path string, name string, mode string, glyph string, glyphColor string) Candidate {
	relativePath, err := filepath.Rel(rootPath, path)
	if err != nil || relativePath == "." {
		relativePath = name
	}
	relativePath = filepath.ToSlash(relativePath)
	displayPathRoot := rootDisplayName(rootName, rootPath)
	displayPath := displayPathRoot
	if relativePath != "" && relativePath != "." {
		if displayPath == string(filepath.Separator) {
			displayPath += relativePath
		} else {
			displayPath += "/" + relativePath
		}
	}
	return Candidate{
		RootName:     rootName,
		Path:         path,
		RelativePath: relativePath,
		DisplayPath:  displayPath,
		Name:         name,
		Mode:         mode,
		Glyph:        glyph,
		GlyphColor:   glyphColor,
	}
}

func rootDisplayName(rootName string, rootPath string) string {
	base := filepath.Base(filepath.Clean(rootPath))
	if strings.TrimSpace(base) == "" || base == "." {
		return rootName
	}
	return base
}

func expandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
