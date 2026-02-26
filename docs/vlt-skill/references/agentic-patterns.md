# Agentic Patterns for vlt

Proven patterns for AI coding agents using vlt as a persistent knowledge layer.
These patterns are extracted from real-world multi-session workflows.

## The Knowledge Flywheel

The core principle: every coding session should leave the vault richer than it found it.

```
Work produces experience
  --> Vault captures knowledge
    --> Knowledge informs future work
      --> Better work produces richer experience
```

vlt enables this flywheel by providing fast, scriptable vault access without GUI dependencies.

---

## Pattern 1: Session Lifecycle

Every agent session follows three phases: load, work, persist.

### Phase 1: Load Context (Session Start)

Before writing any code, consult the vault for prior decisions, patterns, and known issues.

```bash
# Check for project-specific context
vlt vault="Claude" search query="<project-name>"

# Load architectural decisions
vlt vault="Claude" search query="[type:decision] [project:<name>] [status:active]"

# Load known patterns for the current stack
vlt vault="Claude" search query="[type:pattern] [stack:go]"

# Check for known sharp edges in this domain
vlt vault="Claude" search query="[type:debug] [project:<name>]"

# Read the project index note + everything it links to (one call)
vlt vault="Claude" read file="projects/<project-name>" follow

# Read a decision and see what references it
vlt vault="Claude" read file="<Decision Note>" backlinks
```

### Phase 2: Capture During Work

Capture knowledge the moment it emerges, not at the end. Three trigger conditions:

1. **After making a decision** (chose X over Y): create a decision note
2. **After solving a non-obvious bug**: create a debug note
3. **After discovering a reusable pattern**: create a pattern note

```bash
# Decision capture
vlt vault="Claude" create name="<Decision Title>" \
  path="decisions/<Decision Title>.md" \
  content="---
type: decision
project: <project>
status: active
confidence: high
created: $(date +%Y-%m-%d)
---
# <Decision Title>
## Context
<Why this decision was needed>
## Decision
<What was chosen>
## Alternatives Considered
<What was rejected and why>
## Consequences
<What this decision implies>" silent timestamps

# Debug capture
vlt vault="Claude" create name="<Bug Title>" \
  path="debug/<Bug Title>.md" \
  content="---
type: debug
project: <project>
status: active
created: $(date +%Y-%m-%d)
---
# <Bug Title>
## Symptoms
<What was observed>
## Root Cause
<What actually went wrong>
## Fix
<How it was resolved>
## Prevention
<How to avoid this in the future>" silent timestamps
```

### Phase 3: Persist at Session End

Before ending, update the project index and sync.

```bash
# Update project index with session summary
vlt vault="Claude" append file="projects/<project>" \
  content="## Session $(date +%Y-%m-%d)
- <What was accomplished>
- <What decisions were made>
- <Links to new notes: [[Decision Title]], [[Debug Note]]>"

# Verify nothing was missed
vlt vault="Claude" search query="[project:<name>] [status:active]" --json
```

---

## Pattern 2: Decision Records

Decisions are the highest-value knowledge to capture. They prevent re-litigation
of past choices and provide rationale for future maintainers.

### When to Capture

- Technology or library choice
- Architectural pattern selection
- API design decision
- Trade-off resolution
- Convention establishment

### Frontmatter Schema

```yaml
type: decision
project: <project-name>
stack: [<relevant-technologies>]
domain: <business-domain>
status: active | superseded | archived
confidence: high | medium | low
created: YYYY-MM-DD
superseded_by: "[[New Decision]]"  # When status is superseded
```

### Querying Decisions

```bash
# All active decisions for a project
vlt vault="Claude" search query="[type:decision] [project:my-app] [status:active]"

# Decisions about a specific technology
vlt vault="Claude" search query="[type:decision] [stack:postgresql]"

# Low-confidence decisions (candidates for revisiting)
vlt vault="Claude" search query="[type:decision] [confidence:low]"
```

### Superseding a Decision

When a decision is replaced:

```bash
# Mark old decision as superseded
vlt vault="Claude" property:set file="Old Decision" name="status" value="superseded"
vlt vault="Claude" property:set file="Old Decision" name="superseded_by" value="[[New Decision]]"

# Create new decision with reference to old
vlt vault="Claude" create name="New Decision" path="decisions/New Decision.md" \
  content="---
type: decision
project: my-app
status: active
created: $(date +%Y-%m-%d)
---
# New Decision
## Context
Supersedes [[Old Decision]] because <reason>.
..." silent timestamps
```

---

## Pattern 3: Multi-Session Continuity

For work spanning multiple sessions where context will be lost to compaction.

### Session Handoff Note

Create a handoff note at the end of each session:

```bash
vlt vault="Claude" create name="<Project> Handoff $(date +%Y-%m-%d)" \
  path="_inbox/<Project> Handoff $(date +%Y-%m-%d).md" \
  content="---
type: handoff
project: <project>
status: active
created: $(date +%Y-%m-%d)
---
# Handoff: <Project>
## Current State
<Where things stand>
## In Progress
<What was being worked on>
## Blockers
<What is stuck and why>
## Next Steps
<What to do next, in priority order>
## Key Files
<Important files and their roles>" silent timestamps
```

### Resuming Work

At the start of a new session:

```bash
# Find the most recent handoff
vlt vault="Claude" search query="[type:handoff] [project:<name>]"

# Read it
vlt vault="Claude" read file="<Project> Handoff 2026-02-19"

# Mark it as consumed
vlt vault="Claude" property:set file="<Project> Handoff 2026-02-19" name="status" value="archived"
```

---

## Pattern 4: Convention Enforcement

Store coding conventions in the vault and consult them during code generation.

```bash
# Check conventions before starting work
vlt vault="Claude" search query="[type:convention] [project:<name>]"
vlt vault="Claude" search query="[type:convention] [stack:go]"

# Read specific convention
vlt vault="Claude" read file="conventions/Testing Requirements"
```

### Convention Note Structure

```yaml
type: convention
project: <project-name>  # or "global" for cross-project
stack: [<technologies>]
status: active
created: YYYY-MM-DD
```

---

## Pattern 5: Project Index Notes

Every project should have an index note in `projects/` that serves as the entry point.

```bash
vlt vault="Claude" read file="projects/<project-name>"
```

### Index Structure

```markdown
---
type: project
status: active
stack: [go, cli]
created: 2026-01-15
---
# Project Name

## Overview
Brief description of what this project does.

## Architecture
Key architectural notes or links to [[Decision Notes]].

## Active Decisions
- [[Decision 1]]
- [[Decision 2]]

## Known Issues
- [[Debug Note 1]]

## Session Log
## Session 2026-02-19
- Implemented feature X
- Decided [[Use WebSockets over SSE]]

## Session 2026-02-18
- Set up project scaffolding
- Established [[Testing Conventions]]
```

---

## Pattern 6: Composable Pipelines

Combine vlt with Unix tools for powerful queries.

```bash
# Count notes by type
vlt vault="V" search query="[type:decision]" | wc -l

# Find decisions mentioning a specific term
vlt vault="V" search query="[type:decision]" | while read f; do
  vlt vault="V" read file="$f" | grep -l "PostgreSQL" && echo "$f"
done

# Export all active decisions as JSON
vlt vault="V" search query="[type:decision] [status:active]" --json

# Find notes modified today (via filesystem)
vlt vault="V" files --json | jq -r '.[]'

# Batch property update
vlt vault="V" search query="[status:draft]" | while read f; do
  vlt vault="V" property:set file="$f" name="status" value="active"
done
```

---

## Pattern 7: Vault as CI/CD Integration

Use vlt in CI pipelines for vault quality gates.

```bash
# Fail CI if there are unresolved links
unresolved=$(vlt vault="V" unresolved | wc -l)
if [ "$unresolved" -gt 0 ]; then
  echo "ERROR: $unresolved unresolved links found"
  vlt vault="V" unresolved
  exit 1
fi

# Fail CI if orphan notes exceed threshold
orphans=$(vlt vault="V" orphans | wc -l)
if [ "$orphans" -gt 10 ]; then
  echo "WARNING: $orphans orphan notes found"
fi

# Validate all notes have required frontmatter
vlt vault="V" files ext=".md" | while read f; do
  props=$(vlt vault="V" properties file="$f" 2>/dev/null)
  if ! echo "$props" | grep -q "type:"; then
    echo "MISSING type property: $f"
  fi
done
```

---

## Anti-Patterns to Avoid

### 1. Capturing Too Late

Waiting until session end to capture knowledge risks losing details.
Capture the moment a decision is made or a bug is solved.

### 2. Over-Capturing

Not every observation warrants a note. Capture when:
- The knowledge would save >5 minutes if rediscovered
- The decision could be questioned later
- The bug was non-obvious

### 3. Flat Vault Structure

Dumping everything in `_inbox/` defeats discoverability.
Triage notes into proper folders (`decisions/`, `patterns/`, `debug/`).

### 4. Missing Cross-Links

Notes without `[[wikilinks]]` to related notes are isolated islands.
Always link decisions to projects, patterns to decisions, debug notes to root causes.

### 5. Stale Notes

Notes with `status: active` that are no longer relevant create noise.
Periodically audit and archive superseded notes:

```bash
# Find old active notes
vlt vault="V" search query="[status:active]" | while read f; do
  created=$(vlt vault="V" properties file="$f" | grep "created:" | cut -d' ' -f2)
  echo "$created $f"
done | sort | head -20
```
