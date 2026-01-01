package redant

import "context"

// HandlerFunc handles an Invocation of a command.
type HandlerFunc func(ctx context.Context, inv *Invocation) error
