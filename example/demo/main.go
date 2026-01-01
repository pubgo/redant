package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/pubgo/redant"
)

func main() {
	ok := "true"
	// Create root command
	rootCmd := &redant.Command{
		Use:   "myapp <abc>",
		Short: "My sample application.",
		Long:  "A sample application demonstrating all the implemented features.",
		Options: redant.OptionSet{
			{
				Flag:        "upper",
				Value:       redant.StringOf(&ok),
				Shorthand:   "u",
				Description: "Prints the text in upper case.",
			},
		},
	}

	// Create server command
	serverCmd := &redant.Command{
		Use:   "server",
		Short: "Server management commands.",
		Long:  "Commands for managing the server.",
		Options: redant.OptionSet{
			{
				Flag:        "list",
				Description: "List all servers.",
				Value:       redant.EnumOf(new(string), "list", "show", "all"),
			},
			{
				Flag:        "list-enum",
				Description: "List all servers.",
				Value:       redant.EnumArrayOf(new([]string), "list", "show", "all"),
			},
			{
				Flag:        "port",
				Shorthand:   "p",
				Description: "Port to listen on.",
				Required:    true,
				Value:       redant.Int64Of(new(int64)),
				Default:     "8080",
				Envs:        []string{"SERPENT_SERVER_PORT"},
				Deprecated:  "This flag is deprecated. Please use the new configuration method.",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("Server command executed")
			// Get flag values
			if inv.Flags != nil {
				port, _ := inv.Flags.GetInt64("port")
				fmt.Printf("Port: %d\n", port)
			}

			return nil
		},
	}

	// Create server subcommand
	startCmd := &redant.Command{
		Use:   "start",
		Short: "Start the server.",
		Long:  "Start the server with specified options.",
		Options: redant.OptionSet{
			{
				Flag:        "daemon",
				Description: "Run server in daemon mode.",
				Value:       redant.BoolOf(new(bool)),
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("Starting server...")

			// Get flag values
			if inv.Flags != nil {
				inv.Flags.VisitAll(func(f *pflag.Flag) {
					fmt.Printf("Flag: %s, Value: %v, Type: %s\n", f.Name, f.Value.String(), f.Value.Type())
				})
			}

			return nil
		},
	}

	// Create config command
	configCmd := &redant.Command{
		Use:   "config",
		Short: "Configuration commands.",
		Long:  "Commands for managing configuration.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("Config command executed")
			return nil
		},
	}

	// Create config subcommand
	showCmd := &redant.Command{
		Use:   "show",
		Short: "Show configuration.",
		Long:  "Display current configuration.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Println("Showing configuration...")
			return nil
		},
	}

	// Build command tree
	serverCmd.Children = append(serverCmd.Children, startCmd)
	configCmd.Children = append(configCmd.Children, showCmd)
	rootCmd.Children = append(rootCmd.Children, serverCmd, configCmd)

	// Run command
	err := rootCmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
