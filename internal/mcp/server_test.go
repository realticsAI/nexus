package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type echoTool struct{}

func (e *echoTool) Name() string        { return "echo" }
func (e *echoTool) Description() string  { return "Echoes input" }
func (e *echoTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string"},
		},
	}
}
func (e *echoTool) Run(args map[string]any) *ToolCallResult {
	msg, _ := args["message"].(string)
	return TextResult("echo: " + msg)
}

func sendAndRead(t *testing.T, s *Server, req Request) Response {
	t.Helper()
	data, _ := json.Marshal(req)
	in := bytes.NewReader(append(data, '\n'))
	out := &bytes.Buffer{}
	s.SetIO(in, out)
	s.Serve()
	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	return resp
}

func TestInitialize(t *testing.T) {
	s := NewServer("nexus", "test")
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
	}
	resp := sendAndRead(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "nexus") {
		t.Fatalf("expected nexus in result, got %s", data)
	}
}

func TestToolsList(t *testing.T) {
	s := NewServer("nexus", "test")
	s.Register(&echoTool{})
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	}
	resp := sendAndRead(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "echo") {
		t.Fatalf("expected echo tool, got %s", data)
	}
}

func TestToolsCall(t *testing.T) {
	s := NewServer("nexus", "test")
	s.Register(&echoTool{})
	params, _ := json.Marshal(ToolCallParams{
		Name:      "echo",
		Arguments: map[string]any{"message": "hello"},
	})
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`3`),
		Method:  "tools/call",
		Params:  params,
	}
	resp := sendAndRead(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "echo: hello") {
		t.Fatalf("expected echo: hello, got %s", data)
	}
}

func TestUnknownTool(t *testing.T) {
	s := NewServer("nexus", "test")
	params, _ := json.Marshal(ToolCallParams{Name: "nonexistent"})
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`4`),
		Method:  "tools/call",
		Params:  params,
	}
	resp := sendAndRead(t, s, req)
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %s", data)
	}
}

func TestPing(t *testing.T) {
	s := NewServer("nexus", "test")
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`5`),
		Method:  "ping",
	}
	resp := sendAndRead(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
}
