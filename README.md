# shelly

**shelly** is a POSIX `sh` CLI generator. Describe your command-line tool in a YAML spec (`src/shelly.yml`) and shelly assembles a single, portable shell script — complete with argument parsing, flag handling, usage text, validation, and subcommand dispatch — that runs on any system with `/bin/sh`.

Inspired by [bashly](https://bashly.dannyb.co), shelly targets strict POSIX compliance (`shellcheck -s sh`) rather than bash-specific features.

---

## Installation

```sh
go install github.com/ppreeper/shelly@latest
```

Or build from source:

```sh
git clone https://github.com/ppreeper/shelly
cd shelly
go build -o shelly ./cmd/shelly
```

---

## Quick Start

```sh
# scaffold a new project
shelly init

# edit src/shelly.yml, then fill in src/<command>_command.sh body files

# generate the script
shelly generate

# preview without writing to disk
shelly preview

# validate the config
shelly validate
```

---

## How It Works

1. **Describe** your CLI in `src/shelly.yml`
2. **Fill in** command bodies in `src/<name>_command.sh` (body only — no function wrapper)
3. **Run** `shelly generate` — produces a single executable `sh` script

Library functions live in `src/lib/*.sh` (also body-only). shelly wraps them, resolves call order, and inlines everything.

---

## `src/shelly.yml` Reference

### Minimal single-command tool

```yaml
name: download
help: Download a file from a URL
version: 0.1.0

args:
  - name: source
    required: true
    help: URL to download from
  - name: target
    help: "Target filename (default: same as source)"

flags:
  - long: --force
    short: -f
    help: Overwrite existing files

examples:
  - download example.com
  - download example.com ./output -f
```

### Multi-command tool

```yaml
name: cli
help: My CLI tool
version: 0.1.0

environment_variables:
  - name: API_KEY
    help: API authentication key
    required: true

commands:
  - name: download
    alias: d
    help: Download a file
    args:
      - name: source
        required: true
        help: URL to download from
    flags:
      - long: --force
        short: -f
        help: Overwrite existing files
    examples:
      - cli download example.com

  - name: upload
    alias: u
    help: Upload a file
    args:
      - name: source
        required: true
        help: File to upload
    flags:
      - long: --user
        short: -u
        arg: user
        required: true
        help: Username
      - long: --password
        short: -p
        arg: password
        required: true
        help: Password
```

---

## Key Features

### Arguments

```yaml
args:
  - name: env
    required: true
    help: Target environment
    allowed: [production, staging, dev]

  - name: tags
    repeatable: true   # captures all remaining positional args
    unique: true       # deduplicate values
    help: Tags to apply
```

### Flags

```yaml
flags:
  - long: --output
    short: -o
    arg: file
    default: out.txt
    help: Output file path

  - long: --verbose
    short: -v
    repeatable: true   # -v -v -v increments a counter
    help: Increase verbosity

  - long: --format
    arg: format
    allowed: [json, yaml, text]
    default: text
    help: Output format

  - long: --with-deps
    help: Include dependencies
    needs: [--output]        # --with-deps requires --output
    conflicts: [--dry-run]   # mutually exclusive with --dry-run
```

### Validation

```yaml
flags:
  - long: --port
    arg: port
    validate: integer        # calls validate_integer at runtime

args:
  - name: config
    validate_list:           # multiple validators
      - not_empty
      - file_exists
```

Built-in validators (available via `shelly add validations`): `not_empty`, `integer`, `file_exists`, `dir_exists`.

### Environment Variables

```yaml
environment_variables:
  - name: LOG_LEVEL
    default: info
    allowed: [debug, info, warn, error]
    help: Log verbosity level

  - name: SECRET_TOKEN
    required: true
    private: true            # hidden from --help output
```

### Variables

Script-level shell variables set during `initialize()`:

```yaml
variables:
  - name: base_url
    value: https://api.example.com
  - name: timeout
    value: "30"
```

### Dependencies

```yaml
dependencies:
  - name: docker
    help: Install from https://docker.com
  - name: curl
    version: "7."            # checks: curl --version | grep "7."
  - command: [curl, wget]   # OR — either curl or wget satisfies this dep
    help: Install curl or wget
```

### Subcommands and Nesting

```yaml
commands:
  - name: remote
    help: Manage remotes
    commands:
      - name: add
        help: Add a remote
        args:
          - name: name
            required: true
          - name: url
            required: true
      - name: remove
        help: Remove a remote
        args:
          - name: name
            required: true
```

### Default and Force-Default Commands

```yaml
commands:
  - name: serve
    help: Start the server
    default: true     # catches unknown commands
    # default: force  # also runs when no args given
```

### Extensible Commands

```yaml
# dispatch unknown commands to <appname>-<cmd> in PATH (like git)
extensible: true

# or delegate to a specific tool
extensible: git
```

### Private Commands

```yaml
commands:
  - name: debug-dump
    help: Internal diagnostic
    private: true     # hidden from --help; shown when SHELLY_PRIVATE_REVEAL is set

# customize the reveal env var name
private_reveal_key: MYAPP_REVEAL
```

Private flags and env vars use the same reveal mechanism:

```yaml
flags:
  - long: --trace
    private: true

environment_variables:
  - name: DEBUG_TOKEN
    private: true
```

### Strict Mode

```yaml
strict: true   # emits: set -euo pipefail; IFS=$'\n\t'
```

### Global Flags (parsed before subcommand dispatch)

```yaml
flags:
  - long: --verbose
    short: -v
    help: Enable verbose output

commands:
  - name: build
    help: Build the project
```

### Argfile (load defaults from file)

```yaml
commands:
  - name: deploy
    argfile: true    # reads ~/.config/<appname>/deploy and prepends flags
    flags:
      - long: --env
        arg: environment
```

### Command Groups and Expose

```yaml
commands:
  - name: build
    group: "Build Commands"
    help: Build the project

  - name: remote
    help: Manage remotes
    expose: true     # lists remote's subcommands in root --help
    # expose: always # also shows subcommand listing when remote is called with no args
    commands:
      - name: add
      - name: remove
```

### Catch-All Extra Arguments

```yaml
commands:
  - name: run
    help: Run a command
    catch_all:
      label: cmd         # how extra args are shown in usage
      help: Command and arguments to run
      required: true     # fail if no extra args given
      catch_help: true   # pass --help through instead of triggering usage
```

Simple form (no sub-fields needed):

```yaml
commands:
  - name: exec
    catch_all: true
```

### Filters

```yaml
commands:
  - name: deploy
    filters: [filter_logged_in, filter_has_config]
    help: Deploy to production
```

`filter_logged_in` and `filter_has_config` are functions in `src/lib/` — if they print any output, the command is blocked.

### Custom Function Name

Override the generated function base name (useful to avoid naming collisions):

```yaml
commands:
  - name: list
    function: do_list
```

### Show Examples on Error

Display the command's examples block when a required arg or flag is missing:

```yaml
commands:
  - name: deploy
    show_examples_on_error: true
    args:
      - name: env
        required: true
    examples:
      - deploy production
      - deploy staging --dry-run
```

### Custom Help Header

Replace the default `<appname> <cmd> - <help>` line at the top of a usage function:

```yaml
# root-level override
help_header_override: "mytool v2 — the fast way to ship"

commands:
  - name: deploy
    help_header_override: "deploy — push to any environment"
```

### Hooks

```yaml
# src/before.sh runs in run() before command dispatch
# src/after.sh runs in run() after command dispatch
```

Scaffold them with:

```sh
shelly add hooks
```

### Header

Place custom content (copyright, generation notice) in `src/header.sh` — it is injected after the shebang.

---

## Settings

The following fields on the root config control generation behavior:

| Field | Type | Default | Description |
|---|---|---|---|
| `word_wrap` | int | `0` | Wrap help text at this column width in usage output. `0` disables wrapping. |
| `formatter` | string | `""` | Post-process the generated script. `"shfmt"` runs `shfmt -w`; `"none"` or empty skips. |
| `disable_view_markers` | bool | `false` | Suppress `# :command.*` section markers in the generated script. |
| `strict` | bool | `false` | Emit `set -euo pipefail; IFS=$'\n\t'` instead of `set -e`. |
| `private_reveal_key` | string | `SHELLY_PRIVATE_REVEAL` | Env var name that reveals private commands, flags, and env vars in `--help`. |

Example:

```yaml
name: mytool
version: 0.1.0
word_wrap: 100
formatter: shfmt
disable_view_markers: true
strict: true
private_reveal_key: MYTOOL_DEBUG
```

---

## `shelly add` Addons

| Addon | File created | Contents |
|---|---|---|
| `validations` | `src/lib/validations.sh` | `validate_not_empty`, `validate_integer`, `validate_file_exists`, `validate_dir_exists` |
| `colors` | `src/lib/colors.sh` | `red()`, `green()`, `yellow()`, `blue()`, `cyan()`, `bold()`, `dim()` |
| `hooks` | `src/before.sh`, `src/after.sh` | Commented stub files |

```sh
shelly add validations
shelly add colors
shelly add hooks
```

---

## User-Overridable Source Files

| File | Default behavior |
|---|---|
| `src/initialize.sh` | Sets version, `set -e`, global vars/deps/envs |
| `src/usage.sh` | Root `<appname>_usage()` |
| `src/<name>_usage.sh` | Per-command usage function |
| `src/normalize_input.sh` | Splits `--key=val` and `-abc` combos |
| `src/version_command.sh` | `echo "$version"` |
| `src/run_wrapper.sh` | Entire `run()` dispatcher |
| `src/start.sh` | Entry point (`initialize; run "$@"`) |
| `src/header.sh` | Content injected after shebang |
| `src/before.sh` | Injected at top of `run()` body |
| `src/after.sh` | Injected at bottom of `run()` body |
| `src/lib/*.sh` | Wrapped as `<stem>() { ... }` in topological order |
| `src/<name>_command.sh` | Command body (no function wrapper) |

Use `%APP_NAME%` as a substitution token in any override file.

---

## Generated Script Behavior

- Shebang: `#!/usr/bin/env sh`
- Flag normalization: `--flag=value` → `--flag value`, `-abc` → `-a -b -c`
- Unknown flags exit 2 with an error message
- Unknown commands exit 2 and print usage
- `--help` / `-h` prints usage; `--version` prints version
- Library functions are emitted in callee-before-caller order (topological sort)
- Duplicate library function names are a fatal error
- Generated scripts pass `shellcheck -s sh`

---

## CLI Reference

```
shelly init [--minimal]     Scaffold src/shelly.yml (--minimal for single-command mode)
shelly generate [-s] [-v]   Generate the script (-s runs shellcheck, -v verbose config)
shelly preview              Print generated script to stdout without writing to disk
shelly validate [-v]        Parse and validate src/shelly.yml
shelly add <addon>          Add a built-in addon (validations, colors, hooks)
```
