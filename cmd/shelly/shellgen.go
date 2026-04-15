package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func write(f *os.File, content string) {
	_, err := f.WriteString(content)
	if err != nil {
		fmt.Println("Error writing to shell script file:", err)
		os.Exit(1)
	}
}

func (cfg *ShellyCfg) shellGenCommands() {
	// if no commands then write root command
	if len(cfg.Commands) == 0 {
		cfg.commandFunc("root")
		return
	}
	for _, command := range cfg.Commands {
		cfg.commandFunc(command.Name)
	}
}

// readSrcOrDefault reads a file under ./src/ if it exists, otherwise returns def
func readSrcOrDefault(name, def string) string {
	p := filepath.Join("src", name)
	b, err := os.ReadFile(p)
	if err != nil {
		return def
	}
	return string(b)
}

func (cfg *ShellyCfg) shellGen() {
	// compile the final script
	var permissions os.FileMode = 0o755
	filename := fmt.Sprintf("./%s", cfg.Name)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
	if err != nil {
		fmt.Println("Error creating shell script file:", err)
		return
	}
	defer f.Close()

	// write shebang
	write(f, cfg.shebang()+"\n\n")

	// version command
	versionDef := "version_command() {\n\techo \"$version\"\n}\n"
	write(f, "# :command.version_command\n")
	write(f, readSrcOrDefault("version_command.sh", versionDef)+"\n\n")

	// usage
	usageDef := fmt.Sprintf("%s_usage() {\n\tcat <<'USAGE'\nUsage: %s [COMMAND] [OPTIONS]\nUSAGE\n}\n", cfg.Name, cfg.Name)
	usage := strings.ReplaceAll(readSrcOrDefault("usage.sh", usageDef), "%APP_NAME%", cfg.Name)
	write(f, "# :command.usage\n")
	write(f, usage+"\n\n")

	// normalize input
	normalizeDef := "normalize_input() {\n  newargs=\"\"\n  for arg in \"$@\"; do\n    case \"$arg\" in\n      --*=*) newargs=\"$newargs ${arg%%=*} ${arg#*=}\" ;;\n      *) newargs=\"$newargs $arg\" ;;\n    esac\n  done\n  set -- $newargs\n}\n"
	normalize := strings.ReplaceAll(readSrcOrDefault("normalize_input.sh", normalizeDef), "%APP_NAME%", cfg.Name)
	write(f, "# :command.normalize_input\n")
	write(f, normalize+"\n\n")

	// inspect args
	inspectDef := "inspect_args() {\n  # Validate positional args and flags\n  return 0\n}\n"
	inspect := strings.ReplaceAll(readSrcOrDefault("inspect_args.sh", inspectDef), "%APP_NAME%", cfg.Name)
	write(f, "# :command.inspect_args\n")
	write(f, inspect+"\n\n")

	// parse requirements
	parseDef := "parse_requirements() {\n  # Check environment, dependencies\n  return 0\n}\n"
	parse := strings.ReplaceAll(readSrcOrDefault("parse_requirements.sh", parseDef), "%APP_NAME%", cfg.Name)
	write(f, "# :command.parse_requirements\n")
	write(f, parse+"\n\n")

	// command helper functions
	cmdFuncsDef := "# helper functions\n"
	cmdFuncs := strings.ReplaceAll(readSrcOrDefault("command_functions.sh", cmdFuncsDef), "%APP_NAME%", cfg.Name)
	write(f, "# :command.command_functions\n")
	write(f, cmdFuncs+"\n\n")

	// per-command functions
	if len(cfg.Commands) == 0 {
		// if no commands, write root command function
		rootDef := fmt.Sprintf("root() {\n  # default root command\n  %s_usage\n}\n", cfg.Name)
		write(f, "# :command.root\n")
		write(f, rootDef+"\n\n")
	} else {
		for _, cmd := range cfg.Commands {
			srcpath := filepath.Join("src", fmt.Sprintf("%s_command.sh", cmd.Name))
			b, err := os.ReadFile(srcpath)
			var content string
			if err != nil {
				content = fmt.Sprintf("# This file is located at 'src/%s_command.sh'.\n# Implementation for the '%s' command.\n# The code you write here will be included into the generated script.\n\n%s_command() {\n  # TODO: implement %s command\n  inspect_args\n  parse_requirements\n  return 0\n}\n", cmd.Name, cmd.Name, cmd.Name, cmd.Name)
				// write skeleton file so user can edit it later
				_ = cfg.writeCommand(fmt.Sprintf("src/%s_command.sh", cmd.Name), content)
			} else {
				content = string(b)
			}
			content = strings.ReplaceAll(content, "%APP_NAME%", cfg.Name)
			write(f, "# :command.function "+cmd.Name+"\n")
			write(f, content+"\n\n")
		}
	}

	// run function: dispatcher
	var runBuilder strings.Builder
	runBuilder.WriteString("run() {\n")
	runBuilder.WriteString("  normalize_input \"$@\"\n")
	runBuilder.WriteString("  if [ $# -eq 0 ]; then\n")
	runBuilder.WriteString(fmt.Sprintf("    %s_usage\n    return 0\n", cfg.Name))
	runBuilder.WriteString("  fi\n")
	runBuilder.WriteString("  cmd=$1; shift\n")
	runBuilder.WriteString("  case \"$cmd\" in\n")
	for _, c := range cfg.Commands {
		runBuilder.WriteString(fmt.Sprintf("    %s) %s_command \"$@\" ;;\n", c.Name, c.Name))
		if c.Alias != "" {
			runBuilder.WriteString(fmt.Sprintf("    %s) %s_command \"$@\" ;;\n", c.Alias, c.Name))
		}
	}
	runBuilder.WriteString("    -h|--help) " + cfg.Name + "_usage ;;\n")
	runBuilder.WriteString("    *) echo \"Unknown command: $cmd\"; " + cfg.Name + "_usage; exit 2 ;;\n")
	runBuilder.WriteString("  esac\n")
	runBuilder.WriteString("}\n")
	runDef := strings.ReplaceAll(readSrcOrDefault("run_wrapper.sh", runBuilder.String()), "%APP_NAME%", cfg.Name)
	write(f, "# :command.run\n")
	write(f, runDef+"\n\n")

	// start wrapper
	startDef := "case \"$0\" in\n\t*/*) script_name=${0##*/} ;;\n\t*) script_name=$0 ;;\n esac\n\nif [ \"$script_name\" = \"$(basename \"$0\")\" ]; then\n\tcommand_line_args=\"$@\"\n\tinitialize\n\trun \"$@\"\nfi\n"
	start := strings.ReplaceAll(readSrcOrDefault("start.sh", startDef), "%APP_NAME%", cfg.Name)
	write(f, "# :command.start\n")
	write(f, start+"\n\n")

	fmt.Printf("created ./%s\n", cfg.Name)
	fmt.Printf("run ./%s --help to test your script\n", cfg.Name)
}

func (cfg *ShellyCfg) shebang() string {
	return "#!/usr/bin/env sh\n# This script was generated by shelly\n# Modifying it manually is not recommended"
}

func (cfg *ShellyCfg) commandFunc(command string) {
	filecontent := fmt.Sprintf("# This file is located at 'src/%s_command.sh'.\n# It contains the implementation for the '%s' command.\n# The code you write here will be included into the generated script.\n\n%s_command() {\n  # parse command-specific flags and args\n  inspect_args\n  parse_requirements\n  return 0\n}\n", command, command, command)
	filename := fmt.Sprintf("src/%s_command.sh", command)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		_ = cfg.writeCommand("src/"+command+"_command.sh", filecontent)
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
