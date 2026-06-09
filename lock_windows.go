//go:build windows

package vlt

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Windows file locking via kernel32.dll LockFileEx / UnlockFileEx.
// NOTE: this implementation is untested on Windows. Please report issues at
// the project's issue tracker.

var (
	modkernel32      = syscall.MustLoadDLL("kernel32.dll")
	procLockFileEx   = modkernel32.MustFindProc("LockFileEx")
	procUnlockFileEx = modkernel32.MustFindProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001

	errorLockViolation = 33 // ERROR_LOCK_VIOLATION
)

// tryLockVault attempts a non-blocking lock on the vault directory.
// Returns (release, busy, err): busy is true when another process holds a
// conflicting lock; err reports any other failure. The retry/timeout policy
// lives in LockVault (lock.go).
func tryLockVault(vaultDir string, exclusive bool) (func(), bool, error) {
	p := filepath.Join(vaultDir, LockFileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, false, err
	}

	h := syscall.Handle(f.Fd())
	ol := new(syscall.Overlapped)

	flags := uint32(lockfileFailImmediately)
	if exclusive {
		flags |= lockfileExclusiveLock
	}

	r1, _, e1 := procLockFileEx.Call(
		uintptr(h),
		uintptr(flags),
		0, // reserved
		1, // nNumberOfBytesToLockLow
		0, // nNumberOfBytesToLockHigh
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		f.Close()
		if errno, ok := e1.(syscall.Errno); ok && errno == errorLockViolation {
			return nil, true, nil
		}
		return nil, false, e1
	}

	return func() {
		procUnlockFileEx.Call(
			uintptr(h),
			0,
			1,
			0,
			uintptr(unsafe.Pointer(ol)),
		)
		f.Close()
	}, false, nil
}
