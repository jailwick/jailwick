//go:build !unix

package session

import (
	"context"
	"os"
)

// relayed reports whether a session can be relayed: not here, so it attaches directly.
func relayed() bool { return false }

func relay(context.Context, func(), func(*os.File) error) error { panic("unreachable") }
