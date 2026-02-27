package vlt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Task represents a parsed checkbox item from a note.
type Task struct {
	Text string `json:"text"` // task text after the checkbox
	Done bool   `json:"done"` // true if [x] or [X]
	Line int    `json:"line"` // 1-based line number
	File string `json:"file"` // relative path (when searching vault-wide)
}

// TaskOptions parameterises a Tasks call.
type TaskOptions struct {
	File    string // single-file mode: resolve by title
	Path    string // vault-wide mode: limit to subfolder
	Done    bool   // filter: only completed tasks
	Pending bool   // filter: only incomplete tasks
}

// taskPattern matches markdown checkboxes: - [ ] text or - [x] text
// Allows leading whitespace/tabs for nesting.
var taskPattern = regexp.MustCompile(`(?m)^[\t ]*- \[([ xX])\] (.+)$`)

// ParseTasks extracts all checkbox items from text.
func ParseTasks(text string) []Task {
	lines := strings.Split(text, "\n")
	var tasks []Task

	for i, line := range lines {
		m := taskPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		tasks = append(tasks, Task{
			Text: m[2],
			Done: m[1] == "x" || m[1] == "X",
			Line: i + 1,
		})
	}
	return tasks
}

// Tasks lists tasks (checkboxes) from one note or across the vault.
// Supports filters via opts.Done and opts.Pending.
// Supports opts.Path to limit search to a subfolder.
func (v *Vault) Tasks(opts TaskOptions) ([]Task, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Single file mode
	if opts.File != "" {
		path, err := resolveNote(v.dir, opts.File)
		if err != nil {
			return nil, err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		relPath, _ := filepath.Rel(v.dir, path)
		tasks := ParseTasks(string(data))
		tasks = filterTasks(tasks, opts.Done, opts.Pending)

		for i := range tasks {
			tasks[i].File = relPath
		}

		return tasks, nil
	}

	// Vault-wide mode
	searchRoot := v.dir
	if opts.Path != "" {
		var pathErr error
		searchRoot, pathErr = safePath(v.dir, opts.Path)
		if pathErr != nil {
			return nil, fmt.Errorf("tasks path: %w", pathErr)
		}
		if _, err := os.Stat(searchRoot); os.IsNotExist(err) {
			return nil, fmt.Errorf("path filter %q not found in vault", opts.Path)
		}
	}

	var allTasks []Task

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

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(v.dir, path)
		tasks := ParseTasks(string(data))

		for i := range tasks {
			tasks[i].File = relPath
		}

		allTasks = append(allTasks, tasks...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	allTasks = filterTasks(allTasks, opts.Done, opts.Pending)
	return allTasks, nil
}

// filterTasks applies done/pending filters.
func filterTasks(tasks []Task, done, pending bool) []Task {
	if !done && !pending {
		return tasks
	}

	var result []Task
	for _, t := range tasks {
		if done && t.Done {
			result = append(result, t)
		}
		if pending && !t.Done {
			result = append(result, t)
		}
	}
	return result
}
