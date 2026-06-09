package vlt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRegistryRoundTrip verifies that registering a file and then verifying
// returns IntegrityOK.
func TestRegistryRoundTrip(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	content := []byte("# Test\n\nSome content.\n")
	path := filepath.Join(vaultDir, "note.md")
	os.WriteFile(path, content, 0644)

	reg.register(vaultDir, path, content)

	status := reg.verify(vaultDir, path, content)
	if status != IntegrityOK {
		t.Errorf("expected IntegrityOK, got %s", status)
	}
}

// TestRegistryMismatch verifies that modifying a file externally is detected.
func TestRegistryMismatch(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	content := []byte("# Original\n")
	path := filepath.Join(vaultDir, "note.md")
	os.WriteFile(path, content, 0644)

	reg.register(vaultDir, path, content)

	// Modify externally
	modified := []byte("# Modified\n")
	os.WriteFile(path, modified, 0644)

	status := reg.verify(vaultDir, path, modified)
	if status != IntegrityMismatch {
		t.Errorf("expected IntegrityMismatch, got %s", status)
	}
}

// TestRegistryUntracked verifies that a file not in the registry is reported
// as untracked.
func TestRegistryUntracked(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	// Register one file to make the registry "exist"
	known := []byte("# Known\n")
	knownPath := filepath.Join(vaultDir, "known.md")
	os.WriteFile(knownPath, known, 0644)
	reg.register(vaultDir, knownPath, known)

	// Create an untracked file
	untracked := []byte("# Untracked\n")
	untrackedPath := filepath.Join(vaultDir, "untracked.md")
	os.WriteFile(untrackedPath, untracked, 0644)

	status := reg.verify(vaultDir, untrackedPath, untracked)
	if status != IntegrityUntracked {
		t.Errorf("expected IntegrityUntracked, got %s", status)
	}
}

// TestRegistryDeregister verifies that deregistering returns Untracked on verify.
func TestRegistryDeregister(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	content := []byte("# Note\n")
	path := filepath.Join(vaultDir, "note.md")
	os.WriteFile(path, content, 0644)

	reg.register(vaultDir, path, content)
	reg.deregister(vaultDir, path)

	status := reg.verify(vaultDir, path, content)
	if status != IntegrityUntracked {
		t.Errorf("expected IntegrityUntracked after deregister, got %s", status)
	}
}

// TestRegistryAtomicFlush verifies the registry file is written atomically.
func TestRegistryAtomicFlush(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	content := []byte("# Test\n")
	path := filepath.Join(vaultDir, "note.md")
	os.WriteFile(path, content, 0644)

	reg.register(vaultDir, path, content)

	// Verify registry.json exists and tmp doesn't
	regFile := filepath.Join(reg.dir, "registry.json")
	tmpFile := filepath.Join(reg.dir, "registry.json.tmp")

	if _, err := os.Stat(regFile); os.IsNotExist(err) {
		t.Error("registry.json not created")
	}
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("temp file should be removed after atomic rename")
	}

	// Verify the registry survives reload
	reg2 := openRegistry(vaultDir)
	status := reg2.verify(vaultDir, path, content)
	if status != IntegrityOK {
		t.Errorf("expected IntegrityOK after reload, got %s", status)
	}
}

// TestRegistryNoRegistry verifies that verify returns IntegrityNoRegistry
// when no registry has been created.
func TestRegistryNoRegistry(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	content := []byte("# Test\n")
	path := filepath.Join(vaultDir, "note.md")
	os.WriteFile(path, content, 0644)

	status := reg.verify(vaultDir, path, content)
	if status != IntegrityNoRegistry {
		t.Errorf("expected IntegrityNoRegistry, got %s", status)
	}
}

// TestReadIntegrity verifies that Vault.Read returns correct integrity status.
func TestReadIntegrity(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	content := "# ReadTest\n\nBody.\n"
	os.WriteFile(filepath.Join(vaultDir, "ReadTest.md"), []byte(content), 0644)

	// No registry yet -- should return NoRegistry
	result, err := v.Read("ReadTest", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityNoRegistry {
		t.Errorf("expected IntegrityNoRegistry before baseline, got %s", result.Integrity)
	}
	if result.Content != content {
		t.Errorf("content mismatch: got %q", result.Content)
	}

	// Run baseline
	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("IntegrityBaseline: %v", err)
	}

	// Now Read should return OK
	result, err = v.Read("ReadTest", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after baseline, got %s", result.Integrity)
	}

	// Modify externally
	os.WriteFile(filepath.Join(vaultDir, "ReadTest.md"), []byte("# Changed\n"), 0644)

	// Read should return Mismatch
	result, err = v.Read("ReadTest", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityMismatch {
		t.Errorf("expected IntegrityMismatch after external edit, got %s", result.Integrity)
	}
}

// TestWriteRegisters verifies that Create, Write, Append, Prepend register
// the content hash automatically.
func TestWriteRegisters(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	// Create registers
	if err := v.Create("Note", "Note.md", "# Note\n\nOriginal.\n", true, false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	result, err := v.Read("Note", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after Create, got %s", result.Integrity)
	}

	// Write registers
	if err := v.Write("Note", "# Note\n\nUpdated.\n", false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	result, err = v.Read("Note", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after Write, got %s", result.Integrity)
	}

	// Append registers
	if err := v.Append("Note", "\nAppended.\n", false); err != nil {
		t.Fatalf("Append: %v", err)
	}
	result, err = v.Read("Note", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after Append, got %s", result.Integrity)
	}

	// Prepend registers
	if err := v.Prepend("Note", "Prepended line.\n", false); err != nil {
		t.Fatalf("Prepend: %v", err)
	}
	result, err = v.Read("Note", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after Prepend, got %s", result.Integrity)
	}
}

// TestMoveUpdatesRegistry verifies that Move deregisters old path and
// registers new path.
func TestMoveUpdatesRegistry(t *testing.T) {
	vaultDir := t.TempDir()
	os.MkdirAll(filepath.Join(vaultDir, "_inbox"), 0755)
	os.MkdirAll(filepath.Join(vaultDir, "decisions"), 0755)
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	content := "# Decision\n\nImportant.\n"
	if err := v.Create("Decision", "_inbox/Decision.md", content, true, false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := v.Move("_inbox/Decision.md", "decisions/Decision.md")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	// Old path should be deregistered (check via internal registry directly)
	oldPath := filepath.Join(vaultDir, "_inbox", "Decision.md")
	status := v.registry.verify(v.dir, oldPath, []byte(content))
	if status != IntegrityUntracked {
		t.Errorf("expected old path to be untracked after move, got %s", status)
	}

	// New path should be registered (check via registry directly to avoid
	// resolveNote ambiguity if we recreated the old file)
	newPath := filepath.Join(vaultDir, "decisions", "Decision.md")
	newData, readErr := os.ReadFile(newPath)
	if readErr != nil {
		t.Fatalf("read new path: %v", readErr)
	}
	newStatus := v.registry.verify(v.dir, newPath, newData)
	if newStatus != IntegrityOK {
		t.Errorf("expected IntegrityOK at new path, got %s", newStatus)
	}
}

// TestDeleteDeregisters verifies that Delete removes the entry from the registry.
func TestDeleteDeregisters(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	content := "# Temporary\n"
	if err := v.Create("Temporary", "Temporary.md", content, true, false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify it's registered
	result, err := v.Read("Temporary", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK before delete, got %s", result.Integrity)
	}

	// Delete permanently
	_, err = v.Delete("Temporary", "", true)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Registry entry should be gone
	path := filepath.Join(vaultDir, "Temporary.md")
	status := v.registry.verify(v.dir, path, []byte(content))
	if status != IntegrityUntracked {
		t.Errorf("expected IntegrityUntracked after delete, got %s", status)
	}
}

// TestIntegrityBaseline verifies that IntegrityBaseline registers all .md files.
func TestIntegrityBaseline(t *testing.T) {
	vaultDir := t.TempDir()
	os.MkdirAll(filepath.Join(vaultDir, "sub"), 0755)
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	os.WriteFile(filepath.Join(vaultDir, "A.md"), []byte("# A\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "B.md"), []byte("# B\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "sub", "C.md"), []byte("# C\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "ignore.txt"), []byte("not markdown"), 0644)

	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("IntegrityBaseline: %v", err)
	}

	statuses := v.IntegrityStatusAll()
	okCount := 0
	for _, s := range statuses {
		if s == IntegrityOK {
			okCount++
		}
	}
	if okCount != 3 {
		t.Errorf("expected 3 OK entries, got %d (statuses: %v)", okCount, statuses)
	}
}

// TestIntegrityAcknowledge verifies that acknowledging re-registers a modified file.
func TestIntegrityAcknowledge(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	content := "# Note\n"
	os.WriteFile(filepath.Join(vaultDir, "Note.md"), []byte(content), 0644)

	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("IntegrityBaseline: %v", err)
	}

	// Modify externally
	os.WriteFile(filepath.Join(vaultDir, "Note.md"), []byte("# Modified\n"), 0644)

	// Should be mismatch
	result, _ := v.Read("Note", "")
	if result.Integrity != IntegrityMismatch {
		t.Errorf("expected mismatch, got %s", result.Integrity)
	}

	// Acknowledge
	if err := v.IntegrityAcknowledge("Note"); err != nil {
		t.Fatalf("IntegrityAcknowledge: %v", err)
	}

	// Should be OK now
	result, _ = v.Read("Note", "")
	if result.Integrity != IntegrityOK {
		t.Errorf("expected OK after acknowledge, got %s", result.Integrity)
	}
}

// TestIntegrityAcknowledgeSince verifies batch acknowledgement by time.
func TestIntegrityAcknowledgeSince(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	os.WriteFile(filepath.Join(vaultDir, "A.md"), []byte("# A\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "B.md"), []byte("# B\n"), 0644)

	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("IntegrityBaseline: %v", err)
	}

	// Modify both externally
	os.WriteFile(filepath.Join(vaultDir, "A.md"), []byte("# A modified\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "B.md"), []byte("# B modified\n"), 0644)

	count, err := v.IntegrityAcknowledgeSince(1 * time.Hour)
	if err != nil {
		t.Fatalf("IntegrityAcknowledgeSince: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 acknowledged, got %d", count)
	}

	// Both should be OK now
	statuses := v.IntegrityStatusAll()
	for path, s := range statuses {
		if s != IntegrityOK {
			t.Errorf("%s: expected OK, got %s", path, s)
		}
	}
}

// TestIntegrityStatusAll verifies the batch status check returns correct
// mix of statuses.
func TestIntegrityStatusAll(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	// Create and baseline 2 files
	os.WriteFile(filepath.Join(vaultDir, "OK.md"), []byte("# OK\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "Modified.md"), []byte("# Original\n"), 0644)

	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("IntegrityBaseline: %v", err)
	}

	// Modify one externally
	os.WriteFile(filepath.Join(vaultDir, "Modified.md"), []byte("# Changed\n"), 0644)

	// Add an untracked file
	os.WriteFile(filepath.Join(vaultDir, "New.md"), []byte("# New\n"), 0644)

	statuses := v.IntegrityStatusAll()

	if statuses["OK.md"] != IntegrityOK {
		t.Errorf("OK.md: expected OK, got %s", statuses["OK.md"])
	}
	if statuses["Modified.md"] != IntegrityMismatch {
		t.Errorf("Modified.md: expected Mismatch, got %s", statuses["Modified.md"])
	}
	if statuses["New.md"] != IntegrityUntracked {
		t.Errorf("New.md: expected Untracked, got %s", statuses["New.md"])
	}
}

// TestPatchRegisters verifies Patch registers after edits.
func TestPatchRegisters(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	content := "## Section\n\nOriginal.\n"
	if err := v.Create("Note", "Note.md", content, true, false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := v.Patch("Note", PatchOptions{
		Heading: "## Section",
		Content: "\nPatched.\n",
	}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	result, err := v.Read("Note", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after Patch, got %s", result.Integrity)
	}
	if !strings.Contains(result.Content, "Patched.") {
		t.Error("patched content not found")
	}
}

// TestPropertySetRegisters verifies PropertySet registers after edits.
func TestPropertySetRegisters(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	content := "---\ntype: note\n---\n\n# Note\n"
	if err := v.Create("Note", "Note.md", content, true, false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := v.PropertySet("Note", "status", "active"); err != nil {
		t.Fatalf("PropertySet: %v", err)
	}

	result, err := v.Read("Note", "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Integrity != IntegrityOK {
		t.Errorf("expected IntegrityOK after PropertySet, got %s", result.Integrity)
	}
}

// TestIntegrityStatusString verifies the String method on IntegrityStatus.
func TestIntegrityStatusString(t *testing.T) {
	tests := []struct {
		status IntegrityStatus
		want   string
	}{
		{IntegrityOK, "ok"},
		{IntegrityUntracked, "untracked"},
		{IntegrityMismatch, "mismatch"},
		{IntegrityNoRegistry, "no-registry"},
		{IntegrityStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("IntegrityStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// TestVerifyIntegrity verifies the public batch verify method.
func TestVerifyIntegrity(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	os.WriteFile(filepath.Join(vaultDir, "A.md"), []byte("# A\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "B.md"), []byte("# B\n"), 0644)

	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("IntegrityBaseline: %v", err)
	}

	// Modify B
	os.WriteFile(filepath.Join(vaultDir, "B.md"), []byte("# Changed\n"), 0644)

	results := v.VerifyIntegrity("A", "B")
	if results["A"] != IntegrityOK {
		t.Errorf("A: expected OK, got %s", results["A"])
	}
	if results["B"] != IntegrityMismatch {
		t.Errorf("B: expected Mismatch, got %s", results["B"])
	}
}

// TestRegistryDirPermissions verifies registry directory has 0700 permissions.
func TestRegistryDirPermissions(t *testing.T) {
	vaultDir := t.TempDir()
	reg := openRegistry(vaultDir)

	content := []byte("# Test\n")
	path := filepath.Join(vaultDir, "note.md")
	os.WriteFile(path, content, 0644)
	reg.register(vaultDir, path, content)

	info, err := os.Stat(reg.dir)
	if err != nil {
		t.Fatalf("stat registry dir: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("registry dir permissions: got %o, want 0700", perm)
	}
}

// -----------------------------------------------------------------
// Regression tests from the 2026-06-09 full review.
// -----------------------------------------------------------------

// TestReloadRegistryPreservesConcurrentWrites simulates the lost-update race:
// two processes open the vault (loading the registry) before either takes the
// lock. Without a post-lock reload, the second writer's flush would erase the
// first writer's entry.
func TestReloadRegistryPreservesConcurrentWrites(t *testing.T) {
	vaultDir := t.TempDir()

	v1, err := Open(vaultDir)
	if err != nil {
		t.Fatalf("open v1: %v", err)
	}
	v2, err := Open(vaultDir) // loads registry before v1 writes
	if err != nil {
		t.Fatalf("open v2: %v", err)
	}

	if err := v1.Create("A", "A.md", "# A\n", false, false); err != nil {
		t.Fatalf("v1 create: %v", err)
	}

	// v2 reloads (as the CLI now does after acquiring the lock), then writes.
	v2.ReloadRegistry()
	if err := v2.Create("B", "B.md", "# B\n", false, false); err != nil {
		t.Fatalf("v2 create: %v", err)
	}

	reg := openRegistry(vaultDir)
	if _, ok := reg.entries["A.md"]; !ok {
		t.Error("registry entry for A.md lost -- concurrent write erased it")
	}
	if _, ok := reg.entries["B.md"]; !ok {
		t.Error("registry entry for B.md missing")
	}
}

// TestIntegrityBaselineRegistersAllWithSingleRegistry verifies baseline still
// registers every note after the batched single-flush rewrite.
func TestIntegrityBaselineRegistersAll(t *testing.T) {
	vaultDir := t.TempDir()
	v := &Vault{dir: vaultDir, registry: openRegistry(vaultDir)}

	os.MkdirAll(filepath.Join(vaultDir, "sub"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "A.md"), []byte("# A\n"), 0644)
	os.WriteFile(filepath.Join(vaultDir, "sub", "B.md"), []byte("# B\n"), 0644)

	if err := v.IntegrityBaseline(); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	statuses := v.IntegrityStatusAll()
	if statuses["A.md"] != IntegrityOK {
		t.Errorf("A.md status = %v, want OK", statuses["A.md"])
	}
	if statuses[filepath.Join("sub", "B.md")] != IntegrityOK {
		t.Errorf("sub/B.md status = %v, want OK", statuses[filepath.Join("sub", "B.md")])
	}
}
