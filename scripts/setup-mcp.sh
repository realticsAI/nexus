#!/bin/bash
set -e

NEXUS_HOME="$HOME/.nexus"
NEXUS_BIN="$HOME/bin/nexus"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "=== Nexus MCP Setup ==="
echo ""

# Step 1: Build binary
echo "[1/5] Building nexus binary..."
cd "$PROJECT_DIR"
go build -o nexus ./cmd/nexus
mkdir -p "$HOME/bin"
cp nexus "$NEXUS_BIN"
chmod +x "$NEXUS_BIN"
echo "  -> Installed to $NEXUS_BIN"

# Step 2: Create config if missing
if [ ! -f "$NEXUS_HOME/config.yml" ]; then
    echo "[2/5] Creating default config at $NEXUS_HOME/config.yml..."
    mkdir -p "$NEXUS_HOME"
    cat > "$NEXUS_HOME/config.yml" << 'EOF'
workspaces:
  - path: ~/Documents/Rcg_Digital

github:
  token_env: GITHUB_TOKEN

llm:
  provider: bedrock
  model: us.anthropic.claude-opus-4-6-v1
  region: us-east-1
  profile: AWS-DIGITAL-BEDROCK-695957543545
  max_retries: 3

jira:
  base_url: https://jira.rccl.com
  token_env: HALO_ATLASSIAN_JIRA_PAT

confluence:
  base_url: https://confluence.rccl.com
  token_env: HALO_ATLASSIAN_CONFLUENCE_PAT

figma:
  token_env: NEXUS_FIGMA_PAT

agent:
  auto_push: false
  auto_transition: false
  max_files_changed: 10
  require_tests: true
  branch_prefix: "nexus/"
  github_org: "Rcg_Digital"

crawl:
  git_pull_concurrency: 20
  parse_concurrency: 8
  embed_concurrency: 2
EOF
    echo "  -> Config created. Edit $NEXUS_HOME/config.yml to set your workspace path."
else
    echo "[2/5] Config exists at $NEXUS_HOME/config.yml — skipping"
fi

# Step 3: Check required env vars
echo "[3/5] Checking environment..."
MISSING=""
[ -z "$HALO_ATLASSIAN_JIRA_PAT" ] && MISSING="$MISSING HALO_ATLASSIAN_JIRA_PAT"
if [ -n "$MISSING" ]; then
    echo "  !! Missing env vars:$MISSING"
    echo "  Set these in your ~/.zshrc or ~/.bashrc"
else
    echo "  -> All required env vars set"
fi

# Step 4: Index codebase
echo "[4/5] Indexing codebase..."
"$NEXUS_BIN" crawl 2>&1 | tail -5
echo "  -> Index complete"

# Step 5: Register with Claude Code
echo "[5/6] Registering Nexus MCP with Claude Code..."
if command -v claude &>/dev/null; then
    claude mcp add nexus --scope user -- "$NEXUS_BIN" serve 2>/dev/null && \
        echo "  -> Registered globally" || \
        echo "  -> Already registered (or failed — run manually: claude mcp add nexus --scope user -- $NEXUS_BIN serve)"
else
    echo "  !! Claude Code CLI not found. Register manually:"
    echo "     claude mcp add nexus --scope user -- $NEXUS_BIN serve"
fi

# Step 6: Install Claude Code skills
SKILLS_SRC="$PROJECT_DIR/skills"
SKILLS_DST="$HOME/.claude/skills"
echo "[6/6] Installing Claude Code skills..."
if [ -d "$SKILLS_SRC" ]; then
    mkdir -p "$SKILLS_DST"
    for skill_dir in "$SKILLS_SRC"/*/; do
        skill_name=$(basename "$skill_dir")
        if [ -f "$skill_dir/SKILL.md" ]; then
            mkdir -p "$SKILLS_DST/$skill_name"
            cp -f "$skill_dir/SKILL.md" "$SKILLS_DST/$skill_name/SKILL.md"
            echo "  -> Installed skill: $skill_name"
        fi
    done
else
    echo "  !! No skills directory found in $SKILLS_SRC"
fi

# Create handoffs directory
mkdir -p "$HOME/.nexus/handoffs"

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Nexus is a context engine (zero LLM). Claude Code does the thinking."
echo ""
echo "Nexus MCP provides 14 tools:"
echo ""
echo "  Code Intelligence:"
echo "    search, grep, find_endpoints, read_file, list_services,"
echo "    service_profile, trace_dependencies, trace_dependents,"
echo "    impact_analysis, status, review_pr_context"
echo ""
echo "  Context:"
echo "    context      — Jira + code intel → markdown for Claude Code"
echo "    investigate   — Jira + code intel → raw JSON"
echo "    reindex      — re-crawl and update the codebase index"
echo ""
echo "  Skills (invoke with /skill-name in Claude Code):"
echo "    /rcg-nexus-implement <ticket>  — full ticket implementation"
echo ""
echo "Usage:"
echo "  nexus context LOY-17610               — generate context bundle"
echo "  /rcg-nexus-implement LOY-17610        — full implementation (in Claude Code)"
echo ""
