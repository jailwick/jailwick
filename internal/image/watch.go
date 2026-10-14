package image

import "context"

// Event is a build's progress: Step and Log when a build starts, then Line for each line of its
// output, with Done and Total when the line says how far the build is.
type Event struct {
	Step  string // what is being built
	Log   string // the file with the build's whole output
	Line  string
	Done  int // steps of the build started, of Total
	Total int
}

type watchKey struct{}

// Watch returns a context whose image builds report their progress to fn. Builders are shared by
// projects, so progress follows the request (one project's start) rather than the builder.
func Watch(ctx context.Context, fn func(Event)) context.Context {
	return context.WithValue(ctx, watchKey{}, fn)
}

func watcher(ctx context.Context) func(Event) {
	fn, _ := ctx.Value(watchKey{}).(func(Event))
	return fn
}
