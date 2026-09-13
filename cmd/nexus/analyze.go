package main

import (
	"fmt"

	"github.com/anurag/nexus/internal/analyzer"
	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/store"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Build dependency graph and embeddings",
	Long:  "Read per-service index shards, resolve cross-service edges (HTTP, Kafka, build deps), generate keyword index, and write graph.json.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		s := store.NewStore(cfg.StateDir())
		services, err := s.LoadAllServices()
		if err != nil {
			return fmt.Errorf("loading services: %w", err)
		}
		if len(services) == 0 {
			fmt.Println("analyze: no services in store — run 'nexus crawl' first")
			return nil
		}

		fmt.Printf("analyze: building graph from %d service(s)...\n", len(services))
		a := analyzer.NewAnalyzer(services)
		graph := a.BuildGraph()

		if err := s.SaveGraph(graph); err != nil {
			return fmt.Errorf("saving graph: %w", err)
		}

		fmt.Printf("analyze: %d nodes, %d edges, %d keywords\n",
			len(graph.Nodes), len(graph.Edges), len(graph.KeywordIndex))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}
