//go:build windows

package sandbox

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes f's exclusive lock without waiting; ok is false when another holds it.
func tryLock(f *os.File) (ok bool, err error) {
	var o windows.Overlapped
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &o)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

func unlock(f *os.File) error {
	var o windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &o)
}
