package vlt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DailyResult is returned by Daily and reports what happened.
type DailyResult struct {
	RelPath string // vault-relative path of the daily note
	Content string // file content (if the note already existed)
	Created bool   // true if a new note was created
}

// dailyConfig holds the daily note configuration.
type dailyConfig struct {
	Folder   string // subfolder for daily notes (default: "")
	Format   string // Go time format (default: "2006-01-02")
	Template string // template note path (default: "")
}

// loadDailyConfig reads Obsidian's daily note settings from the vault's
// .obsidian directory. Falls back to defaults.
func loadDailyConfig(vaultDir string) dailyConfig {
	config := dailyConfig{
		Format: "2006-01-02",
	}

	// Try core daily-notes plugin first
	corePath := filepath.Join(vaultDir, ".obsidian", "daily-notes.json")
	if data, err := os.ReadFile(corePath); err == nil {
		parseDailyJSON(data, &config)
		return config
	}

	// Try periodic-notes plugin
	periodicPath := filepath.Join(vaultDir, ".obsidian", "plugins", "periodic-notes", "data.json")
	if data, err := os.ReadFile(periodicPath); err == nil {
		parseDailyJSON(data, &config)
		return config
	}

	return config
}

// parseDailyJSON extracts daily note settings from an Obsidian plugin config.
func parseDailyJSON(data []byte, config *dailyConfig) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	if folder, ok := raw["folder"].(string); ok && folder != "" {
		config.Folder = folder
	}
	if format, ok := raw["format"].(string); ok && format != "" {
		config.Format = MomentToGoFormat(format)
	}
	if template, ok := raw["template"].(string); ok && template != "" {
		config.Template = template
	}

	// periodic-notes nests under "daily" key
	if daily, ok := raw["daily"].(map[string]any); ok {
		if folder, ok := daily["folder"].(string); ok && folder != "" {
			config.Folder = folder
		}
		if format, ok := daily["format"].(string); ok && format != "" {
			config.Format = MomentToGoFormat(format)
		}
		if template, ok := daily["template"].(string); ok && template != "" {
			config.Template = template
		}
	}
}

// MomentToGoFormat translates common Moment.js date format tokens to Go's
// reference time format. Uses a two-pass approach with placeholders to avoid
// earlier replacements being corrupted by later ones (e.g., "a" inside "January").
func MomentToGoFormat(moment string) string {
	// Order matters: longest tokens first to avoid partial matches
	replacements := []struct {
		moment string
		goFmt  string
	}{
		{"YYYY", "2006"},
		{"YY", "06"},
		{"MMMM", "January"},
		{"MMM", "Jan"},
		{"MM", "01"},
		{"M", "1"},
		{"DD", "02"},
		{"D", "2"},
		{"dddd", "Monday"},
		{"ddd", "Mon"},
		{"dd", "Mo"},
		{"HH", "15"},
		{"hh", "03"},
		{"mm", "04"},
		{"ss", "05"},
		{"A", "PM"},
		{"a", "pm"},
	}

	// Pass 1: replace Moment tokens with unique placeholders
	result := moment
	for i, r := range replacements {
		placeholder := fmt.Sprintf("\x00%d\x00", i)
		result = strings.ReplaceAll(result, r.moment, placeholder)
	}

	// Pass 2: replace placeholders with Go format strings
	for i, r := range replacements {
		placeholder := fmt.Sprintf("\x00%d\x00", i)
		result = strings.ReplaceAll(result, placeholder, r.goFmt)
	}

	return result
}

// Daily creates or reads a daily note.
// With no date parameter (empty string), uses today.
// With date="2025-01-15", uses that date.
func (v *Vault) Daily(dateStr string) (DailyResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	config := loadDailyConfig(v.dir)

	// Determine the date
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return DailyResult{}, fmt.Errorf("invalid date format %q, expected YYYY-MM-DD", dateStr)
		}
	} else {
		date = time.Now()
	}

	// Compute filename from config format
	filename := date.Format(config.Format) + ".md"
	relPath := filename
	if config.Folder != "" {
		relPath = filepath.Join(config.Folder, filename)
	}

	fullPath := filepath.Join(v.dir, relPath)

	// If note exists, read and return it
	if data, err := os.ReadFile(fullPath); err == nil {
		return DailyResult{
			RelPath: relPath,
			Content: string(data),
			Created: false,
		}, nil
	}

	// Note doesn't exist -- create it
	var content string
	if config.Template != "" {
		tmplPath := filepath.Join(v.dir, config.Template)
		if !strings.HasSuffix(tmplPath, ".md") {
			tmplPath += ".md"
		}
		if tmplData, err := os.ReadFile(tmplPath); err == nil {
			content = string(tmplData)
			// Replace common template variables
			content = strings.ReplaceAll(content, "{{date}}", date.Format("2006-01-02"))
			content = strings.ReplaceAll(content, "{{title}}", date.Format(config.Format))
		}
	}

	if content == "" {
		content = fmt.Sprintf("# %s\n\n", date.Format(config.Format))
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return DailyResult{}, err
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return DailyResult{}, err
	}

	return DailyResult{
		RelPath: relPath,
		Content: content,
		Created: true,
	}, nil
}
