# Nexus — Code-Intelligence + Autonomous SDLC Engine

**Design Spec v1.0** — 2026-09-13
**Author:** realticsAI

---

## 1. Problem Statement

the AI agent has no understanding of how 100+ repos across multiple GitHub orgs connect. Developers repeatedly explain the same context: which service calls what, where endpoints live, what patterns the team follows. Every session starts from zero.

Existing solutions:
- **Legacy Tool** (Acme Corp) — Java monolith, 1200+ services, MCP server + dashboard. Excellent but org-specific and not portable.
- **AI-SDLC** (Vendor) — Pure prompt-engineering framework (122K lines of markdown). No compiled code. Runs entirely inside the AI agent's context window.

Neither is the right fit: Legacy Tool is tied to Acme Corp; AI-SDLC burns tokens on process overhead instead of code intelligence.

**Nexus** is a compiled Go binary that gives the AI agent deep codebase understanding AND autonomous development capability — from Jira ticket to PR — with minimal token overhead.

---

## 2. Comparison: Nexus vs AI-SDLC

### 2.1 Architecture Philosophy

| Dimension | AI-SDLC | Nexus |
|-----------|---------|-------|
| **What it is** | 122K lines of markdown/yaml prompts loaded into the AI agent's context | A compiled Go binary that does work locally, passes structured results to LLM |
| **Where intelligence lives** | Entirely in the LLM — every analysis, every decision is an LLM call | Indexing, graph building, search, pattern detection run as compiled code. LLM used only for planning + code generation |
| **Executable code** | Zero. Pure prompt engineering | Full Go codebase — tree-sitter parsing, graph algorithms, cosine similarity, git operations |
| **Per-run token cost** | ~500K-1M+ tokens. Every agent dispatched loads the canonical shared block (~7K lines of rules + config + role manifest + skill) PLUS feature-specific context | ~50K-100K tokens. Nexus returns structured JSON answers (500-2000 tokens each). LLM only called for plan creation + code generation |
| **Codebase understanding** | Relies on the AI agent reading files on-demand. No pre-built index, no graph, no semantic search | Pre-built index (index.json), dependency graph (graph.json), semantic embeddings. Sub-20ms queries |
| **Cross-service awareness** | None built-in. Each agent reads files in its scope. No HTTP_CALL, Kafka, federation edge detection | Full dependency graph: HTTP_CALL, KAFKA_PRODUCE/CONSUME, FRONTEND_API_CALL, BUILD_DEPENDENCY edges resolved algorithmically |
| **Startup time** | Instant (it's just markdown files) | <100ms (load pre-built JSON files) |
| **Dependency** | the AI agent only (no install) | Go binary + optional Ollama for embeddings |

### 2.2 Token Efficiency Analysis

**AI-SDLC per-run breakdown (estimated):**
```
Canonical shared block (loaded per agent dispatch):
  Agent config                       ~280 lines
  19 rule files                      ~5,200 lines
  Role manifest                      ~200 lines
  Skill file                         ~200 lines
  Template                           ~100 lines
  ─────────────────────────────────
  Subtotal per dispatch:             ~6,000 lines ≈ 24K tokens

Agent dispatches per full pipeline:
  PRE-WORK:    5-7 agents
  STEP-1:      4-6 agents
  STEP-2:      3 agents
  STEP-3:      3-4 agents
  STEP-4:      2-3 agents
  STEP-5:      3-4 agents
  STEP-6:      3 agents
  ─────────────────────────────────
  Total:       ~23-30 agent dispatches

Shared block tokens:    23 × 24K = ~552K (with cache: ~55K write + cache hits)
Per-agent work tokens:  23 × ~15K = ~345K
Feature-specific I/O:   ~100K
  ─────────────────────────────────
  Total estimate:        ~500K-1M tokens per feature
  With prompt caching:   ~300K-600K tokens per feature
```

**Nexus per-run breakdown (estimated):**
```
Agent engine calls LLM API:
  1. Investigation context (structured JSON from Nexus tools):  ~3K tokens
  2. Plan creation (1 LLM call):                                ~10K tokens
  3. Code generation (1-3 LLM calls):                           ~30K tokens
  4. Self-correction on test failure (0-3 calls):               ~15K tokens
  5. PR description generation:                                 ~2K tokens
  ─────────────────────────────────
  Total estimate:        ~50K-100K tokens per feature
```

**Nexus is 5-10x more token-efficient** because:
1. Indexing, graph traversal, search, pattern detection are compiled code — zero tokens
2. No rules/skills/manifests loaded into context — the Go binary encodes the process
3. One LLM call for planning, a few for code generation — not 23+ agent dispatches
4. Investigation results are structured JSON (500-2000 tokens), not full file reads

### 2.3 Strengths of AI-SDLC Worth Adopting

| AI-SDLC Strength | How Nexus Adopts It |
|-------------------|-------------------|
| **Phase gates (HITL)** — human approval required at key checkpoints | `nexus work --approve-each-step` mode. Safety guards require human approval before push/Jira transition |
| **Retry policy** — bounded retries (max 5) with explicit feedback | Nexus agent loop: max 3 self-correction attempts, each must address the specific failure |
| **Classification** — BUG vs FEATURE vs REFACTOR determines agent behavior | Nexus classifier: BUG/FEATURE/REFACTOR/CHORE — investigation strategy differs per type |
| **Audit trail** — every run produces a log of what happened | Nexus `_run-log.json` per ticket — all steps, LLM calls, tool results, test outcomes |
| **Write-once contracts** — phase outputs are immutable once produced | Nexus plan is frozen before execution. Edits during execution are tracked as deviations |
| **Model routing** — use Haiku for simple tasks, Sonnet for reasoning | Nexus uses the LLM only for reasoning/generation — all simple tasks are compiled code |

### 2.4 AI-SDLC Weaknesses Nexus Avoids

| AI-SDLC Weakness | Nexus Solution |
|------------------|---------------|
| **No codebase intelligence** — agents read files cold, no index, no graph | Pre-built index + dependency graph + semantic embeddings |
| **Token bloat** — 122K lines of framework loaded into context | Framework logic is compiled Go code, not prompts |
| **Process over product** — 7 steps + 30 rules + 28 agents before any code is written | Nexus: read ticket → investigate (local tools) → plan (1 LLM call) → implement |
| **No cross-repo awareness** — can't answer "what calls OrderService across all repos" | Dependency graph with typed edges, resolved algorithmically |
| **No incremental knowledge** — each run starts cold | Persistent index + knowledge store. Corrections remembered across runs |
| **Stack-specific everything** — 81K lines in stacks/ for 7 tech stacks | Tree-sitter handles any language with one grammar file per language |
| **Fragile phase ordering** — 23 agents in rigid sequence, any failure cascades | 4 simple phases (investigate → plan → execute → verify), each self-contained |
| **No search** — finding relevant code requires reading files in sequence | Hybrid search: keyword (inverted index) + semantic (embeddings) + graph (BFS) |

---

## 3. Token Efficiency Architecture

Nexus is designed from the ground up to minimize LLM token consumption. Every architectural decision asks: "can this run as compiled code instead of an LLM call?"

### 3.1 The Principle: Compiled Code Does the Heavy Lifting

| Operation | AI-SDLC Approach | Nexus Approach |
|-----------|-----------------|----------------|
| **Find relevant code** | LLM reads files sequentially, decides relevance | Inverted index + cosine similarity → structured JSON in <50ms |
| **Trace dependencies** | LLM reads imports, follows calls manually | Pre-built graph with typed edges, BFS traversal → zero tokens |
| **Detect patterns** | LLM reads each file, identifies patterns | Tree-sitter AST queries, compiled pattern matchers → zero tokens |
| **Classify ticket** | LLM reads ticket + rules manifest (~6K tokens) | Regex + keyword classifier on ticket title/description → zero tokens |
| **Investigate bug** | LLM reads 5-15 files (~50K tokens) | `search` + `trace_endpoint` + `find_error_paths` → structured JSON (~2K tokens) |
| **Decide workflow** | LLM reads process rules (~5K tokens) | Hardcoded phase flow in Go `switch` statement → zero tokens |
| **Generate PR description** | LLM reads rules + templates (~8K tokens) | Template with git diff summary → ~500 tokens |

### 3.2 Token Budget Targets

| Usage Scenario | Target | How |
|----------------|--------|-----|
| **MCP tool call** | <500 tokens response | Structured JSON, not prose. Only return what was asked |
| **Search query** | <1K tokens response | Top-10 results with name + file + line + score. No file contents |
| **Investigation context** | <3K tokens | Summarized findings from 5-10 tool calls, pre-formatted for LLM |
| **Plan generation** | ~10K tokens (input + output) | Ticket + investigation summary → step-by-step plan |
| **Code generation** | ~10-30K tokens per file | Plan step + target file content → edited file |
| **Full ticket (BUG)** | <60K tokens total | investigate(0) + plan(10K) + execute(20K) + verify(0) + ship(2K) |
| **Full ticket (FEATURE)** | <100K tokens total | investigate(0) + plan(10K) + execute(50K) + verify(0) + ship(2K) |

### 3.3 Design Rules for Token Efficiency

1. **No framework prompts** — process logic is Go code, not markdown rules loaded into context
2. **No agent dispatch overhead** — no canonical shared block re-loaded per phase
3. **Structured responses** — MCP tools return JSON with exact fields, not LLM-generated prose
4. **Investigation before LLM** — all code analysis happens locally BEFORE any LLM API call
5. **Minimal context window** — LLM receives ticket + investigation summary + plan, never raw files
6. **No retry bloat** — retries send only the failing step + error, not the whole context
7. **Embeddings are local** — cosine similarity runs in-process, not as an LLM embedding call

---

## 4. Architecture Resilience (Zero Bottlenecks)

Every layer is designed for 100+ repos, hundreds of daily commits, and zero single points of failure.

### 4.1 Sharded Storage (not monolithic JSON)

Single `index.json` would be 100-500MB at scale. Nexus shards per service:

```
~/.nexus/state/
├── manifest.json                      ← lightweight registry (< 1MB)
│   {services: [{key, path, platform, head_sha, file_count, last_indexed}]}
├── services/
│   ├── backend:commerce.cart.json     ← one shard per service (~50-500KB)
│   ├── backend:orders-service.json
│   ├── personal:my-api.json
│   └── ...
├── graph.json                         ← rebuilt from shards
└── embeddings/
    ├── backend:commerce.cart.bin
    └── ...
```

Crawl updates **one shard** — doesn't rewrite the whole index. Graph builder reads all shards in parallel via goroutines. Manifest stays small enough to load in <10ms.

### 4.2 Generational Snapshot Swap (no read/write races)

`nexus serve` reads the index while `nexus crawl` writes. Race condition → crash or inconsistent results.

```go
type IndexStore struct {
    mu      sync.RWMutex
    current *IndexSnapshot  // immutable once published
}

type IndexSnapshot struct {
    Generation int
    Manifest   *Manifest
    Graph      *Graph
    Embeddings *EmbeddingStore
}

// Crawl builds a NEW snapshot, then atomically swaps
func (s *IndexStore) Publish(snap *IndexSnapshot) {
    s.mu.Lock()
    s.current = snap  // old snapshot GC'd when last reader releases
    s.mu.Unlock()
}

// Serve always reads current — never sees partial writes
func (s *IndexStore) Current() *IndexSnapshot {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.current
}
```

On disk: write to temp file → `os.Rename()` (atomic on all OS).

### 4.3 Bounded Concurrency Pools

Unbounded goroutines → file descriptor exhaustion, OOM, network saturation.

```go
// Configurable per operation
crawl:
  git_pull_concurrency: 20     # parallel git pulls
  parse_concurrency: 8         # parallel tree-sitter parses (CPU-bound)
  embed_concurrency: 2         # parallel Ollama calls (GPU-bound)
```

Semaphore pattern on every parallel operation:
```go
sem := make(chan struct{}, cfg.GitPullConcurrency)
for _, repo := range repos {
    sem <- struct{}{}
    go func(r RepoInfo) {
        defer func() { <-sem }()
        pullRepo(r)
    }(repo)
}
```

### 4.4 Large Monorepo Fast-Path

Repos with 10K+ files: `os.ReadDir` + SHA-256 on every file = minutes. Use `git diff` instead:

```go
func DetectChanges(repo RepoInfo, lastHead string) ([]FileChange, error) {
    if repo.FileCount > 10000 {
        // O(changes) not O(total files)
        out, _ := exec.Command("git", "-C", repo.Path, "diff",
            "--name-status", lastHead+"..HEAD").Output()
        return parseGitDiff(out), nil
    }
    return filesystemDiff(repo)  // normal repos: walk + checksum
}
```

100K-file monorepo with 20 changes → instant (vs 30+ seconds for full walk).

### 4.5 Tree-sitter Query Batching (minimize CGo)

Each CGo boundary crossing costs ~100-200ns. Millions of AST node traversals = seconds of overhead.

```go
// ONE query captures ALL patterns — single tree walk
query, _ := sitter.NewQuery(javaLanguage, `
    (class_declaration name: (identifier) @class.name) @class
    (method_declaration name: (identifier) @method.name) @method
    (annotation name: (identifier) @ann.name) @annotation
    (method_invocation object: (identifier) @call.obj
        name: (identifier) @call.method) @call
`)
cursor := sitter.NewQueryCursor()
cursor.Exec(query, tree.RootNode())
// One pass through all matches — 10-100x fewer CGo calls than node-by-node walk
```

### 4.6 Embedding Pipeline (batch + async + LRU)

**Batch:** Ollama `/api/embed` supports batch input since v0.4. One HTTP call per service, not per unit.

**Async:** Serve uses previous embeddings while new ones generate. Never blocks queries.

**LRU eviction:** Don't load all embeddings into RAM. LRU cache holds hot services, cold ones read from disk on-demand (~1ms SSD).

```go
type EmbeddingStore struct {
    cache   *lru.Cache[string, [][]float32]  // service → vectors
    maxMem  int                               // max services in memory (default: 200)
}
```

At 500 services: all-in-memory = ~150MB. With LRU(200): ~60MB, 95%+ cache hit rate.

### 4.7 Semantic Search Pre-Filter

Brute-force cosine over 50K vectors = slow. Pre-filter with keywords first:

```go
func (s *Search) Hybrid(query string) []Result {
    // 1. Keyword search → top-30 candidate SERVICES
    candidates := s.keyword.Search(query, 30)

    // 2. Cosine similarity ONLY within candidates
    //    30 services × ~100 units = 3000 dot products (not 50K)
    semantic := s.semantic.SearchWithin(query, candidates)

    // 3. Graph bonus on keyword hits
    return s.blend(candidates, semantic)
}
```

Cuts search space by 90%. At extreme scale (1000+ services), add HNSW approximate nearest neighbor.

### 4.8 MCP Concurrent Request Handling

JSON-RPC over stdio = one request at a time by default. Fix: goroutine per request, mutex on stdout.

```go
func (s *MCPServer) Serve() {
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        msg := append([]byte{}, scanner.Bytes()...)  // copy
        go func() {
            result := s.dispatch(msg)
            s.writeMu.Lock()
            os.Stdout.Write(result)
            s.writeMu.Unlock()
        }()
    }
}
```

### 4.9 GitHub API ETag Caching

5000 requests/hour limit. With `--github-sync` every 10 min across 4 orgs, burns 1200/hour.

```go
// Conditional request with ETag — 304 responses don't count against rate limit
req.Header.Set("If-None-Match", cachedETag)
if resp.StatusCode == 304 {
    return cachedData  // FREE — no rate limit hit
}
```

After first sync, 90%+ of subsequent calls return 304.

### 4.10 Agent Investigation Parallelism

Investigation tools are independent — run them all concurrently:

```go
func (a *Agent) Investigate(ticket *TicketContext) *InvestigationResult {
    result := &InvestigationResult{}
    var wg sync.WaitGroup
    wg.Add(4)
    go func() { defer wg.Done(); result.Search = a.nexus.Search(ticket.Keywords()) }()
    go func() { defer wg.Done(); result.Deps = a.nexus.TraceDeps(ticket.Service()) }()
    go func() { defer wg.Done(); result.Errors = a.nexus.FindErrorPaths(ticket.Service()) }()
    go func() { defer wg.Done(); result.Patterns = a.nexus.DetectPatterns(ticket.Service()) }()
    wg.Wait()
    return result  // ~50ms total, not ~200ms sequential
}
```

LLM streaming: execute plan steps as they arrive, don't wait for full response.

### 4.11 Circuit Breakers (graceful degradation)

Every external dependency has a fallback:

| Dependency | Circuit Opens After | Fallback |
|------------|-------------------|----------|
| Ollama | 3 consecutive failures | ONNX → keyword-only search |
| Jira | 3 failures / 30s | Report error, don't crash |
| Confluence | 3 failures / 30s | Skip doc enrichment, use ticket only |
| Postman | 3 failures / 30s | Skip contract check |
| GitHub API | Rate limit header | Use cached repo list, skip sync |
| LLM API | 3 failures | Stop agent, report partial progress |

```go
type CircuitBreaker struct {
    failures   int
    threshold  int
    state      string  // closed | open | half-open
    resetAfter time.Duration
}
```

### 4.12 Config Hot-Reload

Adding a workspace path or changing crawl interval shouldn't require restart.

```go
// File watch on config.yml
watcher.Add(configPath)
for event := range watcher.Events {
    if event.Op&fsnotify.Write != 0 {
        newCfg := config.Load(configPath)
        server.ApplyConfig(newCfg)  // add/remove workspace watchers, update intervals
    }
}
// Also: kill -HUP <pid> triggers reload
```

### 4.13 Jira/Confluence Auto-Pagination

```go
func (c *JiraClient) SearchAll(jql string) ([]Issue, error) {
    var all []Issue
    for startAt := 0; ; {
        page, _ := c.search(jql, startAt, 100)
        all = append(all, page.Issues...)
        if startAt+len(page.Issues) >= page.Total { break }
        startAt += len(page.Issues)
    }
    return all, nil
}
```

### 4.14 Scale Ceiling Reference

| Scale | Services | Files | Embeddings RAM | Crawl Time | Search Latency |
|-------|----------|-------|----------------|------------|----------------|
| **Small** (personal) | 10-30 | ~5K | ~5MB (all in RAM) | ~10s | <10ms |
| **Medium** (team) | 50-200 | ~50K | ~60MB (all in RAM) | ~45s | <20ms |
| **Large** (org) | 200-500 | ~200K | ~150MB (LRU 200) | ~2min | <50ms |
| **XL** (multi-org) | 500-2000 | ~1M | ~600MB (LRU 200) | ~5min | <100ms (add HNSW) |

### 4.15 Agent Checkpoint/Resume (no wasted tokens on failure)

If `nexus work` fails at Phase 5 (tests fail after 3 retries), all prior work is lost without checkpointing.

```go
// ~/.nexus/runs/{ticket-key}/state.json — saved after each phase
type RunState struct {
    TicketKey     string    `json:"ticket_key"`
    Phase         string    `json:"phase"`
    Plan          *Plan     `json:"plan,omitempty"`
    Investigation *Context  `json:"investigation,omitempty"`
    FilesChanged  []string  `json:"files_changed,omitempty"`
    Attempts      int       `json:"attempts"`
    StartedAt     time.Time `json:"started_at"`
}

func (a *Agent) Work(ticketKey string) error {
    state := a.loadOrCreateState(ticketKey)
    // Resume from last completed phase — don't re-plan, don't re-investigate
    switch state.Phase {
    case "":            return a.understand(state)
    case "understand":  return a.investigate(state)
    case "investigate": return a.plan(state)
    case "plan":        return a.execute(state)
    case "execute":     return a.verify(state)
    case "verify":      return a.ship(state)
    }
    return nil
}
```

Also supports: `nexus work PROJ-1005 --from-phase=execute` to resume manually.

### 4.16 Index Health Reporting (no silent gaps)

If 5 of 100 repos fail to clone, the index shouldn't silently omit them.

```go
// ~/.nexus/state/health.json — written after every crawl
type CrawlHealth struct {
    TotalRepos    int           `json:"total_repos"`
    Indexed       int           `json:"indexed"`
    Failed        []RepoFailure `json:"failed"`
    CrawlDuration string        `json:"crawl_duration"`
    Timestamp     time.Time     `json:"timestamp"`
}
```

- `nexus status` shows: "98/103 repos indexed. 5 failed: [repo1: auth 401, repo2: timeout]"
- MCP `status` tool exposes this to the AI agent — so it can warn "results may be incomplete, payment-service wasn't indexed"
- Crawl continues on error — one repo failure doesn't halt the whole pipeline

### 4.17 Security: No run_command in MCP

The spec lists `run_command` as an MCP tool. **Removed.** the AI agent already has its own Bash tool with permission gates — Nexus providing another one bypasses those gates. The agent engine uses its own restricted executor internally (only build/test commands, scoped to target service, 5-minute timeout).

---

## 5. System Overview

### 4.1 One-Sentence Description

Nexus is a compiled Go binary that indexes 100+ multi-language repos into a searchable graph, serves that intelligence to the AI agent via MCP, and autonomously implements Jira tickets from investigation through PR.

### 4.2 Pipeline Architecture

```
┌─────────────┐    index.json    ┌─────────────┐   graph.json    ┌─────────────┐
│   STAGE 1   │  ─────────────►  │   STAGE 2   │  ────────────►  │   STAGE 3   │
│   Crawl     │   (per-file     │   Analyze    │  embeddings/   │  MCP Server  │
│   + Parse   │    AST data)    │   + Embed    │                │  + Agent     │
└─────────────┘                  └─────────────┘                └─────────────┘

CLI commands:
  nexus crawl ~/Workspaces        Stage 1 → index.json
  nexus analyze                   Stage 2 → graph.json + embeddings/
  nexus serve                     Stage 3 → MCP over stdio
  nexus work PROJ-1005            Agent engine (reads ticket, uses all 3 stages)
  nexus pipeline ~/Workspaces     Shortcut: crawl + analyze
```

Stages are independent processes connected by JSON files on disk. Each stage is testable, replaceable, and debuggable in isolation.

---

## 6. Stage 1: Crawler + Parser

### 5.1 Repo Discovery (Filesystem-First)

Nexus discovers repos by **walking parent directories on disk** — not from GitHub API. Any repo that's cloned locally gets indexed, regardless of origin (Acme Corp, personal, other orgs, even non-GitHub repos like GitLab or Bitbucket).

```
~/.nexus/config.yml:
workspaces:
  - path: ~/Workspaces/backend     ← Acme Corp backend repos cloned here
  - path: ~/Workspaces/web         ← Acme Corp web repos cloned here
  - path: ~/Projects/personal      ← personal repos cloned here
  - path: ~/Projects/freelance     ← any other repos
```

**Discovery logic:**
```go
func DiscoverRepos(workspaceDirs []string) []RepoInfo {
    var repos []RepoInfo
    for _, dir := range workspaceDirs {
        entries, _ := os.ReadDir(dir)
        for _, entry := range entries {
            repoPath := filepath.Join(dir, entry.Name())
            if isGitRepo(repoPath) {
                repos = append(repos, RepoInfo{
                    Path:      repoPath,
                    Name:      entry.Name(),
                    Workspace: filepath.Base(dir),  // "backend", "web", "personal"
                    Origin:    getGitRemoteOrigin(repoPath),  // optional — for org detection
                })
            }
        }
    }
    return repos
}
```

**How it works:**
1. Walk each configured workspace directory
2. Any subdirectory with `.git/` is a repo
3. Workspace name = parent folder name (e.g. `backend`, `web`, `personal`)
4. Service keys are namespace-prefixed: `backend:commerce.cart`, `personal:my-api`
5. Git remote origin is read (optional) — used for PR creation, not for discovery

**This means:**
- Clone any repo anywhere → add parent folder to config → `nexus crawl` → indexed
- No GitHub token needed for indexing (only needed if you want auto-clone via `--github-sync`)
- Mix Acme Corp + personal + any org in one unified index and graph
- Works with repos from GitHub, GitLab, Bitbucket, or even local-only repos with no remote

### 5.2 GitHub Auto-Sync (Optional)

GitHub API discovery is an **optional add-on** — it auto-clones new repos from configured orgs into the workspace directories. Useful for keeping up with new Acme Corp repos you haven't manually cloned.

```bash
nexus crawl ~/Workspaces                       # filesystem only — index what's on disk
nexus crawl ~/Workspaces --github-sync         # also clone new repos from configured orgs
```

When `--github-sync` is used:
- Discovers repos via GitHub REST API (paginated, multi-org)
- Shallow clone (`--filter=blob:none`) into the matching workspace dir
- Goroutines parallelize cloning across repos
- Only clones repos that don't already exist on disk

**SSO validation** — Acme Corp orgs require enterprise SSO-authorized PATs. Without SSO auth, the GitHub API silently returns a partial repo list. Nexus detects this:

```go
func (c *Client) ValidateOrgAccess(org OrgConfig) error {
    resp, _ := c.http.Head(fmt.Sprintf("https://api.github.com/orgs/%s/repos", org.Name))

    if ssoHeader := resp.Header.Get("X-GitHub-SSO"); org.SSO && strings.Contains(ssoHeader, "required") {
        return fmt.Errorf("org %s requires SSO — authorize your PAT at: %s", org.Name, extractURL(ssoHeader))
    }

    repos, _ := c.ListRepos(org.Name)
    if org.SSO && len(repos) < 10 {
        log.Warn("org %s returned only %d repos — PAT may not be SSO-authorized", org.Name, len(repos))
    }
    return nil
}
```

On `--github-sync`, if any SSO org fails validation, Nexus prints the authorization URL and stops — no silent partial index.

### 5.3 Incremental Index Strategy

With 100+ repos and hundreds of commits/day to main, a full re-parse every time would be wasteful. Nexus uses a **4-layer incremental strategy**:

#### Layer 1: Git Pull (what changed at repo level)

```go
func (c *Crawler) PullAll(repos []RepoInfo) []RepoChange {
    // Parallel goroutines — pull all repos concurrently
    // git fetch + git rev-parse HEAD → compare with stored HEAD SHA
    // Skip repos where HEAD hasn't moved since last crawl
}
```

If a repo's HEAD hasn't changed → skip entirely. On a typical day, maybe 50 of 100 repos have new commits. The other 50 are untouched — zero work.

#### Layer 2: File Checksum Diff (what changed at file level)

```go
type FileChecksum struct {
    Path     string `json:"path"`
    SHA256   string `json:"sha256"`
    Service  string `json:"service"`   // which service this file belongs to
    ModTime  int64  `json:"mod_time"`  // fast pre-filter before hashing
}
```

For repos with new commits:
1. Walk files, check `ModTime` first (filesystem metadata, instant)
2. Only SHA-256 hash files where `ModTime` changed (avoid hashing 90% of files)
3. Compare against stored checksums in `index.json`
4. Collect the list of **changed files** and **affected services**

Even with 100 commits in a repo, usually only 20-50 files actually changed.

#### Layer 3: Targeted Re-parse (only changed files)

```go
func (c *Crawler) IncrementalParse(changes []FileChange) {
    for _, change := range changes {
        switch change.Type {
        case Modified:
            // Re-parse this one file with tree-sitter, update its units in index
        case Added:
            // Parse new file, add to service's unit list
        case Deleted:
            // Remove units from this file, clean up references
        }
    }
}
```

Tree-sitter parses one file in ~5ms. Even 500 changed files = ~2.5 seconds.

#### Layer 4: Selective Rebuild (only affected parts of graph)

```go
func (a *Analyzer) IncrementalRebuild(affectedServices []string) {
    // Graph edges: rebuild only edges FROM or TO affected services
    // Keyword index: update only tokens from changed files
    // Embeddings: re-embed only affected services
}
```

The graph rebuild is the one place where Legacy Tool chose full rebuild every time (fast enough at their scale). Nexus does the same for v1 — graph.json is rebuilt from index.json on every analyze, because:
- Reading index.json + building graph takes ~3 seconds even at 100+ services
- Incremental graph updates are complex (edge deletion, transitive effects) and error-prone
- Full rebuild guarantees correctness

**Embeddings** are the expensive part — only re-embed services with changed files.

#### Timing Breakdown (typical incremental run)

```
Step                                  Time        Why
─────────────────────────────────────────────────────────
git pull 100 repos (parallel)         ~30s        Network bound, 20 goroutines
Skip 50 unchanged repos               0s         HEAD SHA match
ModTime check 50 repos                ~2s        Filesystem metadata only
SHA-256 hash ~2000 changed files      ~1s        Only files with new ModTime
Tree-sitter parse ~200 changed files  ~1s        ~5ms per file
Rebuild graph from index.json         ~3s        Full rebuild, reads JSON not source
Re-embed ~15 affected services        ~10s       Ollama inference
Write index.json + graph.json         ~1s        JSON marshal
─────────────────────────────────────────────────────────
Total incremental:                    ~45s
Full reindex (first run):             ~5-10 min   Parse all files + embed all services
```

#### When to Reindex

| Mode | Command | When |
|------|---------|------|
| **On-demand** | `nexus crawl` | Developer runs manually |
| **Watch** | `nexus crawl --watch` | Watches workspace dirs via fsnotify, re-crawls on file change |
| **Scheduled** | `schedule.crawl_interval: 10m` in config | Background goroutine in `nexus serve`, pulls + re-indexes every N minutes |
| **Pre-query** | Automatic in `nexus serve` | If index is older than `max_age` (configurable), trigger background crawl before serving |
| **Full** | `nexus crawl --full` | Forces full re-parse, ignores checksums — use after schema version bump |

#### Index Staleness Awareness

Every tool response includes index age:
```json
{
  "results": [...],
  "trace": {
    "index_age": "3m",
    "services_stale": ["orders-service"],
    "confidence": "HIGH"
  }
}
```

If a service was last indexed >1 hour ago and its repo has new commits, TraceContext downgrades confidence to MEDIUM and adds a staleness note.

### 5.4 Platform Detection

```go
type Platform int
const (
    JAVA Platform = iota    // pom.xml | build.gradle | src/main/java/
    TYPESCRIPT              // package.json present
    PYTHON                  // pyproject.toml | requirements.txt | setup.py
    UNKNOWN
)
```

Framework detection happens inside the extractor (after Tree-sitter parsing), not during platform detection.

### 5.5 Tree-sitter Parsing

One parser engine, three grammars (via `go-tree-sitter`):

| Grammar | Extracts |
|---------|----------|
| `tree-sitter-java` | Classes, methods, annotations (@RestController, @KafkaListener, @ConfigurationProperties), WebClient targets, dependencies, throw sites |
| `tree-sitter-typescript` | Functions, components (JSX return), fetch/axios API calls, useQuery/useMutation, Route paths, imports |
| `tree-sitter-python` | Classes, functions, @app.route/@router.get endpoints, requests/httpx calls, imports |

**SignalCollector** runs cross-language over config files (application.yml, .env, pyproject.toml) and build files (pom.xml, package.json, requirements.txt).

### 5.6 Output: index.json

```json
{
  "version": 1,
  "services": {
    "orders-service": {
      "path": "backend/orders-service",
      "platform": "java",
      "units": [
        {
          "type": "class",
          "name": "OrderController",
          "fqn": "com.myorg.orders.OrderController",
          "file": "src/.../OrderController.java",
          "line": 15,
          "stereotype": "rest_controller",
          "methods": [...],
          "dependencies": [...],
          "config_refs": [...],
          "kafka_produces": [...],
          "kafka_consumes": [...]
        }
      ],
      "config": { "app.payment-service.url": "http://payment-svc:8080/..." },
      "build_deps": ["commons-lib", "spring-boot-starter-web"]
    }
  },
  "file_checksums": { "path": "sha256:..." }
}
```

---

## 7. Stage 2: Analyzer + Embedder

### 6.1 Graph Building (5 Resolvers)

1. **ServiceRegistry** — builds name/path/host lookup tables from index.json
2. **HttpEdgeResolver** — resolves config_refs + webclient_targets + config URL values → HTTP_CALL edges. Resolves ${placeholder} syntax via config map
3. **KafkaEdgeResolver** — matches kafka_produces × kafka_consumes by topic → KAFKA_PRODUCE/CONSUME edges
4. **ApiCallResolver** — matches frontend api_calls to backend endpoints via path lookup → FRONTEND_API_CALL edges
5. **BuildDepResolver** — matches build dependencies to indexed services → BUILD_DEPENDENCY edges

### 6.2 Edge Types

```go
type EdgeType string
const (
    HTTPCall        EdgeType = "HTTP_CALL"
    KafkaProduce    EdgeType = "KAFKA_PRODUCE"
    KafkaConsume    EdgeType = "KAFKA_CONSUME"
    FrontendAPICall EdgeType = "FRONTEND_API_CALL"
    BuildDependency EdgeType = "BUILD_DEPENDENCY"
    ImportDep       EdgeType = "IMPORT_DEPENDENCY"
)
```

### 6.3 Keyword Index

Built while walking index.json. Inverted index: token → [{service, type, name, score}].

Scoring: service name = 15, class/function = 10, method = 8, endpoint/annotation = 5.

### 6.4 Embeddings

Provider hierarchy:
1. **Ollama** (`localhost:11434`) — `nomic-embed-text` model, 384-dim vectors
2. **ONNX Runtime** (`onnxruntime-go`) — bundled MiniLM model, offline fallback
3. **Disabled** — keyword + graph search still work

Binary format per service. In-memory cosine similarity at query time. No external vector DB.

### 6.5 Output: graph.json + embeddings/

graph.json contains nodes, edges, and the keyword index. embeddings/ contains binary vector files per service plus a manifest.json.

---

## 8. Stage 3: MCP Server + Tools

### 7.1 MCP Protocol

JSON-RPC 2.0 over stdio. Reads from stdin, writes to stdout. No framework — plain Go with `bufio.Scanner` + `encoding/json`.

### 7.2 Tool Set (21 Tools)

**Search (3):** `search`, `grep`, `find_endpoints`
**Read (4):** `read_file`, `list_services`, `service_profile`, `explain_class`
**Graph (4):** `trace_dependencies`, `trace_dependents`, `impact_analysis`, `trace_flow`
**Write (2):** `write_file`, `edit_file` (no `run_command` — the AI agent has its own Bash with permission gates)
**Developer Assist (5):** `detect_patterns`, `scaffold_endpoint`, `trace_endpoint`, `find_error_paths`, `check_service`
**System (2):** `status`, `reindex`
**Learning (2):** `correct`, `forget`

### 7.3 Hybrid Search (3-pass)

1. **Keyword** — inverted index lookup (exact + prefix + fuzzy via Levenshtein ≤2)
2. **Semantic** — cosine similarity over embeddings
3. **Graph** — BFS 1-hop neighbors of keyword matches get a +3 bonus

Score blending: keyword score + semantic (scaled 0-10) + graph bonus. Dedup by (service, unit_name).

### 7.4 TraceContext

Every tool response includes confidence (HIGH/MEDIUM/LOW), services consulted, data sources used, and provenance (traced vs inferred claims).

---

## 9. Agent Engine (Autonomous SDLC)

### 8.1 Overview

```bash
nexus work PROJ-1005    # Ticket in → PR out
```

Reads a Jira ticket, classifies it, investigates using Nexus tools, plans via LLM API, executes the plan, runs tests, creates a PR, updates Jira.

### 8.2 Phase Flow

```
Phase 1: UNDERSTAND
  → Read Jira ticket
  → Classify: BUG | FEATURE | REFACTOR | CHORE

Phase 2: INVESTIGATE (compiled code, zero LLM tokens)
  → BUG: search(error) → trace_endpoint → find_error_paths → suggest_fix
  → FEATURE: detect_patterns → find_endpoints → trace_dependencies
  → REFACTOR: impact_analysis → trace_dependents → check_service

Phase 3: PLAN (1 LLM call)
  → Send ticket + investigation context to LLM API
  → Receive step-by-step plan (files to create/edit, what to change)
  → Validate plan (file paths exist, scope within target service)

Phase 4: EXECUTE (1-3 LLM calls)
  → Create git branch: nexus/{ticket-key}
  → For each step: edit_file / write_file / scaffold_endpoint
  → Self-correction: if step fails → send error to LLM → corrected step → retry (max 3)

Phase 5: VERIFY (compiled code, zero LLM tokens)
  → Run build command (mvn compile / npx tsc / python -m py_compile)
  → Run tests (mvn test / npm test / pytest)
  → If tests fail → send failure to LLM → fix → re-verify (max 3)

Phase 6: SHIP
  → Commit and push
  → Create PR via gh CLI
  → Add structured comment to Jira ticket
  → Transition ticket to "In Review"
```

### 8.3 Token Budget Per Ticket

| Phase | LLM Calls | Estimated Tokens |
|-------|-----------|-----------------|
| Understand | 0 | 0 (Jira REST API) |
| Investigate | 0 | 0 (local Nexus tools) |
| Plan | 1 | ~10K (ticket + context → plan) |
| Execute | 1-3 | ~30K (plan → code) |
| Self-correct | 0-3 | ~15K (only on failure) |
| Ship | 0-1 | ~2K (PR description) |
| **Total** | **2-8** | **~50-100K** |

Compare AI-SDLC: 23-30 agent dispatches, ~300K-600K tokens with caching.

### 8.4 External Integrations (Direct REST API — No MCP)

Nexus talks to Jira, Confluence, and Postman via **direct HTTP calls from Go** — not through MCP tools. This is deliberate:

| Why REST API beats MCP tools | Impact |
|------------------------------|--------|
| **Zero token cost** — Go HTTP client, no LLM involvement | MCP tool calls consume context window tokens for schema + request + response |
| **Typed responses** — Go structs with `json:"..."` tags, compile-time safe | MCP returns untyped JSON that the LLM must parse |
| **No schema loading step** — just call the function | MCP requires `ToolSearch` → schema load → then call |
| **Batching + pagination built-in** — Go handles retry, rate limits, pagination natively | MCP tools are single-shot, one call per request |
| **No JSON-RPC overhead** — direct HTTPS to the API | MCP adds stdio JSON-RPC dispatch layer |
| **Works inside agent loop** — investigation phase calls Jira/Confluence directly | MCP tools only work when the AI agent dispatches them |

#### Jira Client

```go
type JiraClient struct {
    baseURL    string       // from config.yml (e.g. https://jira.yourcompany.com)
    token      string       // from NEXUS_JIRA_TOKEN env (PAT or API token)
    httpClient *http.Client // with timeout, retry, rate-limit
}

// Core operations (REST API v2)
func (c *JiraClient) GetIssue(key string) (*Issue, error)           // GET /rest/api/2/issue/{key}
func (c *JiraClient) SearchJQL(jql string) ([]Issue, error)         // POST /rest/api/2/search
func (c *JiraClient) AddComment(key, body string) error             // POST /rest/api/2/issue/{key}/comment
func (c *JiraClient) GetTransitions(key string) ([]Transition, error) // GET /rest/api/2/issue/{key}/transitions
func (c *JiraClient) Transition(key, transitionID string) error     // POST /rest/api/2/issue/{key}/transitions
func (c *JiraClient) UpdateFields(key string, fields map[string]any) error // PUT /rest/api/2/issue/{key}
func (c *JiraClient) GetSprint(boardID int) (*Sprint, error)        // GET /rest/agile/1.0/board/{id}/sprint
```

**Issue struct** — parsed from Jira JSON into typed Go:
```go
type Issue struct {
    Key         string   `json:"key"`
    Summary     string   `json:"summary"`
    Description string   `json:"description"`
    Type        string   `json:"issuetype"`    // Bug, Story, Task
    Status      string   `json:"status"`
    Priority    string   `json:"priority"`
    Assignee    string   `json:"assignee"`
    Labels      []string `json:"labels"`
    Components  []string `json:"components"`
    AccCriteria string   `json:"acceptance_criteria"` // custom field
}
```

#### Confluence Client

```go
type ConfluenceClient struct {
    baseURL    string       // from config.yml (e.g. https://confluence.yourcompany.com)
    token      string       // from NEXUS_CONFLUENCE_TOKEN env (separate PAT from Jira)
    httpClient *http.Client
}

// Used during investigation to pull design docs, TDs, ADRs
func (c *ConfluenceClient) SearchCQL(cql string) ([]Page, error)       // GET /rest/api/content/search
func (c *ConfluenceClient) GetPage(pageID string) (*Page, error)       // GET /rest/api/content/{id}?expand=body.storage
func (c *ConfluenceClient) GetChildPages(pageID string) ([]Page, error)
func (c *ConfluenceClient) CreatePage(spaceKey, title, body string) (*Page, error)
func (c *ConfluenceClient) UpdatePage(pageID, title, body string, version int) error
```

**Agent uses Confluence for:**
- Reading technical design docs linked from Jira tickets
- Pulling architecture decision records (ADRs) for context
- Creating/updating pages with investigation reports or implementation notes

#### Postman Client

```go
type PostmanClient struct {
    apiKey     string       // from NEXUS_POSTMAN_API_KEY env
    httpClient *http.Client
}

// Used to discover and test API contracts
func (c *PostmanClient) SearchCollections(query string) ([]Collection, error) // GET /collections?name={query}
func (c *PostmanClient) GetCollection(id string) (*Collection, error)        // GET /collections/{id}
func (c *PostmanClient) SearchRequests(query string) ([]Request, error)
func (c *PostmanClient) GetRequest(id string) (*Request, error)              // GET /requests/{id}
func (c *PostmanClient) ExportAsCurl(requestID string) (string, error)
func (c *PostmanClient) ListEnvironments() ([]Environment, error)
```

**Agent uses Postman for:**
- Discovering existing API contracts during investigation
- Verifying endpoint signatures match what the code expects
- Exporting curl commands for integration test scaffolding

#### Token Savings from REST vs MCP

| Operation | Via MCP Tools | Via Direct REST |
|-----------|--------------|-----------------|
| Read Jira ticket | ~2K tokens (schema load + tool call + response in context) | 0 tokens (Go HTTP, result is typed struct) |
| Search Confluence | ~3K tokens (CQL in context + page content returned as text) | 0 tokens (Go parses HTML → extracts relevant sections) |
| Search Postman | ~2K tokens (schema + response) | 0 tokens (Go filters to endpoint signatures only) |
| Full investigation (Jira + Confluence + Postman) | ~10-15K tokens | 0 tokens |

This saves **10-15K tokens per ticket** just in the investigation phase.

### 8.5 LLM Client

```go
type LLMClient interface {
    Complete(prompt string, maxTokens int) (string, error)
}
```

Two implementations: `AnthropicClient` (LLM API) and `OllamaClient` (local Ollama for reasoning — lower quality but zero cost). Configured in config.yml.

### 8.6 Safety Guards

| Guard | What it prevents |
|-------|-----------------|
| `max_files_changed: 10` | Runaway rewrites |
| `require_tests: true` | Shipping untested code |
| `auto_push: false` (default) | Pushing without human approval |
| `auto_transition: false` (default) | Moving Jira tickets without approval |
| No force push, ever | Destroying git history |
| No changes outside target service | Unintended blast radius |
| 3 retry limit | Infinite self-correction loops |
| `--dry-run` mode | See the plan without executing |
| `--approve-each-step` mode | Human approval at every step |

---

## 9. Project Structure

```
nexus/
├── go.mod
├── cmd/nexus/
│   ├── main.go              ← Root Cobra command
│   ├── crawl.go             ← nexus crawl
│   ├── analyze.go           ← nexus analyze
│   ├── serve.go             ← nexus serve (MCP)
│   ├── work.go              ← nexus work (agent)
│   ├── pipeline.go          ← nexus pipeline (crawl+analyze)
│   └── status.go            ← nexus status
│
├── internal/
│   ├── config/              ← Config loading
│   ├── crawler/             ← Stage 1: git sync + tree-sitter parsing
│   │   ├── sync.go, detector.go, parser.go
│   │   ├── java.go, typescript.go, python.go
│   │   ├── signals.go, index.go
│   ├── analyzer/            ← Stage 2: graph + keyword index
│   │   ├── registry.go, http_edges.go, kafka_edges.go
│   │   ├── api_edges.go, build_edges.go, keywords.go, graph.go
│   ├── embedder/            ← Embedding generation
│   │   ├── provider.go, ollama.go, onnx.go, store.go, math.go
│   ├── search/              ← Hybrid search engine
│   │   ├── engine.go, keyword.go, semantic.go, expander.go, scorer.go
│   ├── graph/               ← Graph types + traversal
│   │   ├── types.go, graph.go, impact.go
│   ├── mcp/                 ← MCP JSON-RPC server
│   │   ├── server.go, protocol.go, types.go
│   ├── tools/               ← 21 MCP tool implementations
│   ├── knowledge/           ← Learning store
│   ├── trace/               ← TraceContext
│   ├── jira/                ← Jira REST client (direct HTTP)
│   ├── confluence/          ← Confluence REST client (direct HTTP)
│   ├── postman/             ← Postman REST client (direct HTTP)
│   ├── github/              ← Git + PR operations
│   ├── llm/                 ← LLM client (Anthropic + Ollama)
│   └── agent/               ← Autonomous SDLC engine
│       ├── agent.go, classifier.go, investigator.go
│       ├── planner.go, executor.go, verifier.go, reporter.go
│
├── model/                   ← Public types (shared between stages)
│   ├── index.go, graph.go, embedding.go
│
├── Makefile
├── Dockerfile
├── .goreleaser.yml
└── run-mcp.sh
```

---

## 10. Configuration

```yaml
# ~/.nexus/config.yml
# Primary discovery — index everything under these parent folders
# Any subfolder with .git/ is treated as a repo. No GitHub token needed.
workspaces:
  - path: ~/Workspaces/backend     # Acme Corp backend repos
  - path: ~/Workspaces/web         # Acme Corp web repos
  - path: ~/Projects/personal      # personal repos
  - path: ~/Projects/freelance     # any other repos

# Optional — auto-clone new repos from GitHub orgs into workspace dirs
# Only used with `nexus crawl --github-sync`
github:
  sync_targets:
    - org: acme-backend
      sso: true                     # enterprise SSO — PAT must be SSO-authorized
      clone_into: ~/Workspaces/backend
    - org: acme-web
      sso: true
      clone_into: ~/Workspaces/web
    - org: acme-alpha
      sso: true
      clone_into: ~/Workspaces/backend
    - org: personal-org
      sso: false
      clone_into: ~/Projects/personal
  token_env: GITHUB_TOKEN           # classic PAT with repo scope, SSO-authorized for Acme Corp orgs
  exclude: ["archived-*"]

embeddings:
  provider: ollama              # ollama | onnx | none
  ollama_url: http://localhost:11434
  model: nomic-embed-text
  onnx_model_path: ~/.nexus/models/minilm.onnx

jira:
  base_url: https://jira.yourcompany.com
  token_env: NEXUS_JIRA_TOKEN

confluence:
  base_url: https://confluence.yourcompany.com
  token_env: NEXUS_CONFLUENCE_TOKEN     # separate PAT from Jira

postman:
  api_key_env: NEXUS_POSTMAN_API_KEY    # Postman API key from postman.co

llm:
  provider: anthropic           # anthropic | ollama
  model: claude-sonnet-5
  api_key_env: ANTHROPIC_API_KEY
  max_retries: 3

agent:
  auto_push: false
  auto_transition: false
  max_files_changed: 10
  require_tests: true
  branch_prefix: "nexus/"

schedule:
  crawl_interval: 10m
```

---

## 11. Build & Distribution

```makefile
build:
    CGO_ENABLED=1 go build -o bin/nexus ./cmd/nexus

test:
    go test ./...

install:
    go install ./cmd/nexus

release:
    goreleaser release
```

**Docker (2-stage):**
```dockerfile
FROM golang:1.23-alpine AS build
COPY . /src
RUN cd /src && CGO_ENABLED=1 go build -o /nexus ./cmd/nexus

FROM alpine:3.20
RUN apk add --no-cache git
COPY --from=build /nexus /usr/local/bin/nexus
ENTRYPOINT ["nexus"]
```

CGO_ENABLED=1 for Tree-sitter + ONNX. Binary ~25MB.

---

## 12. CLI Reference

```bash
# Indexing
nexus crawl ~/Workspaces                    # Stage 1
nexus crawl --github-org myorg              # With GitHub discovery
nexus crawl --full                          # Force full re-parse
nexus analyze                               # Stage 2
nexus analyze --skip-embeddings             # No Ollama needed
nexus pipeline ~/Workspaces                 # crawl + analyze

# MCP Server
nexus serve                                 # stdio MCP for the AI agent
nexus serve --watch                         # Auto-reload on index change

# Agent
nexus work PROJ-1005                        # Full autonomous: ticket → PR
nexus investigate PROJ-1005                 # Just read + investigate
nexus plan PROJ-1005                        # Investigate + plan, stop
nexus execute PROJ-1005                     # Execute approved plan
nexus verify PROJ-1005                      # Run tests on current branch
nexus work PROJ-1005 --dry-run              # Show plan, don't execute
nexus work PROJ-1005 --approve-each-step    # Pause at each step
nexus work PROJ-1005 PROJ-1006              # Batch tickets

# Utilities
nexus status                                # Index age, service count, graph stats
```

---

## 13. Wiring to the AI agent

```bash
# Install
go install github.com/yourorg/nexus@latest
# or: brew install nexus

# First index
nexus pipeline ~/Workspaces --github-org myorg

# Register as MCP server (example for one agent CLI)
# Refer to your AI agent's docs for MCP server registration
nexus serve  # stdio-based MCP server
```

---

## 14. Success Criteria

1. **MCP server starts in <100ms** (no JVM warmup)
2. **Search returns in <50ms** for any query across 100+ services
3. **Dependency graph resolves 80%+ of real HTTP/Kafka edges** (validated against manual audit)
4. **Agent completes a BUG ticket in <5 minutes** (investigation + fix + tests + PR)
5. **Agent completes a FEATURE ticket in <15 minutes** (investigation + scaffold + implement + tests + PR)
6. **Token cost per ticket: <100K tokens** (5-10x cheaper than AI-SDLC)
7. **Zero false pushes** — safety guards prevent any unintended push/transition

---

## 15. Implementation Phases

| Phase | What | Duration |
|-------|------|----------|
| **P1: Foundation** | Go project scaffold, config, CLI commands, model types | 1 week |
| **P2: Crawler** | Git sync, platform detection, tree-sitter Java extractor | 2 weeks |
| **P3: Graph** | 5 resolvers, keyword index builder, graph traversal | 2 weeks |
| **P4: MCP Server** | JSON-RPC stdio, 15 core tools, hybrid search | 2 weeks |
| **P5: Embeddings** | Ollama provider, ONNX fallback, vector store, semantic search | 1 week |
| **P6: Multi-lang** | TypeScript + Python tree-sitter extractors | 2 weeks |
| **P7: Agent** | Jira client, LLM client, classifier, investigator, planner, executor | 3 weeks |
| **P8: Polish** | Safety guards, --dry-run, --approve-each-step, audit logging | 1 week |
| **Total** | | **~14 weeks** |

---

## 16. Open Questions

1. **GraphQL federation support** — should Nexus detect @key entities and build FEDERATION_ENTITY_REF edges? (Yes if your services use Apollo Federation / Cosmo)
2. **Confluence/Postman integration** — as separate MCP servers or built into Nexus?
3. **Dashboard** — deferred for now, but should graph.json be designed to support a future React dashboard?
4. **Team server mode** — shared index via HTTP API, or each developer runs their own instance?
5. **Kotlin/Swift support** — tree-sitter grammars exist. Add to P6 or defer?
