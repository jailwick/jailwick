//go:build windows

package untrusted

import (
	"fmt"
	"io/fs"
	"os"
)

const nonBlock = 0

// openNoFollow opens path, refusing a link at its last part. Windows has no O_NOFOLLOW: the
// check comes first (sandboxes don't run on the Windows host's own file system).
func openNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	if st, err := os.Lstat(path); err == nil && st.Mode()&fs.ModeSymlink != 0 {
		if flag&os.O_EXCL != 0 {
			return nil, fs.ErrExist
		}
		return nil, fmt.Errorf("%s: a link", path)
	}
	return os.OpenFile(path, flag, perm)
}
