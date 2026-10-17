package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func write(f io.Writer, content string) {
	_, err := io.WriteString(f, content)
	if err != nil {
		fmt.Println("Error writing to shell script file:", err)
		os.Exit(1)
	}
}

// writeMarker emits a "# :command.xxx\n" section marker unless DisableViewMarkers is set.
func (cfg *ShellyCfg) writeMarker(f io.Writer, marker string) {
	if cfg.DisableViewMarkers {
		return
	}
	write(f, marker)
}

// expandTabs converts leading groups of two spaces into hard tabs (applied to
// the whole generated script when tab_indent is enabled). An odd trailing space
// is preserved.
func expandTabs(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		n := 0
		for n < len(line) && line[n] == ' ' {
			n++
		}
		if n < 2 {
			continue
		}
		lines[i] = strings.Repeat("\t", n/2) + strings.Repeat(" ", n%2) + line[n:]
	}
	return strings.Join(lines, "\n")
}

// debugf prints when SHELLY_DEBUG=1 or SHELLY_DEBUG=true
func debugf(format string, a ...interface{}) {
	v := os.Getenv("SHELLY_DEBUG")
	if v == "1" || strings.ToLower(v) == "true" {
		fmt.Printf(format, a...)
	}
}

func (cfg *ShellyCfg) shellGenCommands() {
	if len(cfg.Commands) == 0 {
		cfg.commandFunc("root")
		return
	}
	for _, command := range cfg.Commands {
		cfg.commandFunc(command.Name)
	}
}

// readSrcOrDefault reads <source_dir>/<name> if it exists, else returns def.
func (cfg *ShellyCfg) readSrcOrDefault(name, def string) string {
	p := filepath.Join(cfg.sourceDir(), name)
	b, err := os.ReadFile(p)
	if err != nil {
		return def
	}
	return string(b)
}

// stemName returns the filename without extension.
func stemName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// indentBody indents every non-empty line with two spaces unless already indented.
func indentBody(body string) string {
	return indentBodyWith(body, "  ")
}

// indentBodyWith indents every non-empty line with the given indent string unless already indented.
func indentBodyWith(body string, indent string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			out = append(out, "")
		} else if l[0] == ' ' || l[0] == '\t' {
			out = append(out, l)
		} else {
			out = append(out, indent+l)
		}
	}
	return strings.Join(out, "\n")
}

// indentBody is the cfg-aware version that uses cfg.indent() for tab_indent support.
func (cfg *ShellyCfg) indentBody(body string) string {
	return indentBodyWith(body, cfg.indent())
}

// shellSingleQuote wraps s in single quotes, escaping embedded single quotes.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// sanitizeVarName strips leading dashes and replaces inner dashes with underscores.
func sanitizeVarName(long string) string {
	return strings.ReplaceAll(strings.TrimLeft(long, "-"), "-", "_")
}

// sanitizeFuncName maps a name into a valid POSIX shell identifier fragment.
// Every character outside [A-Za-z0-9_] becomes '_'; in particular a hyphen is
// forbidden in a shell function name, so "go-blueprint" becomes "go_blueprint".
func sanitizeFuncName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// cmdPath records the two derived names of a command and its position in the
// command tree:
//
//   - raw is the "_"-joined command path used for source-file lookups (e.g.
//     "install_go-blueprint"). Hyphens are preserved so existing body files keep
//     resolving.
//   - disp is the space-separated path shown to users (e.g. "install go-blueprint").
//
// The shell function base for the command is sanitizeFuncName(raw). The zero
// value represents the root, so a top-level command's raw/disp are just its own
// name.
type cmdPath struct {
	raw  string
	disp string
}

// funcBase returns the shell identifier base for the command.
func (p cmdPath) funcBase() string { return sanitizeFuncName(p.raw) }

// child derives the cmdPath of a sub-command.
func (p cmdPath) child(c Command) cmdPath {
	raw, disp := c.Name, c.Name
	if p.raw != "" {
		raw = p.raw + "_" + c.Name
		disp = p.disp + " " + c.Name
	}
	// Function override: use the custom base for generated function names (and,
	// as before, for the default body-file lookup). The display path is unchanged.
	if c.Function != "" {
		raw = c.Function
	}
	return cmdPath{raw: raw, disp: disp}
}

// funcAppName returns the application name sanitized into a valid shell
// identifier fragment, used as the base for root-level function names
// (<app>_usage, <app>_parse). cfg.Name itself is kept raw for display text,
// output filenames and %APP_NAME% substitution.
func (cfg *ShellyCfg) funcAppName() string { return sanitizeFuncName(cfg.Name) }

// depsVarName returns the shell variable suffix used for a resolved dependency
// path. Every character outside [A-Za-z0-9_] becomes '_', so the resolved path
// is always exposed as the stable scalar deps_<suffix>, the POSIX stand-in for
// associative-array access.
func depsVarName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// flagVarName returns the shell variable name for a flag.
// Prefers Long; falls back to Short when Long is empty.
func flagVarName(f Flag) string {
	if f.Long != "" {
		return sanitizeVarName(f.Long)
	}
	return sanitizeVarName(f.Short)
}

// flagDisplayName returns the human-readable flag name for usage and error messages.
// When both Long and Short are set: "-s, --long"; when only Short: "-s"; when only Long: "--long".
func flagDisplayName(f Flag) string {
	if f.Short != "" && f.Long != "" {
		return f.Short + ", " + f.Long
	}
	if f.Short != "" {
		return f.Short
	}
	return f.Long
}

// flagSpellings returns every accepted spelling of a flag: long, short, then
// any aliases, in the order they are emitted as case arms.
func flagSpellings(f Flag) []string {
	var out []string
	if f.Long != "" {
		out = append(out, f.Long)
	}
	if f.Short != "" {
		out = append(out, f.Short)
	}
	out = append(out, f.allAliases()...)
	return out
}

// canonicalFlagVar maps a flag reference (canonical long/short or any alias) to
// the shell variable of the flag that owns it. Unknown references fall back to
// sanitizeVarName(name), preserving the original conflicts/needs behaviour.
func canonicalFlagVar(flags []Flag, name string) string {
	for _, f := range flags {
		for _, s := range flagSpellings(f) {
			if s == name {
				return flagVarName(f)
			}
		}
	}
	return sanitizeVarName(name)
}

// wrapText wraps s at word boundaries so each line is at most width runes.
// width <= 0 returns s unchanged.
func wrapText(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var out strings.Builder
	words := strings.Fields(s)
	lineLen := 0
	for i, w := range words {
		wLen := len(w)
		if i == 0 {
			out.WriteString(w)
			lineLen = wLen
			continue
		}
		if lineLen+1+wLen > width {
			out.WriteByte('\n')
			out.WriteString(w)
			lineLen = wLen
		} else {
			out.WriteByte(' ')
			out.WriteString(w)
			lineLen += 1 + wLen
		}
	}
	return out.String()
}

// quoteShellList formats a Go []string as a shell-quoted list for case patterns.
func quoteShellList(vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = "\"" + v + "\""
	}
	return strings.Join(quoted, " ")
}

// colorBase maps a base color-function name to its ANSI
// numeric foreground code.
var colorBase = map[string]string{
	"black":   "30",
	"red":     "31",
	"green":   "32",
	"yellow":  "33",
	"blue":    "34",
	"magenta": "35",
	"cyan":    "36",
	"white":   "37",
}

// ansiCode maps a color name to a POSIX printf ANSI escape sequence fragment.
//
// It accepts a color vocabulary (base colors plus the
// `bold`, `underlined`, `bold_underlined`, `<color>_bold` and
// `<color>_underlined` forms), a raw escape sequence (starts with \033, \e or
// the ESC byte), and returns "" for an empty name. Unknown names are passed
// through unchanged so raw sequences still work.
func ansiCode(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "":
		return ""
	case "bold":
		return "\\033[1m"
	case "dim":
		return "\\033[2m"
	case "underlined":
		return "\\033[4m"
	case "bold_underlined":
		return "\\033[1;4m"
	}
	// Raw escape sequences pass through unchanged.
	if strings.HasPrefix(n, `\033`) || strings.HasPrefix(n, `\e`) || strings.HasPrefix(n, "\x1b") {
		return name
	}
	// compound forms: <color>, <color>_bold, <color>_underlined.
	if base, style, _ := strings.Cut(n, "_"); base != "" {
		if code, ok := colorBase[base]; ok {
			switch style {
			case "":
				return "\\033[" + code + "m"
			case "bold":
				return "\\033[1;" + code + "m"
			case "underlined":
				return "\\033[4;" + code + "m"
			}
		}
	}
	// Unknown: pass through (raw sequence / unsupported name).
	return name
}

// colorEcho returns a shell echo/printf line that wraps text with an ANSI color.
// When colorName is empty, falls back to plain echo.
func colorEcho(indent, colorName, text string) string {
	code := ansiCode(colorName)
	if code == "" {
		return fmt.Sprintf("%secho \"%s\"\n", indent, text)
	}
	return fmt.Sprintf("%sprintf '%s%%s\\033[0m\\n' \"%s\"\n", indent, code, text)
}

// ─── Usage generation ────────────────────────────────────────────────────────

// writeCommandGrouping emits the grouped listing of a set of commands for a
// usage function. Commands are grouped by their Group field, preserving
// first-seen ordering; an empty group prints the generic "commands" caption and
// named groups print the "group" caption (e.g. "Language Development Commands:").
// Private commands are listed only inside a reveal guard. indent prefixes each
// emitted shell line (e.g. "  " for a top-level usage, "    " inside a block).
func (cfg *ShellyCfg) writeCommandGrouping(b *strings.Builder, cmds []Command, indent string) {
	// collect groups in order, preserving first-seen ordering
	groupOrder := []string{}
	groupMap := map[string][]Command{}
	privateCommands := []Command{}
	// bashly pads every command name to the width of the longest command name
	// in the whole listing (across all groups), then adds three spaces before
	// the help text.
	maxName := 0
	note := func(n string) {
		if len(n) > maxName {
			maxName = len(n)
		}
	}
	for _, c := range cmds {
		if c.Private {
			privateCommands = append(privateCommands, c)
			continue
		}
		g := c.Group
		if _, ok := groupMap[g]; !ok {
			groupOrder = append(groupOrder, g)
		}
		groupMap[g] = append(groupMap[g], c)
		note(c.Name)
		// if expose:true or expose:always, also list immediate subcommands
		if c.Expose.IsEnabled() {
			for _, sub := range c.Commands {
				if !sub.Private {
					note(c.Name + " " + sub.Name)
				}
			}
		}
	}

	for _, g := range groupOrder {
		if g != "" {
			b.WriteString(colorEcho(indent, cfg.UsageColors.Caption, cfg.m("group", map[string]string{"group": g})))
		} else {
			b.WriteString(colorEcho(indent, cfg.UsageColors.Caption, cfg.m("commands", nil)))
		}
		for _, c := range groupMap[g] {
			helpLine := strings.SplitN(c.Help, "\n", 2)[0]
			b.WriteString(fmt.Sprintf("%secho \"  %-*s   %s\"\n", indent, maxName, c.Name, helpLine))
			if c.Expose.IsEnabled() {
				for _, sub := range c.Commands {
					if !sub.Private {
						subHelp := strings.SplitN(sub.Help, "\n", 2)[0]
						b.WriteString(fmt.Sprintf("%secho \"  %-*s   %s\"\n", indent, maxName, c.Name+" "+sub.Name, subHelp))
					}
				}
			}
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
	}

	// private commands: shown only when reveal env var is set
	if len(privateCommands) > 0 {
		b.WriteString(fmt.Sprintf("%sif [ -n \"${%s}\" ]; then\n", indent, cfg.privateRevealEnvVar()))
		b.WriteString(fmt.Sprintf("%secho \"%s\"\n", indent, cfg.m("private_commands", nil)))
		for _, c := range privateCommands {
			b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, c.Name))
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
		b.WriteString(indent + "fi\n")
	}
}

func generateRootUsage(cfg *ShellyCfg) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_usage() {\n", cfg.funcAppName()))
	cfg.writeUsageBody(&b, usageNode{
		disp:               cfg.Name,
		help:               cfg.Help,
		helpHeaderOverride: cfg.HelpHeaderOverride,
		args:               cfg.Args,
		flags:              cfg.Flags,
		envs:               cfg.EnvironmentVariables,
		examples:           cfg.Examples,
		isRoot:             true,
		hasSubs:            len(cfg.Commands) > 0,
		subs:               cfg.Commands,
		footer:             cfg.Footer,
	})
	b.WriteString("}\n")
	return b.String()
}

func (cfg *ShellyCfg) generateCommandUsage(cp cmdPath, cmd Command) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_%s_usage() {\n", cfg.funcAppName(), cp.funcBase()))
	cfg.writeUsageBody(&b, usageNode{
		disp:               cfg.Name + " " + cp.disp,
		help:               cmd.Help,
		helpHeaderOverride: cmd.HelpHeaderOverride,
		aliases:            cmd.allAliases(),
		args:               cmd.Args,
		catchAll:           cmd.CatchAll,
		flags:              cmd.Flags,
		envs:               cmd.EnvironmentVariables,
		examples:           cmd.Examples,
		isRoot:             false,
		hasSubs:            len(cmd.Commands) > 0,
		subs:               cmd.Commands,
		footer:             cmd.Footer,
	})
	b.WriteString("}\n")
	return b.String()
}

// usageNode carries everything needed to render one help/usage function in the
// bashly-compatible layout. It is shared by the root usage and every command
// usage so that the format stays identical across the tree.
type usageNode struct {
	disp               string // display path, e.g. "gup install"; root = app name
	help               string
	helpHeaderOverride string
	aliases            []string
	args               []Arg
	catchAll           *CatchAllConfig
	flags              []Flag
	envs               []EnvironmentVariable
	examples           []string
	isRoot             bool
	hasSubs            bool
	subs               []Command
	footer             string
}

func (n usageNode) hasVisibleUserFlags() bool {
	for _, f := range n.flags {
		if !f.Private {
			return true
		}
	}
	return false
}

func (n usageNode) hasPrivateFlags() bool {
	for _, f := range n.flags {
		if f.Private {
			return true
		}
	}
	return false
}

// shortVAvailable reports whether -v can be used as the short spelling of
// --version (i.e. no user flag has claimed -v).
func (cfg *ShellyCfg) shortVAvailable(flags []Flag) bool {
	for _, f := range flags {
		if f.Short == "-v" {
			return false
		}
	}
	return true
}

// usageArgToken renders an argument as it appears in the one-line usage form,
// e.g. "NAME" (required), "[NAME]" (optional), "NAMES..." (repeatable).
func (cfg *ShellyCfg) usageArgToken(a Arg) string {
	name := strings.ToUpper(a.Name)
	switch {
	case a.Required && a.Repeatable:
		return " " + name + "..."
	case a.Required:
		return " " + name
	case a.Repeatable:
		return " [" + name + "...]"
	default:
		return " [" + name + "]"
	}
}

// usageFlagLabel renders a flag for the Options section, e.g. "--verbose, -v",
// "--mode MODE (required)", "--tag TAG (repeatable)" (bashly order: long first).
func (cfg *ShellyCfg) usageFlagLabel(f Flag) string {
	long := f.Long
	if f.Negatable && strings.HasPrefix(f.Long, "--") {
		long = "--[no-]" + strings.TrimPrefix(f.Long, "--")
	}
	var label string
	switch {
	case long != "" && f.Short != "":
		label = long + ", " + f.Short
	case long != "":
		label = long
	default:
		label = f.Short
	}
	if al := f.allAliases(); len(al) > 0 {
		if label == "" {
			label = strings.Join(al, ", ")
		} else {
			label += ", " + strings.Join(al, ", ")
		}
	}
	if f.Arg != "" {
		label += " " + strings.ToUpper(f.Arg)
	}
	if f.Required {
		label += " " + cfg.m("required", nil)
	}
	if f.Repeatable {
		label += " " + cfg.m("repeatable", nil)
	}
	return label
}

// flagUsageName renders a flag's spelling as bashly uses it in error messages:
// the long form (or short when no long form) plus an upper-cased argument name,
// e.g. "--level LEVEL" or "--force".
func (cfg *ShellyCfg) flagUsageName(f Flag) string {
	name := f.Long
	if name == "" {
		name = f.Short
	}
	if f.Arg != "" {
		name += " " + strings.ToUpper(f.Arg)
	}
	return name
}

// commandUsageLine renders bashly's single-line usage used in required-argument
// error messages, e.g. "mytool run TARGET [FLAVOR] [EXTRA...] [OPTIONS]".
func (cfg *ShellyCfg) commandUsageLine(disp string, cmd Command) string {
	tokens := ""
	for _, a := range cmd.Args {
		tokens += cfg.usageArgToken(a)
	}
	optStr := ""
	for _, f := range cmd.Flags {
		if !f.Private {
			optStr = " [OPTIONS]"
			break
		}
	}
	ca := ""
	if cmd.CatchAll != nil {
		label := strings.ToUpper(cmd.CatchAll.Label)
		if label == "" {
			label = "ARGS"
		}
		ca = " [--] " + label + "..."
	}
	return disp + tokens + optStr + ca
}

// writeUsageBody renders the complete bashly-compatible body of a usage
// function (everything between the function braces) into b.
func (cfg *ShellyCfg) writeUsageBody(b *strings.Builder, n usageNode) {
	// ── header ──────────────────────────────────────────────────────────────
	if n.helpHeaderOverride != "" {
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", n.helpHeaderOverride))
		b.WriteString("  echo \"\"\n")
	} else {
		helpLines := strings.Split(n.help, "\n")
		if len(helpLines) > 1 {
			// Multi-line help: short usage shows only the first line; long
			// usage shows every line indented by two spaces.
			b.WriteString("  if [ -n \"$long_usage\" ]; then\n")
			b.WriteString(fmt.Sprintf("    echo \"%s\"\n", n.disp))
			b.WriteString("    echo \"\"\n")
			for _, l := range helpLines {
				b.WriteString(fmt.Sprintf("    echo \"  %s\"\n", l))
			}
			b.WriteString("    echo \"\"\n")
			b.WriteString("  else\n")
			b.WriteString(fmt.Sprintf("    echo \"%s - %s\"\n", n.disp, helpLines[0]))
			b.WriteString("    echo \"\"\n")
			b.WriteString("  fi\n")
		} else {
			b.WriteString(fmt.Sprintf("  echo \"%s - %s\"\n", n.disp, helpLines[0]))
			b.WriteString("  echo \"\"\n")
		}
	}

	// ── alias line ──────────────────────────────────────────────────────────
	if len(n.aliases) > 0 {
		b.WriteString(fmt.Sprintf("  echo \"Alias: %s\"\n", strings.Join(n.aliases, ", ")))
		b.WriteString("  echo \"\"\n")
	}

	// ── usage block ─────────────────────────────────────────────────────────
	var forms []string
	if n.hasSubs {
		optStr := ""
		if n.hasVisibleUserFlags() {
			optStr = " [OPTIONS]"
		}
		forms = append(forms, n.disp+optStr+" COMMAND")
		forms = append(forms, n.disp+" [COMMAND] --help | -h")
	} else {
		tokens := ""
		for _, a := range n.args {
			tokens += cfg.usageArgToken(a)
		}
		optStr := ""
		if n.hasVisibleUserFlags() {
			optStr = " [OPTIONS]"
		}
		ca := ""
		if n.catchAll != nil {
			label := strings.ToUpper(n.catchAll.Label)
			if label == "" {
				label = "ARGS"
			}
			ca = " [--] " + label + "..."
		}
		forms = append(forms, n.disp+tokens+optStr+ca)
		forms = append(forms, n.disp+" --help | -h")
	}
	if n.isRoot {
		ver := n.disp + " --version"
		if cfg.shortVAvailable(n.flags) {
			ver += " | -v"
		}
		forms = append(forms, ver)
	}
	b.WriteString(colorEcho("  ", cfg.UsageColors.Caption, cfg.m("usage", nil)))
	for _, u := range forms {
		b.WriteString(fmt.Sprintf("  echo \"  %s\"\n", u))
	}
	b.WriteString("  echo \"\"\n")

	// ── subcommand listing (outside long_usage, matching bashly) ────────────
	if n.hasSubs {
		cfg.writeCommandGrouping(b, n.subs, "  ")
	}

	// ── long usage sections ─────────────────────────────────────────────────
	b.WriteString("  if [ -n \"$long_usage\" ]; then\n")
	cfg.writeOptionsHelp(b, n)
	cfg.writeArgumentsHelp(b, n)
	cfg.writeEnvHelp(b, n)
	cfg.writeExamplesHelp(b, n)
	b.WriteString("  fi\n")

	// ── footer (shelly extension) ───────────────────────────────────────────
	if n.footer != "" {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", n.footer))
	}
}

// emitHelpLines writes one or more echo lines for a help description, wrapping
// at the configured word_wrap width (default 80). Every continuation line is
// indented by four spaces, matching the bashly layout.
func (cfg *ShellyCfg) emitHelpLines(b *strings.Builder, indent, text string) {
	const lead = "    "
	wrap := cfg.wordWrap()
	if wrap <= 0 || len(lead)+len(text) <= wrap {
		b.WriteString(fmt.Sprintf("%secho \"%s%s\"\n", indent, lead, text))
		return
	}
	avail := wrap - len(lead)
	for _, l := range strings.Split(wrapText(text, avail), "\n") {
		b.WriteString(fmt.Sprintf("%secho \"%s%s\"\n", indent, lead, l))
	}
}

// writeOptionsHelp emits the Options/Global Options section (user flags in
// declaration order, then the fixed --help/--version flags).
func (cfg *ShellyCfg) writeOptionsHelp(b *strings.Builder, n usageNode) {
	const indent = "    "
	caption := cfg.m("options", nil)
	if n.isRoot && n.hasSubs && n.hasVisibleUserFlags() {
		caption = cfg.m("global_options", nil)
	}
	b.WriteString(colorEcho(indent, cfg.UsageColors.Caption, caption))
	for _, f := range n.flags {
		if f.Private {
			continue
		}
		b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, cfg.usageFlagLabel(f)))
		cfg.emitHelpLines(b, indent, f.Help)
		if len(f.Allowed) > 0 {
			b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("allowed", map[string]string{"values": strings.Join(f.Allowed, ", ")})))
		}
		if f.Default != "" {
			b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("default", map[string]string{"value": f.Default})))
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
	}
	// fixed flags
	b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, "--help, -h"))
	b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("help_flag_text", nil)))
	b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
	if n.isRoot {
		ver := "--version"
		if cfg.shortVAvailable(n.flags) {
			ver = "--version, -v"
		}
		b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, ver))
		b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("version_flag_text", nil)))
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
	}

	// private flags: shown only when the reveal env var is set
	if n.hasPrivateFlags() && cfg.privateRevealEnvVar() != "" {
		b.WriteString(fmt.Sprintf("%sif [ -n \"${%s}\" ]; then\n", indent, cfg.privateRevealEnvVar()))
		b.WriteString(fmt.Sprintf("%secho \"%s\"\n", indent, cfg.m("private_options", nil)))
		for _, f := range n.flags {
			if !f.Private {
				continue
			}
			b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, cfg.usageFlagLabel(f)))
			cfg.emitHelpLines(b, indent, f.Help)
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
		b.WriteString(indent + "fi\n")
	}
}

// writeArgumentsHelp emits the Arguments section (declared positionals, then
// catch_all rendered as "<label>...").
func (cfg *ShellyCfg) writeArgumentsHelp(b *strings.Builder, n usageNode) {
	const indent = "    "
	if len(n.args) == 0 && n.catchAll == nil {
		return
	}
	b.WriteString(colorEcho(indent, cfg.UsageColors.Caption, cfg.m("arguments", nil)))
	for _, a := range n.args {
		label := strings.ToUpper(a.Name)
		if a.Repeatable {
			label += "..."
		}
		b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, label))
		cfg.emitHelpLines(b, indent, a.Help)
		if len(a.Allowed) > 0 {
			b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("allowed", map[string]string{"values": strings.Join(a.Allowed, ", ")})))
		}
		if a.Default != "" {
			b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("default", map[string]string{"value": a.Default})))
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
	}
	if n.catchAll != nil {
		label := strings.ToUpper(n.catchAll.Label)
		if label == "" {
			label = "ARGS"
		}
		b.WriteString(fmt.Sprintf("%secho \"  %s...\"\n", indent, label))
		if n.catchAll.Help != "" {
			cfg.emitHelpLines(b, indent, n.catchAll.Help)
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
	}
}

// writeEnvHelp emits the Environment Variables section.
func (cfg *ShellyCfg) writeEnvHelp(b *strings.Builder, n usageNode) {
	const indent = "    "
	var visible, private []EnvironmentVariable
	for _, ev := range n.envs {
		if ev.Private {
			private = append(private, ev)
		} else {
			visible = append(visible, ev)
		}
	}
	if len(visible) == 0 && (len(private) == 0 || cfg.privateRevealEnvVar() == "") {
		return
	}
	if len(visible) > 0 {
		b.WriteString(colorEcho(indent, cfg.UsageColors.Caption, cfg.m("environment_variables", nil)))
		for _, ev := range visible {
			b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, ev.Name))
			cfg.emitHelpLines(b, indent, ev.Help)
			if len(ev.Allowed) > 0 {
				b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("allowed", map[string]string{"values": strings.Join(ev.Allowed, ", ")})))
			}
			if ev.Default != "" {
				b.WriteString(fmt.Sprintf("%secho \"    %s\"\n", indent, cfg.m("default", map[string]string{"value": ev.Default})))
			}
			b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
		}
	}
	if len(private) > 0 && cfg.privateRevealEnvVar() != "" {
		b.WriteString(fmt.Sprintf("%sif [ -n \"${%s}\" ]; then\n", indent, cfg.privateRevealEnvVar()))
		b.WriteString(fmt.Sprintf("%secho \"%s\"\n", indent, cfg.m("private_environment_variables", nil)))
		for _, ev := range private {
			b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, ev.Name))
			cfg.emitHelpLines(b, indent, ev.Help)
		}
		b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
		b.WriteString(indent + "fi\n")
	}
}

// writeExamplesHelp emits the Examples section.
func (cfg *ShellyCfg) writeExamplesHelp(b *strings.Builder, n usageNode) {
	const indent = "    "
	if len(n.examples) == 0 {
		return
	}
	b.WriteString(colorEcho(indent, cfg.UsageColors.Caption, cfg.m("examples", nil)))
	for _, ex := range n.examples {
		b.WriteString(fmt.Sprintf("%secho \"  %s\"\n", indent, ex))
	}
	b.WriteString(fmt.Sprintf("%secho \"\"\n", indent))
}

// ─── Unified argument parser generation ──────────────────────────────────────

// writeFlagArms emits the case branches for every flag of cmd into b. It is
// shared by the command parser and the root/dispatch flag loop.
func writeFlagArms(b *strings.Builder, cmd Command, otherArgs string, conjoined bool) {
	for _, f := range cmd.Flags {
		varName := flagVarName(f)
		for _, name := range flagSpellings(f) {
			isLong := strings.HasPrefix(name, "--")
			if f.Arg != "" {
				// inline --flag=value is always available; -flag=value only
				// when conjoined_flag_args is enabled.
				if isLong || conjoined {
					if f.Repeatable {
						b.WriteString(fmt.Sprintf("      %s=*) %s=\"${%s} ${1#*=}\"; %s=\"${%s# }\"; shift ;;\n", name, varName, varName, varName, varName))
					} else {
						b.WriteString(fmt.Sprintf("      %s=*) %s=\"${1#*=}\"; shift ;;\n", name, varName))
					}
				}
				if f.Repeatable {
					b.WriteString(fmt.Sprintf("      %s) %s=\"${%s} $2\"; %s=\"${%s# }\"; shift 2 ;;\n", name, varName, varName, varName, varName))
				} else {
					b.WriteString(fmt.Sprintf("      %s) %s=\"$2\"; shift 2 ;;\n", name, varName))
				}
			} else {
				if f.Repeatable {
					b.WriteString(fmt.Sprintf("      %s) %s=$((${%s:-0}+1)); shift ;;\n", name, varName, varName))
				} else {
					b.WriteString(fmt.Sprintf("      %s) %s=1; shift ;;\n", name, varName))
				}
				// negatable: --no-<long> unsets the value
				if f.Negatable && isLong {
					b.WriteString(fmt.Sprintf("      --no-%s) %s=\"\"; shift ;;\n", strings.TrimPrefix(name, "--"), varName))
				}
			}
		}
	}
}

// writeFlagCases emits the flag case branches for cmd. The -h|--help branch is
// deliberately NOT emitted here: it is handled by a leading check in the parser
// so that help is only honoured before the first positional argument (matching
// bashly). The exception is catch_help:true, where -h|--help is accumulated into
// other_args wherever it appears.
func writeFlagCases(b *strings.Builder, cmd Command, otherArgs string, conjoined bool) {
	writeFlagArms(b, cmd, otherArgs, conjoined)
	if cmd.CatchAll != nil && cmd.CatchAll.CatchHelp {
		b.WriteString(fmt.Sprintf("      -h|--help) %s=\"${%s} $1\"; shift ;;\n", otherArgs, otherArgs))
	}
}

// writePositionalCase emits the *) branch that assigns positional arguments to
// their variables, tracking a running index (_pos) so that a trailing
// repeatable argument can accumulate every overflow token. Overflow beyond the
// declared positionals is routed to catch_all when present, otherwise it is an
// error (the invalid-argument behaviour).
func (cfg *ShellyCfg) writePositionalCase(b *strings.Builder, cmd Command, otherArgs string, usageFunc string) {
	args := cmd.Args
	n := len(args)
	repeatIdx := -1
	for i, a := range args {
		if a.Repeatable {
			repeatIdx = i
			break
		}
	}
	simpleEnd := n
	if repeatIdx >= 0 {
		simpleEnd = repeatIdx
	}

	if n == 0 {
		if cmd.CatchAll != nil {
			b.WriteString(fmt.Sprintf("      *) %s=\"${%s} $1\"; shift ;;\n", otherArgs, otherArgs))
		} else {
			// No positionals declared: stop parsing (preserve prior behaviour).
			b.WriteString("      *) break ;;\n")
		}
		return
	}

	b.WriteString("      *)\n")
	b.WriteString("        _pos=$((_pos + 1))\n")
	if simpleEnd > 0 {
		b.WriteString("        case \"$_pos\" in\n")
		for i := 0; i < simpleEnd; i++ {
			b.WriteString(fmt.Sprintf("          %d) %s=\"$1\" ;;\n", i+1, sanitizeVarName(args[i].Name)))
		}
		if repeatIdx < 0 {
			if cmd.CatchAll != nil {
				b.WriteString(fmt.Sprintf("          *) %s=\"${%s} $1\" ;;\n", otherArgs, otherArgs))
			} else {
				b.WriteString(fmt.Sprintf("          *) echo \"%s\" >&2; %s; exit 1 ;;\n", cfg.m("too_many_arguments", nil, "$1"), usageFunc))
			}
		} else {
			b.WriteString("          *) ;;\n")
		}
		b.WriteString("        esac\n")
	}
	if repeatIdx >= 0 {
		rv := sanitizeVarName(args[repeatIdx].Name)
		b.WriteString(fmt.Sprintf("        if [ \"$_pos\" -ge %d ]; then %s=\"${%s} $1\" ; fi\n", repeatIdx+1, rv, rv))
	}
	b.WriteString("        shift ;;\n")
}

// writeExamplesOnError appends an Examples block after an error message when
// the command opts in via show_examples_on_error.
func (cfg *ShellyCfg) writeExamplesOnError(b *strings.Builder, cmd Command) {
	if cmd.ShowExamplesOnError && len(cmd.Examples) > 0 {
		b.WriteString("    echo \"\"\n")
		b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("examples", nil)))
		for _, ex := range cmd.Examples {
			b.WriteString(fmt.Sprintf("    echo \"  %s\"\n", ex))
		}
	}
}

// writeRequiredChecks emits required-argument, required-catch_all and
// required-flag validation in bashly's order: arguments first, then flags.
// Matching bashly, a value counts as missing only when its variable is unset
// (${var+x}); only argument errors append a one-line usage reminder.
func (cfg *ShellyCfg) writeRequiredChecks(b *strings.Builder, cmd Command, usageLine string) {
	for _, a := range cmd.Args {
		if !a.Required {
			continue
		}
		varName := sanitizeVarName(a.Name)
		b.WriteString(fmt.Sprintf("  if [ -z \"${%s+x}\" ]; then\n", varName))
		b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n",
			cfg.m("missing_required_argument", map[string]string{"name": strings.ToUpper(a.Name)})))
		if usageLine != "" {
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n",
				cfg.m("usage_error", map[string]string{"usage": usageLine})))
		}
		cfg.writeExamplesOnError(b, cmd)
		b.WriteString("    exit 1\n")
		b.WriteString("  fi\n")
	}
	if cmd.CatchAll != nil && cmd.CatchAll.Required {
		label := strings.ToUpper(cmd.CatchAll.Label)
		if label == "" {
			label = "EXTRA"
		}
		b.WriteString(fmt.Sprintf("  if [ -z \"${%s}\" ]; then\n", cfg.otherArgsVar()))
		b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n",
			cfg.m("missing_required_argument", map[string]string{"name": label + "..."})))
		if usageLine != "" {
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n",
				cfg.m("usage_error", map[string]string{"usage": usageLine})))
		}
		cfg.writeExamplesOnError(b, cmd)
		b.WriteString("    exit 1\n")
		b.WriteString("  fi\n")
	}
	for _, f := range cmd.Flags {
		if !f.Required {
			continue
		}
		varName := flagVarName(f)
		b.WriteString(fmt.Sprintf("  if [ -z \"${%s+x}\" ]; then\n", varName))
		b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n",
			cfg.m("missing_required_flag", map[string]string{"name": cfg.flagUsageName(f)})))
		cfg.writeExamplesOnError(b, cmd)
		b.WriteString("    exit 1\n")
		b.WriteString("  fi\n")
	}
}

// writeFlagPostChecks emits the post-loop checks for cmd's flags: defaults,
// allowed, conflicts, needs, unique dedup and validators.
func (cfg *ShellyCfg) writeFlagPostChecks(b *strings.Builder, cmd Command) {
	// default injection
	for _, f := range cmd.Flags {
		varName := flagVarName(f)
		if len(f.DefaultList) > 0 {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, strings.Join(f.DefaultList, " ")))
		} else if f.Default != "" {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, f.Default))
		}
	}

	// allowed value checks for flags
	for _, f := range cmd.Flags {
		if len(f.Allowed) > 0 && f.Arg != "" {
			varName := flagVarName(f)
			displayName := flagDisplayName(f)
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", varName))
			b.WriteString(fmt.Sprintf("    case \"${%s}\" in\n", varName))
			b.WriteString(fmt.Sprintf("      %s) ;;\n", strings.Join(f.Allowed, "|")))
			b.WriteString(fmt.Sprintf("      *) echo \"%s\" >&2; exit 1 ;;\n",
				cfg.m("disallowed_flag", map[string]string{"name": displayName, "allowed": strings.Join(f.Allowed, ", ")})))
			b.WriteString("    esac\n")
			b.WriteString("  fi\n")
		}
	}

	// conflicts checks
	for _, f := range cmd.Flags {
		if len(f.Conflicts) > 0 {
			varName := flagVarName(f)
			displayName := flagDisplayName(f)
			for _, conflict := range f.Conflicts {
				conflictVar := canonicalFlagVar(cmd.Flags, conflict)
				b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ] && [ -n \"${%s}\" ]; then\n", varName, conflictVar))
				b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("conflicting_flags", map[string]string{"name": displayName, "other": conflict})))
				b.WriteString("    exit 1\n")
				b.WriteString("  fi\n")
			}
		}
	}

	// needs checks
	for _, f := range cmd.Flags {
		if len(f.Needs) > 0 {
			varName := flagVarName(f)
			displayName := flagDisplayName(f)
			for _, need := range f.Needs {
				needVar := canonicalFlagVar(cmd.Flags, need)
				b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ] && [ -z \"${%s}\" ]; then\n", varName, needVar))
				b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("flag_needs_another", map[string]string{"name": displayName, "need": need})))
				b.WriteString("    exit 1\n")
				b.WriteString("  fi\n")
			}
		}
	}

	// unique dedup for repeatable value flags
	for _, f := range cmd.Flags {
		if f.Repeatable && f.Unique && f.Arg != "" {
			varName := flagVarName(f)
			dedupVar := "_dedup_" + varName
			b.WriteString(fmt.Sprintf("  %s=\"\"\n", dedupVar))
			b.WriteString(fmt.Sprintf("  for _item in ${%s}; do\n", varName))
			b.WriteString(fmt.Sprintf("    case \" ${%s} \" in\n", dedupVar))
			b.WriteString("      *\" ${_item} \"*) ;;\n")
			b.WriteString(fmt.Sprintf("      *) %s=\"${%s} ${_item}\" ;;\n", dedupVar, dedupVar))
			b.WriteString("    esac\n")
			b.WriteString("  done\n")
			b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", varName, dedupVar))
		}
	}

	// validate calls for flags
	for _, f := range cmd.Flags {
		if f.Arg != "" {
			varName := flagVarName(f)
			displayName := flagDisplayName(f)
			validators := []string{}
			if f.Validate != "" {
				validators = append(validators, f.Validate)
			}
			validators = append(validators, f.ValidateList...)
			for _, v := range validators {
				b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", varName))
				b.WriteString(fmt.Sprintf("    validate_%s \"%s\" \"${%s}\"\n", v, displayName, varName))
				b.WriteString("  fi\n")
			}
		}
	}
}

// writeArgPostChecks emits the post-loop checks for cmd's positional args:
// defaults, required, allowed, unique dedup and validators.
func (cfg *ShellyCfg) writeArgPostChecks(b *strings.Builder, cmd Command) {
	// defaults
	for _, a := range cmd.Args {
		varName := sanitizeVarName(a.Name)
		if len(a.DefaultList) > 0 {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, strings.Join(a.DefaultList, " ")))
		} else if a.Default != "" {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, a.Default))
		}
	}

	// allowed checks
	for _, a := range cmd.Args {
		if len(a.Allowed) > 0 {
			varName := sanitizeVarName(a.Name)
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", varName))
			b.WriteString(fmt.Sprintf("    case \"${%s}\" in\n", varName))
			b.WriteString(fmt.Sprintf("      %s) ;;\n", strings.Join(a.Allowed, "|")))
			b.WriteString(fmt.Sprintf("      *) echo \"%s\" >&2; exit 1 ;;\n",
				cfg.m("disallowed_argument", map[string]string{"name": a.Name, "allowed": strings.Join(a.Allowed, ", ")})))
			b.WriteString("    esac\n")
			b.WriteString("  fi\n")
		}
	}

	// unique dedup for repeatable args
	for _, a := range cmd.Args {
		if a.Repeatable && a.Unique {
			varName := sanitizeVarName(a.Name)
			dedupVar := "_dedup_" + varName
			b.WriteString(fmt.Sprintf("  %s=\"\"\n", dedupVar))
			b.WriteString(fmt.Sprintf("  for _item in ${%s}; do\n", varName))
			b.WriteString(fmt.Sprintf("    case \" ${%s} \" in\n", dedupVar))
			b.WriteString("      *\" ${_item} \"*) ;;\n")
			b.WriteString(fmt.Sprintf("      *) %s=\"${%s} ${_item}\" ;;\n", dedupVar, dedupVar))
			b.WriteString("    esac\n")
			b.WriteString("  done\n")
			b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", varName, dedupVar))
		}
	}

	// validate calls for args
	for _, a := range cmd.Args {
		varName := sanitizeVarName(a.Name)
		validators := []string{}
		if a.Validate != "" {
			validators = append(validators, a.Validate)
		}
		validators = append(validators, a.ValidateList...)
		for _, v := range validators {
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", varName))
			b.WriteString(fmt.Sprintf("    validate_%s \"%s\" \"${%s}\"\n", v, a.Name, varName))
			b.WriteString("  fi\n")
		}
	}
}

// generateParser builds a single <cmd>_parse() function that walks the command
// line exactly once, interleaving flags and positionals. Tokens after `--` are
// treated as positionals. When dispatch is true it produces <cmd>_parse() used to consume
// leading global flags before command dispatch (it stops at the first
// non-flag token).
func (cfg *ShellyCfg) generateParser(cmd Command, dispatch bool, usageFunc, disp string) string {
	otherArgs := cfg.otherArgsVar()
	usageLine := cfg.commandUsageLine(disp, cmd)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_parse() {\n", cmd.Name))

	if cmd.CatchAll != nil {
		b.WriteString(fmt.Sprintf("  %s=\"\"\n", otherArgs))
	}
	if !dispatch {
		b.WriteString("  _options_ended=false\n")
		b.WriteString("  _pos=0\n")
	}

	// help is only honoured while it is still the leading token (before any
	// positional argument is consumed), matching bashly. With catch_help it is
	// collected into other_args by writeFlagCases instead.
	if cmd.CatchAll == nil || !cmd.CatchAll.CatchHelp {
		b.WriteString("  if [ \"${1:-}\" = \"-h\" ] || [ \"${1:-}\" = \"--help\" ]; then\n")
		b.WriteString("    long_usage=yes\n")
		b.WriteString(fmt.Sprintf("    %s\n", usageFunc))
		b.WriteString("    exit 0\n")
		b.WriteString("  fi\n")
	}

	b.WriteString("  while [ $# -gt 0 ]; do\n")
	if dispatch {
		b.WriteString("    case \"$1\" in\n")
	} else {
		b.WriteString("    _parse_key=\"$1\"\n")
		b.WriteString("    if [ \"$_options_ended\" = true ]; then\n")
		b.WriteString("      _parse_key=\"\"\n")
		b.WriteString("    fi\n")
		b.WriteString("    case \"$_parse_key\" in\n")
	}

	writeFlagCases(&b, cmd, otherArgs, cfg.conjoinedFlagArgs())

	// compact short flags: expand a combined group (-abc) into -a -b -c and
	// re-queue the individual flags for the loop to parse. The pattern requires
	// at least two alphanumeric characters after '-', so long flags (--x) and
	// single short flags (-a) fall through to the arms below.
	if cfg.compactShortFlags() {
		b.WriteString("      -[a-zA-Z0-9][a-zA-Z0-9]*)\n")
		b.WriteString("        _cluster=\"${1#-}\"\n")
		b.WriteString("        shift\n")
		b.WriteString("        while [ -n \"$_cluster\" ]; do\n")
		b.WriteString("          _c=\"${_cluster#\"${_cluster%?}\"}\"\n")
		b.WriteString("          _cluster=\"${_cluster%?}\"\n")
		b.WriteString("          set -- \"-$_c\" \"$@\"\n")
		b.WriteString("        done\n")
		b.WriteString("        continue ;;\n")
	}

	if dispatch {
		b.WriteString("      --) shift; break ;;\n")
	} else {
		b.WriteString("      --) shift; _options_ended=true ;;\n")
	}
	b.WriteString(fmt.Sprintf("      -*) echo \"%s\" >&2; exit 1 ;;\n", cfg.m("invalid_flag", nil, "$1")))
	if dispatch {
		b.WriteString("      *) break ;;\n")
	} else {
		cfg.writePositionalCase(&b, cmd, otherArgs, usageFunc)
	}
	b.WriteString("    esac\n")
	b.WriteString("  done\n")

	// catch_all trim
	if cmd.CatchAll != nil {
		b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", otherArgs, otherArgs))
	}

	// trim leading space from a repeatable positional accumulator
	for _, a := range cmd.Args {
		if a.Repeatable {
			v := sanitizeVarName(a.Name)
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s+x}\" ]; then %s=\"${%s# }\"; fi\n", v, v, v))
			break
		}
	}

	// bashly order: required arguments/catch_all, required flags, then defaults
	// and the remaining validations.
	cfg.writeRequiredChecks(&b, cmd, usageLine)
	cfg.writeFlagPostChecks(&b, cmd)
	cfg.writeArgPostChecks(&b, cmd)

	b.WriteString("}\n")
	return b.String()
}

// ─── Dependencies / env var checks ───────────────────────────────────────────

// generateDepsCheck builds a parse_requirements() body checking each dependency.
func (cfg *ShellyCfg) generateDepsCheck(deps []Dependency) string {
	if len(deps) == 0 {
		return ""
	}
	var b strings.Builder
	for _, d := range deps {
		names := d.depNames()
		if len(names) == 0 {
			continue
		}
		if len(names) == 1 {
			name := names[0]
			depVar := depsVarName(name)
			// single dep — simple check
			b.WriteString(fmt.Sprintf("  if ! command -v %s >/dev/null 2>&1; then\n", name))
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("missing_dependency", map[string]string{"name": name})))
			if d.Help != "" {
				b.WriteString(fmt.Sprintf("    echo \"  %s\" >&2\n", d.Help))
			}
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
			// capture path into deps_<name> variable
			b.WriteString(fmt.Sprintf("  deps_%s=\"$(command -v %s)\"\n", depVar, name))
			// version check if requested
			if d.Version != "" {
				b.WriteString(fmt.Sprintf("  if ! %s --version 2>&1 | grep -q '%s'; then\n", name, d.Version))
				b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("dependency_version", map[string]string{"name": name, "version": d.Version})))
				b.WriteString("    exit 1\n")
				b.WriteString("  fi\n")
			}
		} else {
			// OR-syntax: any one of the names satisfies the requirement
			conditions := make([]string, len(names))
			for i, n := range names {
				conditions[i] = fmt.Sprintf("! command -v %s >/dev/null 2>&1", n)
			}
			b.WriteString(fmt.Sprintf("  if %s; then\n", strings.Join(conditions, " && ")))
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("missing_dependency_one_of", map[string]string{"names": strings.Join(names, ", ")})))
			if d.Help != "" {
				b.WriteString(fmt.Sprintf("    echo \"  %s\" >&2\n", d.Help))
			}
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
			// capture first found into deps_<name>
			for _, n := range names {
				depVar := depsVarName(n)
				b.WriteString(fmt.Sprintf("  if command -v %s >/dev/null 2>&1; then deps_%s=\"$(command -v %s)\"; fi\n", n, depVar, n))
			}
		}
	}
	return b.String()
}

// depEntry is a logical dependency and the command names that can satisfy it.
type depEntry struct {
	key        string   // logical dependency name
	candidates []string // external commands, any one of which satisfies it
}

// collectDependencies walks the whole config (root, all commands, nested
// commands) and returns every declared dependency keyed by its logical name.
func (cfg *ShellyCfg) collectDependencies() []depEntry {
	var out []depEntry
	add := func(deps []Dependency) {
		for _, d := range deps {
			names := d.depNames()
			if len(names) == 0 {
				continue
			}
			key := d.Name
			if key == "" {
				key = names[0]
			}
			out = append(out, depEntry{key: key, candidates: names})
		}
	}
	add(cfg.Dependencies)
	var walk func(cmds []Command)
	walk = func(cmds []Command) {
		for _, c := range cmds {
			add(c.Dependencies)
			walk(c.Commands)
		}
	}
	walk(cfg.Commands)
	return out
}

// generateDepsHelper emits the deps_get() helper: a POSIX, eval-free way to
// look up a dependency by logical name. It prints the resolved path of a
// dependency (or nothing when unresolved) and returns 1 for unknown names.
func (cfg *ShellyCfg) generateDepsHelper(entries []depEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# deps_get: print the resolved path of a dependency by name.\n")
	b.WriteString("# Use $(deps_get NAME) to look up a resolved dependency path.\n")
	b.WriteString("deps_get() {\n")
	b.WriteString("  case \"$1\" in\n")
	seen := map[string]bool{}
	// emitPath appends the printf for a resolved dependency variable.
	emitPath := func(name string) {
		b.WriteString(fmt.Sprintf("printf '%%s' \"${deps_%s:-}\"", depsVarName(name)))
	}
	// emitSingle handles a logical entry whose candidates resolve one-to-one.
	emitSingle := func(key, candidate string) {
		seen[key] = true
		b.WriteString(fmt.Sprintf("    %s) ", key))
		emitPath(candidate)
		b.WriteString(" ;;\n")
	}
	for _, e := range entries {
		if e.key == "" {
			continue
		}
		if len(e.candidates) == 1 {
			if !seen[e.key] {
				emitSingle(e.key, e.candidates[0])
			}
			continue
		}
		if !seen[e.key] {
			seen[e.key] = true
			b.WriteString(fmt.Sprintf("    %s)\n", e.key))
			for i, c := range e.candidates {
				kw := "if"
				if i > 0 {
					kw = "elif"
				}
				b.WriteString(fmt.Sprintf("      %s [ -n \"${deps_%s:-}\" ]; then ", kw, depsVarName(c)))
				emitPath(c)
				b.WriteString("\n")
			}
			b.WriteString("      fi ;;\n")
		}
		// Also expose each candidate command name directly.
		for _, c := range e.candidates {
			if !seen[c] {
				emitSingle(c, c)
			}
		}
	}
	b.WriteString("    *) return 1 ;;\n")
	b.WriteString("  esac\n")
	b.WriteString("}\n")
	return b.String()
}

// generateEnvCheck builds validation for a slice of EnvironmentVariables.
func (cfg *ShellyCfg) generateEnvCheck(evs []EnvironmentVariable) string {
	if len(evs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, ev := range evs {
		// apply default
		if ev.Default != "" {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", ev.Name, ev.Default))
		}
		if ev.Required {
			b.WriteString(fmt.Sprintf("  if [ -z \"${%s}\" ]; then\n", ev.Name))
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("missing_required_environment_variable", map[string]string{"name": ev.Name})))
			if ev.Help != "" {
				b.WriteString(fmt.Sprintf("    echo \"  %s\" >&2\n", ev.Help))
			}
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
		}
		// allowed value check
		if len(ev.Allowed) > 0 {
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", ev.Name))
			b.WriteString(fmt.Sprintf("    case \"${%s}\" in\n", ev.Name))
			b.WriteString(fmt.Sprintf("      %s) ;;\n", strings.Join(ev.Allowed, "|")))
			b.WriteString(fmt.Sprintf("      *) echo \"%s\" >&2; exit 1 ;;\n",
				cfg.m("disallowed_environment_variable", map[string]string{"name": ev.Name, "allowed": strings.Join(ev.Allowed, ", ")})))
			b.WriteString("    esac\n")
			b.WriteString("  fi\n")
		}
		// validate call
		validators := []string{}
		if ev.Validate != "" {
			validators = append(validators, ev.Validate)
		}
		validators = append(validators, ev.ValidateList...)
		for _, v := range validators {
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", ev.Name))
			b.WriteString(fmt.Sprintf("    validate_%s \"%s\" \"${%s}\"\n", v, ev.Name, ev.Name))
			b.WriteString("  fi\n")
		}
	}
	return b.String()
}

// ─── Recursive command block writer ──────────────────────────────────────────

// writeCommandFunctions recursively writes flag parsers, arg parsers, and
// command bodies for a slice of commands. prefix is the function-name prefix
// built so far (e.g. "remote" for top-level, "remote_add" for nested).
// For commands that have sub-commands, a dispatcher body is generated instead
// of reading a src body file.
func writeCommandFunctions(f io.Writer, cfg *ShellyCfg, cmds []Command, parent cmdPath) {
	for _, cmd := range cmds {
		cp := parent.child(cmd)
		funcBase := cp.funcBase()

		if len(cmd.Commands) > 0 {
			// This is a parent command — generate usage + dispatcher, recurse.
			// Usage for parent command listing its children.
			parentUsage := cfg.generateNestedUsage(cp, cmd)
			cfg.writeMarker(f, fmt.Sprintf("# :command.usage.%s\n", funcBase))
			write(f, parentUsage+"\n\n")

			// Recurse into children first (so children are defined before parent).
			writeCommandFunctions(f, cfg, cmd.Commands, cp)

			// Parent dispatcher function.
			cfg.writeMarker(f, fmt.Sprintf("# :command.function %s\n", funcBase))
			dispatcher := cfg.generateSubDispatcher(cp, cmd)
			write(f, fmt.Sprintf("%s_command() {\n%s\n}\n\n", funcBase, dispatcher))
		} else {
			// Leaf command — generate flag/arg parsers and body.
			// Temporarily rewrite cmd.Name so parser function names use funcBase.
			cmdCopy := cmd
			cmdCopy.Name = funcBase
			parser := cfg.generateParser(cmdCopy, false, cfg.funcAppName()+"_"+funcBase+"_usage", cfg.Name+" "+cp.disp)

			// Filename override: read body from custom path instead of default
			srcpath := cfg.commandPath(cp.raw)
			if cmd.Filename != "" {
				srcpath = cmd.Filename
			}
			rawBody, readErr := os.ReadFile(srcpath)
			var body string
			if readErr != nil {
				// Missing body: use bashly's scaffold as the inline body and,
				// for the default path, persist it as the body file.
				scaffold := cfg.bodyScaffold(
					srcpath,
					cfg.Name+" "+cp.disp,
					cfg.funcAppName()+"_"+funcBase+"_command",
				)
				body = strings.TrimRight(scaffold, "\n")
				if cmd.Filename == "" {
					_ = cfg.writeCommand(cfg.commandPath(cp.raw), scaffold)
				}
			} else {
				body = strings.TrimRight(string(rawBody), "\n")
				body = strings.ReplaceAll(body, "%APP_NAME%", cfg.Name)
			}

			// strip leading comment/blank lines from body
			bodyLines := strings.Split(body, "\n")
			startIdx := 0
			for startIdx < len(bodyLines) {
				trimmed := strings.TrimSpace(bodyLines[startIdx])
				if strings.HasPrefix(trimmed, "#") || trimmed == "" {
					startIdx++
				} else {
					break
				}
			}
			cleanBody := strings.Join(bodyLines[startIdx:], "\n")

			// inject single-pass parser call at top of body
			invokes := fmt.Sprintf("  %s_parse \"$@\"\n", funcBase)
			// inject filter calls
			for _, filter := range cmd.Filters {
				invokes += fmt.Sprintf("  %s\n", filter)
			}
			// inject command-scoped variables
			for _, v := range cmd.Variables {
				invokes += fmt.Sprintf("  %s=\"%s\"\n", v.Name, v.Value)
			}
			fullBody := fmt.Sprintf("  _inspect_cmd=%s\n", funcBase) + invokes + cfg.indentBody(cleanBody)

			// per-command env var + dep checks
			envCheckCmd := cfg.generateEnvCheck(cmd.EnvironmentVariables)
			depsCheckCmd := cfg.generateDepsCheck(cmd.Dependencies)
			preamble := ""
			// argfile: load default flags from a file.
			if cmd.Argfile.Enabled() {
				var argfilePath string
				if p := cmd.Argfile.Path(); p != "" {
					argfilePath = shellSingleQuote(p)
				} else {
					argfilePath = fmt.Sprintf("\"${HOME}/.config/%s/%s\"", cfg.Name, funcBase)
				}
				preamble += "  # argfile: load default flags from file\n"
				preamble += fmt.Sprintf("  _argfile=%s\n", argfilePath)
				preamble += "  if [ -f \"$_argfile\" ]; then\n"
				preamble += "    while IFS= read -r _argfile_line || [ -n \"$_argfile_line\" ]; do\n"
				preamble += "      _argfile_line=\"${_argfile_line%\"${_argfile_line##*[![:space:]]}\"}\"\n"
				preamble += "      _argfile_line=\"${_argfile_line#\"${_argfile_line%%[![:space:]]*}\"}\"\n"
				preamble += "      case \"$_argfile_line\" in\n"
				preamble += "        ''|'#'*) continue ;;\n"
				preamble += "      esac\n"
				preamble += "      _argfile_key=\"${_argfile_line%%[[:space:]]*}\"\n"
				preamble += "      _argfile_value=\"${_argfile_line#\"$_argfile_key\"}\"\n"
				preamble += "      _argfile_value=\"${_argfile_value#\"${_argfile_value%%[![:space:]]*}\"}\"\n"
				preamble += "      _argfile_value=\"${_argfile_value%\"${_argfile_value##*[![:space:]]}\"}\"\n"
				preamble += "      case \"$_argfile_value\" in\n"
				preamble += "        \\\"*\\\") _argfile_value=\"${_argfile_value#\\\"}\"; _argfile_value=\"${_argfile_value%\\\"}\" ;;\n"
				preamble += "        \\'*\\') _argfile_value=\"${_argfile_value#\\'}\"; _argfile_value=\"${_argfile_value%\\'}\" ;;\n"
				preamble += "      esac\n"
				preamble += "      case \"$_argfile_key\" in\n"
				preamble += "        -*) ;;\n"
				preamble += "        *) continue ;;\n"
				preamble += "      esac\n"
				if len(cmd.Flags) > 0 {
					preamble += "      case \"$_argfile_key\" in\n"
					for _, fl := range cmd.Flags {
						varName := flagVarName(fl)
						var assign string
						switch {
						case fl.Arg != "" && fl.Repeatable:
							assign = fmt.Sprintf("%s=\"${%s} $_argfile_value\"", varName, varName)
						case fl.Arg != "":
							assign = fmt.Sprintf("%s=\"$_argfile_value\"", varName)
						case fl.Repeatable:
							assign = fmt.Sprintf("%s=$((${%s:-0}+1))", varName, varName)
						default:
							assign = fmt.Sprintf("%s=1", varName)
						}
						preamble += fmt.Sprintf("        %s) %s ;;\n", strings.Join(flagSpellings(fl), "|"), assign)
					}
					preamble += "      esac\n"
				}
				preamble += "    done < \"$_argfile\"\n"
				preamble += "    unset _argfile_key _argfile_value _argfile_line\n"
				preamble += "  fi\n"
			}
			if envCheckCmd != "" || depsCheckCmd != "" {
				if envCheckCmd != "" {
					preamble += "  # environment variable checks\n" + envCheckCmd
				}
				if depsCheckCmd != "" {
					preamble += "  # dependency checks\n" + depsCheckCmd
				}
			}
			if preamble != "" {
				fullBody = preamble + "\n" + fullBody
			}

			cfg.writeMarker(f, fmt.Sprintf("# :command.function %s\n", funcBase))
			write(f, parser+"\n")
			write(f, fmt.Sprintf("%s_command() {\n%s\n}\n\n", funcBase, fullBody))
		}
	}
}

// generateNestedUsage builds a usage function for a parent command listing its sub-commands.
func (cfg *ShellyCfg) generateNestedUsage(cp cmdPath, cmd Command) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_usage() {\n", cp.funcBase()))
	cfg.writeUsageBody(&b, usageNode{
		disp:               cfg.Name + " " + cp.disp,
		help:               cmd.Help,
		helpHeaderOverride: cmd.HelpHeaderOverride,
		aliases:            cmd.allAliases(),
		args:               cmd.Args,
		catchAll:           cmd.CatchAll,
		flags:              cmd.Flags,
		envs:               cmd.EnvironmentVariables,
		examples:           cmd.Examples,
		isRoot:             false,
		hasSubs:            len(cmd.Commands) > 0,
		subs:               cmd.Commands,
		footer:             cmd.Footer,
	})
	b.WriteString("}\n")
	return b.String()
}

// generateSubDispatcher builds the body of a parent command function that
// dispatches to child command functions.
func (cfg *ShellyCfg) generateSubDispatcher(cp cmdPath, cmd Command) string {
	funcBase := cp.funcBase()
	var b strings.Builder
	// help is only honoured while it leads (before the subcommand word).
	b.WriteString("  if [ \"${1:-}\" = \"-h\" ] || [ \"${1:-}\" = \"--help\" ]; then\n")
	b.WriteString("    long_usage=yes\n")
	b.WriteString(fmt.Sprintf("    %s_usage\n    return 0\n", funcBase))
	b.WriteString("  fi\n")
	b.WriteString("  if [ $# -eq 0 ]; then\n")
	if cmd.Expose.IsAlways() {
		// expose:always — list subcommands inline, then show usage, then exit
		b.WriteString(fmt.Sprintf("    echo \"%s %s - %s\"\n", cfg.Name, cp.disp, cmd.Help))
		b.WriteString("    echo \"\"\n")
		cfg.writeCommandGrouping(&b, cmd.Commands, "    ")
		b.WriteString(fmt.Sprintf("    %s_usage >&2\n    exit 1\n", funcBase))
	} else {
		b.WriteString(fmt.Sprintf("    %s_usage >&2\n    exit 1\n", funcBase))
	}
	b.WriteString("  fi\n")
	b.WriteString("  _subcmd=\"$1\"; shift\n")
	b.WriteString("  case \"$_subcmd\" in\n")
	writeDispatchArms(&b, cmd.Commands, func(c Command) string {
		return cp.child(c).funcBase() + "_command"
	})
	b.WriteString(fmt.Sprintf("    -*) echo \"%s\" >&2; exit 1 ;;\n", cfg.m("invalid_flag", nil, "$_subcmd")))
	b.WriteString(fmt.Sprintf("    *) echo \"%s\" >&2; exit 1 ;;\n", cfg.m("invalid_subcommand", nil, "$_subcmd")))
	b.WriteString("  esac")
	return b.String()
}

// generateInspectArgs builds the default inspect_args() function, matching
// bashly's output. It prints to stdout:
//
//	args: none                     (or a sorted key/value list)
//	other_args: ...                (only when catch_all values were collected)
//	deps: ...                      (only when dependencies were resolved)
//	environment variables: ...     (only when the command declares any)
//
// Shelly targets POSIX sh (no associative arrays), so each command records its
// key/value pairs via a per-command collector function. Pairs are tab-separated
// and sorted at runtime with `sort` so ordering matches bashly's locale-
// collated `sort`. The command that ran is identified by `_inspect_cmd`, which
// every command body sets to its sanitized funcBase.
func (cfg *ShellyCfg) generateInspectArgs() string {
	var b strings.Builder

	// ── generic section printers ────────────────────────────────────────────
	b.WriteString("_inspect_sort() {\n")
	b.WriteString("  sort -t \"$(printf '\\t')\" -k1,1\n")
	b.WriteString("}\n\n")

	b.WriteString("_inspect_args_section() {\n")
	b.WriteString("  _is_head=\"\"\n")
	b.WriteString("  while IFS=\"$(printf '\\t')\" read -r _is_k _is_v; do\n")
	b.WriteString("    if [ -z \"$_is_head\" ]; then\n")
	b.WriteString("      printf '%s\\n' \"args:\"\n")
	b.WriteString("      _is_head=1\n")
	b.WriteString("    fi\n")
	b.WriteString("    printf -- '- ${args[%s]} = %s\\n' \"$_is_k\" \"$_is_v\"\n")
	b.WriteString("  done\n")
	b.WriteString("  if [ -z \"$_is_head\" ]; then\n")
	b.WriteString("    printf '%s\\n' \"args: none\"\n")
	b.WriteString("  fi\n")
	b.WriteString("}\n\n")

	b.WriteString("_inspect_deps_section() {\n")
	b.WriteString("  _id_head=\"\"\n")
	b.WriteString("  while IFS=\"$(printf '\\t')\" read -r _id_k _id_v; do\n")
	b.WriteString("    if [ -z \"$_id_head\" ]; then\n")
	b.WriteString("      printf '%s\\n' \"\" \"deps:\"\n")
	b.WriteString("      _id_head=1\n")
	b.WriteString("    fi\n")
	b.WriteString("    printf -- '- ${deps[%s]} = %s\\n' \"$_id_k\" \"$_id_v\"\n")
	b.WriteString("  done\n")
	b.WriteString("}\n\n")

	// Union of every environment variable name anywhere in the config, so the
	// value lookup can be generated as a POSIX case statement (no eval).
	var allEnvs []string
	addEnvName := func(name string) {
		for _, x := range allEnvs {
			if x == name {
				return
			}
		}
		allEnvs = append(allEnvs, name)
	}
	for _, e := range cfg.EnvironmentVariables {
		addEnvName(e.Name)
	}
	var collectEnvs func(cmds []Command)
	collectEnvs = func(cmds []Command) {
		for _, cmd := range cmds {
			for _, e := range cmd.EnvironmentVariables {
				addEnvName(e.Name)
			}
			collectEnvs(cmd.Commands)
		}
	}
	collectEnvs(cfg.Commands)

	b.WriteString("_inspect_env_section() {\n")
	b.WriteString("  [ \"$#\" -gt 0 ] || return 0\n")
	b.WriteString("  printf '%s\\n' \"\" \"environment variables:\"\n")
	b.WriteString("  printf '%s\\n' \"$@\" | sort | while IFS= read -r _ie_n; do\n")
	b.WriteString("    _inspect_env_value \"$_ie_n\"\n")
	b.WriteString("    printf -- '- $%s = %s\\n' \"$_ie_n\" \"$_ie_v\"\n")
	b.WriteString("  done\n")
	b.WriteString("}\n\n")

	b.WriteString("_inspect_env_value() {\n")
	b.WriteString("  case \"$1\" in\n")
	for _, name := range allEnvs {
		b.WriteString(fmt.Sprintf("    %s) _ie_v=\"${%s:-}\" ;;\n", name, name))
	}
	b.WriteString("    *) _ie_v=\"\" ;;\n")
	b.WriteString("  esac\n")
	b.WriteString("}\n\n")

	b.WriteString("_inspect_other_section() {\n")
	b.WriteString("  printf '%s\\n' \"\" \"other_args:\"\n")
	b.WriteString("  printf -- '- ${other_args[*]} = %s\\n' \"$1\"\n")
	b.WriteString("  _io_i=0\n")
	b.WriteString("  for _io_a in $1; do\n")
	b.WriteString("    printf -- '- ${other_args[%s]} = %s\\n' \"$_io_i\" \"$_io_a\"\n")
	b.WriteString("    _io_i=$((_io_i + 1))\n")
	b.WriteString("  done\n")
	b.WriteString("}\n\n")

	// ── per-node collectors ─────────────────────────────────────────────────
	type node struct {
		fb       string
		envs     []string
		deps     []string
		hasOther bool
		otherVar string
	}
	var nodes []node

	writeCollector := func(fb string, flags []Flag, args []Arg, deps []string) {
		var cb strings.Builder
		for _, f := range flags {
			key := f.Long
			if key == "" {
				key = f.Short
			}
			if key == "" {
				continue
			}
			v := flagVarName(f)
			cb.WriteString(fmt.Sprintf("  if [ -n \"${%s+x}\" ]; then printf '%%s\\t%%s\\n' '%s' \"$%s\"; fi\n", v, key, v))
		}
		for _, a := range args {
			v := sanitizeVarName(a.Name)
			cb.WriteString(fmt.Sprintf("  if [ -n \"${%s+x}\" ]; then printf '%%s\\t%%s\\n' '%s' \"$%s\"; fi\n", v, a.Name, v))
		}
		if cb.Len() == 0 {
			// dash rejects an empty function body.
			cb.WriteString("  :\n")
		}
		b.WriteString(fmt.Sprintf("_inspect_collect_%s() {\n", fb))
		b.WriteString(cb.String())
		b.WriteString("}\n\n")
		if len(deps) > 0 {
			b.WriteString(fmt.Sprintf("_inspect_collect_%s_deps() {\n", fb))
			for _, d := range deps {
				v := "deps_" + depsVarName(d)
				b.WriteString(fmt.Sprintf("  if [ -n \"${%s+x}\" ]; then printf '%%s\\t%%s\\n' '%s' \"$%s\"; fi\n", v, d, v))
			}
			b.WriteString("}\n\n")
		}
	}

	envNames := func(evs []EnvironmentVariable) []string {
		var names []string
		for _, e := range evs {
			names = append(names, e.Name)
		}
		return names
	}
	depNames := func(deps []Dependency) []string {
		var names []string
		for _, d := range deps {
			names = append(names, d.depNames()...)
		}
		return names
	}

	type ctxT struct {
		flags []Flag
		args  []Arg
		envs  []string
		deps  []string
	}
	rootCtx := ctxT{
		flags: cfg.Flags,
		args:  cfg.Args,
		envs:  envNames(cfg.EnvironmentVariables),
		deps:  depNames(cfg.Dependencies),
	}

	merge := func(p ctxT, cmd Command) ctxT {
		c := p
		for _, f := range cmd.Flags {
			found := false
			for _, x := range c.flags {
				if flagVarName(x) == flagVarName(f) {
					found = true
					break
				}
			}
			if !found {
				c.flags = append(c.flags, f)
			}
		}
		for _, a := range cmd.Args {
			found := false
			for _, x := range c.args {
				if sanitizeVarName(x.Name) == sanitizeVarName(a.Name) {
					found = true
					break
				}
			}
			if !found {
				c.args = append(c.args, a)
			}
		}
		addUnique := func(dst []string, add ...string) []string {
			for _, s := range add {
				found := false
				for _, x := range dst {
					if x == s {
						found = true
						break
					}
				}
				if !found {
					dst = append(dst, s)
				}
			}
			return dst
		}
		c.envs = addUnique(c.envs, envNames(cmd.EnvironmentVariables)...)
		c.deps = addUnique(c.deps, depNames(cmd.Dependencies)...)
		return c
	}

	if len(cfg.Commands) == 0 {
		// root-only mode: `_inspect_cmd=root` is set by root_command().
		writeCollector("root", rootCtx.flags, rootCtx.args, rootCtx.deps)
	}

	var walk func(parent cmdPath, pctx ctxT, cmds []Command)
	walk = func(parent cmdPath, pctx ctxT, cmds []Command) {
		for _, cmd := range cmds {
			cp := parent.child(cmd)
			fb := cp.funcBase()
			ctx := merge(pctx, cmd)
			writeCollector(fb, ctx.flags, ctx.args, ctx.deps)
			otherVar := ""
			if cmd.CatchAll != nil {
				otherVar = cfg.otherArgsVar()
			}
			nodes = append(nodes, node{
				fb:       fb,
				envs:     ctx.envs,
				deps:     ctx.deps,
				hasOther: cmd.CatchAll != nil,
				otherVar: otherVar,
			})
			walk(cp, ctx, cmd.Commands)
		}
	}
	walk(cmdPath{}, rootCtx, cfg.Commands)

	// ── dispatch ────────────────────────────────────────────────────────────
	b.WriteString("inspect_args() {\n")
	b.WriteString("  case \"${_inspect_cmd:-}\" in\n")
	for _, n := range nodes {
		b.WriteString(fmt.Sprintf("    %s)\n", n.fb))
		b.WriteString(fmt.Sprintf("      _inspect_collect_%s | _inspect_sort | _inspect_args_section\n", n.fb))
		if n.hasOther {
			b.WriteString(fmt.Sprintf("      if [ -n \"${%s}\" ]; then _inspect_other_section \"${%s}\"; fi\n", n.otherVar, n.otherVar))
		}
		if len(n.deps) > 0 {
			b.WriteString(fmt.Sprintf("      _inspect_collect_%s_deps | _inspect_sort | _inspect_deps_section\n", n.fb))
		}
		if len(n.envs) > 0 {
			parts := make([]string, len(n.envs))
			for i, e := range n.envs {
				parts[i] = "\"" + e + "\""
			}
			b.WriteString(fmt.Sprintf("      _inspect_env_section %s\n", strings.Join(parts, " ")))
		}
		b.WriteString("      ;;\n")
	}
	if len(cfg.Commands) == 0 {
		b.WriteString("    root)\n")
		b.WriteString("      _inspect_collect_root | _inspect_sort | _inspect_args_section\n")
		if len(rootCtx.deps) > 0 {
			b.WriteString("      _inspect_collect_root_deps | _inspect_sort | _inspect_deps_section\n")
		}
		if len(rootCtx.envs) > 0 {
			parts := make([]string, len(rootCtx.envs))
			for i, e := range rootCtx.envs {
				parts[i] = "\"" + e + "\""
			}
			b.WriteString(fmt.Sprintf("      _inspect_env_section %s\n", strings.Join(parts, " ")))
		}
		b.WriteString("      ;;\n")
	}
	b.WriteString("    *)\n")
	b.WriteString("      printf '%s\\n' \"args: none\"\n")
	b.WriteString("      ;;\n")
	b.WriteString("  esac\n")
	b.WriteString("}\n")
	return b.String()
}

func (cfg *ShellyCfg) shellGen() error {
	outPath := cfg.outputPath()
	if err := os.MkdirAll(cfg.targetDir(), 0o755); err != nil {
		return fmt.Errorf("error creating target directory: %w", err)
	}
	var permissions os.FileMode = 0o755
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
	if err != nil {
		return fmt.Errorf("error creating shell script file: %w", err)
	}
	defer f.Close()
	if err := cfg.shellGenToWriter(f); err != nil {
		return err
	}
	fmt.Printf("created %s\n", outPath)
	fmt.Printf("run %s --help to test your script\n", outPath)
	return nil
}

// shellGenToWriter writes the complete generated script to f, applying the
// configured formatter and then the tab_indent transform to the fully
// assembled output.
func (cfg *ShellyCfg) shellGenToWriter(f *os.File) error {
	var buf strings.Builder
	if err := cfg.shellGenBody(&buf); err != nil {
		return err
	}
	out, err := cfg.applyFormatter(stripDesiredComments(buf.String()))
	if err != nil {
		return err
	}
	if cfg.TabIndent {
		out = expandTabs(out)
	}
	if _, err := f.WriteString(out); err != nil {
		return err
	}
	return nil
}

// shellGenBody writes the complete generated script (2-space indented) to f.
// shellGenToWriter wraps it to apply the optional tab-indent transform.
func (cfg *ShellyCfg) shellGenBody(f io.Writer) error {
	// production mode: disable view markers globally
	if cfg.isProduction() {
		cfg.DisableViewMarkers = true
	}

	// load message-string overrides (src/shelly-strings.yml)
	if err := cfg.loadStrings(); err != nil {
		return err
	}

	// Discover lib files across lib_dir + extra_lib_dirs
	var libFiles []string
	for _, dir := range cfg.libDirs() {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.sh"))
		if len(matches) == 0 {
			matches, _ = filepath.Glob(filepath.Join(dir, "*"))
		}
		libFiles = append(libFiles, matches...)
	}
	sort.Strings(libFiles)
	debugf("debug: lib files: %v\n", libFiles)

	// Build lib units. A file that defines shell functions is emitted verbatim
	// (so multi-function libraries work); a body-only file is wrapped as
	// <stem>() { ... }. Duplicate function names across units are fatal.
	type verbatimLib struct{ origin, body string }
	var verbatimLibs []verbatimLib
	funcBodies := map[string]string{}
	funcOrigins := map[string]string{}
	seenFuncs := map[string]string{}
	duplicates := map[string][]string{}
	record := func(fn, origin string) {
		if prev, ok := seenFuncs[fn]; ok {
			duplicates[fn] = append(duplicates[fn], prev, origin)
			return
		}
		seenFuncs[fn] = origin
	}
	for _, p := range libFiles {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(b)
		if defs := libFnDefRe.FindAllStringSubmatch(content, -1); len(defs) > 0 {
			verbatimLibs = append(verbatimLibs, verbatimLib{origin: p, body: content})
			for _, m := range defs {
				record(m[1], p)
			}
			continue
		}
		name := stemName(p)
		if _, exists := funcBodies[name]; !exists {
			funcBodies[name] = content
			funcOrigins[name] = p
		}
		record(name, p)
	}
	if len(duplicates) > 0 {
		var sb strings.Builder
		sb.WriteString("duplicate library function names detected:\n")
		for name, files := range duplicates {
			sb.WriteString(fmt.Sprintf("  %s defined in:\n", name))
			for _, file := range files {
				sb.WriteString(fmt.Sprintf("    - %s\n", file))
			}
		}
		fmt.Print(sb.String())
		return fmt.Errorf("%s", sb.String())
	}
	debugf("debug: lib functions: %v\n", funcOrigins)

	// Topological sort of lib functions (callee before caller)
	funcNames := make([]string, 0, len(funcBodies))
	for n := range funcBodies {
		funcNames = append(funcNames, n)
	}
	sort.Strings(funcNames)
	edges := map[string][]string{}
	for _, name := range funcNames {
		body := funcBodies[name]
		for _, callee := range funcNames {
			if callee != name && strings.Contains(body, callee) {
				edges[name] = append(edges[name], callee)
			}
		}
	}
	visited := map[string]int{}
	order := []string{}
	var cycle []string
	var dfs func(string) bool
	dfs = func(u string) bool {
		visited[u] = 1
		for _, v := range edges[u] {
			if visited[v] == 0 {
				if dfs(v) {
					return true
				}
			} else if visited[v] == 1 {
				cycle = append(cycle, v)
				return true
			}
		}
		visited[u] = 2
		order = append(order, u)
		return false
	}
	for _, n := range funcNames {
		if visited[n] == 0 {
			if dfs(n) {
				break
			}
		}
	}
	if len(cycle) > 0 {
		var sb strings.Builder
		sb.WriteString("detected cycle in library function calls:\n")
		for _, fn := range cycle {
			sb.WriteString(fmt.Sprintf("  %s ->\n", fn))
		}
		return fmt.Errorf("%s", sb.String())
	}

	// --- Write output ---
	// shebang
	write(f, cfg.shebang()+"\n\n")

	// header — optional user-supplied content injected right after shebang
	headerSrc := cfg.readSrcOrDefault("header.sh", "")
	if strings.TrimSpace(headerSrc) != "" {
		cfg.writeMarker(f, "# :command.header\n")
		write(f, strings.TrimRight(headerSrc, "\n")+"\n\n")
	}

	// preamble — raw content injected before any function definitions
	preambleSrc := cfg.readSrcOrDefault("preamble.sh", "")
	if strings.TrimSpace(preambleSrc) != "" {
		preambleSrc = strings.ReplaceAll(preambleSrc, "%APP_NAME%", cfg.Name)
		preambleSrc = strings.ReplaceAll(preambleSrc, "%VERSION%", cfg.Version)
		cfg.writeMarker(f, "# :command.preamble\n")
		write(f, strings.TrimRight(preambleSrc, "\n")+"\n\n")
	}

	// version_command
	versionDef := "version_command() {\n  echo \"$version\"\n}\n"
	cfg.writeMarker(f, "# :command.version_command\n")
	write(f, cfg.readSrcOrDefault("version_command.sh", versionDef)+"\n\n")

	// deps_get — look up a resolved dependency path by name
	if helper := cfg.generateDepsHelper(cfg.collectDependencies()); helper != "" {
		cfg.writeMarker(f, "# :command.deps_get\n")
		write(f, helper+"\n")
	}

	// usage — auto-generated from YAML unless user provides src/usage.sh
	usageSrc := cfg.readSrcOrDefault("usage.sh", "")
	var usageContent string
	if strings.TrimSpace(usageSrc) != "" {
		usageContent = strings.ReplaceAll(usageSrc, "%APP_NAME%", cfg.Name)
	} else {
		usageContent = generateRootUsage(cfg)
	}
	cfg.writeMarker(f, "# :command.usage\n")
	write(f, usageContent+"\n\n")

	// per-command usage functions (always auto-generated unless user file present).
	// Walk the whole command tree so nested commands also get a usage function —
	// every parser references <app>_<funcBase>_usage, so every one must be defined.
	var writeCommandUsages func(parent cmdPath, cmds []Command)
	writeCommandUsages = func(parent cmdPath, cmds []Command) {
		for _, cmd := range cmds {
			if cmd.Private {
				continue
			}
			cp := parent.child(cmd)
			cmdUsageSrc := cfg.readSrcOrDefault(fmt.Sprintf("%s_usage.sh", cp.raw), "")
			var cmdUsage string
			if strings.TrimSpace(cmdUsageSrc) != "" {
				cmdUsage = strings.ReplaceAll(cmdUsageSrc, "%APP_NAME%", cfg.Name)
			} else {
				cmdUsage = cfg.generateCommandUsage(cp, cmd)
			}
			cfg.writeMarker(f, fmt.Sprintf("# :command.usage.%s\n", cp.raw))
			write(f, cmdUsage+"\n\n")
			if len(cmd.Commands) > 0 {
				writeCommandUsages(cp, cmd.Commands)
			}
		}
	}
	writeCommandUsages(cmdPath{}, cfg.Commands)

	// normalize_input
	// normalize_input: input normalization (--flag=value, -a=arg, -abc) is now
	// handled by the command parser, so the default is a no-op. The hook is
	// retained for user overrides (src/normalize_input.sh).
	normalizeDef := "normalize_input() {\n" +
		"  # Input normalization is performed by the command parser.\n" +
		"  # This hook is retained for overrides (src/normalize_input.sh).\n" +
		"  :\n" +
		"}\n"
	normalize := strings.ReplaceAll(cfg.readSrcOrDefault("normalize_input.sh", normalizeDef), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.normalize_input\n")
	write(f, normalize+"\n\n")

	// inspect_args: emit dev utility in dev mode; skip in production
	if !cfg.isProduction() {
		inspectDef := cfg.generateInspectArgs()
		inspect := strings.ReplaceAll(cfg.readSrcOrDefault("inspect_args.sh", inspectDef), "%APP_NAME%", cfg.Name)
		cfg.writeMarker(f, "# :command.inspect_args\n")
		write(f, inspect+"\n\n")
	}

	// parse_requirements stub (overridable)
	parseReqDef := "parse_requirements() {\n  # hook: check env/deps - override in src/parse_requirements.sh\n  return 0\n}\n"
	parseReq := strings.ReplaceAll(cfg.readSrcOrDefault("parse_requirements.sh", parseReqDef), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.parse_requirements\n")
	write(f, parseReq+"\n\n")

	// command_functions shared helpers
	cmdFuncsDef := "# shared helper functions\n"
	cmdFuncs := strings.ReplaceAll(cfg.readSrcOrDefault("command_functions.sh", cmdFuncsDef), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.command_functions\n")
	write(f, cmdFuncs+"\n\n")

	// lib files in topological order
	if len(order) > 0 {
		cfg.writeMarker(f, "# :command.lib_files\n")
		for _, name := range order {
			body := strings.TrimRight(funcBodies[name], "\n")
			origin := funcOrigins[name]
			write(f, fmt.Sprintf("# :lib %s\n", origin))
			write(f, fmt.Sprintf("%s() {\n%s\n}\n\n", name, cfg.indentBody(body)))
		}
	}

	// verbatim library files (multi-function libraries, emitted as-is)
	if len(verbatimLibs) > 0 {
		cfg.writeMarker(f, "# :command.lib_files\n")
		for _, lib := range verbatimLibs {
			write(f, fmt.Sprintf("# :lib %s\n", lib.origin))
			write(f, strings.TrimRight(lib.body, "\n")+"\n\n")
		}
	}

	// per-command functions
	if len(cfg.Commands) == 0 {
		// root (no-subcommand) mode — synthesize a Command to reuse parsers
		rootCmd := Command{
			Name:     "root",
			Args:     cfg.Args,
			Flags:    cfg.Flags,
			CatchAll: nil,
		}
		// single unified parser covering root args and flags
		if len(cfg.Flags) > 0 || len(cfg.Args) > 0 {
			rp := cfg.generateParser(rootCmd, false, cfg.funcAppName()+"_usage", cfg.Name)
			cfg.writeMarker(f, "# :command.root.parse\n")
			write(f, rp+"\n")
		}

		srcpath := filepath.Join(cfg.sourceDir(), "root_command.sh")
		rawBody, readErr := os.ReadFile(srcpath)
		var rootBodyContent string
		if readErr != nil {
			scaffold := cfg.bodyScaffold(srcpath, cfg.Name, "root_command")
			rootBodyContent = cfg.indentBody(strings.TrimRight(scaffold, "\n"))
			_ = cfg.writeCommand(srcpath, scaffold)
		} else {
			rootBodyContent = cfg.indentBody(strings.TrimRight(string(rawBody), "\n"))
		}

		invokes := ""
		if len(cfg.Flags) > 0 || len(cfg.Args) > 0 {
			invokes += "  root_parse \"$@\"\n"
		}
		if invokes == "" {
			invokes = fmt.Sprintf("  %s_usage\n  return 0\n", cfg.funcAppName())
		}
		cfg.writeMarker(f, "# :command.root\n")
		write(f, fmt.Sprintf("root_command() {\n  _inspect_cmd=root\n%s%s\n}\n\n", invokes, rootBodyContent))
	} else {
		writeCommandFunctions(f, cfg, cfg.Commands, cmdPath{})
	}

	// before_hook — wraps src/before.sh body in before_hook() function (if present)
	beforeSrc := cfg.readSrcOrDefault("before.sh", "")
	hasBeforeHook := strings.TrimSpace(beforeSrc) != ""
	if hasBeforeHook {
		beforeSrc = strings.ReplaceAll(beforeSrc, "%APP_NAME%", cfg.Name)
		var beforeBody string
		for _, line := range strings.Split(strings.TrimRight(beforeSrc, "\n"), "\n") {
			beforeBody += "  " + line + "\n"
		}
		cfg.writeMarker(f, "# :command.before_hook\n")
		write(f, "before_hook() {\n"+beforeBody+"}\n\n")
	}

	// after_hook — wraps src/after.sh body in after_hook() function (if present)
	afterSrc := cfg.readSrcOrDefault("after.sh", "")
	hasAfterHook := strings.TrimSpace(afterSrc) != ""
	if hasAfterHook {
		afterSrc = strings.ReplaceAll(afterSrc, "%APP_NAME%", cfg.Name)
		var afterBody string
		for _, line := range strings.Split(strings.TrimRight(afterSrc, "\n"), "\n") {
			afterBody += "  " + line + "\n"
		}
		cfg.writeMarker(f, "# :command.after_hook\n")
		write(f, "after_hook() {\n"+afterBody+"}\n\n")
	}

	// initialize — sets version, runs global env checks
	// Wraps src/initialize.sh body (or auto-generated default) in initialize() function.
	envCheck := cfg.generateEnvCheck(cfg.EnvironmentVariables)
	depsCheck := cfg.generateDepsCheck(cfg.Dependencies)
	setLine := ""
	if d := cfg.Strict.Directive(); d != "" {
		for _, line := range strings.Split(d, "\n") {
			setLine += "  " + line + "\n"
		}
	}
	initBody := fmt.Sprintf("  version=\"%s\"\n%s", cfg.Version, setLine)
	// inject global variables
	if len(cfg.Variables) > 0 {
		for _, v := range cfg.Variables {
			initBody += fmt.Sprintf("  %s=\"%s\"\n", v.Name, v.Value)
		}
	}
	if envCheck != "" {
		initBody += "\n  # environment variable checks\n" + envCheck
	}
	if depsCheck != "" {
		initBody += "\n  # dependency checks\n" + depsCheck
	}
	// allow user override (body-only; wrapped in function)
	initSrc := cfg.readSrcOrDefault("initialize.sh", "")
	if strings.TrimSpace(initSrc) != "" {
		initSrc = strings.ReplaceAll(initSrc, "%APP_NAME%", cfg.Name)
		initSrc = strings.ReplaceAll(initSrc, "%VERSION%", cfg.Version)
		initBody = ""
		for _, line := range strings.Split(strings.TrimRight(initSrc, "\n"), "\n") {
			initBody += "  " + line + "\n"
		}
	}
	initContent := cfg.initFuncName() + "() {\n" + initBody
	if !strings.HasSuffix(initContent, "\n") {
		initContent += "\n"
	}
	initContent += "}\n"
	cfg.writeMarker(f, "# :command.initialize\n")
	write(f, initContent+"\n\n")

	// run dispatcher
	var runBuilder strings.Builder
	runBuilder.WriteString(cfg.runFuncName() + "() {\n")
	runBuilder.WriteString("  long_usage=\"\"\n")
	runBuilder.WriteString("  normalize_input \"$@\"\n")
	// before hook — call before_hook() if src/before.sh was present
	if hasBeforeHook {
		runBuilder.WriteString("  before_hook\n")
	}

	// Peel leading fixed/global flags so dispatch sees the command word. This
	// mirrors bashly: --version and --help are honoured only while they lead,
	// root flags are parsed and shifted off, and any unknown option before the
	// command word is an "invalid option".
	runBuilder.WriteString("  while [ $# -gt 0 ]; do\n")
	runBuilder.WriteString("    case \"$1\" in\n")
	runBuilder.WriteString("      --version) version_command; return 0 ;;\n")
	runBuilder.WriteString(fmt.Sprintf("      -h|--help) long_usage=yes; %s_usage; return 0 ;;\n", cfg.funcAppName()))
	writeFlagArms(&runBuilder, Command{Name: cfg.funcAppName(), Flags: cfg.Flags}, cfg.otherArgsVar(), cfg.conjoinedFlagArgs())
	runBuilder.WriteString("      --) shift; break ;;\n")
	runBuilder.WriteString(fmt.Sprintf("      -*) echo \"%s\" >&2; exit 1 ;;\n", cfg.m("invalid_flag", nil, "$1")))
	runBuilder.WriteString("      *) break ;;\n")
	runBuilder.WriteString("    esac\n")
	runBuilder.WriteString("  done\n")

	// post-checks for root/global flags (required, defaults, allowed, ...).
	// Root-only mode performs these in root_parse instead.
	if len(cfg.Commands) > 0 {
		globalCmd := Command{Name: cfg.funcAppName(), Flags: cfg.Flags}
		cfg.writeRequiredChecks(&runBuilder, globalCmd, "")
		cfg.writeFlagPostChecks(&runBuilder, globalCmd)
	}

	// find default command (if any) and whether it forces on zero args
	defaultCmd := ""
	defaultFuncName := ""
	forceDefault := false
	for _, c := range cfg.Commands {
		isForce := c.DefaultCmd.IsForce()
		if c.DefaultCmd.IsDefault() || isForce {
			defaultCmd = c.Name
			defaultFuncName = cmdPath{}.child(c).funcBase()
			forceDefault = isForce
			break
		}
	}

	if len(cfg.Commands) == 0 {
		// root (no-subcommand) mode: hand the remaining positionals to root_command.
		if len(cfg.Args) == 0 && len(cfg.Flags) == 0 {
			runBuilder.WriteString(fmt.Sprintf("  %s_usage >&2\n", cfg.funcAppName()))
			runBuilder.WriteString("  exit 1\n")
		} else {
			runBuilder.WriteString("  if [ $# -eq 0 ]; then\n")
			runBuilder.WriteString(fmt.Sprintf("    %s_usage >&2\n    exit 1\n", cfg.funcAppName()))
			runBuilder.WriteString("  fi\n")
			runBuilder.WriteString("  root_command \"$@\"\n")
		}
	} else {
		if forceDefault && defaultCmd != "" {
			runBuilder.WriteString("  if [ $# -eq 0 ]; then\n")
			runBuilder.WriteString(fmt.Sprintf("    %s_command\n    return 0\n", defaultFuncName))
			runBuilder.WriteString("  fi\n")
		} else {
			runBuilder.WriteString("  if [ $# -eq 0 ]; then\n")
			runBuilder.WriteString(fmt.Sprintf("    %s_usage >&2\n    exit 1\n", cfg.funcAppName()))
			runBuilder.WriteString("  fi\n")
		}

		runBuilder.WriteString("  _cmd=\"$1\"; shift\n")
		runBuilder.WriteString("  case \"$_cmd\" in\n")

		writeDispatchArms(&runBuilder, cfg.Commands, func(c Command) string {
			return cmdPath{}.child(c).funcBase() + "_command"
		})
		if defaultCmd != "" {
			runBuilder.WriteString(fmt.Sprintf("    *) %s_command \"$_cmd\" \"$@\" ;;\n", defaultFuncName))
		} else if cfg.Extensible.IsBool() {
			// extensible: try <appname>-<cmd> in PATH before erroring
			runBuilder.WriteString("    *)\n")
			runBuilder.WriteString(fmt.Sprintf("      if command -v %s-\"$_cmd\" >/dev/null 2>&1; then\n", cfg.Name))
			runBuilder.WriteString(fmt.Sprintf("        %s-\"$_cmd\" \"$@\"\n", cfg.Name))
			runBuilder.WriteString("      else\n")
			runBuilder.WriteString(fmt.Sprintf("        echo \"%s\" >&2; exit 1\n", cfg.m("invalid_command", nil, "$_cmd")))
			runBuilder.WriteString("      fi ;;\n")
		} else if cfg.Extensible.Delegate() != "" {
			// extensible with delegate: forward unknown commands to the delegate tool
			runBuilder.WriteString("    *)\n")
			runBuilder.WriteString(fmt.Sprintf("      %s \"$_cmd\" \"$@\" ;;\n", cfg.Extensible.Delegate()))
		} else {
			runBuilder.WriteString(fmt.Sprintf("    *) echo \"%s\" >&2; exit 1 ;;\n", cfg.m("invalid_command", nil, "$_cmd")))
		}
		runBuilder.WriteString("  esac\n")
	}
	// after hook — call after_hook() if src/after.sh was present
	if hasAfterHook {
		runBuilder.WriteString("  after_hook\n")
	}
	runBuilder.WriteString("}\n")
	runDef := strings.ReplaceAll(cfg.readSrcOrDefault("run_wrapper.sh", runBuilder.String()), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.run\n")
	write(f, runDef+"\n\n")

	// start
	startDef := cfg.initFuncName() + "\n" + cfg.runFuncName() + " \"$@\"\n"
	start := strings.ReplaceAll(cfg.readSrcOrDefault("start.sh", startDef), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.start\n")
	write(f, start+"\n")
	return nil
}

func (cfg *ShellyCfg) shebang() string {
	if cfg.DisableHeaderComment {
		return "#!/bin/sh"
	}
	return "#!/bin/sh\n# This script was generated by shelly\n# Modifying it manually is not recommended"
}

// bodyScaffold returns the initial body-file content for a command, matching
// bashly's generated scaffold:
//
//	echo "# This file is located at '<filename>'."
//	echo "# It contains the implementation for the '<display>' command."
//	echo "# The code you write here will be wrapped by a function named '<funcName>()'."
//	echo "# Feel free to edit this file; your changes will persist when regenerating."
//	inspect_args
//
// The echo lines intentionally print at runtime (as in bashly) to remind the
// author where the body lives.
func (cfg *ShellyCfg) bodyScaffold(filename, display, funcName string) string {
	return fmt.Sprintf(
		"echo \"# This file is located at '%s'.\"\n"+
			"echo \"# It contains the implementation for the '%s' command.\"\n"+
			"echo \"# The code you write here will be wrapped by a function named '%s()'.\"\n"+
			"echo \"# Feel free to edit this file; your changes will persist when regenerating.\"\n"+
			"inspect_args\n",
		filename, display, funcName,
	)
}

// commandFunc creates a body-only skeleton for a command if not present.
func (cfg *ShellyCfg) commandFunc(command string) {
	filename := cfg.commandPath(command)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		display := cfg.Name + " " + command
		funcName := cfg.funcAppName() + "_" + sanitizeFuncName(command) + "_command"
		// Root (no-subcommand) mode: bashly names the function "root_command"
		// and shows only the app name.
		if command == "root" {
			display = cfg.Name
			funcName = "root_command"
		}
		_ = cfg.writeCommand(filename, cfg.bodyScaffold(filename, display, funcName))
	}
}

func (cfg *ShellyCfg) writeCommand(filename, content string) error {
	if dir := filepath.Dir(filename); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Println("Error creating directory:", err)
			return err
		}
	}
	f, err := os.Create(fmt.Sprintf("./%s", filename))
	if err != nil {
		fmt.Println("Error creating shell script file:", err)
		return err
	}
	defer f.Close()
	write(f, content)
	return nil
}

// runShellcheck runs shellcheck -s sh on the generated script.
func runShellcheck(path string, strict bool) error {
	shPath, err := exec.LookPath("shellcheck")
	if err != nil {
		if strict {
			return fmt.Errorf("shellcheck not found in PATH")
		}
		fmt.Println("warning: shellcheck not found in PATH, skipping shellcheck")
		return nil
	}
	cmd := exec.Command(shPath, "-s", "sh", path)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Printf("shellcheck output:\n%s\n", string(out))
	}
	if err != nil {
		if strict {
			return fmt.Errorf("shellcheck error: %w", err)
		}
		fmt.Println("shellcheck returned non-zero exit (warnings/errors) but strict not set")
	}
	return nil
}

// blankLineRe matches a run of whitespace ending in a newline; the internal
// formatter replaces it with a single blank line.
var blankLineRe = regexp.MustCompile(`[ \t\r\n\f]+\n`)

// libFnDefRe matches a shell function definition at the start of a line
// (capturing the function name). A library file containing one or more of
// these is emitted verbatim instead of being wrapped as a single function.
var libFnDefRe = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(\)`)

// desiredCommentLineRe matches a whole "desired command" comment line: any
// leading whitespace followed by "##". These build-time notes are stripped from
// the generated script, keeping ordinary single-"#" shell comments intact.
var desiredCommentLineRe = regexp.MustCompile(`(?m)^[ \t]*##[^\n]*\n?`)

// stripDesiredComments removes every "##"-prefixed comment line from the
// assembled script. It also removes the "## [@shelly-upgrade ...]" markers
// installed by `shelly add`, so they never leak into generated output.
func stripDesiredComments(s string) string {
	return desiredCommentLineRe.ReplaceAllString(s, "")
}

// applyFormatter post-processes the generated script according to
// cfg.Formatter:
//
//	""/"none"          no formatting
//	"internal"         squash runs of blank lines (the internal formatter)
//	"external"/"shfmt" run `shfmt --case-indent --indent 2` (stdin -> stdout)
//	anything else      custom command read from stdin, written to stdout
//
// A missing shfmt binary is a warning only (the script is left unchanged); a
// missing custom command is an error.
func (cfg *ShellyCfg) applyFormatter(script string) (string, error) {
	switch cfg.Formatter {
	case "", "none":
		return script, nil
	case "internal":
		return internalFormat(script), nil
	case "external", "shfmt":
		return runFormatterCommand(script, []string{"shfmt", "--case-indent", "--indent", "2"}, true)
	default:
		return runFormatterCommand(script, splitCommand(cfg.Formatter), false)
	}
}

// internalFormat squashes every run of blank lines to a single blank line.
func internalFormat(script string) string {
	return blankLineRe.ReplaceAllString(script, "\n\n")
}

// runFormatterCommand pipes script through the given command (args[0] plus
// args[1:]) via stdin and returns its stdout. When tolerant is true a missing
// binary produces a warning and the input is returned unchanged.
func runFormatterCommand(script string, args []string, tolerant bool) (string, error) {
	if len(args) == 0 {
		return script, nil
	}
	binPath, err := exec.LookPath(args[0])
	if err != nil {
		if tolerant {
			fmt.Printf("warning: %s not found in PATH, skipping formatter\n", args[0])
			return script, nil
		}
		return "", fmt.Errorf("formatter command not found: %s", args[0])
	}
	cmd := exec.Command(binPath, args[1:]...)
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("formatter %s failed: %w: %s", args[0], err, msg)
		}
		return "", fmt.Errorf("formatter %s failed: %w", args[0], err)
	}
	return stdout.String(), nil
}

// splitCommand splits a formatter command string into arguments, honouring
// single and double quotes (a small POSIX shellwords equivalent).
func splitCommand(s string) []string {
	var args []string
	var cur strings.Builder
	inWord := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		switch c {
		case ' ', '\t', '\n':
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		case '\'', '"':
			quote = c
			inWord = true
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		args = append(args, cur.String())
	}
	return args
}

// shellGenToString generates the script to a string without writing to the named output file.
func (cfg *ShellyCfg) shellGenToString() (string, error) {
	tmp, err := os.CreateTemp("", "shelly-preview-*.sh")
	if err != nil {
		return "", fmt.Errorf("shellGenToString: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := cfg.shellGenToWriter(tmp); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()

	b, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("shellGenToString: read back: %w", err)
	}
	return string(b), nil
}

// writeDispatchArms emits `case` arms dispatching a command word to a command
// function. Exact names and exact aliases are emitted first (they take
// priority), followed by prefix aliases (trailing '*') which match any word
// beginning with the prefix. cmdFunc returns the already-suffixed function
// name for a command (e.g. "foo_command").
func writeDispatchArms(b *strings.Builder, cmds []Command, cmdFunc func(Command) string) {
	for _, c := range cmds {
		fn := cmdFunc(c)
		b.WriteString(fmt.Sprintf("    %s) %s \"$@\" ;;\n", c.Name, fn))
		for _, a := range c.exactAliases() {
			b.WriteString(fmt.Sprintf("    %s) %s \"$@\" ;;\n", a, fn))
		}
	}
	for _, c := range cmds {
		fn := cmdFunc(c)
		for _, p := range c.prefixAliases() {
			b.WriteString(fmt.Sprintf("    %s*) %s \"$@\" ;;\n", p, fn))
		}
	}
}

// validateSemantics checks the config for semantic errors beyond YAML parsing.
// Returns a slice of human-readable error strings (empty = valid).
func (cfg *ShellyCfg) validateSemantics() []string {
	var errs []string

	// check for duplicate top-level command names
	seen := map[string]bool{}
	for _, c := range cfg.Commands {
		if seen[c.Name] {
			errs = append(errs, fmt.Sprintf("duplicate command name: %q", c.Name))
		}
		seen[c.Name] = true
	}

	// check negatable constraints for a set of flags owned by cmdName ("" = root)
	checkNegatable := func(cmdName string, flags []Flag) {
		loc := ""
		if cmdName != "" {
			loc = fmt.Sprintf("command %q: ", cmdName)
		}
		for _, f := range flags {
			if !f.Negatable {
				continue
			}
			if f.Long == "" {
				errs = append(errs, fmt.Sprintf("%sflag %q: negatable requires a long flag", loc, flagVarName(f)))
			}
			if f.Arg != "" {
				errs = append(errs, fmt.Sprintf("%sflag %q: negatable cannot be used with arg", loc, f.Long))
			}
			if f.Repeatable {
				errs = append(errs, fmt.Sprintf("%sflag %q: negatable cannot be used with repeatable", loc, f.Long))
			}
			if f.Required {
				errs = append(errs, fmt.Sprintf("%sflag %q: negatable cannot be used with required", loc, f.Long))
			}
		}
	}

	// check prefix-alias ambiguity among a group of sibling commands
	type prefixOwner struct{ prefix, cmd string }
	checkPrefix := func(cmds []Command) {
		var prefixes []prefixOwner
		reserved := map[string]string{}
		for i := range cmds {
			c := &cmds[i]
			reserved[c.Name] = c.Name
			for _, a := range c.exactAliases() {
				reserved[a] = c.Name
			}
			for _, p := range c.prefixAliases() {
				if p == "" {
					errs = append(errs, fmt.Sprintf("command %q: prefix alias %q must have a non-empty prefix", c.Name, "*"))
					continue
				}
				prefixes = append(prefixes, prefixOwner{p, c.Name})
			}
		}
		for _, p := range prefixes {
			if owner, ok := reserved[p.prefix]; ok {
				if owner == p.cmd {
					errs = append(errs, fmt.Sprintf("command %q: prefix alias %q* equals its own command name or alias", p.cmd, p.prefix))
				} else {
					errs = append(errs, fmt.Sprintf("command %q: prefix alias %q* shadows command %q", p.cmd, p.prefix, owner))
				}
			}
		}
		for i := 0; i < len(prefixes); i++ {
			for j := i + 1; j < len(prefixes); j++ {
				a, b := prefixes[i], prefixes[j]
				if a.cmd == b.cmd {
					continue
				}
				if strings.HasPrefix(a.prefix, b.prefix) || strings.HasPrefix(b.prefix, a.prefix) {
					errs = append(errs, fmt.Sprintf("ambiguous prefix aliases %q* (command %q) and %q* (command %q)", a.prefix, a.cmd, b.prefix, b.cmd))
				}
			}
		}
	}

	// recursively check needs/conflicts references and negatable constraints
	var walk func(c Command)
	walk = func(c Command) {
		flagSet := map[string]bool{}
		for _, f := range c.Flags {
			for _, s := range flagSpellings(f) {
				flagSet[s] = true
			}
		}
		for _, f := range c.Flags {
			for _, need := range f.Needs {
				if !flagSet[need] {
					errs = append(errs, fmt.Sprintf("command %q: flag %q needs undefined flag %q", c.Name, f.Long, need))
				}
			}
			for _, conflict := range f.Conflicts {
				if !flagSet[conflict] {
					errs = append(errs, fmt.Sprintf("command %q: flag %q conflicts with undefined flag %q", c.Name, f.Long, conflict))
				}
			}
		}
		checkNegatable(c.Name, c.Flags)
		checkPrefix(c.Commands)
		for _, sub := range c.Commands {
			walk(sub)
		}
	}
	for _, c := range cfg.Commands {
		walk(c)
	}
	checkNegatable("", cfg.Flags)
	checkPrefix(cfg.Commands)

	return errs
}
