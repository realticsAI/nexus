package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Run full crawl → analyze → serve pipeline",
	Long:  "Execute all three stages in sequence: crawl repos, analyze and build graph, then start the MCP server.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("pipeline: running crawl → analyze → serve")
		if err := crawlCmd.RunE(crawlCmd, nil); err != nil {
			return fmt.Errorf("crawl stage: %w", err)
		}
		if err := analyzeCmd.RunE(analyzeCmd, nil); err != nil {
			return fmt.Errorf("analyze stage: %w", err)
		}
		if err := serveCmd.RunE(serveCmd, nil); err != nil {
			return fmt.Errorf("serve stage: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pipelineCmd)
}
