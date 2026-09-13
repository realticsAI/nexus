package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anurag/nexus/internal/mcp"
)

type ReadFileTool struct{ ctx *Context }

func NewReadFileTool(ctx *Context) *ReadFileTool { return &ReadFileTool{ctx: ctx} }
func (t *ReadFileTool) Name() string             { return "read_file" }
func (t *ReadFileTool) Description() string      { return "Read a file from an indexed service repository." }
func (t *ReadFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service": map[string]any{"type": "string", "description": "Service key (e.g. backend:orders)"},
			"path":    map[string]any{"type": "string", "description": "File path relative to service root"},
		},
		"required": []string{"service", "path"},
	}
}
func (t *ReadFileTool) Run(args map[string]any) *mcp.ToolCallResult {
	service, _ := args["service"].(string)
	path, _ := args["path"].(string)
	svc, ok := t.ctx.Services[service]
	if !ok {
		return mcp.ErrorResult(fmt.Sprintf("service not found: %s", service))
	}

	// Path jail: resolve and verify the path stays within the service root
	fullPath := filepath.Join(svc.Path, path)
	resolved, err := filepath.Abs(fullPath)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}
	svcRoot, _ := filepath.Abs(svc.Path)
	if !strings.HasPrefix(resolved, svcRoot+string(filepath.Separator)) && resolved != svcRoot {
		return mcp.ErrorResult("access denied: path traversal outside service root")
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("cannot read file: %v", err))
	}
	if len(data) > 100000 {
		data = data[:100000]
	}
	return mcp.TextResult(string(data))
}

type ListServicesTool struct{ ctx *Context }

func NewListServicesTool(ctx *Context) *ListServicesTool { return &ListServicesTool{ctx: ctx} }
func (t *ListServicesTool) Name() string                 { return "list_services" }
func (t *ListServicesTool) Description() string {
	return "List all indexed services with their platform, unit count, and workspace."
}
func (t *ListServicesTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workspace": map[string]any{"type": "string", "description": "Filter by workspace name"},
			"platform":  map[string]any{"type": "string", "description": "Filter by platform (java, typescript, python)"},
		},
	}
}
func (t *ListServicesTool) Run(args map[string]any) *mcp.ToolCallResult {
	wsFilter, _ := args["workspace"].(string)
	platFilter, _ := args["platform"].(string)

	type svcInfo struct {
		Key       string `json:"key"`
		Platform  string `json:"platform"`
		Units     int    `json:"units"`
		Files     int    `json:"files"`
		Workspace string `json:"workspace"`
	}
	var list []svcInfo
	for key, svc := range t.ctx.Services {
		parts := strings.SplitN(key, ":", 2)
		ws := ""
		if len(parts) == 2 {
			ws = parts[0]
		}
		if wsFilter != "" && ws != wsFilter {
			continue
		}
		if platFilter != "" && svc.Platform.String() != platFilter {
			continue
		}
		list = append(list, svcInfo{
			Key: key, Platform: svc.Platform.String(),
			Units: len(svc.Units), Files: len(svc.FileChecksums), Workspace: ws,
		})
	}
	data, _ := json.MarshalIndent(map[string]any{"count": len(list), "services": list}, "", "  ")
	return mcp.TextResult(string(data))
}

type ServiceProfileTool struct{ ctx *Context }

func NewServiceProfileTool(ctx *Context) *ServiceProfileTool {
	return &ServiceProfileTool{ctx: ctx}
}
func (t *ServiceProfileTool) Name() string { return "service_profile" }
func (t *ServiceProfileTool) Description() string {
	return "Get detailed profile of a service: classes, methods, endpoints, dependencies, Kafka topics."
}
func (t *ServiceProfileTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"service": map[string]any{"type": "string", "description": "Service key"}},
		"required":   []string{"service"},
	}
}
func (t *ServiceProfileTool) Run(args map[string]any) *mcp.ToolCallResult {
	service, _ := args["service"].(string)
	svc, ok := t.ctx.Services[service]
	if !ok {
		return mcp.ErrorResult(fmt.Sprintf("service not found: %s", service))
	}

	var endpoints, kafkaTopics []string
	for _, u := range svc.Units {
		endpoints = append(endpoints, u.Endpoints...)
		kafkaTopics = append(kafkaTopics, u.KafkaProduces...)
		kafkaTopics = append(kafkaTopics, u.KafkaConsumes...)
	}

	profile := map[string]any{
		"key":        svc.Key,
		"path":       svc.Path,
		"platform":   svc.Platform.String(),
		"units":      len(svc.Units),
		"files":      len(svc.FileChecksums),
		"endpoints":  endpoints,
		"kafka":      kafkaTopics,
		"build_deps": svc.BuildDeps,
		"indexed_at": svc.IndexedAt,
	}
	data, _ := json.MarshalIndent(profile, "", "  ")
	return mcp.TextResult(string(data))
}
