package mcp

type ToolHandler interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Run(args map[string]any) *ToolCallResult
}
