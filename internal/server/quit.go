package server

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/jailwick/jailwick/internal/notify"
)

// QuitGuard keeps the app from quitting in the middle of a start, restart or stop without
// asking. ShouldQuit answers the app's "may I quit?" (⌘Q, the Dock, the menu bar's Quit): yes when
// nothing runs; otherwise no for now, and it asks in the background. On "Quit anyway" it cleans
// up (Shutdown) and quits; on Cancel nothing happens. The app never blocks while it asks.
type QuitGuard struct {
	Server  *Server
	Quit    func()                               // the app's Quit; it asks ShouldQuit again
	Confirm func(title, message, ok string) bool // nil: notify.Confirm

	allowed atomic.Bool // cleaned up: the next ShouldQuit says yes
	asking  atomic.Bool // a question is up: don't ask twice
}

// quitTimeout bounds how long quitting waits for what it cancels.
const quitTimeout = 30 * time.Second

func (q *QuitGuard) ShouldQuit() bool {
	if q.allowed.Load() {
		return true
	}
	busy := q.Server.Busy()
	if len(busy) == 0 {
		return true
	}
	if q.asking.Swap(true) {
		return false
	}
	go q.ask(busy)
	return false
}

func (q *QuitGuard) ask(busy []string) {
	defer q.asking.Store(false)
	confirm := q.Confirm
	if confirm == nil {
		confirm = notify.Confirm
	}
	msg := strings.Join(busy, "\n") + "\n\nQuitting stops it, and a sandbox it left half started is stopped " +
		"(its data is kept). The next Start picks up from there."
	if !confirm("Quit jailwick?", msg, "Quit anyway") {
		return
	}
	q.Server.Shutdown(quitTimeout)
	q.allowed.Store(true)
	q.Quit()
}
