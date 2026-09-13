package jira

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing auth header")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"key": "PROJ-123",
			"fields": map[string]any{
				"summary":     "Fix payment flow",
				"description": "The payment flow is broken when...",
				"issuetype":   map[string]string{"name": "Bug"},
				"status":      map[string]string{"name": "To Do"},
				"priority":    map[string]string{"name": "High"},
				"assignee":    map[string]string{"displayName": "Anurag"},
				"labels":      []string{"payments", "critical"},
				"components":  []map[string]string{{"name": "payment-service"}},
			},
		})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	issue, err := c.GetIssue("PROJ-123")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Key != "PROJ-123" {
		t.Fatalf("expected PROJ-123, got %s", issue.Key)
	}
	if issue.Type != "Bug" {
		t.Fatalf("expected Bug, got %s", issue.Type)
	}
	if issue.Assignee != "Anurag" {
		t.Fatalf("expected Anurag, got %s", issue.Assignee)
	}
}

func TestSearchJQL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{"key": "PROJ-1", "fields": map[string]any{"summary": "Fix bug", "issuetype": map[string]string{"name": "Bug"}, "status": map[string]string{"name": "Done"}}},
				{"key": "PROJ-2", "fields": map[string]any{"summary": "Add feature", "issuetype": map[string]string{"name": "Story"}, "status": map[string]string{"name": "In Progress"}}},
			},
			"total": 2,
		})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	result, err := c.SearchJQL("project = PROJ", 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 {
		t.Fatalf("expected 2 results, got %d", result.Total)
	}
}

func TestAddComment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	err := c.AddComment("PROJ-123", "Implementation started")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetTransitions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"transitions": []map[string]string{
				{"id": "11", "name": "In Progress"},
				{"id": "21", "name": "Done"},
			},
		})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	transitions, err := c.GetTransitions("PROJ-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 {
		t.Fatalf("expected 2, got %d", len(transitions))
	}
}

func TestNotAvailable(t *testing.T) {
	c := NewClient("", "NONEXISTENT_TOKEN_VAR")
	if c.Available() {
		t.Fatal("should not be available without base URL and token")
	}
}
