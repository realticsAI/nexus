package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/debug"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
)

type Crawler struct {
	cfg     *config.Config
	store   *store.Store
	parsers map[model.Platform]Parser
}

func NewCrawler(cfg *config.Config, s *store.Store) *Crawler {
	c := &Crawler{
		cfg:     cfg,
		store:   s,
		parsers: make(map[model.Platform]Parser),
	}
	c.registerParsers()
	return c
}

func (c *Crawler) registerParsers() {
	jp := NewJavaParser()
	c.parsers[jp.Platform()] = jp

	sp := NewSwiftParser()
	c.parsers[sp.Platform()] = sp

	kp := NewKotlinParser()
	c.parsers[kp.Platform()] = kp

	tp := NewTypeScriptParser()
	c.parsers[tp.Platform()] = tp
}

func (c *Crawler) RegisterParser(p Parser) {
	c.parsers[p.Platform()] = p
}

func (c *Crawler) Run(full bool) (*CrawlResult, error) {
	start := time.Now()
	workspaces := c.cfg.ExpandedWorkspaces()
	if len(workspaces) == 0 {
		return nil, fmt.Errorf("no workspaces configured")
	}
	debug.Log("crawler", "Run full=%v workspaces=%d paths=%v", full, len(workspaces), workspaces)

	discovered := DiscoverRepos(workspaces)
	if len(discovered) == 0 {
		return nil, fmt.Errorf("no git repos found in configured workspaces")
	}
	debug.Log("crawler", "Run discovered=%d repos", len(discovered))

	// Expand monorepos into sub-projects
	var repos []RepoInfo
	for _, repo := range discovered {
		repos = append(repos, ExpandMonorepo(repo)...)
	}
	debug.Log("crawler", "Run after monorepo expansion=%d repos", len(repos))

	storedSHAs := c.loadStoredSHAs()
	var changes []RepoChange
	if full {
		changes = make([]RepoChange, len(repos))
		for i, r := range repos {
			changes[i] = RepoChange{Repo: r, Changed: true, NewSHA: GetHeadSHA(r.Path)}
		}
	} else {
		changes = PullAll(repos, storedSHAs, c.cfg.Crawl.GitPullConcurrency)
	}

	result := &CrawlResult{
		TotalRepos: len(repos),
		StartedAt:  start,
	}

	sem := make(chan struct{}, c.cfg.Crawl.ParseConcurrency)
	if c.cfg.Crawl.ParseConcurrency <= 0 {
		sem = make(chan struct{}, 8)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, change := range changes {
		if !change.Changed && !full {
			result.Skipped++
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(ch RepoChange) {
			defer wg.Done()
			defer func() { <-sem }()

			idx, err := c.indexRepo(ch, full)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Failed = append(result.Failed, model.RepoFailure{
					Name:   ch.Repo.Name,
					Path:   ch.Repo.Path,
					Reason: err.Error(),
				})
			} else {
				result.Indexed++
				key := ServiceKey(ch.Repo.Workspace, ch.Repo.Name)
				c.store.SaveService(key, idx)
				c.store.SaveServiceSummary(key, idx)
			}
		}(change)
	}
	wg.Wait()

	result.Duration = time.Since(start)
	c.saveResults(result, repos, changes)
	return result, nil
}

func (c *Crawler) indexRepo(change RepoChange, full bool) (*model.ServiceIndex, error) {
	t := time.Now()
	repo := change.Repo
	platform := model.DetectPlatform(repo.Path)
	key := ServiceKey(repo.Workspace, repo.Name)
	debug.Log("crawler", "indexRepo key=%s platform=%s path=%s full=%v", key, platform, repo.Path, full)

	var storedChecksums map[string]string
	if !full {
		if existing, err := c.store.LoadService(key); err == nil {
			storedChecksums = existing.FileChecksums
		}
	}
	if storedChecksums == nil {
		storedChecksums = make(map[string]string)
	}

	fileChanges := DetectFileChanges(repo.Path, change.OldSHA, storedChecksums)
	debug.Log("crawler", "indexRepo key=%s fileChanges=%d", key, len(fileChanges))

	parser, ok := c.parsers[platform]
	var units []model.Unit
	parsedFiles := 0

	if ok {
		for _, fc := range fileChanges {
			if fc.Type == Deleted {
				continue
			}
			fullPath := filepath.Join(repo.Path, fc.Path)
			if !hasExtension(fullPath, parser.Extensions()) {
				continue
			}
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			ft := time.Now()
			parsed, err := parser.ParseFile(fc.Path, content)
			if err != nil {
				debug.Log("crawler", "indexRepo parse error file=%s: %v", fc.Path, err)
				continue
			}
			parsedFiles++
			debug.Log("crawler", "indexRepo parsed file=%s units=%d took=%s", fc.Path, len(parsed), debug.Since(ft))
			units = append(units, parsed...)
		}

		if existing, err := c.store.LoadService(key); err == nil {
			changedFiles := make(map[string]bool)
			for _, fc := range fileChanges {
				changedFiles[fc.Path] = true
			}
			for _, u := range existing.Units {
				if !changedFiles[u.File] {
					units = append(units, u)
				}
			}
		}
	}

	debug.Log("crawler", "indexRepo key=%s parsedFiles=%d totalUnits=%d took=%s", key, parsedFiles, len(units), debug.Since(t))

	signals := CollectSignals(repo.Path)
	resolveKafkaTopicsInUnits(units, signals.Config)
	checksums := ComputeChecksums(repo.Path)

	return &model.ServiceIndex{
		Key:           key,
		Path:          repo.Path,
		Platform:      platform,
		Units:         units,
		Config:        signals.Config,
		BuildDeps:     signals.BuildDeps,
		FileChecksums: checksums,
		IndexedAt:     time.Now(),
	}, nil
}

func (c *Crawler) loadStoredSHAs() map[string]string {
	m, err := c.store.LoadManifest()
	if err != nil {
		return nil
	}
	shas := make(map[string]string, len(m.Services))
	for _, svc := range m.Services {
		shas[svc.Key] = svc.HeadSHA
	}
	return shas
}

func (c *Crawler) saveResults(result *CrawlResult, repos []RepoInfo, changes []RepoChange) {
	services := make([]model.ServiceEntry, 0, len(repos))
	for _, ch := range changes {
		key := ServiceKey(ch.Repo.Workspace, ch.Repo.Name)
		platform := model.DetectPlatform(ch.Repo.Path)
		svc := model.ServiceEntry{
			Key:      key,
			Path:     ch.Repo.Path,
			Platform: platform.String(),
			HeadSHA:  ch.NewSHA,
		}
		if idx, err := c.store.LoadService(key); err == nil {
			svc.FileCount = len(idx.FileChecksums)
			svc.UnitCount = len(idx.Units)
			svc.LastIndexed = idx.IndexedAt.Format(time.RFC3339)
		}
		services = append(services, svc)
	}
	c.store.SaveManifest(&model.Manifest{
		Version:     1,
		Services:    services,
		GeneratedAt: time.Now(),
	})
	c.store.SaveHealth(&model.CrawlHealth{
		TotalRepos:    result.TotalRepos,
		Indexed:       result.Indexed,
		Failed:        result.Failed,
		CrawlDuration: result.Duration.Round(time.Millisecond).String(),
		Timestamp:     time.Now(),
	})
}

func hasExtension(path string, exts []string) bool {
	for _, ext := range exts {
		if filepath.Ext(path) == ext {
			return true
		}
	}
	return false
}

type CrawlResult struct {
	TotalRepos int
	Indexed    int
	Skipped    int
	Failed     []model.RepoFailure
	Duration   time.Duration
	StartedAt  time.Time
}

func (r *CrawlResult) String() string {
	return fmt.Sprintf("crawl: %d repos, %d indexed, %d skipped, %d failed (%s)",
		r.TotalRepos, r.Indexed, r.Skipped, len(r.Failed), r.Duration.Round(time.Millisecond))
}
