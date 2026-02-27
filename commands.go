package vlt

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SearchResult holds a single search match.
type SearchResult struct {
	Title   string
	RelPath string
}

// ContextMatch holds a single line-level match with surrounding context.
type ContextMatch struct {
	File    string   // relative path
	Line    int      // 1-based line number of the match
	Match   string   // the matched line text
	Context []string // surrounding lines including the match line
}

// lineRange represents an inclusive range of 0-based line indices.
type lineRange struct {
	start int
	end   int
}

// LinkInfo holds outgoing link information.
type LinkInfo struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Broken bool   `json:"broken"`
}

// UnresolvedLink holds an unresolved link and its source.
type UnresolvedLink struct {
	Target string `json:"target"`
	Source string `json:"source"`
}

// SearchOptions parameterises a Search or SearchWithContext call.
type SearchOptions struct {
	Query    string
	Regex    string
	Path     string
	ContextN int
}

// PatchOptions parameterises a Patch call.
type PatchOptions struct {
	Heading    string
	LineSpec   string
	Content    string
	Delete     bool
	Timestamps bool
}

// MoveResult is returned by Move and reports what changed.
type MoveResult struct {
	OldTitle         string
	NewTitle         string
	WikilinksUpdated int
	MdLinksUpdated   int
}

// ErrNoteExists is returned by Create when a note already exists at the target path.
var ErrNoteExists = fmt.Errorf("note already exists")

// sectionBounds holds the line range of a section identified by findSection.
// HeadingLine is the 0-based index of the heading line itself.
// ContentStart is the 0-based index of the first content line after the heading.
// ContentEnd is the 0-based index one past the last content line (exclusive).
// If the section has no content, ContentStart == ContentEnd.
type sectionBounds struct {
	HeadingLine  int
	ContentStart int
	ContentEnd   int
}

// -----------------------------------------------------------------
// Unexported helpers
// -----------------------------------------------------------------

// findMatchLines returns 0-based line indices where query appears (case-insensitive).
func findMatchLines(lines []string, query string) []int {
	queryLower := strings.ToLower(query)
	var matches []int
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			matches = append(matches, i)
		}
	}
	return matches
}

// findMatchLinesRegex returns 0-based line indices where the compiled regex matches.
func findMatchLinesRegex(lines []string, re *regexp.Regexp) []int {
	var matches []int
	for i, line := range lines {
		if re.MatchString(line) {
			matches = append(matches, i)
		}
	}
	return matches
}

// expandAndMerge takes match line indices and a context radius, producing merged
// non-overlapping ranges clamped to [0, totalLines).
func expandAndMerge(matchLines []int, contextN int, totalLines int) []lineRange {
	if len(matchLines) == 0 {
		return nil
	}

	var ranges []lineRange
	for _, m := range matchLines {
		start := m - contextN
		if start < 0 {
			start = 0
		}
		end := m + contextN
		if end >= totalLines {
			end = totalLines - 1
		}
		ranges = append(ranges, lineRange{start, end})
	}

	// Merge overlapping or adjacent ranges
	merged := []lineRange{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		if ranges[i].start <= last.end+1 {
			if ranges[i].end > last.end {
				last.end = ranges[i].end
			}
		} else {
			merged = append(merged, ranges[i])
		}
	}

	return merged
}

// searchFilterPattern matches [key:value] property filters in search queries.
var searchFilterPattern = regexp.MustCompile(`\[(\w+):([^\]]+)\]`)

// parseSearchQuery splits a query into text terms and property filters.
// Filters are [key:value] pairs extracted from the query string.
func parseSearchQuery(query string) (text string, filters map[string]string) {
	filters = make(map[string]string)
	matches := searchFilterPattern.FindAllStringSubmatch(query, -1)
	for _, m := range matches {
		filters[m[1]] = m[2]
	}
	text = strings.TrimSpace(searchFilterPattern.ReplaceAllString(query, ""))
	return
}

// headingLevel returns the Markdown heading level (number of leading # chars).
// Returns 0 if the line is not a heading.
func headingLevel(line string) int {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0
	}
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	// Must be followed by a space or end of line to be a valid heading
	if level >= len(trimmed) || trimmed[level] == ' ' {
		return level
	}
	return 0
}

// findSection locates a heading in the given lines and returns its bounds.
// The heading parameter should include the # prefix (e.g., "## Section A").
// Heading match is case-insensitive and trims whitespace.
// The section extends from the heading to the line before the next heading of
// equal or higher level (or EOF). This operates on RAW content, not masked.
func findSection(lines []string, heading string) (sectionBounds, bool) {
	heading = strings.TrimSpace(heading)
	targetLevel := headingLevel(heading)
	if targetLevel == 0 {
		return sectionBounds{}, false
	}

	headingTextLower := strings.ToLower(heading)

	for i, line := range lines {
		lineTrimmed := strings.TrimSpace(line)
		if strings.ToLower(lineTrimmed) == headingTextLower {
			// Found the heading. Now find the end of the section.
			contentStart := i + 1
			contentEnd := len(lines) // default: extends to EOF

			for j := contentStart; j < len(lines); j++ {
				lvl := headingLevel(lines[j])
				if lvl > 0 && lvl <= targetLevel {
					contentEnd = j
					break
				}
			}

			return sectionBounds{
				HeadingLine:  i,
				ContentStart: contentStart,
				ContentEnd:   contentEnd,
			}, true
		}
	}

	return sectionBounds{}, false
}

// parseLineSpec parses a line specification like "5" or "5-10" into start and end
// line numbers (1-based, inclusive).
func parseLineSpec(spec string) (start, end int, err error) {
	if idx := strings.Index(spec, "-"); idx >= 0 {
		startStr := spec[:idx]
		endStr := spec[idx+1:]
		start, err = parseInt(startStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid line range start: %s", startStr)
		}
		end, err = parseInt(endStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid line range end: %s", endStr)
		}
		return start, end, nil
	}

	start, err = parseInt(spec)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid line number: %s", spec)
	}
	return start, start, nil
}

// parseInt parses a string as a positive integer.
func parseInt(s string) (int, error) {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		n = n*10 + int(ch-'0')
	}
	if n == 0 {
		return 0, fmt.Errorf("not a positive number: %s", s)
	}
	return n, nil
}

// ParseInt0 parses a string as a non-negative integer (0 is allowed).
// Exported for use by the CLI dispatch layer.
func ParseInt0(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

// encodeURIComponent percent-encodes a string for use as a URI query parameter
// value. Encodes spaces as %20, slashes as %2F, ampersands as %26, plus as %2B,
// and other reserved characters. Uses url.QueryEscape (which encodes everything
// aggressively) then replaces + with %20 since Obsidian expects %20 for spaces.
func encodeURIComponent(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// -----------------------------------------------------------------
// Vault methods
// -----------------------------------------------------------------

// Read returns the contents of a note resolved by title.
// If heading is non-empty, only the specified section is returned
// (heading line + content through the next same-or-higher-level heading).
func (v *Vault) Read(title, heading string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if heading == "" {
		return string(data), nil
	}

	// Heading-scoped read: find the section and return heading + content.
	lines := strings.Split(string(data), "\n")
	bounds, found := findSection(lines, heading)
	if !found {
		return "", fmt.Errorf("heading %q not found in %q", heading, title)
	}

	// Extract from heading line through end of section.
	section := lines[bounds.HeadingLine:bounds.ContentEnd]
	output := strings.Join(section, "\n")

	// Ensure exactly one trailing newline (matches file convention).
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return output, nil
}

// LinkedNote holds a related note's title and content, returned by ReadFollow
// and ReadWithBacklinks.
type LinkedNote struct {
	Title   string // note title (stem of filename)
	Path    string // vault-relative path
	Content string // full file content
}

// ReadFollow returns the content of the requested note plus the full content
// of every note it forward-links to (depth 1). This lets callers retrieve a
// note's entire link neighborhood in a single call.
func (v *Vault) ReadFollow(title, heading string) (string, []LinkedNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return "", nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	primary := string(data)
	if heading != "" {
		lines := strings.Split(primary, "\n")
		bounds, found := findSection(lines, heading)
		if !found {
			return "", nil, fmt.Errorf("heading %q not found in %q", heading, title)
		}
		section := lines[bounds.HeadingLine:bounds.ContentEnd]
		primary = strings.Join(section, "\n")
		if !strings.HasSuffix(primary, "\n") {
			primary += "\n"
		}
	}

	// Parse outgoing wikilinks from the full file (even if heading-scoped)
	links := ParseWikilinks(string(data))
	seen := make(map[string]bool)
	var linked []LinkedNote
	for _, wl := range links {
		if wl.Title == title || seen[wl.Title] {
			continue
		}
		seen[wl.Title] = true

		linkedPath, resolveErr := resolveNote(v.dir, wl.Title)
		if resolveErr != nil {
			continue // skip broken links
		}
		linkedData, readErr := os.ReadFile(linkedPath)
		if readErr != nil {
			continue
		}
		relPath, _ := filepath.Rel(v.dir, linkedPath)
		linked = append(linked, LinkedNote{
			Title:   wl.Title,
			Path:    relPath,
			Content: string(linkedData),
		})
	}

	return primary, linked, nil
}

// ReadWithBacklinks returns the content of the requested note plus the full
// content of every note that links TO it (depth 1 backlinks).
func (v *Vault) ReadWithBacklinks(title, heading string) (string, []LinkedNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return "", nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	primary := string(data)
	if heading != "" {
		lines := strings.Split(primary, "\n")
		bounds, found := findSection(lines, heading)
		if !found {
			return "", nil, fmt.Errorf("heading %q not found in %q", heading, title)
		}
		section := lines[bounds.HeadingLine:bounds.ContentEnd]
		primary = strings.Join(section, "\n")
		if !strings.HasSuffix(primary, "\n") {
			primary += "\n"
		}
	}

	blPaths, err := FindBacklinks(v.dir, title)
	if err != nil {
		return primary, nil, err
	}

	var linked []LinkedNote
	for _, relPath := range blPaths {
		absPath := filepath.Join(v.dir, relPath)
		blData, readErr := os.ReadFile(absPath)
		if readErr != nil {
			continue
		}
		blTitle := strings.TrimSuffix(filepath.Base(relPath), ".md")
		linked = append(linked, LinkedNote{
			Title:   blTitle,
			Path:    relPath,
			Content: string(blData),
		})
	}

	return primary, linked, nil
}

// Search finds notes whose title or content matches opts.Query or opts.Regex.
// Property filters embedded in opts.Query ([key:value]) are also applied.
// Returns results without context lines. For context-aware search use SearchWithContext.
func (v *Vault) Search(opts SearchOptions) ([]SearchResult, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	query := opts.Query
	regexParam := opts.Regex

	if query == "" && regexParam == "" {
		return nil, fmt.Errorf("search requires Query or Regex to be set")
	}

	// Compile regex if provided.
	var re *regexp.Regexp
	useRegex := regexParam != ""

	if useRegex {
		var compileErr error
		re, compileErr = regexp.Compile("(?i)" + regexParam)
		if compileErr != nil {
			return nil, fmt.Errorf("invalid regex %q: %v", regexParam, compileErr)
		}

		// If both query and regex provided, warn and use regex for text matching.
		if query != "" {
			fmt.Fprintf(os.Stderr, "vlt: both Query and Regex provided; Regex takes precedence for text matching\n")
		}
	}

	// Parse property filters from query.
	var textQuery string
	var filters map[string]string
	if query != "" {
		textQuery, filters = parseSearchQuery(query)
	} else {
		filters = make(map[string]string)
	}

	queryLower := strings.ToLower(textQuery)

	searchRoot := v.dir
	if opts.Path != "" {
		var pathErr error
		searchRoot, pathErr = safePath(v.dir, opts.Path)
		if pathErr != nil {
			return nil, fmt.Errorf("search path: %w", pathErr)
		}
		if _, err := os.Stat(searchRoot); os.IsNotExist(err) {
			return nil, fmt.Errorf("path filter %q not found in vault", opts.Path)
		}
	}

	hasTextQuery := useRegex || queryLower != ""
	hasFilters := len(filters) > 0

	if !hasTextQuery && !hasFilters {
		return nil, fmt.Errorf("search requires Query or Regex to be set")
	}

	var results []SearchResult

	err := filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if skipHiddenDir(path, d, searchRoot) {
			return filepath.SkipDir
		}

		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}

		title := strings.TrimSuffix(name, ".md")
		relPath, _ := filepath.Rel(v.dir, path)

		// Read file content (needed for both text search and property filters).
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)

		// Check property filters first if present.
		if hasFilters {
			yaml, _, hasFM := ExtractFrontmatter(content)
			if !hasFM {
				return nil // no frontmatter, can't match property filters
			}
			for k, fv := range filters {
				got, ok := FrontmatterGetValue(yaml, k)
				if !ok || !strings.EqualFold(got, fv) {
					return nil // filter doesn't match
				}
			}
		}

		// If no text query, property filters already passed.
		if !hasTextQuery {
			results = append(results, SearchResult{Title: title, RelPath: relPath})
			return nil
		}

		// Determine matches based on regex or substring.
		var titleMatches, contentMatches bool
		if useRegex {
			titleMatches = re.MatchString(title)
			contentMatches = re.MatchString(content)
		} else {
			titleMatches = strings.Contains(strings.ToLower(title), queryLower)
			contentMatches = strings.Contains(strings.ToLower(content), queryLower)
		}

		if !titleMatches && !contentMatches {
			return nil
		}

		results = append(results, SearchResult{Title: title, RelPath: relPath})
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// SearchWithContext finds notes matching opts.Query or opts.Regex and returns
// line-level matches with opts.ContextN surrounding lines on each side.
func (v *Vault) SearchWithContext(opts SearchOptions) ([]ContextMatch, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	query := opts.Query
	regexParam := opts.Regex

	if query == "" && regexParam == "" {
		return nil, fmt.Errorf("search requires Query or Regex to be set")
	}

	// Compile regex if provided.
	var re *regexp.Regexp
	useRegex := regexParam != ""

	if useRegex {
		var compileErr error
		re, compileErr = regexp.Compile("(?i)" + regexParam)
		if compileErr != nil {
			return nil, fmt.Errorf("invalid regex %q: %v", regexParam, compileErr)
		}

		if query != "" {
			fmt.Fprintf(os.Stderr, "vlt: both Query and Regex provided; Regex takes precedence for text matching\n")
		}
	}

	// Parse property filters from query.
	var textQuery string
	var filters map[string]string
	if query != "" {
		textQuery, filters = parseSearchQuery(query)
	} else {
		filters = make(map[string]string)
	}

	queryLower := strings.ToLower(textQuery)

	searchRoot := v.dir
	if opts.Path != "" {
		var pathErr error
		searchRoot, pathErr = safePath(v.dir, opts.Path)
		if pathErr != nil {
			return nil, fmt.Errorf("search path: %w", pathErr)
		}
		if _, err := os.Stat(searchRoot); os.IsNotExist(err) {
			return nil, fmt.Errorf("path filter %q not found in vault", opts.Path)
		}
	}

	hasTextQuery := useRegex || queryLower != ""
	hasFilters := len(filters) > 0

	if !hasTextQuery && !hasFilters {
		return nil, fmt.Errorf("search requires Query or Regex to be set")
	}

	contextN := opts.ContextN

	var contextResults []ContextMatch

	err := filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if skipHiddenDir(path, d, searchRoot) {
			return filepath.SkipDir
		}

		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}

		title := strings.TrimSuffix(name, ".md")
		relPath, _ := filepath.Rel(v.dir, path)

		// Read file content.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)

		// Check property filters first if present.
		if hasFilters {
			yaml, _, hasFM := ExtractFrontmatter(content)
			if !hasFM {
				return nil
			}
			for k, fv := range filters {
				got, ok := FrontmatterGetValue(yaml, k)
				if !ok || !strings.EqualFold(got, fv) {
					return nil
				}
			}
		}

		// Determine title/content matches.
		var titleMatches, contentMatches bool
		if useRegex {
			titleMatches = re.MatchString(title)
			contentMatches = re.MatchString(content)
		} else {
			titleMatches = strings.Contains(strings.ToLower(title), queryLower)
			contentMatches = strings.Contains(strings.ToLower(content), queryLower)
		}

		if !hasTextQuery {
			// Filters matched but no text query -- synthetic title match.
			contextResults = append(contextResults, ContextMatch{
				File:    relPath,
				Line:    0,
				Match:   title,
				Context: nil,
			})
			return nil
		}

		if !titleMatches && !contentMatches {
			return nil
		}

		// Context mode: find line-level matches.
		lines := strings.Split(content, "\n")
		var matchLineIdxs []int
		if useRegex {
			matchLineIdxs = findMatchLinesRegex(lines, re)
		} else {
			matchLineIdxs = findMatchLines(lines, textQuery)
		}

		if len(matchLineIdxs) > 0 {
			ranges := expandAndMerge(matchLineIdxs, contextN, len(lines))
			for _, r := range ranges {
				for i := r.start; i <= r.end; i++ {
					isMatch := false
					for _, m := range matchLineIdxs {
						if m == i {
							isMatch = true
							break
						}
					}
					if isMatch {
						ctxStart := i - contextN
						if ctxStart < 0 {
							ctxStart = 0
						}
						ctxEnd := i + contextN
						if ctxEnd >= len(lines) {
							ctxEnd = len(lines) - 1
						}
						var ctxLines []string
						for j := ctxStart; j <= ctxEnd; j++ {
							ctxLines = append(ctxLines, lines[j])
						}
						contextResults = append(contextResults, ContextMatch{
							File:    relPath,
							Line:    i + 1, // 1-based
							Match:   lines[i],
							Context: ctxLines,
						})
					}
				}
			}
		} else if titleMatches {
			// Title matched but no content match -- synthetic context match.
			contextResults = append(contextResults, ContextMatch{
				File:    relPath,
				Line:    0,
				Match:   title,
				Context: nil,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return contextResults, nil
}

// Create creates a new note at the given path within the vault.
// Returns ErrNoteExists if the note already exists.
// When timestamps is true (or VLT_TIMESTAMPS=1), created_at and updated_at
// are added to frontmatter.
func (v *Vault) Create(name, path, content string, silent, timestamps bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if name == "" || path == "" {
		return fmt.Errorf("create requires name and path")
	}

	fullPath, err := safePath(v.dir, path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}

	// Don't overwrite existing notes.
	if _, err := os.Stat(fullPath); err == nil {
		return ErrNoteExists
	}

	if timestampsEnabled(timestamps) {
		content = ensureTimestamps(content, true, time.Now())
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}

// Append adds content to the end of an existing note.
// When timestamps is true (or VLT_TIMESTAMPS=1), updated_at is refreshed.
func (v *Vault) Append(title, content string, timestamps bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = fmt.Fprint(f, content); err != nil {
		return err
	}

	if timestampsEnabled(timestamps) {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := ensureTimestamps(string(data), false, time.Now())
		return os.WriteFile(path, []byte(updated), 0644)
	}

	return nil
}

// Prepend inserts content at the top of a note, after frontmatter if present.
// When timestamps is true (or VLT_TIMESTAMPS=1), updated_at is refreshed.
func (v *Vault) Prepend(title, content string, timestamps bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(data)
	_, bodyStart, hasFM := ExtractFrontmatter(text)

	lines := strings.Split(text, "\n")
	var result string

	if hasFM && bodyStart <= len(lines) {
		before := strings.Join(lines[:bodyStart], "\n")
		after := strings.Join(lines[bodyStart:], "\n")
		result = before + "\n" + content + after
	} else {
		result = content + text
	}

	if timestampsEnabled(timestamps) {
		result = ensureTimestamps(result, false, time.Now())
	}

	return os.WriteFile(path, []byte(result), 0644)
}

// Write replaces the body content of an existing note, preserving frontmatter.
// If the note has no frontmatter, the entire file content is replaced.
// When timestamps is true (or VLT_TIMESTAMPS=1), updated_at is refreshed.
func (v *Vault) Write(title, content string, timestamps bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(data)
	_, bodyStart, hasFM := ExtractFrontmatter(text)

	var result string
	if hasFM {
		lines := strings.Split(text, "\n")
		frontmatter := strings.Join(lines[:bodyStart], "\n")
		result = frontmatter + "\n" + content
	} else {
		result = content
	}

	if timestampsEnabled(timestamps) {
		result = ensureTimestamps(result, false, time.Now())
	}

	return os.WriteFile(path, []byte(result), 0644)
}

// Patch performs surgical edits to a note: heading-targeted or line-targeted
// replace/delete. opts.Delete controls whether content is removed or replaced.
// When opts.Timestamps is true (or VLT_TIMESTAMPS=1), updated_at is refreshed.
func (v *Vault) Patch(title string, opts PatchOptions) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(data)
	lines := strings.Split(text, "\n")

	heading := opts.Heading
	lineSpec := opts.LineSpec

	if heading == "" && lineSpec == "" {
		return fmt.Errorf("patch requires Heading or LineSpec to be set")
	}

	content := opts.Content

	var result []string

	if heading != "" {
		// Heading-targeted patch.
		bounds, found := findSection(lines, heading)
		if !found {
			return fmt.Errorf("heading %q not found in %q", heading, title)
		}

		if opts.Delete {
			// Delete mode: remove heading + content.
			result = append(result, lines[:bounds.HeadingLine]...)
			result = append(result, lines[bounds.ContentEnd:]...)
		} else {
			// Replace mode: keep heading, replace content.
			result = append(result, lines[:bounds.ContentStart]...)
			if content != "" {
				contentLines := strings.Split(content, "\n")
				result = append(result, contentLines...)
			}
			result = append(result, lines[bounds.ContentEnd:]...)
		}
	} else {
		// Line-targeted patch.
		startLine, endLine, err := parseLineSpec(lineSpec)
		if err != nil {
			return err
		}

		// Validate range (1-based to 0-based).
		if startLine < 1 || endLine < startLine {
			return fmt.Errorf("invalid line specification: %s", lineSpec)
		}
		if startLine > len(lines) {
			return fmt.Errorf("line %d is beyond file length (%d lines); out of range", startLine, len(lines))
		}
		if endLine > len(lines) {
			return fmt.Errorf("line %d is beyond file length (%d lines); out of range", endLine, len(lines))
		}

		// Convert to 0-based.
		start := startLine - 1
		end := endLine // exclusive (endLine is 1-based, so endLine = 0-based + 1)

		if opts.Delete {
			result = append(result, lines[:start]...)
			result = append(result, lines[end:]...)
		} else {
			result = append(result, lines[:start]...)
			result = append(result, content)
			result = append(result, lines[end:]...)
		}
	}

	output := strings.Join(result, "\n")

	if timestampsEnabled(opts.Timestamps) {
		output = ensureTimestamps(output, false, time.Now())
	}

	return os.WriteFile(path, []byte(output), 0644)
}

// Move moves a note from one path to another within the vault.
// If the filename changes (rename, not just folder move), all wikilinks
// referencing the old title are updated vault-wide.
// Returns a MoveResult describing what was updated.
func (v *Vault) Move(from, to string) (MoveResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	fromPath, err := safePath(v.dir, from)
	if err != nil {
		return MoveResult{}, fmt.Errorf("move source: %w", err)
	}
	toPath, err := safePath(v.dir, to)
	if err != nil {
		return MoveResult{}, fmt.Errorf("move destination: %w", err)
	}

	if _, err := os.Stat(fromPath); os.IsNotExist(err) {
		return MoveResult{}, fmt.Errorf("source not found: %s", from)
	}

	if err := os.MkdirAll(filepath.Dir(toPath), 0755); err != nil {
		return MoveResult{}, err
	}

	oldTitle := strings.TrimSuffix(filepath.Base(from), ".md")
	newTitle := strings.TrimSuffix(filepath.Base(to), ".md")

	if err := os.Rename(fromPath, toPath); err != nil {
		return MoveResult{}, err
	}

	res := MoveResult{
		OldTitle: oldTitle,
		NewTitle: newTitle,
	}

	// If the filename changed, update wikilinks across the vault.
	if oldTitle != newTitle {
		count, err := updateVaultLinks(v.dir, oldTitle, newTitle)
		if err != nil {
			return res, fmt.Errorf("moved file but failed updating links: %w", err)
		}
		res.WikilinksUpdated = count
	}

	// Update markdown-style [text](path.md) links across the vault.
	mdCount, mdErr := updateVaultMdLinks(v.dir, from, to)
	if mdErr != nil {
		return res, fmt.Errorf("moved file but failed updating markdown links: %w", mdErr)
	}
	res.MdLinksUpdated = mdCount

	return res, nil
}

// Delete moves a note to .trash/ (or permanently deletes with permanent=true).
// Returns a human-readable message describing what happened.
func (v *Vault) Delete(title, notePath string, permanent bool) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	var fullPath string

	if notePath != "" {
		var pathErr error
		fullPath, pathErr = safePath(v.dir, notePath)
		if pathErr != nil {
			return "", fmt.Errorf("delete: %w", pathErr)
		}
	} else if title != "" {
		resolved, err := resolveNote(v.dir, title)
		if err != nil {
			return "", err
		}
		fullPath = resolved
	} else {
		return "", fmt.Errorf("delete requires file or path to be specified")
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", fullPath)
	}

	relPath, _ := filepath.Rel(v.dir, fullPath)

	if permanent {
		if err := os.Remove(fullPath); err != nil {
			return "", err
		}
		return fmt.Sprintf("deleted: %s", relPath), nil
	}

	trashDir := filepath.Join(v.dir, ".trash")
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return "", err
	}
	trashPath := filepath.Join(trashDir, filepath.Base(fullPath))
	if err := os.Rename(fullPath, trashPath); err != nil {
		return "", err
	}
	return fmt.Sprintf("trashed: %s -> .trash/%s", relPath, filepath.Base(fullPath)), nil
}

// Properties returns the YAML frontmatter block of a note (with --- delimiters).
func (v *Vault) Properties(title string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	fm := frontmatterReadAll(string(data))
	return fm, nil
}

// PropertySet sets or adds a YAML frontmatter property in a note.
func (v *Vault) PropertySet(title, name, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	// Find frontmatter boundaries (--- ... ---).
	fmStart, fmEnd := -1, -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if fmStart == -1 {
				fmStart = i
			} else {
				fmEnd = i
				break
			}
		}
	}

	if fmStart == -1 || fmEnd == -1 {
		return fmt.Errorf("no frontmatter found in %q", title)
	}

	// Look for existing property line.
	found := false
	prefix := name + ":"
	for i := fmStart + 1; i < fmEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, prefix) {
			lines[i] = fmt.Sprintf("%s: %s", name, value)
			found = true
			break
		}
	}

	// If not found, insert before closing ---.
	if !found {
		newLine := fmt.Sprintf("%s: %s", name, value)
		lines = append(lines[:fmEnd+1], lines[fmEnd:]...)
		lines[fmEnd] = newLine
	}

	result := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(result), 0644)
}

// PropertyRemove removes a property from a note's frontmatter.
func (v *Vault) PropertyRemove(title, name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(data)
	updated := frontmatterRemoveKey(text, name)

	if updated == text {
		return fmt.Errorf("property %q not found in %q", name, title)
	}

	return os.WriteFile(path, []byte(updated), 0644)
}

// Backlinks finds all notes that contain wikilinks to the given title.
func (v *Vault) Backlinks(title string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return FindBacklinks(v.dir, title)
}

// Links lists outgoing wikilinks from a note, reporting which resolve
// and which are broken.
func (v *Vault) Links(title string) ([]LinkInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	links := ParseWikilinks(string(data))
	if len(links) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var results []LinkInfo
	for _, link := range links {
		if seen[link.Title] {
			continue
		}
		seen[link.Title] = true

		resolved, resolveErr := resolveNote(v.dir, link.Title)
		if resolveErr != nil {
			results = append(results, LinkInfo{Target: link.Title, Path: "", Broken: true})
		} else {
			relPath, _ := filepath.Rel(v.dir, resolved)
			results = append(results, LinkInfo{Target: link.Title, Path: relPath, Broken: false})
		}
	}

	return results, nil
}

// Orphans finds notes that have no incoming wikilinks or embeds.
func (v *Vault) Orphans() ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Collect all note titles.
	type noteInfo struct {
		relPath string
		title   string
		aliases []string
	}
	var notes []noteInfo

	filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, v.dir) {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}

		title := strings.TrimSuffix(name, ".md")
		relPath, _ := filepath.Rel(v.dir, path)

		info := noteInfo{relPath: relPath, title: title}

		data, err := os.ReadFile(path)
		if err == nil {
			yaml, _, hasFM := ExtractFrontmatter(string(data))
			if hasFM {
				info.aliases = FrontmatterGetList(yaml, "aliases")
			}
		}

		notes = append(notes, info)
		return nil
	})

	// Collect all referenced titles (from wikilinks and embeds).
	referenced := make(map[string]bool)

	filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, v.dir) {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		for _, link := range ParseWikilinks(string(data)) {
			referenced[strings.ToLower(link.Title)] = true
		}
		return nil
	})

	// Find orphans: notes whose title AND aliases are all unreferenced.
	var orphans []string
	for _, note := range notes {
		if referenced[strings.ToLower(note.title)] {
			continue
		}
		aliasReferenced := false
		for _, a := range note.aliases {
			if referenced[strings.ToLower(a)] {
				aliasReferenced = true
				break
			}
		}
		if !aliasReferenced {
			orphans = append(orphans, note.relPath)
		}
	}

	sort.Strings(orphans)
	return orphans, nil
}

// Unresolved finds all broken wikilinks across the vault.
func (v *Vault) Unresolved() ([]UnresolvedLink, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Build sets of resolvable titles and aliases.
	titles := make(map[string]bool)
	aliases := make(map[string]bool)

	filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, v.dir) {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}

		title := strings.TrimSuffix(name, ".md")
		titles[strings.ToLower(title)] = true

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		yaml, _, hasFM := ExtractFrontmatter(string(data))
		if hasFM {
			for _, alias := range FrontmatterGetList(yaml, "aliases") {
				aliases[strings.ToLower(alias)] = true
			}
		}
		return nil
	})

	// Find links that don't resolve.
	var results []UnresolvedLink
	seenTargets := make(map[string]bool)

	filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, v.dir) {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(v.dir, path)

		for _, link := range ParseWikilinks(string(data)) {
			lower := strings.ToLower(link.Title)
			if seenTargets[lower] {
				continue
			}
			if !titles[lower] && !aliases[lower] {
				seenTargets[lower] = true
				results = append(results, UnresolvedLink{Target: link.Title, Source: relPath})
			}
		}
		return nil
	})

	return results, nil
}

// Files lists files in the vault, optionally filtered by folder and extension.
func (v *Vault) Files(folder, ext string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if ext == "" {
		ext = "md"
	}

	searchRoot := v.dir
	if folder != "" {
		var pathErr error
		searchRoot, pathErr = safePath(v.dir, folder)
		if pathErr != nil {
			return nil, fmt.Errorf("files folder: %w", pathErr)
		}
		if _, err := os.Stat(searchRoot); os.IsNotExist(err) {
			return nil, fmt.Errorf("folder not found: %s", folder)
		}
	}

	var files []string

	filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, searchRoot) {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, "."+ext) {
			return nil
		}

		relPath, _ := filepath.Rel(v.dir, path)
		files = append(files, relPath)
		return nil
	})

	sort.Strings(files)
	return files, nil
}

// URI generates an obsidian:// URI for a note resolved by title.
// The URI format is: obsidian://open?vault=VAULT&file=PATH[&heading=H][&block=B]
// Vault name and file path are URL-encoded. The .md extension is stripped.
// Path separators use forward slash (/).
func (v *Vault) URI(vaultName, title, heading, block string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	path, err := resolveNote(v.dir, title)
	if err != nil {
		return "", err
	}

	// Get relative path from vault root, strip .md extension.
	relPath, _ := filepath.Rel(v.dir, path)
	relPath = strings.TrimSuffix(relPath, ".md")

	// Normalize path separators to forward slash (for Windows compatibility).
	relPath = filepath.ToSlash(relPath)

	// URL-encode vault name and file path.
	encodedVault := encodeURIComponent(vaultName)
	encodedFile := encodeURIComponent(relPath)

	uri := fmt.Sprintf("obsidian://open?vault=%s&file=%s", encodedVault, encodedFile)

	// Optional heading fragment.
	if heading != "" {
		uri += "&heading=" + encodeURIComponent(heading)
	}

	// Optional block fragment.
	if block != "" {
		uri += "&block=" + encodeURIComponent(block)
	}

	return uri, nil
}
