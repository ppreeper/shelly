# shelly Behavior Specification

This document specifies the intended behavior of **shelly**, a POSIX `sh` CLI generator.

It is written normatively: **MUST**, **SHOULD**, and **MAY** carry their usual RFC 2119
meaning. Where shelly intentionally diverges from other generators, the divergence is called out.

Companion documents:

- [`commands.md`](commands.md) — the `shelly` CLI reference.

---

## 1. Scope

shelly reads a YAML description file (`src/shelly.yml`) and emits **one executable POSIX
`sh` script**. The generated script MUST run on any system providing `/bin/sh` and the
POSIX utilities, and MUST pass `shellcheck -s sh`.

shelly MUST NOT emit bashisms: no arrays, no `[[ ]]`, no `local`, no `declare`, no
process substitution, no `${var^^}`, no `$'...'` (except where strictly required and
guarded), and no `function` keyword.

---

## 2. Input file

The configuration file is `src/shelly.yml` by default (relative to the current
directory); a root-level `shelly.yml` is also discovered. When found, `source_dir`
defaults to the directory containing it.

Two shapes are valid:

- **Root mode** — no `commands:` key. The root command has args/flags and a single body.
- **Command mode** — a `commands:` list. The root may also carry global `flags:`.

### 2.1 Root configuration fields

| Field | Type | Required | Behavior |
|---|---|---|---|
| `name` | string | yes | Executable name; used for `%APP_NAME%` and the output filename. |
| `help` | string | no | First line is the summary; later lines are the long description. |
| `version` | string | no | Printed by `--version`. |
| `footer` | string | no | Appended to usage output. |
| `commands` | list | no | Subcommands. Presence selects command mode. |
| `args` | list | no | Root positional args (root mode only). |
| `flags` | list | no | Global flags (also parsed before subcommand dispatch in command mode). |
| `environment_variables` | list | no | Declared env vars. |
| `dependencies` | list or map | no | Required external commands. Accepts the three shorthand forms. |
| `variables` | list | no | Shell variables set during initialize. |
| `examples` | list | no | Example invocations. |
| `extensible` | bool or string | no | Unknown-command dispatch. |
| `private_reveal_key` | string | no | Env var that reveals private items (default `SHELLY_PRIVATE_REVEAL`). |
| `strict` | bool or string | no | Shell `set` directive: `false`→`set -e`, `true`→`set -euo pipefail` + `IFS`, `""`→none, any other string→emitted verbatim (default `false`). |
| `word_wrap` | int or bool | no | Wrap usage text at this column. Unset defaults to `80`; `0` or `false` disables wrapping. |
| `formatter` | string | no | Post-process the generated script: `""`/`none` (default) skips, `internal` squashes runs of blank lines, `external` (alias `shfmt`) runs `shfmt --case-indent --indent 2` via stdin→stdout, any other value is a custom command read from stdin and written to stdout. Runs before `tab_indent`. |
| `help_header_override` | string | no | Replaces the usage header line. |
| `disable_view_markers` | bool | no | Suppress `# :command.*` markers. |
| `disable_header_comment` | bool | no | Suppress the generated header comment. |
| `tab_indent` | bool | no | Indent the generated script with hard tabs instead of 2 spaces. Applied to the whole output. |
| `env` | string | no | `""`, `development`, or `production`. |
| `function_names` | map | no | Override `run` and `initialize` function names. |
| `var_aliases` | map | no | Override the `other_args` variable name. |
| `usage_colors` | map | no | Color per usage element (`caption`, `command`, `arg`, `flag`, `environment_variable`). Accepts color-function names or a raw ANSI sequence. |
| `compact_short_flags` | bool | no | Expand `-abc` into `-a -b -c` (default `true`). |
| `conjoined_flag_args` | bool | no | Accept `-a=value` (default `true`). `--flag=value` is always accepted. |
| `source_dir` | string | no | Directory for src files (config, command bodies, lib). Default `src`. |
| `target_dir` | string | no | Directory the generated script is written to. Default `.`. |
| `lib_dir` | string | no | Library directory, relative to `source_dir` (or absolute). Default `lib`. |
| `extra_lib_dirs` | list or string | no | Additional library dirs searched after `lib_dir`: `~` (none), a YAML list, or a comma-separated string. Relative to the CWD or absolute; `~/` expands to `$HOME`. |
| `commands_dir` | string | no | When set (not `~`/empty), each command body lives in a per-command subfolder `<source_dir>/<commands_dir>/<name>/<name>_command.sh` instead of the flat `<source_dir>/<name>_command.sh`. |

Path settings default to the historical layout (`src/`, `./`, `src/lib/`), so existing
projects are unchanged. `lib_dir` is resolved relative to `source_dir`; `extra_lib_dirs`
are resolved relative to the CWD (or absolute). When `source_dir` is unset, it defaults to
the directory containing the discovered config (`src/shelly.yml` → `src`; a root-level
`shelly.yml` → `.`).

### 2.2 Command fields

`name` (required), `alias`, `aliases`, `help`, `args`, `flags`, `commands`, `examples`,
`environment_variables`, `dependencies`, `catch_all`, `default`, `private`, `group`,
`footer`, `filters`, `expose`, `function`, `filename`, `variables`, `argfile`,
`help_header_override`, `show_examples_on_error`.

Rules:

- A command with `commands` MUST NOT also declare its own `args` or `flags`.
- A `default` command MUST NOT be combined with `extensible`.
- Top-level command names MUST be unique.
- `needs`/`conflicts` entries MUST reference defined flags of the same command.
- `argfile` MAY be `true` (read the legacy default path `~/.config/<app>/<cmd>`) or a
  string path resolved relative to the CWD. File lines supply default flags with these
  semantics: only lines beginning (after optional leading spaces) with `-`/`--` are read,
  a flag's value may appear on the same line, matching outer `"`/`'` quotes are stripped,
  and keys that match no flag spelling are ignored.
- `alias` (single) and `aliases` (list) declare alternate names for a command; an exact
  alias MUST be unique among sibling command names and aliases.
- An alias ending in `*` is a **prefix alias**: it matches any command token that begins
  with the prefix (`fo*` matches `fo`, `foobar`, `foo-x`). Exact command names and exact
  aliases take precedence over prefix aliases. A prefix alias MUST have a non-empty prefix
  and MUST NOT equal or shadow a sibling command name/alias; two prefix aliases on
  different commands where one is a prefix of the other are ambiguous.

### 2.3 Argument fields

`name` (required), `required`, `default`, `default_list`, `allowed`, `help`,
`repeatable`, `unique`, `validate`, `validate_list`.

- An optional argument (`default` present) MUST NOT precede a required argument.
- Only the **last** argument MAY be `repeatable`.

### 2.4 Flag fields

`long`, `short`, `alias`, `aliases`, `arg`, `negatable`, `required`, `default`,
`default_list`, `allowed`, `help`, `repeatable`, `unique`, `conflicts`, `needs`,
`private`, `validate`, `validate_list`.

- At least one of `long`/`short` MUST be present.
- `long` MUST begin with `--`; `short` MUST begin with `-` and be a single character.
- `alias` (single spelling) and `aliases` (list) declare additional spellings that parse
  to the same canonical flag. They MUST NOT change the canonical variable name and MUST
  carry the same arity (`arg`/`repeatable`) as the flag.
- `conflicts`/`needs` entries MAY reference any spelling (long, short, or alias); they
  resolve to the canonical flag of the owning spelling.
- `negatable` (boolean long flag) additionally accepts `--no-<long>`, which unsets the
  canonical value. It MUST have a `long` spelling and MUST NOT be combined with `arg`,
  `repeatable`, or `required`. Usage renders the form as `--[no-]<long>`.
- `unique` is valid only when `repeatable` and `arg` are both set.

### 2.5 Environment variable fields

`name`, `help`, `required`, `default`, `allowed`, `private`, `validate`, `validate_list`.

### 2.6 Dependency fields

`name`, `command` (list = OR alternatives), `version` (substring/grep check), `help`.

`dependencies` accepts three equivalent forms, all normalized to the
object above:

- a sequence of names — `dependencies: [curl, git]` (equivalent to `- name: curl`).
- a mapping of name to help text — `dependencies: { curl: "Install curl" }`.
- the object form, as a sequence (`- name: curl`) or a mapping
  (`http_client: { command: [curl, wget], help: ... }`).

Found commands are exposed as `deps_<name>` scalars and via the `deps_get` helper (see §3.6).

### 2.7 Variable fields

`name`, `value` (scalar only). Array/map values MUST be rejected with a clear error, as
POSIX `sh` has no arrays. This exposes the generator's `variables` feature minus
array/hash support.

### 2.8 Split configuration (`import`)

A configuration MAY be split across files using `import` directives anywhere in the
tree:

- An `import` key whose value is a path to a YAML file, or to a `.sh` file with YAML
  **front matter** above its first `---` line.
- The imported mapping is **merged** into the containing mapping (imported keys win);
  an imported sequence **replaces** the containing node. This lets a command file do
  `flags:\n  import: common_flags.yml` to share flags.
- Paths are resolved relative to the importing file's directory first, then as written
  (CWD-relative).
- Missing files, unparseable YAML, and non-mapping/sequence roots MUST fail with a
  clear error; a missing import file is reported as `cannot find import file "…"`.
- After merging, duplicate command names are reported by `shelly validate`
  (`duplicate command name: "…"`).
- `shelly validate -v` prints the fully merged configuration.

Example:

```yaml
# src/shelly.yml
commands:
  - import: download.yml
  - import: upload.sh
```

---

## 3. Variable naming (POSIX equivalent of `args[...]`)

Because POSIX `sh` has no associative arrays, each parsed value becomes **one global
scalar variable**. This section fixes the naming so user bodies can rely on it.

### 3.1 Flag variables

Given a flag with `long: --output` and/or `short: -o`:

- If `long` is present, the variable name is derived from `long` by stripping the leading
  `--` and replacing every remaining `-` with `_`.
  `--output` → `output`; `--dry-run` → `dry_run`.
- If only `short` is present, the variable name is the short letter.
  `-f` → `f`.
- The value is the flag's argument (`$arg`) for value flags, `1` for booleans, or an
  integer counter for repeatable booleans.

These names are plain shell scalars, e.g. `$output`, `$dry_run`, `$f`.

### 3.2 Argument variables

The variable name is the argument's `name` with `-` → `_` (names are already lowercase
identifiers).

The name is used directly as a scalar, e.g. `$source`.

### 3.3 Catch-all variable

The catch-all value is a single space-delimited scalar named `other_args` (renameable via
`var_aliases.other_args`).

The catch-all is a single space-delimited **string**, not an array. Consumers
MUST NOT rely on word boundaries being preserved for values containing whitespace.

### 3.4 Scoping and collisions

- All parsed variables are **global** (POSIX `sh` has no locals). A variable set by one
  command remains visible to later commands in the same process.
- To avoid collisions between same-named flags/args of sibling commands, nested command
  functions use a `prefix_name` function base (see §6). Variable names remain the plain
  sanitized names; authors SHOULD avoid declaring the same flag/arg name across commands
  with different semantics.
- `function_names` and `var_aliases` exist to escape collisions if needed.

### 3.5 Defaults

Defaults MUST be applied **after** parsing, with `: "${var:=default}"` so that an unset
or empty value takes the default. For `default_list`, the default is the list joined by
spaces.

### 3.6 Dependency variables

Resolved dependency paths are exposed two ways:

- A stable scalar `deps_<name>` per dependency, where `<name>` is the dependency name with
  every character outside `[A-Za-z0-9_]` replaced by `_` (`http-client` → `deps_http_client`,
  `node.js` → `deps_node_js`). For an OR-list (`command: [curl, wget]`), each candidate that
  is found gets its own `deps_<candidate>` scalar (`deps_curl`, `deps_wget`).
- A generated `deps_get <name>` helper that prints the resolved path for a dependency's
  logical name (or the first available OR candidate), and returns non-zero (printing nothing)
  for an unknown name. It is emitted only when at least one dependency is declared (at the
  root or in any command).

```sh
path="$(deps_get http_client)"
```

The lookup is either a function call, `$(deps_get http_client)`, or the scalar
`${deps_http_client}`; POSIX `sh` has no arrays.

### 3.7 Examining parsed arguments

Command bodies (body-only files such as `src/<command>_command.sh`) can inspect parsed
values three ways:

1. **Named variables** (§3.1–§3.6) — reference them directly, e.g. `$source`, `$force`,
   `$other_args`, `$deps_curl`.
2. **`inspect_args()`** — a generated dev helper that prints every known flag/arg/catch-all
   variable to **stderr**. It walks the root and all commands (including nested ones) and reads
   each value with `${var:-}`, so it is safe under `set -u`. Call it at the top of a body:
   `inspect_args`. It is emitted only in non-production builds (omitted under
   `env: production` / `generate -e production`) and can be overridden via
   `src/inspect_args.sh`.
3. **A raw `"$@"` loop** — the command body still sees the arguments as they were dispatched,
   e.g. `for a in "$@"; do echo "arg: $a" >&2; done`.

The `inspect_args` helper is provided for explicit calls rather than being
auto-invoked by command bodies.

---

## 4. Argument parsing (the core rule)

> **Single-pass invariant.** The generated parser MUST traverse the argument vector exactly
> once. Each token MUST be classified as: a flag, a positional argument, the end-of-options
> separator `--`, an unknown flag, or an unparsed extra (catch-all).

The parser walks a single `parse_requirements`-style loop and is the **fix** for the
earlier two-pass bug. It is **implemented**: each command
now has one `<cmd>_parse()` function (issue&nbsp;0001).

### 4.1 Loop structure

The old `<cmd>_parse_flags` and `<cmd>_parse_args` have been **replaced by one**
parser per command:

```sh
<cmd>_parse() {
  _options_ended=false
  _pos=0
  while [ $# -gt 0 ]; do
    _parse_key="$1"
    [ "$_options_ended" = true ] && _parse_key=""
    case "$_parse_key" in
      # one branch per flag (see 4.2)
      --) shift; _options_ended=true ;;
      -*) echo "Unknown flag: $1" >&2; exit 2 ;;
      *)
        _pos=$((_pos + 1))
        case "$_pos" in
          1) source="$1" ;;
          *) echo "Error: too many arguments: $1" >&2; <app>_<cmd>_usage; exit 1 ;;
        esac
        shift ;;
    esac
  done
  # post-parse checks (see 4.5)
}
```

The `run` dispatcher calls `<cmd>_parse "$@"`; root mode (no subcommands) uses
`root_parse()`; a root-level "global flags" pre-parser is emitted as `<appname>_parse()`
and invoked from `run` before dispatch. All three share the same single-pass structure.

`_options_ended`, `_pos`, and `_parse_key` are plain (non-`local`) shell variables; this is
intentional so the script stays clean under `shellcheck -s sh`.

### 4.2 Flag branches

For each flag, emit a branch for every spelling and form:

| Input form | Handling |
|---|---|
| `--long=value` | value = `${1#*=}`; shift once |
| `--long value` | value = `$2`; shift twice |
| `-s value` | value = `$2`; shift twice |
| `-s=value` | value = `${1#*=}`; shift once |
| boolean | set var to `1`; shift once |
| repeatable boolean | increment counter; shift once |
| repeatable value | append `"$2"` to the space-delimited string; shift twice |

`-h`/`--help` MUST print usage. With `catch_help: true` on a catch-all command, it MUST
instead be appended to `other_args` and parsing MUST continue.

### 4.3 Positional branches

Positional arguments MUST be consumed **in order**, each taking the next available
non-flag token. The parser increments a positional counter `_pos=$((_pos + 1))` and matches
it inside a nested `case "$_pos" in`:

- Simple: `<n>) <argvar>="$1" ;;` for the *n*-th declared argument.
- `repeatable` (last only): while `_pos` is at or beyond the repeatable argument's index,
  append `$1` to the space-delimited `<argvar>`.
- Overflow (more positionals than declared): if `catch_all`, append to `other_args`;
  otherwise emit `Error: too many arguments: $1` (plus usage, `exit 1`).

Positionals MUST NOT be read from raw `$1..$N` without classifying flags first — this is
the specific defect being corrected.

### 4.4 `--` end-of-options

On encountering `--`, the parser does `shift` and sets `_options_ended=true`; all
subsequent tokens are treated as positionals (the `_parse_key` is forced empty so they can
never match a flag branch). With `catch_all`, tokens beyond the declared positionals flow
into `other_args` via the overflow rule above. In the root-level *global flags* dispatcher
(a parser that only recognizes flags and stops at the first positional), `--` instead does
`shift; break`.

In every case, everything after `--` is positional.

### 4.5 Post-parse checks

After the single pass, in this order:

1. Catch-all `required` check.
2. Default injection (`: "${var:=default}"`).
3. Required flag checks, then required arg checks.
4. `allowed` checks.
5. `conflicts` and `needs` checks.
6. `unique` de-duplication for repeatable values.
7. `validate` / `validate_list` calls.

Check failures MUST print to **stderr** and `exit 1`. Unknown flags/commands exit `2`.

### 4.6 Input normalization

Input normalization is folded into the unified parser (issue&nbsp;0007); there is no
separate normalization pass and no `eval`. The parser expands these forms inline:
`--flag=value` (always, for value flags), `-a=value` when `conjoined_flag_args` is
enabled (default true), and a combined short-flag group `-abc` when
`compact_short_flags` is enabled (default true), by re-queuing the individual
flags `-a -b -c` for the same parse loop. `normalize_input()` remains as an
overridable no-op hook (`src/normalize_input.sh`) for compatibility.

---

## 5. Usage and help

### 5.1 Trigger points

- `-h` / `--help` → print usage for the current command.
- `--version` → print `version` (root).
- Running the root with no arguments and no `force` default → print root usage.
- Running a parent command with no subcommand → print its subcommand usage.
- Unknown command → print root usage, `exit 2`.
- Unknown flag → error to stderr, `exit 2`.

### 5.2 Root usage content

1. Header line: `<name> - <help-summary>` unless `help_header_override` is set.
2. `Usage: <name> <command> [options]`.
3. Optional long description.
4. **Commands** section, grouped by `group`. Private commands are hidden unless the
   reveal env var is non-empty.
5. **Arguments** section (root mode).
6. **Options / Flags** section.
7. **Environment Variables** section.
8. **Examples** section.
9. `footer`.

Layout: args/env are printed with a `%-20s` label field; flags with `%-24s`. Long labels
MUST wrap onto their own line.

### 5.3 Colors

When `usage_colors` sets a color for an element, the generator MUST emit the
corresponding ANSI sequence via `printf` (POSIX), e.g. `\033[32m…\033[0m`.

Color values MAY be given either as a color-function name or as a raw
ANSI / printf escape sequence. The supported names are this
vocabulary, resolved to plain POSIX `printf` codes:

- base colors: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`
- styles: `bold`, `dim`, `underlined`, `bold_underlined`
- compound forms: `<color>_bold` and `<color>_underlined` (e.g. `green_underlined` → `\033[4;32m`)

Names are matched case-insensitively. Any value beginning with `\033`, `\e`, or
the ESC byte is passed through unchanged, so existing raw-sequence configs keep
working. This is the POSIX-safe subset: it emits static escape codes and does not
depend on a runtime color library or `NO_COLOR`.

### 5.4 Word wrap

When `word_wrap` is a positive integer, help text MUST be wrapped at that
column. Unset it defaults to `80`; an explicit `0` or `false`
disables wrapping. (This is a behaviour change from earlier shelly versions,
which defaulted to no wrapping.)

### 5.5 Message strings

Generated messages (usage captions, error text, help/version flag text, and
`(required)` / `(default: …)` / `[allowed]` suffixes) come from a keyed table
of defaults that MAY be overridden by a message-strings file.

- The override file is `src/shelly-strings.yml`. A missing file is not an
  error.
- Any key present in the file replaces the corresponding default; keys not
  listed keep their default. Unknown keys are ignored.
- Templates MAY use generation-time `%{name}` placeholders (e.g. `%{name}`,
  `%{allowed}`, `%{values}`, `%{group}`, `%{version}`, `%{other}`, `%{need}`,
  `%{names}`) and runtime `%s` placeholders. Literal text is shell
  double-quote-escaped; `%s` is substituted with the relevant runtime shell
  expression (e.g. `$1`).
- `shelly add strings` scaffolds `src/shelly-strings.yml` with every key and
  its default.

### 5.6 Hidden comments

Lines whose leading whitespace is followed by `##` are build-time notes and MUST
be removed from the generated script. Stripping is applied to the fully
assembled output, so it covers command bodies (`src/<cmd>_command.sh`),
`src/lib/*.sh` libraries, and overridable partials alike.

- A `##`-prefixed line (optionally indented) is deleted; ordinary single-`#`
  shell comments are preserved.
- The `## [@shelly-upgrade …]` markers written by `shelly add` are removed the
  same way, so they never leak into the generated script.

---

## 6. Generation pipeline

### 6.1 `shelly generate`

1. Read and validate `src/shelly.yml`.
2. If `commands` is empty, scaffold `src/root_command.sh`; otherwise scaffold
   `src/<name>_command.sh` per top-level command (created once, never overwritten).
3. Discover `src/lib/*.sh`. A file that defines shell functions is emitted
   verbatim (so multi-function libraries work); a body-only file is wrapped as
   `<stem>() { ... }`.
4. Detect duplicate library function names → **fatal**.
5. Topologically sort library functions (callee before caller); detect cycles → **fatal**.
6. Emit sections in order (§6.2).
7. Strip hidden `##` comments — build metadata written into source files by
   `shelly add` (the `[@shelly-upgrade]` markers) and any other author notes —
   from the assembled script (see §5.6, §6.5).
8. Write the output file, `chmod 0755`.
9. If `strict` is set (CLI `-s`), run `shellcheck -s sh` and fail on diagnostics.
10. If `formatter` is set, run it on the assembled script (`internal` squashes blank-line runs; `external`/`shfmt` runs `shfmt --case-indent --indent 2`; any other value is a custom stdin→stdout command), then apply the `tab_indent` transform.

### 6.2 Emitted section order

1. Shebang `#!/bin/sh` (+ header comment unless disabled)
2. `src/header.sh`
3. `src/preamble.sh` (`%APP_NAME%`, `%VERSION%`)
4. `version_command`
5. Root usage (`src/usage.sh` or generated)
6. Per-command usage (`src/<name>_usage.sh` or generated)
7. `normalize_input` (no-op hook; `src/normalize_input.sh` or generated)
8. `inspect_args` (development only)
9. `parse_requirements` (`src/parse_requirements.sh` or stub)
10. Shared command helpers (`src/command_functions.sh`)
11. Library functions in topological order
12. Per-command parsers and `<cmd>_command` functions
13. `before_hook` / `after_hook` (if `src/before.sh` / `src/after.sh` present)
14. `initialize`
15. `run`
16. `start` entry point

### 6.3 Tokens

`%APP_NAME%` and `%VERSION%` MUST be substituted in all user-overridable source files.

### 6.4 Development vs. production

When `env: production` (or `generate -e production`):

- `inspect_args` MUST be omitted.
- View markers MUST be suppressed.

### 6.5 Library upgrade markers

Files installed by `shelly add <addon>` and `shelly add --source <dir> <library>`
carry a marker comment as their first line:

```
## [@shelly-upgrade <name>]              # built-in addon
## [@shelly-upgrade <source>;<library>]  # custom --source library
```

(`#` instead of `##` for YAML targets such as `shelly-strings.yml`.)

- `shelly generate --upgrade` (`-u`) scans the whole source tree before emitting
  the script and refreshes every marked file from its library. Built-in markers
  match by file basename; `source;library` markers match by resolved target path.
- Unknown libraries produce a **warning** on stderr (not a failure) and the file
  is left unchanged.
- Marker lines MUST be stripped from the generated script (§6.1 step 7; they are
  `##` comments, removed by the hidden-comment stripper, §5.6).

---

## 7. Runtime flow

```
start
 └─ initialize            # version, strict mode, global variables, env/deps checks
 └─ run "$@"
     ├─ normalize_input "$@"
     ├─ global flags parse (command mode with root flags)
     ├─ before_hook (if present)
     ├─ no args → force default OR usage
     ├─ dispatch: case "$1" → <cmd>_command "$@"   (single-pass parser inside)
     ├─ --version / -h|--help
     ├─ default command / extensible / unknown (exit 2)
     └─ after_hook (if present)
```

Note the **single-pass parser runs inside `<cmd>_command`**, so it receives the
subcommand's own argument vector and classifies flags and positionals together.

---

## 8. Overridable source files

| File | Replaces |
|---|---|
| `src/initialize.sh` | `initialize()` body |
| `src/usage.sh` | root usage function |
| `src/<name>_usage.sh` | per-command usage |
| `src/normalize_input.sh` | normalization hook (no-op by default) |
| `src/version_command.sh` | version function |
| `src/run_wrapper.sh` | entire `run()` |
| `src/start.sh` | entry point |
| `src/header.sh` | content after shebang |
| `src/preamble.sh` | content before functions |
| `src/before.sh` | `before_hook()` body |
| `src/after.sh` | `after_hook()` body |
| `src/parse_requirements.sh` | env/dep check hook |
| `src/inspect_args.sh` | inspect function |
| `src/lib/*.sh` | library functions (verbatim when the file defines functions; body-only files wrapped as `<stem>() { ... }`) |
| `src/<name>_command.sh` | command body |
| `src/shelly-strings.yml` | message-string overrides |

---

## 9. Exit codes

| Condition | Exit code |
|---|---|
| Success | 0 |
| Missing required arg/flag/env var | 1 |
| Validation / allowed / conflicts / needs failure | 1 |
| Dependency missing | 1 |
| Unknown flag | 2 |
| Unknown command / subcommand | 2 |
| `--help` / `--version` | 0 |

shelly exits `2` for invalid flags during parse (some tools use `1`). This convention
MUST be documented in generated usage if changed.

---

## 10. bash → POSIX design mapping

| bash idiom | shelly (POSIX) |
|---|---|
| `declare -A args=()` | one scalar per flag/arg |
| `${args[--flag]}` | `${flag}` |
| `declare -a other_args=()` | `other_args` space-delimited scalar |
| `declare -a input=()` | re-`set --` the argument vector |
| `declare -A deps=()` | `deps_<name>` scalars + `deps_get` helper |
| `declare -a env_var_names=()` | not supported |
| `declare -gn alias=x` | not supported; use distinct scalar names |
| `[[ ]]` tests | `[ ]` tests |
| `local` | not supported; use `_`-prefixed globals |
| single interleaved parse loop | single `<cmd>_parse` loop (**required**) |
| `run(); <cmd>_command` (no args) | same: command reads global scalars |
| `enable_sourcing` guard | not supported (bash-only) |
| completions | not supported (bash 4+) |
| ERB config | not supported |

---

## 11. Non-goals

shelly MUST NOT attempt to provide: shell completions, bash associative arrays
(`$args`, `$deps`, `$env_var_names`), ERB config preprocessing, a bash version bouncer, a
sourcing guard, template-based rendering, or `generate --watch`. These are bash- or
Ruby-specific or require tooling outside the POSIX target.
