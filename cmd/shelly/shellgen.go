package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func write(f *os.File, content string) {
	_, err := f.WriteString(content)
	if err != nil {
		fmt.Println("Error writing to shell script file:", err)
		os.Exit(1)
	}
}

// writeMarker emits a "# :command.xxx\n" section marker unless DisableViewMarkers is set.
func (cfg *ShellyCfg) writeMarker(f *os.File, marker string) {
	if cfg.DisableViewMarkers {
		return
	}
	write(f, marker)
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

// readSrcOrDefault reads src/<name> if it exists, else returns def.
func readSrcOrDefault(name, def string) string {
	p := filepath.Join("src", name)
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

// sanitizeVarName strips leading dashes and replaces inner dashes with underscores.
func sanitizeVarName(long string) string {
	return strings.ReplaceAll(strings.TrimLeft(long, "-"), "-", "_")
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

// ansiCode maps a color name to a POSIX printf ANSI escape sequence fragment.
// If name is already a raw escape (starts with \033 or \e), it is returned as-is.
// Returns "" if name is empty or unrecognised.
func ansiCode(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "red":
		return "\\033[31m"
	case "green":
		return "\\033[32m"
	case "yellow":
		return "\\033[33m"
	case "blue":
		return "\\033[34m"
	case "magenta":
		return "\\033[35m"
	case "cyan":
		return "\\033[36m"
	case "bold":
		return "\\033[1m"
	case "dim":
		return "\\033[2m"
	case "":
		return ""
	default:
		// pass raw sequences through
		return name
	}
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
		b.WriteString(fmt.Sprintf("  echo \"Usage: %s [command] [options]\"\n", cfg.Name))
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
				b.WriteString(colorEcho("  ", cfg.UsageColors.Caption, g+":"))
			} else {
				b.WriteString(colorEcho("  ", cfg.UsageColors.Caption, "Commands:"))
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
			b.WriteString("    echo \"Private Commands:\"\n")
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
		b.WriteString(fmt.Sprintf("  echo \"Usage: %s%s [options]\"\n", cfg.Name, argStr))
		b.WriteString("  echo \"\"\n")
		writeArgsHelp(&b, cfg.Args, cfg.UsageColors.Caption, cfg.WordWrap)
		writeFlagsHelp(&b, cfg.Flags, cfg.UsageColors.Caption, cfg.WordWrap, cfg.privateRevealEnvVar())
	}
	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--help, -h", "Show this help"))
	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--version", "Show version"))
	if len(cfg.Examples) > 0 {
		b.WriteString("  echo \"\"\n")
		b.WriteString("  echo \"Examples:\"\n")
		for _, ex := range cfg.Examples {
			b.WriteString(fmt.Sprintf("  echo \"  %s\"\n", ex))
		}
	}
	if len(cfg.EnvironmentVariables) > 0 {
		writeEnvVarsHelp(&b, cfg.EnvironmentVariables, cfg.UsageColors.Caption, cfg.WordWrap, cfg.privateRevealEnvVar())
	}
	if cfg.Footer != "" {
		b.WriteString("  echo \"\"\n")
		b.WriteString(fmt.Sprintf("  echo \"%s\"\n", cfg.Footer))
	}
	b.WriteString("}\n")
	return b.String()
}

func generateCommandUsage(appName string, cmd Command, revealKey string, wordWrap int, colors UsageColors) string {
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
	b.WriteString(fmt.Sprintf("  echo \"Usage: %s %s%s [options]\"\n", appName, cmd.Name, argStr))
	b.WriteString("  echo \"\"\n")

	writeArgsHelp(&b, cmd.Args, colors.Caption, wordWrap)
	writeFlagsHelp(&b, cmd.Flags, colors.Caption, wordWrap, revealKey)

	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--help, -h", "Show this help"))

	if len(cmd.Examples) > 0 {
		b.WriteString("  echo \"\"\n")
		b.WriteString("  echo \"Examples:\"\n")
		for _, ex := range cmd.Examples {
			b.WriteString(fmt.Sprintf("  echo \"  %s\"\n", ex))
		}
	}
	if len(cmd.EnvironmentVariables) > 0 {
		writeEnvVarsHelp(&b, cmd.EnvironmentVariables, colors.Caption, wordWrap, revealKey)
	}
	if cmd.CatchAll != nil && cmd.CatchAll.CatchHelp && cmd.CatchAll.Help != "" {
		b.WriteString("  echo \"\"\n")
		b.WriteString("  echo \"Additional arguments:\"\n")
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

func writeArgsHelp(b *strings.Builder, args []Arg, captionColor string, wordWrap ...int) {
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
	b.WriteString(colorEcho("  ", captionColor, "Arguments:"))
	for _, a := range args {
		req := ""
		if a.Required {
			req = " (required)"
		}
		def := ""
		if a.Default != "" {
			def = fmt.Sprintf(" (default: %s)", a.Default)
		}
		allowed := ""
		if len(a.Allowed) > 0 {
			allowed = fmt.Sprintf(" [%s]", strings.Join(a.Allowed, "|"))
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

func writeFlagsHelp(b *strings.Builder, flags []Flag, captionColor string, wordWrap int, revealKey string) {
	if len(flags) == 0 {
		return
	}
	// prefix: "  %-24s " = 2 + 24 + 1 = 27 chars
	const prefixLen = 27
	const indent = "  " + "                          " // 2 + 25 spaces = 27 chars

	b.WriteString(colorEcho("  ", captionColor, "Options:"))
	for _, f := range flags {
		if f.Private {
			continue
		}
		name := f.Long
		if f.Short != "" {
			name = f.Short + ", " + f.Long
		}
		if f.Arg != "" {
			name += " <" + f.Arg + ">"
		}
		req := ""
		if f.Required {
			req = " (required)"
		}
		def := ""
		if f.Default != "" {
			def = fmt.Sprintf(" (default: %s)", f.Default)
		}
		allowed := ""
		if len(f.Allowed) > 0 {
			allowed = fmt.Sprintf(" [%s]", strings.Join(f.Allowed, "|"))
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
			b.WriteString("    echo \"Private Options:\"\n")
			for _, f := range privateFlags {
				name := f.Long
				if f.Short != "" {
					name = f.Short + ", " + f.Long
				}
				if f.Arg != "" {
					name += " <" + f.Arg + ">"
				}
				b.WriteString(fmt.Sprintf("    echo \"  %-24s %s\"\n", name, f.Help))
			}
			b.WriteString("    echo \"\"\n")
			b.WriteString("  fi\n")
		}
	}
}

// ─── Flag parser generation ───────────────────────────────────────────────────

// generateFlagParser builds <cmd>_parse_flags() with:
//   - long/short boolean and value flags
//   - repeatable flags (counter for boolean, accumulate for value)
//   - catch_all: remaining non-flag args accumulated into other_args (or cfg.otherArgsVar())
//   - default value injection after the loop
//   - required flag check after defaults
//   - allowed value check after required
//   - conflicts check (mutual exclusion)
//   - needs check (co-requirement)
func (cfg *ShellyCfg) generateFlagParser(appName string, cmd Command) string {
	otherArgs := cfg.otherArgsVar()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_parse_flags() {\n", cmd.Name))

	// initialise other_args if catch_all is set
	if cmd.CatchAll != nil {
		b.WriteString(fmt.Sprintf("  %s=\"\"\n", otherArgs))
	}

	b.WriteString("  while [ $# -gt 0 ]; do\n")
	b.WriteString("    case \"$1\" in\n")
	for _, f := range cmd.Flags {
		// Derive varName: prefer Long, fall back to Short (strip leading -)
		varName := sanitizeVarName(f.Long)
		if varName == "" && f.Short != "" {
			varName = sanitizeVarName(f.Short)
		}
		if f.Arg != "" {
			// --flag=value inline form only available for long flags
			if f.Long != "" {
				b.WriteString(fmt.Sprintf("      %s=*) %s=\"${1#*=}\"; shift ;;\n", f.Long, varName))
			}
			if f.Repeatable {
				if f.Long != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=\"${%s} $2\"; shift 2 ;;\n", f.Long, varName, varName))
				}
				if f.Short != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=\"${%s} $2\"; shift 2 ;;\n", f.Short, varName, varName))
				}
			} else {
				if f.Long != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=\"$2\"; shift 2 ;;\n", f.Long, varName))
				}
				if f.Short != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=\"$2\"; shift 2 ;;\n", f.Short, varName))
				}
			}
		} else {
			if f.Repeatable {
				if f.Long != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=$((${%s:-0}+1)); shift ;;\n", f.Long, varName, varName))
				}
				if f.Short != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=$((${%s:-0}+1)); shift ;;\n", f.Short, varName, varName))
				}
			} else {
				if f.Long != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=1; shift ;;\n", f.Long, varName))
				}
				if f.Short != "" {
					b.WriteString(fmt.Sprintf("      %s) %s=1; shift ;;\n", f.Short, varName))
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
	b.WriteString("      --) shift; break ;;\n")
	if cmd.CatchAll != nil {
		b.WriteString("      -*) echo \"Unknown flag: $1\" >&2; exit 2 ;;\n")
		b.WriteString(fmt.Sprintf("      *) %s=\"${%s} $1\"; shift ;;\n", otherArgs, otherArgs))
	} else {
		b.WriteString("      -*) echo \"Unknown flag: $1\" >&2; exit 2 ;;\n")
		b.WriteString("      *) break ;;\n")
	}
	b.WriteString("    esac\n")
	b.WriteString("  done\n")

	// catch_all required check
	if cmd.CatchAll != nil && cmd.CatchAll.Required {
		label := cmd.CatchAll.Label
		if label == "" {
			label = "extra"
		}
		b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", otherArgs, otherArgs))
		b.WriteString(fmt.Sprintf("  if [ -z \"${%s}\" ]; then\n", otherArgs))
		b.WriteString(fmt.Sprintf("    echo \"Error: <%s> is required\" >&2\n", label))
		b.WriteString(fmt.Sprintf("    %s_%s_usage\n", appName, cmd.Name))
		b.WriteString("    exit 1\n")
		b.WriteString("  fi\n")
	} else if cmd.CatchAll != nil {
		b.WriteString(fmt.Sprintf("  %s=\"${%s# }\"\n", otherArgs, otherArgs))
	}

	// default injection
	for _, f := range cmd.Flags {
		varName := flagVarName(f)
		if len(f.DefaultList) > 0 {
			// array default: space-delimited string
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
			b.WriteString(fmt.Sprintf("    echo \"Error: %s is required\" >&2\n", displayName))
			b.WriteString(fmt.Sprintf("    %s_%s_usage\n", appName, cmd.Name))
			if cmd.ShowExamplesOnError && len(cmd.Examples) > 0 {
				b.WriteString("    echo \"\"\n")
				b.WriteString("    echo \"Examples:\"\n")
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
			b.WriteString(fmt.Sprintf("      *) echo \"Error: %s must be one of: %s\" >&2; exit 1 ;;\n",
				displayName, strings.Join(f.Allowed, ", ")))
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
				conflictVar := sanitizeVarName(conflict)
				b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ] && [ -n \"${%s}\" ]; then\n", varName, conflictVar))
				b.WriteString(fmt.Sprintf("    echo \"Error: %s and %s cannot be used together\" >&2\n", displayName, conflict))
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
				needVar := sanitizeVarName(need)
				b.WriteString(fmt.Sprintf("  if [ -n \"${%s}\" ] && [ -z \"${%s}\" ]; then\n", varName, needVar))
				b.WriteString(fmt.Sprintf("    echo \"Error: %s requires %s\" >&2\n", displayName, need))
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

	b.WriteString("}\n")
	return b.String()
}

// ─── Arg parser generation ────────────────────────────────────────────────────

// generateArgParser builds <cmd>_parse_args() which:
//   - assigns positional $1..$N to named variables
//   - for the last arg with Repeatable=true, captures all remaining as space-delimited
//   - applies defaults
//   - enforces required
//   - validates allowed values
func generateArgParser(appName string, cmd Command) string {
	if len(cmd.Args) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_parse_args() {\n", cmd.Name))

	for i, a := range cmd.Args {
		varName := sanitizeVarName(a.Name)
		if a.Repeatable {
			// shift past the earlier positionals, then grab all remaining
			if i > 0 {
				b.WriteString(fmt.Sprintf("  shift %d\n", i))
			}
			b.WriteString(fmt.Sprintf("  %s=\"$*\"\n", varName))
			// repeatable consumes all remaining args — stop processing further args
			break
		} else {
			b.WriteString(fmt.Sprintf("  %s=\"${%d}\"\n", varName, i+1))
		}
	}

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
			b.WriteString(fmt.Sprintf("    echo \"Error: <%s> is required\" >&2\n", a.Name))
			b.WriteString(fmt.Sprintf("    %s_%s_usage\n", appName, cmd.Name))
			if cmd.ShowExamplesOnError && len(cmd.Examples) > 0 {
				b.WriteString("    echo \"\"\n")
				b.WriteString("    echo \"Examples:\"\n")
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
			b.WriteString(fmt.Sprintf("      *) echo \"Error: <%s> must be one of: %s\" >&2; exit 1 ;;\n",
				a.Name, strings.Join(a.Allowed, ", ")))
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
			b.WriteString(fmt.Sprintf("      *\" ${_item} \"*) ;;\n"))
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

	b.WriteString("}\n")
	return b.String()
}

// ─── Dependencies / env var checks ───────────────────────────────────────────

// generateDepsCheck builds a parse_requirements() body checking each dependency.
func generateDepsCheck(deps []Dependency) string {
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
			depVar := strings.ReplaceAll(name, "-", "_")
			// single dep — simple check
			b.WriteString(fmt.Sprintf("  if ! command -v %s >/dev/null 2>&1; then\n", name))
			b.WriteString(fmt.Sprintf("    echo \"Error: required dependency '%s' not found\" >&2\n", name))
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
				b.WriteString(fmt.Sprintf("    echo \"Error: %s version %s or compatible required\" >&2\n", name, d.Version))
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
			b.WriteString(fmt.Sprintf("    echo \"Error: one of [%s] is required but none found\" >&2\n", strings.Join(names, ", ")))
			if d.Help != "" {
				b.WriteString(fmt.Sprintf("    echo \"  %s\" >&2\n", d.Help))
			}
			b.WriteString("    exit 1\n")
			b.WriteString("  fi\n")
			// capture first found into deps_<first-name>
			for _, n := range names {
				depVar := strings.ReplaceAll(n, "-", "_")
				b.WriteString(fmt.Sprintf("  if command -v %s >/dev/null 2>&1; then deps_%s=\"$(command -v %s)\"; fi\n", n, depVar, n))
			}
		}
	}
	return b.String()
}

// generateEnvCheck builds validation for a slice of EnvironmentVariables.
func generateEnvCheck(evs []EnvironmentVariable) string {
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
			b.WriteString(fmt.Sprintf("    echo \"Error: environment variable %s is required\" >&2\n", ev.Name))
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
			b.WriteString(fmt.Sprintf("      *) echo \"Error: %s must be one of: %s\" >&2; exit 1 ;;\n",
				ev.Name, strings.Join(ev.Allowed, ", ")))
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
func writeEnvVarsHelp(b *strings.Builder, evs []EnvironmentVariable, captionColor string, wordWrap int, revealKey string) {
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
		b.WriteString(colorEcho("  ", captionColor, "Environment Variables:"))
		for _, ev := range visible {
			req := ""
			if ev.Required {
				req = " (required)"
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
		b.WriteString("    echo \"Private Environment Variables:\"\n")
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
func writeCommandFunctions(f *os.File, cfg *ShellyCfg, cmds []Command, prefix string) {
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
			parentUsage := generateNestedUsage(cfg.Name, funcBase, cmd)
			cfg.writeMarker(f, fmt.Sprintf("# :command.usage.%s\n", funcBase))
			write(f, parentUsage+"\n\n")

			// Recurse into children first (so children are defined before parent).
			writeCommandFunctions(f, cfg, cmd.Commands, funcBase)

			// Parent dispatcher function.
			cfg.writeMarker(f, fmt.Sprintf("# :command.function %s\n", funcBase))
			dispatcher := generateSubDispatcher(cfg.Name, funcBase, cmd)
			write(f, fmt.Sprintf("%s_command() {\n%s\n}\n\n", funcBase, dispatcher))
		} else {
			// Leaf command — generate flag/arg parsers and body.
			// Temporarily rewrite cmd.Name so parser function names use funcBase.
			cmdCopy := cmd
			cmdCopy.Name = funcBase
			flagParser := cfg.generateFlagParser(cfg.Name, cmdCopy)
			argParser := generateArgParser(cfg.Name, cmdCopy)

			// Filename override: read body from custom path instead of default
			srcpath := filepath.Join("src", fmt.Sprintf("%s_command.sh", funcBase))
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
					_ = cfg.writeCommand(fmt.Sprintf("src/%s_command.sh", funcBase), skeleton)
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

			// inject flag + arg parser calls at top of body
			invokes := fmt.Sprintf("  %s_parse_flags \"$@\"\n", funcBase)
			if len(cmd.Args) > 0 {
				invokes += fmt.Sprintf("  %s_parse_args \"$@\"\n", funcBase)
			}
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
			envCheckCmd := generateEnvCheck(cmd.EnvironmentVariables)
			depsCheckCmd := generateDepsCheck(cmd.Dependencies)
			preamble := ""
			// argfile: load default flags from ~/.config/<appname>/<cmd>
			if cmd.Argfile {
				argfilePath := fmt.Sprintf("${HOME}/.config/%s/%s", cfg.Name, funcBase)
				preamble += "  # argfile: load default flags from file\n"
				preamble += fmt.Sprintf("  _argfile=\"%s\"\n", argfilePath)
				preamble += "  if [ -f \"$_argfile\" ]; then\n"
				preamble += "    _argfile_args=\"\"\n"
				preamble += "    while IFS= read -r _argfile_line || [ -n \"$_argfile_line\" ]; do\n"
				preamble += "      case \"$_argfile_line\" in\n"
				preamble += "        ''|'#'*) continue ;;\n"
				preamble += "        -*) _argfile_args=\"${_argfile_args} ${_argfile_line}\" ;;\n"
				preamble += "      esac\n"
				preamble += "    done < \"$_argfile\"\n"
				preamble += "    _argfile_args=\"${_argfile_args# }\"\n"
				preamble += "    if [ -n \"$_argfile_args\" ]; then\n"
				preamble += "      eval \"set -- $_argfile_args \\\"\\$@\\\"\"\n"
				preamble += "    fi\n"
				preamble += "    unset _argfile_args _argfile_line\n"
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
			write(f, flagParser+"\n")
			if argParser != "" {
				write(f, argParser+"\n")
			}
			write(f, fmt.Sprintf("%s_command() {\n%s\n}\n\n", funcBase, fullBody))
		}
	}
}

// generateNestedUsage builds a usage function for a parent command listing its sub-commands.
func generateNestedUsage(appName, funcBase string, cmd Command) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s_usage() {\n", funcBase))
	b.WriteString(fmt.Sprintf("  echo \"%s %s - %s\"\n", appName, strings.ReplaceAll(funcBase, "_", " "), cmd.Help))
	b.WriteString("  echo \"\"\n")
	b.WriteString(fmt.Sprintf("  echo \"Usage: %s %s [subcommand] [options]\"\n", appName, strings.ReplaceAll(funcBase, "_", " ")))
	b.WriteString("  echo \"\"\n")
	b.WriteString("  echo \"Subcommands:\"\n")
	for _, c := range cmd.Commands {
		if !c.Private {
			b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", c.Name, c.Help))
		}
	}
	b.WriteString("  echo \"\"\n")
	b.WriteString(fmt.Sprintf("  echo \"  %-20s %s\"\n", "--help, -h", "Show this help"))
	b.WriteString("}\n")
	return b.String()
}

// generateSubDispatcher builds the body of a parent command function that
// dispatches to child command functions.
func generateSubDispatcher(appName, funcBase string, cmd Command) string {
	var b strings.Builder
	b.WriteString("  if [ $# -eq 0 ]; then\n")
	if cmd.Expose.IsAlways() {
		// expose:always — show subcommand listing inline, then usage, then return
		b.WriteString(fmt.Sprintf("    echo \"%s %s - %s\"\n", appName, strings.ReplaceAll(funcBase, "_", " "), cmd.Help))
		b.WriteString("    echo \"\"\n")
		b.WriteString("    echo \"Subcommands:\"\n")
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
	for _, c := range cmd.Commands {
		childBase := funcBase + "_" + c.Name
		b.WriteString(fmt.Sprintf("    %s) %s_command \"$@\" ;;\n", c.Name, childBase))
		for _, alias := range c.allAliases() {
			b.WriteString(fmt.Sprintf("    %s) %s_command \"$@\" ;;\n", alias, childBase))
		}
	}
	b.WriteString(fmt.Sprintf("    -h|--help) %s_usage ;;\n", funcBase))
	b.WriteString(fmt.Sprintf("    *) echo \"Unknown subcommand: $_subcmd\" >&2; %s_usage; exit 2 ;;\n", funcBase))
	b.WriteString("  esac")
	return b.String()
}

// generateInspectArgs builds the default inspect_args() function that prints all
// parsed flag/arg/other_args variables to stderr for debugging.
func (cfg *ShellyCfg) generateInspectArgs() string {
	var b strings.Builder
	b.WriteString("inspect_args() {\n")

	// collect all var names: root flags/args, or per-command flags/args
	var varNames []string
	if len(cfg.Commands) == 0 {
		for _, f := range cfg.Flags {
			varNames = append(varNames, flagVarName(f))
		}
		for _, a := range cfg.Args {
			varNames = append(varNames, sanitizeVarName(a.Name))
		}
	} else {
		seen := map[string]bool{}
		for _, cmd := range cfg.Commands {
			for _, f := range cmd.Flags {
				n := flagVarName(f)
				if !seen[n] {
					seen[n] = true
					varNames = append(varNames, n)
				}
			}
			for _, a := range cmd.Args {
				n := sanitizeVarName(a.Name)
				if !seen[n] {
					seen[n] = true
					varNames = append(varNames, n)
				}
			}
			if cmd.CatchAll != nil {
				oav := cfg.otherArgsVar()
				if !seen[oav] {
					seen[oav] = true
					varNames = append(varNames, oav)
				}
			}
		}
	}

	if len(varNames) > 0 {
		b.WriteString("  printf 'inspecting args:\\n' >&2\n")
		for _, v := range varNames {
			b.WriteString(fmt.Sprintf("  printf '  %s: %%s\\n' \"${%s}\" >&2\n", v, v))
		}
	} else {
		b.WriteString("  # hook: validate positional args - override in src/inspect_args.sh\n  return 0\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func (cfg *ShellyCfg) shellGen() error {
	outPath := fmt.Sprintf("./%s", cfg.Name)
	var permissions os.FileMode = 0o755
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
	if err != nil {
		return fmt.Errorf("error creating shell script file: %w", err)
	}
	defer f.Close()
	if err := cfg.shellGenToWriter(f); err != nil {
		return err
	}
	fmt.Printf("created ./%s\n", cfg.Name)
	fmt.Printf("run ./%s --help to test your script\n", cfg.Name)
	if cfg.Formatter != "" && cfg.Formatter != "none" {
		if err := runFormatter(outPath, cfg.Formatter); err != nil {
			return err
		}
	}
	return nil
}

// shellGenToWriter writes the complete generated script to f.
// All generation logic lives here; shellGen() and shellGenToString() both call it.
func (cfg *ShellyCfg) shellGenToWriter(f *os.File) error {
	// production mode: disable view markers globally
	if cfg.isProduction() {
		cfg.DisableViewMarkers = true
	}

	// Discover lib files
	libFiles, _ := filepath.Glob("src/lib/*.sh")
	if len(libFiles) == 0 {
		libFiles, _ = filepath.Glob("src/lib/*")
	}
	sort.Strings(libFiles)
	debugf("debug: lib files: %v\n", libFiles)

	// Build name->body map from body-only lib files; detect duplicates
	funcBodies := map[string]string{}
	funcOrigins := map[string]string{}
	duplicates := map[string][]string{}
	for _, p := range libFiles {
		name := stemName(p)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if _, exists := funcBodies[name]; exists {
			duplicates[name] = append(duplicates[name], funcOrigins[name], p)
		} else {
			funcBodies[name] = string(b)
			funcOrigins[name] = p
		}
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
	headerSrc := readSrcOrDefault("header.sh", "")
	if strings.TrimSpace(headerSrc) != "" {
		cfg.writeMarker(f, "# :command.header\n")
		write(f, strings.TrimRight(headerSrc, "\n")+"\n\n")
	}

	// preamble — raw content injected before any function definitions
	preambleSrc := readSrcOrDefault("preamble.sh", "")
	if strings.TrimSpace(preambleSrc) != "" {
		preambleSrc = strings.ReplaceAll(preambleSrc, "%APP_NAME%", cfg.Name)
		preambleSrc = strings.ReplaceAll(preambleSrc, "%VERSION%", cfg.Version)
		cfg.writeMarker(f, "# :command.preamble\n")
		write(f, strings.TrimRight(preambleSrc, "\n")+"\n\n")
	}

	// version_command
	versionDef := "version_command() {\n  echo \"$version\"\n}\n"
	cfg.writeMarker(f, "# :command.version_command\n")
	write(f, readSrcOrDefault("version_command.sh", versionDef)+"\n\n")

	// usage — auto-generated from YAML unless user provides src/usage.sh
	usageSrc := readSrcOrDefault("usage.sh", "")
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
		cmdUsageSrc := readSrcOrDefault(fmt.Sprintf("%s_usage.sh", cmd.Name), "")
		var cmdUsage string
		if strings.TrimSpace(cmdUsageSrc) != "" {
			cmdUsage = strings.ReplaceAll(cmdUsageSrc, "%APP_NAME%", cfg.Name)
		} else {
			cmdUsage = generateCommandUsage(cfg.Name, cmd, cfg.privateRevealEnvVar(), cfg.WordWrap, cfg.UsageColors)
		}
		cfg.writeMarker(f, fmt.Sprintf("# :command.usage.%s\n", cmd.Name))
		write(f, cmdUsage+"\n\n")
	}

	// normalize_input
	normalizeDef := "normalize_input() {\n" +
		"  _newargs=\"\"\n" +
		"  for _arg in \"$@\"; do\n" +
		"    case \"$_arg\" in\n" +
		"      --*=*) _newargs=\"$_newargs ${_arg%%=*} ${_arg#*=}\" ;;\n" +
		"      -*[!-]*)  \n" +
		"        _flag=\"${_arg%${_arg#??}}\"\n" +
		"        _rest=\"-${_arg#??}\"\n" +
		"        if [ ${#_arg} -gt 2 ] && [ \"$_rest\" != \"-\" ]; then\n" +
		"          _newargs=\"$_newargs $_flag $_rest\"\n" +
		"        else\n" +
		"          _newargs=\"$_newargs $_arg\"\n" +
		"        fi ;;\n" +
		"      *) _newargs=\"$_newargs $_arg\" ;;\n" +
		"    esac\n" +
		"  done\n" +
		"  eval set -- $_newargs\n" +
		"}\n"
	normalize := strings.ReplaceAll(readSrcOrDefault("normalize_input.sh", normalizeDef), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.normalize_input\n")
	write(f, normalize+"\n\n")

	// inspect_args: emit dev utility in dev mode; skip in production
	if !cfg.isProduction() {
		inspectDef := cfg.generateInspectArgs()
		inspect := strings.ReplaceAll(readSrcOrDefault("inspect_args.sh", inspectDef), "%APP_NAME%", cfg.Name)
		cfg.writeMarker(f, "# :command.inspect_args\n")
		write(f, inspect+"\n\n")
	}

	// parse_requirements stub (overridable)
	parseReqDef := "parse_requirements() {\n  # hook: check env/deps - override in src/parse_requirements.sh\n  return 0\n}\n"
	parseReq := strings.ReplaceAll(readSrcOrDefault("parse_requirements.sh", parseReqDef), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.parse_requirements\n")
	write(f, parseReq+"\n\n")

	// command_functions shared helpers
	cmdFuncsDef := "# shared helper functions\n"
	cmdFuncs := strings.ReplaceAll(readSrcOrDefault("command_functions.sh", cmdFuncsDef), "%APP_NAME%", cfg.Name)
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

	// per-command functions
	if len(cfg.Commands) == 0 {
		// root (no-subcommand) mode — synthesize a Command to reuse parsers
		rootCmd := Command{
			Name:     "root",
			Args:     cfg.Args,
			Flags:    cfg.Flags,
			CatchAll: nil,
		}
		// generate flag parser named root_parse_flags
		if len(cfg.Flags) > 0 {
			fp := cfg.generateFlagParser(cfg.Name, rootCmd)
			// rename <funcBase>_parse_flags → root_parse_flags
			fp = strings.ReplaceAll(fp, "root_parse_flags()", "root_parse_flags()")
			cfg.writeMarker(f, "# :command.root.flags\n")
			write(f, fp+"\n")
		}
		// generate arg parser named root_parse_args
		if len(cfg.Args) > 0 {
			ap := generateArgParser(cfg.Name, rootCmd)
			cfg.writeMarker(f, "# :command.root.args\n")
			write(f, ap+"\n")
		}

		srcpath := filepath.Join("src", "root_command.sh")
		rawBody, readErr := os.ReadFile(srcpath)
		var rootBodyContent string
		if readErr != nil {
			rootBodyContent = "  # TODO: implement root command\n  return 0"
		} else {
			rootBodyContent = cfg.indentBody(strings.TrimRight(string(rawBody), "\n"))
		}

		invokes := ""
		if len(cfg.Flags) > 0 {
			invokes += "  root_parse_flags \"$@\"\n"
		}
		if len(cfg.Args) > 0 {
			invokes += "  root_parse_args \"$@\"\n"
		}
		if invokes == "" {
			invokes = fmt.Sprintf("  %s_usage\n  return 0", cfg.Name)
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
			gfp := cfg.generateFlagParser(cfg.Name, globalFlagCmd)
			cfg.writeMarker(f, "# :command.global.flags\n")
			write(f, gfp+"\n")
		}

		writeCommandFunctions(f, cfg, cfg.Commands, cfg.Name)
	}

	// before_hook — wraps src/before.sh body in before_hook() function (if present)
	beforeSrc := readSrcOrDefault("before.sh", "")
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
	afterSrc := readSrcOrDefault("after.sh", "")
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
	envCheck := generateEnvCheck(cfg.EnvironmentVariables)
	depsCheck := generateDepsCheck(cfg.Dependencies)
	setLine := "  set -e\n"
	if cfg.Strict {
		setLine = "  set -euo pipefail\n  IFS=$'\\n\\t'\n"
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
	initSrc := readSrcOrDefault("initialize.sh", "")
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
		runBuilder.WriteString(fmt.Sprintf("  %s_parse_flags \"$@\"\n", cfg.Name))
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

	for _, c := range cfg.Commands {
		funcName := c.Name
		if c.Function != "" {
			funcName = c.Function
		}
		runBuilder.WriteString(fmt.Sprintf("    %s) %s_command \"$@\" ;;\n", c.Name, funcName))
		for _, alias := range c.allAliases() {
			runBuilder.WriteString(fmt.Sprintf("    %s) %s_command \"$@\" ;;\n", alias, funcName))
		}
	}
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
		runBuilder.WriteString(fmt.Sprintf("        echo \"Unknown command: $_cmd\" >&2; %s_usage; exit 2\n", cfg.Name))
		runBuilder.WriteString("      fi ;;\n")
	} else if cfg.Extensible.Delegate() != "" {
		// extensible with delegate: forward unknown commands to the delegate tool
		runBuilder.WriteString("    *)\n")
		runBuilder.WriteString(fmt.Sprintf("      %s \"$_cmd\" \"$@\" ;;\n", cfg.Extensible.Delegate()))
	} else {
		runBuilder.WriteString(fmt.Sprintf("    *) echo \"Unknown command: $_cmd\" >&2; %s_usage; exit 2 ;;\n", cfg.Name))
	}
	runBuilder.WriteString("  esac\n")
	// after hook — call after_hook() if src/after.sh was present
	if hasAfterHook {
		runBuilder.WriteString("  after_hook\n")
	}
	runBuilder.WriteString("}\n")
	runDef := strings.ReplaceAll(readSrcOrDefault("run_wrapper.sh", runBuilder.String()), "%APP_NAME%", cfg.Name)
	cfg.writeMarker(f, "# :command.run\n")
	write(f, runDef+"\n\n")

	// start
	startDef := cfg.initFuncName() + "\n" + cfg.runFuncName() + " \"$@\"\n"
	start := strings.ReplaceAll(readSrcOrDefault("start.sh", startDef), "%APP_NAME%", cfg.Name)
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
	filename := fmt.Sprintf("src/%s_command.sh", command)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		content := fmt.Sprintf(
			"# Body of %s_command — do NOT add function header/footer.\n\n"+
				"# TODO: implement %s command\n"+
				"return 0\n",
			command, command,
		)
		_ = cfg.writeCommand("src/"+command+"_command.sh", content)
	}
}

func (cfg *ShellyCfg) writeCommand(filename, content string) error {
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

// runFormatter runs the named formatter on the generated script in-place.
// Supported formatters: "shfmt" (runs shfmt -w <path>).
// If the formatter binary is not found in PATH, a warning is printed and nil is returned.
func runFormatter(path, formatter string) error {
	switch formatter {
	case "shfmt":
		binPath, err := exec.LookPath("shfmt")
		if err != nil {
			fmt.Println("warning: shfmt not found in PATH, skipping formatter")
			return nil
		}
		cmd := exec.Command(binPath, "-w", path)
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			fmt.Printf("shfmt output:\n%s\n", string(out))
		}
		if err != nil {
			return fmt.Errorf("shfmt error: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown formatter %q (supported: shfmt)", formatter)
	}
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

	// for each command, check that needs/conflicts reference existing flags
	for _, c := range cfg.Commands {
		flagSet := map[string]bool{}
		for _, f := range c.Flags {
			flagSet[f.Long] = true
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
	}

	return errs
}
