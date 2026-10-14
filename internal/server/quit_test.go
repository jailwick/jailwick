package server

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestQuitGuard(t *testing.T) {
	s := testServer(t, "")
	quit := make(chan bool, 1)
	var answer atomic.Bool // Confirm is asked from another goroutine
	g := &QuitGuard{Server: s, Quit: func() { quit <- true }, Confirm: func(_, _, _ string) bool { return answer.Load() }}

	if !g.ShouldQuit() {
		t.Fatal("nothing runs: quit at once")
	}

	ctx, end, _ := s.ops.begin("api", opStarting)
	go func() { <-ctx.Done(); end() }() // the start notices it's cancelled
	if g.ShouldQuit() {
		t.Fatal("a start runs: ask first")
	}
	time.Sleep(50 * time.Millisecond)
	if len(quit) != 0 || s.ops.of("api") == "" {
		t.Fatal("Cancel: nothing happens")
	}

	answer.Store(true)
	if g.ShouldQuit() {
		t.Fatal("still asks")
	}
	select {
	case <-quit:
	case <-time.After(5 * time.Second):
		t.Fatal("Quit anyway: it quits")
	}
	if s.ops.of("api") != "" || !g.ShouldQuit() {
		t.Fatal("the start was cancelled, and quitting now goes through")
	}
}
