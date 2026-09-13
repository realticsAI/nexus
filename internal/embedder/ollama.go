package embedder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OllamaProvider struct {
	baseURL    string
	model      string
	client     *http.Client
	dimensions int
}

func NewOllamaProvider(baseURL, modelName string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		model:   modelName,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (o *OllamaProvider) Embed(texts []string) ([][]float32, error) {
	body, err := json.Marshal(ollamaEmbedRequest{
		Model: o.model,
		Input: texts,
	})
	if err != nil {
		return nil, err
	}

	resp, err := o.client.Post(o.baseURL+"/api/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding ollama response: %w", err)
	}

	if len(result.Embeddings) > 0 && o.dimensions == 0 {
		o.dimensions = len(result.Embeddings[0])
	}

	return result.Embeddings, nil
}

func (o *OllamaProvider) Dimensions() int {
	if o.dimensions == 0 {
		return 384
	}
	return o.dimensions
}

func (o *OllamaProvider) Available() bool {
	resp, err := o.client.Get(o.baseURL + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
