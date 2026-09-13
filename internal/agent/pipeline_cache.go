package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
	"github.com/anurag/nexus/model"
)

// RunDir returns the storage path for a ticket's pipeline run.
func RunDir(stateDir, ticketKey string) string {
	return filepath.Join(stateDir, "runs", ticketKey)
}

// SavePipelineTree serializes the tree to disk.
// Backs up the previous tree.json to tree.json.prev before writing.
func SavePipelineTree(stateDir, ticketKey string, tree *PipelineTree) error {
	dir := RunDir(stateDir, ticketKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating run dir: %w", err)
	}

	path := filepath.Join(dir, "tree.json")

	if _, err := os.Stat(path); err == nil {
		prev := filepath.Join(dir, "tree.json.prev")
		os.Rename(path, prev)
	}

	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tree: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadPipelineTree reads a cached tree from disk.
// Returns nil, nil if no cached run exists.
func LoadPipelineTree(stateDir, ticketKey string) (*PipelineTree, error) {
	path := filepath.Join(RunDir(stateDir, ticketKey), "tree.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tree: %w", err)
	}
	var tree PipelineTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("unmarshaling tree: %w", err)
	}
	return &tree, nil
}

// FingerprintTicket hashes the ticket's content fields.
func FingerprintTicket(issue *jira.Issue) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s",
		issue.Key,
		issue.Summary,
		issue.Description,
		issue.AccCriteria,
		strings.Join(issue.Labels, ","),
		strings.Join(issue.Components, ","),
		strings.Join(issue.LinkedIssues, ","),
	)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// FingerprintIndex hashes the service index structure.
// Uses sorted keys + edge count — cheap but catches reindex.
func FingerprintIndex(summaries map[string]*model.ServiceSummary, graph *model.Graph) string {
	keys := make([]string, 0, len(summaries))
	for k := range summaries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		s := summaries[k]
		fmt.Fprintf(h, "%s:%d|", k, s.UnitCount)
	}
	edgeCount := 0
	if graph != nil {
		edgeCount = len(graph.Edges)
	}
	fmt.Fprintf(h, "edges:%d", edgeCount)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// FingerprintLLM hashes the LLM provider identity.
func FingerprintLLM(provider llm.Provider) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s", provider.Name(), provider.Model())
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// FingerprintFiles hashes the file excerpts from investigation.
// Detects if source files changed since they were read.
func FingerprintFiles(excerpts []FileExcerpt) string {
	h := sha256.New()
	for _, e := range excerpts {
		fmt.Fprintf(h, "%s:%s|", e.File, e.Signature)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CheckAndInvalidate compares current fingerprints against stored nodes
// and cascades invalidation forward from the earliest stale phase.
func (t *PipelineTree) CheckAndInvalidate(ticketHash, indexHash, llmHash string) {
	// UNDERSTAND depends on ticket content
	if id, ok := t.Refs[PhaseUnderstand]; ok {
		if node, ok := t.Nodes[id]; ok && node.ExternalHash != ticketHash {
			t.InvalidateFrom(PhaseUnderstand)
			return
		}
	}

	// INVESTIGATE depends on service index
	if id, ok := t.Refs[PhaseInvestigate]; ok {
		if node, ok := t.Nodes[id]; ok && node.ExternalHash != indexHash {
			t.InvalidateFrom(PhaseInvestigate)
			return
		}
	}

	// ANALYZE and all downstream LLM phases depend on model identity
	for _, phase := range []WorkPhase{PhaseAnalyze, PhasePlan, PhaseValidate, PhaseReplan} {
		if id, ok := t.Refs[phase]; ok {
			if node, ok := t.Nodes[id]; ok && node.ExternalHash != llmHash {
				t.InvalidateFrom(phase)
				return
			}
		}
	}
}
