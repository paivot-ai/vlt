package vlt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// vaultIndex caches file locations for note and link resolution so commands
// walk the vault once instead of once per lookup. It is built lazily: the
// directory walk happens on first lookup, and the alias map (which requires
// reading every note) only when a title lookup misses.
//
// All rel paths are slash-separated and vault-relative. "First wins" follows
// filepath.WalkDir lexical order, matching the previous walk-based resolution.
type vaultIndex struct {
	aliasesBuilt bool

	byExactName     map[string]string // "Note.md" -> rel path (md files, case-sensitive)
	byLowerTitle    map[string]string // "note" -> rel path (md files)
	byLowerFile     map[string]string // "photo.png" -> rel path (all files)
	byLowerRelNoExt map[string]string // "sub/note" -> rel path (md files)
	byLowerRel      map[string]string // "sub/photo.png" -> rel path (all files)
	byLowerAlias    map[string]string // frontmatter alias -> rel path (md files)
	mdRel           []string          // rel paths of all md files in walk order
}

// indexMu guards lazy construction and invalidation of Vault.idx. It is
// separate from Vault.mu because read operations build the index while
// holding only the read lock.
type indexedVault struct {
	mu  sync.Mutex
	idx *vaultIndex
}

// index returns the vault index, building the walk-level maps if needed.
func (v *Vault) index() *vaultIndex {
	v.idxState.mu.Lock()
	defer v.idxState.mu.Unlock()
	if v.idxState.idx == nil {
		v.idxState.idx = buildIndex(v.dir)
	}
	return v.idxState.idx
}

// indexWithAliases returns the vault index, additionally building the alias
// map (which reads the frontmatter of every note).
func (v *Vault) indexWithAliases() *vaultIndex {
	v.idxState.mu.Lock()
	defer v.idxState.mu.Unlock()
	if v.idxState.idx == nil {
		v.idxState.idx = buildIndex(v.dir)
	}
	idx := v.idxState.idx
	if !idx.aliasesBuilt {
		buildAliasIndex(v.dir, idx)
	}
	return idx
}

// invalidateIndex discards the cached index. Called after any mutation that
// can change file names, paths, or frontmatter aliases.
func (v *Vault) invalidateIndex() {
	v.idxState.mu.Lock()
	v.idxState.idx = nil
	v.idxState.mu.Unlock()
}

// buildIndex walks the vault once and records file locations. No file
// contents are read.
func buildIndex(dir string) *vaultIndex {
	idx := &vaultIndex{
		byExactName:     make(map[string]string),
		byLowerTitle:    make(map[string]string),
		byLowerFile:     make(map[string]string),
		byLowerRelNoExt: make(map[string]string),
		byLowerRel:      make(map[string]string),
		byLowerAlias:    make(map[string]string),
	}

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, dir) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()

		lowerRel := strings.ToLower(rel)
		if _, ok := idx.byLowerRel[lowerRel]; !ok {
			idx.byLowerRel[lowerRel] = rel
		}
		lowerName := strings.ToLower(name)
		if _, ok := idx.byLowerFile[lowerName]; !ok {
			idx.byLowerFile[lowerName] = rel
		}

		if strings.HasSuffix(name, ".md") {
			idx.mdRel = append(idx.mdRel, rel)
			if _, ok := idx.byExactName[name]; !ok {
				idx.byExactName[name] = rel
			}
			lowerTitle := strings.TrimSuffix(lowerName, ".md")
			if _, ok := idx.byLowerTitle[lowerTitle]; !ok {
				idx.byLowerTitle[lowerTitle] = rel
			}
			lowerRelNoExt := strings.TrimSuffix(lowerRel, ".md")
			if _, ok := idx.byLowerRelNoExt[lowerRelNoExt]; !ok {
				idx.byLowerRelNoExt[lowerRelNoExt] = rel
			}
		}
		return nil
	})

	return idx
}

// buildAliasIndex reads the frontmatter of every note and records aliases.
func buildAliasIndex(dir string, idx *vaultIndex) {
	for _, rel := range idx.mdRel {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		yaml, _, hasFM := ExtractFrontmatter(string(data))
		if !hasFM {
			continue
		}
		for _, alias := range FrontmatterGetList(yaml, "aliases") {
			la := strings.ToLower(alias)
			if _, ok := idx.byLowerAlias[la]; !ok {
				idx.byLowerAlias[la] = rel
			}
		}
	}
	idx.aliasesBuilt = true
}

// resolve finds a note by title, vault-relative path, or frontmatter alias,
// returning its absolute path.
//
// Resolution order (matching Obsidian):
//  1. Vault-relative path ("sub/Note" or "sub/Note.md") if the title
//     contains a slash and the file exists.
//  2. Exact filename match (<title>.md), first in walk order.
//  3. Frontmatter alias, case-insensitive, first in walk order.
func (v *Vault) resolve(title string) (string, error) {
	if strings.ContainsRune(title, '/') {
		rel := title
		if !strings.HasSuffix(rel, ".md") {
			rel += ".md"
		}
		if p, err := safePath(v.dir, filepath.FromSlash(rel)); err == nil {
			if st, statErr := os.Stat(p); statErr == nil && !st.IsDir() {
				return p, nil
			}
		}
	}

	idx := v.index()
	if rel, ok := idx.byExactName[title+".md"]; ok {
		return filepath.Join(v.dir, filepath.FromSlash(rel)), nil
	}

	idx = v.indexWithAliases()
	if rel, ok := idx.byLowerAlias[strings.ToLower(title)]; ok {
		return filepath.Join(v.dir, filepath.FromSlash(rel)), nil
	}

	return "", fmt.Errorf("note %q not found in vault", title)
}

// resolveLink resolves a wikilink target the way Obsidian does: by note
// title, by vault-relative path (with or without extension), by exact
// filename for attachments and embeds, or by frontmatter alias.
// Returns the vault-relative path of the target and whether it resolved.
func (v *Vault) resolveLink(target string) (string, bool) {
	norm := strings.ToLower(filepath.ToSlash(strings.TrimSpace(target)))
	if norm == "" {
		return "", false
	}

	idx := v.index()
	if rel, ok := idx.byLowerTitle[norm]; ok {
		return rel, true
	}
	if rel, ok := idx.byLowerRelNoExt[norm]; ok {
		return rel, true
	}
	if rel, ok := idx.byLowerFile[norm]; ok {
		return rel, true
	}
	if rel, ok := idx.byLowerRel[norm]; ok {
		return rel, true
	}

	idx = v.indexWithAliases()
	if rel, ok := idx.byLowerAlias[norm]; ok {
		return rel, true
	}

	return "", false
}
