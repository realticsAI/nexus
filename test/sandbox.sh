#!/bin/bash
set -e

PASS=0
FAIL=0
WARN=0

pass() { echo "  ✓ $1"; PASS=$((PASS+1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL+1)); }
warn() { echo "  ⚠ $1"; WARN=$((WARN+1)); }
section() { echo ""; echo "[$1] $2"; }

echo "╔══════════════════════════════════════╗"
echo "║     Nexus Full Pipeline Sandbox      ║"
echo "╚══════════════════════════════════════╝"
echo "OS: $(uname -s) $(uname -m)"
echo "User: $(whoami)"
echo ""

# ═══════════════════════════════════
# PHASE 1: Binary & CLI
# ═══════════════════════════════════
section "1/10" "Binary execution"
if nexus --version 2>&1 | grep -q "nexus"; then
    pass "binary runs: $(nexus --version 2>&1)"
else
    fail "binary does not execute"
fi

section "2/10" "CLI subcommands"
CMDS=$(nexus --help 2>&1)
for cmd in work td plan handoff investigate init crawl serve status; do
    if echo "$CMDS" | grep -q "$cmd"; then
        pass "subcommand: $cmd"
    else
        fail "missing subcommand: $cmd"
    fi
done

# ═══════════════════════════════════
# PHASE 2: Init (fresh system)
# ═══════════════════════════════════
section "3/10" "Init on fresh system"
rm -rf ~/.nexus
OUTPUT=$(nexus init --skip-crawl 2>&1 || true)

if [ -f ~/.nexus/config.yml ]; then
    pass "config.yml created"
else
    fail "config.yml not created"
fi

if echo "$OUTPUT" | grep -qi "prerequisite\|checking\|Go\|GitHub CLI\|AWS CLI"; then
    pass "prerequisite check ran"
else
    fail "prerequisite check missing from output"
fi

if echo "$OUTPUT" | grep -qi "environment\|HALO_ATLASSIAN"; then
    pass "env var check ran"
else
    fail "env var check missing"
fi

# ═══════════════════════════════════
# PHASE 3: Start mock server
# ═══════════════════════════════════
section "4/10" "Mock server startup"
/usr/local/bin/mockserver &
MOCK_PID=$!
sleep 1

if curl -s http://localhost:9999/health | grep -q "ok"; then
    pass "mock server running (Jira + LLM on :9999)"
else
    fail "mock server not responding"
    echo "Cannot continue without mock server"
    exit 1
fi

# ═══════════════════════════════════
# PHASE 4: Configure Nexus for mocks
# ═══════════════════════════════════
section "5/10" "Configure for mock services"
cat > ~/.nexus/config.yml << 'YAML'
workspaces:
  - path: /tmp/repos

github:
  token_env: GITHUB_TOKEN

llm:
  provider: anthropic
  model: mock-claude
  api_key_env: MOCK_API_KEY
  base_url: http://localhost:9999
  max_retries: 1

jira:
  base_url: http://localhost:9999
  token_env: MOCK_JIRA_TOKEN

confluence:
  base_url: http://localhost:9999
  token_env: MOCK_CONF_TOKEN

agent:
  auto_push: false
  auto_transition: false
  max_files_changed: 10
  require_tests: true
  branch_prefix: "nexus/"
  github_org: "TestOrg"
YAML

export MOCK_API_KEY="test-key-12345"
export MOCK_JIRA_TOKEN="test-jira-token"

pass "config points to mock server"

# ═══════════════════════════════════
# PHASE 5: LLM pre-flight validation
# ═══════════════════════════════════
section "6/10" "LLM pre-flight (Ping)"
LLM_OUT=$(nexus plan LOY-12345 2>&1 || true)
if echo "$LLM_OUT" | grep -qi "pre-flight failed\|cannot reach\|not configured"; then
    fail "LLM ping failed against mock server: $(echo "$LLM_OUT" | head -3)"
else
    pass "LLM pre-flight passed"
fi

# ═══════════════════════════════════
# PHASE 6: Investigate (no LLM)
# ═══════════════════════════════════
section "7/10" "Investigate (code scan, no LLM)"
INV_OUT=$(nexus investigate LOY-12345 2>&1)
INV_EXIT=$?

if [ $INV_EXIT -eq 0 ]; then
    pass "investigate completed"
else
    # investigate may fail due to no service indexes, but should fetch ticket
    if echo "$INV_OUT" | grep -qi "LOY-12345\|Casino"; then
        pass "investigate fetched ticket from mock Jira"
    else
        fail "investigate failed: $(echo "$INV_OUT" | head -3)"
    fi
fi

if echo "$INV_OUT" | grep -qi "services\|endpoints\|ticket_platform"; then
    pass "investigate returned structured context"
else
    warn "investigate output may be minimal (no indexes)"
fi

# ═══════════════════════════════════
# PHASE 7: Full pipeline — plan
# ═══════════════════════════════════
section "8/10" "Plan pipeline (investigate → analyze → plan)"
PLAN_OUT=$(nexus plan LOY-12345 2>&1)
PLAN_EXIT=$?

if [ $PLAN_EXIT -eq 0 ]; then
    pass "plan pipeline completed"
else
    fail "plan pipeline failed (exit $PLAN_EXIT): $(echo "$PLAN_OUT" | head -5)"
fi

if echo "$PLAN_OUT" | grep -qi "REQ-1\|requirements\|requirement"; then
    pass "analysis extracted requirements"
else
    warn "requirements not visible in plan output"
fi

if echo "$PLAN_OUT" | grep -qi "steps\|summary\|implementation"; then
    pass "plan generated with steps"
else
    fail "plan output missing steps"
fi

if echo "$PLAN_OUT" | grep -qi "validation\|verdict\|WARN\|PASS"; then
    pass "validation ran and returned verdict"
else
    warn "validation verdict not in plan output"
fi

if echo "$PLAN_OUT" | grep -qi "tokens\|token"; then
    pass "token usage tracked"
else
    warn "token tracking not visible"
fi

# ═══════════════════════════════════
# PHASE 8: Handoff (Claude-ready prompt)
# ═══════════════════════════════════
section "9/10" "Handoff (Claude-ready prompt)"
HAND_OUT=$(nexus handoff LOY-12345 2>&1)
HAND_EXIT=$?

if [ $HAND_EXIT -eq 0 ]; then
    pass "handoff completed"
else
    fail "handoff failed (exit $HAND_EXIT): $(echo "$HAND_OUT" | head -5)"
fi

if echo "$HAND_OUT" | grep -qi "implement\|plan\|requirement\|step"; then
    pass "handoff contains implementation instructions"
else
    fail "handoff output not structured"
fi

HAND_LEN=${#HAND_OUT}
if [ $HAND_LEN -gt 200 ]; then
    pass "handoff is substantial ($HAND_LEN chars)"
else
    warn "handoff seems short ($HAND_LEN chars)"
fi

# ═══════════════════════════════════
# PHASE 9: Bug ticket (different type)
# ═══════════════════════════════════
section "10/10" "Bug ticket pipeline"
BUG_OUT=$(nexus plan LOY-99999 2>&1)
BUG_EXIT=$?

if [ $BUG_EXIT -eq 0 ]; then
    pass "bug ticket pipeline completed"
else
    fail "bug ticket failed (exit $BUG_EXIT): $(echo "$BUG_OUT" | head -3)"
fi

if echo "$BUG_OUT" | grep -qi "BUG\|bug"; then
    pass "ticket classified as BUG"
else
    warn "bug classification not visible"
fi

# ═══════════════════════════════════
# PHASE 10: Error handling
# ═══════════════════════════════════
echo ""
echo "[Bonus] Error handling"

# Bad ticket
BAD_OUT=$(nexus investigate INVALID 2>&1 || true)
if [ $? -ne 0 ] || echo "$BAD_OUT" | grep -qi "error\|fail\|invalid"; then
    pass "graceful error on bad ticket key"
else
    warn "no error for bad ticket key"
fi

# Kill mock server, test failure path
kill $MOCK_PID 2>/dev/null || true
sleep 1
DEAD_OUT=$(nexus plan LOY-12345 2>&1 || true)
if echo "$DEAD_OUT" | grep -qi "fail\|error\|cannot\|refused\|pre-flight"; then
    pass "graceful failure when services are down"
else
    fail "no error when services are unreachable"
fi

# ═══════════════════════════════════
# Summary
# ═══════════════════════════════════
echo ""
echo "╔══════════════════════════════════════╗"
echo "║            Test Results              ║"
echo "╠══════════════════════════════════════╣"
printf "║  Passed:   %-3d                       ║\n" $PASS
printf "║  Failed:   %-3d                       ║\n" $FAIL
printf "║  Warnings: %-3d                       ║\n" $WARN
echo "╠══════════════════════════════════════╣"

if [ $FAIL -gt 0 ]; then
    echo "║  RESULT: FAIL                        ║"
    echo "╚══════════════════════════════════════╝"
    exit 1
else
    echo "║  RESULT: ALL PASS                    ║"
    echo "╚══════════════════════════════════════╝"
    exit 0
fi
