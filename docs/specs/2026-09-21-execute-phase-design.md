# EXECUTE Phase — Subagent-Driven Implementation

## Goal

Add an EXECUTE phase to the `nexus-implement` skill that dispatches Sonnet subagents for code implementation. Opus orchestrates, Sonnet codes.

## Principles

1. **Nexus stays zero-LLM.** No new commands. `nexus context` is the only input.
2. **Opus orchestrates, Sonnet executes.** Opus has full context from Phases 1-3. It decides everything.
3. **LLM decides introspectively.** No templates, no topology frameworks. Opus reads its own plan and figures out what to parallelize.
4. **No context loss.** Every subagent gets the full context bundle path + its task. Can Read any file.
5. **No breaking changes.** Nexus's existing commands, tools, and MCP server are untouched.
6. **Disk is shared state.** Subagents write to repo. Next subagents read from disk.

---

## Architecture

```
nexus context PROJ-1234  →  context bundle (zero-LLM, unchanged)
        ↓
nexus-implement skill (Opus)
        ↓
Phase 1: Get context bundle                    (unchanged)
Phase 2: Read all referenced files             (unchanged)
Phase 3: Extract requirements, present plan    (unchanged, waits for user approval)
Phase 4: EXECUTE — fan out Sonnet subagents    ← NEW
Phase 5: Compile-check + commit                ← UPDATED
```

Phase 4 is simple: after the user approves the plan, Opus dispatches `Agent(model: "sonnet")` for each action item. Independent items go parallel (single message, multiple Agent calls). Dependent items go serial. Opus decides which is which — it already knows from reading the code in Phase 2.

---

## What Changes in Nexus (Go codebase)

### Removed files

| File | Reason |
|------|--------|
| `internal/agent/executor.go` | LLM code generation inside Nexus — replaced by subagent dispatch in skill |
| `internal/agent/executor_tools.go` | Multi-turn tool-use code generation — same reason |

Zero external references confirmed. No CLI command, MCP tool, or other code calls `ExecuteParallel`, `GenerateCode`, or `GenerateCodeMultiTurn`.

### Unchanged

Everything else. All 14 MCP tools, `nexus context`, `nexus serve`, `nexus crawl`, pipeline tree, planner, analyzer, validator.

---

## Skill Changes

The skill at `~/.claude/skills/nexus-implement/SKILL.md` gets a rewritten Phase 4 and updated Phase 5.

### Phase 4: EXECUTE

```
After the user approves your plan, dispatch Sonnet subagents to implement
the action items.

- Split your approved plan into 2-4 focused action items (one file or
  tightly-coupled file group each)
- Use Agent(model: "sonnet") for each coding task
- Give each subagent: the context bundle path, its specific task, which
  files to modify, and what other subagents are working on
- Run independent items in parallel (dispatch in a single message)
- Run dependent items in serial (tests after implementation)
- Subagents write directly to disk — subsequent agents read from disk
```

### Phase 5: Validate + Commit

```
After all subagents complete:

1. Run the project's build command to verify compilation
   (detect from repo: pom.xml → mvn, build.gradle → gradle,
    go.mod → go build, tsconfig.json → tsc, etc.)
2. If compilation fails, dispatch one fix-up Agent(sonnet) with
   the errors + context bundle. One retry only.
3. If fix-up also fails, report errors to user and stop.
4. Review git diff — verify only expected files changed.
5. git add <specific files> + git commit
```

---

## Cost Model

| Role | Model | What it does |
|------|-------|-------------|
| Orchestrator | Opus | Planning + dispatch (~5K output tokens) |
| Subagents | Sonnet | Bulk coding (~20K output tokens each) |

Typical 3-subagent ticket: Opus 5K + Sonnet 60K = 65K total tokens.
vs current all-Opus: ~80K tokens, all at Opus pricing, serial execution.

---

## Implementation Scope

### In scope

1. Remove `internal/agent/executor.go` and `internal/agent/executor_tools.go`
2. Update `~/.claude/skills/nexus-implement/SKILL.md` — rewrite Phase 4, update Phase 5
3. Test with a real ticket

### Out of scope

- New Nexus commands or MCP tools
- Changes to `nexus context` output
- Multi-repo subagent dispatch (future)
- Config for subagent model selection (future)
