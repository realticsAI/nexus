package llm

import "github.com/anurag/nexus/internal/config"

func NewFromConfig(cfg config.LLMConfig) Provider {
	switch cfg.Provider {
	case "bedrock":
		return NewBedrockProvider(cfg.Model, cfg.Region, cfg.Profile, cfg.MaxRetries)
	case "ollama":
		return NewOllamaProvider(cfg.BaseURL, cfg.Model)
	case "anthropic", "":
		return NewAnthropicProvider(cfg.Model, cfg.APIKeyEnv, cfg.BaseURL, cfg.MaxRetries)
	default:
		return NewAnthropicProvider(cfg.Model, cfg.APIKeyEnv, cfg.BaseURL, cfg.MaxRetries)
	}
}

func NewFromConfigWithModel(cfg config.LLMConfig, modelOverride string) Provider {
	if modelOverride == "" {
		return NewFromConfig(cfg)
	}
	override := cfg
	switch cfg.Provider {
	case "bedrock":
		override.Model = ResolveBedrockModel(modelOverride)
	default:
		override.Model = modelOverride
	}
	return NewFromConfig(override)
}
