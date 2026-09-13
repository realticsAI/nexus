package tools

import (
	"encoding/json"
	"fmt"

	"github.com/anurag/nexus/internal/agent"
	"github.com/anurag/nexus/internal/mcp"
)

// ContextTool — lean context bundle: Jira + code intelligence, zero LLM
type ContextTool struct{ ctx *Context }

func NewContextTool(ctx *Context) *ContextTool { return &ContextTool{ctx: ctx} }
func (t *ContextTool) Name() string            { return "context" }
func (t *ContextTool) Description() string {
	return "Generate a context bundle for a Jira ticket. Fetches ticket data and runs code intelligence (service matching, endpoints, call chains, file signatures, library APIs). Zero LLM calls. The AI agent reads this and does its own requirements extraction, planning, and implementation."
}
func (t *ContextTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket": map[string]any{"type": "string", "description": "Jira ticket key (e.g. PROJ-1234)"},
		},
		"required": []string{"ticket"},
	}
}
func (t *ContextTool) Run(args map[string]any) *mcp.ToolCallResult {
	ticket, _ := args["ticket"].(string)
	if ticket == "" {
		return mcp.ErrorResult("ticket is required")
	}
	if t.ctx.Jira == nil || !t.ctx.Jira.Available() {
		return mcp.ErrorResult("Jira not configured")
	}

	issue, err := t.ctx.Jira.GetIssue(ticket)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("fetch ticket: %v", err))
	}

	ticketType := agent.Classify(issue)
	investigation := agent.Investigate(issue, t.ctx.Services, t.ctx.Graph, t.ctx.AgentCfg.GitHubOrg, nil)

	// Resolve repo paths
	if len(investigation.Services) > 0 {
		investigation.ServicePaths = make(map[string]string)
		for _, svcKey := range investigation.Services {
			if svc, ok := t.ctx.Services[svcKey]; ok {
				investigation.ServicePaths[svcKey] = svc.Path
			}
		}
		if investigation.RepoPath == "" {
			if path, ok := investigation.ServicePaths[investigation.Services[0]]; ok {
				investigation.RepoPath = path
			}
		}
	}

	// Linked issues — fetch summary + comments
	if len(issue.LinkedIssues) > 0 {
		for _, linkedKey := range issue.LinkedIssues {
			linkedIssue, lErr := t.ctx.Jira.GetIssue(linkedKey)
			if lErr != nil {
				continue
			}
			li := agent.LinkedIssueSummary{
				Key:     linkedIssue.Key,
				Summary: linkedIssue.Summary,
				Status:  linkedIssue.Status,
				Type:    linkedIssue.Type,
			}
			if comments, cErr := t.ctx.Jira.GetComments(linkedKey); cErr == nil && len(comments) > 0 {
				limit := len(comments)
				if limit > 5 {
					limit = 5
				}
				li.Comments = comments[:limit]
			}
			investigation.LinkedIssues = append(investigation.LinkedIssues, li)
		}
	}

	// Jira comments — PO clarifications, scope changes, technical notes
	if comments, cErr := t.ctx.Jira.GetComments(ticket); cErr == nil {
		issue.Comments = comments
	}

	prompt := agent.FormatContext(ticket, ticketType, issue, investigation)
	return mcp.TextResult(prompt)
}

// InvestigateTool — code scan only, no LLM, raw JSON output
type InvestigateTool struct{ ctx *Context }

func NewInvestigateTool(ctx *Context) *InvestigateTool { return &InvestigateTool{ctx: ctx} }
func (t *InvestigateTool) Name() string                { return "investigate" }
func (t *InvestigateTool) Description() string {
	return "Investigate a Jira ticket against the codebase index. Returns matched services, endpoints, facades, call chains, file signatures, and patterns as raw JSON. No LLM call — pure code intelligence."
}
func (t *InvestigateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket": map[string]any{"type": "string", "description": "Jira ticket key (e.g. PROJ-1001)"},
		},
		"required": []string{"ticket"},
	}
}
func (t *InvestigateTool) Run(args map[string]any) *mcp.ToolCallResult {
	ticket, _ := args["ticket"].(string)
	if ticket == "" {
		return mcp.ErrorResult("ticket is required")
	}
	if t.ctx.Jira == nil || !t.ctx.Jira.Available() {
		return mcp.ErrorResult("Jira not configured")
	}

	issue, err := t.ctx.Jira.GetIssue(ticket)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("fetch ticket: %v", err))
	}

	investigation := agent.Investigate(issue, t.ctx.Services, t.ctx.Graph, t.ctx.AgentCfg.GitHubOrg, nil)

	data, _ := json.MarshalIndent(investigation, "", "  ")
	return mcp.TextResult(string(data))
}
