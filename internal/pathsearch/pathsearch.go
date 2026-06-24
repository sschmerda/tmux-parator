package pathsearch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sschmerda/tmux-parator/internal/gitignore"
)

type Options struct {
	Backend        string
	MaxDepth       int
	SkipHidden     bool
	SkipGitignored bool
	SkipDirs       []string
	Limit          int
	RetainQuery    string
}

type Candidate struct {
	Path string
	Name string
}

type Batch struct {
	Candidates []Candidate
	Done       bool
	Limited    bool
	Err        error
}

func Search(ctx context.Context, root string, options Options) ([]Candidate, error) {
	rootPath, err := ExpandRoot(root)
	if err != nil {
		return nil, err
	}
	backend := strings.TrimSpace(options.Backend)
	if backend == "" {
		backend = "auto"
	}
	if backend == "fd" || backend == "auto" {
		if _, err := exec.LookPath("fd"); err == nil {
			return searchFD(ctx, rootPath, options)
		}
	}
	return searchGo(ctx, rootPath, options)
}

func Stream(ctx context.Context, root string, options Options) <-chan Batch {
	ch := make(chan Batch, 8)
	go func() {
		defer close(ch)
		rootPath, err := ExpandRoot(root)
		if err != nil {
			ch <- Batch{Done: true, Err: err}
			return
		}
		backend := strings.TrimSpace(options.Backend)
		if backend == "" {
			backend = "auto"
		}
		if backend == "fd" || backend == "auto" {
			if _, err := exec.LookPath("fd"); err == nil {
				if streamFD(ctx, ch, rootPath, options) {
					return
				}
			}
		}
		streamGo(ctx, ch, rootPath, options)
	}()
	return ch
}

func ExpandRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" || root == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(root, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, root[2:]), nil
	}
	if root == "." {
		return os.Getwd()
	}
	if root == ".." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Dir(cwd), nil
	}
	return filepath.Abs(root)
}

func DirectChildren(root string, options Options) ([]Candidate, error) {
	rootPath, err := ExpandRoot(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}
	ignores := gitignore.New(rootPath, options.SkipGitignored)
	children := make([]Candidate, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name(), options) {
			continue
		}
		path := filepath.Join(rootPath, entry.Name())
		if ignores.Ignored(path, true) {
			continue
		}
		children = append(children, Candidate{Path: path, Name: entry.Name()})
	}
	sortCandidates(children)
	return children, nil
}

func streamGo(ctx context.Context, ch chan<- Batch, root string, options Options) {
	batch := make([]Candidate, 0, 50)
	count := 0
	limited := false
	ignores := gitignore.New(root, options.SkipGitignored)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if filepath.Clean(path) == filepath.Clean(root) {
				return err
			}
			return filepath.SkipDir
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		if options.MaxDepth > 0 && pathDepth(root, path) > options.MaxDepth {
			return filepath.SkipDir
		}
		if shouldSkipDir(entry.Name(), options) {
			return filepath.SkipDir
		}
		if ignores.Ignored(path, true) {
			return filepath.SkipDir
		}
		count++
		candidate := Candidate{Path: path, Name: entry.Name()}
		if shouldRetainCandidate(root, candidate, count, options) {
			batch = append(batch, candidate)
		}
		if len(batch) >= 50 {
			if !sendBatch(ctx, ch, batch, false, false, nil) {
				return ctx.Err()
			}
			batch = make([]Candidate, 0, 50)
		}
		if options.Limit > 0 && count >= options.Limit && strings.TrimSpace(options.RetainQuery) == "" {
			limited = true
			return errLimitReached
		}
		return nil
	})
	if errors.Is(err, errLimitReached) || errors.Is(err, context.Canceled) {
		err = nil
	}
	if len(batch) > 0 && !sendBatch(ctx, ch, batch, false, false, nil) {
		return
	}
	sendBatch(ctx, ch, nil, true, limited, err)
}

func searchGo(ctx context.Context, root string, options Options) ([]Candidate, error) {
	var candidates []Candidate
	count := 0
	ignores := gitignore.New(root, options.SkipGitignored)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if filepath.Clean(path) == filepath.Clean(root) {
				return err
			}
			return filepath.SkipDir
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		if options.MaxDepth > 0 && pathDepth(root, path) > options.MaxDepth {
			return filepath.SkipDir
		}
		if shouldSkipDir(entry.Name(), options) {
			return filepath.SkipDir
		}
		if ignores.Ignored(path, true) {
			return filepath.SkipDir
		}
		count++
		candidate := Candidate{Path: path, Name: entry.Name()}
		if shouldRetainCandidate(root, candidate, count, options) {
			candidates = append(candidates, candidate)
		}
		if options.Limit > 0 && count >= options.Limit && strings.TrimSpace(options.RetainQuery) == "" {
			return errLimitReached
		}
		return nil
	})
	if errors.Is(err, errLimitReached) {
		err = nil
	}
	sortCandidates(candidates)
	return candidates, err
}

func streamFD(ctx context.Context, ch chan<- Batch, root string, options Options) bool {
	cmd := exec.CommandContext(ctx, "fd", fdArgs(root, options)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	scanner := bufio.NewScanner(stdout)
	batch := make([]Candidate, 0, 50)
	count := 0
	limited := false
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path == "" || filepath.Clean(path) == filepath.Clean(root) {
			continue
		}
		count++
		candidate := Candidate{Path: path, Name: filepath.Base(path)}
		if shouldRetainCandidate(root, candidate, count, options) {
			batch = append(batch, candidate)
		}
		if len(batch) >= 50 {
			if !sendBatch(ctx, ch, batch, false, false, nil) {
				_ = cmd.Wait()
				return true
			}
			batch = make([]Candidate, 0, 50)
		}
		if options.Limit > 0 && count >= options.Limit && strings.TrimSpace(options.RetainQuery) == "" {
			limited = true
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		scanErr = nil
		waitErr = nil
	}
	if limited {
		waitErr = nil
	}
	if scanErr == nil {
		waitErr = nil
	}
	if len(batch) > 0 && !sendBatch(ctx, ch, batch, false, false, nil) {
		return true
	}
	if scanErr != nil {
		sendBatch(ctx, ch, nil, true, limited, scanErr)
		return true
	}
	sendBatch(ctx, ch, nil, true, limited, waitErr)
	return true
}

func searchFD(ctx context.Context, root string, options Options) ([]Candidate, error) {
	output, err := exec.CommandContext(ctx, "fd", fdArgs(root, options)...).Output()
	if err != nil {
		return searchGo(ctx, root, options)
	}
	candidates := parseFDOutputWithOptions(root, output, options)
	sortCandidates(candidates)
	return candidates, nil
}

func fdArgs(root string, options Options) []string {
	args := []string{".", root, "--type", "d", "--color", "never", "--absolute-path"}
	if options.MaxDepth > 0 {
		args = append(args, "--max-depth", strconvItoa(options.MaxDepth))
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
	return args
}

func sendBatch(ctx context.Context, ch chan<- Batch, candidates []Candidate, done bool, limited bool, err error) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- Batch{Candidates: candidates, Done: done, Limited: limited, Err: err}:
		return true
	}
}

func parseFDOutput(root string, output []byte, limit int) []Candidate {
	return parseFDOutputWithOptions(root, output, Options{Limit: limit})
}

func parseFDOutputWithOptions(root string, output []byte, options Options) []Candidate {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	var candidates []Candidate
	count := 0
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path == "" || filepath.Clean(path) == filepath.Clean(root) {
			continue
		}
		count++
		candidate := Candidate{Path: path, Name: filepath.Base(path)}
		if shouldRetainCandidate(root, candidate, count, options) {
			candidates = append(candidates, candidate)
		}
		if options.Limit > 0 && count >= options.Limit && strings.TrimSpace(options.RetainQuery) == "" {
			break
		}
	}
	return candidates
}

func shouldRetainCandidate(root string, candidate Candidate, count int, options Options) bool {
	if options.Limit <= 0 || count <= options.Limit {
		return true
	}
	return candidateMatchesRetainQuery(root, candidate, options.RetainQuery)
}

func candidateMatchesRetainQuery(root string, candidate Candidate, query string) bool {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return false
	}
	values := []string{
		candidate.Name,
		filepath.ToSlash(candidate.Path),
	}
	if relativePath, err := filepath.Rel(root, candidate.Path); err == nil && relativePath != "." {
		values = append(values, filepath.ToSlash(relativePath))
	}
	for _, token := range tokens {
		found := false
		for _, value := range values {
			if fuzzyContains(strings.ToLower(value), token) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func fuzzyContains(value string, token string) bool {
	if strings.Contains(value, token) {
		return true
	}
	index := 0
	tokenRunes := []rune(token)
	for _, r := range value {
		if index < len(tokenRunes) && r == tokenRunes[index] {
			index++
		}
	}
	return index == len(tokenRunes)
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].Path) < strings.ToLower(candidates[j].Path)
	})
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

func pathDepth(root string, path string) int {
	relativePath, err := filepath.Rel(root, path)
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

func strconvItoa(value int) string {
	return strconv.FormatInt(int64(value), 10)
}

var errLimitReached = errors.New("path search limit reached")
