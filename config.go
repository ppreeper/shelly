package main

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type EnvironmentVariable struct {
	Name string `json:"name" yaml:"name"`
	Help string `json:"help,omitempty" yaml:"help,omitempty"`
}

type Flag struct {
	Long  string `json:"long" yaml:"long"`
	Short string `json:"short" yaml:"short"`
	Help  string `json:"help,omitempty" yaml:"help,omitempty"`
}

type Arg struct {
	Name     string `json:"name" yaml:"name"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Help     string `json:"help,omitempty" yaml:"help,omitempty"`
}

type Command struct {
	Name                 string                `json:"name" yaml:"name"`
	Alias                string                `json:"alias" yaml:"alias"`
	Help                 string                `json:"help" yaml:"help"`
	Args                 []Arg                 `json:"args,omitempty" yaml:"args,omitempty"`
	Flags                []Flag                `json:"flags,omitempty" yaml:"flags,omitempty"`
	Examples             []string              `json:"examples,omitempty" yaml:"examples,omitempty"`
	EnvironmentVariables []EnvironmentVariable `json:"environment_variables,omitempty" yaml:"environment_variables,omitempty"`
}

type ShellyCfg struct {
	Name                 string                `yaml:"name"`
	Help                 string                `yaml:"help"`
	Version              string                `yaml:"version"`
	EnvironmentVariables []EnvironmentVariable `json:"environment_variables,omitempty" yaml:"environment_variables,omitempty"`
	Commands             []Command             `json:"commands,omitempty" yaml:"commands,omitempty"`
	Args                 []Arg                 `json:"args,omitempty" yaml:"args,omitempty"`
	Flags                []Flag                `json:"flags,omitempty" yaml:"flags,omitempty"`
	Examples             []string              `json:"examples,omitempty" yaml:"examples,omitempty"`
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
	// Check if src directory exists and is empty
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get current working directory: %w", err)
	}
	srcDir := filepath.Join(cwd, "src")
	empty, err := isDirectoryEmpty(srcDir)
	if err != nil {
		return fmt.Errorf("could not check if directory is empty: %w", err)
	}
	if !empty {
		return fmt.Errorf("directory src already exists and is not empty")
	}

	// Create src directory if it doesn't exist
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		os.MkdirAll(srcDir, 0o755)
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
