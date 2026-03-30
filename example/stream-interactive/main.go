package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/pubgo/redant"
)

func main() {
	var persona string

	chatCmd := &redant.Command{
		Use:   "chat [topic]",
		Short: "交互式聊天示例（StreamHandler）",
		Options: redant.OptionSet{
			{
				Flag:        "persona",
				Description: "机器人人设",
				Default:     "assistant",
				Value:       redant.StringOf(&persona),
			},
		},
		StreamHandler: func(ctx context.Context, stream *redant.InvocationStream) error {
			topic := "default-topic"
			if inv := stream.Invocation(); inv != nil && len(inv.Args) > 0 {
				topic = inv.Args[0]
			}

			if err := stream.Control("phase:init\n"); err != nil {
				return err
			}
			if err := stream.Outputf("[%s] topic=%s\n", persona, topic); err != nil {
				return err
			}
			if err := stream.EndRound("topic-announced"); err != nil {
				return err
			}

			if err := stream.OutputChunk("chunk-1: hello\n"); err != nil {
				return err
			}
			if err := stream.OutputChunk("chunk-2: stream\n"); err != nil {
				return err
			}
			if err := stream.EndRound("chunk-finished"); err != nil {
				return err
			}

			return stream.Exit(0, "session-end", false, nil)
		},
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: stream-interactive <stdio|channel>")
		os.Exit(2)
	}

	mode := os.Args[1]
	switch mode {
	case "stdio":
		if err := chatCmd.Invoke().WithOS().Run(); err != nil {
			fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			os.Exit(1)
		}
	case "channel":
		inv := chatCmd.Invoke("--persona", "planner", "stream-topic")
		inv.Annotations = map[string]any{"request_id": "demo.channel"}
		inv.Stdout = io.Discard
		inv.Stderr = io.Discard
		inv.Stdin = nil

		out := inv.ResponseStream()

		if err := inv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("=== channel 输出事件 ===")
		for e := range out {
			b, err := json.Marshal(e)
			if err != nil {
				fmt.Printf("marshal event failed: %v\n", err)
				continue
			}
			fmt.Printf("%s\n", string(b))
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		fmt.Fprintln(os.Stderr, "Usage: stream-interactive <stdio|channel>")
		os.Exit(2)
	}
}
