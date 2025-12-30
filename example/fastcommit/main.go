package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/completioncmd"
)

// mkdir -p ~/.zsh/completions
// go run example/fastcommit/main.go completion zsh > ~/.zsh/completions/_fastcommit

func main() {
	// Create root command
	rootCmd := &redant.Command{
		Use:   "fastcommit",
		Short: "A fast commit tool.",
		Long:  "A tool for making fast commits with various options.",
	}

	// Create commit command
	commitCmd := &redant.Command{
		Use:   "commit",
		Short: "Commit changes.",
		Long:  "Commit changes with a message and other options.",
		Options: redant.OptionSet{
			{
				Flag:        "message",
				Shorthand:   "m",
				Description: "Commit message.",
				Value:       redant.StringOf(new(string)),
			},
			{
				Flag:        "amend",
				Description: "Amend the previous commit.",
				Value:       redant.BoolOf(new(bool)),
			},
		},
		Args: redant.ArgSet{
			{Name: "files", Description: "Files to commit."},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Printf("Commit command executed\n")
			fmt.Printf("Args: %v\n", inv.Args)

			// Get flag values
			if inv.Flags != nil {
				// Get values directly from options
				var message string
				var amend bool

				for _, opt := range inv.Command.Options {
					switch opt.Flag {
					case "message":
						if strVal, ok := opt.Value.(*redant.String); ok {
							message = strVal.String()
						}
					case "amend":
						if boolVal, ok := opt.Value.(*redant.Bool); ok {
							amend = boolVal.Value()
						}
					}
				}

				fmt.Printf("Message: %s\n", message)
				fmt.Printf("Amend: %v\n", amend)
			}

			return nil
		},
	}

	// Create detailed subcommand for commit
	detailedCmd := &redant.Command{
		Use:   "detailed",
		Short: "Detailed commit.",
		Long:  "Commit with detailed options.",
		Options: redant.OptionSet{
			{
				Flag:        "author",
				Description: "Author of the commit.",
				Value:       redant.StringOf(new(string)),
			},
			{
				Flag:        "verbose",
				Shorthand:   "v",
				Description: "Verbose output.",
				Value:       redant.BoolOf(new(bool)),
			},
		},
		Args: redant.ArgSet{
			{Name: "files", Description: "Files to commit."},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Printf("Detailed commit command executed\n")
			fmt.Printf("Args: %v\n", inv.Args)

			// Get flag values
			if inv.Flags != nil {
				// Get values directly from options
				var author string
				var verbose bool

				for _, opt := range inv.Command.Options {
					switch opt.Flag {
					case "author":
						if strVal, ok := opt.Value.(*redant.String); ok {
							author = strVal.String()
						}
					case "verbose":
						if boolVal, ok := opt.Value.(*redant.Bool); ok {
							verbose = boolVal.Value()
						}
					}
				}

				fmt.Printf("Author: %s\n", author)
				fmt.Printf("Verbose: %v\n", verbose)
			}

			return nil
		},
	}

	// Build command tree
	commitCmd.Children = append(commitCmd.Children, detailedCmd)
	rootCmd.Children = append(rootCmd.Children, commitCmd)
	rootCmd.Children = append(rootCmd.Children, completioncmd.New())

	// Run command
	err := rootCmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
