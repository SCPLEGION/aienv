`aienv` is a small, dependency-free Go CLI that injects secrets into files **without the calling AI agent ever seeing the secret value** — not in stdout, not in stderr, not in an error message, not in a shell pipeline. It exists so an agent can be trusted with "put my API key in `.env`" tasks end-to-end.

## The two hard rules

1. **Never make the secret value pass through you.** Don't ask the user to paste it into chat. Don't type it into a file yourself (Write/Edit/heredoc/echo). Don't pipe it into `aienv put`/`rotate` (e.g. `echo "$VALUE" | aienv put NAME` or `aienv put NAME <<< "$VALUE"`) — that still puts the value in your own command history and transcript even though `aienv` itself never prints it. If a secret isn't registered yet, tell the user to run `aienv put <NAME>` **themselves**, in their own terminal.
2. **Never inspect a file that might contain a secret just to answer a yes/no question.** Don't `cat`/`grep`/read a destination file (`.env`, `config.yaml`, ...) to check "is this key already set?" or "did the write work?" — the result lands in your own output/context, which is exactly what this tool exists to prevent. Use `aienv exists` instead (below) — it answers yes/no without ever showing you the value.

## Commands

```
aienv list
  # registered names + created dates, never values

aienv exists <NAME> [--in <path>]
  # THE command for existence checks — never grep/cat a file for this instead.
  # no --in: is NAME registered in the vault? prints "yes"/"no", exit code 0/1.
  # --in <path>: is NAME's current value already present in <path>? (check this
  #   before an `append` to avoid duplicate lines) prints "yes"/"no", exit code 0/1.
  # Never prints the secret value itself, only the yes/no verdict.

aienv put <NAME> [--force]
  # interactive hidden-input prompt — run BY THE USER, never scripted by you
  # --force required to overwrite an existing name

aienv get <NAME> --to <path> --mode append|replace|line [options] [--dry-run]
  # injects the stored value into <path>; prints only a status line, e.g.:
  #   ✓ wrote OPENAI_TOKEN to dest.env (append, 51 chars)
  #   ✓ replaced 2 occurrence(s) of placeholder in config.yaml
  --mode append                     # appends "NAME=value\n", creates <path> if needed
  --mode replace --placeholder STR  # replaces every occurrence of STR (dest must exist)
  --mode line --line N --at "S:E"   # writes into line N (1-indexed) at byte range S:E
                                     # ("S:" is open-ended = "to end of line")
  --dry-run                         # prints only length + sha256, writes nothing —
                                     # use to sanity-check a value without touching anything

aienv rm <NAME> [--yes]
  # deletes the vault entry, asks for confirmation unless --yes

aienv rotate <NAME>
  # like `put --force`, backs up the previous value to vault/.<name>.key.bak first
  # (also interactive — run BY THE USER)

aienv install <target>
  # writes this same guidance into another AI tool's config
  # targets: claude, agents (= codex, antigravity), cursor, windsurf, all, list
```

`--to` and `--in` are both validated to reject any path inside `~/.aienv`, so they can never touch the registry or vault by accident.

## Typical workflow: "add my OpenAI key to .env"

1. `aienv exists OPENAI_TOKEN` — is it already registered?
2. If not: ask the user to run `aienv put OPENAI_TOKEN` themselves, then confirm when done.
3. `aienv exists OPENAI_TOKEN --in .env` — is it already injected there? If yes, stop, nothing to do.
4. `aienv get OPENAI_TOKEN --to .env --mode append`
5. Report the ✓ status line back to the user. Don't open `.env` afterward to "double check" it — that's rule 2 above. If you need more confidence before the real write, use `--dry-run` first.

## What NOT to do

- Don't `cat`/read/grep `~/.aienv/vault/*.key`, or any destination file, to check whether a secret is present — use `aienv exists`.
- Don't script a value into `put`/`rotate` via any pipe, heredoc, or quoted argument — there's no non-interactive value flag, by design.
- Don't echo a destination file's contents back to the user after a `get`.
