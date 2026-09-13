package confluence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
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

func (c *Client) SearchCQL(cql string, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 25
	}
	path := fmt.Sprintf("/rest/api/content/search?cql=%s&limit=%d&expand=body.storage", url.QueryEscape(cql), limit)

	var raw struct {
		Results []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Space struct {
				Key string `json:"key"`
			} `json:"space"`
			Body struct {
				Storage struct {
					Value string `json:"value"`
				} `json:"storage"`
			} `json:"body"`
			Version struct {
				Number int `json:"number"`
			} `json:"version"`
			Links struct {
				WebUI string `json:"webui"`
			} `json:"_links"`
		} `json:"results"`
		Size int `json:"size"`
	}

	if err := c.get(path, &raw); err != nil {
		return nil, err
	}

	pages := make([]Page, len(raw.Results))
	for i, r := range raw.Results {
		pages[i] = Page{
			ID:      r.ID,
			Title:   r.Title,
			Space:   r.Space.Key,
			Body:    r.Body.Storage.Value,
			Version: r.Version.Number,
			URL:     c.baseURL + r.Links.WebUI,
		}
	}
	return &SearchResult{Pages: pages, Total: raw.Size}, nil
}

func (c *Client) GetPage(pageID string) (*Page, error) {
	path := fmt.Sprintf("/rest/api/content/%s?expand=body.storage,version,space", pageID)

	var raw struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
		Body struct {
			Storage struct {
				Value string `json:"value"`
			} `json:"storage"`
		} `json:"body"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
		Links struct {
			WebUI string `json:"webui"`
		} `json:"_links"`
	}

	if err := c.get(path, &raw); err != nil {
		return nil, err
	}

	return &Page{
		ID:      raw.ID,
		Title:   raw.Title,
		Space:   raw.Space.Key,
		Body:    raw.Body.Storage.Value,
		Version: raw.Version.Number,
		URL:     c.baseURL + raw.Links.WebUI,
	}, nil
}

func (c *Client) GetChildPages(pageID string) ([]Page, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/page?expand=version", pageID)

	var raw struct {
		Results []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Version struct {
				Number int `json:"number"`
			} `json:"version"`
		} `json:"results"`
	}

	if err := c.get(path, &raw); err != nil {
		return nil, err
	}

	pages := make([]Page, len(raw.Results))
	for i, r := range raw.Results {
		pages[i] = Page{ID: r.ID, Title: r.Title, Version: r.Version.Number}
	}
	return pages, nil
}

func (c *Client) CreatePage(spaceKey, title, htmlBody string, ancestorID string) (*Page, error) {
	payload := map[string]any{
		"type":  "page",
		"title": title,
		"space": map[string]string{"key": spaceKey},
		"body": map[string]any{
			"storage": map[string]string{"value": htmlBody, "representation": "storage"},
		},
	}
	if ancestorID != "" {
		payload["ancestors"] = []map[string]string{{"id": ancestorID}}
	}

	body, _ := json.Marshal(payload)
	var raw struct {
		ID string `json:"id"`
	}
	if err := c.post("/rest/api/content", body, &raw); err != nil {
		return nil, err
	}
	return &Page{ID: raw.ID, Title: title, Space: spaceKey}, nil
}

func (c *Client) UpdatePage(pageID, title, htmlBody string, currentVersion int) error {
	payload, _ := json.Marshal(map[string]any{
		"type":  "page",
		"title": title,
		"body": map[string]any{
			"storage": map[string]string{"value": htmlBody, "representation": "storage"},
		},
		"version": map[string]int{"number": currentVersion + 1},
	})
	path := fmt.Sprintf("/rest/api/content/%s", pageID)
	return c.put(path, payload)
}

func (c *Client) GetPageComments(pageID string) ([]Comment, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/comment?expand=body.storage&limit=100", pageID)
	var raw struct {
		Results []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Body  struct {
				Storage struct {
					Value string `json:"value"`
				} `json:"storage"`
			} `json:"body"`
			Version struct {
				By struct {
					DisplayName string `json:"displayName"`
				} `json:"by"`
				When string `json:"when"`
			} `json:"version"`
		} `json:"results"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}
	comments := make([]Comment, len(raw.Results))
	for i, r := range raw.Results {
		comments[i] = Comment{
			ID:      r.ID,
			Author:  r.Version.By.DisplayName,
			Body:    r.Body.Storage.Value,
			Created: r.Version.When,
		}
	}
	return comments, nil
}

func (c *Client) AddPageComment(pageID, htmlBody string) error {
	payload, _ := json.Marshal(map[string]any{
		"type": "comment",
		"container": map[string]string{"id": pageID, "type": "page"},
		"body": map[string]any{
			"storage": map[string]string{"value": htmlBody, "representation": "storage"},
		},
	})
	return c.post("/rest/api/content", payload, nil)
}

func (c *Client) put(path string, body []byte) error {
	req, err := http.NewRequest("PUT", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, nil)
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

func (c *Client) do(req *http.Request, result any) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("confluence request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("confluence %s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
