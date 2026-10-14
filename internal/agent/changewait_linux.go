//go:build linux

package agent

import (
	"context"
	"time"

	"golang.org/x/sys/unix"
)

// inotifyMask is what changes a watched folder: a file created, written, moved or removed.
const inotifyMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO |
	unix.IN_MOVED_FROM | unix.IN_ATTRIB

// changeWaiter waits on inotify: one instance for the whole watch, so a change made while the
// status is being built waits in the kernel's queue rather than being missed. Folders are
// (re)added at each wait, as they may appear later. Without inotify it falls back to the timer.
func changeWaiter(dirs func() []string) (wait func(ctx context.Context, max time.Duration), closeFn func()) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return timerWaiter(dirs)
	}
	var wake [2]int // ends a wait when ctx does
	if err := unix.Pipe2(wake[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		unix.Close(fd)
		return timerWaiter(dirs)
	}
	wait = func(ctx context.Context, max time.Duration) {
		for _, d := range dirs() {
			_, _ = unix.InotifyAddWatch(fd, d, inotifyMask) // a missing folder is watched once it exists
		}
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_, _ = unix.Write(wake[1], []byte{0})
			case <-done:
			}
		}()
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}, {Fd: int32(wake[0]), Events: unix.POLLIN}}
		n, _ := unix.Poll(fds, int(max.Milliseconds()))
		drain(wake[0]) // a cancel's byte mustn't wake the next wait
		if n > 0 && fds[0].Revents&unix.POLLIN != 0 {
			// A burst (Claude rewrites its file several times) makes one look.
			select {
			case <-ctx.Done():
			case <-time.After(settle):
			}
			drain(fd)
		}
	}
	return wait, func() { unix.Close(fd); unix.Close(wake[0]); unix.Close(wake[1]) }
}

// drain discards what is queued on a non-blocking fd: inotify's events (what changed doesn't
// matter, only that something did) or the wake pipe's bytes.
func drain(fd int) {
	buf := make([]byte, 64<<10)
	for {
		if n, err := unix.Read(fd, buf); n <= 0 || err != nil {
			return
		}
	}
}
