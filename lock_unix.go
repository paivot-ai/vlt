//go:build !windows

package vlt

import (
	"os"
	"path/filepath"
	"syscall"
)

// tryLockVault attempts a non-blocking advisory lock on the vault directory.
// Returns (release, busy, err): busy is true when another process holds a
// conflicting lock; err reports any other failure. The retry/timeout policy
// lives in LockVault (lock.go).
func tryLockVault(vaultDir string, exclusive bool) (func(), bool, error) {
	p := filepath.Join(vaultDir, LockFileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, false, err
	}

	fd := int(f.Fd()) // #nosec G115 -- file descriptors fit in int
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(fd, how|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, true, nil
		}
		return nil, false, err
	}

	return func() {
		syscall.Flock(fd, syscall.LOCK_UN)
		f.Close()
	}, false, nil
}
