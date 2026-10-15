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

// flagHelpName returns the display name for usage/help output, including aliases.
func flagHelpName(f Flag) string {
	long := f.Long
	if f.Negatable && strings.HasPrefix(f.Long, "--") {
		long = "--[no-]" + strings.TrimPrefix(f.Long, "--")
	}
	name := long
	if f.Short != "" {
		name = f.Short + ", " + long
	}
	if al := f.allAliases(); len(al) > 0 {
		if name == "" {
			name = strings.Join(al, ", ")
		} else {
			name += ", " + strings.Join(al, ", ")
		}
	}
	if f.Arg != "" {
		name += " <" + f.Arg + ">"
	}
	return name
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

func generateRootUsage(cfg *ShellyCfg) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_usage() {\n", cfg.Name))
	if cfg.HelpHeaderOverride != "" {
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.HelpHeaderOverride))
	} else {
		helpLines := strings.SplitN(cfg.Help, "\n", 2)
		b.WriteString(fmt.Sprintf("  echo \"%s - %s\"\n", cfg.Name, helpLines[0]))
		if len(helpLines) > 1 {
			for _, l := range strings.Split(helpLines[1], "\n") {
				b.WriteString(fmt.Sprintf("  echo \"%s\"\n", l))
			}
		}
	}
	b.WriteString("  echo \"\"\n")
	if len(cfg.Commands) > 0 {
		b.WriteString(fmt.Sprintf("  echo \"%s %s [command] [options]\"\n", cfg.m("usage", nil), cfg.Name))
		b.WriteString("  echo \"\"\n")

		// collect groups in order, preserving first-seen ordering
		groupOrder := []string{}
		groupMap := map[string][]Command{}
		privateCommands := []Command{}
		for _, c := range cfg.Commands {
			if c.Private {
				privateCommands = append(privateCommands, c)
				continue
			}
			g := c.Group
			if _, ok := groupMap[g]; !ok {
				groupOrder = append(groupOrder, g)
			}
			groupMap[g] = append(groupMap[g], c)
		}

		for _, g := range groupOrder {
			cmds := groupMap[g]
			if g != "" {
				b.WriteString(colorEcho("  ", cfg.UsageColors.Caption, cfg.m("group", map[string]string{"group": g})))
			} else {
				b.WriteString(colorEcho("  ", cfg.UsageColors.Caption, cfg.m("commands", nil)))
			}
			for _, c := range cmds {
				aliases := c.allAliases()
				aliasStr := ""
				if len(aliases) > 0 {
					aliasStr = " (" + strings.Join(aliases, ", ") + ")"
				}
				helpLine := strings.SplitN(c.Help, "\n", 2)[0]
				b.WriteString(fmt.Sprintf("  echo \"  %-20s %s%s\"\n", c.Name, helpLine, aliasStr))
				// if expose:true or expose:always, also list immediate subcommands in root help
				if c.Expose.IsEnabled() {
					for _, sub := range c.Commands {
						if !sub.Private {
							subHelp := strings.SplitN(sub.Help, "\n", 2)[0]
							b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", c.Name+" "+sub.Name, subHelp))
						}
					}
				}
			}
			b.WriteString("  echo \"\"\n")
		}

		// private commands: shown only when reveal env var is set
		if len(privateCommands) > 0 {
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", cfg.privateRevealEnvVar()))
			b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("private_commands", nil)))
			for _, c := range privateCommands {
				b.WriteString(fmt.Sprintf("    echo \"  %s\"\n", c.Name))
			}
			b.WriteString("    echo \"\"\n")
			b.WriteString("  fi\n")
		}
	} else {
		// root-level args/flags usage
		argStr := ""
		for _, a := range cfg.Args {
			if a.Required {
				argStr += " <" + a.Name + ">"
			} else {
				argStr += " [" + a.Name + "]"
			}
		}
		b.WriteString(fmt.Sprintf("  echo \"%s %s%s [options]\"\n", cfg.m("usage", nil), cfg.Name, argStr))
		b.WriteString("  echo \"\"\n")
		cfg.writeArgsHelp(&b, cfg.Args, cfg.UsageColors.Caption, cfg.wordWrap())
		cfg.writeFlagsHelp(&b, cfg.Flags, cfg.UsageColors.Caption, cfg.wordWrap(), cfg.privateRevealEnvVar())
	}
	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--help, -h", cfg.m("help_flag_text", nil)))
	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--version", cfg.m("version_flag_text", nil)))
	if len(cfg.Examples) > 0 {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.m("examples", nil)))
		for _, ex := range cfg.Examples {
			b.WriteString(fmt.Sprintf("  echo \"  %s\"\n", ex))
		}
	}
	if len(cfg.EnvironmentVariables) > 0 {
		cfg.writeEnvVarsHelp(&b, cfg.EnvironmentVariables, cfg.UsageColors.Caption, cfg.wordWrap(), cfg.privateRevealEnvVar())
	}
	if cfg.Footer != "" {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.Footer))
	}
	b.WriteString("}\n")
	return b.String()
}

func (cfg *ShellyCfg) generateCommandUsage(cmd Command) string {
	appName := cfg.Name
	colors := cfg.UsageColors
	wordWrap := cfg.wordWrap()
	revealKey := cfg.privateRevealEnvVar()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_%s_usage() {\n", appName, cmd.Name))
	if cmd.HelpHeaderOverride != "" {
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cmd.HelpHeaderOverride))
	} else {
		helpLines := strings.SplitN(cmd.Help, "\n", 2)
		b.WriteString(fmt.Sprintf("  echo \"%s %s - %s\"\n", appName, cmd.Name, helpLines[0]))
		if len(helpLines) > 1 {
			for _, l := range strings.Split(helpLines[1], "\n") {
				b.WriteString(fmt.Sprintf("  echo \"%s\"\n", l))
			}
		}
	}
	b.WriteString("  echo \"\"\n")

	argStr := ""
	for _, a := range cmd.Args {
		if a.Required {
			argStr += " <" + a.Name + ">"
		} else {
			argStr += " [" + a.Name + "]"
		}
	}
	b.WriteString(fmt.Sprintf("  echo \"%s %s %s%s [options]\"\n", cfg.m("usage", nil), appName, cmd.Name, argStr))
	b.WriteString("  echo \"\"\n")

	cfg.writeArgsHelp(&b, cmd.Args, colors.Caption, wordWrap)
	cfg.writeFlagsHelp(&b, cmd.Flags, colors.Caption, wordWrap, revealKey)

	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--help, -h", cfg.m("help_flag_text", nil)))

	if len(cmd.Examples) > 0 {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.m("examples", nil)))
		for _, ex := range cmd.Examples {
			b.WriteString(fmt.Sprintf("  echo \"  %s\"\n", ex))
		}
	}
	if len(cmd.EnvironmentVariables) > 0 {
		cfg.writeEnvVarsHelp(&b, cmd.EnvironmentVariables, colors.Caption, wordWrap, revealKey)
	}
	if cmd.CatchAll != nil && cmd.CatchAll.CatchHelp && cmd.CatchAll.Help != "" {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.m("additional_arguments", nil)))
		label := cmd.CatchAll.Label
		if label == "" {
			label = "args"
		}
		b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", label, cmd.CatchAll.Help))
	}
	if cmd.Footer != "" {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cmd.Footer))
	}
	b.WriteString("}\n")
	return b.String()
}

func (cfg *ShellyCfg) writeArgsHelp(b *strings.Builder, args []Arg, captionColor string, wordWrap ...int) {
	if len(args) == 0 {
		return
	}
	wrap := 0
	if len(wordWrap) > 0 {
		wrap = wordWrap[0]
	}
	// prefix: "  %-20s " = 2 + 20 + 1 = 23 chars
	const prefixLen = 23
	const indent = "  " + "                     " // 2 + 21 spaces = 23 chars
	b.WriteString(colorEcho("  ", captionColor, cfg.m("arguments", nil)))
	for _, a := range args {
		req := ""
		if a.Required {
			req = " " + cfg.m("required", nil)
		}
		def := ""
		if a.Default != "" {
			def = " " + cfg.m("default", map[string]string{"value": a.Default})
		}
		allowed := ""
		if len(a.Allowed) > 0 {
			allowed = " " + cfg.m("allowed", map[string]string{"values": strings.Join(a.Allowed, "|")})
		}
		helpFull := a.Help + req + def + allowed
		if wrap > 0 && wrap > prefixLen {
			avail := wrap - prefixLen
			wrapped := wrapText(helpFull, avail)
			lines := strings.Split(wrapped, "\n")
			b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", a.Name, lines[0]))
			for _, cont := range lines[1:] {
				b.WriteString(fmt.Sprintf("  echo \"%s%s\"\n", indent, cont))
			}
		} else {
			b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", a.Name, helpFull))
		}
	}
	b.WriteString("  echo \"\"\n")
}

func (cfg *ShellyCfg) writeFlagsHelp(b *strings.Builder, flags []Flag, captionColor string, wordWrap int, revealKey string) {
	if len(flags) == 0 {
		return
	}
	// prefix: "  %-24s " = 2 + 24 + 1 = 27 chars
	const prefixLen = 27
	const indent = "  " + "                          " // 2 + 25 spaces = 27 chars

	b.WriteString(colorEcho("  ", captionColor, cfg.m("options", nil)))
	for _, f := range flags {
		if f.Private {
			continue
		}
		name := flagHelpName(f)
		req := ""
		if f.Required {
			req = " " + cfg.m("required", nil)
		}
		def := ""
		if f.Default != "" {
			def = " " + cfg.m("default", map[string]string{"value": f.Default})
		}
		allowed := ""
		if len(f.Allowed) > 0 {
			allowed = " " + cfg.m("allowed", map[string]string{"values": strings.Join(f.Allowed, "|")})
		}
		helpFull := f.Help + req + def + allowed
		if wordWrap > 0 && wordWrap > prefixLen {
			avail := wordWrap - prefixLen
			wrapped := wrapText(helpFull, avail)
			lines := strings.Split(wrapped, "\n")
			b.WriteString(fmt.Sprintf("  echo \"  %-24s %s\"\n", name, lines[0]))
			for _, cont := range lines[1:] {
				b.WriteString(fmt.Sprintf("  echo \"%s%s\"\n", indent, cont))
			}
		} else {
			b.WriteString(fmt.Sprintf("  echo \"  %-24s %s\"\n", name, helpFull))
		}
	}
	b.WriteString("  echo \"\"\n")

	// private flags: shown only when reveal env var is set
	if revealKey != "" {
		privateFlags := []Flag{}
		for _, f := range flags {
			if f.Private {
				privateFlags = append(privateFlags, f)
			}
		}
		if len(privateFlags) > 0 {
			b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", revealKey))
			b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("private_options", nil)))
			for _, f := range privateFlags {
				name := flagHelpName(f)
				b.WriteString(fmt.Sprintf("    echo \"  %-24s %s\"\n", name, f.Help))
			}
			b.WriteString("    echo \"\"\n")
			b.WriteString("  fi\n")
		}
	}
}

// ─── Unified argument parser generation ──────────────────────────────────────

// writeFlagCases emits the case branches for every flag of cmd, together with
// the -h|--help branch, into b. It is shared by the command parser and the
// dispatch (global flags) parser.
func writeFlagCases(b *strings.Builder, appName string, cmd Command, otherArgs string, conjoined bool) {
	for _, f := range cmd.Flags {
		varName := flagVarName(f)
		for _, name := range flagSpellings(f) {
			isLong := strings.HasPrefix(name, "--")
			if f.Arg != "" {
				// inline --flag=value is always available; -flag=value only
				// when conjoined_flag_args is enabled.
				if isLong || conjoined {
					b.WriteString(fmt.Sprintf("      %s=*) %s=\"${1#*=}\"; shift ;;\n", name, varName))
				}
				if f.Repeatable {
					b.WriteString(fmt.Sprintf("      %s) %s=\"${%s} $2\"; shift 2 ;;\n", name, varName, varName))
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
	// help: when catch_help:true fall through to other_args; otherwise call usage and exit
	if cmd.CatchAll != nil && cmd.CatchAll.CatchHelp {
		b.WriteString(fmt.Sprintf("      -h|--help) %s=\"${%s} $1\"; shift ;;\n", otherArgs, otherArgs))
	} else {
		b.WriteString(fmt.Sprintf("      -h|--help) %s_%s_usage; exit 0 ;;\n", appName, cmd.Name))
	}
}

// writePositionalCase emits the *) branch that assigns positional arguments to
// their variables, tracking a running index (_pos) so that a trailing
// repeatable argument can accumulate every overflow token. Overflow beyond the
// declared positionals is routed to catch_all when present, otherwise it is an
// error (the invalid-argument behaviour).
func (cfg *ShellyCfg) writePositionalCase(b *strings.Builder, appName string, cmd Command, otherArgs string) {
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
				b.WriteString(fmt.Sprintf("          *) echo \"%s\" >&2; %s_%s_usage; exit 1 ;;\n", cfg.m("too_many_arguments", nil, "$1"), appName, cmd.Name))
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

// writeFlagPostChecks emits the post-loop checks for cmd's flags: defaults,
// required, allowed, conflicts, needs, unique dedup and validators.
func (cfg *ShellyCfg) writeFlagPostChecks(b *strings.Builder, appName string, cmd Command) {
	// default injection
	for _, f := range cmd.Flags {
		varName := flagVarName(f)
		if len(f.DefaultList) > 0 {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, strings.Join(f.DefaultList, " ")))
		} else if f.Default != "" {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, f.Default))
		}
	}

	// required flag checks
	for _, f := range cmd.Flags {
		if f.Required {
			varName := flagVarName(f)
			displayName := flagDisplayName(f)
			b.WriteString(fmt.Sprintf("  if [ -z \"${%s}\" ]; then\n", varName))
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("missing_required_flag", map[string]string{"name": displayName})))
			b.WriteString(fmt.Sprintf("    %s_%s_usage\n", appName, cmd.Name))
			if cmd.ShowExamplesOnError && len(cmd.Examples) > 0 {
				b.WriteString("    echo \"\"\n")
				b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("examples", nil)))
				for _, ex := range cmd.Examples {
					b.WriteString(fmt.Sprintf("    echo \"  %s\"\n", ex))
				}
			}
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
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
func (cfg *ShellyCfg) writeArgPostChecks(b *strings.Builder, appName string, cmd Command) {
	// defaults
	for _, a := range cmd.Args {
		varName := sanitizeVarName(a.Name)
		if len(a.DefaultList) > 0 {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, strings.Join(a.DefaultList, " ")))
		} else if a.Default != "" {
			b.WriteString(fmt.Sprintf("  : \"${%s:=%s}\"\n", varName, a.Default))
		}
	}

	// required checks
	for _, a := range cmd.Args {
		if a.Required {
			varName := sanitizeVarName(a.Name)
			b.WriteString(fmt.Sprintf("  if [ -z \"${%s}\" ]; then\n", varName))
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("missing_required_argument", map[string]string{"name": a.Name})))
			b.WriteString(fmt.Sprintf("    %s_%s_usage\n", appName, cmd.Name))
			if cmd.ShowExamplesOnError && len(cmd.Examples) > 0 {
				b.WriteString("    echo \"\"\n")
				b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("examples", nil)))
				for _, ex := range cmd.Examples {
					b.WriteString(fmt.Sprintf("    echo \"  %s\"\n", ex))
				}
			}
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
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
func (cfg *ShellyCfg) generateParser(appName string, cmd Command, dispatch bool) string {
	otherArgs := cfg.otherArgsVar()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_parse() {\n", cmd.Name))

	if cmd.CatchAll != nil {
		b.WriteString(fmt.Sprintf("  %s=\"\"\n", otherArgs))
	}
	if !dispatch {
		b.WriteString("  _options_ended=false\n")
		b.WriteString("  _pos=0\n")
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

	writeFlagCases(&b, appName, cmd, otherArgs, cfg.conjoinedFlagArgs())

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
	b.WriteString(fmt.Sprintf("      -*) echo \"%s\" >&2; exit 2 ;;\n", cfg.m("invalid_flag", nil, "$1")))
	if dispatch {
		b.WriteString("      *) break ;;\n")
	} else {
		cfg.writePositionalCase(&b, appName, cmd, otherArgs)
	}
	b.WriteString("    esac\n")
	b.WriteString("  done\n")

	// catch_all trim + required check (before everything else, as before)
	if cmd.CatchAll != nil {
		label := cmd.CatchAll.Label
		if label == "" {
			label = "extra"
		}
		b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", otherArgs, otherArgs))
		if cmd.CatchAll.Required {
			b.WriteString(fmt.Sprintf("  if [ -z \"${%s}\" ]; then\n", otherArgs))
			b.WriteString(fmt.Sprintf("    echo \"%s\" >&2\n", cfg.m("missing_required_argument", map[string]string{"name": label})))
			b.WriteString(fmt.Sprintf("    %s_%s_usage\n", appName, cmd.Name))
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
		}
	}

	// trim leading space from a repeatable positional accumulator
	for _, a := range cmd.Args {
		if a.Repeatable {
			v := sanitizeVarName(a.Name)
			b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", v, v))
			break
		}
	}

	cfg.writeFlagPostChecks(&b, appName, cmd)
	cfg.writeArgPostChecks(&b, appName, cmd)

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

// writeEnvVarsHelp writes env var section to usage, skipping private ones.
// Pass revealKey to also emit a reveal block for private env vars.
func (cfg *ShellyCfg) writeEnvVarsHelp(b *strings.Builder, evs []EnvironmentVariable, captionColor string, wordWrap int, revealKey string) {
	reveal := revealKey
	// prefix: "  %-20s " = 2 + 20 + 1 = 23 chars
	const prefixLen = 23
	const indent = "  " + "                     " // 2 + 21 spaces = 23 chars
	visible := make([]EnvironmentVariable, 0, len(evs))
	private := make([]EnvironmentVariable, 0)
	for _, ev := range evs {
		if ev.Private {
			private = append(private, ev)
		} else {
			visible = append(visible, ev)
		}
	}
	if len(visible) == 0 && (reveal == "" || len(private) == 0) {
		return
	}
	if len(visible) > 0 {
		b.WriteString("  echo \"\"\n")
		b.WriteString(colorEcho("  ", captionColor, cfg.m("environment_variables", nil)))
		for _, ev := range visible {
			req := ""
			if ev.Required {
				req = " " + cfg.m("required", nil)
			}
			helpFull := ev.Help + req
			if wordWrap > 0 && wordWrap > prefixLen {
				avail := wordWrap - prefixLen
				wrapped := wrapText(helpFull, avail)
				lines := strings.Split(wrapped, "\n")
				b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", ev.Name, lines[0]))
				for _, cont := range lines[1:] {
					b.WriteString(fmt.Sprintf("  echo \"%s%s\"\n", indent, cont))
				}
			} else {
				b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", ev.Name, helpFull))
			}
		}
	}
	// private env vars: shown only when reveal env var is set
	if reveal != "" && len(private) > 0 {
		b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ]; then\n", reveal))
		b.WriteString("    echo \"\"\n")
		b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("private_environment_variables", nil)))
		for _, ev := range private {
			b.WriteString(fmt.Sprintf("    echo \"  %-20s %s\"\n", ev.Name, ev.Help))
		}
		b.WriteString("  fi\n")
	}
}

// ─── Recursive command block writer ──────────────────────────────────────────

// writeCommandFunctions recursively writes flag parsers, arg parsers, and
// command bodies for a slice of commands. prefix is the function-name prefix
// built so far (e.g. "remote" for top-level, "remote_add" for nested).
// For commands that have sub-commands, a dispatcher body is generated instead
// of reading a src body file.
func writeCommandFunctions(f io.Writer, cfg *ShellyCfg, cmds []Command, prefix string) {
	for _, cmd := range cmds {
		funcBase := cmd.Name
		if prefix != cfg.Name {
			// nested: prefix is e.g. "remote"; funcBase becomes "remote_add"
			funcBase = prefix + "_" + cmd.Name
		}
		// Function override: use custom base name for generated functions
		if cmd.Function != "" {
			funcBase = cmd.Function
		}

		if len(cmd.Commands) > 0 {
			// This is a parent command — generate usage + dispatcher, recurse.
			// Usage for parent command listing its children.
			parentUsage := cfg.generateNestedUsage(funcBase, cmd)
			cfg.writeMarker(f, fmt.Sprintf("# :command.usage.%s\n", funcBase))
			write(f, parentUsage+"\n\n")

			// Recurse into children first (so children are defined before parent).
			writeCommandFunctions(f, cfg, cmd.Commands, funcBase)

			// Parent dispatcher function.
			cfg.writeMarker(f, fmt.Sprintf("# :command.function %s\n", funcBase))
			dispatcher := cfg.generateSubDispatcher(funcBase, cmd)
			write(f, fmt.Sprintf("%s_command() {\n%s\n}\n\n", funcBase, dispatcher))
		} else {
			// Leaf command — generate flag/arg parsers and body.
			// Temporarily rewrite cmd.Name so parser function names use funcBase.
			cmdCopy := cmd
			cmdCopy.Name = funcBase
			parser := cfg.generateParser(cfg.Name, cmdCopy, false)

			// Filename override: read body from custom path instead of default
			srcpath := cfg.commandPath(funcBase)
			if cmd.Filename != "" {
				srcpath = cmd.Filename
			}
			rawBody, readErr := os.ReadFile(srcpath)
			var body string
			if readErr != nil {
				body = fmt.Sprintf("  # TODO: implement %s command\n  return 0", funcBase)
				// only write skeleton if using default path (not custom filename)
				if cmd.Filename == "" {
					skeleton := fmt.Sprintf(
						"# Body of %s_command — do NOT add function header/footer.\n\n"+
							"# TODO: implement %s command\n"+
							"return 0\n",
						funcBase, funcBase,
					)
					_ = cfg.writeCommand(cfg.commandPath(funcBase), skeleton)
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
			fullBody := invokes + cfg.indentBody(cleanBody)

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
func (cfg *ShellyCfg) generateNestedUsage(funcBase string, cmd Command) string {
	appName := cfg.Name
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_usage() {\n", funcBase))
	b.WriteString(fmt.Sprintf("  echo \"%s %s - %s\"\n", appName, strings.ReplaceAll(funcBase, "_", " "), cmd.Help))
	b.WriteString("  echo \"\"\n")
	b.WriteString(fmt.Sprintf("  echo \"%s %s %s [subcommand] [options]\"\n", cfg.m("usage", nil), appName, strings.ReplaceAll(funcBase, "_", " ")))
	b.WriteString("  echo \"\"\n")
	b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.m("subcommands", nil)))
	for _, c := range cmd.Commands {
		if !c.Private {
			b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", c.Name, c.Help))
		}
	}
	b.WriteString("  echo \"\"\n")
	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--help, -h", cfg.m("help_flag_text", nil)))
	b.WriteString("}\n")
	return b.String()
}

// generateSubDispatcher builds the body of a parent command function that
// dispatches to child command functions.
func (cfg *ShellyCfg) generateSubDispatcher(funcBase string, cmd Command) string {
	appName := cfg.Name
	var b strings.Builder
	b.WriteString("  if [ $# -eq 0 ]; then\n")
	if cmd.Expose.IsAlways() {
		// expose:always — show subcommand listing inline, then usage, then return
		b.WriteString(fmt.Sprintf("    echo \"%s %s - %s\"\n", appName, strings.ReplaceAll(funcBase, "_", " "), cmd.Help))
		b.WriteString("    echo \"\"\n")
		b.WriteString(fmt.Sprintf("    echo \"%s\"\n", cfg.m("subcommands", nil)))
		for _, c := range cmd.Commands {
			if !c.Private {
				b.WriteString(fmt.Sprintf("    echo \"  %-20s %s\"\n", c.Name, c.Help))
			}
		}
		b.WriteString("    echo \"\"\n")
		b.WriteString(fmt.Sprintf("    %s_usage\n    return 0\n", funcBase))
	} else {
		b.WriteString(fmt.Sprintf("    %s_usage\n    return 0\n", funcBase))
	}
	b.WriteString("  fi\n")
	b.WriteString("  _subcmd=\"$1\"; shift\n")
	b.WriteString("  case \"$_subcmd\" in\n")
	writeDispatchArms(&b, cmd.Commands, func(c Command) string {
		return funcBase + "_" + c.Name + "_command"
	})
	b.WriteString(fmt.Sprintf("    -h|--help) %s_usage ;;\n", funcBase))
	b.WriteString(fmt.Sprintf("    *) echo \"%s\" >&2; %s_usage; exit 2 ;;\n", cfg.m("invalid_subcommand", nil, "$_subcmd"), funcBase))
	b.WriteString("  esac")
	return b.String()
}

// generateInspectArgs builds the default inspect_args() function that prints all
// parsed flag/arg/other_args variables to stderr for debugging. It walks the
// root and every command (including nested ones) so the same helper works from
// any command body. Values are read with ${var:-} so it is safe under set -u.
func (cfg *ShellyCfg) generateInspectArgs() string {
	var b strings.Builder
	b.WriteString("inspect_args() {\n")

	// collect all var names: root flags/args plus every command's flags/args/catch_all
	var varNames []string
	seen := map[string]bool{}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			varNames = append(varNames, n)
		}
	}
	for _, f := range cfg.Flags {
		add(flagVarName(f))
	}
	for _, a := range cfg.Args {
		add(sanitizeVarName(a.Name))
	}
	var walk func(cmds []Command)
	walk = func(cmds []Command) {
		for _, cmd := range cmds {
			for _, f := range cmd.Flags {
				add(flagVarName(f))
			}
			for _, a := range cmd.Args {
				add(sanitizeVarName(a.Name))
			}
			if cmd.CatchAll != nil {
				add(cfg.otherArgsVar())
			}
			walk(cmd.Commands)
		}
	}
	walk(cfg.Commands)

	if len(varNames) > 0 {
		b.WriteString("  printf 'inspecting args:\\n' >&2\n")
		for _, v := range varNames {
			b.WriteString(fmt.Sprintf("  printf '  %s: %%s\\n' \"${%s:-}\" >&2\n", v, v))
		}
	} else {
		b.WriteString("  # hook: validate positional args - override in src/inspect_args.sh\n  return 0\n")
	}
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

	// per-command usage functions (always auto-generated unless user file present)
	for _, cmd := range cfg.Commands {
		if cmd.Private {
			continue
		}
		cmdUsageSrc := cfg.readSrcOrDefault(fmt.Sprintf("%s_usage.sh", cmd.Name), "")
		var cmdUsage string
		if strings.TrimSpace(cmdUsageSrc) != "" {
			cmdUsage = strings.ReplaceAll(cmdUsageSrc, "%APP_NAME%", cfg.Name)
		} else {
			cmdUsage = cfg.generateCommandUsage(cmd)
		}
		cfg.writeMarker(f, fmt.Sprintf("# :command.usage.%s\n", cmd.Name))
		write(f, cmdUsage+"\n\n")
	}

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
			rp := cfg.generateParser(cfg.Name, rootCmd, false)
			cfg.writeMarker(f, "# :command.root.parse\n")
			write(f, rp+"\n")
		}

		srcpath := filepath.Join(cfg.sourceDir(), "root_command.sh")
		rawBody, readErr := os.ReadFile(srcpath)
		var rootBodyContent string
		if readErr != nil {
			rootBodyContent = "  # TODO: implement root command\n  return 0"
		} else {
			rootBodyContent = cfg.indentBody(strings.TrimRight(string(rawBody), "\n"))
		}

		invokes := ""
		if len(cfg.Flags) > 0 || len(cfg.Args) > 0 {
			invokes += "  root_parse \"$@\"\n"
		}
		if invokes == "" {
			invokes = fmt.Sprintf("  %s_usage\n  return 0\n", cfg.Name)
		}
		cfg.writeMarker(f, "# :command.root\n")
		write(f, fmt.Sprintf("root_command() {\n%s%s\n}\n\n", invokes, rootBodyContent))
	} else {
		// global flags: when the root has flags alongside subcommands, emit a parser first
		if len(cfg.Flags) > 0 {
			globalFlagCmd := Command{
				Name:  cfg.Name,
				Flags: cfg.Flags,
			}
			gfp := cfg.generateParser(cfg.Name, globalFlagCmd, true)
			cfg.writeMarker(f, "# :command.global.flags\n")
			write(f, gfp+"\n")
		}

		writeCommandFunctions(f, cfg, cfg.Commands, cfg.Name)
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
	runBuilder.WriteString("  normalize_input \"$@\"\n")
	// global flag parser call (when root has flags alongside subcommands)
	if len(cfg.Commands) > 0 && len(cfg.Flags) > 0 {
		runBuilder.WriteString(fmt.Sprintf("  %s_parse \"$@\"\n", cfg.Name))
	}
	// before hook — call before_hook() if src/before.sh was present
	if hasBeforeHook {
		runBuilder.WriteString("  before_hook\n")
	}

	// find default command (if any) and whether it forces on zero args
	defaultCmd := ""
	defaultFuncName := ""
	forceDefault := false
	for _, c := range cfg.Commands {
		isForce := c.DefaultCmd.IsForce()
		if c.DefaultCmd.IsDefault() || isForce {
			defaultCmd = c.Name
			defaultFuncName = c.Name
			if c.Function != "" {
				defaultFuncName = c.Function
			}
			forceDefault = isForce
			break
		}
	}

	if forceDefault && defaultCmd != "" {
		runBuilder.WriteString("  if [ $# -eq 0 ]; then\n")
		runBuilder.WriteString(fmt.Sprintf("    %s_command\n    return 0\n", defaultFuncName))
		runBuilder.WriteString("  fi\n")
	} else {
		runBuilder.WriteString("  if [ $# -eq 0 ]; then\n")
		runBuilder.WriteString(fmt.Sprintf("    %s_usage\n    return 0\n", cfg.Name))
		runBuilder.WriteString("  fi\n")
	}

	runBuilder.WriteString("  _cmd=\"$1\"; shift\n")
	runBuilder.WriteString("  case \"$_cmd\" in\n")

	writeDispatchArms(&runBuilder, cfg.Commands, func(c Command) string {
		fn := c.Name
		if c.Function != "" {
			fn = c.Function
		}
		return fn + "_command"
	})
	runBuilder.WriteString("    --version) version_command ;;\n")
	runBuilder.WriteString(fmt.Sprintf("    -h|--help) %s_usage ;;\n", cfg.Name))
	if defaultCmd != "" {
		runBuilder.WriteString(fmt.Sprintf("    *) %s_command \"$_cmd\" \"$@\" ;;\n", defaultFuncName))
	} else if cfg.Extensible.IsBool() {
		// extensible: try <appname>-<cmd> in PATH before erroring
		runBuilder.WriteString(fmt.Sprintf("    *)\n"))
		runBuilder.WriteString(fmt.Sprintf("      if command -v %s-\"$_cmd\" >/dev/null 2>&1; then\n", cfg.Name))
		runBuilder.WriteString(fmt.Sprintf("        %s-\"$_cmd\" \"$@\"\n", cfg.Name))
		runBuilder.WriteString("      else\n")
		runBuilder.WriteString(fmt.Sprintf("        echo \"%s\" >&2; %s_usage; exit 2\n", cfg.m("invalid_command", nil, "$_cmd"), cfg.Name))
		runBuilder.WriteString("      fi ;;\n")
	} else if cfg.Extensible.Delegate() != "" {
		// extensible with delegate: forward unknown commands to the delegate tool
		runBuilder.WriteString("    *)\n")
		runBuilder.WriteString(fmt.Sprintf("      %s \"$_cmd\" \"$@\" ;;\n", cfg.Extensible.Delegate()))
	} else {
		runBuilder.WriteString(fmt.Sprintf("    *) echo \"%s\" >&2; %s_usage; exit 2 ;;\n", cfg.m("invalid_command", nil, "$_cmd"), cfg.Name))
	}
	runBuilder.WriteString("  esac\n")
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

// commandFunc creates a body-only skeleton for a command if not present.
func (cfg *ShellyCfg) commandFunc(command string) {
	filename := cfg.commandPath(command)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		content := fmt.Sprintf(
			"# Body of %s_command — do NOT add function header/footer.\n\n"+
				"# TODO: implement %s command\n"+
				"return 0\n",
			command, command,
		)
		_ = cfg.writeCommand(cfg.commandPath(command), content)
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
