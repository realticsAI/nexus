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
	Figma      FigmaCfg       `yaml:"figma"`
	Postman    PostmanCfg     `yaml:"postman"`
	LLM        LLMConfig      `yaml:"llm"`
	Agent      AgentConfig    `yaml:"agent"`
	Schedule   ScheduleConfig  `yaml:"schedule"`
	Crawl      CrawlConfig     `yaml:"crawl"`
	MCP        MCPConfig       `yaml:"mcp"`
	Security   SecurityConfig  `yaml:"security"`
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

type FigmaCfg struct {
	TokenEnv string `yaml:"token_env"`
}

type PostmanCfg struct {
	APIKeyEnv string `yaml:"api_key_env"`
}

type LLMConfig struct {
	Provider   string `yaml:"provider"`    // anthropic | bedrock | ollama | custom
	Model      string `yaml:"model"`       // model name (provider-specific)
	APIKeyEnv  string `yaml:"api_key_env"` // env var holding the API key
	BaseURL    string `yaml:"base_url"`    // custom endpoint (for Ollama, Azure OpenAI, proxies)
	MaxRetries int    `yaml:"max_retries"`
	Region     string `yaml:"region"`      // AWS region for Bedrock (e.g. us-east-1)
	Profile    string `yaml:"profile"`     // AWS profile name (for SSO)
}

type MCPConfig struct {
	Client string `yaml:"client"` // informational only — MCP protocol is standard across all agents
}

type AgentConfig struct {
	AutoPush               bool   `yaml:"auto_push"`
	AutoTransition         bool   `yaml:"auto_transition"`
	MaxFilesChanged        int    `yaml:"max_files_changed"`
	RequireTests           bool   `yaml:"require_tests"`
	BranchPrefix           string `yaml:"branch_prefix"`
	GitHubOrg              string `yaml:"github_org"`
	DefaultConfluenceSpace string `yaml:"default_confluence_space"`
}

type ScheduleConfig struct {
	CrawlInterval string `yaml:"crawl_interval"`
}

type CrawlConfig struct {
	GitPullConcurrency int `yaml:"git_pull_concurrency"`
	ParseConcurrency   int `yaml:"parse_concurrency"`
	EmbedConcurrency   int `yaml:"embed_concurrency"`
}

type SecurityConfig struct {
	EncryptState  bool `yaml:"encrypt_state"`
	RedactSecrets bool `yaml:"redact_secrets"`
	ScanInjection bool `yaml:"scan_injection"`
	AuditLog      bool `yaml:"audit_log"`
}

func Defaults() *Config {
	return &Config{
		Embeddings: EmbeddingsCfg{
			Provider:  "ollama",
			OllamaURL: "http://localhost:11434",
			Model:     "nomic-embed-text",
		},
		LLM: LLMConfig{
			Provider:   "bedrock",
			Model:      "us.anthropic.claude-opus-4-6-v1",
			Region:     "us-east-1",
			Profile:    "AWS-EXAMPLE-BEDROCK-123456789012",
			MaxRetries: 3,
		},
		MCP: MCPConfig{
			Client: "generic",
		},
		Agent: AgentConfig{
			MaxFilesChanged:        10,
			RequireTests:           true,
			BranchPrefix:           "nexus/",
			GitHubOrg:              "ExampleOrg",
			DefaultConfluenceSpace: "PROJ",
		},
		Schedule: ScheduleConfig{
			CrawlInterval: "48h",
		},
		Crawl: CrawlConfig{
			GitPullConcurrency: 20,
			ParseConcurrency:   8,
			EmbedConcurrency:   2,
		},
		Security: SecurityConfig{
			EncryptState:  false,
			RedactSecrets: true,
			ScanInjection: true,
			AuditLog:      true,
		},
		GitHub:     GitHubConfig{TokenEnv: "GITHUB_TOKEN"},
		Jira:       ServiceCfg{BaseURL: "https://jira.example.com", TokenEnv: "HALO_ATLASSIAN_JIRA_PAT"},
		Confluence: ServiceCfg{BaseURL: "https://confluence.example.com", TokenEnv: "HALO_ATLASSIAN_CONFLUENCE_PAT"},
		Figma:      FigmaCfg{TokenEnv: "NEXUS_FIGMA_PAT"},
		Postman:    PostmanCfg{APIKeyEnv: "NEXUS_POSTMAN_API_KEY"},
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
