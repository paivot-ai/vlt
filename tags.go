package vlt

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// tagPattern matches inline tags: #tag preceded by whitespace or start of line.
// Tags contain Unicode letters, digits, underscores, hyphens, and forward
// slashes (for hierarchical tags like #project/backend).
var tagPattern = regexp.MustCompile(`(?:^|[\s(])#([\p{L}\p{N}_/-]+)`)

// ParseInlineTags extracts inline #tags from text.
// Skips pure-numeric tags (Obsidian requires at least one letter).
// Content inside inert zones (fenced code blocks, etc.) is masked
// before extraction so those tags are ignored.
func ParseInlineTags(text string) []string {
	text = MaskInertContent(text)
	matches := tagPattern.FindAllStringSubmatch(text, -1)
	var tags []string
	for _, m := range matches {
		tag := m[1]
		if hasLetter(tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// AllNoteTags returns all tags from a note (inline body + frontmatter),
// lowercased and deduplicated.
func AllNoteTags(text string) []string {
	seen := make(map[string]bool)
	var result []string

	yaml, bodyStart, hasFM := ExtractFrontmatter(text)
	if hasFM {
		for _, t := range FrontmatterGetList(yaml, "tags") {
			lower := strings.ToLower(t)
			if !seen[lower] {
				seen[lower] = true
				result = append(result, lower)
			}
		}
	}

	// Parse inline tags from body only (skip frontmatter)
	body := text
	if hasFM {
		lines := strings.Split(text, "\n")
		if bodyStart < len(lines) {
			body = strings.Join(lines[bodyStart:], "\n")
		}
	}

	for _, t := range ParseInlineTags(body) {
		lower := strings.ToLower(t)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, lower)
		}
	}

	return result
}

// Tags lists all tags in the vault with their note counts.
// Returns a sorted tag list and a counts map.
// Supports sortBy="count" to sort by frequency (default: alphabetical).
func (v *Vault) Tags(sortBy string) ([]string, map[string]int, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	tagCounts := make(map[string]int)

	err := filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
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

		for _, tag := range AllNoteTags(string(data)) {
			tagCounts[tag]++
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if len(tagCounts) == 0 {
		return nil, tagCounts, nil
	}

	tags := make([]string, 0, len(tagCounts))
	for t := range tagCounts {
		tags = append(tags, t)
	}

	if sortBy == "count" {
		sort.Slice(tags, func(i, j int) bool {
			if tagCounts[tags[i]] != tagCounts[tags[j]] {
				return tagCounts[tags[i]] > tagCounts[tags[j]]
			}
			return tags[i] < tags[j]
		})
	} else {
		sort.Strings(tags)
	}

	return tags, tagCounts, nil
}

// Tag finds notes that have a specific tag or any subtag of it.
// Matches case-insensitively, consistent with Obsidian.
func (v *Vault) Tag(tagName string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	tagName = strings.TrimPrefix(tagName, "#")
	tagLower := strings.ToLower(tagName)

	var results []string

	err := filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
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

		for _, t := range AllNoteTags(string(data)) {
			if t == tagLower || strings.HasPrefix(t, tagLower+"/") {
				relPath, _ := filepath.Rel(v.dir, path)
				results = append(results, relPath)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(results)
	return results, nil
}
