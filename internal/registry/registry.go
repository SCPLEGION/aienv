// Package registry manages ~/.aienv/registry.json, the plaintext index that
// maps a secret's logical name to its file inside the vault. It never
// touches secret values themselves.
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Entry is one registry record. File is a path relative to ~/.aienv (e.g.
// "vault/openai.key"). Created is an RFC3339 timestamp.
type Entry struct {
	File    string `json:"file"`
	Created string `json:"created"`
}

// Registry is the in-memory view of registry.json.
type Registry struct {
	path    string
	entries map[string]Entry
}

// Load reads the registry at path. A missing file is treated as an empty
// registry so first-run commands don't need special-casing.
func Load(path string) (*Registry, error) {
	r := &Registry{path: path, entries: map[string]Entry{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("reading registry: %w", err)
	}
	if len(data) == 0 {
		return r, nil
	}
	if err := json.Unmarshal(data, &r.entries); err != nil {
		return nil, fmt.Errorf("parsing registry %s: %w", path, err)
	}
	return r, nil
}

// Save writes the registry back to disk atomically (temp file + rename) with
// permissions restricted to the owner.
func (r *Registry) Save() error {
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding registry: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp registry file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp registry file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp registry file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("setting registry permissions: %w", err)
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		return fmt.Errorf("committing registry: %w", err)
	}
	return nil
}

// Get returns the entry for name, if present.
func (r *Registry) Get(name string) (Entry, bool) {
	e, ok := r.entries[name]
	return e, ok
}

// Has reports whether name is already registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.entries[name]
	return ok
}

// Put inserts or overwrites the entry for name.
func (r *Registry) Put(name string, e Entry) {
	r.entries[name] = e
}

// Remove deletes the entry for name, reporting whether it existed.
func (r *Registry) Remove(name string) bool {
	if _, ok := r.entries[name]; !ok {
		return false
	}
	delete(r.entries, name)
	return true
}

// List returns all registered key names sorted alphabetically.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
