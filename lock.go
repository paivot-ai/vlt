package vlt

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteCommands lists CLI commands that require an exclusive vault lock; all others use a shared lock.
var writeCommands = map[string]bool{
	"create":                true,
	"append":                true,
	"prepend":               true,
	"write":                 true,
	"patch":                 true,
	"move":                  true,
	"delete":                true,
	"property:set":          true,
	"property:remove":       true,
	"daily":                 true,
	"templates:apply":       true,
	"bookmarks:add":         true,
	"bookmarks:remove":      true,
	"integrity:baseline":    true,
	"integrity:acknowledge": true,
}

// IsWriteCommand returns true if cmd is a write command requiring an exclusive lock.
func IsWriteCommand(cmd string) bool {
	return writeCommands[cmd]
}

// LockFileName is the advisory lock file placed in the vault root.
const LockFileName = ".vlt.lock"

// defaultLockTimeout bounds how long LockVault waits for a contended lock so
// a wedged process holding the lock cannot hang every future command forever.
const defaultLockTimeout = 10 * time.Second

// lockRetryInterval is the polling interval while waiting for a contended lock.
const lockRetryInterval = 50 * time.Millisecond

// lockTimeout returns the lock acquisition timeout and whether it is bounded.
// VLT_LOCK_TIMEOUT accepts a Go duration ("30s", "2m"); a zero or negative
// value disables the timeout (wait forever).
func lockTimeout() (time.Duration, bool) {
	if s := os.Getenv("VLT_LOCK_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			if d <= 0 {
				return 0, false
			}
			return d, true
		}
		fmt.Fprintf(os.Stderr, "vlt: warning: invalid VLT_LOCK_TIMEOUT %q; using default %s\n", s, defaultLockTimeout)
	}
	return defaultLockTimeout, true
}

// LockVault acquires an advisory lock on the vault directory.
// If exclusive is true an exclusive (write) lock is taken; otherwise a shared
// (read) lock is taken. The returned function releases the lock.
// Acquisition is non-blocking with retries; after the configured timeout
// (default 10s, see VLT_LOCK_TIMEOUT) an error is returned instead of
// waiting forever on a lock held by a wedged process.
func LockVault(vaultDir string, exclusive bool) (func(), error) {
	timeout, bounded := lockTimeout()
	deadline := time.Now().Add(timeout)

	for {
		release, busy, err := tryLockVault(vaultDir, exclusive)
		if err != nil {
			return nil, err
		}
		if !busy {
			return release, nil
		}
		if bounded && time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s waiting for vault lock %s -- another vlt process may be holding it (set VLT_LOCK_TIMEOUT to adjust)",
				timeout, filepath.Join(vaultDir, LockFileName))
		}
		time.Sleep(lockRetryInterval)
	}
}
