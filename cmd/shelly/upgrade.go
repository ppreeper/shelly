package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// upgradeMarkerRe captures the payload of an upgrade marker, e.g.
// "## [@shelly-upgrade colors]" -> "colors", or
// "## [@shelly-upgrade libsrc;hello]" -> "libsrc;hello".
//
// The marker lines themselves (which are "##" comments) are stripped from the
// generated script by stripDesiredComments in shellgen.go.
var upgradeMarkerRe = regexp.MustCompile(`\[@shelly-upgrade ([^\]]+)\]`)

// upgradeLibs scans the source directory for files carrying an upgrade marker
// and refreshes each marked file from its originating library (built-in or
// custom source). Unknown libraries produce a warning and are skipped, never an
// error.
func upgradeLibs(cfg *ShellyCfg) error {
	src := cfg.sourceDir()
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		payload := findUpgradeMarker(string(data))
		if payload == "" {
			return nil
		}
		return upgradeFile(cfg, path, payload)
	})
}

// findUpgradeMarker returns the marker payload found in content, or "".
func findUpgradeMarker(content string) string {
	m := upgradeMarkerRe.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// upgradeFile refreshes a single marked file from its library. The payload is
// the first whitespace-delimited token of the marker; it is either a built-in
// addon name ("colors") or a custom "source;library" reference.
func upgradeFile(cfg *ShellyCfg, existing, payload string) error {
	fields := strings.Fields(payload)
	if len(fields) == 0 {
		return nil
	}
	spec := fields[0]
	if strings.Contains(spec, ";") {
		parts := strings.SplitN(spec, ";", 2)
		return upgradeFromSource(cfg, existing, parts[0], parts[1])
	}
	return upgradeBuiltin(cfg, existing, spec)
}

// upgradeBuiltin refreshes a marked file from a built-in addon, matching the
// file by basename.
func upgradeBuiltin(cfg *ShellyCfg, existing, name string) error {
	files, ok := builtinAddonFiles(cfg, name)
	if !ok {
		warnUnknownLibrary(existing, name)
		return nil
	}
	base := filepath.Base(existing)
	for _, f := range files {
		if filepath.Base(f.path) != base {
			continue
		}
		return writeUpdatedFile(existing, withUpgradeMarker(existing, f.content, name))
	}
	fmt.Fprintf(os.Stderr, "warning: not upgrading %s, path mismatch\n", existing)
	return nil
}

// upgradeFromSource refreshes a marked file from a custom library source,
// matching the file by its resolved target path.
func upgradeFromSource(cfg *ShellyCfg, existing, source, library string) error {
	libs, err := loadLibrarySource(source)
	if err != nil {
		warnUnknownLibrary(existing, library)
		return nil
	}
	spec, ok := libs[library]
	if !ok {
		warnUnknownLibrary(existing, library)
		return nil
	}
	for _, f := range spec.Files {
		target := resolveLibraryTarget(f.Target, cfg)
		if filepath.Clean(target) != filepath.Clean(existing) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(source, f.Source))
		if err != nil {
			return fmt.Errorf("upgrade %s: %w", existing, err)
		}
		return writeUpdatedFile(existing, libraryFileContent(source, library, string(data)))
	}
	fmt.Fprintf(os.Stderr, "warning: not upgrading %s, path mismatch\n", existing)
	return nil
}

// warnUnknownLibrary reports a marker whose library cannot be resolved.
func warnUnknownLibrary(existing, name string) {
	fmt.Fprintf(os.Stderr, "warning: not upgrading %s, unknown library %q\n", existing, name)
}

// writeUpdatedFile overwrites an existing file, creating parent directories, and
// reports the refresh.
func writeUpdatedFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("updated %s\n", path)
	return nil
}
