package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anurag/nexus/internal/agent"
	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/confluence"
	"github.com/anurag/nexus/internal/figma"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
	"github.com/spf13/cobra"
)

// nexus context PROJ-1003
// Lean context bundle — Jira + code intelligence, zero LLM calls
var contextCmd = &cobra.Command{
	Use:   "context [ticket-id]",
	Short: "Generate context bundle for AI agents (zero LLM, instant)",
	Long: `Fetches Jira ticket and runs code intelligence scan. Outputs a structured
context bundle with file lists, patterns, call chains, endpoints, and library APIs.
No LLM calls — no requirements extraction, no planning, no validation.

Nexus is a context engine. Your AI agent does the thinking.

  Print:  nexus context PROJ-1234
  Save:   nexus context PROJ-1234  (auto-saves to ~/.nexus/handoffs/)
  Skill:  /nexus-implement PROJ-1234  (in your AI agent)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		s := store.NewStore(cfg.StateDir())

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.TokenEnv)
		if !jiraClient.Available() {
			return fmt.Errorf("Jira not configured — set %s env var", cfg.Jira.TokenEnv)
		}

		confClient := confluence.NewClient(cfg.Confluence.BaseURL, cfg.Confluence.TokenEnv)
		figmaClient := figma.NewClient(cfg.Figma.TokenEnv)

		fmt.Fprintf(os.Stderr, "nexus context: fetching %s...\n", ticketID)
		issue, err := jiraClient.GetIssue(ticketID)
		if err != nil {
			return fmt.Errorf("fetch ticket: %w", err)
		}

		ticketType := agent.Classify(issue)
		fmt.Fprintf(os.Stderr, "nexus context: %s [%s] — %s\n", ticketID, ticketType, issue.Summary)

		summaries, err := s.LoadServiceSummaries()
		if err != nil || len(summaries) == 0 {
			summaries = map[string]*model.ServiceSummary{}
			if n, bErr := s.BackfillSummaries(); bErr == nil && n > 0 {
				summaries, _ = s.LoadServiceSummaries()
			}
		}

		loadedServices := make(map[string]*model.ServiceIndex)
		matchedKeys := agent.ResolveServiceKeys(issue, summaries)
		if len(matchedKeys) > 5 {
			matchedKeys = matchedKeys[:5]
		}
		for _, key := range matchedKeys {
			svc, loadErr := s.LoadService(key)
			if loadErr != nil {
				continue
			}
			loadedServices[key] = svc
		}

		graph, _ := s.LoadGraph()
		if graph == nil {
			graph = &model.Graph{}
		}

		var allServiceKeys []string
		for key := range summaries {
			allServiceKeys = append(allServiceKeys, key)
		}

		fmt.Fprintf(os.Stderr, "nexus context: scanning %d services...\n", len(loadedServices))
		investigation := agent.Investigate(issue, loadedServices, graph, cfg.Agent.GitHubOrg, allServiceKeys)

		// Resolve repo paths
		if len(investigation.Services) > 0 {
			investigation.ServicePaths = make(map[string]string)
			for _, svcKey := range investigation.Services {
				if svc, ok := loadedServices[svcKey]; ok {
					investigation.ServicePaths[svcKey] = svc.Path
				}
			}
			if investigation.RepoPath == "" {
				if path, ok := investigation.ServicePaths[investigation.Services[0]]; ok {
					investigation.RepoPath = path
				}
			}
		}

		// Figma (if available)
		if figmaClient.Available() {
			figmaLinks := figma.ExtractFigmaLinks(issue.AllText())
			for _, link := range figmaLinks {
				fileKey, nodeID := figma.ParseFigmaURL(link)
				if fileKey == "" {
					continue
				}
				ref := agent.FigmaDesignRef{URL: link, FileKey: fileKey, NodeID: nodeID}
				if nodeID != "" {
					nodes, err := figmaClient.GetFileNodes(fileKey, []string{nodeID})
					if err == nil {
						if node, ok := nodes[nodeID]; ok && node != nil {
							ref.FileName = node.Name
							ref.UIText = figma.ExtractAllText(node, 10)
							ref.ScreenStates = figma.ExtractFrameNames(node)
							ref.Frames = ref.ScreenStates
							ref.ScreenAnalysis = figma.AnalyzeAllScreens(node)
						}
					}
				} else {
					file, err := figmaClient.GetFile(fileKey)
					if err == nil {
						ref.FileName = file.Name
						if file.Document != nil {
							ref.ScreenAnalysis = figma.AnalyzeAllScreens(file.Document)
						}
					}
				}
				investigation.FigmaDesigns = append(investigation.FigmaDesigns, ref)
			}
		}
		_ = confClient

		// Linked issues — fetch summary + comments (PO clarifications, implementation notes)
		if len(issue.LinkedIssues) > 0 {
			fmt.Fprintf(os.Stderr, "nexus context: fetching %d linked issues...\n", len(issue.LinkedIssues))
			for _, linkedKey := range issue.LinkedIssues {
				linkedIssue, err := jiraClient.GetIssue(linkedKey)
				if err != nil {
					continue
				}
				li := agent.LinkedIssueSummary{
					Key:     linkedIssue.Key,
					Summary: linkedIssue.Summary,
					Status:  linkedIssue.Status,
					Type:    linkedIssue.Type,
				}
				if comments, cErr := jiraClient.GetComments(linkedKey); cErr == nil && len(comments) > 0 {
					limit := len(comments)
					if limit > 5 {
						limit = 5
					}
					li.Comments = comments[:limit]
				}
				investigation.LinkedIssues = append(investigation.LinkedIssues, li)
			}
		}

		// Jira comments
		if comments, err := jiraClient.GetComments(ticketID); err == nil && len(comments) > 0 {
			fmt.Fprintf(os.Stderr, "nexus context: fetched %d comments\n", len(comments))
			issue.Comments = comments
		}

		prompt := agent.FormatContext(ticketID, ticketType, issue, investigation)

		// Always save
		handoffDir := filepath.Join(os.Getenv("HOME"), ".nexus", "handoffs")
		os.MkdirAll(handoffDir, 0755)
		handoffFile := filepath.Join(handoffDir, ticketID+".md")
		if err := os.WriteFile(handoffFile, []byte(prompt), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "nexus: warning: could not save to %s: %v\n", handoffFile, err)
		} else {
			fmt.Fprintf(os.Stderr, "nexus context: saved to %s\n", handoffFile)
		}

		libClasses := 0
		for _, lib := range investigation.LibraryAPIs {
			libClasses += len(lib.Classes)
		}
		fmt.Fprintf(os.Stderr, "nexus context: %d services, %d classes, %d endpoints, %d lib classes, 0 LLM calls\n",
			len(investigation.Services), len(investigation.RelatedClasses),
			len(investigation.ExistingEndpoints), libClasses)

		fmt.Print(prompt)
		return nil
	},
}

// nexus investigate PROJ-1003
// Code-intelligence scan only — raw JSON output, no LLM
var investigateCmd = &cobra.Command{
	Use:   "investigate [ticket-id]",
	Short: "Scan codebase for ticket context (raw JSON, no LLM)",
	Long:  `Pure code intelligence — matches services, endpoints, facades, call chains. No LLM call. Outputs raw JSON (use 'context' for human-readable markdown).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		s := store.NewStore(cfg.StateDir())

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.TokenEnv)
		if !jiraClient.Available() {
			return fmt.Errorf("Jira not configured — set %s env var", cfg.Jira.TokenEnv)
		}

		issue, err := jiraClient.GetIssue(ticketID)
		if err != nil {
			return fmt.Errorf("fetch ticket: %w", err)
		}

		summaries, err := s.LoadServiceSummaries()
		if err != nil || len(summaries) == 0 {
			summaries = map[string]*model.ServiceSummary{}
			if n, bErr := s.BackfillSummaries(); bErr == nil && n > 0 {
				summaries, _ = s.LoadServiceSummaries()
			}
		}

		loadedServices := make(map[string]*model.ServiceIndex)
		matchedKeys := agent.ResolveServiceKeys(issue, summaries)
		if len(matchedKeys) > 5 {
			matchedKeys = matchedKeys[:5]
		}
		for _, key := range matchedKeys {
			svc, loadErr := s.LoadService(key)
			if loadErr != nil {
				continue
			}
			loadedServices[key] = svc
		}

		graph, _ := s.LoadGraph()
		if graph == nil {
			graph = &model.Graph{}
		}

		var allServiceKeys []string
		for key := range summaries {
			allServiceKeys = append(allServiceKeys, key)
		}

		investigation := agent.Investigate(issue, loadedServices, graph, cfg.Agent.GitHubOrg, allServiceKeys)

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(investigation)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(investigateCmd)
}
