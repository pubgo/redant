# Redant Framework Design Document

## 1. Design Philosophy

Redant is a Go CLI framework based on Cobra, specifically designed for large CLI applications. Compared to Cobra, Redant provides better default help output, more flexible option configuration, and a middleware-based composition pattern.

## 2. Core Features

1. **Command tree structure**: Supports complex nested command structures, subcommands can inherit options from parent commands
2. **Multi-source configuration**: Options can be set from multiple sources including command line flags, environment variables, and configuration files
3. **Middleware system**: Middleware system based on Chi router pattern, facilitating feature extension
4. **Excellent help system**: Inspired by the help output style of Go toolchain
5. **Easy to test**: Clearly separates standard input/output, making unit tests easier to write
6. **Colon-separated commands**: Supports `command:sub-cmd` format command paths
7. **Flexible parameter formats**: Supports query string and form data parameter formats
8. **Global flag system**: Provides unified global flag management

## 3. Unified Command Format

### 3.1 Command Format Specification

Applications follow a unified format: `app-name command:sub-cmd [args...] [flags...]`

1. `app-name` is the application name (root command)
2. `command:sub-cmd` uses colon separator to indicate command hierarchy
   - Supports two invocation methods:
     - Colon-separated: `app commit:detailed`
     - Space-separated: `app commit detailed`
3. `args` support multiple formats:
   - Query string format: `name=value&a=b&name=123`
   - Form format: `name=value name2=value2` (space-separated key-value pairs)
   - Single data format: direct data value
4. `flags` are always placed at the end, and subcommand flags cannot duplicate parent command flags
5. The entire organization format maintains consistency, all operations follow this specification

### 3.2 Command Parsing Mechanism

Command parsing follows these priorities:

1. **Exact matching**: First try to find the complete command path in the command map (e.g., `commit:detailed`)
2. **Colon-separated parsing**: If parameter contains colon, split by colon and look up subcommands level by level
3. **Space-separated parsing**: If exact matching fails, look up subcommands level by level with space-separated parameters
4. **Parameter recognition**: When encountering parameters starting with `-` or containing `=`, stop command parsing

### 3.3 Parameter System Design

#### 3.3.1 Arg Struct
```go
type Arg struct {
    Name        string          // Parameter name
    Description string          // Parameter description
    Required    bool            // Whether required
    Default     string          // Default value
    Value       pflag.Value     // Parameter value (supports multiple types)
}
```

**Function description:**
- `Value` field is used to determine parameter type and automatic type conversion
- If Command defines `Args`, parsed values are automatically set to corresponding `Arg.Value`
- If Command does not define `Args`, the system automatically creates `Args` based on `parsedArgs`, with names in order as `arg1`, `arg2`, `arg3`, etc.
- Supports automatic parsing of positional parameters, query string, form data, and JSON formats

#### 3.3.2 Parameter Format Support

Redant supports three parameter formats:

1. **Query string format**: `name=value&a=b&name=123`
   - Use `&` to separate multiple key-value pairs
   - Support duplicate key names
   - Parsed via `ParseQueryArgs()`

2. **Form format**: `name=value name2=value2`
   - Use space to separate multiple key-value pairs
   - Support quoted values (single or double quotes)
   - Parsed via `ParseFormArgs()`

3. **Single data format**: Direct data values
   - For example: `123456`, `"hello world"`

#### 3.3.3 Parameter Parsing Process

1. During command execution, the system checks if each parameter contains `=`
2. If it contains `=` and does not start with `-`, it's identified as a key-value parameter
3. Based on whether it contains `&` or space, select the appropriate parsing function
4. Parsed values are set to the corresponding flag
5. If parsing fails, the parameter is treated as a regular parameter

#### 3.3.4 Parameter Help Information Display
```
args:
     name int, this is name , env=[$ABC] default=[123] required=[true] enums=[]
```

### 3.4 Flag System Design

#### 3.4.1 Option Struct
```go
type Option struct {
    Flag          string          // Flag name (also used as option identifier)
    Description   string          // Option description
    Required      bool            // Whether required
    Shorthand string          // Flag shorthand
    Envs          []string        // Environment variables (multiple supported)
    Default       string          // Default value
    Value         pflag.Value     // Option value
    Hidden        bool            // Whether hidden
    Deprecated    string          // Deprecation message
    Category      string          // Category
}
```

#### 3.4.2 Flag Organization Rules
1. Flags are always placed at the end of the command line
2. Subcommand flags cannot duplicate parent command flags (subcommand flags override parent command flags with the same name)
3. Global flags are automatically added to all commands
4. Displayed by category in help information

#### 3.4.3 Global Flag System

Redant provides a set of default global flags, which are automatically added to the root command:

- `--help, -h`: Show help information
- `--version, -v`: Show version information
- `--list-commands`: List all commands (including subcommands)
- `--list-flags`: List all flags
- `--config-file, -c`: Configuration file path
- `--debug`: Enable debug mode
- `--log-level`: Set log level (default: info)
- `--environment, -e`: Set environment
- `--env-file`: Environment file path
- `--env-files`: Multiple environment file paths (array)

Global flags can be obtained via the `GetGlobalFlags()` method and displayed separately in help information.

#### 3.4.4 Flag Help Information Display
```
flags(sub-command):
  --name,-n int, short, env:[$ABC] default:[123] required:[true]
  --name,-n int, short, env:[$ABC] default:[123] required:[true]

flags(global):
  --name,-n int, short, env:[$ABC]
  --name,-n int, short, env:[$ABC]
```

### 3.5 Command System Design

#### 3.5.1 Command Struct
```go
type Command struct {
    parent      *Command        // Parent command (set automatically)
    Children    []*Command      // Subcommand list
    Use         string          // Usage description, format: "command [flags] [args...]"
    Aliases     []string        // Aliases
    Short       string          // Short description
    Hidden      bool            // Whether hidden
    Deprecated  string          // Deprecation message (if set, command is marked as deprecated)
    RawArgs     bool            // Whether to use raw arguments (don't parse flags)
    Long        string          // Detailed description
    Options     OptionSet       // Option set
    Args        ArgSet          // Argument set
    Middleware  MiddlewareFunc  // Middleware
    Handler     HandlerFunc     // Handler function
}
```

#### 3.5.2 Command Inheritance Mechanism
1. Subcommands can access all options from parent commands (via `FullOptions()` method)
2. Subcommands can override options with the same name from parent commands (subcommand options have higher priority)
3. Help information automatically includes parent command information
4. Parent command middleware will be combined and executed with subcommand middleware

#### 3.5.3 Command Lookup Mechanism

Command lookup builds a command mapping table via the `getCommands()` function, supporting:

1. **Flattened mapping**: All commands (including subcommands) are registered in the mapping table
2. **Path format**: Use colon-separated paths as keys (e.g., `commit:detailed`)
3. **Root command**: Root command uses its name as the key (without colon)
4. **Duplicate detection**: If duplicate command names are found, panic is triggered

#### 3.5.4 Command Execution Flow

1. Initialize command tree (`init()`)
2. Build command mapping table (`getCommands()`)
3. Parse command path (`getExecCommand()`)
4. Merge global flags and command flags
5. Parse flags and args
6. Process query string parameters
7. Execute middleware chain
8. Execute command handler

## 4. Execution Flow

### 4.1 Command Initialization (`init()`)

Command initialization is automatically called in the `Run()` method, including:

1. **Set Use field**: If empty, default to "unnamed"
2. **Add global flags**: Add global flags only to root command
3. **Validate options**:
   - Ensure each option has Name, Flag, or Env
   - Validate description format (start with capital letter, end with period)
4. **Sorting**:
   - Sort options by name
   - Sort subcommands by name
5. **Recursive initialization**: Recursively initialize all subcommands

### 4.2 Help System

#### 4.2.1 Help Trigger Conditions
1. Command has no handler (`Handler == nil`)
2. Using `--help` or `-h` parameter
3. Flag parsing returns `pflag.ErrHelp` error

#### 4.2.2 Help Information Generation
1. Use template system (`help.tpl`) to generate help text
2. Support terminal width adaptive line wrapping
3. Display command usage, description, subcommands, options and other information
4. Group options by category
5. Support color output (in non-test environment)

#### 4.2.3 Special Help Commands
- `--list-commands`: List all commands (including subcommands), using colon-separated path format
- `--list-flags`: List all flags, distinguishing between global flags and command-specific flags

### 4.3 Command Execution Process (`run()`)

Detailed execution flow:

1. **Set parent command relationship**: Recursively set parent command pointers for all subcommands

2. **Build command mapping table**: Build flattened command mapping via `getCommands()`

3. **Parse command path**: Determine the command to execute via `getExecCommand()`
   - Support colon-separated format: `command:sub-cmd`
   - Support space-separated format: `command sub-cmd`
   - Stop parsing when encountering flags or parameters containing `=`

4. **Handle deprecation warning**: If command is marked as deprecated, output warning information

5. **Merge flags**:
   - First add global flags to the flag set
   - Then add command-specific flags (will override global flags with the same name)

6. **Parse flags**: Use pflag to parse command line flags

7. **Process global flags**:
   - `--list-commands`: List all commands and return
   - `--list-flags`: List all flags and return
   - `--version`: Show version information and return

8. **Recursively look up subcommands**: If there are still unparsed parameters, try to look up subcommands

9. **Process query string parameters**:
   - Identify parameters containing `=`
   - Select parsing method based on whether it contains `&` or space
   - Set parsed values to the corresponding flags

10. **Validate required options**: Check if all options marked as `Required` have been set

11. **Prepare arguments**: Extract command arguments from parsed parameters (skip flags)

12. **Execute middleware chain**:
    - Merge middleware from parent and subcommands
    - Execute middleware chain in order

13. **Execute handler**: Call the command's `Handler` function

## 5. Middleware System Design

### 5.1 Middleware Structure

Adopting middleware pattern similar to Chi router:

```go
type MiddlewareFunc func(next HandlerFunc) HandlerFunc
type HandlerFunc func(inv *Invocation) error
```

1. Middleware function receives the next handler and returns a new handler
2. Support chaining multiple middlewares (via `Chain()` function)
3. Middleware can access command context information (`Invocation`)
4. Parent command middleware will be passed to subcommands (executed in order)

### 5.2 Middleware Execution Order

Middleware execution order follows the "onion model":
- Definition order: `Chain(mw1, mw2, mw3)`
- Actual execution order: `mw1 -> mw2 -> mw3 -> handler -> mw3 -> mw2 -> mw1`

Note: The `Chain()` function reverses the middleware array to ensure definition order matches execution order.

### 5.3 Built-in Middleware

Redant provides some built-in middleware:

- `RequireNArgs(n)`: Require exact number of arguments
- `RequireRangeArgs(start, end)`: Require number of arguments within a range (end as -1 means at least start)

### 5.4 Typical Usage Scenarios
1. **Logging**: Record command execution time, parameters and other information
2. **Permission validation**: Check if user has permission to execute command
3. **Parameter validation**: Validate parameters against specific rules
4. **Resource management**: Automatically acquire and release resources
5. **Error handling**: Uniformly handle errors and recover from panics

## 6. Invocation System

### 6.1 Invocation Structure

```go
type Invocation struct {
    ctx     context.Context
    Command *Command
    Flags   *pflag.FlagSet
    argSet  []Arg
    Args    []string
    Stdout  io.Writer
    Stderr  io.Writer
    Stdin   io.Reader
}
```

### 6.2 Invocation Methods

- `WithOS()`: Use OS default standard input/output
- `WithContext(ctx)`: Set context
- `ParsedFlags()`: Get parsed flag set
- `Context()`: Get context

### 6.3 Test Support

1. **Mock environment**: Can inject custom `Stdout`, `Stderr`, `Stdin`
2. **Test flags**: Inject pre-parsed flags via `WithTestParsedFlags()`
3. **Signal handling**: Override signal handling logic via `WithTestSignalNotifyContext()`

## 7. Error Handling

### 7.1 Error Types

1. **RunCommandError**: Command execution error, containing command and original error
2. **UnknownSubcommandError**: Unknown subcommand error

### 7.2 Error Handling Strategy

1. Provide detailed error information (including command name and parameters)
2. Distinguish between user errors and system errors
3. Return non-zero exit code at appropriate times
4. Help requests are not considered errors (return nil)

## 8. Test Support

1. **Invocation mocking**: Can simulate real command execution environment
2. **Stream injection**: Support injection of standard input/output streams, facilitating output validation
3. **Flag injection**: Can inject pre-parsed flags, skipping actual parsing process
4. **Independent testing**: Can independently test middleware and handler functions
5. **Test mode detection**: Automatically detect test environment, disable color output, etc.

## 9. Performance Considerations

1. **Command initialization**: Initialize command tree only on first run, avoiding duplicate initialization
2. **Option parsing**: Use efficient flag parsing library (pflag)
3. **Memory usage**: Properly manage middleware chain and option value memory allocation
4. **Command mapping**: Use map structure for fast command lookup
5. **Flag copying**: Use efficient copying mechanism when overriding flags

## 10. Best Practices

1. **Command organization**: Properly plan command hierarchy structure, avoid deep nesting (suggest no more than 3 levels)
2. **Option naming**: Use consistent naming conventions, follow kebab-case
3. **Description format**: Option descriptions should start with capital letter, end with period
4. **Error handling**: Provide clear error information, help users quickly locate issues
5. **Documentation**: Provide detailed documentation for each command and option
6. **Test coverage**: Ensure adequate test coverage for critical paths
7. **Parameter validation**: Use middleware to validate parameters, rather than in handlers
8. **Global flags**: Use global flags appropriately, avoid overuse

## 11. Example Usage

### 11.1 Basic Command

```go
cmd := redant.Command{
    Use:   "echo <text>",
    Short: "Prints the given text to the console.",
    Handler: func(inv *redant.Invocation) error {
        if len(inv.Args) == 0 {
            return fmt.Errorf("missing text")
        }
        fmt.Fprintln(inv.Stdout, inv.Args[0])
        return nil
    },
}
```

### 11.2 Command with Options

```go
var upper bool
cmd := redant.Command{
    Use:   "echo <text>",
    Short: "Prints the given text to the console.",
    Options: redant.OptionSet{
        {
            Name:        "upper",
            Flag:        "upper",
            Description: "Prints the text in upper case.",
            Value:       redant.BoolOf(&upper),
        },
    },
    Handler: func(inv *redant.Invocation) error {
        // ...
    },
}
```

### 11.3 Nested Commands

```go
rootCmd := &redant.Command{
    Use:   "app",
    Short: "An example app.",
}
subCmd := &redant.Command{
    Use:   "sub",
    Short: "A subcommand.",
}
rootCmd.Children = append(rootCmd.Children, subCmd)
```

### 11.4 Using Middleware

```go
cmd := redant.Command{
    Use:   "echo <text>",
    Middleware: redant.Chain(
        redant.RequireNArgs(1),
        func(next redant.HandlerFunc) redant.HandlerFunc {
            return func(inv *redant.Invocation) error {
                // Logging
                log.Printf("Executing command: %s", inv.Command.Name())
                return next(inv)
            }
        },
    ),
    Handler: func(inv *redant.Invocation) error {
        // ...
    },
}