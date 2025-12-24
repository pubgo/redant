# Redant

Redant is a powerful Go CLI framework designed for building large CLI applications. It is based on the Cobra framework and provides more flexible option configuration, excellent default help output, and a middleware-based composition pattern.

## Features

- **Command tree structure**: Supports complex nested command structures, subcommands can inherit options from parent commands
- **Multi-source configuration**: Options can be set from multiple sources including command line flags, environment variables, and configuration files
- **Middleware system**: Middleware system based on Chi router pattern, facilitating feature extension
- **Excellent help system**: Inspired by the help output style of Go toolchain
- **Easy to test**: Clearly separates standard input/output, making unit tests easier to write
- **Colon-separated commands**: Supports `command:sub-cmd` format command paths
- **Flexible parameter formats**: Supports query string and form data parameter formats
- **Global flag system**: Provides unified global flag management

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/pubgo/redant"
)

func main() {
    cmd := redant.Command{
        Use:   "echo <text>",
        Short: "Prints the given text to the console.",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            if len(inv.Args) == 0 {
                return fmt.Errorf("missing text")
            }
            fmt.Fprintln(inv.Stdout, inv.Args[0])
            return nil
        },
    }

    err := cmd.Invoke().WithOS().Run()
    if err != nil {
        panic(err)
    }
}
```

### Command with Options

```go
package main

import (
    "context"
    "fmt"
    "os"
    "strings"
    
    "github.com/pubgo/redant"
)

func main() {
    var upper bool
    cmd := redant.Command{
        Use:   "echo <text>",
        Short: "Prints the given text to the console.",
        Options: redant.OptionSet{
            {
                Flag:        "upper",
                Description: "Prints the text in upper case.",
                Value:       redant.BoolOf(&upper),
            },
        },
        Args: redant.ArgSet{
            {},
        },
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            if len(inv.Args) == 0 {
                inv.Stderr.Write([]byte("error: missing text\n"))
                os.Exit(1)
            }

            text := inv.Args[0]
            if upper {
                text = strings.ToUpper(text)
            }

            inv.Stdout.Write([]byte(text))
            return nil
        },
    }

    err := cmd.Invoke().WithOS().Run()
    if err != nil {
        panic(err)
    }
}
```

### Nested Commands

```go
package main

import (
    "github.com/pubgo/redant"
)

func main() {
    rootCmd := &redant.Command{
        Use:   "app",
        Short: "An example app.",
    }
    
    subCmd := &redant.Command{
        Use:   "sub",
        Short: "A subcommand.",
    }
    
    rootCmd.Children = append(rootCmd.Children, subCmd)
    
    err := rootCmd.Invoke().WithOS().Run()
    if err != nil {
        panic(err)
    }
}
```

## Parameter Formats

Redant supports multiple parameter formats:

1. **Positional parameters**: `command arg1 arg2 arg3`
2. **Query string format**: `command "name=value&age=30"`
3. **Form data format**: `command "name=value age=30"`
4. **JSON format**: `command '{"name":"value","age":30}'`

## Middleware

Redant supports middleware pattern:

```go
cmd := redant.Command{
    Use:   "echo <text>",
    Middleware: redant.Chain(
        redant.RequireNArgs(1),
        func(next redant.HandlerFunc) redant.HandlerFunc {
            return func(ctx context.Context, inv *redant.Invocation) error {
                // Log execution
                fmt.Printf("Executing command: %s\n", inv.Command.Name())
                return next(ctx, inv)
            }
        },
    ),
    Handler: func(ctx context.Context, inv *redant.Invocation) error {
        // ...
    },
}
```

## Examples

For more examples, please check the [example](example/) directory.

## Documentation

For detailed design documentation, please check [docs/DESIGN.md](docs/DESIGN.md).

## License

This project is licensed under the MIT License.