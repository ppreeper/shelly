# `shelly` Command Reference

`shelly` is a POSIX `sh` CLI generator. It turns `src/shelly.yml` into one executable
`sh` script.

```text
shelly <command> [options]
```

## Synopsis

```
shelly init [--minimal|-m]
shelly generate [-s|--strict] [-v|--verbose] [-e|--env <env>] [-u|--upgrade]      (alias: g)
shelly preview
shelly validate [-v|--verbose]
shelly add <addon> [-u|--upgrade]
shelly add --source <dir> [<library>] [-u|--upgrade] [-l|--list] [-q|--quiet]
shelly version
```

---

## `shelly init`

Scaffold a new workspace with `src/shelly.yml`.

```sh
shelly init            # multi-command sample (commands:)
shelly init --minimal  # single-command sample (no commands:)
```

| Flag | Description |
|---|---|
| `-m`, `--minimal` | Write `templates/minimal.yml` instead of the full sample. |

After init, edit `src/shelly.yml`, then run `shelly generate`.

---

## `shelly generate`

Read `src/shelly.yml`, scaffold any missing command body files, and write the generated
script to `./<name>`.

```sh
shelly generate
shelly g
shelly generate --env production
shelly generate --strict      # also run shellcheck
shelly generate --upgrade     # refresh [@shelly-upgrade]-marked library files first
```

| Flag | Description |
|---|---|
| `-s`, `--strict` | Run `shellcheck -s sh` on the output and fail on any diagnostic. Requires `shellcheck` installed. |
| `-v`, `--verbose` | Print the parsed configuration before generating. |
| `-e`, `--env <env>` | Override the generation environment (`development` / `production`). Overrides the config's `env`. |
| `-u`, `--upgrade` | Scan the source tree for `[@shelly-upgrade]` markers and refresh each marked file before generating. |

Behavior:

- Missing `src/<name>_command.sh` files are created with a skeleton (once).
- `src/lib/*.sh` are emitted in callee-before-caller order: a body-only file is wrapped as `<stem>()`, while a file that defines functions is included verbatim.
- Duplicate library function names and dependency cycles are fatal errors.
- The output file is marked executable (`0755`).
- `env: production` omits `inspect_args` and view markers.
- If a `formatter` is configured, the assembled script is post-processed (`internal` squashes blank-line runs, `external`/`shfmt` runs `shfmt --case-indent --indent 2`, any other value is a custom stdin→stdout command).
- `--upgrade` refreshes files installed by `shelly add` and `shelly add --source` (see below); unknown libraries produce a warning, not an error.
- `[@shelly-upgrade]` marker lines are stripped from the generated script.

Alias: `shelly g`.

---

## `shelly preview`

Generate the script and print it to **stdout** without writing to disk.

```sh
shelly preview
```

Useful for inspecting output, diffing, or piping to another tool.

---

## `shelly validate`

Parse and check `src/shelly.yml`.

```sh
shelly validate
shelly validate --verbose
```

| Flag | Description |
|---|---|
| `-v`, `--verbose` | Print the parsed configuration (the merged config when split config is used). |

On success prints:

```
File read OK
```

---

## `shelly add`

Install a built-in addon, or a library from a custom source.

```sh
shelly add validations
shelly add colors
shelly add hooks
shelly add strings
shelly add yaml
shelly add ini
shelly add config
```

| Addon | File(s) created | Contents |
|---|---|---|
| `validations` | `src/lib/validations.sh` | `validate_not_empty`, `validate_integer`, `validate_file_exists`, `validate_dir_exists` |
| `colors` | `src/lib/colors.sh` | `red`, `green`, `yellow`, `blue`, `cyan`, `bold`, `dim` |
| `hooks` | `src/before.sh`, `src/after.sh` | Commented stubs |
| `preamble` | `src/preamble.sh` | Starter preamble |
| `strings` | `src/shelly-strings.yml` | Message-string overrides |
| `yaml` | `src/lib/yaml.sh` | `yaml_load` (sed/awk based; scalars and nested hashes) |
| `ini` | `src/lib/ini.sh` | `ini_load`, `ini_get`, `ini_set`, `ini_unset`, `ini_keys`, `ini_show`, `ini_save` (POSIX keyed helpers) |
| `config` | `src/lib/config.sh`, `src/lib/ini.sh` | `config_get`, `config_set`, `config_del`, `config_keys`, `config_show`, `config_has_key`, `config_reload` + the `ini` library |

| Flag | Description |
|---|---|
| `-u`, `--upgrade` | Overwrite existing files with the latest content. |
| `-s`, `--source <dir>` | Install from a libraries source directory containing a `libraries.yml`. |
| `-l`, `--list` | List the libraries available in the source (or the built-in addons). |
| `-q`, `--quiet` | Do not show post-install messages. |

Without `--upgrade`, built-in `add` errors if the target file already exists;
source libraries print `skipped <path> (exists)` and leave the file untouched.

### Custom library sources

`--source <dir>` loads `<dir>/libraries.yml`, where each key
is a library:

```yaml
hello:
  help: A greeting library
  usage: ""                      # optional usage suffix
  post_install_message: Add hello to your command body.
  skip_src_check: false          # bypass the "src dir must exist" guard
  files:
    - source: scripts/hello.sh
      target: '%{user_lib_dir}/hello.%{user_ext}'
```

Target paths support the path tokens:

| Token | Resolves to |
|---|---|
| `%{user_source_dir}` | `source_dir` (default `src`) |
| `%{user_target_dir}` | `target_dir` (default `.`) |
| `%{user_lib_dir}` | resolved library dir (default `src/lib`) |
| `%{user_ext}` | `sh` |

Installed files that contain the `[@shelly-upgrade]` marker have it replaced
with `[@shelly-upgrade <source>;<library>]` so they can be refreshed later.
Run `shelly generate --upgrade` (see above) to refresh every marked file in the
source tree; unknown libraries are reported as warnings.

```sh
shelly add --source ./my-libs --list
shelly add --source ./my-libs hello
shelly add --source ./my-libs hello --upgrade
```

---

## `shelly version`

Prints the shelly version (from build metadata via `internal/vcs`).

---

## Configuration file

See [`specification.md`](specification.md) §2 for the complete field reference.

### Minimal single-command example

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

### Multi-command example

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
    flags:
      - long: --force
        short: -f
    examples:
      - cli download example.com

  - name: upload
    alias: u
    help: Upload a file
    args:
      - name: source
        required: true
    flags:
      - long: --user
        short: -u
        required: true
```

---

## Generated script interface

### Variables available in command bodies

After parsing, each flag/arg is available as a global scalar:

| Declaration | Variable |
|---|---|
| `flags: [{long: --force, short: -f}]` | `${force}` |
| `flags: [{long: --dry-run}]` | `${dry_run}` |
| `flags: [{short: -q}]` | `${q}` |
| `args: [{name: source}]` | `${source}` |
| `catch_all: true` | `${other_args}` |

Booleans are `1` when set. Repeatable booleans increment a counter. Repeatable value
flags/args are space-delimited. See [`specification.md`](specification.md) §3.

### Examining arguments (debugging)

Command bodies are body-only, so to inspect parsed values you can either reference the
named variables above or call the generated `inspect_args` helper, which dumps every known
flag/arg/catch-all variable to **stderr**:

```sh
inspect_args          # dev builds only; omitted under env: production
echo "source=$source force=[$force] target=[$target]"
```

`inspect_args` walks the root and all commands (including nested ones) and reads each value
with `${var:-}`, so it is safe under `set -u`. Override it with `src/inspect_args.sh`. A raw
loop over `"$@"` also works: `for a in "$@"; do echo "arg: $a"; done`.

### Project layout

```
myapp/
├── myapp                  # generated executable (shelly generate)
├── src/
│   ├── shelly.yml         # your configuration
│   ├── root_command.sh    # single-command body (root mode)
│   ├── <name>_command.sh  # per-command body (body only)
│   ├── before.sh          # before_hook body (optional)
│   ├── after.sh           # after_hook body (optional)
│   ├── header.sh          # content after shebang (optional)
│   ├── preamble.sh        # content before functions (optional)
│   ├── initialize.sh      # initialize() override (optional)
│   ├── run_wrapper.sh     # run() override (optional)
│   ├── start.sh           # entry point override (optional)
│   └── lib/*.sh           # library functions (body only)
└── settings...            # (none — settings live in shelly.yml)
```

Command body files contain **only the body** — no function header/footer. shelly wraps
them.

### `%APP_NAME%` and `%VERSION%`

Replaceable tokens in override files, substituted at generation time.

---

## Generated script behavior

- Shebang: `#!/bin/sh`.
- Single-pass argument parsing (flags and positionals interleaved).
- `--flag=value` and compact short forms are normalized safely.
- Unknown flags: error to stderr, exit `2`.
- Unknown commands: usage, exit `2`.
- `-h`/`--help`: usage; `--version`: version.
- Library functions emitted callee-before-caller; duplicates and cycles are fatal.
- Passes `shellcheck -s sh`.

---

## Typical workflow

```sh
mkdir myapp && cd myapp
shelly init
# edit src/shelly.yml
# edit src/<command>_command.sh files
shelly validate --verbose
shelly preview
shelly generate --strict
./myapp --help
```

---

## Related documents

- [`specification.md`](specification.md) — normative behavior specification.
