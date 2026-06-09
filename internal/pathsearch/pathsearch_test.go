package pathsearch

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSearchGoHonorsDepthAndSkipRules(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "one")
	mkdir(t, root, "one", "two")
	mkdir(t, root, ".hidden")
	mkdir(t, root, "node_modules")

	got, err := Search(context.Background(), root, Options{
		Backend:    "go",
		MaxDepth:   1,
		SkipHidden: true,
		SkipDirs:   []string{"node_modules"},
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %#v, want one visible depth-1 directory", got)
	}
	if got[0].Name != "one" {
		t.Fatalf("candidate name = %q, want one", got[0].Name)
	}
}

func TestSearchGoHonorsLimit(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "one")
	mkdir(t, root, "two")

	got, err := Search(context.Background(), root, Options{
		Backend:  "go",
		MaxDepth: 1,
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(got))
	}
}

func TestSearchGoCanFindResultsBeyondOldDefaultLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 600; i++ {
		mkdir(t, root, "dir-"+strconv.Itoa(i))
	}
	mkdir(t, root, "repos")

	got, err := Search(context.Background(), root, Options{
		Backend:  "go",
		MaxDepth: 1,
		Limit:    5000,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	for _, candidate := range got {
		if candidate.Name == "repos" {
			return
		}
	}
	t.Fatalf("repos not found in %d candidates", len(got))
}

func TestSearchGoSkipsGitignoredDirectories(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "keep")
	mkdir(t, root, "ignored")
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	got, err := Search(context.Background(), root, Options{
		Backend:        "go",
		MaxDepth:       1,
		SkipGitignored: true,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "keep" {
		t.Fatalf("candidates = %#v, want only keep", got)
	}
}

func TestSearchGoSkipsUnreadableChildDirectories(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "keep")
	blocked := filepath.Join(root, "blocked")
	mkdir(t, blocked)
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatalf("chmod blocked: %v", err)
	}
	defer func() {
		_ = os.Chmod(blocked, 0o755)
	}()

	got, err := Search(context.Background(), root, Options{
		Backend:  "go",
		MaxDepth: 2,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("candidates = %#v, want at least keep", got)
	}
}

func TestSearchGoCanIncludeGitignoredDirectories(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "ignored")
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	got, err := Search(context.Background(), root, Options{
		Backend:        "go",
		MaxDepth:       1,
		SkipGitignored: false,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "ignored" {
		t.Fatalf("candidates = %#v, want ignored directory", got)
	}
}

func TestFDArgsDoNotRequestUnsupportedErrorMessageFlags(t *testing.T) {
	args := fdArgs("/tmp", Options{})
	for _, arg := range args {
		if arg == "--no-messages" || arg == "--show-errors" {
			t.Fatalf("fd args = %#v, want no explicit message/error flags", args)
		}
	}
}

func mkdir(t *testing.T, parts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(parts...), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
