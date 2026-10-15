package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LibraryFileSpec describes one source→target file mapping in a libraries.yml.
type LibraryFileSpec struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// LibrarySpec describes a single library entry in a libraries.yml.
type LibrarySpec struct {
	Help               string            `yaml:"help"`
	Usage              string            `yaml:"usage"`
	PostInstallMessage string            `yaml:"post_install_message"`
	SkipSrcCheck       bool              `yaml:"skip_src_check"`
	Files              []LibraryFileSpec `yaml:"files"`
}

// LibrarySource is a parsed libraries.yml keyed by library name.
type LibrarySource map[string]LibrarySpec

// loadLibrarySource reads and parses <dir>/libraries.yml.
func loadLibrarySource(dir string) (LibrarySource, error) {
	path := filepath.Join(dir, "libraries.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var src LibrarySource
	if err := yaml.Unmarshal(data, &src); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("%s: no libraries defined", path)
	}
	return src, nil
}

// effectiveConfig returns the project configuration, or an empty one (whose
// accessors expose the built-in path defaults) when no config file exists.
func effectiveConfig() *ShellyCfg {
	if cfg := readConfig(); cfg != nil {
		return cfg
	}
	return &ShellyCfg{}
}

// resolveLibraryTarget substitutes the %{user_*} path tokens in a library
// target path.
func resolveLibraryTarget(target string, cfg *ShellyCfg) string {
	repl := map[string]string{
		"%{user_source_dir}": cfg.sourceDir(),
		"%{user_target_dir}": cfg.targetDir(),
		"%{user_lib_dir}":    cfg.libDir(),
		"%{user_ext}":        "sh",
	}
	out := target
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

// addFromSource installs <library> from the libraries source directory <source>.
// Files are copied to their resolved target paths; an auto-upgrade marker is
// recorded for later refresh (issue 0016).
func addFromSource(source, library string, upgrade, quiet bool) error {
	cfg := effectiveConfig()
	libs, err := loadLibrarySource(source)
	if err != nil {
		return err
	}
	spec, ok := libs[library]
	if !ok {
		return fmt.Errorf("unknown library %q in source %s (run `shelly add --source %s --list`)", library, source, source)
	}
	if !spec.SkipSrcCheck {
		if _, err := os.Stat(cfg.sourceDir()); err != nil {
			return fmt.Errorf("directory %s does not exist (run `shelly init` first)", cfg.sourceDir())
		}
	}
	created := 0
	for _, f := range spec.Files {
		srcPath := filepath.Join(source, f.Source)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}
		content := libraryFileContent(source, library, string(data))
		target := resolveLibraryTarget(f.Target, cfg)
		wrote, err := writeLibraryFile(target, content, upgrade)
		if err != nil {
			return err
		}
		if wrote {
			created++
		}
	}
	if spec.PostInstallMessage != "" && created > 0 && !quiet {
		fmt.Printf("\n%s\n", spec.PostInstallMessage)
	}
	return nil
}

// libraryFileContent substitutes the bare upgrade-marker placeholder in a
// library file with a fully qualified "source;library" marker.
func libraryFileContent(source, library, data string) string {
	return strings.ReplaceAll(data,
		"[@shelly-upgrade]",
		fmt.Sprintf("[@shelly-upgrade %s;%s]", source, library))
}

// writeLibraryFile writes content to path, creating parent directories. When
// the file already exists and upgrade is false it is skipped. It reports
// whether the file was written.
func writeLibraryFile(path, content string, upgrade bool) (bool, error) {
	if !upgrade {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("skipped %s (exists)\n", path)
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("created %s\n", path)
	return true, nil
}

// listLibraries prints the libraries available in a source directory.
func listLibraries(source string) error {
	libs, err := loadLibrarySource(source)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(libs))
	for name := range libs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec := libs[name]
		title := name
		if spec.Usage != "" {
			title += " " + spec.Usage
		}
		fmt.Println(title)
		if spec.Help != "" {
			fmt.Printf("  %s\n", spec.Help)
		}
		fmt.Println()
	}
	return nil
}

// listBuiltins prints the built-in addons available to `shelly add`.
func listBuiltins() {
	builtins := []struct{ name, help string }{
		{"validations", "Validation helpers (not_empty, integer, file_exists, dir_exists)"},
		{"colors", "ANSI color helpers"},
		{"hooks", "before.sh / after.sh hook scaffolding"},
		{"preamble", "Starter preamble content"},
		{"strings", "Message-string overrides (shelly-strings.yml)"},
		{"yaml", "POSIX yaml_load helper library"},
		{"ini", "POSIX ini_load / ini_save library"},
		{"config", "POSIX config_* library (writes config.sh and ini.sh)"},
	}
	for _, b := range builtins {
		fmt.Printf("%s\n  %s\n\n", b.name, b.help)
	}
}
