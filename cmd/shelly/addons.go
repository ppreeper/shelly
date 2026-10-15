package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// addonFile is a single file installed by a built-in addon.
type addonFile struct {
	path    string
	content string
}

// upgradeMarkerPrefix is the token embedded in installed library files so
// that `generate --upgrade` can locate and refresh them.
const upgradeMarkerPrefix = "[@shelly-upgrade "

// builtinAddonNames lists the built-in addons accepted by `shelly add`.
func builtinAddonNames() []string {
	return []string{"validations", "colors", "hooks", "preamble", "strings", "yaml", "ini", "config"}
}

// builtinAddonFiles returns the files a built-in addon installs, relative to the
// configured source directory. The bool reports whether the addon is known.
func builtinAddonFiles(cfg *ShellyCfg, name string) ([]addonFile, bool) {
	src := cfg.sourceDir()
	lib := filepath.Join(src, "lib")
	switch name {
	case "validations":
		return []addonFile{{filepath.Join(lib, "validations.sh"), validationsContent}}, true
	case "colors":
		return []addonFile{{filepath.Join(lib, "colors.sh"), colorsContent}}, true
	case "hooks":
		return []addonFile{
			{filepath.Join(src, "before.sh"), beforeHookContent},
			{filepath.Join(src, "after.sh"), afterHookContent},
		}, true
	case "preamble":
		return []addonFile{{filepath.Join(src, "preamble.sh"), preambleContent}}, true
	case "strings":
		return []addonFile{{filepath.Join(src, "shelly-strings.yml"), stringsFileContent()}}, true
	case "yaml":
		return []addonFile{{filepath.Join(lib, "yaml.sh"), yamlContent}}, true
	case "ini":
		return []addonFile{{filepath.Join(lib, "ini.sh"), iniContent}}, true
	case "config":
		return []addonFile{
			{filepath.Join(lib, "config.sh"), configContent},
			{filepath.Join(lib, "ini.sh"), iniContent},
		}, true
	}
	return nil, false
}

// withUpgradeMarker prepends the upgrade marker line to installed file content
// (a `#` comment for YAML, `##` otherwise).
func withUpgradeMarker(path, content, name string) string {
	prefix := "##"
	if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
		prefix = "#"
	}
	return fmt.Sprintf("%s %s%s]\n%s", prefix, upgradeMarkerPrefix, name, content)
}

// addAddon writes the files of a built-in addon into the source tree.
// Known addons: "validations", "colors", "hooks", "preamble", "strings",
// "yaml", "ini", "config".
// When upgrade is true, existing files are overwritten.
func addAddon(name string, upgrade bool) error {
	cfg := effectiveConfig()
	files, ok := builtinAddonFiles(cfg, name)
	if !ok {
		return fmt.Errorf("unknown addon %q (available: %s)", name, strings.Join(builtinAddonNames(), ", "))
	}
	for _, f := range files {
		if err := writeAddonFile(f.path, withUpgradeMarker(f.path, f.content, name), upgrade); err != nil {
			return err
		}
	}
	return nil
}

// stringsFileContent renders the built-in message defaults as a commented
// src/shelly-strings.yml scaffold. Keys are sorted for a stable file.
func stringsFileContent() string {
	keys := make([]string, 0, len(defaultMessages))
	for k := range defaultMessages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# shelly message strings — override any of the defaults below.\n")
	b.WriteString("# Keys are message-string names. Delete a line to keep its default.\n")
	b.WriteString("# Templates may use %{name} generation-time placeholders and %s runtime placeholders.\n")
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, yamlSingleQuote(defaultMessages[k])))
	}
	return b.String()
}

// yamlSingleQuote quotes s as a YAML single-quoted scalar (doubling embedded
// single quotes), which is safe for every default message.
func yamlSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func writeAddonFile(path, content string, upgrade bool) error {
	if !upgrade {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("addon file already exists: %s (use --upgrade to overwrite)", path)
		}
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("created %s\n", path)
	return nil
}

const validationsContent = `# validate_not_empty NAME VALUE
# Exits with error if VALUE is empty.
validate_not_empty() {
  if [ -z "$2" ]; then
    echo "Error: $1 cannot be empty" >&2
    exit 1
  fi
}

# validate_integer NAME VALUE
# Exits with error if VALUE is not an integer.
validate_integer() {
  case "$2" in
    ''|*[!0-9-]*) echo "Error: $1 must be an integer" >&2; exit 1 ;;
    *) ;;
  esac
}

# validate_file_exists NAME VALUE
# Exits with error if VALUE is not a path to an existing file.
validate_file_exists() {
  if [ ! -f "$2" ]; then
    echo "Error: $1 must be an existing file, got: $2" >&2
    exit 1
  fi
}

# validate_dir_exists NAME VALUE
# Exits with error if VALUE is not a path to an existing directory.
validate_dir_exists() {
  if [ ! -d "$2" ]; then
    echo "Error: $1 must be an existing directory, got: $2" >&2
    exit 1
  fi
}
`

const colorsContent = `# ANSI color helpers.
# Usage: printf "%s\\n" "$(red 'error message')"

red()    { printf '\033[31m%s\033[0m' "$*"; }
green()  { printf '\033[32m%s\033[0m' "$*"; }
yellow() { printf '\033[33m%s\033[0m' "$*"; }
blue()   { printf '\033[34m%s\033[0m' "$*"; }
cyan()   { printf '\033[36m%s\033[0m' "$*"; }
bold()   { printf '\033[1m%s\033[0m'  "$*"; }
dim()    { printf '\033[2m%s\033[0m'  "$*"; }
`

const beforeHookContent = `# src/before.sh — runs inside run() after normalize_input, before command dispatch.
# Use %APP_NAME% to reference the script name.
# Example:
#   echo "Running %APP_NAME%..."
`

const afterHookContent = `# src/after.sh — runs inside run() after command dispatch completes.
# Use %APP_NAME% to reference the script name.
# Example:
#   echo "Done."
`

const preambleContent = `# src/preamble.sh — raw content placed at the top of the compiled script,
# after the shebang/comments and before any function definitions.
# Use %APP_NAME% and %VERSION% as substitution tokens.
# Example:
#   MY_CONFIG_DIR="${HOME}/.config/%APP_NAME%"
`

// iniContent is a POSIX implementation of an INI reader/writer library. Because
// POSIX sh has no associative arrays, it exposes keyed helpers
// (ini_get/ini_set/ini_unset/...) and keeps the in-memory store in the global
// variable INI_DATA.
const iniContent = `# INI functions (POSIX) [@shelly-upgrade ini]
# Part of the shelly standard library.
#
# POSIX sh has no associative arrays, so this library exposes a keyed-helper API
# (ini_get/ini_set/...) rather than an indexed map.
#
# Usage:
#   ini_load  path/to/config.ini     # read the file into the in-memory store
#   ini_get   section.key [default]  # print a value
#   ini_set   section.key value...   # create/update a value (call ini_save to persist)
#   ini_unset section.key            # delete a value (call ini_save to persist)
#   ini_keys                         # print all keys, one per line, sorted
#   ini_show                         # print "key = value" for every pair
#   ini_save  path/to/config.ini     # write the store back to disk
#
# Notes:
#   - Sectionless keys are supported; a key containing "." is treated as
#     "section.key".
#   - Values are single-line; inline comments and multi-line values are not
#     supported.
#   - The in-memory store lives in the global variable INI_DATA as a
#     newline-separated list of "key=value" pairs.

INI_DATA=""

# ini_load FILE - read FILE into INI_DATA.
ini_load() {
  _ini_file="$1"
  INI_DATA=""
  [ -f "$_ini_file" ] || return 0
  _ini_section=""
  while IFS= read -r _ini_line || [ -n "$_ini_line" ]; do
    _ini_line="${_ini_line%"${_ini_line##*[![:space:]]}"}"
    _ini_line="${_ini_line#"${_ini_line%%[![:space:]]*}"}"
    case "$_ini_line" in
      ''|';'*|'#'*) continue ;;
      '['*\])
        _ini_section="${_ini_line#\[}"
        _ini_section="${_ini_section%\]}"
        continue
        ;;
    esac
    case "$_ini_line" in
      *=*) ;;
      *) continue ;;
    esac
    _ini_k="${_ini_line%%=*}"
    _ini_v="${_ini_line#*=}"
    _ini_k="${_ini_k%"${_ini_k##*[![:space:]]}"}"
    _ini_k="${_ini_k#"${_ini_k%%[![:space:]]*}"}"
    case "$_ini_k" in
      \"*\") _ini_k="${_ini_k#\"}"; _ini_k="${_ini_k%\"}" ;;
      \'*\') _ini_k="${_ini_k#\'}"; _ini_k="${_ini_k%\'}" ;;
    esac
    _ini_v="${_ini_v%"${_ini_v##*[![:space:]]}"}"
    _ini_v="${_ini_v#"${_ini_v%%[![:space:]]*}"}"
    case "$_ini_v" in
      \"*\") _ini_v="${_ini_v#\"}"; _ini_v="${_ini_v%\"}" ;;
      \'*\') _ini_v="${_ini_v#\'}"; _ini_v="${_ini_v%\'}" ;;
    esac
    [ -n "$_ini_k" ] || continue
    if [ -n "$_ini_section" ]; then
      INI_DATA="${INI_DATA}${_ini_section}.${_ini_k}=${_ini_v}
"
    else
      INI_DATA="${INI_DATA}${_ini_k}=${_ini_v}
"
    fi
  done <"$_ini_file"
}

# ini_get KEY [DEFAULT] - print the value for KEY, or DEFAULT when unset.
ini_get() {
  _ini_gkey="$1"
  _ini_gdef="${2-}"
  printf '%s\n' "$INI_DATA" | {
    while IFS= read -r _ini_gline; do
      case "$_ini_gline" in
        "$_ini_gkey"=*)
          printf '%s\n' "${_ini_gline#"$_ini_gkey"=}"
          exit 0
          ;;
      esac
    done
    printf '%s\n' "$_ini_gdef"
  }
}

# ini_set KEY VALUE... - create or update KEY (does not write the file).
ini_set() {
  _ini_skey="$1"
  shift
  _ini_sval="$*"
  INI_DATA="$(
    printf '%s\n' "$INI_DATA" | {
      _ini_seen=0
      while IFS= read -r _ini_sline; do
        case "$_ini_sline" in
          '') ;;
          "$_ini_skey"=*) printf '%s=%s\n' "$_ini_skey" "$_ini_sval"; _ini_seen=1 ;;
          *) printf '%s\n' "$_ini_sline" ;;
        esac
      done
      [ "$_ini_seen" = 1 ] || printf '%s=%s\n' "$_ini_skey" "$_ini_sval"
    }
  )"
}

# ini_unset KEY - delete KEY (does not write the file).
ini_unset() {
  _ini_ukey="$1"
  INI_DATA="$(
    printf '%s\n' "$INI_DATA" | while IFS= read -r _ini_uline; do
      case "$_ini_uline" in
        '') ;;
        "$_ini_ukey"=*) ;;
        *) printf '%s\n' "$_ini_uline" ;;
      esac
    done
  )"
}

# ini_keys - print every key, one per line, sorted.
ini_keys() {
  printf '%s\n' "$INI_DATA" | sed -e '/^$/d' -e 's/=.*//' | sort
}

# ini_show - print "key = value" for every pair.
ini_show() {
  printf '%s\n' "$INI_DATA" | while IFS= read -r _ini_shline; do
    [ -n "$_ini_shline" ] || continue
    printf '%s = %s\n' "${_ini_shline%%=*}" "${_ini_shline#*=}"
  done
}

# ini_save FILE - write INI_DATA back to FILE, grouping sectioned keys.
ini_save() {
  _ini_file="$1"
  : >"$_ini_file"
  printf '%s\n' "$INI_DATA" | while IFS= read -r _ini_wline; do
    case "$_ini_wline" in
      ''|*.*=*) ;;
      *=*) printf '%s = %s\n' "${_ini_wline%%=*}" "${_ini_wline#*=}" ;;
    esac
  done >>"$_ini_file"
  printf '%s\n' "$INI_DATA" | {
    _ini_seens=" "
    while IFS= read -r _ini_wline; do
      case "$_ini_wline" in
        *.*=*) ;;
        *) continue ;;
      esac
      _ini_wkey="${_ini_wline%%=*}"
      _ini_wsec="${_ini_wkey%%.*}"
      case "$_ini_seens" in
        *" $_ini_wsec "*) ;;
        *)
          printf '\n[%s]\n' "$_ini_wsec"
          _ini_seens="$_ini_seens$_ini_wsec "
          ;;
      esac
      printf '%s = %s\n' "${_ini_wkey#*.}" "${_ini_wline#*=}"
    done
  } >>"$_ini_file"
}
`

// configContent is a POSIX implementation of a lazily-loaded config library. It
// builds on the ini library (add the "ini" addon as well, or use "config" which
// writes both).
const configContent = `# Config (INI) functions (POSIX) [@shelly-upgrade config]
# Part of the shelly standard library. Requires the ini library (ini.sh).
#
# Set CONFIG_FILE somewhere in your script (for example src/initialize.sh),
# then use the helpers below. INI sections are optional.
#
#   config_get login.email           # print a value
#   config_get login.user guest      # print a value with a default
#   config_set cloud.provider aws    # create/update (saved immediately)
#   config_del login.email           # delete (saved immediately)
#   config_has_key login.password    # exit 0 when the key exists
#   config_keys                      # print all keys
#   config_show                      # print "key = value" for every pair
#
# CONFIG_FILE defaults to "config.ini".

: "${CONFIG_FILE:=config.ini}"
config_loaded=false

# config_load - load CONFIG_FILE into the ini store, unless already loaded.
config_load() {
  [ "$config_loaded" = true ] && return 0
  [ -f "$CONFIG_FILE" ] || : >"$CONFIG_FILE"
  ini_load "$CONFIG_FILE"
  config_loaded=true
}

# config_save - write the ini store back to CONFIG_FILE.
config_save() {
  ini_save "$CONFIG_FILE"
}

# config_reload - force a reload from CONFIG_FILE.
config_reload() {
  config_loaded=false
  config_load
}

# config_get KEY [DEFAULT] - print a value.
config_get() {
  config_load
  ini_get "$1" "${2-}"
}

# config_set KEY VALUE... - create or update a value and save.
config_set() {
  _config_key="$1"
  shift
  config_load
  ini_set "$_config_key" "$*"
  config_save
}

# config_del KEY - delete a value and save.
config_del() {
  config_load
  ini_unset "$1"
  config_save
}

# config_keys - print all keys.
config_keys() {
  config_load
  ini_keys
}

# config_show - print every key/value pair.
config_show() {
  config_load
  ini_show
}

# config_has_key KEY - succeed when KEY exists with a non-empty value.
config_has_key() {
  [ -n "$(config_get "$1" "")" ]
}
`

// yamlContent is a POSIX YAML loader (sed + awk based).
const yamlContent = `# YAML parser (POSIX) [@shelly-upgrade yaml]
# Part of the shelly standard library. Prints shell variable assignments for
# simple (scalar / nested-hash) YAML documents.
#
# Usage:
#   yaml_load settings.yml             # print assignments
#   yaml_load settings.yml config_     # print assignments with a name prefix
#   eval "$(yaml_load settings.yml)"   # create variables in the current scope
#
# Only scalar and nested-hash values are supported (no arrays). Values are
# emitted as double-quoted shell assignments; do not eval untrusted YAML.

# yaml_load FILE [PREFIX] - print "PREFIXa_b=value" assignments for FILE.
yaml_load() {
  _yaml_prefix="$2"
  _yaml_s='[[:space:]]*'
  _yaml_w='[a-zA-Z0-9_]*'
  _yaml_fs=$(printf '@' | tr '@' '\034')
  sed -ne "s|^\($_yaml_s\):|\1|" \
    -e "s|^\($_yaml_s\)\($_yaml_w\)$_yaml_s:${_yaml_s}[\"']\(.*\)[\"']$_yaml_s\$|\1$_yaml_fs\2$_yaml_fs\3|p" \
    -e "s|^\($_yaml_s\)\($_yaml_w\)$_yaml_s:$_yaml_s\(.*\)$_yaml_s\$|\1$_yaml_fs\2$_yaml_fs\3|p" "$1" |
    awk -F"$_yaml_fs" '{
      indent = length($1)/2;
      vname[indent] = $2;
      for (i in vname) {if (i > indent) {delete vname[i]}}
      if (length($3) > 0) {
         vn=""; for (i=0; i<indent; i++) {vn=(vn)(vname[i])("_")}
         printf("%s%s%s=\"%s\"\n", "'"$_yaml_prefix"'",vn, $2, $3);
      }
    }'
}
`
