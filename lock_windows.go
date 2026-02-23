//go:build windows

package main

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
	lockfileExclusiveLock = 0x00000002
)

func lockVault(vaultDir string, exclusive bool) (func(), error) {
	p := filepath.Join(vaultDir, lockFileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	h := syscall.Handle(f.Fd())
	ol := new(syscall.Overlapped)

	var flags uint32
	if exclusive {
		flags = lockfileExclusiveLock
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
		return nil, e1
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
	}, nil
}
