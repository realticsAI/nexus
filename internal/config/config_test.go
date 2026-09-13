package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.StateDir() == "" {
		t.Fatal("StateDir should not be empty")
	}
	if cfg.Embeddings.Provider != "ollama" {
		t.Fatalf("default embedding provider should be ollama, got %s", cfg.Embeddings.Provider)
	}
	if cfg.Agent.MaxFilesChanged != 10 {
		t.Fatalf("default max_files_changed should be 10, got %d", cfg.Agent.MaxFilesChanged)
	}
	if cfg.Agent.AutoPush {
		t.Fatal("auto_push should default to false")
	}
	if cfg.LLM.Provider != "bedrock" {
		t.Fatalf("default LLM provider should be bedrock, got %s", cfg.LLM.Provider)
	}
	if cfg.LLM.Region != "us-east-1" {
		t.Fatalf("default LLM region should be us-east-1, got %s", cfg.LLM.Region)
	}
	if cfg.MCP.Client != "generic" {
		t.Fatalf("default MCP client should be generic, got %s", cfg.MCP.Client)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	err := os.WriteFile(cfgPath, []byte(`
workspaces:
  - path: /tmp/test-workspace
embeddings:
  provider: none
llm:
  provider: openai
  model: gpt-4o
  api_key_env: OPENAI_API_KEY
  base_url: https://api.openai.com/v1
mcp:
  client: copilot
agent:
  max_files_changed: 5
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(cfg.Workspaces))
	}
	if cfg.Embeddings.Provider != "none" {
		t.Fatalf("expected provider none, got %s", cfg.Embeddings.Provider)
	}
	if cfg.LLM.Provider != "openai" {
		t.Fatalf("expected openai, got %s", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", cfg.LLM.Model)
	}
	if cfg.MCP.Client != "copilot" {
		t.Fatalf("expected copilot, got %s", cfg.MCP.Client)
	}
	if cfg.Agent.MaxFilesChanged != 5 {
		t.Fatalf("expected 5, got %d", cfg.Agent.MaxFilesChanged)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/config.yml")
	if err != nil {
		t.Fatal("missing file should return defaults, not error")
	}
	if cfg.Embeddings.Provider != "ollama" {
		t.Fatalf("should return defaults, got provider %s", cfg.Embeddings.Provider)
	}
}

func TestStateDirExpandsHome(t *testing.T) {
	cfg := Defaults()
	stateDir := cfg.StateDir()
	if stateDir == "~/.nexus/state" {
		t.Fatal("StateDir should expand ~ to home directory")
	}
	if !filepath.IsAbs(stateDir) {
		t.Fatalf("StateDir should be absolute, got %s", stateDir)
	}
}

func TestCustomLLMProvider(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	os.WriteFile(cfgPath, []byte(`
llm:
  provider: custom
  model: my-local-model
  base_url: http://localhost:8000/v1
  api_key_env: MY_KEY
`), 0644)

	cfg, _ := LoadFrom(cfgPath)
	if cfg.LLM.Provider != "custom" {
		t.Fatalf("expected custom, got %s", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "http://localhost:8000/v1" {
		t.Fatalf("expected custom base_url, got %s", cfg.LLM.BaseURL)
	}
}
