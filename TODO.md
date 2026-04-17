# TODO — Bashly Feature Gaps

Items are grouped by impact. Check off as implemented.

---

## High Impact (broken or incorrect behavior)

- [x] **`argfile` fix** — fixed: uses `while IFS= read` loop, skips blank/comment lines, only prepends `-`-prefixed lines, `eval set --` for proper word-splitting.

- [x] **`catch_help: true` passthrough** — fixed: when `catch_help: true`, `-h|--help` falls through to `other_args` accumulation instead of calling usage+exit.

- [x] **`--` separator into positionals** — already correct: `--) shift; break` preserves `$@`, and `<cmd>_command` passes `"$@"` to `<cmd>_parse_args`.

- [x] **`expose: "always"` variant** — fixed: `ExposeField` custom type (mirrors `ExtensibleField`); `expose: always` emits inline subcommand listing in the zero-arg branch of the sub-dispatcher in addition to root help.

- [x] **`Variable` array/hash values** — fixed: `VariableValue` custom YAML type returns a clear error when a YAML array or map is provided; plain strings unmarshal unchanged.

- [x] **Private flag/env var reveal** — fixed: `writeFlagsHelp` and `writeEnvVarsHelp` now accept a `revealKey` parameter and emit `if [ -n "${SHELLY_PRIVATE_REVEAL}" ]` blocks mirroring the command reveal pattern.

---

## Medium Impact (missing ergonomics)

- [x] **`show_examples_on_error`** — when a required arg is missing, the error path calls `<cmd>_usage` (which may not include examples in its output path). Add a setting to show the examples block specifically in the required-arg error message.

- [x] **`help_header_override`** — per-app and per-command field that emits a custom block at the top of the usage function, replacing the default `echo "<name> - <help>"` line.

- [x] **`catch_all.required`** — logic implemented (`generateFlagParser` trims `other_args` and errors if empty when `Required: true`); tests: `TestCatchAllRequired`, `TestCatchAllRequiredDefaultLabel`.

- [x] **`environment_variables.default`** — logic implemented (`generateEnvCheck` emits `: "${VAR:=default}"`); tests: `TestEnvVarDefault`, `TestEnvVarDefaultPerCommand`.

---

## Low Impact / Infrastructure

- [x] **`formatter` post-processing** — after generation, optionally run `shfmt` (or a custom command) on the output. Add `formatter: shfmt` (or `none` / `internal`) to settings.

- [x] **`word_wrap`** — wrap help text at a configurable column width (default 80) in generated usage functions. Currently long help strings are emitted raw.

- [x] **`enable_view_markers` toggle** — `# :command.*` section markers are always emitted. Add a settings flag (`disable_view_markers: true`) to suppress them for cleaner production output.

- [x] **`generate --env production`** — slim/production mode that strips `inspect_args`, view markers, and debug hooks from the generated script. Mirrors bashly's `env: production` setting.

- [x] **`inspect_args()` dev utility** — the current stub is a no-op. In dev mode it should print all parsed variable values (`$flagname`, `$argname`, `$other_args`) to stderr for debugging. Could be wired to `SHELLY_DEBUG=1`. Mirrors bashly's `enable_inspect_args` setting.

- [x] **`generate --upgrade`** — re-scaffold any addon lib files that have changed (currently `addAddon` errors if the file already exists; upgrade should overwrite with the latest built-in content).

- [x] **`tab_indent` setting** — generated scripts use 2-space indentation. Add `tab_indent: true` to switch to hard tabs (matches bashly's `tab_indent` setting).

- [x] **`enable_header_comment` toggle** — optionally suppress the "do not modify" / generation header comment at the top of the generated script.

- [x] **`usage_colors` settings** — emit ANSI color codes in usage output for section captions, commands, args, flags, and env vars. Requires color-safe `echo` helper; keys: `caption`, `command`, `arg`, `flag`, `environment_variable`. POSIX-compatible subset only (no `tput` assumptions).

- [x] **`function_names` overrides** — allow renaming internal generated functions `run()` and `initialize()` via `function_names.run` and `function_names.initialize` settings.

- [x] **`var_aliases` settings** — allow renaming the public `other_args` variable via `var_aliases.other_args`. Useful to avoid collisions in large scripts.

- [x] **`help` multiline support** — bashly treats the first line of a multiline `help:` value as the summary and subsequent lines as extended description shown in full usage. Shelly currently emits the entire string as a single `echo` line.

---

## Intentionally Excluded

These are bashly features that shelly deliberately does not implement:

- **Shell completions** — requires bash 4+; shelly targets POSIX sh.
- **`$args` associative array** — bash 4.2+ only; incompatible with POSIX sh target.
- **ERB preprocessing in config** — bashly is Ruby-based; shelly has no config templating.
- **Bash3 bouncer / sourcing guard** — bash-specific syntax (`[[ ]]`).
- **`bashly render`** (markdown/man page generation) — out of scope.
- **`generate --watch`** — filesystem watcher not yet implemented (no watcher dep in go.mod).
- **`enable_bash3_bouncer`** — bash-specific.
- **`enable_sourcing` guard** — uses `[[ "${BASH_SOURCE[0]}" == "$0" ]]`, bash-specific.
- **`enable_deps_array` / `enable_env_var_names_array`** — relies on bash associative arrays.
- **`compact_short_flags` / `conjoined_flag_args` normalization** — shelly's `normalize_input()` already handles `-abc` expansion and `--flag=value` splitting; these are always-on.
- **`watch_evented` / `watch_latency`** — watcher not implemented.
- **`var_aliases.args` / `var_aliases.deps`** — `$args` associative array excluded above.
