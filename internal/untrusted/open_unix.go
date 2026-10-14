//go:build !windows

package untrusted

import (
	"os"
	"syscall"
)

// nonBlock opens a pipe without waiting for a writer: the caller then sees it isn't a file.
const nonBlock = syscall.O_NONBLOCK

// openNoFollow opens path, refusing a link at its last part.
func openNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag|syscall.O_NOFOLLOW, perm)
}
