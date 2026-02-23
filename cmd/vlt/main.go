// vlt -- fast Obsidian vault CLI (no app required)
//
// Drop-in replacement for the obsidian CLI that operates directly on the
// filesystem. No Obsidian app dependency, no Electron round-trips.
//
// Discovers vaults from the Obsidian config file, resolves notes by title
// or alias, and performs file, property, link, and tag operations.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	vlt "github.com/RamXX/vlt"
)

const version = "0.7.0"

var knownCommands = map[string]bool{
	"read": true, "search": true, "create": true,
	"append": true, "prepend": true, "write": true, "patch": true, "move": true, "delete": true,
	"property:set": true, "property:remove": true, "properties": true,
	"backlinks": true, "links": true, "orphans": true, "unresolved": true,
	"tags": true, "tag": true, "files": true,
	"tasks": true, "daily": true, "templates": true, "templates:apply": true,
	"bookmarks": true, "bookmarks:add": true, "bookmarks:remove": true,
	"uri":    true,
	"vaults": true, "help": true, "version": true,
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd, params, flags := parseArgs(os.Args[1:])

	if cmd == "help" || flags["--help"] || flags["-h"] {
		usage()
		return
	}
	if cmd == "version" || flags["--version"] {
		fmt.Println("vlt " + version)
		return
	}
	format := outputFormat(flags)

	if cmd == "vaults" {
		vaults, err := vlt.DiscoverVaults()
		if err != nil {
			die("%v", err)
		}
		printVaults(vaults, format)
		return
	}
	if cmd == "" {
		die("no command specified. Run 'vlt help' for usage.")
	}

	// Resolve vault
	vaultName := params["vault"]
	if vaultName == "" {
		vaultName = os.Getenv("VLT_VAULT")
	}
	if vaultName == "" {
		die("vault not specified. Use vault=\"<name>\" or set VLT_VAULT env var.")
	}

	v, err := vlt.OpenByName(vaultName)
	if err != nil {
		die("%v", err)
	}

	unlock, err := vlt.LockVault(v.Dir(), vlt.IsWriteCommand(cmd))
	if err != nil {
		die("cannot lock vault: %v", err)
	}
	defer unlock()

	ts := timestampsEnabled(flags["timestamps"])

	// Dispatch
	switch cmd {
	case "read":
		err = dispatchRead(v, params)
	case "search":
		err = dispatchSearch(v, params, format)
	case "create":
		err = dispatchCreate(v, params, flags["silent"], ts)
	case "append":
		err = dispatchAppend(v, params, ts)
	case "prepend":
		err = dispatchPrepend(v, params, ts)
	case "write":
		err = dispatchWrite(v, params, ts)
	case "patch":
		err = dispatchPatch(v, params, flags["delete"], ts)
	case "move":
		err = dispatchMove(v, params)
	case "delete":
		err = dispatchDelete(v, params, flags["permanent"])
	case "property:set":
		err = dispatchPropertySet(v, params)
	case "property:remove":
		err = dispatchPropertyRemove(v, params)
	case "properties":
		err = dispatchProperties(v, params, format)
	case "backlinks":
		err = dispatchBacklinks(v, params, format)
	case "links":
		err = dispatchLinks(v, params, format)
	case "orphans":
		err = dispatchOrphans(v, format)
	case "unresolved":
		err = dispatchUnresolved(v, format)
	case "tags":
		err = dispatchTags(v, params, flags["counts"], format)
	case "tag":
		err = dispatchTag(v, params, format)
	case "files":
		err = dispatchFiles(v, params, flags["total"], format)
	case "tasks":
		err = dispatchTasks(v, params, flags)
	case "daily":
		err = dispatchDaily(v, params)
	case "templates":
		err = dispatchTemplates(v, params, format)
	case "templates:apply":
		err = dispatchTemplatesApply(v, params)
	case "bookmarks":
		err = dispatchBookmarks(v, format)
	case "bookmarks:add":
		err = dispatchBookmarksAdd(v, params)
	case "bookmarks:remove":
		err = dispatchBookmarksRemove(v, params)
	case "uri":
		err = dispatchURI(v, vaultName, params)
	default:
		die("unknown command: %s", cmd)
	}

	if err != nil {
		die("%v", err)
	}
}

// parseArgs splits CLI arguments into a command name, key=value parameters,
// and bare-word flags. It preserves the obsidian CLI's key="value" syntax.
func parseArgs(args []string) (string, map[string]string, map[string]bool) {
	params := make(map[string]string)
	flags := make(map[string]bool)
	var cmd string

	for _, arg := range args {
		if i := strings.Index(arg, "="); i > 0 {
			key := arg[:i]
			val := arg[i+1:]
			// Strip surrounding quotes (shouldn't be needed after shell parsing,
			// but handles edge cases like programmatic invocation).
			val = strings.Trim(val, "\"'")
			params[key] = val
		} else if knownCommands[arg] {
			cmd = arg
		} else {
			flags[arg] = true
		}
	}

	return cmd, params, flags
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vlt: "+format+"\n", args...)
	os.Exit(1)
}

// readStdinIfPiped reads all of stdin if it's being piped (not a terminal).
// Returns empty string if stdin is a terminal.
func readStdinIfPiped() string {
	stat, _ := os.Stdin.Stat()
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "" // stdin is a terminal, not piped
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}

// timestampsEnabled returns true if timestamps should be applied,
// based on the explicit flag or the VLT_TIMESTAMPS environment variable.
func timestampsEnabled(flag bool) bool {
	if flag {
		return true
	}
	return os.Getenv("VLT_TIMESTAMPS") == "1"
}

func usage() {
	fmt.Print(`vlt -- fast Obsidian vault CLI (no app required)

Usage:
  vlt vault="<name>" <command> [args...]

File commands:
  read           file="<title>" [heading="<heading>"]         Read a note (or a specific section)
  create         name="<title>" path="<path>" [content=...] [silent] [timestamps]  Create a note
  append         file="<title>" [content="<text>"] [timestamps]      Append to end of note
  prepend        file="<title>" [content="<text>"] [timestamps]      Prepend after frontmatter
  write          file="<title>" [content="<text>"] [timestamps]      Replace body (preserve frontmatter)
  patch          file="<title>" heading="<heading>" [content="<text>"] [delete] [timestamps]  Section edit
  patch          file="<title>" line="<N>" [content="<text>"] [delete] [timestamps]           Line edit
  patch          file="<title>" line="<N-M>" [content="<text>"] [delete] [timestamps]         Line range edit
  move           path="<from>" to="<to>"                     Move/rename (updates wiki + md links)
  delete         file="<title>" [permanent]                  Trash (or permanently delete)
  files          [folder="<dir>"] [ext="<ext>"] [total]      List vault files
  daily          [date="YYYY-MM-DD"]                         Create or read daily note

Property commands:
  properties     file="<title>"                              Show all frontmatter
  property:set   file="<title>" name="<key>" value="<val>"   Set a frontmatter property
  property:remove file="<title>" name="<key>"                Remove a frontmatter property

Link commands:
  backlinks      file="<title>"                              Notes linking to this note
  links          file="<title>"                              Outgoing links (flags broken)
  orphans                                                    Notes with no incoming links
  unresolved                                                 Broken links across vault

Tag commands:
  tags           [sort="count"] [counts]                     List all tags in vault
  tag            tag="<tagname>"                             Find notes with tag (+ subtags)

Task commands:
  tasks          [file="<title>"] [path="<dir>"] [done] [pending]  List tasks (checkboxes)

Template commands:
  templates                                                    List available templates
  templates:apply template="<name>" name="<title>" path="<path>"  Create note from template

Bookmark commands:
  bookmarks                                                    List bookmarked file paths
  bookmarks:add  file="<title>"                                Add a bookmark for a note
  bookmarks:remove file="<title>"                              Remove a bookmark

URI commands:
  uri            file="<title>" [heading="<H>"] [block="<B>"]  Generate obsidian:// URI for a note

Search:
  search         query="<term> [key:value]" [context="N"]    Search by title, content, properties
  search         regex="<pattern>" [context="N"]              Search by regex (case-insensitive)
                                                              context=N shows N lines before/after each match

Other:
  vaults                                                     List discovered vaults

Options:
  vault="<name>"   Vault name (from Obsidian config), absolute path, or VLT_VAULT env var.
  silent           Suppress output on create.
  permanent        Hard delete instead of .trash.
  delete           Remove heading+content or line(s) instead of replacing (patch).
  timestamps       Auto-manage created_at/updated_at frontmatter (or set VLT_TIMESTAMPS=1).
  counts           Show note counts with tags.
  total            Show count instead of listing files.
  done             Show only completed tasks.
  pending          Show only pending tasks.
  --json           Output in JSON format.
  --yaml           Output in YAML format.
  --csv            Output in CSV format.
  --tsv            Output in TSV (tab-separated values) format.
  --tree           Output file lists as a hierarchical directory tree.

Content from stdin:
  If content= is omitted for create/append/prepend/write, content is read from stdin.

Search filters:
  Property filters can be embedded in search queries: query="term [key:value]"
  Multiple filters: query="architecture [status:active] [type:decision]"
  Filter-only: query="[status:active]"
  Regex search: regex="arch\w+ure" (case-insensitive by default)
  Regex + filters: regex="pattern" query="[status:active]"
  If both query= and regex= provide text, regex takes precedence (with a warning).

Wikilink support:
  [[Note]], [[Note#Heading]], [[Note#^block-id]], [[Note|Display]], ![[Embed]]
  Block references (^block-id) are fully supported in parsing, rename, and backlinks.

Library usage:
  import "github.com/RamXX/vlt"

  vault, _ := vlt.OpenByName("MyVault")
  content, _ := vault.Read("Session Operating Mode", "")
  results, _ := vault.Search(vlt.SearchOptions{Query: "architecture"})
  _ = vault.Append("Daily Log", "New entry", false)

Examples:
  vlt vault="AgentVault" read file="Operating Mode"
  vlt vault="AgentVault" read file="Design Doc" heading="## Architecture"
  vlt vault="ProjectVault" search query="architecture"
  vlt vault="ProjectVault" search query="[status:active] [type:decision]"
  vlt vault="AgentVault" create name="My Note" path="_inbox/My Note.md" content="# Hello" silent
  echo "## Update" | vlt vault="AgentVault" append file="My Note"
  vlt vault="AgentVault" prepend file="My Note" content="New section at top"
  vlt vault="AgentVault" write file="My Note" content="# Replacement body"
  vlt vault="ProjectVault" patch file="Note" heading="## Section" content="new content"
  vlt vault="ProjectVault" patch file="Note" heading="## Section" delete
  vlt vault="ProjectVault" patch file="Note" line="5" content="replacement line"
  vlt vault="ProjectVault" patch file="Note" line="5-10" content="replacement block"
  vlt vault="ProjectVault" patch file="Note" line="5" delete
  vlt vault="AgentVault" move path="_inbox/Old.md" to="decisions/New.md"
  vlt vault="AgentVault" delete file="Old Draft"
  vlt vault="AgentVault" delete file="Old Draft" permanent
  vlt vault="ProjectVault" properties file="My Decision"
  vlt vault="ProjectVault" property:set file="Note" name="status" value="archived"
  vlt vault="ProjectVault" property:remove file="Note" name="confidence"
  vlt vault="AgentVault" backlinks file="Operating Mode"
  vlt vault="ProjectVault" links file="Developer Guide"
  vlt vault="ProjectVault" orphans
  vlt vault="ProjectVault" unresolved
  vlt vault="AgentVault" tags counts sort="count"
  vlt vault="AgentVault" tag tag="project"
  vlt vault="ProjectVault" files folder="docs"
  vlt vault="ProjectVault" files total
  vlt vault="ProjectVault" tasks
  vlt vault="ProjectVault" tasks file="Project Plan" pending
  vlt vault="ProjectVault" tasks path="projects" --json
  vlt vault="AgentVault" daily
  vlt vault="AgentVault" daily date="2025-01-15"
  vlt vault="ProjectVault" orphans --json
  vlt vault="ProjectVault" search query="architecture" --csv
  vlt vault="ProjectVault" search query="architecture" context="2"
  vlt vault="ProjectVault" search query="architecture [status:active]" context="1" --json
  vlt vault="AgentVault" search regex="arch\w+ure"
  vlt vault="AgentVault" search regex="\d{4}-\d{2}-\d{2}" context="2"
  vlt vault="AgentVault" search regex="pattern" query="[status:active]"
  vlt vault="AgentVault" create name="Note" path="_inbox/Note.md" content="# Note" timestamps
  vlt vault="AgentVault" append file="Note" content="more" timestamps
  VLT_TIMESTAMPS=1 vlt vault="AgentVault" write file="Note" content="# New Body"
  vlt vault="ProjectVault" templates
  vlt vault="ProjectVault" templates --json
  vlt vault="ProjectVault" templates:apply template="Meeting Notes" name="Q1 Planning" path="meetings/Q1 Planning.md"
  vlt vault="AgentVault" bookmarks
  vlt vault="AgentVault" bookmarks --json
  vlt vault="AgentVault" bookmarks:add file="Important Note"
  vlt vault="AgentVault" bookmarks:remove file="Old Note"
  vlt vault="ProjectVault" uri file="Design Doc"
  vlt vault="ProjectVault" uri file="Design Doc" heading="Architecture"
  vlt vault="ProjectVault" uri file="Note" block="block-id"
  vlt vaults
`)
}
