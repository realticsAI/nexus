# Nexus Phase 1: Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scaffold the Nexus Go project with config loading, CLI commands, shared model types, sharded storage engine, and a working `nexus status` command — the foundation everything else builds on.

**Architecture:** Three-stage pipeline (crawl → analyze → serve) connected by JSON files on disk. This phase builds the skeleton: model types define the contract between stages, the storage engine manages sharded per-service files with atomic writes and snapshot swap, and the CLI wires Cobra commands that later phases fill in.

**Tech Stack:** Go 1.24, Cobra CLI, gopkg.in/yaml.v3, standard library only (no frameworks)

**Spec:** `docs/specs/2026-09-13-nexus-design.md`

## Global Constraints

- Go 1.24+ (already installed at `/opt/homebrew/bin/go`)
- CGO_ENABLED=1 (required later for tree-sitter, set up now)
- All config from `~/.nexus/config.yml`, env vars for secrets only
- Sharded storage at `~/.nexus/state/` — never a single monolithic JSON
- Atomic file writes via temp file + `os.Rename()`
- No external dependencies beyond Cobra + yaml.v3 in this phase
- Every public function has a test
- Project root: `/Users/anurag/Documents/nexus/`

---

## File Structure

```
nexus/
├── go.mod                          ← module github.com/anurag/nexus
├── go.sum
├── Makefile                        ← build, test, install, clean
├── .gitignore
├── cmd/nexus/
│   ├── main.go                     ← Root Cobra command, version flag
│   ├── crawl.go                    ← nexus crawl (stub)
│   ├── analyze.go                  ← nexus analyze (stub)
│   ├── serve.go                    ← nexus serve (stub)
│   ├── work.go                     ← nexus work (stub)
│   ├── pipeline.go                 ← nexus pipeline (stub)
│   └── status.go                   ← nexus status (real — reads manifest + health)
├── model/
│   ├── index.go                    ← ServiceIndex, Unit, Method, FileChecksum
│   ├── graph.go                    ← Graph, Node, Edge, EdgeType constants
│   ├── platform.go                 ← Platform enum, Detect()
│   ├── manifest.go                 ← Manifest, ServiceEntry
│   ├── health.go                   ← CrawlHealth, RepoFailure
│   └── trace.go                    ← TraceContext, Confidence enum
├── internal/
│   ├── config/
│   │   ├── config.go               ← Config struct, Load(), Defaults()
│   │   └── config_test.go
│   └── store/
│       ├── store.go                ← IndexStore: sharded read/write, snapshot swap
│       ├── atomic.go               ← atomicWriteJSON, atomicWriteBytes
│       ├── store_test.go
│       └── atomic_test.go
└── run-mcp.sh                      ← MCP launcher shell script
```

---

### Task 1: Go Module + Build Tooling

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `cmd/nexus/main.go`

**Interfaces:**
- Consumes: nothing (first task)
- Produces: `func main()` — runs root Cobra command; `make build` produces `bin/nexus`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /Users/anurag/Documents/nexus
go mod init github.com/anurag/nexus
```

- [ ] **Step 2: Create the root Cobra command**

Create `cmd/nexus/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "nexus",
	Short: "Code-intelligence + autonomous SDLC engine",
	Long:  "Nexus indexes multi-language repos into a searchable graph, serves that intelligence to AI agents via MCP, and autonomously implements Jira tickets.",
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(fmt.Sprintf("nexus %s\n", version))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Add Cobra dependency**

```bash
cd /Users/anurag/Documents/nexus
go get github.com/spf13/cobra@latest
go mod tidy
```

- [ ] **Step 4: Create the Makefile**

Create `Makefile`:

```makefile
BINARY := nexus
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test clean install

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/nexus

test:
	go test ./... -v -count=1

clean:
	rm -rf $(BUILD_DIR)

install:
	CGO_ENABLED=1 go install $(LDFLAGS) ./cmd/nexus
```

- [ ] **Step 5: Create .gitignore**

Create `.gitignore`:

```
bin/
*.exe
*.test
*.out
.DS_Store
```

- [ ] **Step 6: Build and verify**

```bash
cd /Users/anurag/Documents/nexus
make build
./bin/nexus --version
./bin/nexus --help
```

Expected: prints `nexus dev` for version, shows help text with "Code-intelligence + autonomous SDLC engine".

- [ ] **Step 7: Commit**

```bash
git init
git add go.mod go.sum Makefile .gitignore cmd/nexus/main.go
git commit -m "feat: initialize Go module with Cobra CLI skeleton"
```

---

### Task 2: Model Types — The Contract Between Stages

**Files:**
- Create: `model/platform.go`
- Create: `model/index.go`
- Create: `model/graph.go`
- Create: `model/manifest.go`
- Create: `model/health.go`
- Create: `model/trace.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type Platform int` with constants `Java`, `TypeScript`, `Python`, `Unknown` and `func DetectPlatform(path string) Platform`
  - `type ServiceIndex struct` with `Key`, `Path`, `Platform`, `Units`, `Config`, `BuildDeps`, `FileChecksums`
  - `type Unit struct` with `Type`, `Name`, `FQN`, `File`, `Line`, `Stereotype`, `Methods`, `Dependencies`, `ConfigRefs`, `KafkaProduces`, `KafkaConsumes`
  - `type Method struct` with `Name`, `Line`, `ReturnType`, `Parameters`, `Annotations`
  - `type Graph struct` with `Nodes`, `Edges`, `KeywordIndex`
  - `type Edge struct` with `From`, `To`, `Type`, `Evidence`
  - `type EdgeType string` constants: `HTTPCall`, `KafkaProduce`, `KafkaConsume`, `FrontendAPICall`, `BuildDependency`, `ImportDep`
  - `type Manifest struct` with `Version`, `Services []ServiceEntry`, `GeneratedAt`
  - `type ServiceEntry struct` with `Key`, `Path`, `Platform`, `HeadSHA`, `FileCount`, `LastIndexed`
  - `type CrawlHealth struct` with `TotalRepos`, `Indexed`, `Failed []RepoFailure`, `CrawlDuration`, `Timestamp`
  - `type TraceContext struct` with `Confidence`, `ServicesConsulted`, `DataSources`, `IndexAge`
  - `type Confidence int` constants: `High`, `Medium`, `Low`

- [ ] **Step 1: Write Platform type with detection**

Create `model/platform.go`:

```go
package model

import (
	"os"
	"path/filepath"
)

type Platform int

const (
	Java Platform = iota
	TypeScript
	Python
	Unknown
)

func (p Platform) String() string {
	switch p {
	case Java:
		return "java"
	case TypeScript:
		return "typescript"
	case Python:
		return "python"
	default:
		return "unknown"
	}
}

func ParsePlatform(s string) Platform {
	switch s {
	case "java":
		return Java
	case "typescript":
		return TypeScript
	case "python":
		return Python
	default:
		return Unknown
	}
}

func DetectPlatform(repoPath string) Platform {
	if fileExists(filepath.Join(repoPath, "pom.xml")) ||
		fileExists(filepath.Join(repoPath, "build.gradle")) {
		return Java
	}
	if fileExists(filepath.Join(repoPath, "package.json")) {
		return TypeScript
	}
	if fileExists(filepath.Join(repoPath, "pyproject.toml")) ||
		fileExists(filepath.Join(repoPath, "requirements.txt")) ||
		fileExists(filepath.Join(repoPath, "setup.py")) {
		return Python
	}
	return Unknown
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 2: Write index model types**

Create `model/index.go`:

```go
package model

import "time"

type ServiceIndex struct {
	Key           string            `json:"key"`
	Path          string            `json:"path"`
	Platform      Platform          `json:"platform"`
	Units         []Unit            `json:"units"`
	Config        map[string]string `json:"config,omitempty"`
	BuildDeps     []string          `json:"build_deps,omitempty"`
	FileChecksums map[string]string `json:"file_checksums"`
	IndexedAt     time.Time         `json:"indexed_at"`
}

type Unit struct {
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	FQN           string   `json:"fqn,omitempty"`
	File          string   `json:"file"`
	Line          int      `json:"line"`
	Stereotype    string   `json:"stereotype,omitempty"`
	Methods       []Method `json:"methods,omitempty"`
	Dependencies  []string `json:"dependencies,omitempty"`
	ConfigRefs    []string `json:"config_refs,omitempty"`
	KafkaProduces []string `json:"kafka_produces,omitempty"`
	KafkaConsumes []string `json:"kafka_consumes,omitempty"`
	Endpoints     []string `json:"endpoints,omitempty"`
	ApiCalls      []string `json:"api_calls,omitempty"`
	Imports       []string `json:"imports,omitempty"`
}

type Method struct {
	Name        string   `json:"name"`
	Line        int      `json:"line"`
	ReturnType  string   `json:"return_type,omitempty"`
	Parameters  []string `json:"parameters,omitempty"`
	Annotations []string `json:"annotations,omitempty"`
	HTTPMethod  string   `json:"http_method,omitempty"`
	HTTPPath    string   `json:"http_path,omitempty"`
}
```

- [ ] **Step 3: Write graph model types**

Create `model/graph.go`:

```go
package model

import "time"

type EdgeType string

const (
	HTTPCall        EdgeType = "HTTP_CALL"
	KafkaProduce    EdgeType = "KAFKA_PRODUCE"
	KafkaConsume    EdgeType = "KAFKA_CONSUME"
	FrontendAPICall EdgeType = "FRONTEND_API_CALL"
	BuildDependency EdgeType = "BUILD_DEPENDENCY"
	ImportDep       EdgeType = "IMPORT_DEPENDENCY"
)

type Graph struct {
	Version      int              `json:"version"`
	Nodes        []Node           `json:"nodes"`
	Edges        []Edge           `json:"edges"`
	KeywordIndex map[string][]Hit `json:"keyword_index,omitempty"`
	GeneratedAt  time.Time        `json:"generated_at"`
}

type Node struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Workspace string `json:"workspace"`
}

type Edge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Type     EdgeType `json:"type"`
	Evidence string   `json:"evidence,omitempty"`
}

type Hit struct {
	ServiceKey string `json:"service_key"`
	UnitType   string `json:"unit_type"`
	UnitName   string `json:"unit_name"`
	Score      int    `json:"score"`
}
```

- [ ] **Step 4: Write manifest and health types**

Create `model/manifest.go`:

```go
package model

import "time"

type Manifest struct {
	Version     int            `json:"version"`
	Services    []ServiceEntry `json:"services"`
	GeneratedAt time.Time      `json:"generated_at"`
}

type ServiceEntry struct {
	Key         string `json:"key"`
	Path        string `json:"path"`
	Platform    string `json:"platform"`
	HeadSHA     string `json:"head_sha"`
	FileCount   int    `json:"file_count"`
	UnitCount   int    `json:"unit_count"`
	LastIndexed string `json:"last_indexed"`
}
```

Create `model/health.go`:

```go
package model

import "time"

type CrawlHealth struct {
	TotalRepos    int           `json:"total_repos"`
	Indexed       int           `json:"indexed"`
	Failed        []RepoFailure `json:"failed,omitempty"`
	CrawlDuration string        `json:"crawl_duration"`
	Timestamp     time.Time     `json:"timestamp"`
}

type RepoFailure struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
```

- [ ] **Step 5: Write trace context type**

Create `model/trace.go`:

```go
package model

import "time"

type Confidence int

const (
	High Confidence = iota
	Medium
	Low
)

func (c Confidence) String() string {
	switch c {
	case High:
		return "HIGH"
	case Medium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

type TraceContext struct {
	Confidence        Confidence `json:"confidence"`
	ServicesConsulted  []string   `json:"services_consulted"`
	DataSources       []string   `json:"data_sources"`
	IndexAge          string     `json:"index_age"`
	StaleServices     []string   `json:"stale_services,omitempty"`
	IndexTimestamp    time.Time  `json:"index_timestamp"`
}
```

- [ ] **Step 6: Verify all types compile**

```bash
cd /Users/anurag/Documents/nexus
go build ./model/...
```

Expected: clean build, no errors.

- [ ] **Step 7: Commit**

```bash
git add model/
git commit -m "feat: add shared model types — ServiceIndex, Graph, Manifest, TraceContext"
```

---

### Task 3: Config Loading

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type Config struct` — full config structure matching `~/.nexus/config.yml`
  - `func Load() (*Config, error)` — loads from `~/.nexus/config.yml` with defaults
  - `func (c *Config) StateDir() string` — returns `~/.nexus/state/`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
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
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	err := os.WriteFile(cfgPath, []byte(`
workspaces:
  - path: /tmp/test-workspace
embeddings:
  provider: none
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
	if cfg.Workspaces[0].Path != "/tmp/test-workspace" {
		t.Fatalf("unexpected workspace path: %s", cfg.Workspaces[0].Path)
	}
	if cfg.Embeddings.Provider != "none" {
		t.Fatalf("expected provider none, got %s", cfg.Embeddings.Provider)
	}
	if cfg.Agent.MaxFilesChanged != 5 {
		t.Fatalf("expected max_files_changed 5, got %d", cfg.Agent.MaxFilesChanged)
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/anurag/Documents/nexus
go test ./internal/config/ -v
```

Expected: compilation error — package and types don't exist yet.

- [ ] **Step 3: Write the config implementation**

Create `internal/config/config.go`:

```go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Workspaces []Workspace    `yaml:"workspaces"`
	GitHub     GitHubConfig   `yaml:"github"`
	Embeddings EmbeddingsCfg  `yaml:"embeddings"`
	Jira       ServiceCfg     `yaml:"jira"`
	Confluence ServiceCfg     `yaml:"confluence"`
	Postman    PostmanCfg     `yaml:"postman"`
	LLM        LLMConfig      `yaml:"llm"`
	Agent      AgentConfig    `yaml:"agent"`
	Schedule   ScheduleConfig `yaml:"schedule"`
	Crawl      CrawlConfig    `yaml:"crawl"`
}

type Workspace struct {
	Path string `yaml:"path"`
}

type GitHubConfig struct {
	SyncTargets []SyncTarget `yaml:"sync_targets"`
	TokenEnv    string       `yaml:"token_env"`
	Exclude     []string     `yaml:"exclude"`
}

type SyncTarget struct {
	Org       string `yaml:"org"`
	SSO       bool   `yaml:"sso"`
	CloneInto string `yaml:"clone_into"`
}

type EmbeddingsCfg struct {
	Provider      string `yaml:"provider"`
	OllamaURL     string `yaml:"ollama_url"`
	Model         string `yaml:"model"`
	OnnxModelPath string `yaml:"onnx_model_path"`
}

type ServiceCfg struct {
	BaseURL  string `yaml:"base_url"`
	TokenEnv string `yaml:"token_env"`
}

type PostmanCfg struct {
	APIKeyEnv string `yaml:"api_key_env"`
}

type LLMConfig struct {
	Provider   string `yaml:"provider"`    // anthropic | openai | ollama | custom
	Model      string `yaml:"model"`       // model name (provider-specific)
	APIKeyEnv  string `yaml:"api_key_env"` // env var holding the API key
	BaseURL    string `yaml:"base_url"`    // custom endpoint (for Ollama, Azure, proxies)
	MaxRetries int    `yaml:"max_retries"`
}

type MCPConfig struct {
	Client string `yaml:"client"` // informational only — MCP protocol is standard
}

type AgentConfig struct {
	AutoPush        bool   `yaml:"auto_push"`
	AutoTransition  bool   `yaml:"auto_transition"`
	MaxFilesChanged int    `yaml:"max_files_changed"`
	RequireTests    bool   `yaml:"require_tests"`
	BranchPrefix    string `yaml:"branch_prefix"`
}

type ScheduleConfig struct {
	CrawlInterval string `yaml:"crawl_interval"`
}

type CrawlConfig struct {
	GitPullConcurrency int `yaml:"git_pull_concurrency"`
	ParseConcurrency   int `yaml:"parse_concurrency"`
	EmbedConcurrency   int `yaml:"embed_concurrency"`
}

func Defaults() *Config {
	return &Config{
		Embeddings: EmbeddingsCfg{
			Provider:  "ollama",
			OllamaURL: "http://localhost:11434",
			Model:     "nomic-embed-text",
		},
		LLM: LLMConfig{
			Provider:   "anthropic",
			Model:      "claude-sonnet-5",
			APIKeyEnv:  "ANTHROPIC_API_KEY",
			MaxRetries: 3,
		},
		Agent: AgentConfig{
			MaxFilesChanged: 10,
			RequireTests:    true,
			BranchPrefix:    "nexus/",
		},
		Schedule: ScheduleConfig{
			CrawlInterval: "10m",
		},
		Crawl: CrawlConfig{
			GitPullConcurrency: 20,
			ParseConcurrency:   8,
			EmbedConcurrency:   2,
		},
		GitHub: GitHubConfig{
			TokenEnv: "GITHUB_TOKEN",
		},
		Jira: ServiceCfg{
			TokenEnv: "NEXUS_JIRA_TOKEN",
		},
		Confluence: ServiceCfg{
			TokenEnv: "NEXUS_CONFLUENCE_TOKEN",
		},
		Postman: PostmanCfg{
			APIKeyEnv: "NEXUS_POSTMAN_API_KEY",
		},
	}
}

func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Defaults(), nil
	}
	return LoadFrom(filepath.Join(home, ".nexus", "config.yml"))
}

func LoadFrom(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) StateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nexus/state"
	}
	return filepath.Join(home, ".nexus", "state")
}

func (c *Config) ExpandedWorkspaces() []string {
	home, _ := os.UserHomeDir()
	paths := make([]string, len(c.Workspaces))
	for i, w := range c.Workspaces {
		p := w.Path
		if len(p) > 1 && p[0] == '~' {
			p = filepath.Join(home, p[1:])
		}
		paths[i] = p
	}
	return paths
}
```

- [ ] **Step 4: Add yaml.v3 dependency and run tests**

```bash
cd /Users/anurag/Documents/nexus
go get gopkg.in/yaml.v3@latest
go mod tidy
go test ./internal/config/ -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: config loading from ~/.nexus/config.yml with defaults"
```

---

### Task 4: Sharded Storage Engine + Atomic Writes

**Files:**
- Create: `internal/store/atomic.go`
- Create: `internal/store/atomic_test.go`
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `model.ServiceIndex`, `model.Manifest`, `model.CrawlHealth`, `model.Graph`
- Produces:
  - `func AtomicWriteJSON(path string, v any) error` — write to .tmp, rename
  - `type Store struct` — sharded storage engine
  - `func NewStore(stateDir string) *Store`
  - `func (s *Store) SaveService(key string, idx *model.ServiceIndex) error`
  - `func (s *Store) LoadService(key string) (*model.ServiceIndex, error)`
  - `func (s *Store) LoadAllServices() (map[string]*model.ServiceIndex, error)`
  - `func (s *Store) SaveManifest(m *model.Manifest) error`
  - `func (s *Store) LoadManifest() (*model.Manifest, error)`
  - `func (s *Store) SaveHealth(h *model.CrawlHealth) error`
  - `func (s *Store) LoadHealth() (*model.CrawlHealth, error)`
  - `func (s *Store) SaveGraph(g *model.Graph) error`
  - `func (s *Store) LoadGraph() (*model.Graph, error)`

- [ ] **Step 1: Write the failing tests for atomic writes**

Create `internal/store/atomic_test.go`:

```go
package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := map[string]string{"hello": "world"}
	if err := AtomicWriteJSON(path, data); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(content) == 0 {
		t.Fatal("file should not be empty")
	}

	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp file should not exist after atomic write")
	}
}

func TestAtomicWriteJSONCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "test.json")

	if err := AtomicWriteJSON(path, "hello"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/anurag/Documents/nexus
go test ./internal/store/ -v -run TestAtomic
```

Expected: compilation error.

- [ ] **Step 3: Implement atomic writes**

Create `internal/store/atomic.go`:

```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func AtomicWriteJSON(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, path)
}

func ReadJSON[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
```

- [ ] **Step 4: Run atomic tests to verify they pass**

```bash
cd /Users/anurag/Documents/nexus
go test ./internal/store/ -v -run TestAtomic
```

Expected: both tests PASS.

- [ ] **Step 5: Write failing tests for the Store**

Create `internal/store/store_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/anurag/nexus/model"
)

func TestSaveAndLoadService(t *testing.T) {
	s := NewStore(t.TempDir())

	idx := &model.ServiceIndex{
		Key:      "backend:orders-service",
		Path:     "backend/orders-service",
		Platform: model.Java,
		Units: []model.Unit{
			{Type: "class", Name: "OrderController", File: "src/OrderController.java", Line: 10},
		},
		FileChecksums: map[string]string{"src/OrderController.java": "sha256:abc123"},
		IndexedAt:     time.Now(),
	}

	if err := s.SaveService(idx.Key, idx); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadService(idx.Key)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Key != idx.Key {
		t.Fatalf("expected key %s, got %s", idx.Key, loaded.Key)
	}
	if len(loaded.Units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(loaded.Units))
	}
	if loaded.Units[0].Name != "OrderController" {
		t.Fatalf("expected OrderController, got %s", loaded.Units[0].Name)
	}
}

func TestLoadAllServices(t *testing.T) {
	s := NewStore(t.TempDir())

	s.SaveService("svc-a", &model.ServiceIndex{Key: "svc-a", FileChecksums: map[string]string{}})
	s.SaveService("svc-b", &model.ServiceIndex{Key: "svc-b", FileChecksums: map[string]string{}})

	all, err := s.LoadAllServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 services, got %d", len(all))
	}
	if all["svc-a"] == nil || all["svc-b"] == nil {
		t.Fatal("both services should be loaded")
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	s := NewStore(t.TempDir())

	m := &model.Manifest{
		Version: 1,
		Services: []model.ServiceEntry{
			{Key: "svc-a", Platform: "java", FileCount: 42},
		},
		GeneratedAt: time.Now(),
	}

	if err := s.SaveManifest(m); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Services) != 1 {
		t.Fatalf("expected 1 service entry, got %d", len(loaded.Services))
	}
	if loaded.Services[0].FileCount != 42 {
		t.Fatalf("expected file_count 42, got %d", loaded.Services[0].FileCount)
	}
}

func TestSaveAndLoadHealth(t *testing.T) {
	s := NewStore(t.TempDir())

	h := &model.CrawlHealth{
		TotalRepos:    100,
		Indexed:       98,
		Failed:        []model.RepoFailure{{Name: "broken-repo", Reason: "auth 401"}},
		CrawlDuration: "45s",
		Timestamp:     time.Now(),
	}

	if err := s.SaveHealth(h); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadHealth()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TotalRepos != 100 {
		t.Fatalf("expected 100, got %d", loaded.TotalRepos)
	}
	if loaded.Indexed != 98 {
		t.Fatalf("expected 98, got %d", loaded.Indexed)
	}
	if len(loaded.Failed) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(loaded.Failed))
	}
}

func TestLoadServiceNotFound(t *testing.T) {
	s := NewStore(t.TempDir())

	_, err := s.LoadService("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent service")
	}
}

func TestLoadManifestNotFound(t *testing.T) {
	s := NewStore(t.TempDir())

	m, err := s.LoadManifest()
	if err != nil {
		t.Fatal("missing manifest should not error")
	}
	if m == nil {
		t.Fatal("should return empty manifest")
	}
	if len(m.Services) != 0 {
		t.Fatal("should have no services")
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

```bash
cd /Users/anurag/Documents/nexus
go test ./internal/store/ -v
```

Expected: compilation error — `NewStore` doesn't exist yet.

- [ ] **Step 7: Implement the Store**

Create `internal/store/store.go`:

```go
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anurag/nexus/model"
)

type Store struct {
	dir string
}

func NewStore(stateDir string) *Store {
	return &Store{dir: stateDir}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) serviceDir() string {
	return filepath.Join(s.dir, "services")
}

func (s *Store) servicePath(key string) string {
	safe := strings.ReplaceAll(key, ":", "_")
	return filepath.Join(s.serviceDir(), safe+".json")
}

func (s *Store) SaveService(key string, idx *model.ServiceIndex) error {
	return AtomicWriteJSON(s.servicePath(key), idx)
}

func (s *Store) LoadService(key string) (*model.ServiceIndex, error) {
	return ReadJSON[model.ServiceIndex](s.servicePath(key))
}

func (s *Store) LoadAllServices() (map[string]*model.ServiceIndex, error) {
	dir := s.serviceDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*model.ServiceIndex{}, nil
		}
		return nil, err
	}

	result := make(map[string]*model.ServiceIndex, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		svc, err := ReadJSON[model.ServiceIndex](path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		result[svc.Key] = svc
	}
	return result, nil
}

func (s *Store) DeleteService(key string) error {
	return os.Remove(s.servicePath(key))
}

func (s *Store) SaveManifest(m *model.Manifest) error {
	return AtomicWriteJSON(filepath.Join(s.dir, "manifest.json"), m)
}

func (s *Store) LoadManifest() (*model.Manifest, error) {
	m, err := ReadJSON[model.Manifest](filepath.Join(s.dir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &model.Manifest{Version: 1}, nil
		}
		return nil, err
	}
	return m, nil
}

func (s *Store) SaveHealth(h *model.CrawlHealth) error {
	return AtomicWriteJSON(filepath.Join(s.dir, "health.json"), h)
}

func (s *Store) LoadHealth() (*model.CrawlHealth, error) {
	h, err := ReadJSON[model.CrawlHealth](filepath.Join(s.dir, "health.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &model.CrawlHealth{}, nil
		}
		return nil, err
	}
	return h, nil
}

func (s *Store) SaveGraph(g *model.Graph) error {
	return AtomicWriteJSON(filepath.Join(s.dir, "graph.json"), g)
}

func (s *Store) LoadGraph() (*model.Graph, error) {
	g, err := ReadJSON[model.Graph](filepath.Join(s.dir, "graph.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &model.Graph{Version: 1}, nil
		}
		return nil, err
	}
	return g, nil
}
```

- [ ] **Step 8: Run all tests to verify they pass**

```bash
cd /Users/anurag/Documents/nexus
go test ./internal/store/ -v
```

Expected: all 7 tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/store/
git commit -m "feat: sharded storage engine with atomic writes and per-service JSON files"
```

---

### Task 5: CLI Commands (Cobra Stubs)

**Files:**
- Create: `cmd/nexus/crawl.go`
- Create: `cmd/nexus/analyze.go`
- Create: `cmd/nexus/serve.go`
- Create: `cmd/nexus/work.go`
- Create: `cmd/nexus/pipeline.go`
- Create: `cmd/nexus/status.go`

**Interfaces:**
- Consumes: `config.Load()`, `store.NewStore()`
- Produces: All Cobra subcommands registered on `rootCmd`. `nexus status` is fully functional — others print "not implemented yet" with correct flag definitions.

- [ ] **Step 1: Create crawl command stub**

Create `cmd/nexus/crawl.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var crawlCmd = &cobra.Command{
	Use:   "crawl [workspace-dirs...]",
	Short: "Stage 1: discover repos and parse source files",
	Long:  "Walk workspace directories, detect platforms, parse source files with tree-sitter, and write per-service index shards.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("nexus crawl: not implemented yet")
		return nil
	},
}

func init() {
	crawlCmd.Flags().Bool("github-sync", false, "Also clone new repos from configured GitHub orgs")
	crawlCmd.Flags().Bool("full", false, "Force full re-parse, ignore checksums")
	crawlCmd.Flags().Bool("watch", false, "Watch workspace dirs for changes and re-crawl")
	rootCmd.AddCommand(crawlCmd)
}
```

- [ ] **Step 2: Create analyze command stub**

Create `cmd/nexus/analyze.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Stage 2: build dependency graph and generate embeddings",
	Long:  "Read service index shards, run 5 graph resolvers, build keyword index, and generate embeddings.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("nexus analyze: not implemented yet")
		return nil
	},
}

func init() {
	analyzeCmd.Flags().Bool("skip-embeddings", false, "Skip embedding generation (no Ollama needed)")
	rootCmd.AddCommand(analyzeCmd)
}
```

- [ ] **Step 3: Create serve command stub**

Create `cmd/nexus/serve.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Stage 3: start MCP server over stdio",
	Long:  "Start JSON-RPC 2.0 MCP server on stdin/stdout for AI agent integration.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("nexus serve: not implemented yet")
		return nil
	},
}

func init() {
	serveCmd.Flags().Bool("watch", false, "Auto-reload index on changes")
	rootCmd.AddCommand(serveCmd)
}
```

- [ ] **Step 4: Create work command stub**

Create `cmd/nexus/work.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var workCmd = &cobra.Command{
	Use:   "work [ticket-keys...]",
	Short: "Autonomous SDLC: read Jira ticket, investigate, plan, execute, ship",
	Long:  "Full autonomous pipeline: classify ticket, investigate with Nexus tools, plan via LLM API, execute, verify tests, create PR.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("nexus work %v: not implemented yet\n", args)
		return nil
	},
}

func init() {
	workCmd.Flags().Bool("dry-run", false, "Show plan without executing")
	workCmd.Flags().Bool("approve-each-step", false, "Pause for approval at each step")
	workCmd.Flags().String("from-phase", "", "Resume from a specific phase (understand|investigate|plan|execute|verify|ship)")
	rootCmd.AddCommand(workCmd)
}
```

- [ ] **Step 5: Create pipeline command stub**

Create `cmd/nexus/pipeline.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pipelineCmd = &cobra.Command{
	Use:   "pipeline [workspace-dirs...]",
	Short: "Run crawl + analyze in sequence",
	Long:  "Shortcut for running Stage 1 (crawl) followed by Stage 2 (analyze).",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("nexus pipeline: not implemented yet")
		return nil
	},
}

func init() {
	pipelineCmd.Flags().Bool("github-sync", false, "Also clone new repos from configured GitHub orgs")
	pipelineCmd.Flags().Bool("skip-embeddings", false, "Skip embedding generation")
	rootCmd.AddCommand(pipelineCmd)
}
```

- [ ] **Step 6: Create the real status command**

Create `cmd/nexus/status.go`:

```go
package main

import (
	"fmt"
	"time"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index status: service count, graph stats, health",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		s := store.NewStore(cfg.StateDir())

		manifest, err := s.LoadManifest()
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}

		health, err := s.LoadHealth()
		if err != nil {
			return fmt.Errorf("loading health: %w", err)
		}

		graph, err := s.LoadGraph()
		if err != nil {
			return fmt.Errorf("loading graph: %w", err)
		}

		fmt.Println("Nexus Status")
		fmt.Println("============")

		if len(manifest.Services) == 0 {
			fmt.Println("\nNo index found. Run 'nexus crawl' to index your workspaces.")
			fmt.Printf("State dir: %s\n", cfg.StateDir())
			fmt.Printf("Workspaces configured: %d\n", len(cfg.Workspaces))
			for _, w := range cfg.Workspaces {
				fmt.Printf("  - %s\n", w.Path)
			}
			return nil
		}

		fmt.Printf("\nServices:    %d\n", len(manifest.Services))
		fmt.Printf("Graph nodes: %d\n", len(graph.Nodes))
		fmt.Printf("Graph edges: %d\n", len(graph.Edges))

		if !manifest.GeneratedAt.IsZero() {
			age := time.Since(manifest.GeneratedAt).Truncate(time.Second)
			fmt.Printf("Index age:   %s\n", age)
		}

		platforms := map[string]int{}
		totalFiles := 0
		for _, svc := range manifest.Services {
			platforms[svc.Platform]++
			totalFiles += svc.FileCount
		}

		fmt.Printf("Files:       %d\n", totalFiles)
		fmt.Printf("Platforms:   ")
		first := true
		for p, count := range platforms {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%s(%d)", p, count)
			first = false
		}
		fmt.Println()

		if health.TotalRepos > 0 {
			fmt.Printf("\nCrawl Health\n")
			fmt.Printf("  Repos:     %d/%d indexed\n", health.Indexed, health.TotalRepos)
			fmt.Printf("  Duration:  %s\n", health.CrawlDuration)
			if len(health.Failed) > 0 {
				fmt.Printf("  Failed:    %d\n", len(health.Failed))
				for _, f := range health.Failed {
					fmt.Printf("    - %s: %s\n", f.Name, f.Reason)
				}
			}
		}

		fmt.Printf("\nWorkspaces:  %d configured\n", len(cfg.Workspaces))
		for _, w := range cfg.Workspaces {
			fmt.Printf("  - %s\n", w.Path)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
```

- [ ] **Step 7: Build and verify all commands**

```bash
cd /Users/anurag/Documents/nexus
make build
./bin/nexus --help
./bin/nexus crawl --help
./bin/nexus analyze --help
./bin/nexus serve --help
./bin/nexus work --help
./bin/nexus pipeline --help
./bin/nexus status
```

Expected: help shows all 6 subcommands with correct flags. `nexus status` prints "No index found" message with state dir path.

- [ ] **Step 8: Commit**

```bash
git add cmd/nexus/
git commit -m "feat: CLI scaffold with all Cobra commands — status is functional, others are stubs"
```

---

### Task 6: run-mcp.sh Launcher + End-to-End Smoke Test

**Files:**
- Create: `run-mcp.sh`
- Create: `test/smoke_test.go`

**Interfaces:**
- Consumes: `make build`, `config.Load()`, `store.NewStore()`
- Produces:
  - `run-mcp.sh` — builds if needed, launches `nexus serve` with env vars
  - Smoke test that exercises the full Task 1-5 chain: config → store → manifest → status output

- [ ] **Step 1: Create run-mcp.sh**

Create `run-mcp.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$SCRIPT_DIR/bin/nexus"

if [[ ! -f "$BINARY" ]] || [[ "$SCRIPT_DIR/go.sum" -nt "$BINARY" ]]; then
    echo "Building nexus..." >&2
    cd "$SCRIPT_DIR"
    make build >&2
fi

exec "$BINARY" serve "$@"
```

```bash
chmod +x /Users/anurag/Documents/nexus/run-mcp.sh
```

- [ ] **Step 2: Write the smoke test**

Create `test/smoke_test.go`:

```go
package test

import (
	"testing"
	"time"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
)

func TestFoundationSmokeTest(t *testing.T) {
	cfg := config.Defaults()
	if cfg.Embeddings.Provider != "ollama" {
		t.Fatal("bad default")
	}

	dir := t.TempDir()
	s := store.NewStore(dir)

	svc := &model.ServiceIndex{
		Key:      "backend:orders",
		Path:     "backend/orders",
		Platform: model.Java,
		Units: []model.Unit{
			{
				Type:       "class",
				Name:       "OrderController",
				File:       "src/main/java/OrderController.java",
				Line:       15,
				Stereotype: "rest_controller",
				Methods: []model.Method{
					{Name: "getOrder", Line: 20, HTTPMethod: "GET", HTTPPath: "/api/orders/{id}"},
				},
				Endpoints:     []string{"GET /api/orders/{id}"},
				KafkaProduces: []string{"order.created"},
			},
		},
		Config:        map[string]string{"payment.url": "http://payment-svc:8080"},
		BuildDeps:     []string{"spring-boot-starter-web"},
		FileChecksums: map[string]string{"src/main/java/OrderController.java": "sha256:abc123"},
		IndexedAt:     time.Now(),
	}

	if err := s.SaveService(svc.Key, svc); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadService(svc.Key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Units[0].Methods[0].HTTPPath != "/api/orders/{id}" {
		t.Fatal("deep field roundtrip failed")
	}
	if loaded.Units[0].KafkaProduces[0] != "order.created" {
		t.Fatal("kafka topic roundtrip failed")
	}

	manifest := &model.Manifest{
		Version: 1,
		Services: []model.ServiceEntry{
			{Key: "backend:orders", Platform: "java", FileCount: 42, UnitCount: 1},
		},
		GeneratedAt: time.Now(),
	}
	s.SaveManifest(manifest)

	health := &model.CrawlHealth{
		TotalRepos: 10, Indexed: 9,
		Failed:        []model.RepoFailure{{Name: "broken", Reason: "auth"}},
		CrawlDuration: "3s",
		Timestamp:     time.Now(),
	}
	s.SaveHealth(health)

	graph := &model.Graph{
		Version: 1,
		Nodes:   []model.Node{{Key: "backend:orders", Name: "orders", Platform: "java"}},
		Edges: []model.Edge{
			{From: "backend:orders", To: "backend:payments", Type: model.HTTPCall, Evidence: "config: payment.url"},
		},
		GeneratedAt: time.Now(),
	}
	s.SaveGraph(graph)

	m, _ := s.LoadManifest()
	if len(m.Services) != 1 {
		t.Fatal("manifest roundtrip failed")
	}
	h, _ := s.LoadHealth()
	if h.Indexed != 9 {
		t.Fatal("health roundtrip failed")
	}
	g, _ := s.LoadGraph()
	if len(g.Edges) != 1 || g.Edges[0].Type != model.HTTPCall {
		t.Fatal("graph roundtrip failed")
	}

	all, _ := s.LoadAllServices()
	if len(all) != 1 {
		t.Fatal("LoadAllServices failed")
	}

	if model.DetectPlatform(t.TempDir()) != model.Unknown {
		t.Fatal("empty dir should detect as Unknown")
	}
}
```

- [ ] **Step 3: Run full test suite**

```bash
cd /Users/anurag/Documents/nexus
go test ./... -v -count=1
```

Expected: all tests PASS across config, store, and smoke test.

- [ ] **Step 4: Run make build and verify status output**

```bash
cd /Users/anurag/Documents/nexus
make build
./bin/nexus status
./bin/nexus --version
```

Expected: `status` shows "No index found" with workspace info. Version prints correctly.

- [ ] **Step 5: Commit**

```bash
git add run-mcp.sh test/
git commit -m "feat: MCP launcher script and end-to-end smoke test for foundation"
```

---

## Summary

| Task | What | Tests | Files |
|------|------|-------|-------|
| 1 | Go module + Makefile + Cobra root | build check | 4 |
| 2 | Model types (ServiceIndex, Graph, Manifest, TraceContext) | compile check | 6 |
| 3 | Config loading from ~/.nexus/config.yml | 4 tests | 2 |
| 4 | Sharded storage + atomic writes | 7 tests | 4 |
| 5 | CLI commands (6 subcommands, status is real) | manual verify | 6 |
| 6 | run-mcp.sh + smoke test | 1 integration test | 2 |
| **Total** | | **12+ tests** | **24 files** |

After this phase: `nexus status` works, all other commands are wired up as stubs ready for Phase 2 (Crawler), the storage engine handles sharded per-service files with atomic writes, and all model types are defined as the contract between stages.
