package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type ToolCallEvent struct {
	Timestamp string         `json:"timestamp"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args,omitempty"`
	Duration  string         `json:"duration,omitempty"`
	Error     bool           `json:"error,omitempty"`
}

type Server struct {
	name     string
	version  string
	tools    map[string]ToolHandler
	toolList []ToolDef
	writeMu  sync.Mutex
	wg       sync.WaitGroup
	in       io.Reader
	out      io.Writer
	eventLog []ToolCallEvent
	eventMu  sync.RWMutex
	eventCh  chan ToolCallEvent
}

func NewServer(name, version string) *Server {
	return &Server{
		name:    name,
		version: version,
		tools:   make(map[string]ToolHandler),
		in:      os.Stdin,
		out:     os.Stdout,
		eventCh: make(chan ToolCallEvent, 100),
	}
}

func (s *Server) Events() <-chan ToolCallEvent { return s.eventCh }

func (s *Server) RecentEvents(n int) []ToolCallEvent {
	s.eventMu.RLock()
	defer s.eventMu.RUnlock()
	if n <= 0 || n > len(s.eventLog) {
		n = len(s.eventLog)
	}
	start := len(s.eventLog) - n
	out := make([]ToolCallEvent, n)
	copy(out, s.eventLog[start:])
	return out
}

func (s *Server) logEvent(e ToolCallEvent) {
	s.eventMu.Lock()
	s.eventLog = append(s.eventLog, e)
	if len(s.eventLog) > 500 {
		s.eventLog = s.eventLog[len(s.eventLog)-500:]
	}
	s.eventMu.Unlock()
	select {
	case s.eventCh <- e:
	default:
	}
	writeEventToDisk(e)
}

func writeEventToDisk(e ToolCallEvent) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	logPath := home + "/.nexus/state/mcp-events.jsonl"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	data, _ := json.Marshal(e)
	f.Write(data)
	f.Write([]byte("\n"))
}

func (s *Server) SetIO(in io.Reader, out io.Writer) {
	s.in = in
	s.out = out
}

func (s *Server) Register(h ToolHandler) {
	s.tools[h.Name()] = h
	s.toolList = append(s.toolList, ToolDef{
		Name:        h.Name(),
		Description: h.Description(),
		InputSchema: h.InputSchema(),
	})
}

func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		msg := make([]byte, len(line))
		copy(msg, line)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleMessage(msg)
		}()
	}
	s.wg.Wait()
	return scanner.Err()
}

func (s *Server) handleMessage(msg []byte) {
	var req Request
	if err := json.Unmarshal(msg, &req); err != nil {
		s.sendError(nil, -32700, "parse error")
		return
	}

	if req.ID == nil || string(req.ID) == "null" {
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "ping":
		s.sendResult(req.ID, map[string]any{})
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req Request) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo:      ServerInfo{Name: s.name, Version: s.version},
		Capabilities:    Capabilities{Tools: &ToolsCapability{}},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req Request) {
	s.sendResult(req.ID, ToolsListResult{Tools: s.toolList})
}

func (s *Server) handleToolsCall(req Request) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "invalid tool call params")
		return
	}

	handler, ok := s.tools[params.Name]
	if !ok {
		s.sendResult(req.ID, ErrorResult(fmt.Sprintf("unknown tool: %s", params.Name)))
		return
	}

	start := time.Now()
	result := handler.Run(params.Arguments)
	elapsed := time.Since(start)

	s.logEvent(ToolCallEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tool:      params.Name,
		Args:      params.Arguments,
		Duration:  elapsed.Round(time.Millisecond).String(),
		Error:     result.IsError,
	})

	s.sendResult(req.ID, result)
}

func (s *Server) sendResult(id json.RawMessage, result any) {
	resp := Response{JSONRPC: "2.0", ID: id, Result: result}
	s.writeResponse(resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string) {
	resp := Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
	s.writeResponse(resp)
}

func (s *Server) writeResponse(resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.out.Write(data)
	s.out.Write([]byte("\n"))
}
