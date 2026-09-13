package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/anurag/nexus/internal/debug"
)

type BedrockProvider struct {
	client     *bedrockruntime.Client
	model      string
	region     string
	profile    string
	maxRetries int
	initErr    error
}

func NewBedrockProvider(model, region, profile string, maxRetries int) *BedrockProvider {
	if region == "" {
		region = "us-east-1"
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}

	p := &BedrockProvider{
		model:      model,
		region:     region,
		profile:    profile,
		maxRetries: maxRetries,
	}

	p.initErr = p.refreshClient()
	return p
}

func (b *BedrockProvider) refreshClient() error {
	debug.Log("llm", "refreshClient region=%s profile=%s", b.region, b.profile)
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(b.region),
	}
	if b.profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(b.profile))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	b.client = bedrockruntime.NewFromConfig(cfg)
	b.initErr = nil
	return nil
}

func (b *BedrockProvider) Name() string  { return "bedrock" }
func (b *BedrockProvider) Model() string { return b.model }

func (b *BedrockProvider) MaxOutputTokens() int {
	return modelMaxOutputTokens(b.model)
}

func (b *BedrockProvider) Available() bool {
	return b.Ping() == nil
}

func (b *BedrockProvider) isCredentialError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "expired") || strings.Contains(msg, "security token") ||
		strings.Contains(msg, "UnrecognizedClientException") || strings.Contains(msg, "get credentials") ||
		strings.Contains(msg, "get identity") || strings.Contains(msg, "context deadline exceeded")
}

func (b *BedrockProvider) Ping() error {
	debug.Log("llm", "Ping starting model=%s region=%s", b.model, b.region)
	for attempt := 0; attempt < 3; attempt++ {
		if b.initErr != nil {
			if attempt > 0 || b.isCredentialError(b.initErr) {
				if err := b.refreshClient(); err != nil {
					if attempt == 2 {
						return b.ssoHint(err)
					}
					time.Sleep(time.Duration(attempt+1) * 3 * time.Second)
					continue
				}
			} else {
				return b.ssoHint(b.initErr)
			}
		}
		if b.client == nil {
			return fmt.Errorf("bedrock client not initialized")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     strPtr(b.model),
			ContentType: strPtr("application/json"),
			Body:        []byte(`{"anthropic_version":"bedrock-2023-05-31","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`),
		})
		cancel()

		if err == nil {
			return nil
		}

		errMsg := err.Error()
		if strings.Contains(errMsg, "ThrottlingException") {
			return nil
		}
		if strings.Contains(errMsg, "AccessDeniedException") {
			return fmt.Errorf("no access to model %s — check Bedrock model access in AWS console for region %s", b.model, b.region)
		}
		if strings.Contains(errMsg, "ResourceNotFoundException") || strings.Contains(errMsg, "ValidationException") {
			return fmt.Errorf("model %s not found in region %s — check the model ID is correct", b.model, b.region)
		}

		if b.isCredentialError(err) {
			if attempt < 2 {
				fmt.Printf("nexus: credential error, refreshing session (attempt %d/3)...\n", attempt+2)
				time.Sleep(time.Duration(attempt+1) * 3 * time.Second)
				b.initErr = fmt.Errorf("%w", err)
				continue
			}
			return b.ssoHint(err)
		}

		return fmt.Errorf("bedrock ping failed for model %s: %w", b.model, err)
	}
	return fmt.Errorf("bedrock ping failed after 3 attempts")
}

func (b *BedrockProvider) Complete(req CompletionRequest) (*CompletionResponse, error) {
	debug.Log("llm", "Complete starting model=%s maxTokens=%d systemLen=%d userLen=%d", b.model, req.MaxTokens, len(req.SystemPrompt), len(req.UserPrompt))
	t := time.Now()
	if b.initErr != nil {
		return nil, b.ssoHint(b.initErr)
	}
	if b.client == nil {
		return nil, fmt.Errorf("bedrock client not initialized")
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        maxTokens,
		"messages":          []Message{{Role: "user", Content: req.UserPrompt}},
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	payload, _ := json.Marshal(body)

	var lastErr error
	credRetried := false
	for attempt := 0; attempt < b.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		output, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     strPtr(b.model),
			ContentType: strPtr("application/json"),
			Body:        payload,
		})
		cancel()

		if err != nil {
			if b.isCredentialError(err) && !credRetried {
				credRetried = true
				fmt.Println("nexus: credential error during invoke, refreshing session...")
				if refreshErr := b.refreshClient(); refreshErr == nil {
					attempt--
					continue
				}
				return nil, b.ssoHint(err)
			}
			errMsg := err.Error()
			if strings.Contains(errMsg, "ThrottlingException") || strings.Contains(errMsg, "ServiceUnavailable") {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("bedrock invoke failed: %w", err)
		}

		var result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal(output.Body, &result); err != nil {
			return nil, fmt.Errorf("bedrock response parse error: %w", err)
		}

		text := ""
		if len(result.Content) > 0 {
			text = result.Content[0].Text
		}

		debug.Log("llm", "Complete done model=%s input=%d output=%d total=%d took=%s", b.model, result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.InputTokens+result.Usage.OutputTokens, debug.Since(t))
		return &CompletionResponse{
			Content:    text,
			TokensUsed: result.Usage.InputTokens + result.Usage.OutputTokens,
			Model:      result.Model,
		}, nil
	}

	return nil, fmt.Errorf("bedrock: exhausted retries: %w", lastErr)
}

func (b *BedrockProvider) ssoHint(err error) error {
	cmd := "aws sso login"
	if b.profile != "" {
		cmd += " --profile " + b.profile
	}
	return fmt.Errorf("AWS credentials expired or invalid — run: %s\n  error: %w", cmd, err)
}

func (b *BedrockProvider) Converse(req ConverseRequest) (*ConverseResponse, error) {
	debug.Log("llm", "Converse starting model=%s maxTokens=%d messages=%d tools=%d", b.model, req.MaxTokens, len(req.Messages), len(req.Tools))
	t := time.Now()
	if b.initErr != nil {
		return nil, b.ssoHint(b.initErr)
	}
	if b.client == nil {
		return nil, fmt.Errorf("bedrock client not initialized")
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	apiMessages := make([]map[string]any, len(req.Messages))
	for i, msg := range req.Messages {
		blocks := make([]map[string]any, len(msg.Content))
		for j, b := range msg.Content {
			block := map[string]any{"type": b.Type}
			switch b.Type {
			case "text":
				block["text"] = b.Text
			case "tool_use":
				block["id"] = b.ID
				block["name"] = b.Name
				block["input"] = json.RawMessage(b.Input)
			case "tool_result":
				block["tool_use_id"] = b.ToolUseID
				block["content"] = b.Content
			}
			blocks[j] = block
		}
		apiMessages[i] = map[string]any{
			"role":    msg.Role,
			"content": blocks,
		}
	}

	body := map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        maxTokens,
		"messages":          apiMessages,
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, tool := range req.Tools {
			tools[i] = map[string]any{
				"name":         tool.Name,
				"description":  tool.Description,
				"input_schema": json.RawMessage(tool.InputSchema),
			}
		}
		body["tools"] = tools
	}

	payload, _ := json.Marshal(body)

	var lastErr error
	credRetried := false
	for attempt := 0; attempt < b.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		output, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     strPtr(b.model),
			ContentType: strPtr("application/json"),
			Body:        payload,
		})
		cancel()

		if err != nil {
			if b.isCredentialError(err) && !credRetried {
				credRetried = true
				fmt.Println("nexus: credential error during invoke, refreshing session...")
				if refreshErr := b.refreshClient(); refreshErr == nil {
					attempt--
					continue
				}
				return nil, b.ssoHint(err)
			}
			errMsg := err.Error()
			if strings.Contains(errMsg, "ThrottlingException") || strings.Contains(errMsg, "ServiceUnavailable") {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("bedrock invoke failed: %w", err)
		}

		var result struct {
			StopReason string `json:"stop_reason"`
			Content    []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text,omitempty"`
				ID    string          `json:"id,omitempty"`
				Name  string          `json:"name,omitempty"`
				Input json.RawMessage `json:"input,omitempty"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal(output.Body, &result); err != nil {
			return nil, fmt.Errorf("bedrock response parse error: %w", err)
		}

		var blocks []ContentBlock
		for _, c := range result.Content {
			blocks = append(blocks, ContentBlock{
				Type:  c.Type,
				Text:  c.Text,
				ID:    c.ID,
				Name:  c.Name,
				Input: c.Input,
			})
		}

		debug.Log("llm", "Converse done model=%s stop=%s blocks=%d input=%d output=%d took=%s",
			b.model, result.StopReason, len(blocks), result.Usage.InputTokens, result.Usage.OutputTokens, debug.Since(t))

		return &ConverseResponse{
			StopReason: result.StopReason,
			Content:    blocks,
			TokensUsed: result.Usage.InputTokens + result.Usage.OutputTokens,
			Model:      result.Model,
		}, nil
	}

	return nil, fmt.Errorf("bedrock: exhausted retries: %w", lastErr)
}

func strPtr(s string) *string { return &s }
