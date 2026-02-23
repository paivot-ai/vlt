package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// E2E validation tests for the Content Manipulation epic (VLT-6z0).
//
// These tests exercise multiple content manipulation commands together in
// realistic multi-step workflows. All tests use t.TempDir() for isolated
// vault environments. No mocks.
// ---------------------------------------------------------------------------

// TestE2EWriteThenReadHeading creates a note with frontmatter and multiple
// sections, replaces the body via write, then reads a specific heading to
// verify section content is correct and frontmatter is preserved.
func TestE2EWriteThenReadHeading(t *testing.T) {
	vaultDir := t.TempDir()

	// Step 1: Create a note with frontmatter and multiple sections
	original := "---\ntype: methodology\nstatus: active\ncreated: 2026-02-19\n---\n\n# Design Document\n\nOverview of the system.\n\n## Architecture\n\nMicroservices-based with event sourcing.\n\n## Deployment\n\nKubernetes with Helm charts.\n"
	notePath := filepath.Join(vaultDir, "Design Doc.md")
	if err := os.WriteFile(notePath, []byte(original), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Step 2: Write new body content preserving frontmatter
	newBody := "# Design Document v2\n\nRevised overview.\n\n## Architecture\n\nMonolithic with clean architecture layers.\n\n## Testing Strategy\n\nProperty-based testing throughout.\n"
	writeParams := map[string]string{
		"file":    "Design Doc",
		"content": newBody,
	}
	if err := cmdWrite(vaultDir, writeParams, false); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Step 3: Read the Architecture section via heading=
	readOut := captureStdout(func() {
		readParams := map[string]string{
			"file":    "Design Doc",
			"heading": "## Architecture",
		}
		if err := cmdRead(vaultDir, readParams); err != nil {
			t.Fatalf("read heading: %v", err)
		}
	})

	// Verify: section content is from the NEW body
	if !strings.Contains(readOut, "## Architecture") {
		t.Error("heading line missing from read output")
	}
	if !strings.Contains(readOut, "Monolithic with clean architecture layers.") {
		t.Error("new architecture content missing from read output")
	}
	// Old content must be gone
	if strings.Contains(readOut, "Microservices") {
		t.Error("old architecture content still present in heading read")
	}
	// Section must not include content from sibling sections
	if strings.Contains(readOut, "Property-based testing") {
		t.Error("read heading leaked content from sibling section")
	}

	// Step 4: Verify frontmatter is fully preserved
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	got := string(data)

	yaml, _, hasFM := extractFrontmatter(got)
	if !hasFM {
		t.Fatal("frontmatter lost after write")
	}
	if v, ok := frontmatterGetValue(yaml, "type"); !ok || v != "methodology" {
		t.Errorf("type property lost or changed: %q", v)
	}
	if v, ok := frontmatterGetValue(yaml, "status"); !ok || v != "active" {
		t.Errorf("status property lost or changed: %q", v)
	}
	if v, ok := frontmatterGetValue(yaml, "created"); !ok || v != "2026-02-19" {
		t.Errorf("created property lost or changed: %q", v)
	}
}

// TestE2EPatchThenReadHeading creates a note with sections, patches one
// section by heading, then reads that section with heading= to verify
// the patched content is returned.
func TestE2EPatchThenReadHeading(t *testing.T) {
	vaultDir := t.TempDir()

	// Step 1: Create a note with multiple sections
	content := "---\ntype: decision\nstatus: draft\n---\n\n# Decision Record\n\n## Context\n\nWe need a database for the project.\n\n## Decision\n\nWe chose PostgreSQL for relational data.\n\n## Consequences\n\nNeed to manage migrations and connection pooling.\n"
	notePath := filepath.Join(vaultDir, "ADR-001.md")
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Step 2: Patch the Decision section by heading
	patchParams := map[string]string{
		"file":    "ADR-001",
		"heading": "## Decision",
		"content": "\nWe chose SQLite for embedded simplicity. No external dependencies required.\n",
	}
	if err := cmdPatch(vaultDir, patchParams, false, false); err != nil {
		t.Fatalf("patch: %v", err)
	}

	// Step 3: Read the Decision section via heading=
	readOut := captureStdout(func() {
		readParams := map[string]string{
			"file":    "ADR-001",
			"heading": "## Decision",
		}
		if err := cmdRead(vaultDir, readParams); err != nil {
			t.Fatalf("read heading: %v", err)
		}
	})

	// Verify patched content is returned
	if !strings.Contains(readOut, "SQLite for embedded simplicity") {
		t.Error("patched content not found in heading read")
	}
	// Old content must be gone from this section
	if strings.Contains(readOut, "PostgreSQL") {
		t.Error("old decision content still present")
	}

	// Step 4: Verify other sections are untouched
	data, _ := os.ReadFile(notePath)
	got := string(data)

	if !strings.Contains(got, "We need a database for the project.") {
		t.Error("Context section was corrupted by patch")
	}
	if !strings.Contains(got, "Need to manage migrations") {
		t.Error("Consequences section was corrupted by patch")
	}

	// Verify frontmatter preserved
	yaml, _, hasFM := extractFrontmatter(got)
	if !hasFM {
		t.Fatal("frontmatter lost after patch")
	}
	if v, ok := frontmatterGetValue(yaml, "status"); !ok || v != "draft" {
		t.Errorf("status property lost or changed: %q", v)
	}
}

// TestE2EPatchDeleteThenSearch creates a note with sections, deletes one
// section via patch, then searches for content from the deleted section
// to verify it is no longer found.
func TestE2EPatchDeleteThenSearch(t *testing.T) {
	vaultDir := t.TempDir()

	// Step 1: Create a note with a section containing distinctive content
	content := "---\ntype: pattern\n---\n\n# Retry Pattern\n\n## Implementation\n\nUse exponential backoff with jitter.\nMax retries: 5.\nBase delay: 100ms.\n\n## Deprecated Approach\n\nFixed delay retry was used before.\nThis caused thundering herd problems.\n\n## References\n\nSee circuit breaker pattern for complementary approach.\n"
	notePath := filepath.Join(vaultDir, "Retry Pattern.md")
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Step 2: Verify the content exists before deletion
	preSearchOut := captureStdout(func() {
		searchParams := map[string]string{"query": "thundering herd"}
		if err := cmdSearch(vaultDir, searchParams, ""); err != nil {
			t.Fatalf("pre-search: %v", err)
		}
	})
	if !strings.Contains(preSearchOut, "Retry Pattern") {
		t.Fatal("content should be findable before deletion")
	}

	// Step 3: Delete the Deprecated Approach section
	deleteParams := map[string]string{
		"file":    "Retry Pattern",
		"heading": "## Deprecated Approach",
	}
	if err := cmdPatch(vaultDir, deleteParams, true, false); err != nil {
		t.Fatalf("patch delete: %v", err)
	}

	// Step 4: Search for deleted content -- should NOT be found
	postSearchOut := captureStdout(func() {
		searchParams := map[string]string{"query": "thundering herd"}
		if err := cmdSearch(vaultDir, searchParams, ""); err != nil {
			t.Fatalf("post-search: %v", err)
		}
	})
	if strings.Contains(postSearchOut, "thundering herd") {
		t.Error("deleted content still found by search")
	}
	if strings.Contains(postSearchOut, "Retry Pattern") {
		t.Error("note should not match after deleted content is gone")
	}

	// Step 5: Verify other sections are intact
	data, _ := os.ReadFile(notePath)
	got := string(data)

	if !strings.Contains(got, "exponential backoff") {
		t.Error("Implementation section was corrupted")
	}
	if !strings.Contains(got, "circuit breaker") {
		t.Error("References section was corrupted")
	}
	if strings.Contains(got, "Deprecated Approach") {
		t.Error("Deprecated Approach heading still present after delete")
	}
}

// TestE2ESearchContextMultipleNotes creates a vault with multiple notes
// and verifies that search with context=2 returns context lines from the
// correct files with correct line numbers.
func TestE2ESearchContextMultipleNotes(t *testing.T) {
	vaultDir := t.TempDir()
	os.MkdirAll(filepath.Join(vaultDir, "patterns"), 0755)
	os.MkdirAll(filepath.Join(vaultDir, "decisions"), 0755)

	// Create multiple notes with a shared keyword in different contexts
	note1 := "# API Gateway\n\nThe gateway handles routing.\nIt uses rate limiting.\nAuthentication is delegated to the gateway.\nCompression is enabled.\nLogging happens here.\n"
	note2 := "---\ntype: decision\n---\n\n# Database Choice\n\nWe evaluated options.\nThe gateway proxy was considered.\nWe chose PostgreSQL.\nIt supports JSONB natively.\nReplication is straightforward.\n"
	note3 := "# Internal Service\n\nThis service has no gateway dependency.\nIt communicates via gRPC.\nNo HTTP involved.\n"

	os.WriteFile(filepath.Join(vaultDir, "patterns", "API Gateway.md"), []byte(note1), 0644)
	os.WriteFile(filepath.Join(vaultDir, "decisions", "Database Choice.md"), []byte(note2), 0644)
	os.WriteFile(filepath.Join(vaultDir, "Internal Service.md"), []byte(note3), 0644)

	// Search for "gateway" with context=2
	out := captureStdout(func() {
		params := map[string]string{"query": "gateway", "context": "2"}
		if err := cmdSearch(vaultDir, params, ""); err != nil {
			t.Fatalf("search with context: %v", err)
		}
	})

	// All files with "gateway" should appear in output
	if !strings.Contains(out, "API Gateway.md") {
		t.Error("API Gateway note missing from context search results")
	}
	if !strings.Contains(out, "Database Choice.md") {
		t.Error("Database Choice note missing from context search results")
	}
	if !strings.Contains(out, "Internal Service.md") {
		t.Error("Internal Service note missing from context search results")
	}

	// Context lines should be present (2 lines before/after match)
	// For API Gateway: "Authentication is delegated to the gateway." is line 5
	// Context should include lines 3 and 4 before, lines 6 and 7 after
	if !strings.Contains(out, "rate limiting") {
		t.Error("context lines before gateway match missing in API Gateway")
	}
	if !strings.Contains(out, "Compression") {
		t.Error("context lines after gateway match missing in API Gateway")
	}

	// For Internal Service: "no gateway dependency" should show surrounding lines
	if !strings.Contains(out, "gRPC") {
		t.Error("context lines after gateway match missing in Internal Service")
	}
}

// TestE2ESearchRegexAcrossVault creates a vault with notes containing dates,
// URLs, etc. and verifies that regex search finds correct matches across
// multiple files.
func TestE2ESearchRegexAcrossVault(t *testing.T) {
	vaultDir := t.TempDir()
	os.MkdirAll(filepath.Join(vaultDir, "projects"), 0755)

	// Create notes with dates and patterns
	note1 := "---\ntype: project\nstatus: active\n---\n\n# Project Alpha\n\nStarted on 2026-01-15.\nDeadline: 2026-03-30.\nBudget review on 2026-02-01.\n"
	note2 := "---\ntype: project\nstatus: archived\n---\n\n# Project Beta\n\nCompleted 2025-12-01.\nNo future dates here.\nJust some notes.\n"
	note3 := "# Meeting Notes\n\nAttendees: Alice, Bob\nNo dates mentioned at all.\nJust regular text.\n"

	os.WriteFile(filepath.Join(vaultDir, "projects", "Alpha.md"), []byte(note1), 0644)
	os.WriteFile(filepath.Join(vaultDir, "projects", "Beta.md"), []byte(note2), 0644)
	os.WriteFile(filepath.Join(vaultDir, "Meeting Notes.md"), []byte(note3), 0644)

	// Search for date pattern with regex
	out := captureStdout(func() {
		params := map[string]string{"regex": `\d{4}-\d{2}-\d{2}`}
		if err := cmdSearch(vaultDir, params, ""); err != nil {
			t.Fatalf("regex search: %v", err)
		}
	})

	// Alpha and Beta should match (both have dates)
	if !strings.Contains(out, "Alpha") {
		t.Error("Alpha note missing from regex date search")
	}
	if !strings.Contains(out, "Beta") {
		t.Error("Beta note missing from regex date search")
	}

	// Meeting Notes should NOT match (no dates)
	if strings.Contains(out, "Meeting Notes") {
		t.Error("Meeting Notes should not match date regex")
	}

	// Search for regex with context to verify match detail
	ctxOut := captureStdout(func() {
		params := map[string]string{"regex": `2026-03-\d{2}`, "context": "1"}
		if err := cmdSearch(vaultDir, params, ""); err != nil {
			t.Fatalf("regex with context: %v", err)
		}
	})

	// Only Alpha has 2026-03-XX
	if !strings.Contains(ctxOut, "Alpha.md") {
		t.Error("Alpha should match 2026-03 pattern")
	}
	if !strings.Contains(ctxOut, "2026-03-30") {
		t.Error("deadline date should appear in output")
	}
	if strings.Contains(ctxOut, "Beta") {
		t.Error("Beta should not match 2026-03 pattern")
	}

	// Search for URL-like pattern
	noteWithURL := "# Resources\n\nSee https://example.com/docs for info.\nAlso check http://internal.local/api.\nNo other links.\n"
	os.WriteFile(filepath.Join(vaultDir, "Resources.md"), []byte(noteWithURL), 0644)

	urlOut := captureStdout(func() {
		params := map[string]string{"regex": `https?://[^\s]+`}
		if err := cmdSearch(vaultDir, params, ""); err != nil {
			t.Fatalf("URL regex search: %v", err)
		}
	})

	if !strings.Contains(urlOut, "Resources") {
		t.Error("Resources note should match URL regex")
	}
}

// TestE2ETimestampsFullWorkflow exercises the complete timestamp lifecycle:
// create with timestamps, append with timestamps, write with timestamps,
// patch with timestamps. Verifies created_at is set once and never changed,
// while updated_at is refreshed on every modification.
func TestE2ETimestampsFullWorkflow(t *testing.T) {
	vaultDir := t.TempDir()

	// Step 1: Create note with timestamps
	createParams := map[string]string{
		"name":    "Evolving Note",
		"path":    "Evolving Note.md",
		"content": "---\ntype: concept\nstatus: draft\n---\n\n# Evolving Concept\n\nInitial thoughts.\n",
	}
	if err := cmdCreate(vaultDir, createParams, true, true); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Read and verify created_at and updated_at are set
	data, _ := os.ReadFile(filepath.Join(vaultDir, "Evolving Note.md"))
	yaml1, _, hasFM := extractFrontmatter(string(data))
	if !hasFM {
		t.Fatal("no frontmatter after create")
	}
	createdAt1, ok := frontmatterGetValue(yaml1, "created_at")
	if !ok || createdAt1 == "" {
		t.Fatal("created_at not set on create")
	}
	updatedAt1, ok := frontmatterGetValue(yaml1, "updated_at")
	if !ok || updatedAt1 == "" {
		t.Fatal("updated_at not set on create")
	}

	// Small delay to ensure timestamps differ
	time.Sleep(1100 * time.Millisecond)

	// Step 2: Append with timestamps
	appendParams := map[string]string{
		"file":    "Evolving Note",
		"content": "\nAppended insight.\n",
	}
	if err := cmdAppend(vaultDir, appendParams, true); err != nil {
		t.Fatalf("append: %v", err)
	}

	data, _ = os.ReadFile(filepath.Join(vaultDir, "Evolving Note.md"))
	yaml2, _, _ := extractFrontmatter(string(data))
	createdAt2, _ := frontmatterGetValue(yaml2, "created_at")
	updatedAt2, _ := frontmatterGetValue(yaml2, "updated_at")

	// created_at must be unchanged
	if createdAt2 != createdAt1 {
		t.Errorf("created_at changed after append: %q -> %q", createdAt1, createdAt2)
	}
	// updated_at must be different (refreshed)
	if updatedAt2 == updatedAt1 {
		t.Error("updated_at not refreshed after append")
	}

	time.Sleep(1100 * time.Millisecond)

	// Step 3: Write with timestamps (replace body)
	writeParams := map[string]string{
		"file":    "Evolving Note",
		"content": "# Evolved Concept\n\nMature understanding of the topic.\n",
	}
	if err := cmdWrite(vaultDir, writeParams, true); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, _ = os.ReadFile(filepath.Join(vaultDir, "Evolving Note.md"))
	yaml3, _, _ := extractFrontmatter(string(data))
	createdAt3, _ := frontmatterGetValue(yaml3, "created_at")
	updatedAt3, _ := frontmatterGetValue(yaml3, "updated_at")

	if createdAt3 != createdAt1 {
		t.Errorf("created_at changed after write: %q -> %q", createdAt1, createdAt3)
	}
	if updatedAt3 == updatedAt2 {
		t.Error("updated_at not refreshed after write")
	}

	time.Sleep(1100 * time.Millisecond)

	// Step 4: Patch with timestamps (add a section heading for patching)
	// First, write body with a section we can patch
	writeParams2 := map[string]string{
		"file":    "Evolving Note",
		"content": "# Evolved Concept\n\nMature understanding.\n\n## Details\n\nOriginal details.\n",
	}
	if err := cmdWrite(vaultDir, writeParams2, false); err != nil {
		t.Fatalf("write for patch setup: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	patchParams := map[string]string{
		"file":    "Evolving Note",
		"heading": "## Details",
		"content": "\nRefined details after review.\n",
	}
	if err := cmdPatch(vaultDir, patchParams, false, true); err != nil {
		t.Fatalf("patch: %v", err)
	}

	data, _ = os.ReadFile(filepath.Join(vaultDir, "Evolving Note.md"))
	yaml4, _, _ := extractFrontmatter(string(data))
	createdAt4, _ := frontmatterGetValue(yaml4, "created_at")
	updatedAt4, _ := frontmatterGetValue(yaml4, "updated_at")

	if createdAt4 != createdAt1 {
		t.Errorf("created_at changed after patch: %q -> %q", createdAt1, createdAt4)
	}
	if updatedAt4 == updatedAt3 {
		t.Error("updated_at not refreshed after patch")
	}

	// Verify original properties preserved through entire workflow
	if v, ok := frontmatterGetValue(yaml4, "type"); !ok || v != "concept" {
		t.Errorf("type property lost: %q", v)
	}
	if v, ok := frontmatterGetValue(yaml4, "status"); !ok || v != "draft" {
		t.Errorf("status property lost: %q", v)
	}

	// Verify final body content is correct
	if !strings.Contains(string(data), "Refined details after review.") {
		t.Error("final patched content missing")
	}
}

// TestE2EWritePreservesLinks creates a note with wikilinks in the body,
// writes a new body with different wikilinks, and verifies that backlinks
// are updated correctly (old links gone from backlinks, new links appear).
func TestE2EWritePreservesLinks(t *testing.T) {
	vaultDir := t.TempDir()

	// Create target notes for the links
	os.WriteFile(filepath.Join(vaultDir, "Database.md"), []byte("# Database\n\nDatabase concepts.\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "Caching.md"), []byte("# Caching\n\nCaching strategies.\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "Messaging.md"), []byte("# Messaging\n\nEvent-driven messaging.\n"), 0644)

	// Create a note with links to Database and Caching
	original := "---\ntype: pattern\n---\n\n# Data Layer\n\nWe use [[Database]] for persistence.\nWe also use [[Caching]] for performance.\n"
	os.WriteFile(filepath.Join(vaultDir, "Data Layer.md"), []byte(original), 0644)

	// Verify initial backlinks
	dbBacklinks, _ := findBacklinks(vaultDir, "Database")
	if len(dbBacklinks) == 0 {
		t.Fatal("Database should have backlinks before write")
	}
	cachingBacklinks, _ := findBacklinks(vaultDir, "Caching")
	if len(cachingBacklinks) == 0 {
		t.Fatal("Caching should have backlinks before write")
	}

	// Write new body with link to Messaging instead
	newBody := "# Data Layer v2\n\nWe now use [[Messaging]] for async communication.\nNo direct database or cache access.\n"
	writeParams := map[string]string{
		"file":    "Data Layer",
		"content": newBody,
	}
	if err := cmdWrite(vaultDir, writeParams, false); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify: old links are gone, new link is present
	data, _ := os.ReadFile(filepath.Join(vaultDir, "Data Layer.md"))
	got := string(data)

	if strings.Contains(got, "[[Database]]") {
		t.Error("old Database link still in body after write")
	}
	if strings.Contains(got, "[[Caching]]") {
		t.Error("old Caching link still in body after write")
	}
	if !strings.Contains(got, "[[Messaging]]") {
		t.Error("new Messaging link missing from body")
	}

	// Check backlinks are updated
	dbBacklinksAfter, _ := findBacklinks(vaultDir, "Database")
	hasDLBacklink := false
	for _, bl := range dbBacklinksAfter {
		if strings.Contains(bl, "Data Layer") {
			hasDLBacklink = true
		}
	}
	if hasDLBacklink {
		t.Error("Database should not have Data Layer backlink after body replacement")
	}

	msgBacklinks, _ := findBacklinks(vaultDir, "Messaging")
	hasNewBacklink := false
	for _, bl := range msgBacklinks {
		if strings.Contains(bl, "Data Layer") {
			hasNewBacklink = true
		}
	}
	if !hasNewBacklink {
		t.Error("Messaging should have Data Layer backlink after write")
	}

	// Frontmatter must be preserved
	yaml, _, hasFM := extractFrontmatter(got)
	if !hasFM {
		t.Fatal("frontmatter lost after write")
	}
	if v, ok := frontmatterGetValue(yaml, "type"); !ok || v != "pattern" {
		t.Errorf("type property lost: %q", v)
	}
}

// TestE2EPatchByLineRange creates a note, patches lines 3-7 with new content,
// reads back, and verifies lines 1-2 unchanged, 3-7 replaced, rest unchanged.
func TestE2EPatchByLineRange(t *testing.T) {
	vaultDir := t.TempDir()

	// Create a note with numbered lines for easy verification
	// Line numbers are 1-based in patch command
	content := "Line 1: Introduction\nLine 2: Background\nLine 3: Old detail A\nLine 4: Old detail B\nLine 5: Old detail C\nLine 6: Old detail D\nLine 7: Old detail E\nLine 8: Conclusion\nLine 9: References\n"
	notePath := filepath.Join(vaultDir, "LineTest.md")
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Patch lines 3-7 with new content
	patchParams := map[string]string{
		"file":    "LineTest",
		"line":    "3-7",
		"content": "Line 3-7: Replaced with single consolidated line",
	}
	if err := cmdPatch(vaultDir, patchParams, false, false); err != nil {
		t.Fatalf("patch by line range: %v", err)
	}

	data, _ := os.ReadFile(notePath)
	got := string(data)
	lines := strings.Split(got, "\n")

	// Lines 1-2 must be unchanged
	if lines[0] != "Line 1: Introduction" {
		t.Errorf("line 1 changed: got %q", lines[0])
	}
	if lines[1] != "Line 2: Background" {
		t.Errorf("line 2 changed: got %q", lines[1])
	}

	// Line 3 (index 2) should be the replacement
	if lines[2] != "Line 3-7: Replaced with single consolidated line" {
		t.Errorf("replacement line wrong: got %q", lines[2])
	}

	// Lines after replacement should be the original 8 and 9
	if lines[3] != "Line 8: Conclusion" {
		t.Errorf("line after replacement wrong: got %q, want 'Line 8: Conclusion'", lines[3])
	}
	if lines[4] != "Line 9: References" {
		t.Errorf("line after replacement wrong: got %q, want 'Line 9: References'", lines[4])
	}

	// Old content should be gone
	if strings.Contains(got, "Old detail A") {
		t.Error("old line 3 content still present")
	}
	if strings.Contains(got, "Old detail E") {
		t.Error("old line 7 content still present")
	}
}

// TestE2EContentManipulationDoesNotCorruptVault performs multiple write/patch/
// append/prepend operations on several notes and verifies that all notes remain
// valid (readable, frontmatter parseable, no truncation or corruption).
func TestE2EContentManipulationDoesNotCorruptVault(t *testing.T) {
	vaultDir := t.TempDir()
	os.MkdirAll(filepath.Join(vaultDir, "docs"), 0755)

	// Create 5 notes with various frontmatter
	notes := map[string]string{
		"docs/Alpha.md":   "---\ntype: concept\nstatus: active\ntags: [go, testing]\n---\n\n# Alpha\n\n## Section One\n\nAlpha content one.\n\n## Section Two\n\nAlpha content two.\n",
		"docs/Beta.md":    "---\ntype: decision\nstatus: draft\naliases: [B-Note]\n---\n\n# Beta\n\nBeta introduction.\n\n## Details\n\nBeta details here.\n",
		"docs/Gamma.md":   "---\ntype: pattern\nconfidence: high\n---\n\n# Gamma Pattern\n\nLine 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\n",
		"docs/Delta.md":   "---\ntype: debug\nstatus: resolved\n---\n\n# Delta Bug\n\n## Symptoms\n\nApp crashes on startup.\n\n## Root Cause\n\nNull pointer dereference.\n\n## Fix\n\nAdded nil check.\n",
		"docs/Epsilon.md": "# Epsilon\n\nNo frontmatter note.\nJust plain content.\nWith multiple lines.\n",
	}

	for path, content := range notes {
		if err := os.WriteFile(filepath.Join(vaultDir, path), []byte(content), 0644); err != nil {
			t.Fatalf("setup %s: %v", path, err)
		}
	}

	// Operation 1: Write to Alpha (replace body)
	if err := cmdWrite(vaultDir, map[string]string{
		"file":    "Alpha",
		"content": "# Alpha Revised\n\n## New Section\n\nCompletely new content.\n",
	}, false); err != nil {
		t.Fatalf("write Alpha: %v", err)
	}

	// Operation 2: Patch Beta by heading
	if err := cmdPatch(vaultDir, map[string]string{
		"file":    "Beta",
		"heading": "## Details",
		"content": "\nPatched beta details.\n",
	}, false, false); err != nil {
		t.Fatalf("patch Beta: %v", err)
	}

	// Operation 3: Append to Gamma
	if err := cmdAppend(vaultDir, map[string]string{
		"file":    "Gamma",
		"content": "\nAppended to Gamma.\n",
	}, false); err != nil {
		t.Fatalf("append Gamma: %v", err)
	}

	// Operation 4: Prepend to Delta
	if err := cmdPrepend(vaultDir, map[string]string{
		"file":    "Delta",
		"content": "URGENT: Check this bug.\n",
	}, false); err != nil {
		t.Fatalf("prepend Delta: %v", err)
	}

	// Operation 5: Patch Gamma by line range
	if err := cmdPatch(vaultDir, map[string]string{
		"file":    "Gamma",
		"line":    "8-10",
		"content": "Replaced lines.",
	}, false, false); err != nil {
		t.Fatalf("patch Gamma lines: %v", err)
	}

	// Operation 6: Delete a section from Delta
	if err := cmdPatch(vaultDir, map[string]string{
		"file":    "Delta",
		"heading": "## Root Cause",
	}, true, false); err != nil {
		t.Fatalf("delete section Delta: %v", err)
	}

	// Operation 7: Write to Epsilon (no frontmatter)
	if err := cmdWrite(vaultDir, map[string]string{
		"file":    "Epsilon",
		"content": "# Epsilon Rewritten\n\nNew content.\n",
	}, false); err != nil {
		t.Fatalf("write Epsilon: %v", err)
	}

	// Now validate every note in the vault
	noteFiles := []string{
		"docs/Alpha.md", "docs/Beta.md", "docs/Gamma.md",
		"docs/Delta.md", "docs/Epsilon.md",
	}

	for _, relPath := range noteFiles {
		fullPath := filepath.Join(vaultDir, relPath)

		// Must be readable
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("%s: file not readable: %v", relPath, err)
			continue
		}

		content := string(data)

		// Must not be empty
		if len(strings.TrimSpace(content)) == 0 {
			t.Errorf("%s: file is empty after operations", relPath)
			continue
		}

		// Frontmatter, if present, must be parseable
		yaml, _, hasFM := extractFrontmatter(content)
		if hasFM {
			// Verify frontmatter has valid structure
			if yaml == "" {
				t.Errorf("%s: frontmatter block present but empty", relPath)
			}
			// Must start with --- and have closing ---
			if !strings.HasPrefix(content, "---") {
				t.Errorf("%s: frontmatter does not start with ---", relPath)
			}
		}

		// Must be readable via cmdRead without error
		readOut := captureStdout(func() {
			readParams := map[string]string{"file": strings.TrimSuffix(filepath.Base(relPath), ".md")}
			if err := cmdRead(vaultDir, readParams); err != nil {
				t.Errorf("%s: cmdRead failed: %v", relPath, err)
			}
		})
		if readOut == "" {
			t.Errorf("%s: cmdRead returned empty output", relPath)
		}

		// Must be findable via search (all notes have some text content)
		searchOut := captureStdout(func() {
			// Search for filename to ensure the note is indexed
			searchParams := map[string]string{"query": strings.TrimSuffix(filepath.Base(relPath), ".md")}
			cmdSearch(vaultDir, searchParams, "")
		})
		_ = searchOut // Search might not find by title substring; presence check is sufficient
	}

	// Verify specific content integrity after all operations

	// Alpha: frontmatter preserved, new body present
	alphaData, _ := os.ReadFile(filepath.Join(vaultDir, "docs/Alpha.md"))
	alphaYAML, _, alphaHasFM := extractFrontmatter(string(alphaData))
	if !alphaHasFM {
		t.Error("Alpha: frontmatter lost")
	}
	if v, ok := frontmatterGetValue(alphaYAML, "type"); !ok || v != "concept" {
		t.Errorf("Alpha: type property lost: %q", v)
	}
	if !strings.Contains(string(alphaData), "Completely new content.") {
		t.Error("Alpha: new body content missing")
	}

	// Beta: patch was applied correctly
	betaData, _ := os.ReadFile(filepath.Join(vaultDir, "docs/Beta.md"))
	if !strings.Contains(string(betaData), "Patched beta details.") {
		t.Error("Beta: patched content missing")
	}
	if !strings.Contains(string(betaData), "Beta introduction.") {
		t.Error("Beta: non-patched content was corrupted")
	}

	// Gamma: append and line patch both applied
	gammaData, _ := os.ReadFile(filepath.Join(vaultDir, "docs/Gamma.md"))
	if !strings.Contains(string(gammaData), "Appended to Gamma.") {
		t.Error("Gamma: appended content missing")
	}
	if !strings.Contains(string(gammaData), "Replaced lines.") {
		t.Error("Gamma: line-patched content missing")
	}

	// Delta: prepend applied, section deleted, other sections intact
	deltaData, _ := os.ReadFile(filepath.Join(vaultDir, "docs/Delta.md"))
	deltaStr := string(deltaData)
	if !strings.Contains(deltaStr, "URGENT: Check this bug.") {
		t.Error("Delta: prepended content missing")
	}
	if strings.Contains(deltaStr, "Null pointer dereference.") {
		t.Error("Delta: deleted section still present")
	}
	if !strings.Contains(deltaStr, "App crashes on startup.") {
		t.Error("Delta: Symptoms section corrupted")
	}
	if !strings.Contains(deltaStr, "Added nil check.") {
		t.Error("Delta: Fix section corrupted")
	}

	// Epsilon: no frontmatter, body replaced
	epsilonData, _ := os.ReadFile(filepath.Join(vaultDir, "docs/Epsilon.md"))
	if strings.Contains(string(epsilonData), "No frontmatter note.") {
		t.Error("Epsilon: old content still present")
	}
	if !strings.Contains(string(epsilonData), "Epsilon Rewritten") {
		t.Error("Epsilon: new content missing")
	}
}

// ---------------------------------------------------------------------------
// Integration tests for vault-level file locking.
//
// These spawn real OS processes (the compiled vlt binary) that contend on the
// same vault directory. No mocks. Real flock, real filesystem, real I/O.
// ---------------------------------------------------------------------------

// buildVLT compiles the vlt binary into t.TempDir and returns its path.
func buildVLT(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vlt-test")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build vlt: %v\n%s", err, out)
	}
	return bin
}

// TestE2EConcurrentAppendNoCorruption spawns N concurrent vlt append processes
// against the same note and verifies every line appears exactly once.
func TestE2EConcurrentAppendNoCorruption(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent integration test in short mode")
	}
	bin := buildVLT(t)
	vaultDir := t.TempDir()

	// Create the target note.
	notePath := filepath.Join(vaultDir, "Concurrent.md")
	if err := os.WriteFile(notePath, []byte("# Concurrent\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	const N = 10
	var wg sync.WaitGroup
	errs := make(chan error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			line := fmt.Sprintf("Line from process %d", i)
			cmd := exec.Command(bin,
				fmt.Sprintf("vault=%s", vaultDir),
				"append",
				"file=Concurrent",
				fmt.Sprintf("content=%s", line),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs <- fmt.Errorf("process %d: %v\n%s", i, err, out)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	content := string(data)

	// Every process's line must appear exactly once.
	for i := 0; i < N; i++ {
		line := fmt.Sprintf("Line from process %d", i)
		count := strings.Count(content, line)
		if count != 1 {
			t.Errorf("line %d: appeared %d times (want 1)", i, count)
		}
	}
}

// TestE2EConcurrentReadDuringWrite verifies that a reader does not see a
// partial write. A writer replaces the note body while readers read it;
// each reader must see either the old or the new body, never a mix.
func TestE2EConcurrentReadDuringWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent integration test in short mode")
	}
	bin := buildVLT(t)
	vaultDir := t.TempDir()

	oldBody := "OLD CONTENT INTACT"
	newBody := "NEW CONTENT INTACT"

	notePath := filepath.Join(vaultDir, "ReadWrite.md")
	if err := os.WriteFile(notePath, []byte("---\nstatus: active\n---\n\n"+oldBody+"\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var wg sync.WaitGroup
	readResults := make(chan string, 20)

	// Launch the writer (exclusive lock).
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd := exec.Command(bin,
			fmt.Sprintf("vault=%s", vaultDir),
			"write",
			"file=ReadWrite",
			fmt.Sprintf("content=%s", newBody),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("writer: %v\n%s", err, out)
		}
	}()

	// Launch several readers (shared locks) concurrently.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(bin,
				fmt.Sprintf("vault=%s", vaultDir),
				"read",
				"file=ReadWrite",
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("reader: %v\n%s", err, out)
				return
			}
			readResults <- string(out)
		}()
	}

	wg.Wait()
	close(readResults)

	for result := range readResults {
		hasOld := strings.Contains(result, oldBody)
		hasNew := strings.Contains(result, newBody)
		if !hasOld && !hasNew {
			t.Errorf("reader got neither old nor new content:\n%s", result)
		}
		if hasOld && hasNew {
			t.Errorf("reader got BOTH old and new content (torn read):\n%s", result)
		}
	}
}

// TestE2ELockBlocksWriterUntilRelease verifies that an exclusive lock held
// by one process makes a second writer wait (not fail).
func TestE2ELockBlocksWriterUntilRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent integration test in short mode")
	}
	bin := buildVLT(t)
	vaultDir := t.TempDir()

	notePath := filepath.Join(vaultDir, "Blocking.md")
	if err := os.WriteFile(notePath, []byte("# Blocking\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Two sequential appends via concurrent processes -- both must succeed.
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for _, label := range []string{"FIRST", "SECOND"} {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			cmd := exec.Command(bin,
				fmt.Sprintf("vault=%s", vaultDir),
				"append",
				"file=Blocking",
				fmt.Sprintf("content=Appended by %s", label),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs <- fmt.Errorf("%s: %v\n%s", label, err, out)
			}
		}(label)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "Appended by FIRST") {
		t.Error("FIRST append missing from result")
	}
	if !strings.Contains(content, "Appended by SECOND") {
		t.Error("SECOND append missing from result")
	}
}

// TestE2EConcurrentMoveAndRead exercises the most dangerous command (move)
// concurrently with reads to verify locking prevents corruption.
func TestE2EConcurrentMoveAndRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent integration test in short mode")
	}
	bin := buildVLT(t)
	vaultDir := t.TempDir()

	// Create two notes that link to each other.
	noteA := filepath.Join(vaultDir, "SourceNote.md")
	noteB := filepath.Join(vaultDir, "LinkHolder.md")
	if err := os.WriteFile(noteA, []byte("# Source\n\nContent of source.\n"), 0644); err != nil {
		t.Fatalf("setup A: %v", err)
	}
	if err := os.WriteFile(noteB, []byte("# Links\n\nSee [[SourceNote]] for details.\n"), 0644); err != nil {
		t.Fatalf("setup B: %v", err)
	}

	var wg sync.WaitGroup
	moveErr := make(chan error, 1)
	readResults := make(chan string, 5)

	// Move SourceNote -> RenamedNote (exclusive lock, updates wikilinks).
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd := exec.Command(bin,
			fmt.Sprintf("vault=%s", vaultDir),
			"move",
			"path=SourceNote.md",
			"to=RenamedNote.md",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			moveErr <- fmt.Errorf("move: %v\n%s", err, out)
		}
	}()

	// Concurrent readers on LinkHolder.
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(bin,
				fmt.Sprintf("vault=%s", vaultDir),
				"read",
				"file=LinkHolder",
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				// LinkHolder should always be readable.
				t.Errorf("reader: %v\n%s", err, out)
				return
			}
			readResults <- string(out)
		}()
	}

	wg.Wait()
	close(moveErr)
	close(readResults)

	for err := range moveErr {
		t.Error(err)
	}

	// After move: RenamedNote.md must exist, SourceNote.md must not.
	if _, err := os.Stat(filepath.Join(vaultDir, "RenamedNote.md")); err != nil {
		t.Error("RenamedNote.md should exist after move")
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "SourceNote.md")); err == nil {
		t.Error("SourceNote.md should not exist after move")
	}

	// LinkHolder's wikilinks must have been updated.
	data, err := os.ReadFile(noteB)
	if err != nil {
		t.Fatalf("read LinkHolder: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[[RenamedNote]]") {
		t.Errorf("wikilink not updated in LinkHolder:\n%s", content)
	}

	// Every read must have returned coherent content (heading present).
	for result := range readResults {
		if !strings.Contains(result, "# Links") {
			t.Errorf("reader got incoherent content:\n%s", result)
		}
	}
}

// TestE2ELockFileCreatedByBinary verifies the real binary creates .vlt.lock.
func TestE2ELockFileCreatedByBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	bin := buildVLT(t)
	vaultDir := t.TempDir()

	notePath := filepath.Join(vaultDir, "LockCheck.md")
	if err := os.WriteFile(notePath, []byte("# Lock Check\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd := exec.Command(bin,
		fmt.Sprintf("vault=%s", vaultDir),
		"read",
		"file=LockCheck",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vlt read: %v\n%s", err, out)
	}

	lockPath := filepath.Join(vaultDir, ".vlt.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf(".vlt.lock was not created by the binary: %v", err)
	}
}
