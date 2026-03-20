package redant

import "sync"

// CommandDispatchInput describes the dispatch context before flag parsing.
type CommandDispatchInput struct {
	// Parent is the invocation root command for the current run() frame.
	Parent *Command
	// Command is the command resolved by default dispatch.
	Command *Command
	// RawAllArgs are the original args before subcommand consumption.
	RawAllArgs []string
	// RemainingArgs are args after the consumed subcommand path.
	RemainingArgs []string
	// Consumed is the number of consumed command path args.
	Consumed int
}

// CommandDispatchOutput describes an override result returned by a dispatch hook.
type CommandDispatchOutput struct {
	// Command is the overridden target command.
	Command *Command
	// Args are the overridden args for subsequent flag parsing.
	Args []string
}

// CommandDispatchHook allows extensions to override command dispatch without
// coupling core to specific extension modules.
type CommandDispatchHook func(input CommandDispatchInput) (CommandDispatchOutput, bool)

var (
	commandDispatchHooksMu sync.RWMutex
	commandDispatchHooks   []CommandDispatchHook
)

// RegisterCommandDispatchHook registers a command dispatch hook.
func RegisterCommandDispatchHook(hook CommandDispatchHook) {
	if hook == nil {
		return
	}
	commandDispatchHooksMu.Lock()
	defer commandDispatchHooksMu.Unlock()
	commandDispatchHooks = append(commandDispatchHooks, hook)
}

func applyCommandDispatchHooks(input CommandDispatchInput) (CommandDispatchOutput, bool) {
	commandDispatchHooksMu.RLock()
	hooks := append([]CommandDispatchHook(nil), commandDispatchHooks...)
	commandDispatchHooksMu.RUnlock()

	for _, hook := range hooks {
		if out, ok := hook(input); ok {
			return out, true
		}
	}

	return CommandDispatchOutput{}, false
}
