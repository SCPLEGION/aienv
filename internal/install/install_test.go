package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfInstall(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "aienv-downloaded")
	content := []byte("fake binary content")
	if err := os.WriteFile(srcPath, content, 0o644); err != nil {
		t.Fatalf("seeding fake binary: %v", err)
	}

	destPath, err := SelfInstall(srcPath, destDir)
	if err != nil {
		t.Fatalf("SelfInstall: %v", err)
	}

	wantPath := filepath.Join(destDir, "aienv")
	if destPath != wantPath {
		t.Fatalf("SelfInstall returned %q, want %q", destPath, wantPath)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading installed binary: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("installed binary content = %q, want %q", got, content)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Fatalf("installed binary permissions = %o, want 0755", perm)
	}
}

func TestResolve(t *testing.T) {
	if _, ok := Resolve("claude"); !ok {
		t.Fatal(`expected Resolve("claude") to succeed`)
	}
	if resolved, ok := Resolve("CODEX"); !ok || resolved.Name != "agents" {
		t.Fatalf(`expected Resolve("CODEX") to resolve the "agents" target via alias, got %+v, ok=%v`, resolved, ok)
	}
	if _, ok := Resolve("antigravity"); !ok {
		t.Fatal(`expected Resolve("antigravity") to succeed`)
	}
	if _, ok := Resolve("nonexistent-tool"); ok {
		t.Fatal(`expected Resolve("nonexistent-tool") to fail`)
	}
}

func TestAll_ReturnsEveryTarget(t *testing.T) {
	names := map[string]bool{}
	for _, target := range All() {
		names[target.Name] = true
	}
	for _, want := range []string{"claude", "agents", "cursor", "windsurf"} {
		if !names[want] {
			t.Fatalf("expected All() to include target %q, got %v", want, names)
		}
	}
}

func TestWrite_DedicatedTargetOverwritesWholeFile(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()

	target, ok := Resolve("claude")
	if !ok {
		t.Fatal(`Resolve("claude") failed`)
	}

	path, err := Write(target, home, cwd)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	wantPath := filepath.Join(home, ".claude", "skills", "aienv-secrets", "SKILL.md")
	if path != wantPath {
		t.Fatalf("Write returned path %q, want %q", path, wantPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading installed file: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\nname: aienv-secrets\n") {
		t.Fatalf("installed claude skill missing expected frontmatter, got: %q", content[:min(60, len(content))])
	}
	if !strings.Contains(content, "aienv exists") {
		t.Fatal("installed claude skill should mention the `exists` command")
	}
}

func TestWrite_NonDedicatedTargetMergesIntoExistingFile(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()

	agentsPath := filepath.Join(cwd, "AGENTS.md")
	original := "# My Project\n\nSome existing project-specific instructions.\n"
	if err := os.WriteFile(agentsPath, []byte(original), 0o644); err != nil {
		t.Fatalf("seeding AGENTS.md: %v", err)
	}

	target, ok := Resolve("agents")
	if !ok {
		t.Fatal(`Resolve("agents") failed`)
	}

	if _, err := Write(target, home, cwd); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("reading merged AGENTS.md: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, original) {
		t.Fatalf("merge dropped or reordered pre-existing content, got: %q", content)
	}
	if !strings.Contains(content, startMarker) || !strings.Contains(content, endMarker) {
		t.Fatal("merged AGENTS.md missing aienv marker comments")
	}
	if strings.Count(content, startMarker) != 1 {
		t.Fatalf("expected exactly one start marker, got content: %q", content)
	}
}

func TestWrite_NonDedicatedTargetIsIdempotent(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()

	target, ok := Resolve("agents")
	if !ok {
		t.Fatal(`Resolve("agents") failed`)
	}

	agentsPath := filepath.Join(cwd, "AGENTS.md")

	if _, err := Write(target, home, cwd); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	first, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("reading AGENTS.md after first write: %v", err)
	}

	if _, err := Write(target, home, cwd); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	second, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("reading AGENTS.md after second write: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("re-running Write changed an already-up-to-date file:\nfirst:  %q\nsecond: %q", first, second)
	}
	if strings.Count(string(second), startMarker) != 1 {
		t.Fatalf("re-running Write duplicated the aienv block: %q", second)
	}
}
