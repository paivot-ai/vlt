package vlt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

func TestDiscoverTemplateFolder(t *testing.T) {
	vaultDir := t.TempDir()

	// Set up .obsidian/templates.json with a configured folder
	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "templates.json"),
		[]byte(`{"folder":"my-templates"}`),
		0644,
	)
	// Create the folder so it exists
	os.MkdirAll(filepath.Join(vaultDir, "my-templates"), 0755)

	folder, err := discoverTemplateFolder(vaultDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "my-templates" {
		t.Errorf("got %q, want %q", folder, "my-templates")
	}
}

func TestDiscoverTemplateFolderDefault(t *testing.T) {
	vaultDir := t.TempDir()

	// No .obsidian config, but create a templates/ folder
	os.MkdirAll(filepath.Join(vaultDir, "templates"), 0755)

	folder, err := discoverTemplateFolder(vaultDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "templates" {
		t.Errorf("got %q, want %q", folder, "templates")
	}
}

func TestTemplatesNoFolderError(t *testing.T) {
	vaultDir := t.TempDir()

	// No config, no templates/ folder
	_, err := discoverTemplateFolder(vaultDir)
	if err == nil {
		t.Fatal("expected error when no template folder configured or found")
	}
	if !strings.Contains(err.Error(), "no template folder configured or found") {
		t.Errorf("error message = %q, want to contain %q", err.Error(), "no template folder configured or found")
	}
}

func TestTemplateVariableSubstitution(t *testing.T) {
	now := time.Date(2026, 2, 19, 14, 30, 0, 0, time.UTC)
	input := "# {{title}}\nDate: {{date}}\nTime: {{time}}\n"

	got := substituteTemplateVars(input, "My Note", now)

	if !strings.Contains(got, "# My Note") {
		t.Errorf("title not substituted: %q", got)
	}
	if !strings.Contains(got, "Date: 2026-02-19") {
		t.Errorf("date not substituted: %q", got)
	}
	if !strings.Contains(got, "Time: 14:30") {
		t.Errorf("time not substituted: %q", got)
	}
}

func TestTemplateCustomDateFormat(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	input := "Created: {{date:YYYY-MM-DD}}\nYear: {{date:YYYY}}\nShort: {{date:MM/DD}}\n"

	got := substituteTemplateVars(input, "Test", now)

	if !strings.Contains(got, "Created: 2026-03-15") {
		t.Errorf("custom date YYYY-MM-DD not substituted: %q", got)
	}
	if !strings.Contains(got, "Year: 2026") {
		t.Errorf("custom date YYYY not substituted: %q", got)
	}
	if !strings.Contains(got, "Short: 03/15") {
		t.Errorf("custom date MM/DD not substituted: %q", got)
	}
}

func TestTemplateCustomTimeFormat(t *testing.T) {
	now := time.Date(2026, 1, 1, 9, 5, 0, 0, time.UTC)
	input := "Now: {{time:HH:mm}}\nFull: {{time:HH:mm:ss}}\n"

	got := substituteTemplateVars(input, "Test", now)

	if !strings.Contains(got, "Now: 09:05") {
		t.Errorf("custom time HH:mm not substituted: %q", got)
	}
	if !strings.Contains(got, "Full: 09:05:00") {
		t.Errorf("custom time HH:mm:ss not substituted: %q", got)
	}
}

func TestTemplateNoVariables(t *testing.T) {
	now := time.Now()
	input := "# Plain note\n\nNo variables here.\n"

	got := substituteTemplateVars(input, "Test", now)

	if got != input {
		t.Errorf("content changed: got %q, want %q", got, input)
	}
}

func TestTemplateUnknownVariable(t *testing.T) {
	now := time.Now()
	input := "# {{title}}\n\nUnknown: {{foo}}\nAnother: {{bar:baz}}\n"

	got := substituteTemplateVars(input, "Test", now)

	if !strings.Contains(got, "{{foo}}") {
		t.Errorf("unknown variable {{foo}} was removed: %q", got)
	}
	if !strings.Contains(got, "{{bar:baz}}") {
		t.Errorf("unknown variable {{bar:baz}} was removed: %q", got)
	}
	if !strings.Contains(got, "# Test") {
		t.Errorf("known variable {{title}} not substituted: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (real files, no mocks)
// ---------------------------------------------------------------------------

func TestTemplatesListIntegration(t *testing.T) {
	vaultDir := t.TempDir()

	// Set up template folder via config
	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "templates.json"),
		[]byte(`{"folder":"templates"}`),
		0644,
	)

	// Create template files
	tmplDir := filepath.Join(vaultDir, "templates")
	os.MkdirAll(tmplDir, 0755)
	os.WriteFile(filepath.Join(tmplDir, "Meeting Notes.md"), []byte("# {{title}}"), 0644)
	os.WriteFile(filepath.Join(tmplDir, "Daily.md"), []byte("# {{date}}"), 0644)
	os.WriteFile(filepath.Join(tmplDir, "not-a-template.txt"), []byte("skip me"), 0644)

	v := &Vault{dir: vaultDir}
	templates, err := v.Templates()
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}

	// Should list .md files, sorted, not .txt
	found := map[string]bool{}
	for _, tmpl := range templates {
		found[tmpl] = true
	}

	if !found["Daily.md"] {
		t.Errorf("missing Daily.md in templates: %v", templates)
	}
	if !found["Meeting Notes.md"] {
		t.Errorf("missing Meeting Notes.md in templates: %v", templates)
	}
	if found["not-a-template.txt"] {
		t.Errorf("non-md file listed in templates: %v", templates)
	}

	// Verify sorted order: Daily before Meeting Notes
	if len(templates) >= 2 {
		dailyIdx := -1
		meetingIdx := -1
		for i, tmpl := range templates {
			if tmpl == "Daily.md" {
				dailyIdx = i
			}
			if tmpl == "Meeting Notes.md" {
				meetingIdx = i
			}
		}
		if dailyIdx > meetingIdx {
			t.Errorf("templates not sorted: Daily.md at %d, Meeting Notes.md at %d", dailyIdx, meetingIdx)
		}
	}
}

func TestTemplatesApplyIntegration(t *testing.T) {
	vaultDir := t.TempDir()

	// Set up template folder
	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "templates.json"),
		[]byte(`{"folder":"templates"}`),
		0644,
	)

	tmplDir := filepath.Join(vaultDir, "templates")
	os.MkdirAll(tmplDir, 0755)
	os.WriteFile(filepath.Join(tmplDir, "Meeting Notes.md"),
		[]byte("---\ntype: meeting\n---\n# {{title}}\n\nDate: {{date}}\nTime: {{time}}\n\n## Attendees\n\n## Notes\n"),
		0644,
	)

	v := &Vault{dir: vaultDir}
	if err := v.TemplatesApply("Meeting Notes", "Q1 Planning", "meetings/Q1 Planning.md"); err != nil {
		t.Fatalf("TemplatesApply: %v", err)
	}

	// Read the created note
	data, err := os.ReadFile(filepath.Join(vaultDir, "meetings", "Q1 Planning.md"))
	if err != nil {
		t.Fatalf("note not created: %v", err)
	}

	content := string(data)

	// Verify variable substitution
	if !strings.Contains(content, "# Q1 Planning") {
		t.Errorf("title not substituted: %q", content)
	}

	today := time.Now().Format("2006-01-02")
	if !strings.Contains(content, "Date: "+today) {
		t.Errorf("date not substituted: %q", content)
	}

	// Time should be in HH:MM format
	if !strings.Contains(content, "Time: ") {
		t.Errorf("time not substituted: %q", content)
	}

	// Structure should be preserved
	if !strings.Contains(content, "## Attendees") {
		t.Errorf("template structure not preserved: %q", content)
	}
	if !strings.Contains(content, "type: meeting") {
		t.Errorf("frontmatter not preserved: %q", content)
	}
}

func TestTemplatesApplyExistingNote(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "templates.json"),
		[]byte(`{"folder":"templates"}`),
		0644,
	)

	tmplDir := filepath.Join(vaultDir, "templates")
	os.MkdirAll(tmplDir, 0755)
	os.WriteFile(filepath.Join(tmplDir, "Simple.md"), []byte("# {{title}}"), 0644)

	// Create the target note first
	os.MkdirAll(filepath.Join(vaultDir, "notes"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "notes", "Existing.md"), []byte("# Existing"), 0644)

	v := &Vault{dir: vaultDir}
	err := v.TemplatesApply("Simple", "Existing", "notes/Existing.md")
	if err == nil {
		t.Fatal("expected error when applying to existing note")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

func TestTemplatesApplyNotFound(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "templates.json"),
		[]byte(`{"folder":"templates"}`),
		0644,
	)
	os.MkdirAll(filepath.Join(vaultDir, "templates"), 0755)

	v := &Vault{dir: vaultDir}
	err := v.TemplatesApply("Nonexistent", "Test", "test.md")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "template") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to mention template not found", err.Error())
	}
}

func TestTemplatesApplyCreatesDirectories(t *testing.T) {
	vaultDir := t.TempDir()

	os.MkdirAll(filepath.Join(vaultDir, ".obsidian"), 0755)
	os.WriteFile(
		filepath.Join(vaultDir, ".obsidian", "templates.json"),
		[]byte(`{"folder":"templates"}`),
		0644,
	)

	tmplDir := filepath.Join(vaultDir, "templates")
	os.MkdirAll(tmplDir, 0755)
	os.WriteFile(filepath.Join(tmplDir, "Simple.md"), []byte("# {{title}}"), 0644)

	v := &Vault{dir: vaultDir}
	if err := v.TemplatesApply("Simple", "Deep Note", "deeply/nested/dir/Deep Note.md"); err != nil {
		t.Fatalf("TemplatesApply failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(vaultDir, "deeply", "nested", "dir", "Deep Note.md"))
	if err != nil {
		t.Fatalf("note not created at deep path: %v", err)
	}

	if !strings.Contains(string(data), "# Deep Note") {
		t.Errorf("title not substituted in deep note: %q", string(data))
	}
}

func TestTemplatesListNoFolderError(t *testing.T) {
	vaultDir := t.TempDir()

	// No config, no templates/ folder
	v := &Vault{dir: vaultDir}
	_, err := v.Templates()
	if err == nil {
		t.Fatal("expected error when no template folder configured or found")
	}
	if !strings.Contains(err.Error(), "no template folder configured or found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no template folder configured or found")
	}
}

func TestTemplatesApplyNoFolderError(t *testing.T) {
	vaultDir := t.TempDir()

	// No config, no templates/ folder
	v := &Vault{dir: vaultDir}
	err := v.TemplatesApply("Something", "Test", "test.md")
	if err == nil {
		t.Fatal("expected error when no template folder configured or found")
	}
	if !strings.Contains(err.Error(), "no template folder configured or found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no template folder configured or found")
	}
}
