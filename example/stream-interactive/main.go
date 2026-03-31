package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pubgo/redant"
)

func main() {
	var persona string

	chatCmd := &redant.Command{
		Use:   "chat [topic]",
		Short: "交互式聊天示例（ResponseStreamHandler）",
		Options: redant.OptionSet{
			{
				Flag:        "persona",
				Description: "机器人人设",
				Default:     "assistant",
				Value:       redant.StringOf(&persona),
			},
		},
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			topic := "default-topic"
			if inv != nil && len(inv.Args) > 0 {
				topic = inv.Args[0]
			}

			if err := out.Send(fmt.Sprintf("[%s] topic=%s\n", persona, topic)); err != nil {
				return err
			}
			if err := out.Send("chunk-1: hello\n"); err != nil {
				return err
			}
			return out.Send("chunk-2: stream\n")
		}),
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: stream-interactive <stdio|callback>")
		os.Exit(2)
	}

	mode := os.Args[1]
	switch mode {
	case "stdio":
		if err := chatCmd.Invoke().WithOS().Run(); err != nil {
			fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			os.Exit(1)
		}
	case "callback", "channel":
		inv := chatCmd.Invoke("--persona", "planner", "stream-topic")
		inv.Annotations = map[string]any{"request_id": "demo.channel"}
		inv.Stdout = io.Discard
		inv.Stderr = io.Discard
		inv.Stdin = nil

		fmt.Println("=== callback 实时事件 ===")
		eventCount := 0
		if err := redant.RunCallback[string](inv, func(chunk string) error {
			eventCount++
			fmt.Println(chunk)
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("=== callback 完成，事件总数: %d ===\n", eventCount)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		fmt.Fprintln(os.Stderr, "Usage: stream-interactive <stdio|callback>")
		os.Exit(2)
	}
}
