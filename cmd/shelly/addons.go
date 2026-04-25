package main

import (
	"fmt"
	"os"
)

// addAddon writes addon files into the src/ tree.
// Known addons: "validations", "colors", "hooks".
// When upgrade is true, existing files are overwritten.
func addAddon(name string, upgrade bool) error {
	switch name {
	case "validations":
		return writeAddonFile("src/lib/validations.sh", validationsContent, upgrade)
	case "colors":
		return writeAddonFile("src/lib/colors.sh", colorsContent, upgrade)
	case "hooks":
		if err := writeAddonFile("src/before.sh", beforeHookContent, upgrade); err != nil {
			return err
		}
		return writeAddonFile("src/after.sh", afterHookContent, upgrade)
	case "preamble":
		return writeAddonFile("src/preamble.sh", preambleContent, upgrade)
	default:
		return fmt.Errorf("unknown addon %q (available: validations, colors, hooks, preamble)", name)
	}
}

func writeAddonFile(path, content string, upgrade bool) error {
	if !upgrade {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("addon file already exists: %s (use --upgrade to overwrite)", path)
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
