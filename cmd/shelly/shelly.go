package main

import (
	"fmt"
	"os"

	"github.com/ppreeper/shelly/internal/vcs"
	"github.com/ppreeper/shelly/templates"
	"github.com/spf13/cobra"
)

// version is overridden at build time with:
//
//	go build -ldflags "-X main.version=v1.2.3"
//
// When it is left at its default (or built without ldflags) the version
// recorded in the module build info is used instead.
var version = "dev"

// resolveVersion returns the ldflags-provided version when set, otherwise the
// version embedded in the module build info (e.g. `go install ...@latest`).
func resolveVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if v := vcs.Version(); v != "" && v != "(devel)" {
		return v
	}
	return version
}

func main() {
	templates := templates.Files
	app := newApp("shelly", "sh cli generator", resolveVersion(), templates)
	rootCmd := &cobra.Command{
		Use:          app.Name,
		Short:        app.Usage,
		Version:      app.Version,
		SilenceUsage: false,
	}

	rootCmd.AddCommand(app.Init())
	rootCmd.AddCommand(app.Preview())
	rootCmd.AddCommand(app.Validate())
	rootCmd.AddCommand(app.Generate())
	rootCmd.AddCommand(app.Add())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error executing command:", err)
		os.Exit(1)
	}
}

func (app *App) Init() *cobra.Command {
	var minimal bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := writeConfig(app.EmbedFS, minimal)
			if err != nil {
				return err
			}
			fmt.Printf("run %s generate to create the bash script\n", app.Name)
			return nil
		},
	}
	cmd.PersistentFlags().BoolVarP(&minimal, "minimal", "m", false, "Use a minimal configuration file (without commands)")
	return cmd
}

func (app *App) Validate() *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Scan the configuration file for errors",
		Run: func(cmd *cobra.Command, args []string) {
			shellyCfg, err := readConfigE()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error: "+err.Error())
				os.Exit(1)
			}
			if shellyCfg == nil {
				fmt.Println("Error: could not read configuration file")
				return
			}

			if verbose {
				fmt.Println(prettyprint(shellyCfg))
			}

			if errs := shellyCfg.validateSemantics(); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, "error: "+e)
				}
				os.Exit(1)
			}

			fmt.Println("File read OK")
		},
	}
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show the shelly configuration file prior to validating. This is useful when using split config (import) since it will show the final compiled configuration.")
	return cmd
}

func (app *App) Generate() *cobra.Command {
	var strict bool
	var verbose bool
	var env string
	var upgrade bool
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generate the bash script and required files",
		RunE: func(cmd *cobra.Command, args []string) error {
			shellyCfg, err := readConfigE()
			if err != nil {
				return fmt.Errorf("could not read configuration: %w", err)
			}
			if shellyCfg == nil {
				return fmt.Errorf("Error: could not read configuration file")
			}
			if verbose {
				fmt.Println(prettyprint(shellyCfg))
			}
			// --env flag overrides config file value
			if env != "" {
				shellyCfg.Env = env
			}
			// Generate commands
			shellyCfg.shellGenCommands()
			// Refresh marked library files before writing the master script
			if upgrade {
				if err := upgradeLibs(shellyCfg); err != nil {
					return fmt.Errorf("upgrade failed: %w", err)
				}
			}
			// Generate main script
			if err := shellyCfg.shellGen(); err != nil {
				return fmt.Errorf("generate failed: %w", err)
			}

			// run shellcheck if requested
			if strict {
				if err := runShellcheck(shellyCfg.outputPath(), strict); err != nil {
					return fmt.Errorf("shellcheck failed: %w", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&strict, "strict", "s", false, "Run shellcheck and fail on any diagnostics (requires shellcheck installed)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show the shelly configuration file prior to generating")
	cmd.Flags().StringVarP(&env, "env", "e", "", "Override generation environment (e.g. production)")
	cmd.Flags().BoolVarP(&upgrade, "upgrade", "u", false, "Refresh library files marked with [@shelly-upgrade] before generating")
	return cmd
}

func (app *App) Preview() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Generate the bash script to STDOUT",
		RunE: func(cmd *cobra.Command, args []string) error {
			shellyCfg, err := readConfigE()
			if err != nil {
				return fmt.Errorf("could not read configuration: %w", err)
			}
			if shellyCfg == nil {
				return fmt.Errorf("could not read configuration file")
			}
			out, err := shellyCfg.shellGenToString()
			if err != nil {
				return fmt.Errorf("preview failed: %w", err)
			}
			fmt.Print(out)
			return nil
		},
	}
	return cmd
}

func (app *App) Add() *cobra.Command {
	var upgrade bool
	var source string
	var list bool
	var quiet bool
	cmd := &cobra.Command{
		Use:   "add [library]",
		Short: "Add a built-in addon, or install a library from a source",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if source != "" {
				if list {
					return listLibraries(source)
				}
				if len(args) == 0 {
					return fmt.Errorf("add: library name required (or use --list)")
				}
				return addFromSource(source, args[0], upgrade, quiet)
			}
			if list {
				listBuiltins()
				return nil
			}
			if len(args) == 0 {
				return fmt.Errorf("add: addon name required")
			}
			return addAddon(args[0], upgrade)
		},
	}
	cmd.Flags().BoolVarP(&upgrade, "upgrade", "u", false, "Overwrite existing files")
	cmd.Flags().StringVarP(&source, "source", "s", "", "Install from a libraries source directory (containing libraries.yml)")
	cmd.Flags().BoolVarP(&list, "list", "l", false, "List available libraries")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Do not show post-install messages")
	return cmd
}
