package main

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed embed/README.md
var embeddedReadme string

var guideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Show the full setup and usage guide",
	Long:  "Prints the embedded README — no internet or source code needed.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(embeddedReadme)
	},
}

func init() {
	rootCmd.AddCommand(guideCmd)
}
