# aienv

`aienv` is a small CLI that injects secrets into files on disk without ever
printing the secret value to stdout, stderr, or an error message. It is
designed to be driven by an AI agent (or a human): the caller tells `aienv`
*where* a secret should go — which file, which mode, which placeholder or
line — and `aienv` handles reading the value and writing it, but the value
itself never appears in any command output.

Secret values are stored once, outside of source control, in a local vault
under `~/.aienv/`, and are looked up by a logical name (e.g. `OPENAI_TOKEN`)
each time you need to inject them somewhere.

## Install

```sh
go build -o aienv ./cmd/aienv
sudo install -m 755 aienv /usr/local/bin/aienv
```

or, using the provided Makefile:

```sh
make install
```

## Commands

### `list`

List every registered secret name and when it was created. Values are never
shown.

```sh
$ aienv list
DB_PASSWORD                             created 2026-07-18T09:12:03Z
OPENAI_TOKEN                             created 2026-07-19T12:00:00Z
```

### `exists`

Check whether a secret is registered, and optionally whether its value is
already present in a destination file — without ever printing the value.
Prints `yes`/`no` and exits `0`/`1`, so it's safe for scripts (and AI agents)
to use instead of grepping or catting a file to find out:

```sh
$ aienv exists OPENAI_TOKEN
yes
$ aienv exists OPENAI_TOKEN --in .env
no
```

### `put`

Store a new secret under a logical name. The value is read from an
interactive, echo-disabled terminal prompt — there is no `--value` flag,
since passing the secret as a command-line argument would leak it into shell
history and process listings (`ps`). This is intentional.

```sh
$ aienv put OPENAI_TOKEN
Value for OPENAI_TOKEN: <hidden input, not echoed>
✓ stored OPENAI_TOKEN (51 chars)
```

Use `--force` to overwrite an existing key:

```sh
$ aienv put OPENAI_TOKEN --force
```

### `get`

Inject a stored secret into a destination file. `--to` and `--mode` are
required. Three modes are supported:

**`append`** — append `NAME=value` to the end of the file, creating it if
needed:

```sh
$ aienv get OPENAI_TOKEN --to .env --mode append
✓ wrote OPENAI_TOKEN to .env (append, 51 chars)
```

**`replace`** — substitute every occurrence of a placeholder string. The
destination file must already exist:

```sh
$ aienv get OPENAI_TOKEN --to config.yaml --mode replace --placeholder "__OPENAI_TOKEN__"
✓ replaced 1 occurrence(s) of placeholder in config.yaml
```

**`line`** — write into a specific 1-indexed line at a byte range
`start:end` (or `start:` for open-ended, i.e. "to the end of the line").
Missing lines are created as empty lines, and a line shorter than `start` is
padded with spaces first:

```sh
$ aienv get OPENAI_TOKEN --to config.ini --mode line --line 4 --at "12:"
✓ wrote OPENAI_TOKEN to config.ini (line 4, 51 chars)
```

**`--dry-run`** — works with any mode. Prints the value's length and SHA-256
checksum instead of writing anything, so you can verify a secret is what you
expect without ever seeing it or touching the destination file:

```sh
$ aienv get OPENAI_TOKEN --to .env --mode append --dry-run
dry-run: OPENAI_TOKEN -> .env (mode=append, 51 chars, sha256=3a7bd3e2360a...)
```

### `rm`

Remove a stored secret from the vault and the registry. Prompts for
confirmation unless `--yes` is given:

```sh
$ aienv rm OPENAI_TOKEN
Remove OPENAI_TOKEN? [y/N] y
✓ removed OPENAI_TOKEN
```

### `rotate`

Replace a secret's value with a new one, keeping a backup of the previous
value in the vault. Prompts interactively for the new value, same as `put`:

```sh
$ aienv rotate OPENAI_TOKEN
New value for OPENAI_TOKEN: <hidden input, not echoed>
✓ rotated OPENAI_TOKEN (54 chars, previous value backed up)
```

### `install`

Set up another AI tool to know about `aienv` automatically — writes (or
merges into) whatever config file that tool reads its instructions from:

```sh
$ aienv install claude      # global Claude Code skill (~/.claude/skills/aienv-secrets)
$ aienv install agents      # AGENTS.md in the current project (also: codex, antigravity)
$ aienv install cursor      # ./.cursor/rules/aienv.mdc
$ aienv install windsurf    # ./.windsurf/rules/aienv.md
$ aienv install all         # every target above
$ aienv install list        # show available targets
```

`agents`/`cursor`/`windsurf` are project-scoped — run them from inside the
project you want the integration in. `claude` is global and works
regardless of the current directory. Re-running any target is safe: it
overwrites (dedicated files) or merges without duplicating (`AGENTS.md`).

## For AI agents

If you're an AI coding agent working in a repo that uses `aienv`, run
`aienv install <target>` for your tool (see above) to load the exact usage
rules — including the one hard rule: never see or write a secret value
yourself, and never grep/cat a file to check if one is already there (use
`aienv exists` instead). [`AGENTS.md`](AGENTS.md) in this repo has the same
content if your tool doesn't auto-load it.

## Security model

- **Secret values never touch stdout, stderr, or logs.** `put` and `rotate`
  print only a character count; `get` prints only a character count (or, in
  `--dry-run`, a length and a SHA-256 checksum). No function in this codebase
  ever formats a secret value into an error message.
- **Vault files are locked down.** Each secret is stored as its own file
  under `~/.aienv/vault/`, written with `0600` (owner read/write only)
  permissions. The vault directory itself is `0700` (owner access only).
- **Destination writes are atomic.** Every write to a destination file (or to
  the registry) goes through a temp-file-plus-rename sequence in the same
  directory, so a crash mid-write can never leave a truncated or
  half-written file.
- **`--to` cannot target the vault.** The destination path passed to `get` is
  resolved to an absolute path and checked against `~/.aienv`; a `--to` that
  would land inside `~/.aienv` (accidentally or otherwise) is rejected, so a
  typo can't overwrite the registry or vault.

## Data layout

```
~/.aienv/
├── registry.json
└── vault/
    ├── openai_token.key
    └── db_password.key
```

`registry.json` is a plaintext index mapping each logical secret name to its
file inside the vault and its creation time. It never contains secret
values themselves — only metadata:

```json
{
  "OPENAI_TOKEN": {
    "file": "vault/openai_token.key",
    "created": "2026-07-19T12:00:00Z"
  }
}
```

Note that `put` lowercases the key name to derive the vault filename, so
`OPENAI_TOKEN` is stored at `vault/openai_token.key`.

Each `vault/*.key` file holds exactly one raw secret value, `0600`, with no
extra formatting.

## Example `.gitignore`

If you keep `~/.aienv` inside a project directory instead of your home
directory, make sure it's excluded from version control:

```
.aienv/
*.key
```
