package confluence

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchCQL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id": "12345", "title": "PROJ-1006 Technical Design",
					"space":   map[string]string{"key": "PROJ"},
					"body":    map[string]any{"storage": map[string]string{"value": "<p>Design content</p>"}},
					"version": map[string]int{"number": 3},
					"_links":  map[string]string{"webui": "/display/LOY/page"},
				},
			},
			"size": 1,
		})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	result, err := c.SearchCQL("title = \"PROJ-1006\"", 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1, got %d", result.Total)
	}
	if result.Pages[0].Title != "PROJ-1006 Technical Design" {
		t.Fatalf("wrong title: %s", result.Pages[0].Title)
	}
}

func TestGetPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": "12345", "title": "Test Page",
			"space":   map[string]string{"key": "PROJ"},
			"body":    map[string]any{"storage": map[string]string{"value": "<p>Page body</p>"}},
			"version": map[string]int{"number": 2},
			"_links":  map[string]string{"webui": "/display/LOY/test"},
		})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	page, err := c.GetPage("12345")
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Test Page" {
		t.Fatalf("expected Test Page, got %s", page.Title)
	}
}

func TestNotAvailable(t *testing.T) {
	c := NewClient("", "NONEXISTENT_TOKEN")
	if c.Available() {
		t.Fatal("should not be available")
	}
}
