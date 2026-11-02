package auth

import "context"

// WithCallerName notes an agent key's name on a context the way the guard does, for tests of the handlers that read it.
func WithCallerName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, callerKey{}, name)
}

// WithViewer marks a context as a viewer's the way the guard does, for tests of the handlers that keep a viewer to its reach.
func WithViewer(ctx context.Context) context.Context {
	return context.WithValue(ctx, viewerKey{}, true)
}
