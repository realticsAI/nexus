package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anurag/nexus/internal/debug"
	"github.com/anurag/nexus/model"
)

type Store struct {
	dir string
}

func NewStore(stateDir string) *Store {
	return &Store{dir: stateDir}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) serviceDir() string {
	return filepath.Join(s.dir, "services")
}

func (s *Store) servicePath(key string) string {
	safe := strings.ReplaceAll(key, ":", "_")
	return filepath.Join(s.serviceDir(), safe+".json")
}

func (s *Store) SaveService(key string, idx *model.ServiceIndex) error {
	return AtomicWriteJSON(s.servicePath(key), idx)
}

func (s *Store) LoadService(key string) (*model.ServiceIndex, error) {
	t := time.Now()
	path := s.servicePath(key)
	debug.Log("store", "LoadService key=%s path=%s fileSize=%s", key, path, debug.FormatBytes(uint64(debug.FileSize(path))))
	svc, err := ReadJSON[model.ServiceIndex](path)
	if err == nil {
		debug.Log("store", "LoadService key=%s units=%d took=%s", key, len(svc.Units), debug.Since(t))
	}
	return svc, err
}

func (s *Store) LoadAllServices() (map[string]*model.ServiceIndex, error) {
	t := time.Now()
	dir := s.serviceDir()
	debug.Log("store", "LoadAllServices dir=%s", dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*model.ServiceIndex{}, nil
		}
		return nil, err
	}
	result := make(map[string]*model.ServiceIndex, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".summary.json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fSize := debug.FileSize(path)
		svc, err := ReadJSON[model.ServiceIndex](path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		debug.Log("store", "LoadAllServices loaded key=%s units=%d fileSize=%s", svc.Key, len(svc.Units), debug.FormatBytes(uint64(fSize)))
		result[svc.Key] = svc
	}
	debug.Log("store", "LoadAllServices total=%d took=%s", len(result), debug.Since(t))
	return result, nil
}

func (s *Store) DeleteService(key string) error {
	os.Remove(s.summaryPath(key))
	return os.Remove(s.servicePath(key))
}

func (s *Store) summaryPath(key string) string {
	safe := strings.ReplaceAll(key, ":", "_")
	return filepath.Join(s.serviceDir(), safe+".summary.json")
}

func (s *Store) SaveServiceSummary(key string, idx *model.ServiceIndex) error {
	summary := model.BuildServiceSummary(idx)
	err := AtomicWriteJSON(s.summaryPath(key), summary)
	if err == nil {
		debug.Log("store", "SaveServiceSummary key=%s keywords=%d endpoints=%d fileSize=%s", key, len(summary.Keywords), len(summary.Endpoints), debug.FormatBytes(uint64(debug.FileSize(s.summaryPath(key)))))
	}
	return err
}

func (s *Store) LoadServiceSummary(key string) (*model.ServiceSummary, error) {
	return ReadJSON[model.ServiceSummary](s.summaryPath(key))
}

func (s *Store) LoadServiceSummaries() (map[string]*model.ServiceSummary, error) {
	t := time.Now()
	dir := s.serviceDir()
	debug.Log("store", "LoadServiceSummaries dir=%s", dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*model.ServiceSummary{}, nil
		}
		return nil, err
	}
	result := make(map[string]*model.ServiceSummary, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".summary.json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		summary, err := ReadJSON[model.ServiceSummary](path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		debug.Log("store", "LoadServiceSummaries loaded key=%s units=%d keywords=%d fileSize=%s", summary.Key, summary.UnitCount, len(summary.Keywords), debug.FormatBytes(uint64(debug.FileSize(path))))
		result[summary.Key] = summary
	}
	debug.Log("store", "LoadServiceSummaries total=%d took=%s", len(result), debug.Since(t))
	return result, nil
}

func (s *Store) RegenerateSummaries(force bool) (int, error) {
	dir := s.serviceDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".summary.json") {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		key = strings.ReplaceAll(key, "_", ":")
		if !force {
			if _, err := os.Stat(s.summaryPath(key)); err == nil {
				continue
			}
		}
		idx, err := ReadJSON[model.ServiceIndex](filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		summary := model.BuildServiceSummary(idx)
		if err := AtomicWriteJSON(s.summaryPath(key), summary); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

func (s *Store) BackfillSummaries() (int, error) {
	t := time.Now()
	dir := s.serviceDir()
	debug.Log("store", "BackfillSummaries dir=%s", dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".summary.json") {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		key = strings.ReplaceAll(key, "_", ":")
		if _, err := os.Stat(s.summaryPath(key)); err == nil {
			debug.Log("store", "BackfillSummaries skip key=%s (exists)", key)
			continue
		}
		debug.Log("store", "BackfillSummaries generating key=%s", key)
		idx, err := ReadJSON[model.ServiceIndex](filepath.Join(dir, e.Name()))
		if err != nil {
			debug.Log("store", "BackfillSummaries error key=%s: %v", key, err)
			continue
		}
		summary := model.BuildServiceSummary(idx)
		if err := AtomicWriteJSON(s.summaryPath(key), summary); err != nil {
			debug.Log("store", "BackfillSummaries write error key=%s: %v", key, err)
			continue
		}
		debug.Log("store", "BackfillSummaries created key=%s keywords=%d endpoints=%d", key, len(summary.Keywords), len(summary.Endpoints))
		count++
	}
	debug.Log("store", "BackfillSummaries done count=%d took=%s", count, debug.Since(t))
	return count, nil
}

func (s *Store) SaveManifest(m *model.Manifest) error {
	return AtomicWriteJSON(filepath.Join(s.dir, "manifest.json"), m)
}

func (s *Store) LoadManifest() (*model.Manifest, error) {
	m, err := ReadJSON[model.Manifest](filepath.Join(s.dir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &model.Manifest{Version: 1}, nil
		}
		return nil, err
	}
	return m, nil
}

func (s *Store) SaveHealth(h *model.CrawlHealth) error {
	return AtomicWriteJSON(filepath.Join(s.dir, "health.json"), h)
}

func (s *Store) LoadHealth() (*model.CrawlHealth, error) {
	h, err := ReadJSON[model.CrawlHealth](filepath.Join(s.dir, "health.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &model.CrawlHealth{}, nil
		}
		return nil, err
	}
	return h, nil
}

func (s *Store) SaveGraph(g *model.Graph) error {
	return AtomicWriteJSON(filepath.Join(s.dir, "graph.json"), g)
}

func (s *Store) LoadGraph() (*model.Graph, error) {
	t := time.Now()
	path := filepath.Join(s.dir, "graph.json")
	debug.Log("store", "LoadGraph path=%s fileSize=%s", path, debug.FormatBytes(uint64(debug.FileSize(path))))
	g, err := ReadJSON[model.Graph](path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.Graph{Version: 1}, nil
		}
		return nil, err
	}
	debug.Log("store", "LoadGraph edges=%d nodes=%d took=%s", len(g.Edges), len(g.Nodes), debug.Since(t))
	return g, nil
}
