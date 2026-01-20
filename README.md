# Redant

[![Go Reference](https://pkg.go.dev/badge/github.com/pubgo/redant.svg)](https://pkg.go.dev/github.com/pubgo/redant)
[![Go Report Card](https://goreportcard.com/badge/github.com/pubgo/redant)](https://goreportcard.com/report/github.com/pubgo/redant)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

English | [简体中文](README_CN.md)

Redant is a powerful Go CLI framework designed for building large CLI applications. It provides flexible option configuration, excellent default help output, and a middleware-based composition pattern.

## Features

- **Command Tree Structure**: Supports complex nested command structures with flag inheritance
- **Multi-source Configuration**: Options can be set from command line flags and environment variables
- **Middleware System**: Based on Chi router pattern for easy feature extension
- **Excellent Help System**: Inspired by Go toolchain's help output style
- **Easy to Test**: Clear separation of stdin/stdout/stderr for unit testing
- **Flexible Parameter Formats**: Supports query string, form data, and JSON formats
- **Rich Value Types**: String, Int64, Float64, Bool, Duration, Enum, URL, HostPort, and more

## Installation

```bash
go get github.com/pubgo/redant
```

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
    "strings"
    
    "github.com/pubgo/redant"
)

func main() {
    var (
        port    int64
        host    string
        verbose bool
    )
    
    cmd := redant.Command{
        Use:   "server",
        Short: "Start the HTTP server",
        Options: redant.OptionSet{
            {
                Flag:        "port",
                Shorthand:   "p",
                Description: "Port to listen on",
                Default:     "8080",
                Value:       redant.Int64Of(&port),
            },
            {
                Flag:        "host",
                Description: "Host to bind",
                Default:     "localhost",
                Envs:        []string{"SERVER_HOST"},
                Value:       redant.StringOf(&host),
            },
            {
                Flag:        "verbose",
                Shorthand:   "v",
                Description: "Enable verbose output",
                Value:       redant.BoolOf(&verbose),
            },
        },
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintf(inv.Stdout, "Starting server on %s:%d\n", host, port)
            if verbose {
                fmt.Fprintln(inv.Stdout, "Verbose mode enabled")
            }
            return nil
        },
    }

    if err := cmd.Invoke().WithOS().Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

### Nested Commands

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/pubgo/redant"
)

func main() {
    rootCmd := &redant.Command{
        Use:   "app",
        Short: "An example application",
    }
    
    serverCmd := &redant.Command{
        Use:   "server",
        Short: "Server commands",
    }
    
    startCmd := &redant.Command{
        Use:   "start",
        Short: "Start the server",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintln(inv.Stdout, "Server started!")
            return nil
        },
    }
    
    stopCmd := &redant.Command{
        Use:   "stop",
        Short: "Stop the server",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintln(inv.Stdout, "Server stopped!")
            return nil
        },
    }
    
    serverCmd.Children = append(serverCmd.Children, startCmd, stopCmd)
    rootCmd.Children = append(rootCmd.Children, serverCmd)
    
    if err := rootCmd.Invoke().WithOS().Run(); err != nil {
        panic(err)
    }
}
```

## Value Types

Redant provides a rich set of value types:

| Type | Function | Description |
|------|----------|-------------|
| `String` | `StringOf(&v)` | String value |
| `Int64` | `Int64Of(&v)` | 64-bit integer |
| `Float64` | `Float64Of(&v)` | 64-bit float |
| `Bool` | `BoolOf(&v)` | Boolean value |
| `Duration` | `DurationOf(&v)` | Time duration |
| `StringArray` | `StringArrayOf(&v)` | String slice |
| `Enum` | `EnumOf(&v, choices...)` | Enum with validation |
| `EnumArray` | `EnumArrayOf(&v, choices...)` | Enum array |
| `URL` | `&URL{}` | URL parsing |
| `HostPort` | `&HostPort{}` | Host:port parsing |

### Validation

```go
var port int64

opt := redant.Option{
    Flag:  "port",
    Value: redant.Validate(redant.Int64Of(&port), func(v *redant.Int64) error {
        if v.Value() < 1 || v.Value() > 65535 {
            return fmt.Errorf("port must be between 1 and 65535")
        }
        return nil
    }),
}
```

## Middleware

Redant supports a middleware pattern for cross-cutting concerns:

```go
cmd := redant.Command{
    Use:   "example",
    Short: "Example command",
    Middleware: redant.Chain(
        // Require exactly 1 argument
        redant.RequireNArgs(1),
        // Custom logging middleware
        func(next redant.HandlerFunc) redant.HandlerFunc {
            return func(ctx context.Context, inv *redant.Invocation) error {
                fmt.Printf("Executing: %s\n", inv.Command.Name())
                err := next(ctx, inv)
                fmt.Printf("Completed: %s\n", inv.Command.Name())
                return err
            }
        },
    ),
    Handler: func(ctx context.Context, inv *redant.Invocation) error {
        // Handler logic
        return nil
    },
}
```

## Parameter Formats

Redant supports multiple parameter formats:

```bash
# Positional parameters
app arg1 arg2 arg3

# Query string format
app "name=value&age=30"

# Form data format  
app "name=value age=30"

# JSON format
app '{"name":"value","age":30}'
```

## Global Flags

Built-in global flags available for all commands:

| Flag | Description |
|------|-------------|
| `--help, -h` | Show help information |
| `--list-commands` | List all available commands |
| `--list-flags` | List all flags |

## Testing

Redant makes testing easy by separating I/O:

```go
func TestCommand(t *testing.T) {
    var stdout, stderr bytes.Buffer
    
    cmd := &redant.Command{
        Use: "test",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintln(inv.Stdout, "Hello, World!")
            return nil
        },
    }
    
    inv := cmd.Invoke()
    inv.Stdout = &stdout
    inv.Stderr = &stderr
    
    err := inv.Run()
    if err != nil {
        t.Fatal(err)
    }
    
    if got := stdout.String(); got != "Hello, World!\n" {
        t.Errorf("got %q, want %q", got, "Hello, World!\n")
    }
}
```

## Documentation

- [Design Document](docs/DESIGN.md) - Detailed architecture and design decisions
- [Evaluation Report](docs/EVALUATION.md) - Framework evaluation and recommendations
- [Changelog](docs/CHANGELOG.md) - Version history and changes
- [Examples](example/) - Example applications

## Examples

For more examples, check the [example](example/) directory:

- [echo](example/echo/) - Simple echo command
- [demo](example/demo/) - Feature demonstration
- [args-test](example/args-test/) - Parameter format testing
- [env-test](example/env-test/) - Environment variable testing
- [globalflags](example/globalflags/) - Global flags usage

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.