package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	vlt "github.com/RamXX/vlt"
)

// outputFormat extracts the output format from flags.
// Returns "json", "csv", "yaml", "tsv", "tree", or "" for plain text.
func outputFormat(flags map[string]bool) string {
	if flags["--json"] {
		return "json"
	}
	if flags["--csv"] {
		return "csv"
	}
	if flags["--yaml"] {
		return "yaml"
	}
	if flags["--tsv"] {
		return "tsv"
	}
	if flags["--tree"] {
		return "tree"
	}
	return ""
}

// formatList outputs a []string in the requested format.
// For plain text, one item per line.
func formatList(items []string, format string) {
	switch format {
	case "json":
		data, _ := json.Marshal(items)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		for _, item := range items {
			w.Write([]string{item})
		}
		w.Flush()
	case "yaml":
		for _, item := range items {
			fmt.Printf("- %s\n", item)
		}
	case "tsv":
		fmt.Println("file")
		for _, item := range items {
			fmt.Println(item)
		}
	case "tree":
		renderTree(items)
	default:
		for _, item := range items {
			fmt.Println(item)
		}
	}
}

// formatTable outputs rows of key-value data in the requested format.
// fields controls column order for CSV and key order for YAML/JSON.
func formatTable(rows []map[string]string, fields []string, format string) {
	switch format {
	case "json":
		data, _ := json.Marshal(rows)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write(fields) // header row
		for _, row := range rows {
			record := make([]string, len(fields))
			for i, f := range fields {
				record[i] = row[f]
			}
			w.Write(record)
		}
		w.Flush()
	case "tsv":
		fmt.Println(strings.Join(fields, "\t"))
		for _, row := range rows {
			record := make([]string, len(fields))
			for i, f := range fields {
				record[i] = row[f]
			}
			fmt.Println(strings.Join(record, "\t"))
		}
	case "yaml":
		for i, row := range rows {
			if i > 0 {
				fmt.Println("---")
			}
			for _, f := range fields {
				if v, ok := row[f]; ok {
					fmt.Printf("%s: %s\n", f, yamlEscapeValue(v))
				}
			}
		}
	default:
		// Plain text: tab-separated, fields in order
		for _, row := range rows {
			parts := make([]string, 0, len(fields))
			for _, f := range fields {
				if v, ok := row[f]; ok {
					parts = append(parts, v)
				}
			}
			fmt.Println(strings.Join(parts, "\t"))
		}
	}
}

// formatTagCounts outputs tag-count pairs in the requested format.
func formatTagCounts(tags []string, counts map[string]int, format string) {
	switch format {
	case "json":
		type tagEntry struct {
			Tag   string `json:"tag"`
			Count int    `json:"count"`
		}
		entries := make([]tagEntry, len(tags))
		for i, t := range tags {
			entries[i] = tagEntry{Tag: t, Count: counts[t]}
		}
		data, _ := json.Marshal(entries)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"tag", "count"})
		for _, t := range tags {
			w.Write([]string{t, fmt.Sprintf("%d", counts[t])})
		}
		w.Flush()
	case "tsv":
		fmt.Println("tag\tcount")
		for _, t := range tags {
			fmt.Printf("%s\t%d\n", t, counts[t])
		}
	case "yaml":
		for _, t := range tags {
			fmt.Printf("- tag: %s\n  count: %d\n", t, counts[t])
		}
	default:
		for _, t := range tags {
			fmt.Printf("#%s\t%d\n", t, counts[t])
		}
	}
}

// formatVaults outputs vault name-path pairs in the requested format.
func formatVaults(names []string, vaults map[string]string, format string) {
	switch format {
	case "json":
		type vaultInfo struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		entries := make([]vaultInfo, len(names))
		for i, n := range names {
			entries[i] = vaultInfo{Name: n, Path: vaults[n]}
		}
		data, _ := json.Marshal(entries)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"name", "path"})
		for _, n := range names {
			w.Write([]string{n, vaults[n]})
		}
		w.Flush()
	case "tsv":
		fmt.Println("name\tpath")
		for _, n := range names {
			fmt.Printf("%s\t%s\n", n, vaults[n])
		}
	case "yaml":
		for _, n := range names {
			fmt.Printf("- name: %s\n  path: %s\n", n, vaults[n])
		}
	default:
		for _, n := range names {
			fmt.Printf("%s\t%s\n", n, vaults[n])
		}
	}
}

// formatSearchResults outputs search results in the requested format.
func formatSearchResults(results []vlt.SearchResult, format string) {
	switch format {
	case "json":
		type jsonResult struct {
			Title string `json:"title"`
			Path  string `json:"path"`
		}
		entries := make([]jsonResult, len(results))
		for i, r := range results {
			entries[i] = jsonResult{Title: r.Title, Path: r.RelPath}
		}
		data, _ := json.Marshal(entries)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"title", "path"})
		for _, r := range results {
			w.Write([]string{r.Title, r.RelPath})
		}
		w.Flush()
	case "tsv":
		fmt.Println("title\tpath")
		for _, r := range results {
			fmt.Printf("%s\t%s\n", r.Title, r.RelPath)
		}
	case "yaml":
		for _, r := range results {
			fmt.Printf("- title: %s\n  path: %s\n", yamlEscapeValue(r.Title), r.RelPath)
		}
	default:
		for _, r := range results {
			fmt.Printf("%s (%s)\n", r.Title, r.RelPath)
		}
	}
}

// formatSearchWithContext outputs context-aware search results in the requested format.
func formatSearchWithContext(matches []vlt.ContextMatch, format string) {
	switch format {
	case "json":
		type jsonContextMatch struct {
			File    string   `json:"file"`
			Line    int      `json:"line"`
			Match   string   `json:"match"`
			Context []string `json:"context"`
		}
		entries := make([]jsonContextMatch, len(matches))
		for i, m := range matches {
			ctx := m.Context
			if ctx == nil {
				ctx = []string{}
			}
			entries[i] = jsonContextMatch{
				File:    m.File,
				Line:    m.Line,
				Match:   m.Match,
				Context: ctx,
			}
		}
		data, _ := json.Marshal(entries)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"file", "line", "content"})
		for _, m := range matches {
			if m.Context == nil {
				w.Write([]string{m.File, fmt.Sprintf("%d", m.Line), m.Match})
				continue
			}
			startLine := m.Line - 1
			ctxBefore := 0
			for j, c := range m.Context {
				if c == m.Match && j <= startLine {
					ctxBefore = j
					break
				}
			}
			baseLineNum := m.Line - ctxBefore
			for j, c := range m.Context {
				w.Write([]string{m.File, fmt.Sprintf("%d", baseLineNum+j), c})
			}
		}
		w.Flush()
	case "tsv":
		fmt.Println("file\tline\tcontent")
		for _, m := range matches {
			if m.Context == nil {
				fmt.Printf("%s\t%d\t%s\n", m.File, m.Line, m.Match)
				continue
			}
			ctxBefore := 0
			for j, c := range m.Context {
				if c == m.Match && j <= m.Line-1 {
					ctxBefore = j
					break
				}
			}
			baseLineNum := m.Line - ctxBefore
			for j, c := range m.Context {
				fmt.Printf("%s\t%d\t%s\n", m.File, baseLineNum+j, c)
			}
		}
	case "yaml":
		for i, m := range matches {
			if i > 0 {
				fmt.Println("---")
			}
			fmt.Printf("file: %s\n", m.File)
			fmt.Printf("line: %d\n", m.Line)
			fmt.Printf("match: %s\n", yamlEscapeValue(m.Match))
			if m.Context != nil {
				fmt.Println("context:")
				for _, c := range m.Context {
					fmt.Printf("  - %s\n", yamlEscapeValue(c))
				}
			}
		}
	default:
		type fileLineKey struct {
			file string
			line int
		}
		emitted := make(map[fileLineKey]bool)
		prevFile := ""

		for _, m := range matches {
			if m.Context == nil {
				fmt.Printf("%s (title match)\n", m.File)
				continue
			}

			if prevFile != "" && m.File != prevFile {
				fmt.Println("--")
			}
			prevFile = m.File

			ctxBefore := 0
			for j, c := range m.Context {
				if c == m.Match {
					ctxBefore = j
					break
				}
			}
			baseLineNum := m.Line - ctxBefore

			for j, c := range m.Context {
				lineNum := baseLineNum + j
				key := fileLineKey{m.File, lineNum}
				if emitted[key] {
					continue
				}
				emitted[key] = true
				fmt.Printf("%s:%d:%s\n", m.File, lineNum, c)
			}
		}
	}
}

// formatLinks outputs link information in the requested format.
func formatLinks(links []vlt.LinkInfo, format string) {
	switch format {
	case "json":
		data, _ := json.Marshal(links)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"target", "path", "broken"})
		for _, l := range links {
			broken := "false"
			if l.Broken {
				broken = "true"
			}
			w.Write([]string{l.Target, l.Path, broken})
		}
		w.Flush()
	case "tsv":
		fmt.Println("target\tpath\tbroken")
		for _, l := range links {
			broken := "false"
			if l.Broken {
				broken = "true"
			}
			fmt.Printf("%s\t%s\t%s\n", l.Target, l.Path, broken)
		}
	case "yaml":
		for _, l := range links {
			fmt.Printf("- target: %s\n  path: %s\n  broken: %v\n", yamlEscapeValue(l.Target), l.Path, l.Broken)
		}
	default:
		for _, l := range links {
			if l.Broken {
				fmt.Printf("  BROKEN: [[%s]]\n", l.Target)
			} else {
				fmt.Printf("  [[%s]] -> %s\n", l.Target, l.Path)
			}
		}
	}
}

// formatUnresolved outputs unresolved link information.
func formatUnresolved(results []vlt.UnresolvedLink, format string) {
	switch format {
	case "json":
		data, _ := json.Marshal(results)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"target", "source"})
		for _, r := range results {
			w.Write([]string{r.Target, r.Source})
		}
		w.Flush()
	case "tsv":
		fmt.Println("target\tsource")
		for _, r := range results {
			fmt.Printf("%s\t%s\n", r.Target, r.Source)
		}
	case "yaml":
		for _, r := range results {
			fmt.Printf("- target: %s\n  source: %s\n", yamlEscapeValue(r.Target), r.Source)
		}
	default:
		for _, r := range results {
			fmt.Printf("[[%s]] in %s\n", r.Target, r.Source)
		}
	}
}

// formatProperties outputs frontmatter properties in the requested format.
func formatProperties(text string, format string) {
	if format == "" {
		fmt.Println(text)
		return
	}

	lines := strings.Split(text, "\n")
	props := make(map[string]string)
	var keys []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "---" || line == "" {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			props[key] = val
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	switch format {
	case "json":
		data, _ := json.Marshal(props)
		fmt.Println(string(data))
	case "csv":
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"key", "value"})
		for _, k := range keys {
			w.Write([]string{k, props[k]})
		}
		w.Flush()
	case "tsv":
		fmt.Println("key\tvalue")
		for _, k := range keys {
			fmt.Printf("%s\t%s\n", k, props[k])
		}
	case "yaml":
		for _, k := range keys {
			fmt.Printf("%s: %s\n", k, props[k])
		}
	}
}

// outputTasks prints tasks in the requested format.
func outputTasks(tasks []vlt.Task, format string) {
	switch format {
	case "json":
		data, _ := json.Marshal(tasks)
		fmt.Println(string(data))
	case "csv":
		fmt.Println("done,text,line,file")
		for _, t := range tasks {
			done := "false"
			if t.Done {
				done = "true"
			}
			fmt.Printf("%s,%q,%d,%s\n", done, t.Text, t.Line, t.File)
		}
	case "yaml":
		for _, t := range tasks {
			fmt.Printf("- text: %s\n  done: %v\n  line: %d\n  file: %s\n", yamlEscapeValue(t.Text), t.Done, t.Line, t.File)
		}
	default:
		for _, t := range tasks {
			check := " "
			if t.Done {
				check = "x"
			}
			fmt.Printf("- [%s] %s (%s:%d)\n", check, t.Text, t.File, t.Line)
		}
	}
}

// treeNode represents a node in a directory tree for tree-format rendering.
type treeNode struct {
	name     string
	isDir    bool
	children []*treeNode
}

// renderTree outputs paths as a hierarchical directory tree using Unicode
// box-drawing characters. Directories are sorted before files at each level.
func renderTree(items []string) {
	if len(items) == 0 {
		return
	}

	root := &treeNode{name: ".", isDir: true}

	for _, item := range items {
		parts := strings.Split(item, "/")
		current := root
		for i, part := range parts {
			isDir := i < len(parts)-1
			var child *treeNode
			for _, c := range current.children {
				if c.name == part && c.isDir == isDir {
					child = c
					break
				}
			}
			if child == nil {
				child = &treeNode{name: part, isDir: isDir}
				current.children = append(current.children, child)
			}
			current = child
		}
	}

	sortTree(root)

	for i, child := range root.children {
		isLast := i == len(root.children)-1
		printTreeNode(child, "", isLast)
	}
}

func sortTree(node *treeNode) {
	sort.Slice(node.children, func(i, j int) bool {
		a, b := node.children[i], node.children[j]
		if a.isDir != b.isDir {
			return a.isDir
		}
		return a.name < b.name
	})
	for _, child := range node.children {
		sortTree(child)
	}
}

func printTreeNode(node *treeNode, prefix string, isLast bool) {
	connector := "\u251c\u2500\u2500 "
	if isLast {
		connector = "\u2514\u2500\u2500 "
	}

	displayName := node.name
	if node.isDir {
		displayName += "/"
	}

	fmt.Printf("%s%s%s\n", prefix, connector, displayName)

	childPrefix := prefix + "\u2502   "
	if isLast {
		childPrefix = prefix + "    "
	}

	for i, child := range node.children {
		childIsLast := i == len(node.children)-1
		printTreeNode(child, childPrefix, childIsLast)
	}
}

// yamlEscapeValue wraps a value in quotes if it contains characters
// that need escaping in YAML (colons, brackets, etc).
func yamlEscapeValue(s string) string {
	if s == "" {
		return `""`
	}
	needsQuoting := false
	for _, c := range s {
		if c == ':' || c == '#' || c == '[' || c == ']' || c == '{' || c == '}' ||
			c == ',' || c == '&' || c == '*' || c == '!' || c == '|' || c == '>' ||
			c == '\'' || c == '"' || c == '%' || c == '@' || c == '`' {
			needsQuoting = true
			break
		}
	}
	if needsQuoting {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}
