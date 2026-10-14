package cli

import (
	"net"
	"testing"
)

// The browser app comes back on the port it had: the page keeps its origin, and so the browser's
// storage for it.
func TestListenAppKeepsItsPort(t *testing.T) {
	t.Setenv("JAILWICK_STATE", t.TempDir())
	first, err := listenApp(0)
	if err != nil {
		t.Fatal(err)
	}
	port := first.Addr().(*net.TCPAddr).Port
	first.Close()
	again, err := listenApp(0)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.Addr().(*net.TCPAddr).Port; got != port {
		t.Fatalf("port %d, then %d", port, got)
	}
}
