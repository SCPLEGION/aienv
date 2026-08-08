package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}
	if r == nil {
		t.Fatal("Load returned a nil registry")
	}
	if names := r.List(); len(names) != 0 {
		t.Fatalf("expected empty registry for a nonexistent file, got %v", names)
	}
}

func TestPutSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	r.Put("OPENAI_TOKEN", Entry{File: "vault/openai_token.key", Created: "2026-07-19T12:00:00Z"})
	r.Put("DB_PASSWORD", Entry{File: "vault/db_password.key", Created: "2026-07-19T12:05:00Z"})

	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	r2, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	entry, ok := r2.Get("OPENAI_TOKEN")
	if !ok {
		t.Fatal("expected OPENAI_TOKEN to be present after reload")
	}
	if entry.File != "vault/openai_token.key" || entry.Created != "2026-07-19T12:00:00Z" {
		t.Fatalf("unexpected entry after reload: %+v", entry)
	}

	entry2, ok := r2.Get("DB_PASSWORD")
	if !ok {
		t.Fatal("expected DB_PASSWORD to be present after reload")
	}
	if entry2.File != "vault/db_password.key" || entry2.Created != "2026-07-19T12:05:00Z" {
		t.Fatalf("unexpected entry after reload: %+v", entry2)
	}
}

func TestHasGetRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if r.Has("MISSING") {
		t.Fatal(`expected Has("MISSING") to be false on an empty registry`)
	}
	if _, ok := r.Get("MISSING"); ok {
		t.Fatal(`expected Get("MISSING") to report not found`)
	}
	if r.Remove("MISSING") {
		t.Fatal(`expected Remove("MISSING") to report false on an empty registry`)
	}

	r.Put("API_KEY", Entry{File: "vault/api_key.key", Created: "2026-07-19T00:00:00Z"})

	if !r.Has("API_KEY") {
		t.Fatal(`expected Has("API_KEY") to be true after Put`)
	}
	entry, ok := r.Get("API_KEY")
	if !ok {
		t.Fatal(`expected Get("API_KEY") to find the entry`)
	}
	if entry.File != "vault/api_key.key" {
		t.Fatalf("unexpected file: %q", entry.File)
	}

	if !r.Remove("API_KEY") {
		t.Fatal(`expected Remove("API_KEY") to report true`)
	}
	if r.Has("API_KEY") {
		t.Fatal(`expected Has("API_KEY") to be false after Remove`)
	}
	if r.Remove("API_KEY") {
		t.Fatal(`expected a second Remove("API_KEY") to report false`)
	}
}

func TestList_Sorted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	r.Put("ZEBRA", Entry{File: "vault/zebra.key", Created: "2026-07-19T00:00:00Z"})
	r.Put("ALPHA", Entry{File: "vault/alpha.key", Created: "2026-07-19T00:00:00Z"})
	r.Put("MIKE", Entry{File: "vault/mike.key", Created: "2026-07-19T00:00:00Z"})

	got := r.List()
	want := []string{"ALPHA", "MIKE", "ZEBRA"}
	if len(got) != len(want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List() = %v, want %v", got, want)
		}
	}
}

func TestSave_ProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r.Put("OPENAI_TOKEN", Entry{File: "vault/openai_token.key", Created: "2026-07-19T12:00:00Z"})

	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved registry file: %v", err)
	}

	var raw map[string]struct {
		File    string `json:"file"`
		Created string `json:"created"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved registry is not valid JSON in the expected shape: %v", err)
	}

	got, ok := raw["OPENAI_TOKEN"]
	if !ok {
		t.Fatal(`expected "OPENAI_TOKEN" key in the saved registry JSON`)
	}
	if got.File != "vault/openai_token.key" {
		t.Fatalf("unexpected file field: %q", got.File)
	}
	if got.Created != "2026-07-19T12:00:00Z" {
		t.Fatalf("unexpected created field: %q", got.Created)
	}

	// Save's contract promises owner-only permissions on the written file.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved registry file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected registry file permissions 0600, got %o", perm)
	}
}
