package main

import (
	"fmt"
	"os"

	"github.com/pubgo/redant"
)

func main() {
	// Create root command
	rootCmd := &redant.Command{
		Use:   "app",
		Short: "A test application.",
		Long:  "A test application to demonstrate global flags.",
	}

	// Create subcommand
	testCmd := &redant.Command{
		Use:   "test",
		Short: "Test command.",
		Long:  "A test command with custom flags.",
		Options: redant.OptionSet{
			{
				Flag:        "name",
				Description: "Name parameter.",
				Value:       redant.StringOf(new(string)),
			},
		},
		Handler: func(inv *redant.Invocation) error {
			fmt.Println("Test command executed")

			// Get flag values
			if inv.Flags != nil {
				name, _ := inv.Flags.GetString("name")
				fmt.Printf("Name: %s\n", name)
			}

			return nil
		},
	}

	subCmd := &redant.Command{
		Use:   "sub",
		Short: "Sub command.",
		Long:  "A sub command.",
		Handler: func(inv *redant.Invocation) error {
			fmt.Println("Sub command executed")
			return nil
		},
	}

	// Build command tree
	subCmd.Children = append(subCmd.Children, &redant.Command{
		Use:   "nested",
		Short: "Nested command.",
		Long:  "A nested command.",
		Handler: func(inv *redant.Invocation) error {
			fmt.Println("Nested command executed")
			return nil
		},
	})

	testCmd.Children = append(testCmd.Children, subCmd)
	rootCmd.Children = append(rootCmd.Children, testCmd)

	// Run command
	err := rootCmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
