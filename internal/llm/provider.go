package llm

import "encoding/json"

type Provider interface {
	Complete(req CompletionRequest) (*CompletionResponse, error)
	Converse(req ConverseRequest) (*ConverseResponse, error)
	Available() bool
	Ping() error
	Name() string
	Model() string
	MaxOutputTokens() int
}

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type CompletionResponse struct {
	Content    string
	TokensUsed int
	Model      string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type ConversationMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

func TextMessage(role, text string) ConversationMessage {
	return ConversationMessage{
		Role:    role,
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func ToolResultMessage(toolUseID, content string) ConversationMessage {
	return ConversationMessage{
		Role: "user",
		Content: []ContentBlock{{
			Type:      "tool_result",
			ToolUseID: toolUseID,
			Content:   content,
		}},
	}
}

func ToolResultsMessage(results []ContentBlock) ConversationMessage {
	return ConversationMessage{
		Role:    "user",
		Content: results,
	}
}

type ConverseRequest struct {
	SystemPrompt string
	Messages     []ConversationMessage
	Tools        []Tool
	MaxTokens    int
}

type ConverseResponse struct {
	StopReason string
	Content    []ContentBlock
	TokensUsed int
	Model      string
}

func (r *ConverseResponse) TextContent() string {
	for _, b := range r.Content {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}

func (r *ConverseResponse) ToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, b := range r.Content {
		if b.Type == "tool_use" {
			calls = append(calls, b)
		}
	}
	return calls
}
