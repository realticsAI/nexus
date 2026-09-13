# Nexus

Code intelligence engine for AI coding agents. Indexes multi-language repos (Java, TypeScript, Python) into a searchable dependency graph and serves it via [MCP](https://modelcontextprotocol.io) — giving any MCP-compatible AI agent deep understanding of your entire codebase.

## What it does

- **Indexes** your repos using tree-sitter parsing — classes, methods, endpoints, configs, dependencies
- **Builds a graph** of service-to-service relationships (HTTP calls, Kafka topics, imports, build deps)
- **Serves 16 MCP tools** so your AI agent can search code, trace dependencies, analyze impact, and more
- **Autonomous SDLC** — give it a Jira ticket and it investigates, plans, implements, and ships a PR

---

## AI Agent Setup Instructions

> **If you are an AI agent and a user has given you the `nexus` binary to set up, follow these steps exactly. Do not skip steps. Do not improvise.**

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
[4/6] Checking environment...                   ✓ (warnings about missing env vars are OK)
[5/6] Indexing codebase...                      ✓ (takes 10-60 seconds)
[6/6] Registering MCP server...                  ✓
```

**If step 2 fails:** Run `brew install go git gh awscli` then re-run init.
**If step 5 shows 0 services:** The path has no git repos. Ask user for the correct repos directory.
**If step 6 fails:** Register manually with your AI agent's MCP configuration.

### Step 2: Set environment variables

Add to `~/.zshrc` (or `~/.bashrc`). The user must provide their own tokens — do not generate placeholder values.

```bash
export NEXUS_JIRA_PAT="<user provides their Jira PAT>"        # Required for ticket analysis
export NEXUS_CONFLUENCE_PAT="<user provides their Confluence PAT>"  # Optional
export GITHUB_TOKEN="<user provides their GitHub token>"                # Optional, for PR creation
```

Tell the user: "You need a Jira Personal Access Token. Go to your Jira instance, then Profile, then Personal Access Tokens, then Create Token."

After adding, run `source ~/.zshrc` to apply.

### Step 3: Verify installation

```bash
nexus status
```

**Success — services count must be greater than 0:**
```
services: 10
nodes:    10
edges:    1
keywords: 19193
last crawl: 10 indexed, 0 failed (31.279s)
```

**If `command not found`:** Run `export PATH="$HOME/bin:$PATH"` and add it to `~/.zshrc`.
**If services is 0:** Run `nexus init /correct/repos/path` with the right directory.

### Step 4: Restart your AI agent

Nexus MCP tools load at session start. The user must open a **new** agent session. Then verify with: "Use nexus to list services"

### What Nexus provides after setup

Once registered, every agent session has access to these MCP tools:

| Tool | What it does |
|------|-------------|
| `search` | Keyword search across all indexed code |
| `grep` | Regex search across all repos |
| `find_endpoints` | List REST/GraphQL endpoints for a service |
| `read_file` | Read any file from any indexed service |
| `list_services` | Show all indexed services |
| `service_profile` | Detailed service info (tech stack, endpoints, deps) |
| `trace_dependencies` | What does service X depend on? |
| `trace_dependents` | What depends on service X? |
| `impact_analysis` | Blast radius if service X changes |
| `investigate` | Scan codebase for a Jira ticket (no LLM cost) |
| `plan` | Generate implementation plan for a Jira ticket |
| `validate` | Full pipeline: investigate + analyze + plan + validate |
| `handoff` | Generate a ready-to-use implementation prompt |

### Keeping the index fresh

```bash
nexus crawl          # incremental update (seconds) — run after git pull
nexus crawl --full   # force full re-parse if index seems stale
```

---

## Quick Start (One Command)

```bash
cd ~/my-repos
nexus init .
```

Nexus installs itself, indexes your repos, and registers as an MCP server. Open a new agent session and start using it.

---

## Step-by-Step Setup Guide (Human-Readable)

### Prerequisites

| Tool | Required | Why |
|------|----------|-----|
| **macOS** | Yes (for now) | Homebrew-based auto-install. Linux users install deps manually. |
| **Go 1.24+** | Yes | Nexus is a Go binary. `nexus init` installs it via brew if missing. |
| **Git** | Yes | Repo discovery and sync. |
| **GitHub CLI** (`gh`) | Recommended | PR creation, repo cloning. |
| **AWS CLI** | For Bedrock LLM | Only needed if using AWS Bedrock as LLM provider. |
| **MCP-compatible agent** | Yes | Any AI agent that supports MCP (e.g. Claude Code, Copilot, Cursor). |

> **Don't worry if you're missing tools** — `nexus init` detects and installs them automatically via Homebrew.

### Step 1: Install Nexus

Download the `nexus` binary and make it executable:

```bash
chmod +x nexus
./nexus init .
```

That's it — `nexus init` self-installs to `~/bin/nexus` and adds it to your PATH. No source code, no build tools, no dependencies to install manually.

### Step 2: Clone Your Repos

Clone all the repos you want indexed under a single parent directory:

```bash
mkdir -p ~/my-repos
cd ~/my-repos
gh repo clone my-org/service-a
gh repo clone my-org/service-b
gh repo clone my-org/service-c
```

The directory structure should look like:

```
~/my-repos/
  ├── service-a/
  ├── service-b/
  └── service-c/
```

Nexus will auto-discover all git repos under this directory — nested repos, monorepos, and multi-module projects are all supported.

### Step 3: Run `nexus init`

```bash
cd ~/my-repos
nexus init .
```

Or with an absolute path:

```bash
nexus init ~/my-repos
```

**What `nexus init` does (6 steps):**

```
=== Nexus Setup ===

[1/6] Installing nexus to ~/bin...
      Copies the binary to ~/bin/nexus so it's on your PATH.
      Skipped if already running from ~/bin.

[2/6] Checking prerequisites...
      Detects git, go, gh, aws on your PATH.
      Missing tools are installed via Homebrew automatically.
      Adds ~/bin to PATH in ~/.zshrc if needed.

[3/6] Creating config at ~/.nexus/config.yml
      Saves your workspace path (resolved to absolute).
      Configures defaults for LLM, Jira, Confluence, crawl settings.
      Skipped if config already exists.

[4/6] Checking environment...
      Verifies required env vars (NEXUS_JIRA_PAT).
      Reports optional vars (GITHUB_TOKEN, NEXUS_FIGMA_PAT, etc.).

[5/6] Indexing codebase...
      Walks your workspace, discovers all git repos.
      Pulls latest changes (bounded concurrency).
      Parses Java/TS/Python with tree-sitter.
      Builds keyword index + dependency graph.
      Typically takes 10-60 seconds depending on repo count.

[6/6] Registering MCP server...
      Registers Nexus as a global MCP server for all agent sessions.
```

After this, open a **new** agent session and Nexus tools are available.

### Step 4: Set Up Environment Variables (Optional)

Add these to your `~/.zshrc` for full functionality:

```bash
# Required for Jira integration
export NEXUS_JIRA_PAT="your-jira-personal-access-token"

# Optional — enables additional features
export NEXUS_CONFLUENCE_PAT="your-confluence-pat"
export GITHUB_TOKEN="your-github-token"
export NEXUS_FIGMA_PAT="your-figma-pat"
```

**How to get a Jira PAT:**
1. Go to your Jira instance (e.g. https://jira.example.com)
2. Profile → Personal Access Tokens → Create Token
3. Copy the token and add it to `~/.zshrc`

### Step 5: Verify

```bash
nexus status
```

You should see:

```
services: 10
nodes:    10
edges:    1
keywords: 19193
last crawl: 10 indexed, 0 failed (31.279s)
```

Or from your AI agent, ask:

> "Use nexus to search for UserService"

---

## Configuration

Config lives at `~/.nexus/config.yml`. Created automatically by `nexus init`.

```yaml
# Workspace — directories containing your cloned repos
workspaces:
  - path: ~/my-repos

# GitHub (for PR creation in agent mode)
github:
  token_env: GITHUB_TOKEN

# LLM provider for agent engine (analyze, plan, execute)
llm:
  provider: bedrock              # bedrock | anthropic | ollama
  model: us.anthropic.claude-opus-4-6-v1
  region: us-east-1
  profile: your-bedrock-profile

# Jira integration
jira:
  base_url: https://jira.example.com
  token_env: NEXUS_JIRA_PAT

# Confluence integration
confluence:
  base_url: https://confluence.example.com
  token_env: NEXUS_CONFLUENCE_PAT

# Agent behavior
agent:
  auto_push: false               # never auto-push without approval
  auto_transition: false         # never auto-transition Jira tickets
  max_files_changed: 10          # safety limit per PR
  require_tests: true            # agent must add tests
  branch_prefix: "nexus/"

# Crawl performance tuning
crawl:
  git_pull_concurrency: 20
  parse_concurrency: 8
  embed_concurrency: 2
```

### Adding Multiple Workspaces

```yaml
workspaces:
  - path: ~/repos/team-a
  - path: ~/repos/team-b
  - path: ~/projects/personal
```

Nexus indexes all repos across all workspaces into a single unified graph.

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `nexus init [path]` | First-time setup — prereqs, config, index, MCP registration |
| `nexus crawl` | Discover and parse repos (incremental — only re-parses changed files) |
| `nexus crawl --full` | Force full re-parse, ignoring checksums |
| `nexus analyze` | Build dependency graph from parsed index |
| `nexus serve` | Start MCP server (stdio — used by AI agents) |
| `nexus pipeline` | Run crawl → analyze → serve in sequence |
| `nexus status` | Show index health (service count, last crawl, etc.) |
| `nexus work TICKET-ID` | Autonomous SDLC — Jira ticket to PR |

---

## MCP Tools

Once registered, these tools are available to your AI agent:

### Code Intelligence

| Tool | Description |
|------|-------------|
| `search` | Keyword + graph-boosted search across all indexed code |
| `grep` | Fast regex search across workspaces |
| `find_endpoints` | List REST/GraphQL endpoints for a service |
| `read_file` | Read a file from any indexed service |
| `list_services` | List all indexed services with platform and file counts |
| `service_profile` | Detailed service profile (tech stack, endpoints, dependencies) |
| `trace_dependencies` | What does this service depend on? |
| `trace_dependents` | What depends on this service? |
| `impact_analysis` | Blast radius analysis for a change |
| `status` | Index health and crawl stats |
| `review_pr_context` | Gather context for reviewing a PR |

### Agent Pipeline

| Tool | Description |
|------|-------------|
| `investigate` | Scan codebase for a Jira ticket (no LLM needed) |
| `analyze` | Extract requirements via LLM |
| `plan` | Generate implementation plan |
| `design` | Architecture design with trade-offs |
| `validate` | Full pipeline with validation |
| `handoff` | Generate agent-ready implementation prompt |

---

## Usage Examples

**From your AI agent:**

```
"Use nexus to search for PaymentService"
"Use nexus to find endpoints in order.service"
"Use nexus to trace dependencies of commerce.cart"
"Use nexus to investigate PROJ-1001"
"Use nexus to analyze the impact of changing OfferService"
```

**From terminal:**

```bash
# Re-index after pulling new changes
nexus crawl

# Check what's indexed
nexus status

# Autonomous: investigate a ticket and create a PR
nexus work PROJ-1001
```

---

## Keeping the Index Fresh

Nexus uses **incremental indexing** — it only re-parses files that changed since the last crawl. Run `nexus crawl` periodically or after pulling new changes:

```bash
# Quick incremental update (seconds)
nexus crawl

# Full re-index (if something seems stale)
nexus crawl --full
```

The crawl config in `~/.nexus/config.yml` controls concurrency:

```yaml
crawl:
  git_pull_concurrency: 20   # parallel git pulls
  parse_concurrency: 8       # parallel file parses
  embed_concurrency: 2       # parallel embedding calls
```

---

## Troubleshooting

### "no workspaces configured"

Run `nexus init .` from your repos directory, or add workspaces manually to `~/.nexus/config.yml`.

### "CGo errors" during build

Tree-sitter requires CGo. Ensure:

```bash
CGO_ENABLED=1 make build
```

On macOS, Apple Clang must be installed (`xcode-select --install`).

### "nexus tools not showing in my AI agent"

1. Verify registration: check your agent's MCP configuration for a `nexus` entry
2. Re-register Nexus as an MCP server in your agent's settings
3. **Start a new agent session** — MCP servers load at session start

### "AWS SSO expired" (Bedrock users)

SSO tokens expire every 2-3 hours. Re-authenticate:

```bash
aws sso login --profile your-bedrock-profile
```

### Index seems stale

```bash
nexus crawl --full    # force full re-parse
nexus status          # verify counts
```

---

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  Git Repos   │────▶│   Crawler    │────▶│   Store     │
│ (workspace)  │     │ (tree-sitter)│     │ (sharded    │
└─────────────┘     └──────────────┘     │  JSON)      │
                                          └──────┬──────┘
                                                 │
                    ┌──────────────┐              │
                    │  Analyzer    │◀─────────────┘
                    │ (5 edge      │
                    │  resolvers)  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐     ┌─────────────┐
                    │    Graph     │────▶│  MCP Server  │
                    │ (BFS/DFS,   │     │ (stdio,      │
                    │  impact)    │     │  16 tools)   │
                    └─────────────┘     └──────┬───────┘
                                               │
                                        ┌──────▼───────┐
                                        │  AI Agent    │
                                        │  (MCP client)│
                                        └──────────────┘
```

**Stages:**
1. **Crawl** — discover repos, git pull, parse with tree-sitter, build keyword index
2. **Analyze** — resolve edges (HTTP, Kafka, imports, build deps), build dependency graph
3. **Serve** — expose everything via MCP tools over stdio

---

## License

AGPL-3.0 — see [LICENSE](LICENSE).
