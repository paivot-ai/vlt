# vlt Command Reference

Complete reference for all vlt commands, parameters, and flags.

## Global Parameters

| Parameter | Description |
|-----------|-------------|
| `vault="name"` | Select vault by name, absolute path, or `~/` path |
| `--json` | Output as JSON array |
| `--yaml` | Output as YAML list |
| `--csv` | Output as CSV with headers |
| `--tsv` | Output as tab-separated values |
| `--tree` | Output as directory tree (file listings only) |
| `--help`, `-h` | Show usage information |
| `--version` | Print version |

Environment variables:
- `VLT_VAULT` -- Default vault name (overridden by `vault=` parameter)
- `VLT_VAULT_PATH` -- Direct path to vault (fallback when Obsidian config unavailable)
- `VLT_TIMESTAMPS` -- Set to `1` to enable timestamps on all write operations

---

## File Operations

### read

Print note content, optionally scoped to a specific heading section. Can include forward-linked or back-linked notes for graph-aware retrieval.

```bash
vlt vault="V" read file="Note Title"
vlt vault="V" read file="Note Title" heading="## Section Name"
vlt vault="V" read file="Note Title" follow
vlt vault="V" read file="Note Title" backlinks
```

**Parameters:**
- `file=` (required) -- Note title or alias
- `heading=` (optional) -- Heading to scope output to (include `#` prefix)

**Flags:**
- `follow` -- After the primary note, append the full content of every note it links to (depth 1 forward links). Broken links are silently skipped. Self-links and duplicates are excluded.
- `backlinks` -- After the primary note, append the full content of every note that links TO it (depth 1 backlinks).

**Behavior:**
- Outputs the full note content to stdout
- When `heading=` is specified, the primary output is scoped to that section, but `follow` still resolves links from the full note
- When `follow` or `backlinks` is used, linked notes are separated by `--- [[Title]] (path) ---` delimiters
- Resolves notes by filename first, then by alias
- Exit 1 if note not found

**Why use follow/backlinks:** Retrieves a note's link neighborhood in a single call. Without these flags, an agent would need N+1 calls (read the note, parse links, read each linked note). With `follow`, it's one call.

---

### create

Create a new note in the vault.

```bash
vlt vault="V" create name="Title" path="folder/Title.md" content="Body text"
vlt vault="V" create name="Title" path="folder/Title.md" content="Body" silent timestamps
```

**Parameters:**
- `name=` (required) -- Display name for the note
- `path=` (required) -- Relative path from vault root (must end in `.md`)
- `content=` (optional) -- Note body; if omitted, reads from stdin

**Flags:**
- `silent` -- Suppress success output
- `timestamps` -- Add `created_at` and `updated_at` frontmatter properties

**Behavior:**
- Creates parent directories as needed
- Exits 0 with a stderr warning if a file already exists at the path (no-op)
- Content may include frontmatter (fenced by `---`)

---

### append

Append content to the end of a note.

```bash
vlt vault="V" append file="Note" content="New paragraph."
echo "piped content" | vlt vault="V" append file="Note"
```

**Parameters:**
- `file=` (required) -- Note title or alias
- `content=` (optional) -- Content to append; if omitted, reads from stdin

**Flags:**
- `timestamps` -- Update `updated_at` property

---

### prepend

Insert content immediately after frontmatter (or at the beginning if no frontmatter).

```bash
vlt vault="V" prepend file="Note" content="Inserted at top of body."
```

**Parameters:**
- `file=` (required) -- Note title or alias
- `content=` (optional) -- Content to prepend; if omitted, reads from stdin

**Flags:**
- `timestamps` -- Update `updated_at` property

---

### write

Replace the entire body of a note while preserving frontmatter.

```bash
vlt vault="V" write file="Note" content="Completely new body."
```

**Parameters:**
- `file=` (required) -- Note title or alias
- `content=` (optional) -- New body; if omitted, reads from stdin

**Flags:**
- `timestamps` -- Update `updated_at` property

**Behavior:**
- Frontmatter block is preserved untouched
- Everything after the closing `---` is replaced

---

### patch

Targeted edits: replace or delete content by heading, line number, or find-and-replace.

```bash
# Replace section under a heading
vlt vault="V" patch file="Note" heading="## Status" content="Done."

# Delete a section
vlt vault="V" patch file="Note" heading="## Deprecated" delete

# Replace a single line (1-based)
vlt vault="V" patch file="Note" line="5" content="Replacement line."

# Replace a line range
vlt vault="V" patch file="Note" line="10-15" content="Replacement block."

# Delete lines
vlt vault="V" patch file="Note" line="10-15" delete

# Find and replace (file-wide)
vlt vault="V" patch file="Note" old="old text" new="new text"

# Find and replace (scoped to a heading)
vlt vault="V" patch file="Note" heading="## Section" old="old text" new="new text"
```

**Parameters:**
- `file=` (required) -- Note title or alias
- `heading=` (optional) -- Target heading. Accepts `"## Section"` (exact level match) or `"Section"` (any level)
- `line=` (optional, mutually exclusive with `heading=`) -- Line number or range (`N` or `N-M`)
- `content=` (optional) -- Replacement content; if omitted, reads from stdin
- `old=` + `new=` (optional) -- Find-and-replace mode. Can be scoped with `heading=` or `line=`

**Flags:**
- `delete` -- Delete the targeted section/lines instead of replacing
- `timestamps` -- Update `updated_at` property

**Behavior with headings:**
- Replaces from the heading line through the next heading of same or higher level (exclusive)
- The heading line itself is preserved; content beneath it is replaced
- If `delete` is set, both the heading and its content are removed
- The heading must be unique within the note; duplicate headings produce an error with match count and line numbers

---

### delete

Remove a note from the vault.

```bash
vlt vault="V" delete file="Note"
vlt vault="V" delete file="Note" permanent
```

**Parameters:**
- `file=` or `path=` (one required) -- Note title/alias or vault-relative path

**Flags:**
- `permanent` -- Hard-delete instead of moving to `.trash/`

---

### move

Move or rename a note, automatically repairing all wikilinks and markdown links vault-wide.

```bash
vlt vault="V" move path="old/path.md" to="new/path.md"
```

**Parameters:**
- `path=` (required) -- Current relative path from vault root
- `to=` (required) -- New relative path from vault root

**Behavior:**
- Creates destination directories as needed
- Updates all `[[wikilinks]]` referencing the old title
- Updates all `[markdown](links)` with recomputed relative paths
- Preserves heading, block, and display-text fragments in links

---

### daily

Create or read a daily note using Obsidian daily note configuration.

```bash
vlt vault="V" daily                    # Today's note (creates if missing, prints if exists)
vlt vault="V" daily date="2025-01-15"  # Specific date
```

**Parameters:**
- `date=` (optional) -- Date in `YYYY-MM-DD` format (defaults to today)

**Behavior:**
- If the daily note exists, prints its content
- If the daily note does not exist, creates it (using template from Obsidian config if configured)
- Reads config from `.obsidian/daily-notes.json` or `.obsidian/plugins/periodic-notes/data.json`
- Respects configured folder and date format
- Translates Moment.js format tokens to Go equivalents

---

### files

List files in the vault with optional filtering.

```bash
vlt vault="V" files
vlt vault="V" files folder="decisions" ext=".md"
vlt vault="V" files total
vlt vault="V" files --json
vlt vault="V" files --tree
```

**Parameters:**
- `folder=` (optional) -- Restrict to a subdirectory
- `ext=` (optional) -- Filter by file extension (e.g., `.md`)

**Flags:**
- `total` -- Show count instead of listing files

---

## Property Operations

### properties

Display the raw YAML frontmatter block of a note.

```bash
vlt vault="V" properties file="Note"
```

### property:set

Set or update a YAML frontmatter property.

```bash
vlt vault="V" property:set file="Note" name="status" value="active"
vlt vault="V" property:set file="Note" name="tags" value="[go, cli]"
```

**Parameters:**
- `file=` (required) -- Note title or alias
- `name=` (required) -- Property key
- `value=` (required) -- Property value (strings, numbers, arrays in YAML syntax)

### property:remove

Remove a YAML frontmatter property.

```bash
vlt vault="V" property:remove file="Note" name="deprecated_field"
```

---

## Link Operations

### backlinks

Find all notes that link to a given note (includes embeds).

```bash
vlt vault="V" backlinks file="Note"
```

**Output:** One relative path per line.

### links

Show outgoing links from a note, marking broken ones.

```bash
vlt vault="V" links file="Note"
```

**Output:** Lines in the format `target` or `target [broken]`.

### orphans

Find notes with no incoming links (alias-aware).

```bash
vlt vault="V" orphans
```

### unresolved

Find all broken wikilinks across the entire vault.

```bash
vlt vault="V" unresolved
```

**Output:** Lines in the format `target\tsource_path`.

---

## Search Operations

### search

Find notes by title, content, frontmatter properties, or regex.

```bash
# Text search
vlt vault="V" search query="authentication"

# Property-filtered search
vlt vault="V" search query="[status:active] [type:decision]"

# Combined text + property filter
vlt vault="V" search query="caching [project:my-app]"

# Regex search
vlt vault="V" search regex="TODO|FIXME"

# Regex with context lines
vlt vault="V" search regex="func\s+\w+Error" context="3"

# Regex + property filter
vlt vault="V" search regex="pattern" query="[status:active]"
```

**Parameters:**
- `query=` (optional) -- Text and/or `[key:value]` property filters
- `regex=` (optional) -- Regular expression pattern (case-insensitive)
- `context=` (optional) -- Number of surrounding context lines (like `grep -C`)
- `path=` (optional) -- Restrict search to a vault subdirectory (e.g., `path="decisions"`)

**Output modes:**
- Default: One file path per line
- With `context=`: File, line number, match, and surrounding lines

---

## Tag Operations

### tags

List all tags found across the vault.

```bash
vlt vault="V" tags
vlt vault="V" tags counts              # Include note counts
vlt vault="V" tags counts sort="count" # Sort by frequency
```

**Parameters:**
- `sort="count"` (optional) -- Sort tags by frequency (most used first)

**Flags:**
- `counts` -- Show count of notes per tag

### tag

Find notes containing a specific tag, with hierarchical support.

```bash
vlt vault="V" tag tag="architecture"
vlt vault="V" tag tag="design"  # Also finds #design/patterns, #design/ux
```

Tags are collected from both frontmatter `tags` field and inline `#tag` syntax.
Matching is case-insensitive. Subtags are included automatically.

---

## Task Operations

### tasks

List checkbox items from notes.

```bash
vlt vault="V" tasks                        # All tasks vault-wide
vlt vault="V" tasks file="Sprint Plan"     # Tasks from one note
vlt vault="V" tasks path="projects"        # Tasks from folder
vlt vault="V" tasks pending                # Unchecked only
vlt vault="V" tasks done                   # Checked only
```

**Parameters:**
- `file=` (optional) -- Single note
- `path=` (optional) -- Scope to folder/directory

**Flags:**
- `done` -- Only completed tasks (`- [x]` or `- [X]`)
- `pending` -- Only incomplete tasks (`- [ ]`)

---

## Template Operations

### templates

List available templates discovered from `.obsidian/templates.json` or the `templates/` directory.

```bash
vlt vault="V" templates
```

### templates:apply

Create a new note from a template with variable substitution.

```bash
vlt vault="V" templates:apply template="Meeting Notes" name="Team Sync" path="meetings/Team Sync.md"
```

**Variables supported:**
- `{{title}}` -- Note name
- `{{date}}` -- Current date (default: YYYY-MM-DD)
- `{{date:FORMAT}}` -- Formatted date (Moment.js tokens translated to Go)
- `{{time}}` -- Current time (default: HH:mm)
- `{{time:FORMAT}}` -- Formatted time

---

## Bookmark Operations

### bookmarks

List bookmarked file paths from `.obsidian/bookmarks.json`.

```bash
vlt vault="V" bookmarks
```

### bookmarks:add

Add a bookmark for a note.

```bash
vlt vault="V" bookmarks:add file="Important Note"
```

### bookmarks:remove

Remove a bookmark.

```bash
vlt vault="V" bookmarks:remove file="Important Note"
```

---

## Integrity Operations

### integrity:baseline

Register SHA-256 content hashes for all `.md` files in the vault. This creates the baseline for tamper detection.

```bash
vlt vault="V" integrity:baseline
```

**Behavior:**
- Walks all `.md` files in the vault (skipping hidden directories)
- Registers each file's content hash in `~/.vlt/registries/<vault-id>/registry.json`
- Overwrites any existing registry entries
- Prints confirmation on success

---

### integrity:status

Show the integrity status of all registered files plus any untracked files.

```bash
vlt vault="V" integrity:status
vlt vault="V" integrity:status --json
```

**Output:**
- Lists files with issues (mismatch, untracked) with their status
- Reports total count of OK files and issues
- JSON format returns an array of `{path, status}` objects

**Statuses:**
- `ok` -- Content matches the registered hash
- `untracked` -- File exists but has no registry entry
- `mismatch` -- Content differs from the registered hash

---

### integrity:acknowledge

Re-register a file after an external modification, accepting the current content as the new baseline.

```bash
vlt vault="V" integrity:acknowledge file="Note Title"
vlt vault="V" integrity:acknowledge since="1h"
```

**Parameters:**
- `file=` (one of) -- Note title or alias to acknowledge
- `since=` (one of) -- Duration string (Go format: `1h`, `30m`, `2h30m`). Acknowledges all `.md` files modified within the duration.

**Behavior:**
- Re-reads the file and registers its current hash
- `since=` walks all vault files and filters by modification time

---

## URI Generation

### uri

Generate an `obsidian://` URI for opening a note in the desktop app.

```bash
vlt vault="V" uri file="Note"
vlt vault="V" uri file="Note" heading="## Section"
vlt vault="V" uri file="Note" block="block-id"
```

**Parameters:**
- `file=` (required) -- Note title
- `heading=` (optional) -- Target heading
- `block=` (optional) -- Target block reference

---

## Discovery Commands

### vaults

List all Obsidian vaults discovered from the config file.

```bash
vlt vaults
```

### help

Show usage information.

```bash
vlt help
vlt --help
vlt -h
```

### version

Print the vlt version.

```bash
vlt version
vlt --version
```
