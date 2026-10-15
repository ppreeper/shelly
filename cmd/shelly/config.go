package main

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExtensibleField accepts either a bool (true = PATH-based extensible) or
// a string (delegate name, e.g. "git").
type ExtensibleField struct {
	boolVal     bool
	delegateVal string
}

func (e ExtensibleField) IsEnabled() bool  { return e.boolVal || e.delegateVal != "" }
func (e ExtensibleField) IsBool() bool     { return e.boolVal }
func (e ExtensibleField) Delegate() string { return e.delegateVal }

func (e *ExtensibleField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!bool":
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		e.boolVal = b
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		if s == "true" {
			e.boolVal = true
		} else {
			e.delegateVal = s
		}
	}
	return nil
}

// ArgfileField accepts either a bool (true = read the legacy default path
// ~/.config/<app>/<command>) or a string path (resolved relative to CWD).
type ArgfileField struct {
	boolVal bool
	pathVal string
}

func (a ArgfileField) Enabled() bool { return a.boolVal || a.pathVal != "" }
func (a ArgfileField) Path() string  { return a.pathVal }

func (a *ArgfileField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!bool":
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		a.boolVal = b
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		switch s {
		case "true":
			a.boolVal = true
		case "false":
			a.boolVal = false
		default:
			a.pathVal = s
		}
	}
	return nil
}

// ExposeField accepts bool (true = show subcommands in root help) or the string
// "always" (show in root help AND inline when parent command has no args).
type ExposeField struct {
	enabled  bool
	isAlways bool
}

func (e ExposeField) IsEnabled() bool { return e.enabled || e.isAlways }
func (e ExposeField) IsAlways() bool  { return e.isAlways }
func NewExposeField(always bool) ExposeField {
	if always {
		return ExposeField{isAlways: true}
	}
	return ExposeField{enabled: true}
}

func (e *ExposeField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!bool":
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		e.enabled = b
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		switch s {
		case "always":
			e.isAlways = true
		case "true":
			e.enabled = true
		}
	}
	return nil
}

// DefaultField accepts either a bool (true = default) or a string ("force").
type DefaultField struct {
	isDefault bool
	isForce   bool
}

func (d DefaultField) IsDefault() bool { return d.isDefault || d.isForce }
func (d DefaultField) IsForce() bool   { return d.isForce }

func (d *DefaultField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!bool":
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		d.isDefault = b
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		if s == "force" {
			d.isForce = true
		} else if s == "true" {
			d.isDefault = true
		}
	}
	return nil
}

// StrictField accepts a bool or a string:
//
//	false    -> set -e
//	true     -> set -euo pipefail plus IFS=$'\n\t'
//	""       -> no directive
//	<string> -> that exact directive, verbatim
type StrictField struct {
	boolVal  bool
	strVal   string
	isString bool
}

// Directive returns the shell text to emit (without indentation), or "" when no
// directive should be emitted.
func (s StrictField) Directive() string {
	if s.isString {
		return s.strVal
	}
	if s.boolVal {
		return "set -euo pipefail\nIFS=$'\\n\\t'"
	}
	return "set -e"
}

func (s *StrictField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!bool":
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		s.boolVal = b
	case "!!null":
		// strict: (no value) -> no directive
	default:
		var str string
		if err := value.Decode(&str); err != nil {
			return err
		}
		s.isString = true
		s.strVal = str
	}
	return nil
}

// DefaultWordWrap is the wrap width applied when word_wrap is unset.
const DefaultWordWrap = 80

// WordWrapField configures help-text wrapping in generated usage functions.
// It accepts an integer column width, a bool, or nothing. When unset the default
// is DefaultWordWrap (80); an explicit integer of 0 or `false` disables wrapping.
type WordWrapField struct {
	width    int
	explicit bool
}

// Width returns the effective wrap column, or 0 when wrapping is disabled.
func (w WordWrapField) Width() int {
	if !w.explicit {
		return DefaultWordWrap
	}
	return w.width
}

func (w *WordWrapField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!null":
		// unset -> default
		return nil
	case "!!bool":
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		w.explicit = true
		if b {
			w.width = DefaultWordWrap
		} else {
			w.width = 0
		}
		return nil
	case "!!int":
		var n int
		if err := value.Decode(&n); err != nil {
			return err
		}
		w.explicit = true
		w.width = n
		return nil
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		w.explicit = true
		switch strings.ToLower(s) {
		case "", "false", "none", "off":
			w.width = 0
		default:
			n, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("word_wrap: invalid value %q", s)
			}
			w.width = n
		}
		return nil
	}
}

type Dependency struct {
	Name     string   `json:"name,omitempty" yaml:"name,omitempty"`
	Commands []string `json:"command,omitempty" yaml:"command,omitempty"`
	Version  string   `json:"version,omitempty" yaml:"version,omitempty"`
	Help     string   `json:"help,omitempty" yaml:"help,omitempty"`
}

// depNames returns the canonical list of command names to check.
// If Commands is set (OR-syntax), use that; else fall back to Name.
func (d Dependency) depNames() []string {
	if len(d.Commands) > 0 {
		return d.Commands
	}
	if d.Name != "" {
		return []string{d.Name}
	}
	return nil
}

// DependencyList accepts several dependency shorthand syntaxes and normalizes
// them into []Dependency:
//
//	dependencies: [curl, git]                 # sequence of names
//	dependencies: { curl: "HTTP client" }     # mapping name -> help
//	dependencies:                             # sequence of object form
//	  - name: curl
//	    version: "7"
//	dependencies:                             # mapping name -> object
//	  http_client:
//	    command: [curl, wget]
//	    help: HTTP client
type DependencyList []Dependency

func (d *DependencyList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*d = DependencyList{{Name: value.Value}}
		return nil
	case yaml.SequenceNode:
		out := make(DependencyList, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind == yaml.ScalarNode {
				out = append(out, Dependency{Name: item.Value})
				continue
			}
			var dep Dependency
			if err := item.Decode(&dep); err != nil {
				return err
			}
			out = append(out, dep)
		}
		*d = out
		return nil
	case yaml.MappingNode:
		out := make(DependencyList, 0, len(value.Content)/2)
		for i := 0; i+1 < len(value.Content); i += 2 {
			key := value.Content[i].Value
			val := value.Content[i+1]
			if val.Kind == yaml.ScalarNode {
				out = append(out, Dependency{Name: key, Help: val.Value})
				continue
			}
			var dep Dependency
			if err := val.Decode(&dep); err != nil {
				return err
			}
			if dep.Name == "" {
				dep.Name = key
			}
			out = append(out, dep)
		}
		*d = out
		return nil
	}
	return fmt.Errorf("dependencies: unsupported YAML node kind %d", value.Kind)
}

// PathList accepts either a comma-separated scalar or a YAML sequence of paths
// (`~` | array | comma string). A value of
// "~" or "" means "no extra dirs".
type PathList []string

func (p *PathList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		v := value.Value
		if v == "" || v == "~" {
			*p = nil
			return nil
		}
		var out PathList
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" && s != "~" {
				out = append(out, s)
			}
		}
		*p = out
		return nil
	case yaml.SequenceNode:
		out := make(PathList, 0, len(value.Content))
		for _, item := range value.Content {
			var s string
			if err := item.Decode(&s); err != nil {
				return err
			}
			s = strings.TrimSpace(s)
			if s != "" && s != "~" {
				out = append(out, s)
			}
		}
		*p = out
		return nil
	}
	return fmt.Errorf("extra_lib_dirs: unsupported YAML node kind %d", value.Kind)
}

type EnvironmentVariable struct {
	Name         string   `json:"name" yaml:"name"`
	Help         string   `json:"help,omitempty" yaml:"help,omitempty"`
	Required     bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Default      string   `json:"default,omitempty" yaml:"default,omitempty"`
	Allowed      []string `json:"allowed,omitempty" yaml:"allowed,omitempty"`
	Private      bool     `json:"private,omitempty" yaml:"private,omitempty"`
	Validate     string   `json:"validate,omitempty" yaml:"validate,omitempty"`
	ValidateList []string `json:"validate_list,omitempty" yaml:"validate_list,omitempty"`
}

type Flag struct {
	Long         string   `json:"long" yaml:"long"`
	Short        string   `json:"short,omitempty" yaml:"short,omitempty"`
	Arg          string   `json:"arg,omitempty" yaml:"arg,omitempty"`
	Required     bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Default      string   `json:"default,omitempty" yaml:"default,omitempty"`
	DefaultList  []string `json:"default_list,omitempty" yaml:"default_list,omitempty"`
	Allowed      []string `json:"allowed,omitempty" yaml:"allowed,omitempty"`
	Help         string   `json:"help,omitempty" yaml:"help,omitempty"`
	Repeatable   bool     `json:"repeatable,omitempty" yaml:"repeatable,omitempty"`
	Unique       bool     `json:"unique,omitempty" yaml:"unique,omitempty"`
	Conflicts    []string `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
	Needs        []string `json:"needs,omitempty" yaml:"needs,omitempty"`
	Private      bool     `json:"private,omitempty" yaml:"private,omitempty"`
	Validate     string   `json:"validate,omitempty" yaml:"validate,omitempty"`
	ValidateList []string `json:"validate_list,omitempty" yaml:"validate_list,omitempty"`
	// Alias accepts a single alternative spelling; use Aliases for multiple.
	Alias   string   `json:"alias,omitempty" yaml:"alias,omitempty"`
	Aliases []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	// Negatable boolean long flags also accept --no-<long>, which unsets the value.
	Negatable bool `json:"negatable,omitempty" yaml:"negatable,omitempty"`
}

// allAliases returns both Alias (legacy) and Aliases merged, deduplicated.
func (f *Flag) allAliases() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range append([]string{f.Alias}, f.Aliases...) {
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

type Arg struct {
	Name         string   `json:"name" yaml:"name"`
	Required     bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Default      string   `json:"default,omitempty" yaml:"default,omitempty"`
	DefaultList  []string `json:"default_list,omitempty" yaml:"default_list,omitempty"`
	Allowed      []string `json:"allowed,omitempty" yaml:"allowed,omitempty"`
	Help         string   `json:"help,omitempty" yaml:"help,omitempty"`
	Repeatable   bool     `json:"repeatable,omitempty" yaml:"repeatable,omitempty"`
	Unique       bool     `json:"unique,omitempty" yaml:"unique,omitempty"`
	Validate     string   `json:"validate,omitempty" yaml:"validate,omitempty"`
	ValidateList []string `json:"validate_list,omitempty" yaml:"validate_list,omitempty"`
}

// CatchAllConfig holds configuration for catch_all remaining arguments.
type CatchAllConfig struct {
	Label     string `json:"label,omitempty" yaml:"label,omitempty"`
	Help      string `json:"help,omitempty" yaml:"help,omitempty"`
	Required  bool   `json:"required,omitempty" yaml:"required,omitempty"`
	CatchHelp bool   `json:"catch_help,omitempty" yaml:"catch_help,omitempty"`
}

// VariableValue is a string that rejects YAML arrays and maps at unmarshal time,
// giving a clear error instead of silently producing broken shell output.
type VariableValue struct {
	s string
}

func (v VariableValue) String() string        { return v.s }
func NewVariableValue(s string) VariableValue { return VariableValue{s: s} }

func (v *VariableValue) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!seq":
		return fmt.Errorf("variable value must be a string; got a YAML array (POSIX sh has no arrays)")
	case "!!map":
		return fmt.Errorf("variable value must be a string; got a YAML map (POSIX sh has no associative arrays)")
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		v.s = s
	}
	return nil
}

// Variable holds a globally scoped named shell variable.
type Variable struct {
	Name  string        `json:"name" yaml:"name"`
	Value VariableValue `json:"value,omitempty" yaml:"value,omitempty"`
}

// FunctionNames allows renaming the internal generated functions run() and initialize().
type FunctionNames struct {
	Run        string `json:"run,omitempty" yaml:"run,omitempty"`
	Initialize string `json:"initialize,omitempty" yaml:"initialize,omitempty"`
}

// VarAliases allows renaming generated shell variables.
// OtherArgs overrides the default "other_args" variable name used by catch_all.
type VarAliases struct {
	OtherArgs string `json:"other_args,omitempty" yaml:"other_args,omitempty"`
}

// UsageColors configures ANSI color codes emitted around usage sections.
// Each field holds a color-function name (e.g. "green_underlined",
// "red_bold"), a named style ("red", "green", "yellow", "blue", "magenta",
// "cyan", "black", "white", "bold", "dim", "underlined", "bold_underlined"),
// or a raw ANSI escape sequence. Empty string disables color for that section.
// Colors are POSIX-safe (printf).
type UsageColors struct {
	Caption             string `json:"caption,omitempty" yaml:"caption,omitempty"`
	Command             string `json:"command,omitempty" yaml:"command,omitempty"`
	Arg                 string `json:"arg,omitempty" yaml:"arg,omitempty"`
	Flag                string `json:"flag,omitempty" yaml:"flag,omitempty"`
	EnvironmentVariable string `json:"environment_variable,omitempty" yaml:"environment_variable,omitempty"`
}

type Command struct {
	Name string `json:"name" yaml:"name"`
	// Alias accepts a single string; use Aliases for multiple.
	Alias                string                `json:"alias,omitempty" yaml:"alias,omitempty"`
	Aliases              []string              `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Help                 string                `json:"help,omitempty" yaml:"help,omitempty"`
	Args                 []Arg                 `json:"args,omitempty" yaml:"args,omitempty"`
	Flags                []Flag                `json:"flags,omitempty" yaml:"flags,omitempty"`
	Commands             []Command             `json:"commands,omitempty" yaml:"commands,omitempty"`
	Examples             []string              `json:"examples,omitempty" yaml:"examples,omitempty"`
	EnvironmentVariables []EnvironmentVariable `json:"environment_variables,omitempty" yaml:"environment_variables,omitempty"`
	Dependencies         DependencyList        `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	CatchAll             *CatchAllConfig       `json:"catch_all,omitempty" yaml:"catch_all,omitempty"`
	// DefaultCmd unified field: accepts true or "force" via YAML.
	DefaultCmd DefaultField `json:"default,omitempty" yaml:"default,omitempty"`
	Private    bool         `json:"private,omitempty" yaml:"private,omitempty"`
	Group      string       `json:"group,omitempty" yaml:"group,omitempty"`
	Footer     string       `json:"footer,omitempty" yaml:"footer,omitempty"`
	Filters    []string     `json:"filters,omitempty" yaml:"filters,omitempty"`
	Expose     ExposeField  `json:"expose,omitempty" yaml:"expose,omitempty"`
	Function   string       `json:"function,omitempty" yaml:"function,omitempty"`
	Filename   string       `json:"filename,omitempty" yaml:"filename,omitempty"`
	Variables  []Variable   `json:"variables,omitempty" yaml:"variables,omitempty"`
	Argfile    ArgfileField `json:"argfile,omitempty" yaml:"argfile,omitempty"`
	// HelpHeaderOverride replaces the default "appname cmdname - help" line in the usage function.
	HelpHeaderOverride string `json:"help_header_override,omitempty" yaml:"help_header_override,omitempty"`
	// ShowExamplesOnError: when true, the command's examples are emitted in the
	// required-arg/flag error path before exit 1.
	ShowExamplesOnError bool `json:"show_examples_on_error,omitempty" yaml:"show_examples_on_error,omitempty"`
}

// allAliases returns both Alias (legacy) and Aliases merged, deduplicated.
func (c *Command) allAliases() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range append([]string{c.Alias}, c.Aliases...) {
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// exactAliases returns aliases that are matched literally (no trailing '*').
func (c *Command) exactAliases() []string {
	var out []string
	for _, a := range c.allAliases() {
		if !strings.HasSuffix(a, "*") {
			out = append(out, a)
		}
	}
	return out
}

// prefixAliases returns the prefixes of aliases ending in '*', in declaration order.
// A trailing '*' means the alias matches any command word beginning with the prefix.
func (c *Command) prefixAliases() []string {
	var out []string
	for _, a := range c.allAliases() {
		if p, ok := strings.CutSuffix(a, "*"); ok {
			out = append(out, p)
		}
	}
	return out
}

type ShellyCfg struct {
	Name                 string                `yaml:"name"`
	Help                 string                `yaml:"help,omitempty"`
	Version              string                `yaml:"version,omitempty"`
	Footer               string                `yaml:"footer,omitempty"`
	Dependencies         DependencyList        `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	EnvironmentVariables []EnvironmentVariable `json:"environment_variables,omitempty" yaml:"environment_variables,omitempty"`
	Commands             []Command             `json:"commands,omitempty" yaml:"commands,omitempty"`
	Args                 []Arg                 `json:"args,omitempty" yaml:"args,omitempty"`
	Flags                []Flag                `json:"flags,omitempty" yaml:"flags,omitempty"`
	Examples             []string              `json:"examples,omitempty" yaml:"examples,omitempty"`
	Variables            []Variable            `json:"variables,omitempty" yaml:"variables,omitempty"`
	// Extensible unified field: accepts true or a delegate name (e.g. "git").
	Extensible       ExtensibleField `json:"extensible,omitempty" yaml:"extensible,omitempty"`
	PrivateRevealKey string          `json:"private_reveal_key,omitempty" yaml:"private_reveal_key,omitempty"`
	Strict           StrictField     `json:"strict,omitempty" yaml:"strict,omitempty"`
	// WordWrap: wrap help text at this column width in generated usage functions.
	// Unset defaults to 80; 0 or false disables wrapping.
	WordWrap  WordWrapField `json:"word_wrap,omitempty" yaml:"word_wrap,omitempty"`
	Formatter string        `json:"formatter,omitempty" yaml:"formatter,omitempty"`
	// HelpHeaderOverride replaces the default "appname - help" line in the root usage function.
	HelpHeaderOverride string `json:"help_header_override,omitempty" yaml:"help_header_override,omitempty"`
	// DisableViewMarkers suppresses "# :command.*" section markers in the generated script.
	// Default false = markers emitted (current behavior).
	DisableViewMarkers bool `json:"disable_view_markers,omitempty" yaml:"disable_view_markers,omitempty"`
	// DisableHeaderComment suppresses the "generated by shelly / do not modify" comment in the shebang.
	DisableHeaderComment bool `json:"disable_header_comment,omitempty" yaml:"disable_header_comment,omitempty"`
	// TabIndent switches generated indentation from 2 spaces to a hard tab.
	TabIndent bool `json:"tab_indent,omitempty" yaml:"tab_indent,omitempty"`
	// Env controls generation mode: "" or "development" = full output; "production" = slim output.
	Env string `json:"env,omitempty" yaml:"env,omitempty"`
	// FunctionNames allows renaming run() and initialize() in the generated script.
	FunctionNames FunctionNames `json:"function_names,omitempty" yaml:"function_names,omitempty"`
	// VarAliases allows renaming generated shell variables (e.g. other_args).
	VarAliases VarAliases `json:"var_aliases,omitempty" yaml:"var_aliases,omitempty"`
	// UsageColors configures ANSI color codes for usage output sections.
	UsageColors UsageColors `json:"usage_colors,omitempty" yaml:"usage_colors,omitempty"`
	// CompactShortFlags (default true) expands a combined short-flag group
	// (-abc) into individual flags (-a -b -c) during parsing.
	CompactShortFlags *bool `json:"compact_short_flags,omitempty" yaml:"compact_short_flags,omitempty"`
	// ConjoinedFlagArgs (default true) allows a short flag and its value to be
	// joined with '=' (e.g. -o=file).
	ConjoinedFlagArgs *bool `json:"conjoined_flag_args,omitempty" yaml:"conjoined_flag_args,omitempty"`
	// Path settings. Defaults preserve the src/ + ./ layout.
	// SourceDir is where src files (shelly.yml, *_command.sh, lib) live (default "src").
	SourceDir string `json:"source_dir,omitempty" yaml:"source_dir,omitempty"`
	// TargetDir is the directory the generated script is written to (default ".").
	TargetDir string `json:"target_dir,omitempty" yaml:"target_dir,omitempty"`
	// LibDir is the library directory, relative to SourceDir (default "lib").
	LibDir string `json:"lib_dir,omitempty" yaml:"lib_dir,omitempty"`
	// ExtraLibDirs are additional library directories searched after LibDir.
	// Relative to CWD or absolute; "~" expands to the home directory.
	ExtraLibDirs PathList `json:"extra_lib_dirs,omitempty" yaml:"extra_lib_dirs,omitempty"`
	// CommandsDir, when set (not ""/"~"), places each command body in a
	// per-command subdirectory under SourceDir instead of a flat layout.
	CommandsDir string `json:"commands_dir,omitempty" yaml:"commands_dir,omitempty"`
	// messages holds overrides loaded from src/shelly-strings.yml; see strings.go.
	messages map[string]string
}

// privateRevealEnvVar returns the env var used to reveal private commands.
// Defaults to "SHELLY_PRIVATE_REVEAL" if PrivateRevealKey is not set.
func (cfg *ShellyCfg) privateRevealEnvVar() string {
	if cfg.PrivateRevealKey != "" {
		return cfg.PrivateRevealKey
	}
	return "SHELLY_PRIVATE_REVEAL"
}

// compactShortFlags returns whether combined short flags (-abc) are expanded
// into individual flags (-a -b -c). Defaults to true when unset.
func (cfg *ShellyCfg) compactShortFlags() bool {
	return cfg.CompactShortFlags == nil || *cfg.CompactShortFlags
}

// conjoinedFlagArgs returns whether a short flag and its value may be joined
// with '=' (e.g. -o=file). Defaults to true when unset.
func (cfg *ShellyCfg) conjoinedFlagArgs() bool {
	return cfg.ConjoinedFlagArgs == nil || *cfg.ConjoinedFlagArgs
}

// wordWrap returns the effective help-text wrap column, or 0 when wrapping is
// disabled. Unset defaults to DefaultWordWrap (80).
func (cfg *ShellyCfg) wordWrap() int {
	return cfg.WordWrap.Width()
}

// sourceDir returns the configured source directory (default "src").
func (cfg *ShellyCfg) sourceDir() string {
	if cfg.SourceDir != "" && cfg.SourceDir != "~" {
		return cfg.SourceDir
	}
	return "src"
}

// targetDir returns the configured output directory (default ".").
func (cfg *ShellyCfg) targetDir() string {
	if cfg.TargetDir != "" && cfg.TargetDir != "~" {
		return cfg.TargetDir
	}
	return "."
}

// libDir returns the resolved library directory (LibDir relative to SourceDir).
func (cfg *ShellyCfg) libDir() string {
	d := cfg.LibDir
	if d == "" || d == "~" {
		d = "lib"
	}
	if filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(cfg.sourceDir(), d)
}

// expandHome expands a leading "~" or "~/" to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// extraLibDirs returns the resolved extra library directories (relative to CWD
// or absolute; "~" expands to the home directory).
func (cfg *ShellyCfg) extraLibDirs() []string {
	out := make([]string, 0, len(cfg.ExtraLibDirs))
	for _, d := range cfg.ExtraLibDirs {
		if d == "" || d == "~" {
			continue
		}
		out = append(out, expandHome(d))
	}
	return out
}

// libDirs returns LibDir followed by any ExtraLibDirs, in search order.
func (cfg *ShellyCfg) libDirs() []string {
	dirs := make([]string, 0, 1+len(cfg.ExtraLibDirs))
	dirs = append(dirs, cfg.libDir())
	dirs = append(dirs, cfg.extraLibDirs()...)
	return dirs
}

// commandsDir returns the per-command subdirectory (relative to SourceDir), or
// "" for the flat layout. "~" means flat.
func (cfg *ShellyCfg) commandsDir() string {
	if cfg.CommandsDir == "" || cfg.CommandsDir == "~" {
		return ""
	}
	return cfg.CommandsDir
}

// commandPath returns the source path for a command's body file.
// Flat: <source_dir>/<funcBase>_command.sh.
// commands_dir: <source_dir>/<commands_dir>/<funcBase>/<funcBase>_command.sh.
func (cfg *ShellyCfg) commandPath(funcBase string) string {
	if cd := cfg.commandsDir(); cd != "" {
		return filepath.Join(cfg.sourceDir(), cd, funcBase, funcBase+"_command.sh")
	}
	return filepath.Join(cfg.sourceDir(), funcBase+"_command.sh")
}

// outputPath returns the target path of the generated script.
func (cfg *ShellyCfg) outputPath() string {
	return filepath.Join(cfg.targetDir(), cfg.Name)
}

// runFuncName returns the name of the generated run() function.
func (cfg *ShellyCfg) runFuncName() string {
	if cfg.FunctionNames.Run != "" {
		return cfg.FunctionNames.Run
	}
	return "run"
}

// initFuncName returns the name of the generated initialize() function.
func (cfg *ShellyCfg) initFuncName() string {
	if cfg.FunctionNames.Initialize != "" {
		return cfg.FunctionNames.Initialize
	}
	return "initialize"
}

// otherArgsVar returns the shell variable name used to accumulate catch_all args.
func (cfg *ShellyCfg) otherArgsVar() string {
	if cfg.VarAliases.OtherArgs != "" {
		return cfg.VarAliases.OtherArgs
	}
	return "other_args"
}

// isProduction reports whether the generation mode is "production".
func (cfg *ShellyCfg) isProduction() bool {
	return strings.EqualFold(cfg.Env, "production")
}

// indent returns the indentation string: a hard tab when TabIndent is set, else two spaces.
func (cfg *ShellyCfg) indent() string {
	if cfg.TabIndent {
		return "\t"
	}
	return "  "
}

// parseYAML unmarshals YAML bytes into a ShellyCfg.
func parseYAML(data []byte, cfg *ShellyCfg) error {
	return yaml.Unmarshal(data, cfg)
}

func readConfig() *ShellyCfg {
	cfg, err := readConfigE()
	if err != nil {
		return nil
	}
	return cfg
}

// readConfigE discovers and reads the configuration file, resolving any
// `import:` directives. It returns (nil, nil) when no configuration file exists.
func readConfigE() (*ShellyCfg, error) {
	yamlFilename := ""
	for _, candidate := range []string{"src/shelly.yml", "shelly.yml"} {
		if _, err := os.Stat(candidate); err == nil {
			yamlFilename = candidate
			break
		}
	}
	if yamlFilename == "" {
		return nil, nil
	}
	return loadConfigFile(yamlFilename)
}

// loadConfigFile reads and parses a configuration file, resolving `import:`
// directives (either plain YAML files or the front matter above a `---`
// separator in a `.sh` file).
func loadConfigFile(path string) (*ShellyCfg, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("%s: empty configuration", path)
	}
	root, err := resolveConfigImports(doc.Content[0], filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	var cfg ShellyCfg
	if err := root.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Default the source dir to the directory containing the config so that a
	// root-level shelly.yml resolves src files relative to the CWD.
	if cfg.SourceDir == "" {
		cfg.SourceDir = filepath.Dir(path)
	}
	return &cfg, nil
}

// resolveConfigImports walks a YAML node tree and resolves `import:` directives:
// an imported mapping is merged
// into the containing mapping (imported keys win), while an imported sequence
// replaces the containing node. Imports may appear at any level.
func resolveConfigImports(node *yaml.Node, baseDir string) (*yaml.Node, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return node, nil
		}
		resolved, err := resolveConfigImports(node.Content[0], baseDir)
		if err != nil {
			return nil, err
		}
		node.Content[0] = resolved
		return node, nil
	case yaml.MappingNode:
		result := &yaml.Node{Kind: yaml.MappingNode, Tag: node.Tag}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			if key.Value == "import" {
				if val.Kind != yaml.ScalarNode || val.Value == "" {
					return nil, fmt.Errorf("import: expected a file path")
				}
				imported, err := loadImportNode(val.Value, baseDir)
				if err != nil {
					return nil, err
				}
				nextBase := filepath.Dir(importFilePath(val.Value, baseDir))
				resolved, err := resolveConfigImports(imported, nextBase)
				if err != nil {
					return nil, err
				}
				switch resolved.Kind {
				case yaml.SequenceNode:
					// An imported sequence replaces the whole node.
					result = resolved
				case yaml.MappingNode:
					for j := 0; j+1 < len(resolved.Content); j += 2 {
						setMappingKey(result, resolved.Content[j], resolved.Content[j+1])
					}
				default:
					return nil, fmt.Errorf("import %q: expected a mapping or sequence", val.Value)
				}
				continue
			}
			rv, err := resolveConfigImports(val, baseDir)
			if err != nil {
				return nil, err
			}
			setMappingKey(result, key, rv)
		}
		return result, nil
	case yaml.SequenceNode:
		out := &yaml.Node{Kind: yaml.SequenceNode, Tag: node.Tag}
		out.Content = make([]*yaml.Node, 0, len(node.Content))
		for _, item := range node.Content {
			ri, err := resolveConfigImports(item, baseDir)
			if err != nil {
				return nil, err
			}
			out.Content = append(out.Content, ri)
		}
		return out, nil
	default:
		return node, nil
	}
}

// setMappingKey sets key to val in a mapping node, replacing an existing entry
// with the same key (so later/imported values win), else appending it.
func setMappingKey(m *yaml.Node, key, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key.Value {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, key, val)
}

// importFilePath resolves an import path: absolute paths are used as-is,
// otherwise it is tried relative to baseDir first and falls back to the path
// exactly as written (which the OS resolves relative to the CWD).
func importFilePath(path, baseDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if baseDir != "" {
		candidate := filepath.Join(baseDir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return path
}

// loadImportNode reads an imported configuration file and returns its root YAML
// node. A `.sh` file is read from the front matter above its first `---` line;
// any other file is parsed as a single YAML document.
func loadImportNode(path, baseDir string) (*yaml.Node, error) {
	full := importFilePath(path, baseDir)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("cannot find import file %q", path)
	}
	content := data
	if strings.EqualFold(filepath.Ext(path), ".sh") {
		fm, ok := frontMatter(data)
		if !ok {
			return nil, fmt.Errorf("cannot find a valid YAML front matter in %q", path)
		}
		content = fm
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse import file %q: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("cannot find a valid YAML in %q", path)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode && root.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("cannot find a valid YAML in %q", path)
	}
	return root, nil
}

// frontMatter returns the bytes above the first `---` separator line (the
// front-matter convention for `.sh` config files).
func frontMatter(data []byte) ([]byte, bool) {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			return []byte(strings.Join(lines[:i], "\n")), true
		}
	}
	return nil, false
}

func writeConfig(embedFS embed.FS, isMinimal bool) error {
	// Check if src directory exists; create if missing; ensure empty if present
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get current working directory: %w", err)
	}
	srcDir := filepath.Join(cwd, "src")

	// If src does not exist, create it and treat as empty
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			return fmt.Errorf("could not create src directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("could not stat src directory: %w", err)
	} else {
		empty, err := isDirectoryEmpty(srcDir)
		if err != nil {
			return fmt.Errorf("could not check if directory is empty: %w", err)
		}
		if !empty {
			return fmt.Errorf("directory src already exists and is not empty")
		}
	}

	// Create shelly.yml file from template
	configfile, err := os.Create(filepath.Join(srcDir, "shelly.yml"))
	if err != nil {
		return fmt.Errorf("create Caddyfile failed %w", err)
	}
	defer configfile.Close()
	templateName := "shelly.yml"
	if isMinimal {
		templateName = "minimal.yml"
	}
	configTemplate, err := template.ParseFS(embedFS, templateName)
	if err != nil {
		return fmt.Errorf("error parsing minimal shelly.yml template %w", err)
	}
	err = configTemplate.Execute(configfile, nil)
	if err != nil {
		return fmt.Errorf("error excuting minimal shelly.yml template %w", err)
	}
	fmt.Println("created src/shelly.yml")

	return nil
}
