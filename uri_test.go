package vlt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Unit tests ---

func TestURIBasic(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Hello.md"), []byte("# Hello\n"), 0644)

	out, err := v.URI("TestVault", "Hello", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=TestVault&file=Hello"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURIWithSpaces(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Session Operating Mode.md"), []byte("# Session\n"), 0644)

	out, err := v.URI("My Vault", "Session Operating Mode", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=My%20Vault&file=Session%20Operating%20Mode"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURIWithSubfolder(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.MkdirAll(filepath.Join(vaultDir, "methodology"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "methodology", "Session Operating Mode.md"), []byte("# Session\n"), 0644)

	out, err := v.URI("Claude", "Session Operating Mode", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=methodology%2FSession%20Operating%20Mode"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURIWithHeading(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Note.md"), []byte("# Note\n## Section A\ncontent\n"), 0644)

	out, err := v.URI("Claude", "Note", "Section A", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=Note&heading=Section%20A"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURIWithBlock(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Note.md"), []byte("# Note\ncontent ^block-123\n"), 0644)

	out, err := v.URI("Claude", "Note", "", "block-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=Note&block=block-123"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURISpecialCharacters(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.MkdirAll(filepath.Join(vaultDir, "notes & ideas"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "notes & ideas", "C++ Patterns.md"), []byte("# C++\n"), 0644)

	out, err := v.URI("Dev & Notes", "C++ Patterns", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Vault name, folder, and filename should all be URL-encoded
	if !strings.HasPrefix(out, "obsidian://open?vault=") {
		t.Fatalf("unexpected prefix: %q", out)
	}
	// Verify & is encoded in vault name
	if strings.Contains(out, "vault=Dev & Notes") {
		t.Error("vault name not URL-encoded: contains literal '&'")
	}
	// Verify the URI contains encoded ampersand
	if !strings.Contains(out, "vault=Dev%20%26%20Notes") {
		t.Errorf("vault name incorrectly encoded, got %q", out)
	}
	// Verify ++ is encoded in file path
	if !strings.Contains(out, "C%2B%2B") {
		t.Errorf("C++ not properly encoded in path, got %q", out)
	}
	// Verify folder & is encoded
	if !strings.Contains(out, "notes%20%26%20ideas") {
		t.Errorf("folder with & not properly encoded, got %q", out)
	}
}

func TestURIRequiresFile(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}

	_, err := v.URI("Claude", "", "", "")
	if err == nil {
		t.Fatal("expected error for missing file parameter")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestURIHeadingWithSpecialChars(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Doc.md"), []byte("# Doc\n## Q&A Section\ncontent\n"), 0644)

	out, err := v.URI("Claude", "Doc", "Q&A Section", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=Doc&heading=Q%26A%20Section"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURIBlockWithSpecialChars(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Doc.md"), []byte("# Doc\ncontent ^my-block\n"), 0644)

	out, err := v.URI("Claude", "Doc", "", "my-block")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=Doc&block=my-block"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// --- Integration tests (real files, real vault structure) ---

func TestURIIntegration(t *testing.T) {
	// Full integration: create a realistic vault, create a note, generate URI
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.MkdirAll(filepath.Join(vaultDir, "methodology"), 0755)

	noteContent := "---\ntype: concept\nstatus: active\n---\n\n# Test Concept\n\nSome content here.\n"
	os.WriteFile(filepath.Join(vaultDir, "methodology", "Test Concept.md"), []byte(noteContent), 0644)

	out, err := v.URI("Claude", "Test Concept", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=methodology%2FTest%20Concept"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}

	// Verify the URI starts with the correct scheme
	if !strings.HasPrefix(out, "obsidian://open?") {
		t.Errorf("URI should start with obsidian://open?, got %q", out)
	}
}

func TestURISubfolderIntegration(t *testing.T) {
	// Integration: deeply nested note
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.MkdirAll(filepath.Join(vaultDir, "projects", "active"), 0755)

	noteContent := "---\ntype: project\n---\n\n# Deep Note\n"
	os.WriteFile(filepath.Join(vaultDir, "projects", "active", "Deep Note.md"), []byte(noteContent), 0644)

	out, err := v.URI("Work Vault", "Deep Note", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Work%20Vault&file=projects%2Factive%2FDeep%20Note"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestURINoteNotFound(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}

	// Write some note so the vault isn't empty
	os.WriteFile(filepath.Join(vaultDir, "Existing.md"), []byte("# Existing\n"), 0644)

	_, err := v.URI("Claude", "Nonexistent Note", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent note")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestURIHeadingAndBlockTogether(t *testing.T) {
	// When both heading and block are provided, both should appear in the URI
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "Note.md"), []byte("# Note\n## Section\ncontent ^blk\n"), 0644)

	out, err := v.URI("Claude", "Note", "Section", "blk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both parameters should be present
	if !strings.Contains(out, "&heading=Section") {
		t.Errorf("URI should contain heading parameter, got %q", out)
	}
	if !strings.Contains(out, "&block=blk") {
		t.Errorf("URI should contain block parameter, got %q", out)
	}
}

func TestURIRootLevelNote(t *testing.T) {
	// Integration: note at vault root (no subfolder)
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir}
	os.WriteFile(filepath.Join(vaultDir, "README.md"), []byte("# README\n"), 0644)

	out, err := v.URI("Claude", "README", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "obsidian://open?vault=Claude&file=README"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
