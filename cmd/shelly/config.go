package main

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ExtensibleField accepts either a bool (true = PATH-based extensible) or
// a string (delegate name, e.g. "git"). Maps to bashly's single `extensible` key.
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
// Maps to bashly's single `default` key on commands.
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
	Dependencies         []Dependency          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
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
	Argfile    bool         `json:"argfile,omitempty" yaml:"argfile,omitempty"`
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

type ShellyCfg struct {
	Name                 string                `yaml:"name"`
	Help                 string                `yaml:"help,omitempty"`
	Version              string                `yaml:"version,omitempty"`
	Footer               string                `yaml:"footer,omitempty"`
	Dependencies         []Dependency          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	EnvironmentVariables []EnvironmentVariable `json:"environment_variables,omitempty" yaml:"environment_variables,omitempty"`
	Commands             []Command             `json:"commands,omitempty" yaml:"commands,omitempty"`
	Args                 []Arg                 `json:"args,omitempty" yaml:"args,omitempty"`
	Flags                []Flag                `json:"flags,omitempty" yaml:"flags,omitempty"`
	Examples             []string              `json:"examples,omitempty" yaml:"examples,omitempty"`
	Variables            []Variable            `json:"variables,omitempty" yaml:"variables,omitempty"`
	// Extensible unified field: accepts true or a delegate name (e.g. "git").
	Extensible       ExtensibleField `json:"extensible,omitempty" yaml:"extensible,omitempty"`
	PrivateRevealKey string          `json:"private_reveal_key,omitempty" yaml:"private_reveal_key,omitempty"`
	Strict           bool            `json:"strict,omitempty" yaml:"strict,omitempty"`
	// WordWrap: wrap help text at this column width in generated usage functions.
	// 0 (default) disables wrapping.
	WordWrap  int    `json:"word_wrap,omitempty" yaml:"word_wrap,omitempty"`
	Formatter string `json:"formatter,omitempty" yaml:"formatter,omitempty"`
	// HelpHeaderOverride replaces the default "appname - help" line in the root usage function.
	HelpHeaderOverride string `json:"help_header_override,omitempty" yaml:"help_header_override,omitempty"`
	// DisableViewMarkers suppresses "# :command.*" section markers in the generated script.
	// Default false = markers emitted (current behavior).
	DisableViewMarkers bool `json:"disable_view_markers,omitempty" yaml:"disable_view_markers,omitempty"`
}

// privateRevealEnvVar returns the env var used to reveal private commands.
// Defaults to "SHELLY_PRIVATE_REVEAL" if PrivateRevealKey is not set.
func (cfg *ShellyCfg) privateRevealEnvVar() string {
	if cfg.PrivateRevealKey != "" {
		return cfg.PrivateRevealKey
	}
	return "SHELLY_PRIVATE_REVEAL"
}

// parseYAML unmarshals YAML bytes into a ShellyCfg.
func parseYAML(data []byte, cfg *ShellyCfg) error {
	return yaml.Unmarshal(data, cfg)
}

func readConfig() *ShellyCfg {
	var shellyConfig *ShellyCfg
	yamlFilename := "src/shelly.yml"
	yamlFile, err := os.ReadFile(yamlFilename)
	if err != nil {
		return shellyConfig
	}

	err = yaml.Unmarshal(yamlFile, &shellyConfig)
	if err != nil {
		return shellyConfig
	}
	return shellyConfig
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
