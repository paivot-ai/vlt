package vlt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Vault represents an opened Obsidian vault. It carries the vault root
// directory and a mutex for goroutine-safe operations.
type Vault struct {
	dir      string
	registry *Registry
	mu       sync.RWMutex
	idxState indexedVault
}

// Open opens a vault at the given directory path, validating that it exists.
func Open(dir string) (*Vault, error) {
	dir, err := validateVaultDir(dir)
	if err != nil {
		return nil, err
	}
	v := &Vault{dir: dir}
	v.registry = openRegistry(dir)
	return v, nil
}

// OpenByName resolves a vault by name (or path) via the Obsidian config
// and returns an opened Vault.
func OpenByName(name string) (*Vault, error) {
	dir, err := resolveVault(name)
	if err != nil {
		return nil, err
	}
	v := &Vault{dir: dir}
	v.registry = openRegistry(dir)
	return v, nil
}

// Dir returns the vault root directory path.
func (v *Vault) Dir() string {
	return v.dir
}

// obsidianConfig is the top-level structure of Obsidian's config file.
type obsidianConfig struct {
	Vaults map[string]vaultEntry `json:"vaults"`
}

// vaultEntry represents a single vault in Obsidian's config.
type vaultEntry struct {
	Path string `json:"path"`
	TS   int64  `json:"ts"`
}

// resolveVault turns a vault name (or path) into an absolute directory path.
//
// If name looks like an absolute path, it's used directly.
// Otherwise, it's looked up by directory basename in the Obsidian config.
func resolveVault(name string) (string, error) {
	// Direct absolute path (filepath.IsAbs handles both /unix and C:\windows forms)
	if filepath.IsAbs(name) {
		return validateVaultDir(name)
	}
	// Home-relative path
	if strings.HasPrefix(name, "~") {
		home, _ := os.UserHomeDir()
		return validateVaultDir(filepath.Join(home, name[1:]))
	}
	// Relative path (e.g., ".vault/knowledge", "./subdir") -- resolve against CWD.
	if strings.HasPrefix(name, ".") {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot resolve relative vault path: %w", err)
		}
		abs := filepath.Join(cwd, name)
		return validateVaultDir(abs)
	}

	// Look up by name
	vaults, err := DiscoverVaults()
	if err != nil {
		// Fall back to VLT_VAULT_PATH env var
		if p := os.Getenv("VLT_VAULT_PATH"); p != "" {
			return validateVaultDir(p)
		}
		return "", fmt.Errorf("cannot discover vaults: %w", err)
	}

	path, ok := vaults[name]
	if !ok {
		// Fall back to VLT_VAULT_PATH when the name is not in the config
		// (not only when the config itself is unreadable).
		if p := os.Getenv("VLT_VAULT_PATH"); p != "" {
			return validateVaultDir(p)
		}
		available := make([]string, 0, len(vaults))
		for k := range vaults {
			available = append(available, k)
		}
		return "", fmt.Errorf("vault %q not found. Available: %s", name, strings.Join(available, ", "))
	}

	return validateVaultDir(path)
}

func validateVaultDir(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("vault directory not found: %s", path)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("vault path is not a directory: %s", path)
	}
	return path, nil
}

// ErrPathTraversal is returned when a user-supplied path escapes the vault root.
var ErrPathTraversal = fmt.Errorf("path escapes vault boundary")

// safePath validates that a user-supplied relative path, when joined with the
// vault root, stays within the vault. Returns the cleaned absolute path.
// Rejects absolute paths, paths containing "..", and any result that does not
// start with the vault root prefix.
func safePath(vaultDir, userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("empty path")
	}
	// Reject absolute paths -- user paths must be vault-relative.
	if filepath.IsAbs(userPath) {
		return "", ErrPathTraversal
	}
	joined := filepath.Join(vaultDir, userPath)
	cleaned := filepath.Clean(joined)
	// The cleaned path must be under vaultDir (or equal for edge cases).
	// Use vaultDir+"/" so that "/tmp/vault2" is not treated as inside "/tmp/vault".
	vaultPrefix := filepath.Clean(vaultDir) + string(filepath.Separator)
	if cleaned != filepath.Clean(vaultDir) && !strings.HasPrefix(cleaned, vaultPrefix) {
		return "", ErrPathTraversal
	}
	return cleaned, nil
}

// DiscoverVaults reads the Obsidian config file and returns a map of
// vault name (directory basename) to absolute path.
// When two vaults share a basename, the most recently used one (highest
// Obsidian timestamp) wins, deterministically, instead of map iteration order.
func DiscoverVaults() (map[string]string, error) {
	configPath := obsidianConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", configPath, err)
	}

	var config obsidianConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}

	vaults := make(map[string]string, len(config.Vaults))
	newest := make(map[string]int64, len(config.Vaults))
	for _, entry := range config.Vaults {
		name := filepath.Base(entry.Path)
		if prev, ok := newest[name]; ok && prev >= entry.TS {
			continue
		}
		newest[name] = entry.TS
		vaults[name] = entry.Path
	}

	return vaults, nil
}

// obsidianConfigPath returns the platform-appropriate path to obsidian.json.
func obsidianConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		// macOS fallback
		configDir = filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(configDir, "obsidian", "obsidian.json")
}

// skipHiddenDir reports whether a WalkDir callback should skip this directory.
// Hidden directories (prefixed with ".") and ".trash" are skipped, except the
// walk root itself -- a vault directory may legitimately be dot-prefixed
// (e.g., ".vault") and must never be skipped.
func skipHiddenDir(path string, d os.DirEntry, walkRoot string) bool {
	if !d.IsDir() {
		return false
	}
	if path == walkRoot {
		return false
	}
	name := d.Name()
	return strings.HasPrefix(name, ".") || name == ".trash"
}

// resolveNote finds a note by title within the vault using the vault index.
// Kept as a package-level helper for callers that construct a Vault directly;
// resolution order is documented on Vault.resolve.
func resolveNote(vaultDir, title string) (string, error) {
	v := &Vault{dir: vaultDir}
	return v.resolve(title)
}
