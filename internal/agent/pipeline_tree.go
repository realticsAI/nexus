package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anurag/nexus/internal/jira"
)

// PhaseOrder defines the pipeline execution sequence.
var PhaseOrder = []WorkPhase{
	PhaseUnderstand,
	PhaseInvestigate,
	PhaseAnalyze,
	PhasePlan,
	PhaseValidate,
}

var phaseDeps = map[WorkPhase][]WorkPhase{
	PhaseUnderstand:  {},
	PhaseInvestigate: {PhaseUnderstand},
	PhaseAnalyze:     {PhaseUnderstand, PhaseInvestigate},
	PhasePlan:        {PhaseUnderstand, PhaseInvestigate, PhaseAnalyze},
	PhaseValidate:    {PhaseUnderstand, PhaseInvestigate, PhaseAnalyze, PhasePlan},
}

// PhaseNode is a node in the pipeline DAG.
type PhaseNode struct {
	ID           string        `json:"id"`
	Phase        WorkPhase     `json:"phase"`
	Parents      []string      `json:"parents"`
	Children     []string      `json:"children"`
	ExternalHash string        `json:"external_hash"`
	Valid        bool          `json:"valid"`
	CreatedAt    time.Time     `json:"created_at"`
	Duration     time.Duration `json:"duration"`
	TokensUsed   int           `json:"tokens_used"`
}

// PipelineTree is a content-addressed DAG of phase nodes for a ticket run.
type PipelineTree struct {
	TicketKey string                `json:"ticket_key"`
	RunID     string                `json:"run_id"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	LastPhase WorkPhase             `json:"last_phase"`
	Nodes     map[string]*PhaseNode `json:"nodes"`
	Refs      map[WorkPhase]string  `json:"refs"`

	Issue         *jira.Issue          `json:"issue,omitempty"`
	TicketType    TicketType           `json:"ticket_type,omitempty"`
	Investigation *InvestigationContext `json:"investigation,omitempty"`
	Analysis      *AnalysisResult      `json:"analysis,omitempty"`
	Plan          *Plan                `json:"plan,omitempty"`
	Validation    *ValidationResult    `json:"validation,omitempty"`
	TokensUsed    int                  `json:"tokens_used"`
}

func NewPipelineTree(ticketKey string) *PipelineTree {
	b := make([]byte, 8)
	rand.Read(b)
	return &PipelineTree{
		TicketKey: ticketKey,
		RunID:     hex.EncodeToString(b),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes:     make(map[string]*PhaseNode),
		Refs:      make(map[WorkPhase]string),
	}
}

// AddNode creates a phase node with parent links derived from phaseDeps,
// computes its content-addressed ID, and wires it into the tree.
func (t *PipelineTree) AddNode(phase WorkPhase, externalHash string, duration time.Duration, tokensUsed int) {
	parentIDs := t.resolveParentIDs(phase)

	id := nodeID(phase, parentIDs, externalHash)

	node := &PhaseNode{
		ID:           id,
		Phase:        phase,
		Parents:      parentIDs,
		ExternalHash: externalHash,
		Valid:        true,
		CreatedAt:    time.Now(),
		Duration:     duration,
		TokensUsed:   tokensUsed,
	}

	// Wire children on parent nodes
	for _, pid := range parentIDs {
		if parent, ok := t.Nodes[pid]; ok {
			if !containsStr(parent.Children, id) {
				parent.Children = append(parent.Children, id)
			}
		}
	}

	t.Nodes[id] = node
	t.Refs[phase] = id
	t.LastPhase = phase
	t.UpdatedAt = time.Now()
	t.TokensUsed += tokensUsed
}

// Invalidate marks a node invalid and cascades forward to all children.
func (t *PipelineTree) Invalidate(nodeID string) {
	node, ok := t.Nodes[nodeID]
	if !ok {
		return
	}
	node.Valid = false
	for _, childID := range node.Children {
		t.Invalidate(childID)
	}
}

// InvalidateFrom invalidates the current ref for the given phase and cascades.
func (t *PipelineTree) InvalidateFrom(phase WorkPhase) {
	if id, ok := t.Refs[phase]; ok {
		t.Invalidate(id)
	}
}

// Backtrack walks parent chain from a node, returning all ancestors (deduplicated).
func (t *PipelineTree) Backtrack(nodeID string) []*PhaseNode {
	visited := make(map[string]bool)
	var result []*PhaseNode
	t.backtrackWalk(nodeID, visited, &result)
	return result
}

func (t *PipelineTree) backtrackWalk(nodeID string, visited map[string]bool, result *[]*PhaseNode) {
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true
	node, ok := t.Nodes[nodeID]
	if !ok {
		return
	}
	*result = append(*result, node)
	for _, pid := range node.Parents {
		t.backtrackWalk(pid, visited, result)
	}
}

// ResumePoint returns the first phase in PhaseOrder whose ref is missing or invalid.
func (t *PipelineTree) ResumePoint() WorkPhase {
	for _, phase := range PhaseOrder {
		if !t.IsValid(phase) {
			return phase
		}
	}
	return "DONE"
}

// IsValid checks if the phase has a valid node in the tree.
func (t *PipelineTree) IsValid(phase WorkPhase) bool {
	id, ok := t.Refs[phase]
	if !ok || id == "" {
		return false
	}
	node, ok := t.Nodes[id]
	if !ok {
		return false
	}
	return node.Valid
}

// PhaseCount returns the number of valid and invalid phase nodes.
func (t *PipelineTree) PhaseCount() (valid, invalid int) {
	for _, phase := range PhaseOrder {
		id, ok := t.Refs[phase]
		if !ok || id == "" {
			continue
		}
		node, ok := t.Nodes[id]
		if !ok {
			continue
		}
		if node.Valid {
			valid++
		} else {
			invalid++
		}
	}
	return
}

func (t *PipelineTree) resolveParentIDs(phase WorkPhase) []string {
	deps := phaseDeps[phase]
	var ids []string
	for _, dep := range deps {
		if id, ok := t.Refs[dep]; ok && id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func nodeID(phase WorkPhase, parentIDs []string, externalHash string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s", string(phase), strings.Join(parentIDs, ","), externalHash)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
