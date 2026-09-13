---
name: nexus-implement
description: Implement a Jira ticket using Nexus code intelligence as context. Nexus provides the context (files, patterns, endpoints, library APIs); Claude Code reads the actual code, extracts requirements, makes architectural decisions, and generates the implementation.
allowed-tools: Bash, Read, Write, Edit, Grep, Glob, Agent, AskUserQuestion, mcp__nexus__context, mcp__nexus__investigate, mcp__nexus__search, mcp__nexus__read_file
---

# Nexus Implement

Use Nexus code intelligence to understand a Jira ticket, then implement it with Claude Code's own reasoning.

**Key principle:** Nexus is a CONTEXT ENGINE — it provides code intelligence (files, patterns, endpoints, call chains, library APIs) and raw Jira data. Claude Code is the BRAIN — it extracts requirements, makes architectural decisions, and generates minimal, correct code. Nexus does NOT plan or validate; Claude Code does everything from requirements to commit.

## Arguments

$ARGUMENTS — Jira ticket key (e.g. `PROJ-1234`)

## Phase 1: Get Nexus Context

### Step 1: Check for saved context

```bash
TICKET="<ticket-key-from-arguments>"
CONTEXT_FILE="$HOME/.nexus/handoffs/$TICKET.md"
```

If the file exists and is less than 1 hour old, read it. Otherwise proceed to Step 2.

### Step 2: Run Nexus context

Try in order:
1. **MCP tool** (preferred): Call `mcp__nexus__context` with the ticket key
2. **Binary fallback**: Run `~/bin/nexus context <ticket>`
3. **Legacy fallback**: Run `~/bin/nexus handoff <ticket>` (older binary)

If all fail, inform the user and stop.

### Step 3: Parse the context output

Extract these sections from the context markdown:

| Section | What to extract |
|---------|----------------|
| **Target** | Service name, repo path |
| **Ticket** | Raw summary, ACs, description — YOU extract requirements from this |
| **Read These Files First** | File paths to read before any changes |
| **Existing Endpoints** | DO NOT recreate these |
| **Existing Facades** | Reuse these, DO NOT duplicate |
| **Patterns** | Detected code patterns (reactive, caching, etc.) |
| **Call chain** | Dependency flow between classes |
| **Related Classes** | Class names + file paths + stereotypes |
| **Shared Library APIs** | Library class signatures — use these, don't reinvent |
| **Linked Issues** | Related tickets for additional context |

## Phase 2: Deep Code Reading

This is where Claude Code adds value. Read the ACTUAL code to understand types, signatures, and patterns.

### Step 1: Read all files from "Read These Files First"

Read every file listed. For each one, note:
- Actual field types (not just names — e.g. `ItemType` is an object with `code` and `name`, not a String)
- Method signatures and return types
- Annotations and framework patterns
- Constructor patterns (builder, DI, etc.)

### Step 2: Read model/DTO classes referenced in the ticket

For any model class mentioned in the ticket or related classes:
- Read the full class
- Understand field types, especially value objects vs primitives
- Check for `@Value` (immutable) vs `@Data` (mutable) — this determines how to modify instances

### Step 3: Read test files for classes being modified

Understand existing test patterns:
- What testing framework? (JUnit 5 + Mockito, StepVerifier, etc.)
- How are mocks set up?
- What assertion style? (AssertJ, Hamcrest, JUnit assertions)
- What's the test naming convention?

### Step 4: Identify the right architectural layer

**This is critical.** Check:
- Where is the data being modified actually constructed? (Read the builder/constructor call site)
- Is it built in a Facade, Service, or Controller?
- Resolution/transformation logic belongs where the object is CREATED, not where it's consumed
- Prefer the layer closest to the data source

## Phase 3: Requirements + Plan (Claude Code's Own)

Based on actual code reading and raw ticket data, Claude Code does ALL of this:

### Extract requirements
- Read the ticket summary, ACs, and description word by word
- Extract every requirement (explicit and implied)
- Identify edge cases and state transitions
- Flag unknowns — make reasonable assumptions based on code patterns

### Architecture decisions
- Which layer to place the change in
- Whether to create new classes or modify existing ones
- Minimal change set — don't add audit trail fields, resolver classes, or abstractions unless the ticket explicitly requires them

### Present the plan to the user

Before writing any code, show a brief plan:

```
## Implementation Plan for <TICKET>

**Summary:** <1-line from ticket>
**Approach:** <1-line summary of what you'll do>

**Requirements extracted:**
- REQ-1: <requirement> [MUST/SHOULD]
- ...

**Edge cases:**
- EC-1: <trigger> → <expected behavior>
- ...

**Files to modify:**
- <file> — <what changes>

**Files to create:**
- <file> — <why needed> (only if genuinely needed)

**Tests:**
- <test file> — <what tests>
```

Wait for user approval before proceeding to Phase 4.

## Phase 4: Implement

### Step 1: Create branch

```bash
cd <repo-path>
git checkout -b feature/<TICKET> origin/main
```

### Step 2: Make changes

After the user approves the plan, dispatch Sonnet subagents using `Agent(model: "sonnet")` to implement the action items in parallel where possible. Each subagent gets the context bundle path and its specific task. Run dependent items (like tests) after the code they test is written. After all subagents complete, run the project's build command to verify compilation. If it fails, dispatch a fix-up agent.

Follow these rules:
- **Read before edit** — always read the current file state before modifying
- **Follow existing patterns** — match the code style, annotations, naming conventions of surrounding code
- **Minimal changes** — don't refactor surrounding code, don't add features not in the requirements
- **Correct types** — use actual types from the code you read in Phase 2
- **Reactive patterns** — if the codebase uses WebFlux (Mono/Flux), keep changes within the reactive chain using `.map()`, `.flatMap()`, etc. Never `.block()`
- **Immutable models** — for `@Value` classes, use `.toBuilder().field(val).build()` pattern

### Step 3: Write tests

- Place tests in the same test class that covers the modified production code
- Match existing test patterns exactly (same mock setup, same assertion style)
- Test the golden path + edge cases you identified
- For reactive code, use `StepVerifier`

### Step 4: Validate

If Maven/Gradle is available:
```bash
mvn compile -q 2>&1 | tail -20
```

If compilation fails, fix the errors.

## Phase 5: Commit

```bash
git add <specific-files>
git commit -m "<conventional commit message>"
```

**Rules:**
- Only commit code — no PR creation
- Add specific files, not `git add -A`
- Use conventional commit format: `feat:`, `fix:`, `refactor:` etc.
- Don't push unless the user asks

## Known Pitfalls

Watch for these common mistakes when implementing from context:

1. **Layer placement** — Put logic where objects are CONSTRUCTED (usually Facade), not where they're consumed (usually Service)
2. **Type confusion** — Read the actual class before assuming a field is a String. Value objects like `ItemType` have `.getCode()` and `.getName()`
3. **Over-engineering** — Don't create dedicated @Component classes for simple inline logic, or add audit trail fields not in the ticket
4. **Model class confusion** — Verify which model is actually used at the modification point (e.g. `OrderLineItem` vs `Order`)
