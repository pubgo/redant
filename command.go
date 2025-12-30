package redant

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// Command describes an executable command.
type Command struct {
	// parent is the parent command.
	parent *Command

	// Children is a list of direct descendants.
	Children []*Command

	// Use is provided in form "command [flags] [args...]".
	Use string

	// Aliases is a list of alternative names for the command.
	Aliases []string

	// Short is a one-line description of the command.
	Short string

	// Hidden determines whether the command should be hidden from help.
	Hidden bool

	// Deprecated indicates whether this command is deprecated.
	// If empty, the command is not deprecated.
	// If set, the value is used as the deprecation message.
	Deprecated string `json:"deprecated,omitempty"`

	// RawArgs determines whether the command should receive unparsed arguments.
	// No flags are parsed when set, and the command is responsible for parsing
	// its own flags.
	RawArgs bool

	// Long is a detailed description of the command,
	// presented on its help page. It may contain examples.
	Long    string
	Options OptionSet
	Args    ArgSet

	// Middleware is called before the Handler.
	// Use Chain() to combine multiple middlewares.
	Middleware MiddlewareFunc
	Handler    HandlerFunc
}

func ascendingSortFn[T cmp.Ordered](a, b T) int {
	if a < b {
		return -1
	} else if a == b {
		return 0
	}
	return 1
}

// init performs initialization and linting on the command and all its children.
func (c *Command) init() error {
	if c.Use == "" {
		c.Use = "unnamed"
	}
	var merr error

	// Add global flags to the root command only
	if c.parent == nil {
		globalFlags := GlobalFlags()
		c.Options = append(c.Options, globalFlags...)
	}

	for i := range c.Options {
		opt := &c.Options[i]
		// Validate that option has an identifier (Flag or Env)
		if opt.Flag == "" && len(opt.Envs) == 0 {
			merr = errors.Join(merr, fmt.Errorf("option must have a Flag or Env field"))
		}
		if opt.Description != "" {
			// Get option name for error messages
			optName := opt.Flag
			if optName == "" && len(opt.Envs) > 0 {
				optName = opt.Envs[0]
			}

			opt.Description = strings.Trim(strings.ToTitle(strings.TrimSpace(opt.Description)), ".") + "."
		}
	}

	slices.SortFunc(c.Options, func(a, b Option) int {
		// Use Flag for sorting, fallback to Env if Flag is empty
		nameA := a.Flag
		if nameA == "" && len(a.Envs) > 0 {
			nameA = a.Envs[0]
		}
		nameB := b.Flag
		if nameB == "" && len(b.Envs) > 0 {
			nameB = b.Envs[0]
		}
		return ascendingSortFn(nameA, nameB)
	})
	slices.SortFunc(c.Children, func(a, b *Command) int {
		return ascendingSortFn(a.Name(), b.Name())
	})
	for _, child := range c.Children {
		child.parent = c
		err := child.init()
		if err != nil {
			merr = errors.Join(merr, fmt.Errorf("command %v: %w", child.Name(), err))
		}
	}
	return merr
}

// Name returns the first word in the Use string.
func (c *Command) Name() string {
	return strings.Split(c.Use, " ")[0]
}

// FullName returns the full invocation name of the command,
// as seen on the command line.
func (c *Command) FullName() string {
	var names []string
	if c.parent != nil {
		names = append(names, c.parent.FullName())
	}
	names = append(names, c.Name())
	return strings.Join(names, " ")
}

// Parent returns the parent command of this command.
func (c *Command) Parent() *Command {
	return c.parent
}

func (c *Command) FullUsage() string {
	var uses []string
	if c.parent != nil {
		uses = append(uses, c.parent.FullName())
	}
	uses = append(uses, c.Use)
	return strings.Join(uses, " ")
}

// FullOptions returns the options of the command and its parents.
func (c *Command) FullOptions() OptionSet {
	var opts OptionSet
	if c.parent != nil {
		opts = append(opts, c.parent.FullOptions()...)
	}
	opts = append(opts, c.Options...)
	return opts
}

// GetGlobalFlags returns the global flags from the root command
// All non-hidden options in the root command are considered global flags
func (c *Command) GetGlobalFlags() OptionSet {
	// Traverse to the root command
	root := c
	for root.parent != nil {
		root = root.parent
	}

	// Return all non-hidden options from root command as global flags
	var globalFlags OptionSet
	for _, opt := range root.Options {
		if opt.Flag != "" && !opt.Hidden {
			globalFlags = append(globalFlags, opt)
		}
	}
	return globalFlags
}

// Invoke creates a new invocation of the command, with
// stdio discarded.
//
// The returned invocation is not live until Run() is called.
func (c *Command) Invoke(args ...string) *Invocation {
	return &Invocation{
		Command: c,
		Args:    args,
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Stdin:   strings.NewReader(""),
	}
}

func (c *Command) Run(ctx context.Context) error {
	i := &Invocation{
		Command: c,
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Stdin:   strings.NewReader(""),
	}
	return i.WithOS().WithContext(ctx).Run()
}

// Invocation represents an instance of a command being executed.
type Invocation struct {
	ctx     context.Context
	Command *Command
	Flags   *pflag.FlagSet

	// Args is reduced into the remaining arguments after parsing flags
	// during Run.
	Args []string

	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// Annotations is a map of arbitrary annotations to attach to the invocation.
	Annotations map[string]any

	// testing
	signalNotifyContext func(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc)
}

// WithOS returns the invocation as a main package, filling in the invocation's unset
// fields with OS defaults.
func (inv *Invocation) WithOS() *Invocation {
	return inv.with(func(i *Invocation) {
		i.Stdout = os.Stdout
		i.Stderr = os.Stderr
		i.Stdin = os.Stdin
		i.Args = os.Args[1:]
	})
}

// WithTestSignalNotifyContext allows overriding the default implementation of SignalNotifyContext.
// This should only be used in testing.
func (inv *Invocation) WithTestSignalNotifyContext(
	_ testing.TB, // ensure we only call this from tests
	f func(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc),
) *Invocation {
	return inv.with(func(i *Invocation) {
		i.signalNotifyContext = f
	})
}

// SignalNotifyContext is equivalent to signal.NotifyContext, but supports being overridden in
// tests.
func (inv *Invocation) SignalNotifyContext(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc) {
	if inv.signalNotifyContext == nil {
		return signal.NotifyContext(parent, signals...)
	}
	return inv.signalNotifyContext(parent, signals...)
}

func (inv *Invocation) WithTestParsedFlags(
	_ testing.TB, // ensure we only call this from tests
	parsedFlags *pflag.FlagSet,
) *Invocation {
	return inv.with(func(i *Invocation) {
		i.Flags = parsedFlags
	})
}

func (inv *Invocation) Context() context.Context {
	if inv.ctx == nil {
		return context.Background()
	}
	return inv.ctx
}

func (inv *Invocation) ParsedFlags() *pflag.FlagSet {
	if inv.Flags == nil {
		panic("flags not parsed, has Run() been called?")
	}
	return inv.Flags
}

type runState struct {
	allArgs      []string
	commandDepth int

	flagParseErr error
}

func copyFlagSetWithout(fs *pflag.FlagSet, without string) *pflag.FlagSet {
	fs2 := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs2.Usage = func() {}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == without {
			return
		}
		fs2.AddFlag(f)
	})
	return fs2
}

func (inv *Invocation) CurWords() (prev, cur string) {
	switch len(inv.Args) {
	// All the shells we support will supply at least one argument (empty string),
	// but we don't want to panic.
	case 0:
		cur = ""
		prev = ""
	case 1:
		cur = inv.Args[0]
		prev = ""
	default:
		cur = inv.Args[len(inv.Args)-1]
		prev = inv.Args[len(inv.Args)-2]
	}
	return prev, cur
}

func getExecCommand(parentCmd *Command, commands map[string]*Command, args []string) (*Command, int) {
	for i := 0; i < len(args); i++ {
		// Stop at first flag
		if strings.HasPrefix(args[i], "-") {
			args = args[:i]
			break
		}

		// If the argument contains '=', treat it as a parameter and stop processing
		if strings.Contains(args[i], "=") {
			args = args[:i]
			break
		}
	}

	// Check if args is empty
	if len(args) == 0 {
		return parentCmd, 0
	}

	// Try to find command by exact match first
	if cmd, exists := commands[args[0]]; exists {
		return cmd, 1
	}

	// Try to find command by colon-separated path
	if strings.Contains(args[0], ":") {
		if cmd, exists := commands[args[0]]; exists {
			return cmd, 1
		}
		// If not found, try to find parent command and process subcommands
		parts := strings.Split(args[0], ":")
		if len(parts) > 0 {
			cmd := parentCmd
			for _, part := range parts {
				found := false
				for _, child := range cmd.Children {
					if child.Name() == part {
						cmd = child
						found = true
						break
					}
				}
				if !found {
					return parentCmd, 0 // Return parent if path not found
				}
			}
			return cmd, 1
		}
	}

	// Handle multiple arguments - look for command in the chain
	currentCmd := parentCmd
	consumedArgs := 0
	for i, arg := range args {
		found := false
		for _, child := range currentCmd.Children {
			if child.Name() == arg {
				currentCmd = child
				consumedArgs = i + 1
				found = true
				break
			}
		}
		if !found {
			break // Stop if command not found
		}
	}

	return currentCmd, consumedArgs
}

func getCommands(cmd *Command, parentName string) map[string]*Command {
	if cmd == nil {
		return nil
	}

	commandMap := make(map[string]*Command)

	name := parentName + ":" + cmd.Name()
	if parentName == "" {
		name = cmd.Name()
	}

	commandMap[name] = cmd
	for _, child := range cmd.Children {
		for n, command := range getCommands(child, name) {
			if commandMap[n] != nil {
				log.Panicf("duplicate command name: %s", n)
			}
			commandMap[n] = command
		}
	}

	return commandMap
}

func (inv *Invocation) setParentCommand(parent *Command, children []*Command) {
	for _, child := range children {
		child.parent = parent
		inv.setParentCommand(child, child.Children)
	}
}

// run recursively executes the command and its children.
// allArgs is wired through the stack so that global flags can be accepted
// anywhere in the command invocation.
func (inv *Invocation) run(state *runState) error {
	parent := inv.Command
	inv.setParentCommand(inv.Command, inv.Command.Children)

	if inv.Command.Deprecated != "" {
		fmt.Fprintf(inv.Stderr, "%s %q is deprecated!. %s\n",
			prettyHeader("warning"),
			inv.Command.FullName(),
			inv.Command.Deprecated,
		)
	}

	// Organize command tree
	commands := getCommands(parent, "")

	// Use the command returned by getExecCommand
	var consumed int
	inv.Command, consumed = getExecCommand(parent, commands, state.allArgs)
	if consumed > 0 && consumed <= len(state.allArgs) {
		state.allArgs = state.allArgs[consumed:]
	}

	// Check for global flags before proceeding
	if inv.Flags == nil {
		inv.Flags = pflag.NewFlagSet(inv.Command.Name(), pflag.ContinueOnError)
		// We handle Usage ourselves.
		inv.Flags.Usage = func() {}
	}

	// Add global flags to the flag set
	globalFlags := inv.Command.GetGlobalFlags()
	globalFlagSet := globalFlags.FlagSet(inv.Command.Name())
	globalFlagSet.VisitAll(func(f *pflag.Flag) {
		if inv.Flags.Lookup(f.Name) == nil {
			inv.Flags.AddFlag(f)
		}
	})

	// If we find a duplicate flag, we want the deeper command's flag to override
	// the shallow one. Unfortunately, pflag has no way to remove a flag, so we
	// have to create a copy of the flagset without a value.
	inv.Command.Options.FlagSet(inv.Command.Name()).VisitAll(func(f *pflag.Flag) {
		if inv.Flags.Lookup(f.Name) != nil {
			inv.Flags = copyFlagSetWithout(inv.Flags, f.Name)
		}
		inv.Flags.AddFlag(f)
	})

	var parsedArgs []string

	// Parse flags first to get the correct command context
	if !inv.Command.RawArgs {
		// Flag parsing will fail on intermediate commands in the command tree,
		// so we check the error after looking for a child command.
		state.flagParseErr = inv.Flags.Parse(state.allArgs)
		parsedArgs = inv.Flags.Args()
	}

	// Handle global flags
	if inv.Flags != nil {
		// Check for --list-commands flag
		if listCommands, err := inv.Flags.GetBool("list-commands"); err == nil && listCommands {
			PrintCommands(parent) // Use parent to show full tree
			return nil
		}

		// Check for --list-flags flag
		if listFlags, err := inv.Flags.GetBool("list-flags"); err == nil && listFlags {
			PrintFlags(parent)
			return nil
		}
	}

	// Run child command if found (next child only)
	// We must setParentCommand subcommand detection after flag parsing so we don't mistake flag
	// values for subcommand names.
	if len(parsedArgs) > state.commandDepth {
		nextArg := parsedArgs[state.commandDepth]
		if child, ok := inv.Command.children()[nextArg]; ok {
			child.parent = inv.Command
			inv.Command = child
			state.commandDepth++
			return inv.run(state)
		}
	}

	// At this point, we have the final command, so collect args
	// Query string, form data, and JSON format args should be kept as args
	// for the handler to process, not parsed into flags
	remainingArgs := make([]string, 0, len(state.allArgs))
	for _, arg := range state.allArgs {
		// Skip flags
		if strings.HasPrefix(arg, "-") {
			remainingArgs = append(remainingArgs, arg)
			continue
		}

		// Check if the argument is in JSON format (starts with { or [)
		// JSON args should be kept as args, not parsed into flags
		trimmedArg := strings.TrimSpace(arg)
		if (strings.HasPrefix(trimmedArg, "{") && strings.HasSuffix(trimmedArg, "}")) ||
			(strings.HasPrefix(trimmedArg, "[") && strings.HasSuffix(trimmedArg, "]")) {
			// JSON format args should be kept as args for handler to process
			remainingArgs = append(remainingArgs, arg)
			continue
		}

		// Check if the argument is in query string or form format
		// These should be treated as args, not flags
		if strings.Contains(arg, "=") {
			// Query string and form format args should be kept as args
			// They will be available in inv.Args for the handler to process
			remainingArgs = append(remainingArgs, arg)
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	// Update state.allArgs with remaining args and re-parse flags
	state.allArgs = remainingArgs
	if !inv.Command.RawArgs {
		state.flagParseErr = inv.Flags.Parse(state.allArgs)
		parsedArgs = inv.Flags.Args()
	} else {
		parsedArgs = state.allArgs
	}

	ignoreFlagParseErrors := inv.Command.RawArgs

	// Flag parse errors are irrelevant for raw args commands.
	if !ignoreFlagParseErrors && state.flagParseErr != nil && !errors.Is(state.flagParseErr, pflag.ErrHelp) {
		return fmt.Errorf(
			"parsing flags (%v) for %q: %w",
			state.allArgs,
			inv.Command.FullName(), state.flagParseErr,
		)
	}

	// Check for help flag before validating required options
	isHelpRequested := false
	if inv.Flags != nil {
		if help, err := inv.Flags.GetBool("help"); err == nil && help {
			isHelpRequested = true
		} else if h, err := inv.Flags.GetBool("h"); err == nil && h {
			isHelpRequested = true
		}
	}

	// All options should be set. Check all required options have sources,
	// meaning they were set by the user in some way (env, flag, etc).
	// Don't validate required flags if help was requested or if there's a help error.
	if !isHelpRequested && !errors.Is(state.flagParseErr, pflag.ErrHelp) {
		var missing []string
		for _, opt := range inv.Command.Options {
			if opt.Required {
				// Required means the flag must have a value, not that the flag must be present.
				// A flag has a value if:
				// 1. User explicitly set it (flag.Changed)
				// 2. It has a default value (opt.Default != "")
				// 3. It can be set via environment variable (opt.Envs)
				hasValue := false

				if inv.Flags != nil && opt.Flag != "" {
					if flag := inv.Flags.Lookup(opt.Flag); flag != nil {
						// Flag was explicitly set by user
						hasValue = flag.Changed
					}
				}

				// If not set by user, check if there's a default value
				if !hasValue && opt.Default != "" {
					hasValue = true
				}

				// If still no value, check if environment variable is available
				// (we can't check if env var is actually set here, but if it's configured,
				// we assume it might be set)
				if !hasValue && len(opt.Envs) > 0 {
					hasValue = true
				}

				if !hasValue {
					name := opt.Flag
					// use env as a fallback if flag is empty
					if name == "" && len(opt.Envs) > 0 {
						name = opt.Envs[0]
					}
					missing = append(missing, name)
				}
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing values for the required flags: %s", strings.Join(missing, ", "))
		}
	}

	// Execute Action callbacks for options that were set
	// Don't execute actions if help was requested
	if !isHelpRequested && !errors.Is(state.flagParseErr, pflag.ErrHelp) && inv.Flags != nil {
		// Use a map to track which flags we've already processed
		// This prevents executing Action multiple times for the same flag
		processedFlags := make(map[string]bool)

		// Get all options (including global and command-specific)
		// Process from root to current command to respect override order
		var allOptions OptionSet
		cmd := inv.Command
		for cmd != nil {
			// Prepend to maintain order (root first, then parent, then current)
			allOptions = append(cmd.Options, allOptions...)
			cmd = cmd.parent
		}

		// Execute actions in reverse order (current command first, then parent, then root)
		// This way, if a flag is overridden, we execute the most specific Action
		for i := len(allOptions) - 1; i >= 0; i-- {
			opt := allOptions[i]
			if opt.Action != nil && opt.Flag != "" && !processedFlags[opt.Flag] {
				if ff := inv.Flags.Lookup(opt.Flag); ff != nil && ff.Changed {
					if err := opt.Action(ff.Value); err != nil {
						return fmt.Errorf("action for flag %q failed: %w", opt.Flag, err)
					}
					processedFlags[opt.Flag] = true
				}
			}
		}
	}

	// Parse and assign arguments
	if inv.Command.RawArgs {
		// If we're at the root command, then the name is omitted
		// from the arguments, so we can just use the entire slice.
		if state.commandDepth == 0 {
			inv.Args = state.allArgs
		} else {
			argPos, err := findArg(inv.Command.Name(), state.allArgs, inv.Flags)
			if err != nil {
				panic(err)
			}
			inv.Args = state.allArgs[argPos+1:]
		}
	} else {
		// In non-raw-arg mode, we want to skip over flags.
		inv.Args = parsedArgs[state.commandDepth:]
	}

	// Parse args and set values to Arg.Value if Args are defined
	// Skip args parsing and validation if help was requested
	if len(inv.Command.Args) > 0 && !isHelpRequested && !errors.Is(state.flagParseErr, pflag.ErrHelp) {
		if err := parseAndSetArgs(inv.Command.Args, inv.Args); err != nil {
			return fmt.Errorf("parsing args: %w", err)
		}
	} else {
		// If Command doesn't define Args, auto-create them from parsed args
		// Name will be arg1, arg2, arg3, etc.
		if len(inv.Args) > 0 {
			autoArgs := make(ArgSet, len(inv.Args))
			for i, argStr := range inv.Args {
				autoArgs[i] = Arg{
					Name: fmt.Sprintf("arg%d", i+1),
				}
				// Create a String value for auto-created args
				strVal := StringOf(new(string))
				autoArgs[i].Value = strVal
				// Set the value
				if err := strVal.Set(argStr); err != nil {
					return fmt.Errorf("setting arg%d value: %w", i+1, err)
				}
			}
			inv.Command.Args = autoArgs
		}
	}

	// Collect all middlewares from root to current command
	// We collect from current (child) to root (parent), then reverse
	// to get [root, parent, ..., child] order. Chain() will reverse again
	// to ensure execution order is root -> parent -> ... -> child -> handler
	var middlewareChain []MiddlewareFunc
	cmd := inv.Command
	for cmd != nil {
		if cmd.Middleware != nil {
			middlewareChain = append(middlewareChain, cmd.Middleware)
		}
		cmd = cmd.parent
	}
	// Reverse to get order from root (parent) to current (child)
	// This ensures Chain() will execute them in the correct order: root -> parent -> child -> handler
	for i, j := 0, len(middlewareChain)-1; i < j; i, j = i+1, j-1 {
		middlewareChain[i], middlewareChain[j] = middlewareChain[j], middlewareChain[i]
	}

	var mw MiddlewareFunc
	if len(middlewareChain) > 0 {
		mw = Chain(middlewareChain...)
	} else {
		mw = Chain()
	}

	ctx := inv.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	inv = inv.WithContext(ctx)

	// Check for help flag
	if inv.Flags != nil {
		if help, err := inv.Flags.GetBool("help"); err == nil && help {
			return DefaultHelpFn()(ctx, inv)
		}
	}

	if inv.Command.Handler == nil || errors.Is(state.flagParseErr, pflag.ErrHelp) {
		return DefaultHelpFn()(ctx, inv)
	}

	err := mw(inv.Command.Handler)(ctx, inv)
	if err != nil {
		return &RunCommandError{
			Cmd: inv.Command,
			Err: err,
		}
	}
	return nil
}

type RunCommandError struct {
	Cmd *Command
	Err error
}

func (e *RunCommandError) Unwrap() error {
	return e.Err
}

func (e *RunCommandError) Error() string {
	return fmt.Sprintf("running command %q: %+v", e.Cmd.FullName(), e.Err)
}

// findArg returns the index of the first occurrence of arg in args, skipping
// over all flags.
func findArg(want string, args []string, fs *pflag.FlagSet) (int, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			if arg == want {
				return i, nil
			}
			continue
		}

		// This is a flag!
		if strings.Contains(arg, "=") {
			// The flag contains the value in the same arg, just skip.
			continue
		}

		// We need to check if NoOptValue is set, then we should not wait
		// for the next arg to be the value.
		f := fs.Lookup(strings.TrimLeft(arg, "-"))
		if f == nil {
			return -1, fmt.Errorf("unknown flag: %s", arg)
		}
		if f.NoOptDefVal != "" {
			continue
		}

		if i == len(args)-1 {
			return -1, fmt.Errorf("flag %s requires a value", arg)
		}

		// Skip the value.
		i++
	}

	return -1, fmt.Errorf("arg %s not found", want)
}

// parseAndSetArgs parses args and sets values to Arg.Value
// It handles different arg formats: positional, query string, form data, and JSON
func parseAndSetArgs(argsDef ArgSet, args []string) error {
	if len(args) == 0 {
		// Check for required args and set defaults
		for i, argDef := range argsDef {
			if argDef.Required && argDef.Default == "" {
				name := argDef.Name
				if name == "" {
					name = fmt.Sprintf("arg%d", i+1)
				}
				return fmt.Errorf("required argument %q is missing", name)
			}
			// Set default value if available
			if argDef.Default != "" && argDef.Value != nil {
				if err := argDef.Value.Set(argDef.Default); err != nil {
					name := argDef.Name
					if name == "" {
						name = fmt.Sprintf("arg%d", i+1)
					}
					return fmt.Errorf("setting default value for %q: %w", name, err)
				}
			}
		}
		return nil
	}

	argIndex := 0
	for i, argDef := range argsDef {
		if argIndex >= len(args) {
			// No more args provided
			if argDef.Required && argDef.Default == "" {
				name := argDef.Name
				if name == "" {
					name = fmt.Sprintf("arg%d", i+1)
				}
				return fmt.Errorf("required argument %q is missing", name)
			}
			// Set default value if available
			if argDef.Default != "" && argDef.Value != nil {
				if err := argDef.Value.Set(argDef.Default); err != nil {
					name := argDef.Name
					if name == "" {
						name = fmt.Sprintf("arg%d", i+1)
					}
					return fmt.Errorf("setting default value for %q: %w", name, err)
				}
			}
			continue
		}

		argStr := args[argIndex]
		trimmedArg := strings.TrimSpace(argStr)

		// Check if it's a query string, form data, or JSON format
		if strings.Contains(argStr, "=") && !strings.HasPrefix(argStr, "-") {
			// Query string or form data format
			var values map[string][]string
			var err error

			if strings.Contains(argStr, "&") || !strings.Contains(argStr, " ") {
				// Query string format
				values, err = ParseQueryArgs(argStr)
			} else {
				// Form data format
				values, err = ParseFormArgs(argStr)
			}

			if err == nil && len(values) > 0 {
				// Try to find matching arg by name
				found := false
				for key, valueList := range values {
					if len(valueList) > 0 {
						// Find arg by name
						for j := range argsDef {
							if argsDef[j].Name == key && argsDef[j].Value != nil {
								if err := argsDef[j].Value.Set(valueList[0]); err != nil {
									return fmt.Errorf("setting value for arg %q: %w", key, err)
								}
								found = true
								break
							}
						}
					}
				}
				if found {
					argIndex++
					continue
				}
			}
		} else if (strings.HasPrefix(trimmedArg, "{") && strings.HasSuffix(trimmedArg, "}")) ||
			(strings.HasPrefix(trimmedArg, "[") && strings.HasSuffix(trimmedArg, "]")) {
			// JSON format
			values, err := ParseJSONArgs(trimmedArg)
			if err == nil && len(values) > 0 {
				// Try to find matching arg by name
				found := false
				for key, valueList := range values {
					if len(valueList) > 0 && key != "" {
						// Find arg by name
						for j := range argsDef {
							if argsDef[j].Name == key && argsDef[j].Value != nil {
								if err := argsDef[j].Value.Set(valueList[0]); err != nil {
									return fmt.Errorf("setting value for arg %q: %w", key, err)
								}
								found = true
								break
							}
						}
					}
				}
				if found {
					argIndex++
					continue
				}
			}
		}

		// Regular positional argument
		if argDef.Value != nil {
			if err := argDef.Value.Set(argStr); err != nil {
				name := argDef.Name
				if name == "" {
					name = fmt.Sprintf("arg%d", i+1)
				}
				return fmt.Errorf("setting value for arg %q: %w", name, err)
			}
		}
		argIndex++
	}

	return nil
}

// Run executes the command.
// If two command share a flag name, the first command wins.
//
//nolint:revive
func (inv *Invocation) Run() (err error) {
	for _, child := range inv.Command.Children {
		child.parent = inv.Command
	}

	err = inv.Command.init()
	if err != nil {
		return fmt.Errorf("initializing command: %w", err)
	}

	defer func() {
		// Pflag is panicky, so additional context is helpful in tests.
		if flag.Lookup("test.v") == nil {
			return
		}
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered for %s: %v", inv.Command.FullName(), r)
			panic(err)
		}
	}()

	// We close Stdin to prevent deadlocks, e.g. when the command
	// has ended but an io.Copy is still reading from Stdin.
	defer func() {
		if inv.Stdin == nil {
			return
		}
		rc, ok := inv.Stdin.(io.ReadCloser)
		if !ok {
			return
		}
		e := rc.Close()
		err = errors.Join(err, e)
	}()
	err = inv.run(&runState{
		allArgs: inv.Args,
	})
	return err
}

// WithContext returns a copy of the Invocation with the given context.
func (inv *Invocation) WithContext(ctx context.Context) *Invocation {
	return inv.with(func(i *Invocation) {
		i.ctx = ctx
	})
}

// with returns a copy of the Invocation with the given function applied.
func (inv *Invocation) with(fn func(*Invocation)) *Invocation {
	i2 := *inv
	fn(&i2)
	return &i2
}

// MiddlewareFunc returns the next handler in the chain,
// or nil if there are no more.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

func chain(ms ...MiddlewareFunc) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		if len(ms) > 0 {
			return chain(ms[1:]...)(ms[0](next))
		}
		return next
	}
}

// Chain returns a Handler that first calls middleware in order.
//
//nolint:revive
func Chain(ms ...MiddlewareFunc) MiddlewareFunc {
	// We need to reverse the array to provide top-to-bottom execution
	// order when defining a command.
	reversed := make([]MiddlewareFunc, len(ms))
	for i := range ms {
		reversed[len(ms)-1-i] = ms[i]
	}
	return chain(reversed...)
}

func RequireNArgs(want int) MiddlewareFunc {
	return RequireRangeArgs(want, want)
}

// RequireRangeArgs returns a Middleware that requires the number of arguments
// to be between start and end (inclusive). If end is -1, then the number of
// arguments must be at least start.
func RequireRangeArgs(start, end int) MiddlewareFunc {
	if start < 0 {
		panic("start must be >= 0")
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, i *Invocation) error {
			got := len(i.Args)
			switch {
			case start == end && got != start:
				switch start {
				case 0:
					if len(i.Command.Children) > 0 {
						return fmt.Errorf("unrecognized subcommand %q", i.Args[0])
					}
					return fmt.Errorf("wanted no args but got %v %v", got, i.Args)
				default:
					return fmt.Errorf(
						"wanted %v args but got %v %v",
						start,
						got,
						i.Args,
					)
				}
			case start > 0 && end == -1:
				switch {
				case got < start:
					return fmt.Errorf(
						"wanted at least %v args but got %v",
						start,
						got,
					)
				default:
					return next(ctx, i)
				}
			case start > end:
				panic("start must be <= end")
			case got < start || got > end:
				return fmt.Errorf(
					"wanted between %v and %v args but got %v",
					start, end,
					got,
				)
			default:
				return next(ctx, i)
			}
		}
	}
}

// children returns a map of child command names to their respective commands.
func (c *Command) children() map[string]*Command {
	childrenMap := make(map[string]*Command)
	for _, child := range c.Children {
		childrenMap[child.Name()] = child
	}
	return childrenMap
}
