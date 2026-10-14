package server

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

// A start, restart or stop is reported while it runs, and a second one is refused (423, not the
// 409 that asks to end running sessions).
func TestOps(t *testing.T) {
	o := newOps()
	_, end, err := o.begin("api", opStopping)
	if err != nil || o.of("api") != opStopping {
		t.Fatalf("begin: %v, of %q", err, o.of("api"))
	}
	_, _, err = o.begin("api", opRestarting)
	var busy errBusy
	if !errors.As(err, &busy) || err.Error() != "api is already stopping" {
		t.Fatalf("second op: %v", err)
	}
	w := httptest.NewRecorder()
	apiError(w, err)
	if w.Code != statusBusy {
		t.Errorf("status %d", w.Code)
	}
	if _, _, err := o.begin("web", opStarting); err != nil {
		t.Errorf("another project is independent: %v", err)
	}
	end()
	if o.of("api") != "" {
		t.Error("op not forgotten")
	}
}

// Quitting cancels every op, waits for them, and refuses new ones.
func TestOpsCancelAll(t *testing.T) {
	o := newOps()
	ctx, end, _ := o.begin("api", opStarting)
	go func() {
		<-ctx.Done() // the op notices, cleans up and ends
		end()
	}()
	if got := o.cancelAll(); len(got) != 1 || got[0] != "api" {
		t.Fatalf("interrupted: %v", got)
	}
	if !o.wait(time.Second) {
		t.Fatal("the op didn't end")
	}
	if _, _, err := o.begin("web", opStarting); !errors.Is(err, errClosing) {
		t.Fatalf("no new op while quitting: %v", err)
	}
}
