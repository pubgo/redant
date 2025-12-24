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
				Value:       redant.BoolOf(&upper),
				Description: "Prints the text in upper case.",
			},
		},
		Args: redant.ArgSet{
			{},
		},
		Middleware: redant.Chain(func(next redant.HandlerFunc) redant.HandlerFunc {
			return func(ctx context.Context, i *redant.Invocation) error {
				fmt.Printf("Debug: Args = %v\n", i.Args)
				fmt.Printf("Debug: upper = %v\n", upper)
				return next(ctx, i)
			}
		}),
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			fmt.Printf("Handler: Args = %v\n", inv.Args)
			fmt.Printf("Handler: upper = %v\n", upper)
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
