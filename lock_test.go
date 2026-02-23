//go:build !windows

package vlt

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLockVaultExclusive(t *testing.T) {
	dir := t.TempDir()

	unlock, err := LockVault(dir, true)
	if err != nil {
		t.Fatalf("first exclusive lock: %v", err)
	}
	defer unlock()

	// Try a non-blocking exclusive lock on the same file from this process.
	// It should fail with EWOULDBLOCK because we already hold the lock.
	p := filepath.Join(dir, LockFileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer f.Close()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		t.Fatal("expected EWOULDBLOCK for second exclusive lock, got nil")
	}
}

func TestLockVaultSharedCompatible(t *testing.T) {
	dir := t.TempDir()

	unlock1, err := LockVault(dir, false)
	if err != nil {
		t.Fatalf("first shared lock: %v", err)
	}
	defer unlock1()

	unlock2, err := LockVault(dir, false)
	if err != nil {
		t.Fatalf("second shared lock: %v", err)
	}
	defer unlock2()
}

func TestLockVaultUnlockReleases(t *testing.T) {
	dir := t.TempDir()

	unlock, err := LockVault(dir, true)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	unlock()

	// Should succeed immediately since the lock was released.
	done := make(chan error, 1)
	go func() {
		u, err := LockVault(dir, true)
		if err == nil {
			u()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("re-acquire after unlock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lock after unlock")
	}
}

func TestLockVaultCreatesLockFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, LockFileName)

	if _, err := os.Stat(p); err == nil {
		t.Fatal("lock file should not exist before lockVault")
	}

	unlock, err := LockVault(dir, true)
	if err != nil {
		t.Fatalf("lockVault: %v", err)
	}
	defer unlock()

	if _, err := os.Stat(p); err != nil {
		t.Fatalf("lock file should exist after lockVault: %v", err)
	}
}

func TestIsWriteCommand(t *testing.T) {
	writes := []string{
		"create", "append", "prepend", "write", "patch",
		"move", "delete", "property:set", "property:remove",
		"daily", "templates:apply", "bookmarks:add", "bookmarks:remove",
	}
	for _, cmd := range writes {
		if !IsWriteCommand(cmd) {
			t.Errorf("IsWriteCommand(%q) = false, want true", cmd)
		}
	}

	reads := []string{
		"read", "search", "properties", "backlinks", "links",
		"orphans", "unresolved", "tags", "tag", "files",
		"tasks", "templates", "bookmarks", "uri",
	}
	for _, cmd := range reads {
		if IsWriteCommand(cmd) {
			t.Errorf("IsWriteCommand(%q) = true, want false", cmd)
		}
	}
}
