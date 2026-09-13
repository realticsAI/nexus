package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (o *OllamaProvider) Name() string  { return "ollama" }
func (o *OllamaProvider) Model() string { return o.model }

func (o *OllamaProvider) MaxOutputTokens() int {
	return modelMaxOutputTokens(o.model)
}

func (o *OllamaProvider) Available() bool {
	return o.Ping() == nil
}

func (o *OllamaProvider) Ping() error {
	resp, err := o.client.Get(o.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama: cannot reach server at %s — %w", o.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("ollama: server returned %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil // server is up, model check is best-effort
	}
	for _, m := range result.Models {
		if m.Name == o.model || fmt.Sprintf("%s:latest", o.model) == m.Name {
			return nil
		}
	}
	return fmt.Errorf("ollama: model %s not found — run 'ollama pull %s'", o.model, o.model)
}

func (o *OllamaProvider) Complete(req CompletionRequest) (*CompletionResponse, error) {
	prompt := req.UserPrompt
	if req.SystemPrompt != "" {
		prompt = req.SystemPrompt + "\n\n" + prompt
	}

	body, _ := json.Marshal(map[string]any{
		"model":  o.model,
		"prompt": prompt,
		"stream": false,
	})

	resp, err := o.client.Post(o.baseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama response parse error: %w", err)
	}

	return &CompletionResponse{
		Content: result.Response,
		Model:   result.Model,
	}, nil
}

func (o *OllamaProvider) Converse(req ConverseRequest) (*ConverseResponse, error) {
	return nil, fmt.Errorf("ollama: tool use (Converse) not supported — use bedrock or anthropic provider")
}
