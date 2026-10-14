//go:build unix

package sandbox

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes f's exclusive lock without waiting; ok is false when another holds it.
func tryLock(f *os.File) (ok bool, err error) {
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
