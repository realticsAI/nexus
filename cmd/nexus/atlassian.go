package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/confluence"
	"github.com/anurag/nexus/internal/jira"
	"github.com/spf13/cobra"
)

var jiraCmd = &cobra.Command{
	Use:   "jira [ticket-id]",
	Short: "Read full Jira ticket with comments and linked issues",
	Long: `Fetches everything from a Jira ticket via REST API:
  issue details, all comments, linked issue summaries, remote links.

  Read:     nexus jira PROJ-1002
  Comment:  nexus jira PROJ-1002 --comment "TTL filter fix implemented"

Output is printed to the terminal — visible to both you and the AI agent.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := strings.ToUpper(args[0])

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		client := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.TokenEnv)
		if !client.Available() {
			return fmt.Errorf("Jira not configured — set jira.base_url and %s env var", cfg.Jira.TokenEnv)
		}

		commentText, _ := cmd.Flags().GetString("comment")
		if commentText != "" {
			fmt.Fprintf(os.Stderr, "nexus: posting comment to %s (%d chars)...\n", ticketID, len(commentText))
			if err := client.AddComment(ticketID, commentText); err != nil {
				return fmt.Errorf("add comment: %w", err)
			}
			fmt.Fprintf(os.Stderr, "nexus: comment posted to %s ✓\n", ticketID)
			return nil
		}

		issue, err := client.GetIssue(ticketID)
		if err != nil {
			return fmt.Errorf("fetch ticket: %w", err)
		}

		printIssue(issue)

		comments, err := client.GetComments(ticketID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "nexus: warning: could not fetch comments: %v\n", err)
		} else {
			printComments(comments)
		}

		if len(issue.LinkedIssues) > 0 {
			printLinkedIssues(client, issue.LinkedIssues)
		}

		if len(issue.RemoteLinks) > 0 {
			fmt.Print("\n## Remote Links\n\n")
			for _, link := range issue.RemoteLinks {
				fmt.Printf("- %s\n", link)
			}
		}

		return nil
	},
}

func printIssue(issue *jira.Issue) {
	fmt.Printf("# %s\n\n", issue.Key)
	fmt.Printf("**%s**\n\n", issue.Summary)
	fmt.Printf("| Field | Value |\n|---|---|\n")
	fmt.Printf("| Status | %s |\n", issue.Status)
	fmt.Printf("| Type | %s |\n", issue.Type)
	fmt.Printf("| Priority | %s |\n", issue.Priority)
	fmt.Printf("| Assignee | %s |\n", issue.Assignee)
	if issue.Reporter != "" {
		fmt.Printf("| Reporter | %s |\n", issue.Reporter)
	}
	if len(issue.Components) > 0 {
		fmt.Printf("| Components | %s |\n", strings.Join(issue.Components, ", "))
	}
	if len(issue.Labels) > 0 {
		fmt.Printf("| Labels | %s |\n", strings.Join(issue.Labels, ", "))
	}
	if issue.DueDate != "" {
		fmt.Printf("| Due Date | %s |\n", issue.DueDate)
	}
	fmt.Printf("| Created | %s |\n", formatTime(issue.Created))
	fmt.Printf("| Updated | %s |\n", formatTime(issue.Updated))

	if issue.Description != "" {
		fmt.Printf("\n## Description\n\n%s\n", issue.Description)
	}

	if issue.AccCriteria != "" {
		fmt.Printf("\n## Acceptance Criteria\n\n%s\n", issue.AccCriteria)
	}
}

func printComments(comments []jira.Comment) {
	if len(comments) == 0 {
		return
	}
	fmt.Printf("\n## Comments (%d)\n\n", len(comments))
	for _, c := range comments {
		fmt.Printf("**%s** (%s):\n%s\n\n", c.Author, formatTime(c.Created), c.Body)
	}
}

func printLinkedIssues(client *jira.Client, keys []string) {
	fmt.Print("\n## Linked Issues\n\n")
	for _, key := range keys {
		linked, err := client.GetIssue(key)
		if err != nil {
			fmt.Printf("- %s (could not fetch)\n", key)
			continue
		}
		fmt.Printf("- **%s** [%s] %s — %s\n", linked.Key, linked.Type, linked.Status, linked.Summary)
	}
}

var confluenceCmd = &cobra.Command{
	Use:   "confluence [page-id or search-query]",
	Short: "Read Confluence page with comments, or search pages",
	Long: `Fetches a Confluence page by ID or searches by CQL query.

  By ID:      nexus confluence 12345
  By title:   nexus confluence --search 'title = "PROJ-1002"'
  By text:    nexus confluence --search 'text ~ "order notifications"'
  Comment:    nexus confluence 12345 --comment "Updated implementation notes"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		client := confluence.NewClient(cfg.Confluence.BaseURL, cfg.Confluence.TokenEnv)
		if !client.Available() {
			return fmt.Errorf("Confluence not configured — set confluence.base_url and token env var")
		}

		commentText, _ := cmd.Flags().GetString("comment")
		if commentText != "" {
			pageID := args[0]
			htmlBody := fmt.Sprintf("<p>%s</p>", commentText)
			fmt.Fprintf(os.Stderr, "nexus: posting comment to page %s (%d chars)...\n", pageID, len(commentText))
			if err := client.AddPageComment(pageID, htmlBody); err != nil {
				return fmt.Errorf("add comment: %w", err)
			}
			fmt.Fprintf(os.Stderr, "nexus: comment posted to page %s ✓\n", pageID)
			return nil
		}

		searchFlag, _ := cmd.Flags().GetBool("search")

		if searchFlag {
			return searchConfluence(client, args[0])
		}
		return readConfluencePage(client, args[0])
	},
}

func readConfluencePage(client *confluence.Client, pageID string) error {
	page, err := client.GetPage(pageID)
	if err != nil {
		return fmt.Errorf("fetch page: %w", err)
	}

	fmt.Printf("# %s\n\n", page.Title)
	fmt.Printf("Space: %s | Version: %d\n", page.Space, page.Version)
	if page.URL != "" {
		fmt.Printf("URL: %s\n", page.URL)
	}
	fmt.Printf("\n## Content\n\n%s\n", page.Body)

	comments, err := client.GetPageComments(pageID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nexus: warning: could not fetch comments: %v\n", err)
	} else if len(comments) > 0 {
		fmt.Printf("\n## Comments (%d)\n\n", len(comments))
		for _, c := range comments {
			fmt.Printf("**%s** (%s):\n%s\n\n", c.Author, formatTime(c.Created), c.Body)
		}
	}

	return nil
}

func searchConfluence(client *confluence.Client, cql string) error {
	result, err := client.SearchCQL(cql, 10)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	fmt.Printf("## Results (%d)\n\n", result.Total)
	for _, p := range result.Pages {
		fmt.Printf("- **%s** (id: %s, space: %s)\n", p.Title, p.ID, p.Space)
		if p.URL != "" {
			fmt.Printf("  %s\n", p.URL)
		}
	}
	return nil
}

func formatTime(raw string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000+0000", raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return raw
	}
	return t.Format("2006-01-02 15:04")
}

func init() {
	jiraCmd.Flags().String("comment", "", "post a comment to the ticket")
	rootCmd.AddCommand(jiraCmd)
	confluenceCmd.Flags().Bool("search", false, "treat argument as CQL search query instead of page ID")
	confluenceCmd.Flags().String("comment", "", "post a comment to the page")
	rootCmd.AddCommand(confluenceCmd)
}
