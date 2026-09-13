package store

import (
	"testing"
	"time"

	"github.com/anurag/nexus/model"
)

func TestSaveAndLoadService(t *testing.T) {
	s := NewStore(t.TempDir())
	idx := &model.ServiceIndex{
		Key: "backend:orders", Path: "backend/orders", Platform: model.Java,
		Units:         []model.Unit{{Type: "class", Name: "OrderController", File: "src/OrderController.java", Line: 10}},
		FileChecksums: map[string]string{"src/OrderController.java": "sha256:abc123"},
		IndexedAt:     time.Now(),
	}
	if err := s.SaveService(idx.Key, idx); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadService(idx.Key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Key != idx.Key {
		t.Fatalf("expected %s, got %s", idx.Key, loaded.Key)
	}
	if len(loaded.Units) != 1 || loaded.Units[0].Name != "OrderController" {
		t.Fatal("unit roundtrip failed")
	}
}

func TestLoadAllServices(t *testing.T) {
	s := NewStore(t.TempDir())
	s.SaveService("svc-a", &model.ServiceIndex{Key: "svc-a", FileChecksums: map[string]string{}})
	s.SaveService("svc-b", &model.ServiceIndex{Key: "svc-b", FileChecksums: map[string]string{}})
	all, err := s.LoadAllServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	s := NewStore(t.TempDir())
	m := &model.Manifest{Version: 1, Services: []model.ServiceEntry{{Key: "svc-a", Platform: "java", FileCount: 42}}, GeneratedAt: time.Now()}
	s.SaveManifest(m)
	loaded, _ := s.LoadManifest()
	if len(loaded.Services) != 1 || loaded.Services[0].FileCount != 42 {
		t.Fatal("manifest roundtrip failed")
	}
}

func TestSaveAndLoadHealth(t *testing.T) {
	s := NewStore(t.TempDir())
	h := &model.CrawlHealth{TotalRepos: 100, Indexed: 98, Failed: []model.RepoFailure{{Name: "broken", Reason: "auth"}}, CrawlDuration: "45s", Timestamp: time.Now()}
	s.SaveHealth(h)
	loaded, _ := s.LoadHealth()
	if loaded.TotalRepos != 100 || loaded.Indexed != 98 || len(loaded.Failed) != 1 {
		t.Fatal("health roundtrip failed")
	}
}

func TestSaveAndLoadGraph(t *testing.T) {
	s := NewStore(t.TempDir())
	g := &model.Graph{Version: 1, Nodes: []model.Node{{Key: "svc"}}, Edges: []model.Edge{{From: "a", To: "b", Type: model.HTTPCall}}, GeneratedAt: time.Now()}
	s.SaveGraph(g)
	loaded, _ := s.LoadGraph()
	if len(loaded.Edges) != 1 || loaded.Edges[0].Type != model.HTTPCall {
		t.Fatal("graph roundtrip failed")
	}
}

func TestLoadServiceNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.LoadService("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadManifestNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	m, err := s.LoadManifest()
	if err != nil {
		t.Fatal("should not error")
	}
	if len(m.Services) != 0 {
		t.Fatal("should have no services")
	}
}
