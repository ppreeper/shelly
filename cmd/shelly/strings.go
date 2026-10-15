package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultMessages holds the built-in message templates keyed by stable names.
// Templates may contain generation-time placeholders of the form
// %{name} and runtime placeholders %s (the latter are replaced by shell
// expressions such as $1 when the message is embedded in the generated script).
//
// Users may override any key by creating src/shelly-strings.yml;
// see `shelly add strings`.
var defaultMessages = map[string]string{
	// Usage captions.
	"usage":                         "Usage:",
	"options":                       "Options:",
	"global_options":                "Global Options:",
	"arguments":                     "Arguments:",
	"commands":                      "Commands:",
	"subcommands":                   "Subcommands:",
	"examples":                      "Examples:",
	"environment_variables":         "Environment Variables:",
	"group":                         "%{group} Commands:",
	"additional_arguments":          "Additional arguments:",
	"private_commands":              "Private Commands:",
	"private_options":               "Private Options:",
	"private_environment_variables": "Private Environment Variables:",

	// Usage helpers.
	"required":   "(required)",
	"repeatable": "(repeatable)",
	"default":    "(default: %{value})",
	"allowed":    "[%{values}]",

	// Fixed flags help text.
	"help_flag_text":    "Show this help",
	"version_flag_text": "Show version",

	// Error messages.
	"invalid_flag":                          "Unknown flag: %s",
	"invalid_command":                       "Unknown command: %s",
	"invalid_subcommand":                    "Unknown subcommand: %s",
	"too_many_arguments":                    "Error: too many arguments: %s",
	"missing_required_flag":                 "Error: %{name} is required",
	"missing_required_argument":             "Error: <%{name}> is required",
	"disallowed_flag":                       "Error: %{name} must be one of: %{allowed}",
	"disallowed_argument":                   "Error: <%{name}> must be one of: %{allowed}",
	"disallowed_environment_variable":       "Error: %{name} must be one of: %{allowed}",
	"missing_required_environment_variable": "Error: environment variable %{name} is required",
	"conflicting_flags":                     "Error: %{name} and %{other} cannot be used together",
	"flag_needs_another":                    "Error: %{name} requires %{need}",
	"missing_dependency":                    "Error: required dependency '%{name}' not found",
	"dependency_version":                    "Error: %{name} version %{version} or compatible required",
	"missing_dependency_one_of":             "Error: one of [%{names}] is required but none found",
}

// msgPlaceholderRe matches any leftover %{...} generation-time placeholder so
// unknown placeholders can be stripped safely.
var msgPlaceholderRe = regexp.MustCompile(`%\{[^}]*\}`)

var shellDQReplacer = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"`", "\\`",
	`$`, `\$`,
)

// shellDQEscape escapes s so it can be safely embedded inside a double-quoted
// shell string.
func shellDQEscape(s string) string {
	return shellDQReplacer.Replace(s)
}

// expandMessage renders a message template for embedding inside a double-quoted
// shell string. Generation-time placeholders %{key} are replaced from vars; each
// runtime %s is replaced, in order, by the corresponding shell expression in
// args. Literal text is escaped so it cannot break out of the double quotes.
func expandMessage(tmpl string, vars map[string]string, args ...string) string {
	parts := strings.Split(tmpl, "%s")
	var b strings.Builder
	for i, part := range parts {
		for k, v := range vars {
			part = strings.ReplaceAll(part, "%{"+k+"}", v)
		}
		part = msgPlaceholderRe.ReplaceAllString(part, "")
		b.WriteString(shellDQEscape(part))
		if i < len(args) {
			b.WriteString(args[i])
		}
	}
	return b.String()
}

// loadStrings reads message overrides from src/shelly-strings.yml. A missing
// file is not an error.
func (cfg *ShellyCfg) loadStrings() error {
	cfg.messages = nil
	for _, name := range []string{"shelly-strings.yml"} {
		path := filepath.Join(cfg.sourceDir(), name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		m := map[string]string{}
		if err := yaml.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		cfg.messages = m
		break
	}
	return nil
}

// message returns the override for key if present, else the built-in default,
// else "".
func (cfg *ShellyCfg) message(key string) string {
	if v, ok := cfg.messages[key]; ok && v != "" {
		return v
	}
	return defaultMessages[key]
}

// m renders the message for key, substituting generation-time placeholders from
// vars and runtime %s placeholders from args.
func (cfg *ShellyCfg) m(key string, vars map[string]string, args ...string) string {
	return expandMessage(cfg.message(key), vars, args...)
}
