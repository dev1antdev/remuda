package auth

import "context"

type callerKey struct{}

// CallerName is the name of the agent key a request came with (the label of the machine it was placed on), "" for
// the token or a signed-in device. The guard sets it; WithCallerName is for tests.
func CallerName(ctx context.Context) string {
	name, _ := ctx.Value(callerKey{}).(string)
	return name
}

// WithCallerName notes an agent key's name on a context.
func WithCallerName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, callerKey{}, name)
}
