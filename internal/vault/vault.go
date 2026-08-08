// Package vault handles the on-disk storage of secret values under
// ~/.aienv/vault/. Every function here treats its []byte payload as
// sensitive: nothing in this package prints, logs, or wraps a value into an
// error message.
package vault

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EnsureDir creates dir (and parents) and locks it down to owner-only access.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("setting vault directory permissions: %w", err)
	}
	return nil
}

// WriteSecret atomically writes value to path with owner-only (0600)
// permissions. The parent directory is created if needed.
func WriteSecret(path string, value []byte) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".vault-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp vault file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(value); err != nil {
		tmp.Close()
		return fmt.Errorf("writing vault file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing vault file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("setting vault file permissions: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("committing vault file: %w", err)
	}
	return nil
}

// ReadSecret reads the raw value stored at path.
func ReadSecret(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading vault file: %w", err)
	}
	return data, nil
}

// Exists reports whether a vault file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CopyFile copies src to dst with owner-only (0600) permissions, atomically.
// Used by `rotate` to back up the previous value before overwriting it.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".vault-bak-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp backup file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return fmt.Errorf("writing backup file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing backup file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("setting backup file permissions: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("committing backup file: %w", err)
	}
	return nil
}

// Remove deletes the vault file at path. A missing file is not an error.
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing vault file: %w", err)
	}
	return nil
}
