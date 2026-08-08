package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Append -----------------------------------------------------------

func TestAppend_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.env")

	res, err := Append(path, "API_KEY", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "API_KEY=fake-value-123\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
	if res.Mode != "append" {
		t.Fatalf("Result.Mode = %q, want %q", res.Mode, "append")
	}
	if res.Chars != len("fake-value-123") {
		t.Fatalf("Result.Chars = %d, want %d", res.Chars, len("fake-value-123"))
	}
}

func TestAppend_ExistingFileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.env")

	if err := os.WriteFile(path, []byte("EXISTING=1"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	res, err := Append(path, "NEW_KEY", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "EXISTING=1\nNEW_KEY=fake-value-123\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
	if res.Chars != len("fake-value-123") {
		t.Fatalf("Result.Chars = %d, want %d", res.Chars, len("fake-value-123"))
	}
}

func TestAppend_ExistingFileWithTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.env")

	if err := os.WriteFile(path, []byte("EXISTING=1\n"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	res, err := Append(path, "NEW_KEY", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "EXISTING=1\nNEW_KEY=fake-value-123\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
	if res.Chars != len("fake-value-123") {
		t.Fatalf("Result.Chars = %d, want %d", res.Chars, len("fake-value-123"))
	}
}

// --- Replace ------------------------------------------------------------

func TestReplace_SingleOccurrence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(path, []byte("hello __PLACEHOLDER__ world"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	res, err := Replace(path, "__PLACEHOLDER__", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "hello fake-value-123 world"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
	if res.Occurrences != 1 {
		t.Fatalf("Result.Occurrences = %d, want 1", res.Occurrences)
	}
	if res.Chars != len("fake-value-123") {
		t.Fatalf("Result.Chars = %d, want %d", res.Chars, len("fake-value-123"))
	}
}

func TestReplace_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	if err := os.WriteFile(path, []byte("A=__PLACEHOLDER__\nB=__PLACEHOLDER__\n"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	res, err := Replace(path, "__PLACEHOLDER__", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "A=fake-value-123\nB=fake-value-123\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
	if res.Occurrences != 2 {
		t.Fatalf("Result.Occurrences = %d, want 2", res.Occurrences)
	}
}

func TestReplace_ZeroOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")

	original := "no placeholder here"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	res, err := Replace(path, "__PLACEHOLDER__", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if res.Occurrences != 0 {
		t.Fatalf("Result.Occurrences = %d, want 0", res.Occurrences)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}
	if string(data) != original {
		t.Fatalf("file was modified despite zero occurrences: got %q, want %q", string(data), original)
	}
}

func TestReplace_DestinationDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.txt")

	_, err := Replace(path, "__PLACEHOLDER__", []byte("fake-value-123"))
	if err == nil {
		t.Fatal("expected an error when the destination file does not exist, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %q, want it to mention the file does not exist", err.Error())
	}
}

// --- Line -----------------------------------------------------------------

func TestLine_NewFileCreatesEarlierLinesEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.conf")

	res, err := Line(path, 3, "0:", []byte("fake-value-123"))
	if err != nil {
		t.Fatalf("Line: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "\n\nfake-value-123\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
	if res.Chars != len("fake-value-123") {
		t.Fatalf("Result.Chars = %d, want %d", res.Chars, len("fake-value-123"))
	}
}

func TestLine_OpenEndedRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.conf")

	if err := os.WriteFile(path, []byte("line1\n0123456789\n"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	_, err := Line(path, 2, "5:", []byte("NEW"))
	if err != nil {
		t.Fatalf("Line: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "line1\n01234NEW\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
}

func TestLine_ClosedRangePreservesTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.conf")

	if err := os.WriteFile(path, []byte("abcdefgh\n"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	_, err := Line(path, 1, "2:6", []byte("XY"))
	if err != nil {
		t.Fatalf("Line: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "abXYgh\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
}

func TestLine_PadsShortLineWithSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.conf")

	if err := os.WriteFile(path, []byte("ab\n"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	_, err := Line(path, 1, "5:7", []byte("Z"))
	if err != nil {
		t.Fatalf("Line: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result file: %v", err)
	}

	want := "ab   Z\n"
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
}

// --- ParseRange -------------------------------------------------------------

func TestParseRange_ValidClosed(t *testing.T) {
	start, end, openEnded, err := ParseRange("0:5")
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	if start != 0 || end != 5 || openEnded {
		t.Fatalf("ParseRange(\"0:5\") = (%d, %d, %v), want (0, 5, false)", start, end, openEnded)
	}
}

func TestParseRange_ValidOpenEnded(t *testing.T) {
	start, end, openEnded, err := ParseRange("3:")
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	if start != 3 || end != 0 || !openEnded {
		t.Fatalf("ParseRange(\"3:\") = (%d, %d, %v), want (3, 0, true)", start, end, openEnded)
	}
}

func TestParseRange_InvalidNoColon(t *testing.T) {
	_, _, _, err := ParseRange("abc")
	if err == nil {
		t.Fatal(`expected an error for ParseRange("abc"), got nil`)
	}
}

func TestParseRange_InvalidNonNumeric(t *testing.T) {
	_, _, _, err := ParseRange("a:5")
	if err == nil {
		t.Fatal(`expected an error for ParseRange("a:5"), got nil`)
	}
}

func TestParseRange_InvalidEndBeforeStart(t *testing.T) {
	_, _, _, err := ParseRange("5:3")
	if err == nil {
		t.Fatal(`expected an error for ParseRange("5:3") where end < start, got nil`)
	}
}
