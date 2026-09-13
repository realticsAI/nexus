package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	fieldMap   map[string]string // name → customfield ID, discovered once
}

func NewClient(baseURL, tokenEnv string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   os.Getenv(tokenEnv),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Available() bool {
	return c.baseURL != "" && c.token != ""
}

// discoverFields fetches /rest/api/2/field once and builds a name→id map.
func (c *Client) discoverFields() {
	if c.fieldMap != nil {
		return
	}
	c.fieldMap = map[string]string{}
	var fields []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.get("/rest/api/2/field", &fields); err != nil {
		return
	}
	for _, f := range fields {
		lower := strings.ToLower(f.Name)
		c.fieldMap[lower] = f.ID
	}
}

// resolveField finds a custom field ID by trying multiple name variants.
func (c *Client) resolveField(names ...string) string {
	c.discoverFields()
	for _, name := range names {
		if id, ok := c.fieldMap[strings.ToLower(name)]; ok {
			return id
		}
	}
	return ""
}

func (c *Client) GetIssue(key string) (*Issue, error) {
	c.discoverFields()

	// Build dynamic field list from discovered names
	acField := c.resolveField("Acceptance Criteria", "acceptance criteria", "AC")
	storyPtsField := c.resolveField("Story Points", "story points")
	sprintField := c.resolveField("Sprint", "sprint")
	dueDateField := c.resolveField("Due date", "due date", "Target end")
	epicField := c.resolveField("Epic Link", "epic link")

	fieldList := "summary,description,issuetype,status,priority,assignee,reporter,labels,components,issuelinks,duedate,created,updated"
	for _, f := range []string{acField, storyPtsField, sprintField, dueDateField, epicField} {
		if f != "" && !strings.Contains(fieldList, f) {
			fieldList += "," + f
		}
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s?fields=%s", key, fieldList)

	// Parse into generic structure to handle dynamic custom fields
	var raw struct {
		Key    string                 `json:"key"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}

	getString := func(field string) string {
		msg, ok := raw.Fields[field]
		if !ok || msg == nil {
			return ""
		}
		var s string
		if json.Unmarshal(msg, &s) == nil {
			return s
		}
		return ""
	}

	getNameField := func(field string) string {
		msg, ok := raw.Fields[field]
		if !ok || msg == nil {
			return ""
		}
		var obj struct{ Name string }
		if json.Unmarshal(msg, &obj) == nil {
			return obj.Name
		}
		return ""
	}

	getDisplayName := func(field string) string {
		msg, ok := raw.Fields[field]
		if !ok || msg == nil {
			return ""
		}
		var obj struct{ DisplayName string }
		if json.Unmarshal(msg, &obj) == nil {
			return obj.DisplayName
		}
		return ""
	}

	getStringArray := func(field string) []string {
		msg, ok := raw.Fields[field]
		if !ok || msg == nil {
			return nil
		}
		var arr []string
		if json.Unmarshal(msg, &arr) == nil {
			return arr
		}
		return nil
	}

	getNameArray := func(field string) []string {
		msg, ok := raw.Fields[field]
		if !ok || msg == nil {
			return nil
		}
		var arr []struct{ Name string }
		if json.Unmarshal(msg, &arr) == nil {
			names := make([]string, len(arr))
			for i, a := range arr {
				names[i] = a.Name
			}
			return names
		}
		return nil
	}

	// Extract linked issues
	var linkedIssues []string
	if msg, ok := raw.Fields["issuelinks"]; ok && msg != nil {
		var links []struct {
			InwardIssue  *struct{ Key string } `json:"inwardIssue"`
			OutwardIssue *struct{ Key string } `json:"outwardIssue"`
		}
		if json.Unmarshal(msg, &links) == nil {
			for _, link := range links {
				if link.InwardIssue != nil {
					linkedIssues = append(linkedIssues, link.InwardIssue.Key)
				}
				if link.OutwardIssue != nil {
					linkedIssues = append(linkedIssues, link.OutwardIssue.Key)
				}
			}
		}
	}

	// Get AC from the discovered field
	accCriteria := ""
	if acField != "" {
		accCriteria = getString(acField)
	}

	// Get due date — try the custom field first, then standard duedate
	dueDate := ""
	if dueDateField != "" {
		dueDate = getString(dueDateField)
	}
	if dueDate == "" {
		dueDate = getString("duedate")
	}

	issue := &Issue{
		Key:          raw.Key,
		Summary:      getString("summary"),
		Description:  getString("description"),
		Type:         getNameField("issuetype"),
		Status:       getNameField("status"),
		Priority:     getNameField("priority"),
		Assignee:     getDisplayName("assignee"),
		Reporter:     getDisplayName("reporter"),
		Labels:       getStringArray("labels"),
		Components:   getNameArray("components"),
		AccCriteria:  accCriteria,
		DueDate:      dueDate,
		Created:      getString("created"),
		Updated:      getString("updated"),
		LinkedIssues: linkedIssues,
	}

	if remoteLinks, err := c.GetRemoteLinks(key); err == nil {
		issue.RemoteLinks = remoteLinks
	}

	return issue, nil
}

func (c *Client) GetRemoteLinks(key string) ([]string, error) {
	path := fmt.Sprintf("/rest/api/2/issue/%s/remotelink", key)
	var raw []struct {
		Object struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"object"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}
	var links []string
	for _, rl := range raw {
		if rl.Object.URL != "" {
			links = append(links, rl.Object.URL)
		}
	}
	return links, nil
}

func (c *Client) SearchJQL(jql string, maxResults int) (*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	body, _ := json.Marshal(map[string]any{
		"jql":        jql,
		"maxResults": maxResults,
		"fields":     []string{"summary", "description", "issuetype", "status", "priority", "assignee", "labels", "components", "created", "updated"},
	})

	var raw struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary     string `json:"summary"`
				Description string `json:"description"`
				IssueType   struct {
					Name string `json:"name"`
				} `json:"issuetype"`
				Status struct {
					Name string `json:"name"`
				} `json:"status"`
				Priority struct {
					Name string `json:"name"`
				} `json:"priority"`
				Assignee *struct {
					DisplayName string `json:"displayName"`
				} `json:"assignee"`
				Labels     []string `json:"labels"`
				Components []struct {
					Name string `json:"name"`
				} `json:"components"`
				Created string `json:"created"`
				Updated string `json:"updated"`
			} `json:"fields"`
		} `json:"issues"`
		Total int `json:"total"`
	}

	if err := c.post("/rest/api/2/search", body, &raw); err != nil {
		return nil, err
	}

	issues := make([]Issue, len(raw.Issues))
	for i, ri := range raw.Issues {
		assignee := ""
		if ri.Fields.Assignee != nil {
			assignee = ri.Fields.Assignee.DisplayName
		}
		components := make([]string, len(ri.Fields.Components))
		for j, comp := range ri.Fields.Components {
			components[j] = comp.Name
		}
		issues[i] = Issue{
			Key:         ri.Key,
			Summary:     ri.Fields.Summary,
			Description: ri.Fields.Description,
			Type:        ri.Fields.IssueType.Name,
			Status:      ri.Fields.Status.Name,
			Priority:    ri.Fields.Priority.Name,
			Assignee:    assignee,
			Labels:      ri.Fields.Labels,
			Components:  components,
			Created:     ri.Fields.Created,
			Updated:     ri.Fields.Updated,
		}
	}
	return &SearchResult{Issues: issues, Total: raw.Total}, nil
}

func (c *Client) GetComments(key string) ([]Comment, error) {
	path := fmt.Sprintf("/rest/api/2/issue/%s/comment?orderBy=-created", key)
	var raw struct {
		Comments []struct {
			ID     string `json:"id"`
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body    string `json:"body"`
			Created string `json:"created"`
		} `json:"comments"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}
	comments := make([]Comment, len(raw.Comments))
	for i, rc := range raw.Comments {
		comments[i] = Comment{
			ID:      rc.ID,
			Author:  rc.Author.DisplayName,
			Body:    rc.Body,
			Created: rc.Created,
		}
	}
	return comments, nil
}

func (c *Client) AddComment(key, body string) error {
	payload, _ := json.Marshal(map[string]string{"body": body})
	path := fmt.Sprintf("/rest/api/2/issue/%s/comment", key)
	return c.post(path, payload, nil)
}

func (c *Client) GetTransitions(key string) ([]Transition, error) {
	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", key)
	var raw struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"transitions"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}
	transitions := make([]Transition, len(raw.Transitions))
	for i, t := range raw.Transitions {
		transitions[i] = Transition{ID: t.ID, Name: t.Name}
	}
	return transitions, nil
}

func (c *Client) Transition(key, transitionID string) error {
	payload, _ := json.Marshal(map[string]any{
		"transition": map[string]string{"id": transitionID},
	})
	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", key)
	return c.post(path, payload, nil)
}

func (c *Client) UpdateFields(key string, fields map[string]any) error {
	payload, _ := json.Marshal(map[string]any{"fields": fields})
	path := fmt.Sprintf("/rest/api/2/issue/%s", key)
	return c.put(path, payload)
}

func (c *Client) get(path string, result any) error {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) post(path string, body []byte, result any) error {
	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, result)
}

func (c *Client) put(path string, body []byte) error {
	req, err := http.NewRequest("PUT", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, nil)
}

func (c *Client) do(req *http.Request, result any) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira %s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
