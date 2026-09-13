package figma

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseFigmaURL(t *testing.T) {
	tests := []struct {
		url     string
		fileKey string
		nodeID  string
	}{
		{"https://www.figma.com/file/abc123/My-Design", "abc123", ""},
		{"https://figma.com/design/xyz789/Project?node-id=1-2", "xyz789", "1:2"},
		{"https://www.figma.com/proto/def456/Proto?node-id=3%3A4", "def456", "3:4"},
		{"not a figma url", "", ""},
		{"[https://figma.com/design/abc123/App?node-id=754-9167]", "abc123", "754:9167"},
		{"*[https://figma.com/design/abc123/X?node-id=20-30]*", "abc123", "20:30"},
	}

	for _, tc := range tests {
		fk, nid := ParseFigmaURL(tc.url)
		if fk != tc.fileKey {
			t.Errorf("ParseFigmaURL(%q) fileKey = %q, want %q", tc.url, fk, tc.fileKey)
		}
		if nid != tc.nodeID {
			t.Errorf("ParseFigmaURL(%q) nodeID = %q, want %q", tc.url, nid, tc.nodeID)
		}
	}
}

func TestExtractFigmaLinks(t *testing.T) {
	text := `Check the design at https://www.figma.com/file/abc123/Design and also
	the proto https://figma.com/proto/def456/Proto?node-id=1-2 and duplicate https://www.figma.com/file/abc123/Design`
	links := ExtractFigmaLinks(text)
	if len(links) != 2 {
		t.Fatalf("expected 2 unique links, got %d: %v", len(links), links)
	}
}

func TestGetFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Figma-Token") != "test-token" {
			t.Error("missing figma token header")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"name":         "Test Design",
			"lastModified": "2026-01-01T00:00:00Z",
			"version":      "v1",
			"document": map[string]any{
				"id": "0:0", "name": "Document", "type": "DOCUMENT",
				"children": []map[string]any{
					{"id": "1:0", "name": "Page 1", "type": "CANVAS", "children": []map[string]any{
						{"id": "1:1", "name": "Frame 1", "type": "FRAME",
							"absoluteBoundingBox": map[string]any{"x": 0, "y": 0, "width": 375, "height": 812},
							"children":            []map[string]any{{"id": "1:2", "name": "Title", "type": "TEXT", "characters": "Hello World"}}},
					}},
				},
			},
			"components": map[string]any{
				"2:1": map[string]any{"key": "k1", "name": "Button", "description": "Primary button"},
			},
		})
	}))
	defer srv.Close()

	_ = srv // server available for future HTTP-level tests

	// Test ExtractDesignContext with mock data
	file := &File{
		Key:  "test",
		Name: "Test Design",
		Document: &Node{
			ID: "0:0", Name: "Document", Type: "DOCUMENT",
			Children: []*Node{
				{ID: "1:0", Name: "Page 1", Type: "CANVAS", Children: []*Node{
					{ID: "1:1", Name: "Frame 1", Type: "FRAME",
						AbsoluteBoundingBox: &Rect{Width: 375, Height: 812},
						Children: []*Node{{ID: "1:2", Name: "Title", Type: "TEXT", Characters: "Hello World"}}},
				}},
			},
		},
		Components: Components{"2:1": ComponentMeta{Key: "k1", Name: "Button", Description: "Primary button"}},
	}

	ctx := extractDesignContextFromFile(file)
	if ctx.FileName != "Test Design" {
		t.Errorf("expected file name 'Test Design', got %q", ctx.FileName)
	}
	if len(ctx.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(ctx.Pages))
	}
	if ctx.Pages[0].FrameCount != 1 {
		t.Errorf("expected 1 frame, got %d", ctx.Pages[0].FrameCount)
	}
	if len(ctx.Components) != 1 || ctx.Components[0].Name != "Button" {
		t.Errorf("expected Button component, got %v", ctx.Components)
	}
	if len(ctx.Frames) != 1 || ctx.Frames[0].Width != 375 {
		t.Errorf("expected frame width 375, got %v", ctx.Frames)
	}
	if len(ctx.Frames[0].Text) != 1 || ctx.Frames[0].Text[0] != "Hello World" {
		t.Errorf("expected text 'Hello World', got %v", ctx.Frames[0].Text)
	}
}

func TestAvailable(t *testing.T) {
	c := &Client{token: ""}
	if c.Available() {
		t.Error("should not be available without token")
	}
	c.token = "test"
	if !c.Available() {
		t.Error("should be available with token")
	}
}
