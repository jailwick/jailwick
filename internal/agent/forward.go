package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Forward listens on 127.0.0.1:local and connects each client to host:port, so env files that
// say localhost:<port> reach the sandbox's services. It runs until ctx ends.
func Forward(ctx context.Context, spec string) error {
	parts := strings.Split(spec, ":")
	if len(parts) != 3 {
		return fmt.Errorf("forward %q: want localport:host:port", spec)
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return fmt.Errorf("forward %q: bad local port", spec)
	}
	target := net.JoinHostPort(parts[1], parts[2])
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort("127.0.0.1", parts[0]))
	if err != nil {
		return fmt.Errorf("forward %s: %w", spec, err)
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("forward %s: %v", spec, err)
			continue
		}
		go pipe(ctx, conn, target)
	}
}

func pipe(ctx context.Context, client net.Conn, target string) {
	defer client.Close()
	var d net.Dialer
	server, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		log.Printf("forward to %s: %v", target, err)
		return
	}
	defer server.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	copyHalf := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}
	go copyHalf(server, client)
	go copyHalf(client, server)
	wg.Wait()
}
