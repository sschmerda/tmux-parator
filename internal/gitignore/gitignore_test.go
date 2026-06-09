package gitignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatcherIgnoresDirectoryPattern(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "ignored"))
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	matcher := New(root, true)
	if !matcher.Ignored(filepath.Join(root, "ignored"), true) {
		t.Fatal("Ignored() = false, want true")
	}
}

func TestMatcherHonorsNegation(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "ignored"))
	mkdir(t, filepath.Join(root, "keep"))
	writeFile(t, filepath.Join(root, ".gitignore"), "*\n!keep/\n")

	matcher := New(root, true)
	if !matcher.Ignored(filepath.Join(root, "ignored"), true) {
		t.Fatal("ignored dir = false, want true")
	}
	if matcher.Ignored(filepath.Join(root, "keep"), true) {
		t.Fatal("keep dir = true, want false")
	}
}

func TestMatcherReadsNestedGitignore(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "parent", "ignored"))
	writeFile(t, filepath.Join(root, "parent", ".gitignore"), "ignored/\n")

	matcher := New(root, true)
	if !matcher.Ignored(filepath.Join(root, "parent", "ignored"), true) {
		t.Fatal("nested ignored dir = false, want true")
	}
}

func TestMatcherCanBeDisabled(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "ignored"))
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	matcher := New(root, false)
	if matcher.Ignored(filepath.Join(root, "ignored"), true) {
		t.Fatal("disabled matcher ignored dir = true, want false")
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
