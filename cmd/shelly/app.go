package main

import (
	"embed"
	"fmt"
	"os"

	gocy "github.com/goccy/go-yaml"
)

type App struct {
	Name    string
	Usage   string
	Version string
	EmbedFS embed.FS
}

func newApp(name, usage, version string, embedFS embed.FS) *App {
	return &App{
		Name:    name,
		Usage:   usage,
		Version: version,
		EmbedFS: embedFS,
	}
}

func isDirectoryEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func prettyprint(i any) string {
	ymlData, err := gocy.MarshalWithOptions(i, gocy.Indent(2), gocy.IndentSequence(true))
	if err != nil {
		fmt.Println("Error marshaling to YAML:", err)
		return ""
	}
	return string(ymlData)
}
