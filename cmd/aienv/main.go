// Command aienv is a secure middle layer for injecting secrets into files
// on disk. It is designed to be driven by an AI agent: the agent tells
// aienv *where* a secret should go, but the secret value itself never
// appears in stdout, stderr, or any error message.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/term"

	"aienv/internal/inject"
	"aienv/internal/install"
	"aienv/internal/registry"
	"aienv/internal/vault"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z"
// (see scripts/release.sh). Left as "dev" for local builds.
var version = "dev"

var keyNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type env struct {
	baseDir      string
	registryPath string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}
	baseDir := filepath.Join(home, ".aienv")
	e := env{
		baseDir:      baseDir,
		registryPath: filepath.Join(baseDir, "registry.json"),
	}

	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("no command given")
	}

	switch os.Args[1] {
	case "list":
		return e.cmdList(os.Args[2:])
	case "exists":
		return e.cmdExists(os.Args[2:])
	case "put":
		return e.cmdPut(os.Args[2:])
	case "get":
		return e.cmdGet(os.Args[2:])
	case "rm":
		return e.cmdRm(os.Args[2:])
	case "rotate":
		return e.cmdRotate(os.Args[2:])
	case "install":
		return e.cmdInstall(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	case "-v", "--version", "version":
		fmt.Println("aienv " + version)
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `aienv - secure secret injection CLI

Usage:
  aienv list
  aienv exists <NAME> [--in <path>]
  aienv put <NAME> [--force]
  aienv get <NAME> --to <path> --mode append|replace|line [options] [--dry-run]
  aienv rm <NAME> [--yes]
  aienv rotate <NAME>
  aienv install [--yes]
  aienv install <target>
  aienv --version

get options:
  --placeholder <STRING>   required for --mode replace
  --line <N>               required for --mode line (1-indexed)
  --at <start:end|start:>  required for --mode line, byte range within the line

install (no target): copies this binary to /usr/local/bin/aienv, asks to
confirm unless --yes is given. This is what a downloaded release tarball's
"cd <dir> && ./aienv install" is meant to do.

install <target>: writes AI-tool integration guidance instead of installing
the binary itself.
  claude     global Claude Code skill (~/.claude/skills/aienv-secrets)
  agents     AGENTS.md in the current project (also: codex, antigravity)
  cursor     Cursor project rule (./.cursor/rules/aienv.mdc)
  windsurf   Windsurf project rule (./.windsurf/rules/aienv.md)
  all        install every target above
  list       show this list`)
}

// splitNameArg pulls the leading positional NAME argument off args before
// any flags are parsed. The stdlib flag package stops parsing at the first
// non-flag token, so with a "<command> <NAME> --flag value" layout the name
// must be consumed manually rather than left for fs.Parse.
func splitNameArg(command string, args []string) (name string, rest []string, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, fmt.Errorf("usage: aienv %s <NAME> [options]", command)
	}
	return args[0], args[1:], nil
}

// validateKeyName restricts secret names to a safe identifier pattern,
// since the name is also used to derive a filename inside the vault.
func validateKeyName(name string) error {
	if !keyNamePattern.MatchString(name) {
		return fmt.Errorf("invalid key name %q: must match %s", name, keyNamePattern.String())
	}
	return nil
}

// validateDestPath ensures the destination file for `get` is not inside
// ~/.aienv, so a mistaken --to can never clobber the registry or vault.
func (e env) validateDestPath(dest string) (string, error) {
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("resolving destination path: %w", err)
	}
	absDest = filepath.Clean(absDest)

	absBase, err := filepath.Abs(e.baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base directory: %w", err)
	}
	absBase = filepath.Clean(absBase)

	if absDest == absBase || strings.HasPrefix(absDest, absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("destination %s is inside %s, refusing to write there", dest, e.baseDir)
	}
	return absDest, nil
}

func (e env) cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Parse(args)

	reg, err := registry.Load(e.registryPath)
	if err != nil {
		return err
	}

	names := reg.List()
	if len(names) == 0 {
		fmt.Println("(no secrets registered)")
		return nil
	}
	for _, name := range names {
		entry, _ := reg.Get(name)
		fmt.Printf("%-40s created %s\n", name, entry.Created)
	}
	return nil
}

// cmdExists is the safe alternative to grepping/catting a file to check
// whether a secret is registered or already injected somewhere: it prints
// only "yes"/"no" (exit 0/1) and never the value itself.
func (e env) cmdExists(args []string) error {
	name, rest, err := splitNameArg("exists", args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("exists", flag.ExitOnError)
	in := fs.String("in", "", "also check whether NAME's current value already appears in this file")
	fs.Parse(rest)

	reg, err := registry.Load(e.registryPath)
	if err != nil {
		return err
	}
	entry, ok := reg.Get(name)
	if !ok {
		fmt.Println("no")
		os.Exit(1)
	}

	if *in == "" {
		fmt.Println("yes")
		return nil
	}

	destPath, err := e.validateDestPath(*in)
	if err != nil {
		return err
	}
	fullVaultPath := filepath.Join(e.baseDir, entry.File)
	value, err := vault.ReadSecret(fullVaultPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no")
			os.Exit(1)
		}
		return fmt.Errorf("reading %s: %w", *in, err)
	}

	if bytes.Contains(data, value) {
		fmt.Println("yes")
		return nil
	}
	fmt.Println("no")
	os.Exit(1)
	return nil
}

func (e env) cmdPut(args []string) error {
	name, rest, err := splitNameArg("put", args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("put", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite an existing key")
	fs.Parse(rest)

	if err := validateKeyName(name); err != nil {
		return err
	}

	reg, err := registry.Load(e.registryPath)
	if err != nil {
		return err
	}
	if reg.Has(name) && !*force {
		return fmt.Errorf("key %q already exists, use --force to overwrite", name)
	}

	value, err := readSecretInteractive(fmt.Sprintf("Value for %s: ", name))
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return fmt.Errorf("empty value refused")
	}

	relFile := filepath.Join("vault", strings.ToLower(name)+".key")
	fullPath := filepath.Join(e.baseDir, relFile)

	if err := vault.WriteSecret(fullPath, value); err != nil {
		return err
	}
	reg.Put(name, registry.Entry{File: relFile, Created: time.Now().UTC().Format(time.RFC3339)})
	if err := reg.Save(); err != nil {
		return err
	}

	fmt.Printf("✓ stored %s (%d chars)\n", name, len(value))
	return nil
}

func (e env) cmdRotate(args []string) error {
	name, rest, err := splitNameArg("rotate", args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("rotate", flag.ExitOnError)
	fs.Parse(rest)

	if err := validateKeyName(name); err != nil {
		return err
	}

	reg, err := registry.Load(e.registryPath)
	if err != nil {
		return err
	}

	relFile := filepath.Join("vault", strings.ToLower(name)+".key")
	fullPath := filepath.Join(e.baseDir, relFile)

	if entry, ok := reg.Get(name); ok {
		oldFullPath := filepath.Join(e.baseDir, entry.File)
		if vault.Exists(oldFullPath) {
			backupRel := filepath.Join("vault", "."+strings.ToLower(name)+".key.bak")
			backupFull := filepath.Join(e.baseDir, backupRel)
			if err := vault.CopyFile(oldFullPath, backupFull); err != nil {
				return fmt.Errorf("backing up previous value: %w", err)
			}
		}
	}

	value, err := readSecretInteractive(fmt.Sprintf("New value for %s: ", name))
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return fmt.Errorf("empty value refused")
	}

	if err := vault.WriteSecret(fullPath, value); err != nil {
		return err
	}
	reg.Put(name, registry.Entry{File: relFile, Created: time.Now().UTC().Format(time.RFC3339)})
	if err := reg.Save(); err != nil {
		return err
	}

	fmt.Printf("✓ rotated %s (%d chars, previous value backed up)\n", name, len(value))
	return nil
}

func (e env) cmdRm(args []string) error {
	name, rest, err := splitNameArg("rm", args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	fs.Parse(rest)

	reg, err := registry.Load(e.registryPath)
	if err != nil {
		return err
	}
	entry, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("key %q not found", name)
	}

	if !*yes {
		fmt.Fprintf(os.Stderr, "Remove %s? [y/N] ", name)
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			fmt.Println("aborted")
			return nil
		}
	}

	fullPath := filepath.Join(e.baseDir, entry.File)
	if err := vault.Remove(fullPath); err != nil {
		return err
	}
	reg.Remove(name)
	if err := reg.Save(); err != nil {
		return err
	}

	fmt.Printf("✓ removed %s\n", name)
	return nil
}

func (e env) cmdGet(args []string) error {
	name, rest, err := splitNameArg("get", args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	to := fs.String("to", "", "destination file path")
	mode := fs.String("mode", "", "append|replace|line")
	placeholder := fs.String("placeholder", "", "placeholder string for --mode replace")
	line := fs.Int("line", 0, "1-indexed line number for --mode line")
	at := fs.String("at", "", `byte range "start:end" or "start:" for --mode line`)
	dryRun := fs.Bool("dry-run", false, "show length and sha256 of the value without writing anything")
	fs.Parse(rest)

	if *to == "" {
		return fmt.Errorf("--to is required")
	}
	if *mode != "append" && *mode != "replace" && *mode != "line" {
		return fmt.Errorf("--mode must be one of: append, replace, line")
	}

	reg, err := registry.Load(e.registryPath)
	if err != nil {
		return err
	}
	entry, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("key %q not found", name)
	}

	destPath, err := e.validateDestPath(*to)
	if err != nil {
		return err
	}

	fullVaultPath := filepath.Join(e.baseDir, entry.File)
	value, err := vault.ReadSecret(fullVaultPath)
	if err != nil {
		return err
	}

	if *dryRun {
		sum := sha256.Sum256(value)
		fmt.Printf("dry-run: %s -> %s (mode=%s, %d chars, sha256=%s)\n",
			name, *to, *mode, len(value), hex.EncodeToString(sum[:]))
		return nil
	}

	switch *mode {
	case "append":
		res, err := inject.Append(destPath, name, value)
		if err != nil {
			return err
		}
		fmt.Printf("✓ wrote %s to %s (append, %d chars)\n", name, *to, res.Chars)

	case "replace":
		if *placeholder == "" {
			return fmt.Errorf("--placeholder is required for --mode replace")
		}
		res, err := inject.Replace(destPath, *placeholder, value)
		if err != nil {
			return err
		}
		fmt.Printf("✓ replaced %d occurrence(s) of placeholder in %s\n", res.Occurrences, *to)

	case "line":
		if *line < 1 {
			return fmt.Errorf("--line is required for --mode line")
		}
		if *at == "" {
			return fmt.Errorf("--at is required for --mode line")
		}
		res, err := inject.Line(destPath, *line, *at, value)
		if err != nil {
			return err
		}
		fmt.Printf("✓ wrote %s to %s (line %d, %d chars)\n", name, *to, *line, res.Chars)
	}

	return nil
}

// cmdInstall either self-installs the running binary onto the system PATH
// (no target — the flow for a freshly downloaded/extracted release
// tarball), or writes aienv's AI-agent integration guidance to the
// location a given tool (Claude Code, Codex CLI, Antigravity, Cursor,
// Windsurf, ...) reads it from, so a user never has to hand-copy AGENTS.md
// or a skill file between projects themselves.
func (e env) cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	yes := fs.Bool("yes", false, "skip confirmation prompts")
	fs.Parse(args)

	if fs.NArg() == 0 {
		return cmdInstallSelf(*yes)
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aienv install [<target>] (run 'aienv install list' to see targets)")
	}
	name := strings.ToLower(fs.Arg(0))

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	if name == "list" {
		for _, t := range install.All() {
			fmt.Printf("%-10s %s\n", t.Name, t.Describe)
		}
		return nil
	}

	if name == "all" {
		for _, t := range install.All() {
			path, err := install.Write(t, home, cwd)
			if err != nil {
				return fmt.Errorf("installing %s: %w", t.Name, err)
			}
			fmt.Printf("✓ %s -> %s\n", t.Name, path)
		}
		return nil
	}

	t, ok := install.Resolve(name)
	if !ok {
		return fmt.Errorf("unknown install target %q (run 'aienv install list')", name)
	}
	path, err := install.Write(t, home, cwd)
	if err != nil {
		return err
	}
	fmt.Printf("✓ installed %s -> %s\n", t.Name, path)
	return nil
}

// selfInstallDir is where `aienv install` (no target) copies the running
// binary — the same location the README/Makefile already document.
const selfInstallDir = "/usr/local/bin"

// cmdInstallSelf lets a freshly downloaded/extracted release tarball
// bootstrap itself: cd into it, run `./aienv install`, confirm, done.
func cmdInstallSelf(yes bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving current binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	destPath := filepath.Join(selfInstallDir, "aienv")
	if exePath == destPath {
		fmt.Printf("aienv is already installed at %s\n", destPath)
		return nil
	}

	if !yes {
		fmt.Printf("Install aienv to %s? [y/N] ", destPath)
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			fmt.Println("aborted")
			return nil
		}
	}

	path, err := install.SelfInstall(exePath, selfInstallDir)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied writing to %s — re-run with: sudo aienv install --yes", selfInstallDir)
		}
		return err
	}

	fmt.Printf("✓ installed aienv -> %s\n", path)
	if !onPath(selfInstallDir) {
		fmt.Printf("note: %s is not on your PATH — add it to use the plain \"aienv\" command\n", selfInstallDir)
	}
	return nil
}

// onPath reports whether dir appears in $PATH.
func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}

// readSecretInteractive prompts on stderr and reads a value from the
// terminal with echo disabled, the same way `sudo` reads a password.
func readSecretInteractive(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("reading secret value: %w", err)
	}
	return value, nil
}
