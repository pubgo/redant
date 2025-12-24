package redant

// HandlerFunc handles an Invocation of a command.
type HandlerFunc func(i *Invocation) error
