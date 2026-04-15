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

- [ ] **Alias wildcard dispatch** — `alias: "d*"` is collected verbatim by `allAliases()` but never emitted as a shell glob pattern in the `case` dispatcher. A `d*` pattern in a POSIX `case` statement is valid and should be emitted as-is.

- [ ] **Short-only flags** — `Flag.Long` is effectively required; a flag with only a `-f` short form cannot be expressed. The flag parser and usage generator need to handle `Long == ""` with only `Short` set.

- [x] **`show_examples_on_error`** — when a required arg is missing, the error path calls `<cmd>_usage` (which may not include examples in its output path). Add a setting to show the examples block specifically in the required-arg error message.

- [x] **`help_header_override`** — per-app and per-command field that emits a custom block at the top of the usage function, replacing the default `echo "<name> - <help>"` line.

- [ ] **`shelly add --list`** — no way to discover available addons. Add a `--list` flag to `shelly add` that prints the addon catalogue.

- [ ] **`shelly add yaml`** — scaffold `src/lib/yaml.sh` with a minimal POSIX YAML/key-value parser helper.

- [ ] **`shelly add config`** — scaffold `src/lib/config.sh` with INI-style config file read/write helpers.

- [ ] **`shelly add ini`** — scaffold `src/lib/ini.sh` with low-level INI section/key parsing helpers.

---

## Low Impact / Infrastructure

- [ ] **`settings.yml` concept** — all paths are hardcoded (`src/`, `src/lib/`, output to `./`). Add an optional `src/settings.yml` (or `shelly-settings.yml`) that controls:
  - `source_dir` (default: `src`)
  - `target_dir` (default: `.`)
  - `lib_dir` (default: `src/lib`)
  - `partials_extension` (default: `sh`)

- [x] **`formatter` post-processing** — after generation, optionally run `shfmt` (or a custom command) on the output. Add `formatter: shfmt` (or `none` / `internal`) to settings.

- [x] **`word_wrap`** — wrap help text at a configurable column width (default 80) in generated usage functions. Currently long help strings are emitted raw.

- [x] **`enable_view_markers` toggle** — `# :command.*` section markers are always emitted. Add a settings flag (`disable_view_markers: true`) to suppress them for cleaner production output.

- [ ] **`generate --env production`** — slim/production mode that strips `inspect_args`, view markers, and debug hooks from the generated script.

- [ ] **`inspect_args()` dev utility** — the current stub is a no-op. In dev mode it should print all parsed variable values (`$flagname`, `$argname`, `$other_args`) to stderr for debugging. Could be wired to `SHELLY_DEBUG=1`.

- [ ] **`generate --upgrade`** — re-scaffold any addon lib files that have changed (currently `addAddon` errors if the file already exists; upgrade should overwrite with the latest built-in content).

---

## Intentionally Excluded

These are bashly features that shelly deliberately does not implement:

- **Shell completions** — requires bash 4+; shelly targets POSIX sh.
- **`$args` associative array** — bash 4.2+ only; incompatible with POSIX sh target.
- **ERB preprocessing in config** — bashly is Ruby-based; shelly has no config templating.
- **Bash3 bouncer / sourcing guard** — bash-specific syntax (`[[ ]]`).
- **`bashly render`** (markdown/man page generation) — out of scope.
- **`generate --watch`** — filesystem watcher not yet implemented (no watcher dep in go.mod).
