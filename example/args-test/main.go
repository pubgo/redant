package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pubgo/redant"
)

func main() {
	// Create root command
	rootCmd := &redant.Command{
		Use:   "app",
		Short: "Args test application.",
		Long:  "A comprehensive test application for various argument formats.",
	}

	// Test command 1: Multiple regular arguments (with Value types)
	multiArgsCmd := &redant.Command{
		Use:   "multi",
		Short: "Test multiple positional arguments.",
		Args: redant.ArgSet{
			{Name: "arg1", Description: "First argument.", Value: redant.StringOf(new(string))},
			{Name: "arg2", Description: "Second argument.", Value: redant.Int64Of(new(int64))},
			{Name: "arg3", Description: "Third argument.", Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Multiple Positional Arguments ===")
			fmt.Printf("Args count: %d\n", len(inv.Args))
			for i, arg := range inv.Command.Args {
				if arg.Value != nil {
					fmt.Printf("  %s: %s (type: %s)\n", arg.Name, arg.Value.String(), arg.Value.Type())
				} else {
					fmt.Printf("  arg[%d]: %s\n", i, inv.Args[i])
				}
			}
			return nil
		},
	}

	// Test command 2: URL Query format
	queryCmd := &redant.Command{
		Use:   "query",
		Short: "Test URL query string format.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== URL Query String Format ===")
			fmt.Printf("Args: %v\n", inv.Args)

			// Query format parameters should be parsed from args
			if len(inv.Args) > 0 {
				arg := inv.Args[0]
				values, err := redant.ParseQueryArgs(arg)
				if err != nil {
					return fmt.Errorf("failed to parse query args: %w", err)
				}

				fmt.Println("Parsed query parameters:")
				for key, valueList := range values {
					if len(valueList) == 1 {
						fmt.Printf("  %s: %s\n", key, valueList[0])
					} else {
						fmt.Printf("  %s: %v\n", key, valueList)
					}
				}
			}
			return nil
		},
	}

	// Test command 3: Form Data format
	formCmd := &redant.Command{
		Use:   "form",
		Short: "Test form data format.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Form Data Format ===")
			fmt.Printf("Args: %v\n", inv.Args)

			// Form format parameters should be parsed from args
			if len(inv.Args) > 0 {
				arg := inv.Args[0]
				values, err := redant.ParseFormArgs(arg)
				if err != nil {
					return fmt.Errorf("failed to parse form args: %w", err)
				}

				fmt.Println("Parsed form parameters:")
				for key, valueList := range values {
					if len(valueList) == 1 {
						fmt.Printf("  %s: %s\n", key, valueList[0])
					} else {
						fmt.Printf("  %s: %v\n", key, valueList)
					}
				}
			}
			return nil
		},
	}

	// Test command 4: JSON format
	jsonCmd := &redant.Command{
		Use:   "json",
		Short: "Test JSON format.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== JSON Format ===")
			fmt.Printf("Args: %v\n", inv.Args)

			// JSON format parameters should be parsed from args
			if len(inv.Args) > 0 {
				arg := inv.Args[0]
				values, err := redant.ParseJSONArgs(arg)
				if err != nil {
					return fmt.Errorf("failed to parse JSON args: %w", err)
				}

				fmt.Println("Parsed JSON parameters:")
				for key, valueList := range values {
					if key == "" {
						// Array format
						fmt.Printf("  [array]: %v\n", valueList)
					} else {
						if len(valueList) == 1 {
							fmt.Printf("  %s: %s\n", key, valueList[0])
						} else {
							fmt.Printf("  %s: %v\n", key, valueList)
						}
					}
				}
			}
			return nil
		},
	}

	// Test command 5: Mixed format
	mixedCmd := &redant.Command{
		Use:   "mixed",
		Short: "Test mixed argument formats.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Mixed Format ===")
			fmt.Printf("Args: %v\n", inv.Args)

			// Process mixed format: positional and query format arguments
			for i, arg := range inv.Args {
				if strings.Contains(arg, "=") {
					// Query format parameters
					values, err := redant.ParseQueryArgs(arg)
					if err == nil {
						fmt.Printf("Query arg[%d]: %v\n", i, values)
					} else {
						fmt.Printf("Positional arg[%d]: %s\n", i, arg)
					}
				} else {
					fmt.Printf("Positional arg[%d]: %s\n", i, arg)
				}
			}
			return nil
		},
	}

	// Test command 6: Subcommand conflict case
	conflictParentCmd := &redant.Command{
		Use:   "conflict",
		Short: "Test command with subcommand conflict.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Conflict Parent Command ===")
			fmt.Printf("Args: %v\n", inv.Args)

			// If there are query format parameters, parse them
			if len(inv.Args) > 0 && strings.Contains(inv.Args[0], "=") {
				values, err := redant.ParseQueryArgs(inv.Args[0])
				if err == nil {
					fmt.Println("Parsed query parameters:")
					for key, valueList := range values {
						fmt.Printf("  %s: %v\n", key, valueList)
					}
				}
			}
			return nil
		},
	}

	// Subcommand (may conflict with args)
	conflictSubCmd := &redant.Command{
		Use:   "sub",
		Short: "Subcommand that might conflict with args.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Conflict Subcommand ===")
			fmt.Printf("Args: %v\n", inv.Args)
			return nil
		},
	}

	// Test command 7: Complex scenario - multiple argument formats mixed
	complexCmd := &redant.Command{
		Use:   "complex",
		Short: "Test complex scenarios.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Complex Scenario ===")
			fmt.Printf("Args: %v\n", inv.Args)

			// Process complex scenario: positional, query format, JSON format mixed
			for i, arg := range inv.Args {
				trimmedArg := strings.TrimSpace(arg)
				if strings.HasPrefix(trimmedArg, "{") || strings.HasPrefix(trimmedArg, "[") {
					// JSON format
					values, err := redant.ParseJSONArgs(trimmedArg)
					if err == nil {
						fmt.Printf("JSON arg[%d]: %v\n", i, values)
					} else {
						fmt.Printf("Positional arg[%d]: %s\n", i, arg)
					}
				} else if strings.Contains(arg, "=") {
					// Query or Form format
					if strings.Contains(arg, "&") || !strings.Contains(arg, " ") {
						values, err := redant.ParseQueryArgs(arg)
						if err == nil {
							fmt.Printf("Query arg[%d]: %v\n", i, values)
						} else {
							fmt.Printf("Positional arg[%d]: %s\n", i, arg)
						}
					} else {
						values, err := redant.ParseFormArgs(arg)
						if err == nil {
							fmt.Printf("Form arg[%d]: %v\n", i, values)
						} else {
							fmt.Printf("Positional arg[%d]: %s\n", i, arg)
						}
					}
				} else {
					fmt.Printf("Positional arg[%d]: %s\n", i, arg)
				}
			}
			return nil
		},
	}

	// Build command tree
	conflictParentCmd.Children = append(conflictParentCmd.Children, conflictSubCmd)
	rootCmd.Children = append(rootCmd.Children,
		multiArgsCmd,
		queryCmd,
		formCmd,
		jsonCmd,
		mixedCmd,
		conflictParentCmd,
		complexCmd,
	)

	// Run command
	err := rootCmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
