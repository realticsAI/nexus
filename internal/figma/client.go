package figma

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const baseURL = "https://api.figma.com/v1"

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(tokenEnv string) *Client {
	return &Client{
		token: os.Getenv(tokenEnv),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Available() bool {
	return c.token != ""
}

func (c *Client) GetFile(fileKey string) (*File, error) {
	path := fmt.Sprintf("/files/%s?depth=3", fileKey)
	var raw struct {
		Name         string     `json:"name"`
		LastModified string     `json:"lastModified"`
		Version      string     `json:"version"`
		Document     *Node      `json:"document"`
		Components   Components `json:"components"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}
	return &File{
		Key:          fileKey,
		Name:         raw.Name,
		LastModified: raw.LastModified,
		Version:      raw.Version,
		Document:     raw.Document,
		Components:   raw.Components,
	}, nil
}

func (c *Client) GetFileNodes(fileKey string, nodeIDs []string) (map[string]*Node, error) {
	ids := url.QueryEscape(strings.Join(nodeIDs, ","))
	path := fmt.Sprintf("/files/%s/nodes?ids=%s&depth=7", fileKey, ids)
	var raw struct {
		Nodes map[string]struct {
			Document *Node `json:"document"`
		} `json:"nodes"`
	}
	if err := c.get(path, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]*Node, len(raw.Nodes))
	for id, n := range raw.Nodes {
		result[id] = n.Document
	}
	return result, nil
}

func (c *Client) GetImages(fileKey string, nodeIDs []string, format string, scale float64) (*ImageResponse, error) {
	if format == "" {
		format = "png"
	}
	if scale <= 0 {
		scale = 2
	}
	ids := url.QueryEscape(strings.Join(nodeIDs, ","))
	path := fmt.Sprintf("/images/%s?ids=%s&format=%s&scale=%g", fileKey, ids, format, scale)
	var resp ImageResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetComponents(fileKey string) (Components, error) {
	file, err := c.GetFile(fileKey)
	if err != nil {
		return nil, err
	}
	return file.Components, nil
}

func (c *Client) ExtractDesignContext(fileKey string) (*DesignContext, error) {
	file, err := c.GetFile(fileKey)
	if err != nil {
		return nil, fmt.Errorf("fetching figma file: %w", err)
	}
	return extractDesignContextFromFile(file), nil
}

func extractDesignContextFromFile(file *File) *DesignContext {
	ctx := &DesignContext{
		FileName: file.Name,
		FileKey:  file.Key,
	}

	for id, comp := range file.Components {
		ctx.Components = append(ctx.Components, ComponentInfo{
			Name:        comp.Name,
			Description: comp.Description,
			NodeID:      id,
		})
	}

	if file.Document != nil {
		for _, page := range file.Document.Children {
			if page.Type != "CANVAS" {
				continue
			}
			ps := PageSummary{Name: page.Name}
			for _, frame := range page.Children {
				if frame.Type != "FRAME" && frame.Type != "COMPONENT" && frame.Type != "COMPONENT_SET" {
					continue
				}
				ps.FrameCount++
				fs := FrameSummary{
					Name:     frame.Name,
					NodeID:   frame.ID,
					Page:     page.Name,
					Children: countChildren(frame),
				}
				if frame.AbsoluteBoundingBox != nil {
					fs.Width = frame.AbsoluteBoundingBox.Width
					fs.Height = frame.AbsoluteBoundingBox.Height
				}
				fs.Text = extractText(frame, 3)
				ctx.Frames = append(ctx.Frames, fs)
			}
			ctx.Pages = append(ctx.Pages, ps)
		}
	}

	return ctx
}

var figmaURLPattern = regexp.MustCompile(`https?://(?:www\.)?figma\.com/(?:file|design|proto)/([a-zA-Z0-9]+)(?:/[^?\s]*)?(?:\?node-id=([^&\s]+))?`)

func ParseFigmaURL(rawURL string) (fileKey string, nodeID string) {
	matches := figmaURLPattern.FindStringSubmatch(rawURL)
	if len(matches) < 2 {
		return "", ""
	}
	fileKey = matches[1]
	if len(matches) > 2 {
		nodeID, _ = url.QueryUnescape(matches[2])
	}
	// Strip trailing Jira/wiki markup characters
	nodeID = strings.TrimRight(nodeID, "])*|")
	// Figma URLs use hyphens (754-9167) but the API uses colons (754:9167)
	nodeID = strings.ReplaceAll(nodeID, "-", ":")
	return
}

func ExtractFigmaLinks(text string) []string {
	matches := figmaURLPattern.FindAllString(text, -1)
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

func countChildren(node *Node) int {
	if node == nil || len(node.Children) == 0 {
		return 0
	}
	count := len(node.Children)
	for _, child := range node.Children {
		count += countChildren(child)
	}
	return count
}

func extractText(node *Node, maxDepth int) []string {
	if maxDepth <= 0 {
		return nil
	}
	var texts []string
	if node.Type == "TEXT" && node.Characters != "" {
		texts = append(texts, node.Characters)
	}
	for _, child := range node.Children {
		texts = append(texts, extractText(child, maxDepth-1)...)
	}
	if len(texts) > 20 {
		texts = texts[:20]
	}
	return texts
}

func ExtractAllText(node *Node, maxDepth int) []string {
	return extractText(node, maxDepth)
}

func ExtractFrameNames(node *Node) []string {
	var names []string
	if node == nil {
		return names
	}
	for _, child := range node.Children {
		if child.Type == "FRAME" || child.Type == "COMPONENT" || child.Type == "COMPONENT_SET" {
			names = append(names, child.Name)
		}
	}
	return names
}

func (c *Client) get(path string, result any) error {
	req, err := http.NewRequest("GET", baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Figma-Token", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("figma request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("figma %s returned %d: %s", req.URL.Path, resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}
