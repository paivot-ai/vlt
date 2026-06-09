# Advanced vlt Techniques

Deep coverage of vlt's advanced features for power users and complex workflows.

## Inert Zone Masking

vlt uses a 6-pass masking system to prevent false positives when scanning for links, tags,
and references. Content inside these zones is masked (replaced with spaces, preserving byte
offsets and line numbers) before any scanning occurs:

| Pass | Zone | Syntax |
|------|------|--------|
| 1 | Fenced code blocks | ` ``` ... ``` ` |
| 2 | Inline code | `` ` ... ` `` and ` `` ... `` ` |
| 3 | Obsidian comments | `%% ... %%` |
| 4 | HTML comments | `<!-- ... -->` |
| 5 | Display math | `$$ ... $$` |
| 6 | Inline math | `$ ... $` |

**Why this matters for agents:** When searching, `backlinks`, `links`, `tags`, `orphans`,
and `unresolved` all respect inert zones. A wikilink inside a code block is never reported
as a backlink or an unresolved reference. This matches Obsidian's own behavior.

**Edge case:** An unclosed fenced code block masks everything from the opening fence to EOF.
This matches Obsidian's rendering behavior.

---

## Wikilink Formats

vlt handles the full Obsidian wikilink specification:

| Format | Example | Components |
|--------|---------|------------|
| Simple | `[[Note Title]]` | title only |
| Heading | `[[Note#Section]]` | title + heading |
| Block ref | `[[Note#^abc123]]` | title + block ID |
| Display text | `[[Note\|Custom Text]]` | title + alias |
| Embed | `![[Note]]` | embedded note |
| Combined | `![[Note#Section\|Text]]` | all components |

Path-form links (`[[folder/Note]]`) and attachment embeds (`![[photo.png]]`) resolve
like in Obsidian: `backlinks`, `links`, `orphans`, and `unresolved` all account for them.

When `move` renames a note, all wikilink variants are updated. Heading, block,
and display-text fragments are preserved. Markdown links (`[text](path.md)`) are
also updated with recomputed relative paths.

---

## Property-Based Search Deep Dive

The `search` command supports a structured query syntax for filtering by frontmatter properties.

### Syntax

Property filters are enclosed in square brackets within the `query=` parameter:

```
[key:value]           -- Exact match
[key:value] text      -- Property filter + text search
[k1:v1] [k2:v2]      -- Multiple property filters (AND logic)
```

### How It Works

1. vlt extracts `[key:value]` pairs from the query string
2. Remaining text becomes the text search term
3. Notes must match ALL property filters AND the text term
4. Property matching checks the YAML frontmatter of each note

### Practical Examples

```bash
# All active decisions for a project
vlt vault="V" search query="[type:decision] [project:my-app] [status:active]"

# Pattern notes for Go
vlt vault="V" search query="[type:pattern] [stack:go]"

# Notes about "caching" in a specific project
vlt vault="V" search query="caching [project:my-app]"

# Combine regex with property filter
vlt vault="V" search regex="func\s+Test" query="[type:pattern]"
```

### Limitations

- Values are matched as substrings within the property line
- Array properties (`tags: [go, cli]`) match if the value appears anywhere in the array string
- No negation (`[type:!decision]` is not supported)
- No comparison operators (no `[priority:>3]`)

---

## Regex Search with Context

The `regex=` parameter enables powerful pattern matching across vault content.

```bash
# Find TODO comments with surrounding context
vlt vault="V" search regex="TODO|FIXME|HACK" context="3"

# Find function definitions
vlt vault="V" search regex="^## .+Error" context="2"

# Find notes referencing specific identifiers
vlt vault="V" search regex="PostgreSQL.*index" context="1"
```

### Context Output Format

When `context=` is specified, output includes file path, line number, the matching line,
and surrounding lines:

```
path/to/note.md:15: The matching line here
path/to/note.md-13: Context line before
path/to/note.md-14: Context line before
path/to/note.md-16: Context line after
path/to/note.md-17: Context line after
```

---

## Template Variable System

Templates support variable substitution with Moment.js-compatible format tokens.

### Built-in Variables

| Variable | Default Output | Description |
|----------|---------------|-------------|
| `{{title}}` | Note name | Passed via `name=` parameter |
| `{{date}}` | YYYY-MM-DD | Current date |
| `{{time}}` | HH:mm | Current time |

### Custom Date/Time Formats

| Variable | Example Format | Example Output |
|----------|---------------|----------------|
| `{{date:YYYY-MM-DD}}` | 2026-02-19 | ISO date |
| `{{date:DD MMM YYYY}}` | 19 Feb 2026 | Human date |
| `{{date:dddd}}` | Wednesday | Day name |
| `{{time:HH:mm:ss}}` | 14:30:00 | Time with seconds |
| `{{time:hh:mm A}}` | 02:30 PM | 12-hour format |

### Moment.js Token Translation

vlt translates Moment.js tokens (used by Obsidian) to Go time format:

| Moment.js | Go | Meaning |
|-----------|----|---------|
| YYYY | 2006 | 4-digit year |
| MM | 01 | Month (zero-padded) |
| DD | 02 | Day (zero-padded) |
| HH | 15 | Hour (24-hour) |
| mm | 04 | Minute |
| ss | 05 | Second |
| dddd | Monday | Day name |
| MMMM | January | Month name |
| A | PM | AM/PM |

---

## Daily Note Configuration

vlt reads daily note configuration from Obsidian plugin configs:

### Core Daily Notes Plugin

Config path: `.obsidian/daily-notes.json`

```json
{
  "folder": "daily",
  "format": "YYYY-MM-DD",
  "template": "templates/Daily Template"
}
```

### Periodic Notes Plugin

Config path: `.obsidian/plugins/periodic-notes/data.json`

```json
{
  "daily": {
    "folder": "daily",
    "format": "YYYY-MM-DD",
    "template": "templates/Daily Template"
  }
}
```

vlt checks the core plugin first, then falls back to periodic-notes.

---

## Bookmark Management

Bookmarks are stored in `.obsidian/bookmarks.json` and support nested groups.

```json
{
  "items": [
    { "type": "file", "ctime": 1708300000, "path": "important.md" },
    {
      "type": "group",
      "title": "Project Notes",
      "items": [
        { "type": "file", "ctime": 1708300001, "path": "projects/app.md" }
      ]
    }
  ]
}
```

vlt handles nested groups recursively when listing, adding, or removing bookmarks.

---

## Stdin and Piping Patterns

Commands that accept `content=` also read from stdin when `content=` is omitted.
This enables powerful piping workflows:

```bash
# Pipe command output into a note
date | vlt vault="V" append file="Log"

# Create a note from a heredoc
vlt vault="V" create name="Meeting Notes" path="_inbox/Meeting Notes.md" <<'EOF'
---
type: meeting
created: 2026-02-19
---
# Team Sync
## Attendees
- Alice
- Bob
## Notes
Discussion about architecture.
EOF

# Pipe between vlt commands
vlt vault="V" read file="Template" | vlt vault="V" create name="New Note" path="notes/New Note.md"

# Transform content through Unix tools
vlt vault="V" read file="Raw Data" | sort | uniq | vlt vault="V" write file="Sorted Data"
```

---

## Error Handling

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (including empty results -- Unix convention) |
| 1 | Error (note not found, invalid parameters, file system error) |

### Error Output

All errors go to stderr with `vlt:` prefix:

```
vlt: note not found: "Nonexistent Note"
vlt: vault not found: "BadName"
vlt: file already exists: "path/to/note.md"
```

### Silent Empty Results

Like `grep`, vlt returns exit 0 with no output for empty result sets.
This enables clean conditional logic:

```bash
# Check if any orphans exist
if vlt vault="V" orphans | grep -q .; then
  echo "Orphan notes found"
fi

# Count results without error on empty
count=$(vlt vault="V" search query="[type:decision]" | wc -l)
```

---

## File Integrity Workflow

### Initial Setup

Run `integrity:baseline` once to register all existing vault files:

```bash
vlt vault="V" integrity:baseline
# integrity baseline registered for all vault files
```

After this, all subsequent writes through vlt automatically update the registry.

### Checking Status

```bash
# Quick check -- are any files tampered?
vlt vault="V" integrity:status

# Programmatic check
vlt vault="V" integrity:status --json
```

### Handling Mismatches

When a file is modified outside vlt (e.g., by Obsidian, a text editor, or git):

```bash
# Read will warn on stderr
vlt vault="V" read file="Modified Note"
# vlt: INTEGRITY MISMATCH for "Modified Note" -- file modified outside vlt
# (content is still returned)

# Acknowledge the change (accept current content as new baseline)
vlt vault="V" integrity:acknowledge file="Modified Note"

# Or acknowledge all recent changes (e.g., after a git pull)
vlt vault="V" integrity:acknowledge since="5m"
```

### Library API: ReadResult

For Go library consumers, `Read`, `ReadFollow`, and `ReadWithBacklinks` return a `ReadResult`
struct instead of a raw string (breaking change in v0.9.0):

```go
type ReadResult struct {
    Content   string
    Integrity IntegrityStatus  // OK, Untracked, Mismatch, NoRegistry
}

result, err := vault.Read("Note", "")
fmt.Print(result.Content)
if result.Integrity == vlt.IntegrityMismatch {
    // Handle tampered file
}
```

---

## Advisory Locking

vlt uses kernel-managed advisory locks for safe concurrent access:

- **Read commands** are lock-free by default (atomic writes guarantee a complete
  old or new file); pass `--strict-flock` to acquire a shared lock instead
- **Write commands** acquire an exclusive lock (blocks other writers)
- Lock file: `.vlt.lock` in the vault root
- Implementation: `flock(2)` on Unix, `LockFileEx`/`UnlockFileEx` on Windows
- Auto-releases on process crash or kill -- no stale lock cleanup needed
- **Timeout**: acquisition fails after 10 seconds instead of waiting forever on a
  wedged holder; tune with `VLT_LOCK_TIMEOUT` (a Go duration like `30s`, `0` = wait forever)

This is transparent to CLI users. Library consumers can use `vlt.LockVault()` directly.

---

## Performance Considerations

vlt is designed for speed with zero external dependencies:

- **No index or database**: Reads directly from filesystem
- **No daemon**: Stateless, runs and exits
- **Lazy per-invocation index**: One vault walk serves all lookups in a process; the alias map is built only when a filename lookup misses
- **Compiled Go binary**: Sub-millisecond startup

For large vaults (10,000+ notes), commands that scan all files (`search`, `orphans`,
`unresolved`, `tags`) will take longer than targeted commands (`read`, `properties`,
`backlinks`). Structure queries to narrow scope when possible:

```bash
# Faster: property-filtered search narrows files early
vlt vault="V" search query="[project:my-app] caching"

# Slower: text search across entire vault
vlt vault="V" search query="caching"

# Faster: scoped task listing
vlt vault="V" tasks path="projects/my-app"

# Slower: vault-wide task listing
vlt vault="V" tasks
```
