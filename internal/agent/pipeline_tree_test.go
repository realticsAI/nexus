package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anurag/nexus/internal/jira"
)

func TestNewPipelineTree(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")
	if tree.TicketKey != "PROJ-123" {
		t.Fatalf("expected PROJ-123, got %s", tree.TicketKey)
	}
	if len(tree.RunID) == 0 {
		t.Fatal("expected non-empty RunID")
	}
	if len(tree.Nodes) != 0 {
		t.Fatal("expected empty nodes")
	}
}

func TestAddNodeAndRefs(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")
	tree.AddNode(PhaseUnderstand, "hash1", 100*time.Millisecond, 0)

	if _, ok := tree.Refs[PhaseUnderstand]; !ok {
		t.Fatal("expected PhaseUnderstand ref")
	}
	if len(tree.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(tree.Nodes))
	}
	if tree.LastPhase != PhaseUnderstand {
		t.Fatalf("expected last phase UNDERSTAND, got %s", tree.LastPhase)
	}

	node := tree.Nodes[tree.Refs[PhaseUnderstand]]
	if !node.Valid {
		t.Fatal("expected node to be valid")
	}
	if node.ExternalHash != "hash1" {
		t.Fatalf("expected hash1, got %s", node.ExternalHash)
	}
}

func TestParentWiring(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")
	tree.AddNode(PhaseUnderstand, "h1", time.Second, 0)
	tree.AddNode(PhaseInvestigate, "h2", time.Second, 0)

	invID := tree.Refs[PhaseInvestigate]
	invNode := tree.Nodes[invID]

	undID := tree.Refs[PhaseUnderstand]
	if len(invNode.Parents) != 1 || invNode.Parents[0] != undID {
		t.Fatalf("expected investigate parent to be understand, got %v", invNode.Parents)
	}

	undNode := tree.Nodes[undID]
	if len(undNode.Children) != 1 || undNode.Children[0] != invID {
		t.Fatalf("expected understand child to be investigate, got %v", undNode.Children)
	}
}

func TestInvalidateCascade(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")
	tree.AddNode(PhaseUnderstand, "h1", time.Second, 0)
	tree.AddNode(PhaseInvestigate, "h2", time.Second, 0)
	tree.AddNode(PhaseAnalyze, "h3", time.Second, 0)

	tree.InvalidateFrom(PhaseUnderstand)

	for _, phase := range []WorkPhase{PhaseUnderstand, PhaseInvestigate, PhaseAnalyze} {
		if tree.IsValid(phase) {
			t.Fatalf("expected %s to be invalid after cascade", phase)
		}
	}
}

func TestResumePoint(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")

	if rp := tree.ResumePoint(); rp != PhaseUnderstand {
		t.Fatalf("empty tree should resume from UNDERSTAND, got %s", rp)
	}

	tree.AddNode(PhaseUnderstand, "h1", time.Second, 0)
	tree.AddNode(PhaseInvestigate, "h2", time.Second, 0)
	tree.AddNode(PhaseAnalyze, "h3", time.Second, 0)

	if rp := tree.ResumePoint(); rp != PhasePlan {
		t.Fatalf("expected resume from PLAN, got %s", rp)
	}

	tree.InvalidateFrom(PhaseInvestigate)
	if rp := tree.ResumePoint(); rp != PhaseInvestigate {
		t.Fatalf("expected resume from INVESTIGATE after invalidation, got %s", rp)
	}
}

func TestPhaseCount(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")
	tree.AddNode(PhaseUnderstand, "h1", time.Second, 100)
	tree.AddNode(PhaseInvestigate, "h2", time.Second, 200)
	tree.AddNode(PhaseAnalyze, "h3", time.Second, 300)

	valid, invalid := tree.PhaseCount()
	if valid != 3 || invalid != 0 {
		t.Fatalf("expected 3 valid 0 invalid, got %d valid %d invalid", valid, invalid)
	}

	tree.InvalidateFrom(PhaseAnalyze)
	valid, invalid = tree.PhaseCount()
	if valid != 2 || invalid != 1 {
		t.Fatalf("expected 2 valid 1 invalid, got %d valid %d invalid", valid, invalid)
	}
}

func TestBacktrack(t *testing.T) {
	tree := NewPipelineTree("PROJ-123")
	tree.AddNode(PhaseUnderstand, "h1", time.Second, 0)
	tree.AddNode(PhaseInvestigate, "h2", time.Second, 0)
	tree.AddNode(PhaseAnalyze, "h3", time.Second, 0)

	analyzeID := tree.Refs[PhaseAnalyze]
	ancestors := tree.Backtrack(analyzeID)

	if len(ancestors) != 3 {
		t.Fatalf("expected 3 ancestors (including self), got %d", len(ancestors))
	}
}

func TestContentAddressedID(t *testing.T) {
	tree1 := NewPipelineTree("PROJ-1")
	tree1.AddNode(PhaseUnderstand, "same-hash", time.Second, 0)
	id1 := tree1.Refs[PhaseUnderstand]

	tree2 := NewPipelineTree("PROJ-2")
	tree2.AddNode(PhaseUnderstand, "same-hash", time.Second, 0)
	id2 := tree2.Refs[PhaseUnderstand]

	if id1 != id2 {
		t.Fatalf("same inputs should produce same node ID: %s vs %s", id1, id2)
	}

	tree3 := NewPipelineTree("PROJ-3")
	tree3.AddNode(PhaseUnderstand, "different-hash", time.Second, 0)
	id3 := tree3.Refs[PhaseUnderstand]

	if id1 == id3 {
		t.Fatal("different inputs should produce different node IDs")
	}
}

func TestSaveLoadPipelineTree(t *testing.T) {
	dir := t.TempDir()
	tree := NewPipelineTree("PROJ-456")
	tree.Issue = &jira.Issue{Key: "PROJ-456", Summary: "Test ticket"}
	tree.TicketType = Bug
	tree.AddNode(PhaseUnderstand, "h1", time.Second, 100)
	tree.AddNode(PhaseInvestigate, "h2", 2*time.Second, 200)

	if err := SavePipelineTree(dir, "PROJ-456", tree); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path := filepath.Join(dir, "runs", "PROJ-456", "tree.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("tree.json not created: %v", err)
	}

	loaded, err := LoadPipelineTree(dir, "PROJ-456")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.TicketKey != "PROJ-456" {
		t.Fatalf("expected PROJ-456, got %s", loaded.TicketKey)
	}
	if loaded.Issue == nil || loaded.Issue.Summary != "Test ticket" {
		t.Fatal("issue not preserved")
	}
	if len(loaded.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(loaded.Nodes))
	}
	if loaded.TokensUsed != 300 {
		t.Fatalf("expected 300 tokens, got %d", loaded.TokensUsed)
	}
}

func TestLoadPipelineTreeNotExist(t *testing.T) {
	dir := t.TempDir()
	tree, err := LoadPipelineTree(dir, "PROJ-NOPE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree != nil {
		t.Fatal("expected nil tree for non-existent run")
	}
}

func TestSaveCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	tree := NewPipelineTree("PROJ-789")
	tree.AddNode(PhaseUnderstand, "h1", time.Second, 0)

	SavePipelineTree(dir, "PROJ-789", tree)
	tree.AddNode(PhaseInvestigate, "h2", time.Second, 0)
	SavePipelineTree(dir, "PROJ-789", tree)

	prevPath := filepath.Join(dir, "runs", "PROJ-789", "tree.json.prev")
	if _, err := os.Stat(prevPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
}

func TestCheckAndInvalidate(t *testing.T) {
	tree := NewPipelineTree("PROJ-100")
	tree.AddNode(PhaseUnderstand, "ticket-v1", time.Second, 0)
	tree.AddNode(PhaseInvestigate, "index-v1", time.Second, 0)
	tree.AddNode(PhaseAnalyze, "llm-v1", time.Second, 0)
	tree.AddNode(PhasePlan, "llm-v1", time.Second, 0)

	tree.CheckAndInvalidate("ticket-v1", "index-v1", "llm-v1")
	if rp := tree.ResumePoint(); rp != PhaseValidate {
		t.Fatalf("no changes: expected resume from VALIDATE, got %s", rp)
	}

	tree2 := NewPipelineTree("PROJ-101")
	tree2.AddNode(PhaseUnderstand, "ticket-v1", time.Second, 0)
	tree2.AddNode(PhaseInvestigate, "index-v1", time.Second, 0)
	tree2.AddNode(PhaseAnalyze, "llm-v1", time.Second, 0)
	tree2.AddNode(PhasePlan, "llm-v1", time.Second, 0)

	tree2.CheckAndInvalidate("ticket-v2", "index-v1", "llm-v1")
	if rp := tree2.ResumePoint(); rp != PhaseUnderstand {
		t.Fatalf("ticket changed: expected resume from UNDERSTAND, got %s", rp)
	}

	tree3 := NewPipelineTree("PROJ-102")
	tree3.AddNode(PhaseUnderstand, "ticket-v1", time.Second, 0)
	tree3.AddNode(PhaseInvestigate, "index-v1", time.Second, 0)
	tree3.AddNode(PhaseAnalyze, "llm-v1", time.Second, 0)
	tree3.AddNode(PhasePlan, "llm-v1", time.Second, 0)

	tree3.CheckAndInvalidate("ticket-v1", "index-v2", "llm-v1")
	if rp := tree3.ResumePoint(); rp != PhaseInvestigate {
		t.Fatalf("index changed: expected resume from INVESTIGATE, got %s", rp)
	}
}
