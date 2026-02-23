package vlt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMomentToGoFormat(t *testing.T) {
	tests := []struct {
		moment string
		want   string
	}{
		{"YYYY-MM-DD", "2006-01-02"},
		{"YY-M-D", "06-1-2"},
		{"YYYY/MM/DD", "2006/01/02"},
		{"dddd, MMMM D, YYYY", "Monday, January 2, 2006"},
		{"ddd MMM DD", "Mon Jan 02"},
		{"YYYY-MM-DD HH:mm", "2006-01-02 15:04"},
	}

	for _, tt := range tests {
		t.Run(tt.moment, func(t *testing.T) {
			got := MomentToGoFormat(tt.moment)
			if got != tt.want {
				t.Errorf("MomentToGoFormat(%q) = %q, want %q", tt.moment, got, tt.want)
			}
		})
	}
}

func TestLoadDailyConfig_Default(t *testing.T) {
	vaultDir := t.TempDir()

	config := loadDailyConfig(vaultDir)

	if config.Format != "2006-01-02" {
		t.Errorf("default format = %q, want %q", config.Format, "2006-01-02")
	}
	if config.Folder != "" {
		t.Errorf("default folder = %q, want empty", config.Folder)
	}
	if config.Template != "" {
		t.Errorf("default template = %q, want empty", config.Template)
	}
}

func TestLoadDailyConfig_FromFile(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "daily-notes.json"),
		[]byte(`{"folder":"daily","format":"YYYY/MM/DD","template":"_templates/daily"}`),
		0644,
	)

	config := loadDailyConfig(vaultDir)

	if config.Folder != "daily" {
		t.Errorf("folder = %q, want %q", config.Folder, "daily")
	}
	if config.Format != "2006/01/02" {
		t.Errorf("format = %q, want %q", config.Format, "2006/01/02")
	}
	if config.Template != "_templates/daily" {
		t.Errorf("template = %q, want %q", config.Template, "_templates/daily")
	}
}

func TestLoadDailyConfig_PeriodicNotes(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian", "plugins", "periodic-notes"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "plugins", "periodic-notes", "data.json"),
		[]byte(`{"daily":{"folder":"journal","format":"YYYY-MM-DD"}}`),
		0644,
	)

	config := loadDailyConfig(vaultDir)

	if config.Folder != "journal" {
		t.Errorf("folder = %q, want %q", config.Folder, "journal")
	}
	if config.Format != "2006-01-02" {
		t.Errorf("format = %q, want %q", config.Format, "2006-01-02")
	}
}

func TestDaily_CreateNew(t *testing.T) {
	vaultDir := t.TempDir()

	v := &Vault{dir: vaultDir}
	result, err := v.Daily("")
	if err != nil {
		t.Fatalf("Daily create: %v", err)
	}

	if !result.Created {
		t.Error("expected Created=true for new note")
	}

	// Should create today's note
	today := time.Now().Format("2006-01-02")
	notePath := filepath.Join(vaultDir, today+".md")

	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("daily note not created: %v", err)
	}

	if !strings.Contains(string(data), "# "+today) {
		t.Errorf("daily note content: %q, expected heading with date", string(data))
	}
}

func TestDaily_ReadExisting(t *testing.T) {
	vaultDir := t.TempDir()

	today := time.Now().Format("2006-01-02")
	content := "# Existing Note\n\nSome content.\n"
	os.WriteFile(
		filepath.Join(vaultDir, today+".md"),
		[]byte(content),
		0644,
	)

	v := &Vault{dir: vaultDir}
	result, err := v.Daily("")
	if err != nil {
		t.Fatalf("Daily read: %v", err)
	}

	if result.Created {
		t.Error("expected Created=false for existing note")
	}
	if result.Content != content {
		t.Errorf("got Content %q, want %q", result.Content, content)
	}
}

func TestDaily_SpecificDate(t *testing.T) {
	vaultDir := t.TempDir()

	v := &Vault{dir: vaultDir}
	result, err := v.Daily("2025-06-15")
	if err != nil {
		t.Fatalf("Daily specific date: %v", err)
	}

	if !result.Created {
		t.Error("expected Created=true for new note")
	}

	notePath := filepath.Join(vaultDir, "2025-06-15.md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("daily note not created: %v", err)
	}

	if !strings.Contains(string(data), "# 2025-06-15") {
		t.Errorf("daily note content: %q", string(data))
	}
}

func TestDaily_WithTemplate(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.MkdirAll(filepath.Join(vaultDir, "_templates"), 0755)

	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "daily-notes.json"),
		[]byte(`{"template":"_templates/daily"}`),
		0644,
	)

	os.WriteFile(
		filepath.Join(vaultDir, "_templates", "daily.md"),
		[]byte("---\ndate: {{date}}\n---\n\n# {{title}}\n\n## Tasks\n\n## Notes\n"),
		0644,
	)

	v := &Vault{dir: vaultDir}
	result, err := v.Daily("2025-03-20")
	if err != nil {
		t.Fatalf("Daily with template: %v", err)
	}

	if !result.Created {
		t.Error("expected Created=true")
	}

	notePath := filepath.Join(vaultDir, "2025-03-20.md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("daily note not created: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "date: 2025-03-20") {
		t.Errorf("template date not substituted: %q", got)
	}
	if !strings.Contains(got, "# 2025-03-20") {
		t.Errorf("template title not substituted: %q", got)
	}
	if !strings.Contains(got, "## Tasks") {
		t.Errorf("template structure not preserved: %q", got)
	}
}

func TestDaily_WithFolder(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "daily-notes.json"),
		[]byte(`{"folder":"journal"}`),
		0644,
	)

	v := &Vault{dir: vaultDir}
	_, err := v.Daily("2025-06-15")
	if err != nil {
		t.Fatalf("Daily with folder: %v", err)
	}

	notePath := filepath.Join(vaultDir, "journal", "2025-06-15.md")
	if _, err := os.Stat(notePath); os.IsNotExist(err) {
		t.Errorf("daily note not created in folder")
	}
}

func TestDaily_InvalidDate(t *testing.T) {
	vaultDir := t.TempDir()

	v := &Vault{dir: vaultDir}
	_, err := v.Daily("not-a-date")
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}
