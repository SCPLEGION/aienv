package main

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir switches to dir for the duration of the test and restores the
// original working directory afterward.
func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
}

func TestDifferentLocalBinary_NoLocalFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if _, ok := differentLocalBinary("/usr/local/bin/aienv"); ok {
		t.Fatal("expected no hit when the current directory has no aienv file")
	}
}

func TestDifferentLocalBinary_SameFileAsDest(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	destPath := filepath.Join(dir, "installed-aienv")
	if err := os.WriteFile(destPath, []byte("binary"), 0o755); err != nil {
		t.Fatalf("seeding dest binary: %v", err)
	}
	localPath := filepath.Join(dir, "aienv")
	if err := os.Symlink(destPath, localPath); err != nil {
		t.Fatalf("symlinking local aienv to dest: %v", err)
	}

	if _, ok := differentLocalBinary(destPath); ok {
		t.Fatal("expected no hit when the local aienv resolves to the same file as destPath")
	}
}

func TestDifferentLocalBinary_DifferentFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	destPath := filepath.Join(t.TempDir(), "installed-aienv")
	if err := os.WriteFile(destPath, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("seeding dest binary: %v", err)
	}
	localPath := filepath.Join(dir, "aienv")
	if err := os.WriteFile(localPath, []byte("new binary"), 0o755); err != nil {
		t.Fatalf("seeding local binary: %v", err)
	}

	got, ok := differentLocalBinary(destPath)
	if !ok {
		t.Fatal("expected a hit when the current directory has a different aienv binary")
	}
	if got != localPath {
		t.Fatalf("differentLocalBinary returned %q, want %q", got, localPath)
	}
}

func TestOnPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin")

	if !onPath(dir) {
		t.Fatalf("expected onPath(%q) to be true given PATH=%q", dir, os.Getenv("PATH"))
	}
	if onPath("/definitely/not/on/path") {
		t.Fatal("expected onPath to be false for a directory not in PATH")
	}
}
