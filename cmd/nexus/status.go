package main

import (
	"fmt"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index health and stats",
	Long:  "Display current crawl health, manifest stats, and index age.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		s := store.NewStore(cfg.StateDir())
		m, err := s.LoadManifest()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		h, err := s.LoadHealth()
		if err != nil {
			return fmt.Errorf("loading health: %w", err)
		}
		fmt.Printf("nexus status\n")
		fmt.Printf("  state dir:  %s\n", s.Dir())
		fmt.Printf("  services:   %d\n", len(m.Services))
		fmt.Printf("  manifest v: %d\n", m.Version)
		if h.TotalRepos > 0 {
			fmt.Printf("  last crawl: %d repos, %d indexed, %d failed (%s)\n",
				h.TotalRepos, h.Indexed, len(h.Failed), h.CrawlDuration)
		} else {
			fmt.Printf("  last crawl: none\n")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
