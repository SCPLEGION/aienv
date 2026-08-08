// Package install writes aienv's AI-agent integration files (a Claude Code
// skill, AGENTS.md, Cursor/Windsurf project rules) to the right location
// for each tool, so a user never has to hand-copy them between projects.
// All template content is embedded at compile time, keeping the binary
// self-contained.
package install

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aienv/internal/inject"
)

//go:embed templates
var templates embed.FS

const bodyFile = "templates/body.md"

const startMarker = "<!-- aienv:start -->"
const endMarker = "<!-- aienv:end -->"

// Target describes one supported AI-tool integration.
type Target struct {
	Name       string
	Aliases    []string
	Describe   string
	headerFile string
	// dedicated targets are files fully owned by aienv and overwritten
	// outright; non-dedicated targets (AGENTS.md) are merged into any
	// existing file between marker comments instead.
	dedicated bool
	pathFn    func(home, cwd string) string
}

var targets = []Target{
	{
		Name:       "claude",
		Describe:   "global Claude Code skill (~/.claude/skills/aienv-secrets/SKILL.md)",
		headerFile: "templates/claude-header.md",
		dedicated:  true,
		pathFn: func(home, cwd string) string {
			return filepath.Join(home, ".claude", "skills", "aienv-secrets", "SKILL.md")
		},
	},
	{
		Name:       "agents",
		Aliases:    []string{"codex", "antigravity"},
		Describe:   "AGENTS.md in the current project (Codex CLI, Antigravity, and other AGENTS.md-aware tools)",
		headerFile: "templates/agents-header.md",
		dedicated:  false,
		pathFn: func(home, cwd string) string {
			return filepath.Join(cwd, "AGENTS.md")
		},
	},
	{
		Name:       "cursor",
		Describe:   "Cursor project rule (./.cursor/rules/aienv.mdc)",
		headerFile: "templates/cursor-header.mdc",
		dedicated:  true,
		pathFn: func(home, cwd string) string {
			return filepath.Join(cwd, ".cursor", "rules", "aienv.mdc")
		},
	},
	{
		Name:       "windsurf",
		Describe:   "Windsurf project rule (./.windsurf/rules/aienv.md)",
		headerFile: "templates/windsurf-header.md",
		dedicated:  true,
		pathFn: func(home, cwd string) string {
			return filepath.Join(cwd, ".windsurf", "rules", "aienv.md")
		},
	},
}

// All returns every supported target, in a stable order.
func All() []Target { return targets }

// Resolve looks up a target by name or alias, case-insensitively.
func Resolve(name string) (Target, bool) {
	name = strings.ToLower(name)
	for _, t := range targets {
		if t.Name == name {
			return t, true
		}
		for _, alias := range t.Aliases {
			if alias == name {
				return t, true
			}
		}
	}
	return Target{}, false
}

// Write renders t's template (header + shared body) and installs it at its
// resolved path, returning the path written.
func Write(t Target, home, cwd string) (string, error) {
	header, err := templates.ReadFile(t.headerFile)
	if err != nil {
		return "", fmt.Errorf("reading embedded template %s: %w", t.headerFile, err)
	}
	body, err := templates.ReadFile(bodyFile)
	if err != nil {
		return "", fmt.Errorf("reading embedded template %s: %w", bodyFile, err)
	}

	content := strings.TrimRight(string(header), "\n") + "\n\n" + string(body)
	path := t.pathFn(home, cwd)

	final := content
	if !t.dedicated {
		final, err = mergeBlock(path, content)
		if err != nil {
			return "", err
		}
	}

	if err := inject.AtomicWriteFile(path, []byte(final), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// mergeBlock wraps block in marker comments and splices it into the
// existing content at path: replacing a prior aienv block in place if one
// exists, or appending a new one. A missing file just gets the block. The
// surrounding content is trimmed and reassembled with a fixed separator so
// repeated calls are idempotent instead of accumulating blank lines.
func mergeBlock(path, block string) (string, error) {
	wrapped := startMarker + "\n" + strings.TrimRight(block, "\n") + "\n" + endMarker + "\n"

	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return wrapped, nil
		}
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(existing)
	startIdx := strings.Index(content, startMarker)
	endIdx := strings.Index(content, endMarker)
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		before := strings.TrimRight(content[:startIdx], "\n")
		after := strings.TrimLeft(content[endIdx+len(endMarker):], "\n")
		result := wrapped
		if before != "" {
			result = before + "\n\n" + result
		}
		if after != "" {
			result += "\n" + after
		}
		return result, nil
	}

	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return wrapped, nil
	}
	return trimmed + "\n\n" + wrapped, nil
}
