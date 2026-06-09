package vlt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// IntegrityStatus represents the result of a file integrity check.
type IntegrityStatus int

const (
	// IntegrityOK means the file content matches the registered hash.
	IntegrityOK IntegrityStatus = iota
	// IntegrityUntracked means no registry entry exists for this file.
	IntegrityUntracked
	// IntegrityMismatch means the file content differs from the registered hash.
	IntegrityMismatch
	// IntegrityNoRegistry means no registry file was found (first use).
	IntegrityNoRegistry
)

// String returns a human-readable label for the status.
func (s IntegrityStatus) String() string {
	switch s {
	case IntegrityOK:
		return "ok"
	case IntegrityUntracked:
		return "untracked"
	case IntegrityMismatch:
		return "mismatch"
	case IntegrityNoRegistry:
		return "no-registry"
	default:
		return "unknown"
	}
}

// ReadResult wraps the content of a Read operation with its integrity status.
type ReadResult struct {
	Content   string
	Integrity IntegrityStatus
}

// registryEntry stores the hash and timestamp for a single tracked file.
type registryEntry struct {
	Hash string `json:"hash"`
	Ts   string `json:"ts"`
}

// Registry tracks content hashes for vault files written through vlt.
type Registry struct {
	dir     string                   // ~/.vlt/registries/<vault-id>/
	entries map[string]registryEntry // keyed by vault-relative path
	exists  bool                     // true if the registry file was loaded from disk
	mu      sync.Mutex
}

// vaultID computes a stable identifier from a vault's absolute path.
// Uses the first 16 hex characters of the SHA-256 hash.
func vaultID(vaultDir string) string {
	abs, err := filepath.Abs(vaultDir)
	if err != nil {
		abs = vaultDir
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:8])
}

// registryDir returns the registry directory for a vault.
// Layout: ~/.vlt/registries/<vault-id>/
func registryDir(vaultDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".vlt", "registries", vaultID(vaultDir))
}

// openRegistry loads (or creates) a Registry for the given vault directory.
// Failures other than "no registry yet" are reported on stderr: for a tamper
// detection feature, silently resetting on corruption would itself hide
// tampering.
func openRegistry(vaultDir string) *Registry {
	dir := registryDir(vaultDir)
	r := &Registry{
		dir:     dir,
		entries: make(map[string]registryEntry),
	}

	regPath := filepath.Join(dir, "registry.json")
	data, err := os.ReadFile(regPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "vlt: warning: cannot read integrity registry %s: %v\n", regPath, err)
		}
		return r // no registry yet
	}

	if err := json.Unmarshal(data, &r.entries); err != nil {
		fmt.Fprintf(os.Stderr, "vlt: warning: integrity registry %s is corrupted (%v); integrity status resets -- run integrity:baseline to re-register\n", regPath, err)
		r.entries = make(map[string]registryEntry)
		return r
	}
	r.exists = true
	return r
}

// ReloadRegistry re-reads the integrity registry from disk. Call after
// acquiring the vault lock: the registry is first loaded when the vault is
// opened, before the lock is held, so a concurrent writer may have flushed
// new entries in between. Without the reload those entries would be lost on
// the next flush.
func (v *Vault) ReloadRegistry() {
	v.mu.Lock()
	v.registry = openRegistry(v.dir)
	v.mu.Unlock()
}

// contentHash computes the SHA-256 hex digest of content.
func contentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// register records the hash of content written to absPath.
// Must be called after a successful write, passing the content that was written
// (not re-read from disk, to avoid TOCTOU).
func (r *Registry) register(vaultDir, absPath string, content []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.setEntryLocked(vaultDir, absPath, content) {
		return
	}
	r.flush()
}

// setEntryLocked records a hash entry without flushing. Caller must hold r.mu.
// Returns false if the path could not be made vault-relative.
func (r *Registry) setEntryLocked(vaultDir, absPath string, content []byte) bool {
	rel, err := filepath.Rel(vaultDir, absPath)
	if err != nil {
		return false
	}

	r.entries[rel] = registryEntry{
		Hash: contentHash(content),
		Ts:   time.Now().UTC().Format(time.RFC3339),
	}
	r.exists = true
	return true
}

// deregister removes a file from the registry.
func (r *Registry) deregister(vaultDir, absPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rel, err := filepath.Rel(vaultDir, absPath)
	if err != nil {
		return
	}

	delete(r.entries, rel)
	r.flush()
}

// verify checks whether the content matches the registered hash.
func (r *Registry) verify(vaultDir, absPath string, content []byte) IntegrityStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.exists {
		return IntegrityNoRegistry
	}

	rel, err := filepath.Rel(vaultDir, absPath)
	if err != nil {
		return IntegrityUntracked
	}

	entry, ok := r.entries[rel]
	if !ok {
		return IntegrityUntracked
	}

	if entry.Hash == contentHash(content) {
		return IntegrityOK
	}
	return IntegrityMismatch
}

// flush writes the registry to disk atomically (write temp + rename).
// Caller must hold r.mu. Failures are reported on stderr rather than
// silently dropped -- a failed flush means integrity tracking is stale.
func (r *Registry) flush() {
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "vlt: warning: cannot create integrity registry dir %s: %v\n", r.dir, err)
		return
	}

	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "vlt: warning: cannot encode integrity registry: %v\n", err)
		return
	}

	tmpPath := filepath.Join(r.dir, "registry.json.tmp")
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "vlt: warning: cannot write integrity registry: %v\n", err)
		return
	}
	if err := os.Rename(tmpPath, filepath.Join(r.dir, "registry.json")); err != nil {
		fmt.Fprintf(os.Stderr, "vlt: warning: cannot update integrity registry: %v\n", err)
	}
}

// VerifyIntegrity checks the integrity of specific vault files.
// If no paths are provided, all registered files are checked.
func (v *Vault) VerifyIntegrity(paths ...string) map[string]IntegrityStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()

	results := make(map[string]IntegrityStatus)

	if len(paths) == 0 {
		// Check all registered entries.
		v.registry.mu.Lock()
		for rel := range v.registry.entries {
			absPath := filepath.Join(v.dir, rel)
			data, err := os.ReadFile(absPath)
			if err != nil {
				// File deleted outside vlt -- treat as mismatch.
				results[rel] = IntegrityMismatch
				continue
			}
			entry := v.registry.entries[rel]
			if entry.Hash == contentHash(data) {
				results[rel] = IntegrityOK
			} else {
				results[rel] = IntegrityMismatch
			}
		}
		v.registry.mu.Unlock()
		return results
	}

	for _, title := range paths {
		path, err := v.resolve(title)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		results[title] = v.registry.verify(v.dir, path, data)
	}

	return results
}

// IntegrityBaseline walks all .md files in the vault and registers each one.
// Entries are recorded in memory and flushed to disk once, so baselining a
// large vault does not rewrite the registry file per note.
func (v *Vault) IntegrityBaseline() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	r := v.registry
	r.mu.Lock()
	defer r.mu.Unlock()

	walkErr := filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, v.dir) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		r.setEntryLocked(v.dir, path, data)
		return nil
	})

	r.flush()
	return walkErr
}

// IntegrityAcknowledge re-reads a file and updates its registry entry,
// accepting the current content as the new baseline.
func (v *Vault) IntegrityAcknowledge(title string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	path, err := v.resolve(title)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	v.registry.register(v.dir, path, data)
	return nil
}

// IntegrityAcknowledgeSince re-registers all .md files modified within the
// given duration. Returns the number of files re-registered.
// Entries are flushed to disk once at the end.
func (v *Vault) IntegrityAcknowledgeSince(d time.Duration) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	cutoff := time.Now().Add(-d)
	count := 0

	r := v.registry
	r.mu.Lock()
	defer r.mu.Unlock()

	err := filepath.WalkDir(v.dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, entry, v.dir) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if r.setEntryLocked(v.dir, path, data) {
			count++
		}
		return nil
	})

	if count > 0 {
		r.flush()
	}
	return count, err
}

// IntegrityStatus returns the integrity status of all registered files plus
// any untracked .md files in the vault.
func (v *Vault) IntegrityStatusAll() map[string]IntegrityStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()

	results := make(map[string]IntegrityStatus)

	// Check registered files.
	v.registry.mu.Lock()
	if !v.registry.exists {
		v.registry.mu.Unlock()
		return results
	}
	for rel := range v.registry.entries {
		absPath := filepath.Join(v.dir, rel)
		data, err := os.ReadFile(absPath)
		if err != nil {
			results[rel] = IntegrityMismatch
			continue
		}
		entry := v.registry.entries[rel]
		if entry.Hash == contentHash(data) {
			results[rel] = IntegrityOK
		} else {
			results[rel] = IntegrityMismatch
		}
	}
	registeredSet := make(map[string]bool, len(v.registry.entries))
	for rel := range v.registry.entries {
		registeredSet[rel] = true
	}
	v.registry.mu.Unlock()

	// Walk for untracked files.
	filepath.WalkDir(v.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skipHiddenDir(path, d, v.dir) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(v.dir, path)
		if relErr != nil {
			return nil
		}
		if !registeredSet[rel] {
			results[rel] = IntegrityUntracked
		}
		return nil
	})

	return results
}

// IntegrityRegistryDir returns the registry directory for this vault (for testing).
func (v *Vault) IntegrityRegistryDir() string {
	return v.registry.dir
}
