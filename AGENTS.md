# AGENTS.md — instructions for AI coding agents

This file follows the [AGENTS.md](https://agents.md) convention read by most
AI coding tools (Codex CLI, Cursor, Windsurf, Amp, opencode, Zed, and others).
If your tool doesn't auto-load it, paste this file's contents into context
before doing secret-related work in a repo that uses `aienv`.

## What this project is

`aienv` is a CLI that injects secrets (API keys, tokens, passwords) into
files on disk **without the calling AI agent ever seeing the secret value**
in stdout, stderr, or any error message. It's designed specifically so an
AI agent can be trusted with "put my API key in `.env`" tasks.

## The one hard rule for any agent using `aienv`

**Never ask the user to paste a secret value into the chat, and never write
a secret value into a file yourself.** Both defeat the entire purpose of
this tool. If a secret isn't registered yet, ask the user to run
`aienv put <NAME>` themselves in their own terminal — it prompts with
hidden input, like `sudo`. There is no non-interactive `--value` flag, by
design: passing a secret as a CLI argument would leak it into shell history
and process listings.

Your role is to call `aienv list` / `aienv get` / `aienv rm` / `aienv rotate`
and act on their (secret-free) status output — never the value itself.

## Command reference

```
aienv list
  # prints registered names + created dates, never values

aienv put <NAME> [--force]
  # interactive hidden-input prompt (run BY THE USER, not the agent)

aienv get <NAME> --to <path> --mode append|replace|line [options] [--dry-run]
  # injects the stored value into <path>; prints only a status line:
  #   ✓ wrote OPENAI_TOKEN to dest.env (append, 51 chars)
  #   ✓ replaced 2 occurrence(s) of placeholder in config.yaml

  --mode append                     # appends "NAME=value\n", creates <path> if needed
  --mode replace --placeholder STR  # replaces every occurrence of STR (dest must exist)
  --mode line --line N --at "S:E"   # writes into line N (1-indexed) at byte range S:E
                                     # ("S:" is open-ended = "from S to end of line")
  --dry-run                         # prints only length + sha256 of the value, writes nothing

aienv rm <NAME> [--yes]
  # deletes the vault entry, asks for confirmation unless --yes

aienv rotate <NAME>
  # like `put --force`, backs up the previous value to vault/.<name>.key.bak first
  # (also interactive — run BY THE USER)
```

`--to` rejects any destination path inside `~/.aienv`, so it can never
overwrite the registry or vault by mistake.

## What NOT to do

- Don't read `~/.aienv/vault/*.key` files directly.
- Don't try to script a value into `put`/`rotate` (e.g. via heredoc or a
  quoted argument) on the user's behalf — stop and ask them to run it.
- Don't cat/print a destination file back after a `get` if that would echo
  the secret into your own output. Use `--dry-run` (hash/length only) to
  verify correctness instead.

## Development on this repo itself

Standard Go workflow: `go build ./...`, `go vet ./...`, `go test ./...`.
See `README.md` for the full command reference and security model, and
`Makefile` for `build`/`install`/`test`/`clean` targets.
