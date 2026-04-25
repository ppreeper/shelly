package main

import (
	"fmt"
	"os"

	"github.com/ppreeper/shelly/internal/vcs"
	"github.com/ppreeper/shelly/templates"
	"github.com/spf13/cobra"
)

// Make version a variable (rather than a constant) and set its value to vcs.Version().
var (
	version = vcs.Version()
)

func main() {
	templates := templates.Files
	app := newApp("shelly", "sh cli generator", version, templates)
	rootCmd := &cobra.Command{
		Use:          app.Name,
		Short:        app.Usage,
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
			shellyCfg := readConfig()
			if shellyCfg == nil {
				fmt.Println("Error: could not read configuration file")
				return
			}

			if verbose {
				fmt.Println(prettyprint(shellyCfg))
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
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generate the bash script and required files",
		RunE: func(cmd *cobra.Command, args []string) error {
			shellyCfg := readConfig()
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
			// Generate main script
			if err := shellyCfg.shellGen(); err != nil {
				return fmt.Errorf("generate failed: %w", err)
			}

			// run shellcheck if requested
			if strict {
				if err := runShellcheck(fmt.Sprintf("./%s", shellyCfg.Name), strict); err != nil {
					return fmt.Errorf("shellcheck failed: %w", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&strict, "strict", "s", false, "Run shellcheck and fail on any diagnostics (requires shellcheck installed)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show the shelly configuration file prior to generating")
	cmd.Flags().StringVarP(&env, "env", "e", "", "Override generation environment (e.g. production)")
	return cmd
}

func (app *App) Preview() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Generate the bash script to STDOUT",
		RunE: func(cmd *cobra.Command, args []string) error {
			shellyCfg := readConfig()
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
	cmd := &cobra.Command{
		Use:   "add <addon>",
		Short: "Add a built-in addon to the project (validations, colors, hooks)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return addAddon(args[0], upgrade)
		},
	}
	cmd.Flags().BoolVarP(&upgrade, "upgrade", "u", false, "Overwrite existing addon files")
	return cmd
}
