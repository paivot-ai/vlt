//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockVault acquires an advisory lock on the vault directory.
// If exclusive is true an exclusive (write) lock is taken; otherwise a shared
// (read) lock is taken. The returned function releases the lock.
func lockVault(vaultDir string, exclusive bool) (func(), error) {
	p := filepath.Join(vaultDir, lockFileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, err
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
