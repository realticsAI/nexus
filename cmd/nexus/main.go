package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "nexus",
	Short: "Code-intelligence + autonomous SDLC engine",
	Long:  "Nexus indexes multi-language repos into a searchable graph, serves that intelligence via MCP, and autonomously implements Jira tickets.",
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(fmt.Sprintf("nexus %s\n", version))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
