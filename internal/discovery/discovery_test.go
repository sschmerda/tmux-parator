package discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sschmerda/tmux-parator/internal/config"
)

func TestDiscoverSubdirs(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "one"))
	mkdir(t, filepath.Join(root, "two"))
	mkdir(t, filepath.Join(root, "one", "nested"))
	mkdir(t, filepath.Join(root, ".hidden"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "subdir"}}, Options{SkipHidden: true})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Discover() len = %d, want 2: %#v", len(got), got)
	}
	if got[0].DisplayPath == "" || got[0].RootName != "work" {
		t.Fatalf("display path/root not set: %#v", got[0])
	}
	if wantPrefix := filepath.Base(root) + "/"; !strings.HasPrefix(got[0].DisplayPath, wantPrefix) {
		t.Fatalf("display path = %q, want prefix %q", got[0].DisplayPath, wantPrefix)
	}
}

func TestDiscoverSubdirsDepth(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "one", "nested", "deeper"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "subdir", Depth: 2}}, Options{SkipHidden: true})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Discover() len = %d, want 2: %#v", len(got), got)
	}
	if !hasRelativePath(got, "one") || !hasRelativePath(got, "one/nested") {
		t.Fatalf("relative paths = %#v, want one and one/nested", got)
	}
}

func TestDiscoverRepos(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "repo", ".git"))
	mkdir(t, filepath.Join(root, "plain"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "repo"}}, Options{SkipHidden: true})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "repo" {
		t.Fatalf("Discover() = %#v, want repo", got)
	}
	if got[0].RelativePath != "repo" || got[0].DisplayPath != filepath.Base(root)+"/repo" {
		t.Fatalf("paths = %#v, want relative/display paths", got[0])
	}
}

func TestDisplayPathUsesRootPathBasenameNotConfiguredName(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "documents")
	mkdir(t, filepath.Join(root, "notes"))

	got, err := Discover(context.Background(), []config.Root{{Name: "docs", Path: root, Mode: "subdir"}}, Options{SkipHidden: true})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Discover() len = %d, want 1: %#v", len(got), got)
	}
	if got[0].RootName != "docs" {
		t.Fatalf("RootName = %q, want configured name docs", got[0].RootName)
	}
	if got[0].DisplayPath != "documents/notes" {
		t.Fatalf("DisplayPath = %q, want documents/notes", got[0].DisplayPath)
	}
}

func TestDiscoverReposMaxDepth(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "one", "nested", "repo", ".git"))
	mkdir(t, filepath.Join(root, "top", ".git"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "repo", MaxDepth: 2}}, Options{SkipHidden: true})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != "top" {
		t.Fatalf("Discover() = %#v, want only top repo within max depth", got)
	}
}

func TestDiscoverCanIncludeHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".hidden"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "subdir"}}, Options{SkipHidden: false})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != ".hidden" {
		t.Fatalf("Discover() = %#v, want hidden directory", got)
	}
}

func TestDiscoverUsesRootSkipHiddenOverride(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".hidden"))

	got, err := Discover(
		context.Background(),
		[]config.Root{{Name: "work", Path: root, Mode: "subdir", SkipHidden: false, SkipDirs: []string{}}},
		Options{SkipHidden: true},
	)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != ".hidden" {
		t.Fatalf("Discover() = %#v, want hidden directory from root override", got)
	}
}

func TestDiscoverUsesRootSkipDirsOverride(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "tmp"))
	mkdir(t, filepath.Join(root, "node_modules"))

	got, err := Discover(
		context.Background(),
		[]config.Root{{Name: "work", Path: root, Mode: "subdir", SkipHidden: true, SkipDirs: []string{"tmp"}}},
		Options{SkipHidden: true, SkipDirs: []string{"node_modules"}},
	)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != "node_modules" {
		t.Fatalf("Discover() = %#v, want node_modules because root override replaced global skip_dirs", got)
	}
}

func TestDiscoverSkipsConfiguredDirectories(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "keep"))
	mkdir(t, filepath.Join(root, "node_modules"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "subdir"}}, Options{SkipHidden: true, SkipDirs: []string{"node_modules"}})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != "keep" {
		t.Fatalf("Discover() = %#v, want only keep", got)
	}
}

func TestDiscoverSkipsGitignoredDirectories(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "keep"))
	mkdir(t, filepath.Join(root, "ignored"))
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "subdir"}}, Options{SkipGitignored: true})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != "keep" {
		t.Fatalf("Discover() = %#v, want only keep", got)
	}
}

func TestDiscoverUsesRootSkipGitignoredOverride(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "ignored"))
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	got, err := Discover(
		context.Background(),
		[]config.Root{{Name: "work", Path: root, Mode: "subdir", SkipGitignored: false, SkipDirs: []string{}}},
		Options{SkipGitignored: true},
	)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != "ignored" {
		t.Fatalf("Discover() = %#v, want ignored directory from root override", got)
	}
}

func TestDiscoverReposStillDetectsGitWhenDotGitWouldBeSkipped(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "repo", ".git"))

	got, err := Discover(context.Background(), []config.Root{{Name: "work", Path: root, Mode: "repo"}}, Options{SkipHidden: true, SkipDirs: []string{".git"}})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].RelativePath != "repo" {
		t.Fatalf("Discover() = %#v, want repo despite .git skip rule", got)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hasRelativePath(candidates []Candidate, relativePath string) bool {
	for _, candidate := range candidates {
		if candidate.RelativePath == relativePath {
			return true
		}
	}
	return false
}
