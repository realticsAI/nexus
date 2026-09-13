package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type AnthropicProvider struct {
	apiKey     string
	model      string
	baseURL    string
	maxRetries int
	client     *http.Client
}

func NewAnthropicProvider(model, apiKeyEnv, baseURL string, maxRetries int) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &AnthropicProvider{
		apiKey:     os.Getenv(apiKeyEnv),
		model:      model,
		baseURL:    baseURL,
		maxRetries: maxRetries,
		client:     &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *AnthropicProvider) Name() string  { return "anthropic" }
func (a *AnthropicProvider) Model() string { return a.model }

func (a *AnthropicProvider) MaxOutputTokens() int {
	return modelMaxOutputTokens(a.model)
}

func (a *AnthropicProvider) Available() bool {
	return a.Ping() == nil
}

func (a *AnthropicProvider) Ping() error {
	if a.apiKey == "" {
		return fmt.Errorf("anthropic: API key not set")
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      a.model,
		"max_tokens": 1,
		"messages":   []Message{{Role: "user", Content: "hi"}},
	})
	req, _ := http.NewRequest("POST", a.baseURL+"/v1/messages", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic: cannot reach API — %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("anthropic: invalid API key")
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("anthropic: model %s not found", a.model)
	}
	if resp.StatusCode == 429 {
		return nil // rate-limited means auth works
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("anthropic: server error %d — try again", resp.StatusCode)
	}
	return nil
}

func (a *AnthropicProvider) Complete(req CompletionRequest) (*CompletionResponse, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: API key not set")
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := map[string]any{
		"model":      a.model,
		"max_tokens": maxTokens,
		"messages":   []Message{{Role: "user", Content: req.UserPrompt}},
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	payload, _ := json.Marshal(body)

	var lastErr error
	for attempt := 0; attempt < a.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		httpReq, err := http.NewRequest("POST", a.baseURL+"/v1/messages", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", a.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		resp, err := a.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("anthropic request failed: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
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
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("anthropic response parse error: %w", err)
		}

		text := ""
		if len(result.Content) > 0 {
			text = result.Content[0].Text
		}

		return &CompletionResponse{
			Content:    text,
			TokensUsed: result.Usage.InputTokens + result.Usage.OutputTokens,
			Model:      result.Model,
		}, nil
	}

	return nil, fmt.Errorf("anthropic: exhausted retries: %w", lastErr)
}

func (a *AnthropicProvider) Converse(req ConverseRequest) (*ConverseResponse, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: API key not set")
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
		"model":      a.model,
		"max_tokens": maxTokens,
		"messages":   apiMessages,
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
	for attempt := 0; attempt < a.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		httpReq, err := http.NewRequest("POST", a.baseURL+"/v1/messages", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", a.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		resp, err := a.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("anthropic request failed: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
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
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("anthropic response parse error: %w", err)
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

		return &ConverseResponse{
			StopReason: result.StopReason,
			Content:    blocks,
			TokensUsed: result.Usage.InputTokens + result.Usage.OutputTokens,
			Model:      result.Model,
		}, nil
	}

	return nil, fmt.Errorf("anthropic: exhausted retries: %w", lastErr)
}
