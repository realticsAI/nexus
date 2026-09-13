package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditLogger struct {
	path string
	mu   sync.Mutex
}

type AuditEntry struct {
	Timestamp string `json:"ts"`
	Op        string `json:"op"`
	Ticket    string `json:"ticket,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Tokens    int    `json:"tokens,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Error     string `json:"error,omitempty"`
}

func NewAuditLogger(nexusDir string) *AuditLogger {
	return &AuditLogger{
		path: filepath.Join(nexusDir, "audit.log"),
	}
}

func (a *AuditLogger) Log(entry AuditEntry) {
	if a == nil {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nexus: audit log write failed: %v\n", err)
		return
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f.Write(line)
	f.Write([]byte("\n"))
}

func (a *AuditLogger) LogLLM(ticket, model string, tokens int, dur time.Duration, err error) {
	entry := AuditEntry{
		Op:       "llm_call",
		Ticket:   ticket,
		Detail:   model,
		Tokens:   tokens,
		Duration: dur.Round(time.Millisecond).String(),
	}
	if err != nil {
		entry.Error = err.Error()
	}
	a.Log(entry)
}

func (a *AuditLogger) LogGit(op, ticket, detail string, err error) {
	entry := AuditEntry{
		Op:     op,
		Ticket: ticket,
		Detail: detail,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	a.Log(entry)
}

func (a *AuditLogger) LogJira(op, ticket, detail string, err error) {
	entry := AuditEntry{
		Op:     op,
		Ticket: ticket,
		Detail: detail,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	a.Log(entry)
}
