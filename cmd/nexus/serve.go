package main

import (
	"fmt"
	"os"
	"time"

	"github.com/anurag/nexus/internal/analyzer"
	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/confluence"
	"github.com/anurag/nexus/internal/crawler"
	"github.com/anurag/nexus/internal/figma"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/mcp"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/internal/tools"
	"github.com/anurag/nexus/model"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server over stdio",
	Long:  "Launch JSON-RPC 2.0 MCP server on stdin/stdout. Nexus is a context engine — all tools are zero-LLM code intelligence. The AI agent does the thinking.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		s := store.NewStore(cfg.StateDir())
		services, err := s.LoadAllServices()
		if err != nil {
			fmt.Fprintf(os.Stderr, "nexus: warning: could not load services: %v\n", err)
			services = make(map[string]*model.ServiceIndex)
		}

		graph, err := s.LoadGraph()
		if err != nil {
			fmt.Fprintf(os.Stderr, "nexus: warning: could not load graph: %v\n", err)
			graph = &model.Graph{Version: 1}
		}

		fmt.Fprintf(os.Stderr, "nexus: loaded %d services, %d edges\n", len(services), len(graph.Edges))

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.TokenEnv)
		confClient := confluence.NewClient(cfg.Confluence.BaseURL, cfg.Confluence.TokenEnv)
		figmaClient := figma.NewClient(cfg.Figma.TokenEnv)

		ctx := &tools.Context{
			Store:      s,
			Services:   services,
			Graph:      graph,
			Jira:       jiraClient,
			Confluence: confClient,
			Figma:      figmaClient,
			AgentCfg:   cfg.Agent,
			Cfg:        cfg,
		}

		server := mcp.NewServer("nexus", version)

		// Code intelligence tools (all zero-LLM)
		server.Register(tools.NewSearchTool(ctx))
		server.Register(tools.NewGrepTool(ctx))
		server.Register(tools.NewFindEndpointsTool(ctx))
		server.Register(tools.NewReadFileTool(ctx))
		server.Register(tools.NewListServicesTool(ctx))
		server.Register(tools.NewServiceProfileTool(ctx))
		server.Register(tools.NewTraceDependenciesTool(ctx))
		server.Register(tools.NewTraceDependentsTool(ctx))
		server.Register(tools.NewImpactAnalysisTool(ctx))
		server.Register(tools.NewStatusTool(ctx))
		server.Register(tools.NewReviewPRContextTool(ctx))

		// Context tools (zero-LLM — Jira + code intel)
		server.Register(tools.NewContextTool(ctx))
		server.Register(tools.NewInvestigateTool(ctx))
		server.Register(tools.NewReindexTool(ctx))

		toolCount := 14
		if jiraClient.Available() {
			fmt.Fprintf(os.Stderr, "nexus: Jira connected\n")
		}
		fmt.Fprintf(os.Stderr, "nexus: MCP server ready (%d tools, zero LLM)\n", toolCount)

		go backgroundReindex(cfg, ctx)

		return server.Serve()
	},
}

func backgroundReindex(cfg *config.Config, ctx *tools.Context) {
	interval, err := time.ParseDuration(cfg.Schedule.CrawlInterval)
	if err != nil || interval <= 0 {
		interval = 48 * time.Hour
	}
	fmt.Fprintf(os.Stderr, "nexus: auto-reindex every %s\n", interval)

	// Check if index is stale on startup — covers server restarts
	if health, hErr := ctx.Store.LoadHealth(); hErr == nil {
		age := time.Since(health.Timestamp)
		if health.Timestamp.IsZero() || age > interval {
			fmt.Fprintf(os.Stderr, "nexus: index is stale (last crawl: %s ago), reindexing now...\n",
				age.Round(time.Minute))
			runReindex(cfg, ctx)
		} else {
			fmt.Fprintf(os.Stderr, "nexus: index is fresh (last crawl: %s ago)\n",
				age.Round(time.Minute))
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		runReindex(cfg, ctx)
	}
}

func runReindex(cfg *config.Config, ctx *tools.Context) {
	fmt.Fprintf(os.Stderr, "nexus: auto-reindex starting...\n")
	start := time.Now()

	c := crawler.NewCrawler(cfg, ctx.Store)
	result, crawlErr := c.Run(false)
	if crawlErr != nil {
		fmt.Fprintf(os.Stderr, "nexus: auto-reindex crawl failed: %v\n", crawlErr)
		return
	}
	fmt.Fprintf(os.Stderr, "nexus: auto-reindex crawled %d services\n", result.Indexed)

	services, loadErr := ctx.Store.LoadAllServices()
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "nexus: auto-reindex load failed: %v\n", loadErr)
		return
	}

	a := analyzer.NewAnalyzer(services)
	graph := a.BuildGraph()
	if saveErr := ctx.Store.SaveGraph(graph); saveErr != nil {
		fmt.Fprintf(os.Stderr, "nexus: auto-reindex save graph failed: %v\n", saveErr)
		return
	}

	ctx.SwapIndex(services, graph)
	fmt.Fprintf(os.Stderr, "nexus: auto-reindex done — %d services, %d edges (%s)\n",
		len(services), len(graph.Edges), time.Since(start).Round(time.Millisecond))
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
