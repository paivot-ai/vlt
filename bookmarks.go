package vlt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// bookmarksFile represents the top-level structure of .obsidian/bookmarks.json.
type bookmarksFile struct {
	Items []bookmark `json:"items"`
}

// bookmark represents a single bookmark entry. Groups contain nested items.
type bookmark struct {
	Type  string     `json:"type"`
	Ctime int64      `json:"ctime"`
	Path  string     `json:"path,omitempty"`
	Title string     `json:"title,omitempty"`
	Items []bookmark `json:"items,omitempty"`
}

// bookmarksPath returns the filesystem path to the bookmarks.json file.
func bookmarksPath(vaultDir string) string {
	return filepath.Join(vaultDir, ".obsidian", "bookmarks.json")
}

// loadBookmarks reads and parses .obsidian/bookmarks.json.
// Returns an empty bookmarksFile (no error) if the file does not exist.
func loadBookmarks(vaultDir string) (bookmarksFile, error) {
	path := bookmarksPath(vaultDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return bookmarksFile{Items: []bookmark{}}, nil
		}
		return bookmarksFile{}, err
	}

	var bm bookmarksFile
	if err := json.Unmarshal(data, &bm); err != nil {
		return bookmarksFile{}, fmt.Errorf("cannot parse bookmarks.json: %w", err)
	}

	if bm.Items == nil {
		bm.Items = []bookmark{}
	}
	return bm, nil
}

// saveBookmarks writes the bookmarksFile to .obsidian/bookmarks.json.
// Creates the .obsidian directory if it does not exist.
func saveBookmarks(vaultDir string, bm *bookmarksFile) error {
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		return fmt.Errorf("cannot create .obsidian directory: %w", err)
	}

	data, err := json.MarshalIndent(bm, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal bookmarks: %w", err)
	}

	return os.WriteFile(bookmarksPath(vaultDir), data, 0644)
}

// flattenBookmarks recursively collects all file-type bookmark paths,
// descending into groups.
func flattenBookmarks(items []bookmark) []string {
	var paths []string
	for _, item := range items {
		switch item.Type {
		case "file":
			paths = append(paths, item.Path)
		case "group":
			paths = append(paths, flattenBookmarks(item.Items)...)
		}
	}
	return paths
}

// containsBookmark checks whether a path is already bookmarked,
// recursing into groups.
func containsBookmark(items []bookmark, path string) bool {
	for _, item := range items {
		if item.Type == "file" && item.Path == path {
			return true
		}
		if item.Type == "group" && containsBookmark(item.Items, path) {
			return true
		}
	}
	return false
}

// addBookmark adds a file bookmark to the top-level items array.
// Returns false if the path is already bookmarked (no-op).
func addBookmark(bm *bookmarksFile, path string) bool {
	if containsBookmark(bm.Items, path) {
		return false
	}

	bm.Items = append(bm.Items, bookmark{
		Type:  "file",
		Ctime: time.Now().UnixMilli(),
		Path:  path,
	})
	return true
}

// removeBookmark removes a file bookmark matching the given path,
// searching recursively into groups. Returns false if not found.
func removeBookmark(bm *bookmarksFile, path string) bool {
	return removeFromItems(&bm.Items, path)
}

// removeFromItems removes a bookmark matching path from a slice,
// recursing into groups. Returns true if found and removed.
func removeFromItems(items *[]bookmark, path string) bool {
	for i, item := range *items {
		if item.Type == "file" && item.Path == path {
			*items = append((*items)[:i], (*items)[i+1:]...)
			return true
		}
		if item.Type == "group" {
			if removeFromItems(&(*items)[i].Items, path) {
				return true
			}
		}
	}
	return false
}

// Bookmarks lists all bookmarked file paths (flat, recursing into groups).
func (v *Vault) Bookmarks() ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	bm, err := loadBookmarks(v.dir)
	if err != nil {
		return nil, err
	}

	paths := flattenBookmarks(bm.Items)
	if paths == nil {
		paths = []string{}
	}

	return paths, nil
}

// BookmarksAdd adds a bookmark for a note resolved by title.
// Returns a human-readable message (e.g. "bookmarked: path" or "already bookmarked: path").
func (v *Vault) BookmarksAdd(title string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	notePath, err := resolveNote(v.dir, title)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(v.dir, notePath)
	if err != nil {
		return "", err
	}

	bm, err := loadBookmarks(v.dir)
	if err != nil {
		return "", err
	}

	if !addBookmark(&bm, relPath) {
		return fmt.Sprintf("already bookmarked: %s", relPath), nil
	}

	if err := saveBookmarks(v.dir, &bm); err != nil {
		return "", err
	}

	return fmt.Sprintf("bookmarked: %s", relPath), nil
}

// BookmarksRemove removes a bookmark for a note resolved by title.
func (v *Vault) BookmarksRemove(title string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check that bookmarks.json exists (error on remove when missing)
	bmPath := bookmarksPath(v.dir)
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		return fmt.Errorf("no bookmarks file found in vault")
	}

	notePath, err := resolveNote(v.dir, title)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(v.dir, notePath)
	if err != nil {
		return err
	}

	bm, err := loadBookmarks(v.dir)
	if err != nil {
		return err
	}

	if !removeBookmark(&bm, relPath) {
		return fmt.Errorf("bookmark not found for %q (%s)", title, relPath)
	}

	return saveBookmarks(v.dir, &bm)
}
