package main

import (
	"fmt"
	"os"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/crawler"
	"github.com/anurag/nexus/internal/store"
	"github.com/spf13/cobra"
)

var crawlFull bool

var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Discover and parse repositories",
	Long:  "Walk configured workspaces, discover git repos, pull latest changes, and parse source files into per-service JSON index shards.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		workspaces := cfg.ExpandedWorkspaces()
		if len(workspaces) == 0 {
			fmt.Fprintln(os.Stderr, "no workspaces configured — add entries to ~/.nexus/config.yml")
			return nil
		}

		s := store.NewStore(cfg.StateDir())
		c := crawler.NewCrawler(cfg, s)

		result, err := c.Run(crawlFull)
		if err != nil {
			return err
		}
		fmt.Println(result)
		for _, f := range result.Failed {
			fmt.Fprintf(os.Stderr, "  FAILED: %s — %s\n", f.Name, f.Reason)
		}
		return nil
	},
}

var summarizeCmd = &cobra.Command{
	Use:   "summarize",
	Short: "Generate lightweight service summaries for all indexed services",
	Long:  "Reads each service index and generates a summary file (~1KB each). Summaries are used by the agent to avoid loading full indexes into memory.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		s := store.NewStore(cfg.StateDir())

		force, _ := cmd.Flags().GetBool("force")
		count, err := s.RegenerateSummaries(force)
		if err != nil {
			return err
		}
		if force {
			fmt.Fprintf(os.Stderr, "Regenerated %d summaries\n", count)
		} else {
			fmt.Fprintf(os.Stderr, "Backfilled %d missing summaries\n", count)
		}
		return nil
	},
}

func init() {
	crawlCmd.Flags().BoolVar(&crawlFull, "full", false, "Force full re-parse, ignoring checksums")
	summarizeCmd.Flags().Bool("force", false, "Regenerate all summaries, not just missing ones")
	rootCmd.AddCommand(crawlCmd)
	rootCmd.AddCommand(summarizeCmd)
}
