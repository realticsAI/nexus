# Nexus

[![Latest release](https://img.shields.io/github/v/release/realticsAI/nexus)](https://github.com/realticsAI/nexus/releases/latest)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![CI](https://img.shields.io/github/actions/workflow/status/realticsAI/nexus/go.yml?label=CI)](https://github.com/realticsAI/nexus/actions)
[![Languages](https://img.shields.io/badge/languages-6-orange)](#languages-supported)
[![MCP Tools](https://img.shields.io/badge/MCP_tools-14-blueviolet)](#mcp-tools-14-total)
[![Platform](https://img.shields.io/badge/macOS_%7C_Linux-supported-lightgrey)](https://github.com/realticsAI/nexus/releases/latest)
[![Zero LLM](https://img.shields.io/badge/LLM_tokens-zero-brightgreen)](#how-it-works)

**Zero-LLM code intelligence engine for AI coding agents.** Indexes multi-language, multi-repo
codebases into a searchable dependency graph and serves it via
[MCP](https://modelcontextprotocol.io) — giving your AI agent deep structural understanding
of your entire codebase without spending a single LLM token.

Nexus is a **context engine**, not an AI agent. It provides code intelligence — files, classes,
methods, endpoints, call chains, dependency graphs, config references, Kafka flows, and library
APIs — combined with Jira ticket data. Your AI agent is the brain: it extracts requirements,
makes architectural decisions, resolves ambiguity, and writes code. Nexus provides the raw
material; your agent provides the reasoning.

- [Quick Start](#quick-start)
- [MCP Tools](#mcp-tools-14-total)
- [How It Works](#how-it-works)
- [The Context Workflow](#the-context-workflow)
- [Languages Supported](#languages-supported)
- [What Nexus Extracts](#what-nexus-extracts)
- [Architecture](#architecture)
- [CLI Commands](#cli-commands)
- [Context Bundle Format](#context-bundle-format)
- [Configuration](#configuration)
- [Build from Source](#build-from-source)
- [AI Agent Setup](#ai-agent-setup-instructions)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## Quick Start

```bash
cd ~/my-repos
nexus init .
```

Nexus installs itself, indexes your repos with tree-sitter, builds the dependency graph, and
registers as an MCP server. Open a new AI agent session and start using it — your agent now
has structural understanding of every service in your codebase.

## MCP Tools (14 total)

Every tool is deterministic — zero LLM calls, zero API keys needed for code intelligence.
Results in seconds, not minutes.

### Code Intelligence (11 tools)

| Tool | Purpose |
|------|---------|
| `search` | Keyword + graph-boosted search across all indexed code |
| `grep` | Fast regex search across all workspaces |
| `find_endpoints` | List REST/GraphQL endpoints for a service |
| `read_file` | Read a file from any indexed service |
| `list_services` | All indexed services with platform and file counts |
| `service_profile` | Detailed profile: tech stack, endpoints, dependencies, patterns |
| `trace_dependencies` | What does this service depend on? |
| `trace_dependents` | What depends on this service? |
| `impact_analysis` | Blast radius analysis for a proposed change |
| `status` | Index health and crawl stats |
| `review_pr_context` | Gather cross-service context for reviewing a PR |

### Context Tools (3 tools)

| Tool | Purpose |
|------|---------|
| `context` | Jira ticket + code intelligence → structured context bundle |
| `investigate` | Code-only scan for a ticket (raw JSON, no formatting) |
| `reindex` | Trigger crawl + graph rebuild from within a session |

Every tool runs from the CLI too:

```bash
nexus search "OrderService"
nexus context PROJ-1234
nexus status
```

## How It Works

**Zero LLM tokens.** Every step is deterministic code intelligence — tree-sitter parsing,
graph traversal, keyword matching, pattern detection, Jira API calls. Your AI agent applies
its own reasoning to the raw context. This means:

| Property | Benefit |
|----------|---------|
| **No LLM costs** | Nexus never calls an LLM — all intelligence is structural |
| **Reproducible** | Same input → same output, every time |
| **Fast** | Full context bundle in 2–5 seconds |
| **Offline-capable** | Works without internet (except Jira/Confluence fetches) |
| **Agent-agnostic** | Works with any MCP-compatible AI agent |

### Why Zero-LLM?

Your AI agent already has world-class reasoning. What it lacks is **structural context** —
which files exist, how services connect, what endpoints are exposed, where config lives. Nexus
fills that gap with deterministic analysis and lets the LLM do what it's best at: understanding
code, resolving ambiguity, and generating implementations.

### Auto-Reindex

When the MCP server starts (`nexus serve`), it checks when the last crawl happened:

- **If stale** (older than configured interval, default 48h) — reindexes immediately
- **If fresh** — waits for the next scheduled interval

Each reindex:
1. Runs `git pull --ff-only` on every repo (20 concurrent)
2. Compares HEAD SHA — only re-parses repos that changed
3. Rebuilds the dependency graph
4. Hot-swaps the in-memory index (no restart needed)

## The Context Workflow

The primary workflow is `nexus context <TICKET>` — a 3-phase pipeline that produces everything
your AI agent needs to implement a Jira ticket:

```
CLASSIFY → INVESTIGATE → CONTEXT
```

| Phase | What happens |
|-------|-------------|
| **CLASSIFY** | Fetches Jira ticket (summary, ACs, description, comments), classifies ticket type |
| **INVESTIGATE** | Matches services, endpoints, facades, call chains, patterns, related classes |
| **CONTEXT** | Resolves linked issues, detects Figma designs, discovers library APIs, outputs bundle |

The output is a structured markdown document saved to `~/.nexus/handoffs/<TICKET>.md` and
printed to stdout. See [Context Bundle Format](#context-bundle-format) for the full structure.

## Languages Supported

| Language | Parser | Extraction |
|----------|--------|------------|
| **Java** | tree-sitter | Classes, methods, fields, endpoints, Kafka, config refs, DI, stereotypes |
| **TypeScript** | tree-sitter | Classes, functions, exports, imports, endpoints |
| **Python** | tree-sitter | Classes, functions, decorators, imports |
| **Kotlin** | tree-sitter | Classes, functions, annotations, endpoints |
| **Swift** | tree-sitter | Classes, structs, protocols, functions |
| **Go** | tree-sitter | Structs, functions, methods, interfaces |

## What Nexus Extracts

Nexus uses tree-sitter to parse source code into a rich structural model. For each class/file:

### Per-Unit (Class/Interface/Enum)

| Feature | Details |
|---------|---------|
| Identity | Name, FQN, file path, line number |
| Type system | Extends, implements, imports |
| Stereotype | `rest_controller`, `service`, `repository`, `configuration`, `component` |
| Annotations | Class-level annotations (Spring, Jakarta, custom) |
| Dependencies | Injected services via `private final` fields |
| Config refs | `@Value("${...}")` and `@ConfigurationProperties` |
| Kafka | Topics produced (`kafkaTemplate.send`) and consumed (`@KafkaListener`) |
| API calls | Outbound HTTP via `WebClient.uri()` |

### Per-Method

| Feature | Details |
|---------|---------|
| Signature | Name, return type, parameters, line number |
| Annotations | Method-level annotations |
| HTTP endpoint | Method + path (`GET /products`, `POST /orders/{id}`) |
| Method calls | Target + method name for every invocation in the body |

### Per-Field

| Feature | Details |
|---------|---------|
| Identity | Name, type |
| Annotations | Field-level annotations (`@Value`, `@Autowired`, custom) |

### Service-Level Graph

| Edge type | How detected |
|-----------|-------------|
| HTTP calls | Endpoint ↔ `WebClient.uri()` / `RestTemplate` calls |
| Kafka flows | Producer topic ↔ consumer topic |
| Build dependencies | Maven/Gradle dependency declarations |
| Import references | Cross-service type imports |
| Config sharing | Shared configuration property prefixes |



## CLI Commands

| Command | Description |
|---------|-------------|
| `nexus init [path]` | First-time setup — prereqs, config, index, MCP registration |
| `nexus crawl` | Incremental reindex (only changed repos) |
| `nexus crawl --full` | Force full re-parse of all repos |
| `nexus serve` | Start MCP server (stdio — used by AI agents) |
| `nexus context <TICKET>` | Generate context bundle for a Jira ticket |
| `nexus investigate <TICKET>` | Code-only scan (raw JSON) |
| `nexus status` | Show index health |
| `nexus demo` | Launch live dashboard in browser |

## Context Bundle Format

The `context` tool/command outputs a structured markdown document with everything your AI
agent needs to implement a ticket:

```markdown
# PROJ-1234 — FEATURE

## Target
Service: `ecommerce_platform:order.service`
Path: `/Users/you/repos/order.service`
Branch: `feature/PROJ-1234`

## Ticket
**Summary:** Update order pricing from external API
**Acceptance Criteria:** ...
**Description:** ...
**Comments:**
> **Alex Johnson** (2025-03-18T14:30:00):
> PO clarification: only apply to Premium membership type

## Read These Files First
- `src/main/java/.../OrderFacade.java`
- `src/main/java/.../AppConfig.java`

## Existing Endpoints (DO NOT recreate)
- `GET /products` → ProductController

## Existing Facades (reuse these)
- `InventoryFacade` (src/.../InventoryFacade.java)

## Related Classes
- `OrderFacade` [service] — src/.../OrderFacade.java
- `Promotion` — src/.../Promotion.java

## Shared Library APIs
**com.example.platform.commons:commons-core:2.1.0**
- `Tenant` — fields: code, isPremium, isEnterprise...

## Rules
1. Read all files listed above before editing anything
2. Follow detected patterns (reactive, caching, etc.)
3. Reuse existing facades — do NOT create duplicates
4. Write tests for new/modified code
5. Only code commit, no PR
```

## Configuration

Config lives at `~/.nexus/config.yml`. Created automatically by `nexus init`.

```yaml
workspaces:
  - path: ~/my-repos

jira:
  base_url: https://jira.example.com
  token_env: JIRA_PAT

confluence:
  base_url: https://confluence.example.com
  token_env: CONFLUENCE_PAT

figma:
  token_env: FIGMA_PAT

agent:
  auto_push: false
  auto_transition: false
  max_files_changed: 10
  require_tests: true

schedule:
  crawl_interval: 48h

crawl:
  git_pull_concurrency: 20
  parse_concurrency: 8
```

### Multiple Workspaces

```yaml
workspaces:
  - path: ~/repos/team-a
  - path: ~/repos/team-b
  - path: ~/projects/personal
```

Nexus indexes all repos across all workspaces into a single unified graph.

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `JIRA_PAT` | Jira Personal Access Token (configurable via `token_env`) |
| `CONFLUENCE_PAT` | Confluence PAT (optional) |
| `GITHUB_TOKEN` | GitHub token for private repos (optional) |
| `NEXUS_FIGMA_PAT` | Figma PAT for design detection (optional) |

## Build from Source

Requires Go 1.24+.

```bash
git clone https://github.com/realticsAI/nexus.git
cd nexus
go build -o nexus ./cmd/nexus/
```

Install to PATH:

```bash
cp nexus ~/bin/nexus
```

Or one-liner:

```bash
go install github.com/realticsAI/nexus/cmd/nexus@latest
```

Run tests:

```bash
go test ./...
```

## AI Agent Setup Instructions

> **If you are an AI agent and a user has given you the `nexus` binary to set up, follow these
> steps exactly.**

### Step 0: Locate the binary and the user's repos

Ask the user two things if not already provided:
1. Where is the `nexus` binary? (example: `~/Downloads/nexus`)
2. Where are their cloned git repos? (example: `~/Workspaces` or `~/repos`)

### Step 1: Install and initialize

```bash
chmod +x /path/to/nexus
cd /path/to/repos
/path/to/nexus init .
```

**Expected output — all 6 steps should show checkmarks:**
```
=== Nexus Setup ===
[1/6] Installing nexus to ~/bin...              ✓
[2/6] Checking prerequisites...                 ✓
[3/6] Creating config at ~/.nexus/config.yml    ✓
[4/6] Checking environment...                   ✓
[5/6] Indexing codebase...                      ✓
[6/6] Registering MCP server...                 ✓
```

**If step 2 fails:** Run `brew install go git gh awscli` then re-run init.
**If step 5 shows 0 services:** The path has no git repos. Ask user for the correct directory.
**If step 6 fails:** Register manually with your AI agent's MCP configuration.

### Step 2: Set environment variables

Add to `~/.zshrc` (or `~/.bashrc`):

```bash
export JIRA_PAT="<your Jira Personal Access Token>"
export CONFLUENCE_PAT="<your Confluence PAT>"          # Optional
export GITHUB_TOKEN="<your GitHub token>"               # Optional
export NEXUS_FIGMA_PAT="<your Figma PAT>"              # Optional
```

After adding, run `source ~/.zshrc` to apply.

### Step 3: Verify installation

```bash
nexus status
```

**Success — services count must be greater than 0:**
```
services: 76
nodes:    76
edges:    42
keywords: 19193
last crawl: 76 indexed, 0 failed (6.725s)
```

### Step 4: Restart your AI agent

Nexus MCP tools load at session start. Open a **new** agent session, then verify:
"Use nexus to list services."

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "no workspaces configured" | Run `nexus init .` from your repos directory, or add workspaces to `~/.nexus/config.yml` |
| Nexus tools not showing in agent | Check MCP config for `nexus` entry → re-register → start new session |
| Index seems stale | `nexus crawl --full` then `nexus status` to verify |
| "command not found" | Add `export PATH="$HOME/bin:$PATH"` to `~/.zshrc` |

## License

Copyright (c) 2025 realticsAI. All rights reserved.

Licensed under the [GNU Affero General Public License v3.0](LICENSE).

You may use, modify, and distribute this software under the terms of the AGPL-3.0.
If you run a modified version as a network service, you must make the source code
available to its users under the same license.
