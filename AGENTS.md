# Nexus — Agent Instructions

> For any AI coding agent (MCP-compatible)

## What is Nexus

Nexus is a compiled Go binary that gives AI coding agents deep codebase understanding across 100+ repos. It indexes multi-language repos (Java, TypeScript, Python) into a searchable dependency graph and serves that intelligence via MCP (Model Context Protocol). It also has an autonomous SDLC engine that goes from Jira ticket to PR.

**Nexus is NOT an LLM.** It's a retrieval + tooling layer. The AI agent (you) provides reasoning; Nexus provides structured context.

## Build & Test

```bash
# Build (requires CGO_ENABLED=1 for tree-sitter + future ONNX)
make build          # outputs bin/nexus

# Test
go test ./... -v    # all tests
go test ./internal/crawler/ -v   # just crawler
go test ./internal/analyzer/ -v  # just analyzer

# Run
./bin/nexus crawl           # index repos from configured workspaces
./bin/nexus crawl --full    # force full re-parse
./bin/nexus analyze         # build dependency graph from index
./bin/nexus serve           # start MCP server (stdio)
./bin/nexus pipeline        # crawl + analyze + serve in sequence
./bin/nexus status          # show index health
./bin/nexus work TICKET-ID  # autonomous: Jira ticket to PR
```

## Project Layout

```
cmd/nexus/          CLI commands (Cobra). Each file = one subcommand.
model/              Shared types between all stages. Public API.
internal/
  config/           Config loading from ~/.nexus/config.yml
  store/            Sharded JSON storage with atomic writes
  crawler/          Stage 1 — repo discovery, git sync, tree-sitter parsing
  analyzer/         Stage 2 — 5 edge resolvers, keyword index, graph builder
  graph/            Graph traversal (BFS/DFS), impact analysis
  embedder/         Ollama embedding provider, binary vector store, cosine similarity
  search/           Hybrid search engine (keyword + graph BFS + semantic interface)
  mcp/              JSON-RPC 2.0 MCP server over stdio
  tools/            11 MCP tool implementations (search, graph, read, status, review)
  jira/             Jira REST v2 client (GetIssue, SearchJQL, AddComment, Transitions)
  confluence/       Confluence REST client (SearchCQL, GetPage, CreatePage)
  llm/              LLM client — Anthropic + Ollama providers, factory from config
  gitops/           Git operations (branch, commit, push, PR creation, safety guards)
  agent/            Autonomous SDLC engine (ticket → PR in 6 phases)
  postman/          (planned) Postman REST client
```

## Architecture (3-Stage Pipeline)

1. **Crawl** — walk workspace dirs, discover git repos, pull, parse with tree-sitter, write per-service JSON shards to `~/.nexus/state/services/`
2. **Analyze** — read shards, resolve cross-service edges (HTTP, Kafka, frontend API calls, build deps), build keyword index, write `graph.json`
3. **Serve** — MCP server over stdio, hybrid search (keyword + semantic + graph BFS), 21 tools

Stages connect via JSON files on disk, not in-memory. Each can run independently.

## Key Design Decisions

- **Filesystem-first repo discovery** — walk parent dirs for `.git/`, not GitHub API. Any local repo gets indexed.
- **Sharded storage** — one JSON file per service, not a monolithic index. Atomic writes via temp + rename.
- **Tree-sitter parsing** — CGo bindings (`github.com/smacker/go-tree-sitter`). One batched query per file, not node-by-node walks.
- **Bounded concurrency** — semaphore pattern on all parallel ops (git pulls, parsing, embedding). Configurable in `config.yml`.
- **4-layer incremental indexing** — HEAD SHA check, ModTime pre-filter, SHA-256 diff, targeted re-parse. Avoids full repo walks.
- **LLM-agnostic** — config supports `anthropic | openai | ollama | custom` providers. MCP client field is informational only.
- **Separate PATs** — Jira and Confluence each carry their own token via env vars.

## Config

Nexus loads config from `~/.nexus/config.yml`. Missing file = sensible defaults (Ollama embeddings, Anthropic LLM, generic MCP client).

Key config sections: `workspaces`, `github` (sync_targets with SSO flags), `embeddings`, `jira`, `confluence`, `postman`, `llm`, `mcp`, `agent`, `crawl` (concurrency limits), `schedule`.

See `internal/config/config.go` for the full struct and defaults.

## State

All persistent state lives under `~/.nexus/state/`:
```
manifest.json               lightweight service registry
services/<key>.json          per-service index shard
graph.json                   dependency graph + keyword index
health.json                  last crawl stats
embeddings/                  (planned) binary vector files
```

## Conventions

- Go 1.24, `CGO_ENABLED=1`
- No comments unless the WHY is non-obvious
- Prefer small, focused files over large ones
- Tests live next to the code they test (`*_test.go` in the same package)
- Error handling: return errors, don't panic. Log warnings for non-fatal issues.
- Concurrency: always use bounded semaphore pattern, never unbounded goroutines
- JSON on disk: pretty-printed (2-space indent) via `json.Encoder.SetIndent`
- Atomic writes: write to `.tmp`, then `os.Rename()`
- Service keys: `workspace:name` format (e.g., `backend:commerce.cart`)

## Design Spec

Full design spec with all architecture details, token efficiency analysis, bottleneck analysis, and implementation phases: `docs/specs/2026-09-13-nexus-design.md`
