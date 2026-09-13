package jira

import "strings"

type Issue struct {
	Key            string   `json:"key"`
	Summary        string   `json:"summary"`
	Description    string   `json:"description"`
	Type           string   `json:"type"`
	Status         string   `json:"status"`
	Priority       string   `json:"priority"`
	Assignee       string   `json:"assignee"`
	Labels         []string `json:"labels"`
	Components     []string `json:"components"`
	Reporter       string   `json:"reporter,omitempty"`
	AccCriteria    string   `json:"acceptance_criteria"`
	DueDate        string   `json:"due_date,omitempty"`
	Created        string   `json:"created,omitempty"`
	Updated        string   `json:"updated,omitempty"`
	RemoteLinks    []string `json:"remote_links,omitempty"`
	LinkedIssues   []string  `json:"linked_issues,omitempty"`
	LinkedServices []string  `json:"linked_services,omitempty"`
	Comments       []Comment `json:"comments,omitempty"`
}

func (i *Issue) AllText() string {
	parts := []string{i.Summary, i.Description, i.AccCriteria}
	parts = append(parts, i.RemoteLinks...)
	return strings.Join(parts, " ")
}

type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Comment struct {
	ID      string `json:"id"`
	Author  string `json:"author"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

type SearchResult struct {
	Issues []Issue `json:"issues"`
	Total  int     `json:"total"`
}
