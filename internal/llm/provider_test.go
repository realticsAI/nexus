package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anurag/nexus/internal/config"
)

func TestAnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatal("missing api key")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatal("missing version header")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"text": "Hello from LLM"}},
			"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
			"model":   "claude-sonnet-5",
		})
	}))
	defer srv.Close()

	p := &AnthropicProvider{
		apiKey:     "test-key",
		model:      "claude-sonnet-5",
		baseURL:    srv.URL,
		maxRetries: 1,
		client:     srv.Client(),
	}

	resp, err := p.Complete(CompletionRequest{
		SystemPrompt: "You are a helpful assistant",
		UserPrompt:   "Say hello",
		MaxTokens:    100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello from LLM" {
		t.Fatalf("expected Hello from LLM, got %s", resp.Content)
	}
	if resp.TokensUsed != 15 {
		t.Fatalf("expected 15 tokens, got %d", resp.TokensUsed)
	}
}

func TestAnthropicRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(500)
			w.Write([]byte("server error"))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"text": "recovered"}},
			"usage":   map[string]int{"input_tokens": 5, "output_tokens": 3},
			"model":   "claude-sonnet-5",
		})
	}))
	defer srv.Close()

	p := &AnthropicProvider{
		apiKey:     "test-key",
		model:      "claude-sonnet-5",
		baseURL:    srv.URL,
		maxRetries: 3,
		client:     srv.Client(),
	}

	resp, err := p.Complete(CompletionRequest{UserPrompt: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "recovered" {
		t.Fatalf("expected recovered, got %s", resp.Content)
	}
}

func TestOllamaComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": "Hello from Ollama",
			"model":    "llama3",
		})
	}))
	defer srv.Close()

	p := &OllamaProvider{baseURL: srv.URL, model: "llama3", client: srv.Client()}
	resp, err := p.Complete(CompletionRequest{UserPrompt: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello from Ollama" {
		t.Fatalf("expected Hello from Ollama, got %s", resp.Content)
	}
}

func TestFactory(t *testing.T) {
	p := NewFromConfig(config.LLMConfig{Provider: "anthropic", Model: "claude-sonnet-5", APIKeyEnv: "TEST_KEY"})
	if p.Name() != "anthropic" {
		t.Fatalf("expected anthropic, got %s", p.Name())
	}

	p2 := NewFromConfig(config.LLMConfig{Provider: "ollama", Model: "llama3"})
	if p2.Name() != "ollama" {
		t.Fatalf("expected ollama, got %s", p2.Name())
	}

	p3 := NewFromConfig(config.LLMConfig{Provider: "bedrock", Model: "anthropic.claude-sonnet-5", Region: "us-east-1"})
	if p3.Name() != "bedrock" {
		t.Fatalf("expected bedrock, got %s", p3.Name())
	}
}

func TestBedrockSSOHint(t *testing.T) {
	p := NewBedrockProvider("anthropic.claude-sonnet-5", "us-east-1", "my-profile", 1)
	p.initErr = fmt.Errorf("expired token")
	_, err := p.Complete(CompletionRequest{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "aws sso login --profile my-profile") {
		t.Fatalf("expected SSO hint, got: %s", err.Error())
	}
}

func TestNotAvailableWithoutKey(t *testing.T) {
	p := NewAnthropicProvider("claude-sonnet-5", "NONEXISTENT_KEY_VAR", "", 1)
	if p.Available() {
		t.Fatal("should not be available without API key")
	}
}
