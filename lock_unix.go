//go:build !windows

package vlt

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockVault acquires an advisory lock on the vault directory.
// If exclusive is true an exclusive (write) lock is taken; otherwise a shared
// (read) lock is taken. The returned function releases the lock.
func LockVault(vaultDir string, exclusive bool) (func(), error) {
	p := filepath.Join(vaultDir, LockFileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	fd := int(f.Fd()) // #nosec G115 -- file descriptors fit in int
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(fd, how); err != nil {
		f.Close()
		return nil, err
	}

	return func() {
		syscall.Flock(fd, syscall.LOCK_UN)
		f.Close()
	}, nil
}
