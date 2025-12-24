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
		Use:   "app",
		Short: "Environment variable test application.",
		Long:  "Test application for multiple environment variables.",
	}

	// Test command: multiple environment variables
	testCmd := &redant.Command{
		Use:   "test",
		Short: "Test command with multiple env vars.",
		Options: redant.OptionSet{
			{
				Flag:        "port",
				Shorthand:   "p",
				Description: "Port to listen on.",
				Value:       redant.Int64Of(new(int64)),
				Envs:        []string{"PORT", "SERVER_PORT", "APP_PORT"}, // Multiple environment variables
				Default:     "8080",
			},
			{
				Flag:        "host",
				Description: "Host to bind to.",
				Value:       redant.StringOf(new(string)),
				Envs:        []string{"HOST", "SERVER_HOST"}, // Multiple environment variables
				Default:     "localhost",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("=== Environment Variable Test ===")

			var port int64
			var host string

			for _, opt := range inv.Command.Options {
				switch opt.Flag {
				case "port":
					if intVal, ok := opt.Value.(*redant.Int64); ok {
						port = intVal.Value()
					}
				case "host":
					if strVal, ok := opt.Value.(*redant.String); ok {
						host = strVal.String()
					}
				}
			}

			fmt.Printf("Port: %d\n", port)
			fmt.Printf("Host: %s\n", host)

			// Display environment variable values
			fmt.Println("\nEnvironment variables:")
			for _, opt := range inv.Command.Options {
				if len(opt.Envs) > 0 {
					fmt.Printf("  %s:\n", opt.Flag)
					for _, envName := range opt.Envs {
						envValue := os.Getenv(envName)
						if envValue != "" {
							fmt.Printf("    $%s = %s (used)\n", envName, envValue)
						} else {
							fmt.Printf("    $%s = (not set)\n", envName)
						}
					}
				}
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
