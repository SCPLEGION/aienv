// Package inject writes secret values into destination files without ever
// returning the value itself. Every exported function returns only
// metadata (character counts, occurrence counts) suitable for printing to
// the user.
package inject

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Result describes what happened during an injection, safe to print.
type Result struct {
	Mode        string // "append", "replace", or "line"
	Chars       int    // length of the value that was written
	Occurrences int    // replace: number of placeholders substituted
}

// AtomicWriteFile writes content to path via a temp file + rename in the
// same directory, so a crash mid-write never leaves a truncated or
// half-written destination file. perm is applied before the rename.
func AtomicWriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".aienv-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("committing file: %w", err)
	}
	return nil
}

// destPerm preserves an existing file's mode, defaulting to 0644 for new
// files, so atomic rewrites don't silently change permissions.
func destPerm(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

// Append writes "KEY=value\n" to the end of path, creating the file if it
// doesn't exist and inserting a newline first if the existing content
// doesn't already end with one.
func Append(path, key string, value []byte) (Result, error) {
	perm := destPerm(path)

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var buf strings.Builder
	buf.Write(existing)
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString(key)
	buf.WriteByte('=')
	buf.Write(value)
	buf.WriteByte('\n')

	if err := AtomicWriteFile(path, []byte(buf.String()), perm); err != nil {
		return Result{}, err
	}
	return Result{Mode: "append", Chars: len(value)}, nil
}

// Replace substitutes every occurrence of placeholder in path with value.
// The destination file must already exist.
func Replace(path, placeholder string, value []byte) (Result, error) {
	if placeholder == "" {
		return Result{}, fmt.Errorf("placeholder must not be empty")
	}
	perm := destPerm(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("destination %s does not exist", path)
		}
		return Result{}, fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	count := strings.Count(content, placeholder)
	if count == 0 {
		return Result{Mode: "replace", Occurrences: 0}, nil
	}

	newContent := strings.ReplaceAll(content, placeholder, string(value))
	if err := AtomicWriteFile(path, []byte(newContent), perm); err != nil {
		return Result{}, err
	}
	return Result{Mode: "replace", Chars: len(value), Occurrences: count}, nil
}

// ParseRange parses a character range of the form "start:end" or "start:".
// The second form ("open-ended") means "from start to the end of the
// line". Both start and end are 0-indexed byte offsets within the line.
func ParseRange(at string) (start, end int, openEnded bool, err error) {
	parts := strings.SplitN(at, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false, fmt.Errorf(`invalid range %q, expected "start:end" or "start:"`, at)
	}

	start, err = strconv.Atoi(parts[0])
	if err != nil || start < 0 {
		return 0, 0, false, fmt.Errorf("invalid range start %q", parts[0])
	}

	if parts[1] == "" {
		return start, 0, true, nil
	}

	end, err = strconv.Atoi(parts[1])
	if err != nil || end < start {
		return 0, 0, false, fmt.Errorf("invalid range end %q", parts[1])
	}
	return start, end, false, nil
}

// Line inserts value into line lineNum (1-indexed) of path, at the byte
// range described by at. Missing lines are created as empty lines; if the
// target line is shorter than the range's start, it is padded with spaces.
// The destination file is created if it doesn't exist.
func Line(path string, lineNum int, at string, value []byte) (Result, error) {
	if lineNum < 1 {
		return Result{}, fmt.Errorf("line number must be >= 1, got %d", lineNum)
	}
	start, end, openEnded, err := ParseRange(at)
	if err != nil {
		return Result{}, err
	}
	perm := destPerm(path)

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var lines []string
	if len(existing) > 0 {
		lines = strings.Split(string(existing), "\n")
		// A trailing newline produces a spurious empty final element;
		// drop it so line count matches what a human sees in an editor.
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	for len(lines) < lineNum {
		lines = append(lines, "")
	}

	target := lines[lineNum-1]
	if len(target) < start {
		target += strings.Repeat(" ", start-len(target))
	}

	var newLine string
	switch {
	case openEnded:
		newLine = target[:start] + string(value)
	case end >= len(target):
		newLine = target[:start] + string(value)
	default:
		newLine = target[:start] + string(value) + target[end:]
	}
	lines[lineNum-1] = newLine

	out := strings.Join(lines, "\n") + "\n"
	if err := AtomicWriteFile(path, []byte(out), perm); err != nil {
		return Result{}, err
	}
	return Result{Mode: "line", Chars: len(value)}, nil
}
