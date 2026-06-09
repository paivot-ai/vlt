# Vault Architecture Guide

Design principles, folder structures, frontmatter conventions, and linking strategies
for building effective Obsidian vaults operated by AI coding agents via vlt.

## Design Principles

### 1. Atomic Notes

Each note captures exactly one idea, decision, pattern, or insight.
A note about "PostgreSQL performance" should not also cover "Redis caching strategy."
Atomic notes are easier to link, easier to find, and easier to keep current.

### 2. Structured Frontmatter

YAML frontmatter is the note's metadata API. Property-based search (`[key:value]`)
depends on consistent frontmatter. Every note must have:

- `type` -- Categorizes the note for querying
- `status` -- Lifecycle state for filtering active vs. archived content
- `created` -- Timestamp for temporal queries

### 3. Rich Interlinking

Notes gain value through connections. Isolated notes are invisible to backlink
navigation and orphan detection. Link generously using `[[wikilinks]]`.

### 4. Convention over Configuration

Establish folder and naming conventions early. Consistent structure enables
pattern-based queries and reduces cognitive overhead for both humans and agents.

---

## Recommended Folder Structure

```
vault/
  _inbox/           # Unsorted capture, triage into proper folders
  _templates/       # Note templates for templates:apply
  methodology/      # Process and methodology notes
  conventions/      # Coding standards, team agreements, style guides
  decisions/        # Architectural Decision Records (ADRs)
  patterns/         # Reusable solutions and idioms
  debug/            # Bug investigations with root cause analysis
  concepts/         # Language, framework, and tool knowledge
  projects/         # One index note per project (entry points)
  people/           # Team member preferences and working styles
```

### Folder Semantics

| Folder | What Goes Here | Frontmatter Type |
|--------|---------------|------------------|
| `_inbox/` | Anything captured quickly, before triage | varies |
| `_templates/` | Template files for `templates:apply` | n/a |
| `methodology/` | Process documentation, workflow guides | `methodology` |
| `conventions/` | Coding standards, testing policies | `convention` |
| `decisions/` | Why X was chosen over Y | `decision` |
| `patterns/` | Reusable solutions that worked | `pattern` |
| `debug/` | Bug postmortems with root cause | `debug` |
| `concepts/` | Technology and domain knowledge | `concept` |
| `projects/` | Project index notes | `project` |
| `people/` | Team conventions and preferences | `person` |

---

## Frontmatter Schema

### Required Properties (All Notes)

```yaml
type: <type>          # One of the types listed above
status: active        # active | superseded | archived
created: YYYY-MM-DD   # Creation date
```

### Common Optional Properties

```yaml
project: <name>         # Project this note belongs to
stack: [go, react]      # Technologies involved
domain: <area>          # Business domain
confidence: high        # high | medium | low (for decisions)
superseded_by: "[[X]]"  # Link to replacement (when superseded)
tags: [tag1, tag2]      # Additional categorization
aliases: [alt-name]     # Alternative names for note resolution
```

### Type-Specific Properties

**Decisions:**
```yaml
type: decision
confidence: high | medium | low
superseded_by: "[[New Decision]]"  # When status: superseded
```

**Debug Notes:**
```yaml
type: debug
severity: critical | major | minor
resolved: true | false
```

**Patterns:**
```yaml
type: pattern
applicability: [when, to, apply]
```

**Projects:**
```yaml
type: project
repo: https://github.com/org/repo
stack: [technologies]
```

---

## Linking Strategies

### Contextual Wikilinks

Embed links where they add context, not in isolation:

```markdown
# Good -- Link adds context
Chose WebSockets for the transport layer ([[Use WebSockets over SSE]]).

# Bad -- Link without context
Related: [[Use WebSockets over SSE]]
```

### Cross-Type Linking

Build a web of connections across note types:

- **Decision -> Project**: Every decision should link to its project index
- **Debug -> Decision**: Link root causes to the decisions that led to them
- **Pattern -> Decision**: Link patterns to the decisions that established them
- **Convention -> Project**: Link conventions to the projects that follow them

### Backlink Discovery

Use `backlinks` to discover implicit relationships:

```bash
# What links to this decision? (implementations, related debug notes)
vlt vault="V" backlinks file="Use WebSockets over SSE"

# What links to this project? (all related knowledge)
vlt vault="V" backlinks file="projects/my-app"
```

---

## Triage Workflow

Notes start in `_inbox/` and must be triaged:

### Step 1: Classify

Determine the note type based on content:
- Contains "chose X over Y" or "decided to" -> `decision`
- Contains "symptoms / root cause / fix" -> `debug`
- Contains "when to use" or "recipe for" -> `pattern`
- Contains "always do X" or "never do Y" -> `convention`

### Step 2: Add Frontmatter

Ensure required properties exist:

```bash
vlt vault="V" property:set file="Inbox Note" name="type" value="decision"
vlt vault="V" property:set file="Inbox Note" name="status" value="active"
vlt vault="V" property:set file="Inbox Note" name="project" value="my-app"
```

### Step 3: Add Links

Add `[[wikilinks]]` to related notes:

```bash
vlt vault="V" append file="Inbox Note" content="
## Related
- [[projects/my-app]]
- [[Previous Decision]]"
```

### Step 4: Move to Folder

```bash
vlt vault="V" move path="_inbox/Inbox Note.md" to="decisions/Inbox Note.md"
```

The `move` command updates all links across the vault automatically, including path-form `[[folder/Note]]` links on folder-only moves. It refuses to overwrite an existing destination unless `force` is passed.

---

## Naming Conventions

### Note Titles

Use descriptive, natural-language titles that work as wikilink text:

| Good | Bad |
|------|-----|
| Use WebSockets over SSE | ws-vs-sse |
| PostgreSQL JSONB index gotcha | pg-jsonb-bug |
| Testing Requirements | test-reqs |
| Session Operating Mode | SOM |

### Why Natural Language

- `[[Use WebSockets over SSE]]` reads naturally in prose
- Easier to discover via text search
- Self-documenting when seen in backlinks output
- Alias resolution handles alternative references

---

## Vault Hygiene

### Regular Maintenance

Run these periodically to keep the vault healthy:

```bash
# Find orphan notes (no incoming links)
vlt vault="V" orphans

# Find broken links
vlt vault="V" unresolved

# Audit tag consistency
vlt vault="V" tags counts

# Find notes still in inbox
vlt vault="V" files folder="_inbox"
```

### Archival

When notes are no longer relevant, archive rather than delete:

```bash
vlt vault="V" property:set file="Old Note" name="status" value="archived"
```

Archived notes remain searchable but are excluded from `[status:active]` queries.
Reserve deletion for truly incorrect or duplicate content.

### Supersession

When a decision is replaced by a newer one:

```bash
# Mark old as superseded with forward reference
vlt vault="V" property:set file="Old Decision" name="status" value="superseded"
vlt vault="V" property:set file="Old Decision" name="superseded_by" value="[[New Decision]]"
```

This preserves history while directing readers to current guidance.

---

## Integrity Registry

vlt maintains a SHA-256 content-hash registry outside the vault directory to detect
modifications not made through vlt (e.g., manual edits in Obsidian, `git pull`, text editors).

### Storage Layout

```
~/.vlt/
  registries/
    <vault-id>/           # SHA-256(vault-abs-path)[:16]
      registry.json       # {rel-path: {hash, ts}} mapping
```

- Registry is stored outside the vault to avoid polluting notes
- Directory permissions are 0700; file permissions are 0600
- Writes are atomic (write temp + rename) to prevent corruption
- One registry per vault, identified by a stable hash of the vault's absolute path

### How It Works

1. Every write operation (Create, Append, Prepend, Write, Patch, Move, Delete, PropertySet, PropertyRemove, Daily, TemplatesApply) registers the content hash after a successful write
2. Read operations (Read, ReadFollow, ReadWithBacklinks) verify the hash and return an IntegrityStatus
3. Mismatches produce a stderr warning but do not block the read
4. `integrity:baseline` registers all existing files at once
5. `integrity:acknowledge` re-registers specific files or files modified within a time window

### Integration with Agents

For agentic workflows, integrity tracking answers: "Was this note modified by something other than me?" This is useful for:
- Detecting when Obsidian or a human has edited a note the agent is managing
- Identifying vault changes after `git pull` or sync operations
- Auditing which notes were modified outside automated workflows

## Path Traversal Protection

All user-supplied paths are validated by `safePath()` before any filesystem operation:
- Rejects absolute paths
- Rejects `..` components
- Verifies the resolved path is within the vault boundary
- Applies to: Create, Move, Delete, Search, Files, Tasks, Daily, TemplatesApply

This is critical for agentic workflows where file paths may originate from untrusted or
LLM-generated input.

## Scaling Considerations

### Small Vaults (< 500 Notes)

No special considerations. All commands complete instantly.
Flat folder structures work fine at this scale.

### Medium Vaults (500-5,000 Notes)

- Use `folder=` and `[key:value]` filters to narrow searches
- Keep `_inbox/` small through regular triage
- Consider sub-folders within type folders (e.g., `decisions/2026/`)

### Large Vaults (5,000+ Notes)

- Property-filtered searches are faster than full-text searches
- Scope `tasks` and `files` to specific folders
- Use `--json` output and pipe through `jq` for complex queries
- Consider archiving old notes to a separate vault

---

## Template Design

### Effective Templates

Templates reduce friction for common note types. Store them in `_templates/`
or the folder configured in `.obsidian/templates.json`.

```markdown
---
type: decision
project: {{title}}
status: active
confidence: medium
created: {{date}}
---
# {{title}}

## Context
Why this decision is needed.

## Decision
What was chosen.

## Alternatives Considered
What was rejected and why.

## Consequences
What this decision implies going forward.
```

### Template Best Practices

- Include all required frontmatter properties
- Use `{{title}}` and `{{date}}` variables
- Provide section headings as scaffolding
- Keep templates minimal; agents fill in the content
- One template per note type

### Applying Templates

```bash
vlt vault="V" templates:apply \
  template="Decision Template" \
  name="Use Redis for caching" \
  path="decisions/Use Redis for caching.md"
```
