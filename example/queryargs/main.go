package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pubgo/redant"
)

func main() {
	// Create root command
	rootCmd := &redant.Command{
		Use:   "queryargs",
		Short: "A test application for query arguments.",
		Long:  "A test application to demonstrate query argument functionality.",
	}

	// Create test command
	testCmd := &redant.Command{
		Use:   "test",
		Short: "Test command with query args.",
		Options: redant.OptionSet{
			{
				Flag:        "name",
				Description: "Name parameter.",
				Value:       redant.StringOf(new(string)),
			},
			{
				Flag:        "age",
				Description: "Age parameter.",
				Value:       redant.Int64Of(new(int64)),
			},
			{
				Flag:        "verbose",
				Description: "Verbose output.",
				Value:       redant.BoolOf(new(bool)),
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Query Args Test ===")

			// Get flag values
			if inv.Flags != nil {
				// Directly get values from options
				var name, ageStr string
				var verbose bool

				for _, opt := range inv.Command.Options {
					switch opt.Flag {
					case "name":
						if strVal, ok := opt.Value.(*redant.String); ok {
							name = strVal.String()
						}
					case "age":
						if intVal, ok := opt.Value.(*redant.Int64); ok {
							ageStr = intVal.String()
						}
					case "verbose":
						if boolVal, ok := opt.Value.(*redant.Bool); ok {
							verbose = boolVal.Value()
						}
					}
				}

				// Convert age value
				var age int64
				if ageStr != "" {
					fmt.Sscanf(ageStr, "%d", &age)
				}

				fmt.Printf("Name: %s\n", name)
				fmt.Printf("Age: %d\n", age)
				fmt.Printf("Verbose: %v\n", verbose)
			}

			return nil
		},
	}

	// Build command tree
	rootCmd.Children = append(rootCmd.Children, testCmd)

	// Run command
	err := rootCmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
