package redant

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"
	"time"
)

func TestResponseStreamHandlerFallsBackToStdIO(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			stream := out.Raw()
			if err := stream.Send(map[string]any{"event": "control", "data": "phase:init\n"}); err != nil {
				return err
			}
			return stream.Send(map[string]any{"event": "output", "data": "hello, redant"})
		}),
	}

	var stdout bytes.Buffer
	inv := cmd.Invoke()
	inv.Stdin = bytes.NewBuffer(nil)
	inv.Stdout = &stdout
	inv.Stderr = io.Discard

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := stdout.String(), "phase:init\nhello, redant"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestResponseStreamHandlerWithChannels(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			stream := out.Raw()
			if err := stream.Send(map[string]any{"event": "output", "data": "echo:ping"}); err != nil {
				return err
			}
			return stream.Send(map[string]any{"event": "round_end", "data": map[string]any{"round": 1, "reason": "done"}})
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	inv.Stdin = bytes.NewBuffer(nil)

	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	for evt := range out {
		if event, _ := evt["event"].(string); event == "output" {
			got = evt
			break
		}
	}

	if event, _ := got["event"].(string); event != "output" {
		t.Fatalf("event = %q, want %q", event, "output")
	}
	if text, _ := got["data"].(string); text != "echo:ping" {
		t.Fatalf("text = %q, want %q", text, "echo:ping")
	}
}

func TestStreamControlUsesControlMethod(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			return out.Raw().Send(map[string]any{"event": "control", "data": "name?"})
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	inv.Stdin = bytes.NewBuffer(nil)

	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var msg map[string]any
	for evt := range out {
		if event, _ := evt["event"].(string); event == "control" {
			msg = evt
			break
		}
	}
	if event, _ := msg["event"].(string); event != "control" {
		t.Fatalf("event = %q, want %q", event, "control")
	}
}

func TestInvocationRunClosesResponseStream(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			return out.Raw().Send(map[string]any{"event": "output", "data": "done"})
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	inv.Stdin = bytes.NewBuffer(nil)
	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := 0
	for range out {
		count++
	}
	if count == 0 {
		t.Fatalf("expected at least one stream response")
	}
}

func TestStreamEndRoundIsSequential(t *testing.T) {
	var rounds []int

	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			stream := out.Raw()
			for i := 0; i < 2; i++ {
				if err := stream.Send(map[string]any{"event": "round_end", "data": map[string]any{"round": i + 1, "reason": "ok"}}); err != nil {
					return err
				}
			}
			return nil
		}),
	}

	inv := cmd.Invoke()
	inv.Stdin = bytes.NewBuffer(nil)
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for evt := range out {
		if event, _ := evt["event"].(string); event == "round_end" {
			if data, ok := evt["data"].(map[string]any); ok {
				if round, ok := data["round"].(int); ok {
					rounds = append(rounds, round)
					continue
				}
				if round, ok := data["round"].(float64); ok {
					rounds = append(rounds, int(round))
				}
			}
		}
	}
	want := []int{1, 2}
	if !slices.Equal(rounds, want) {
		t.Fatalf("rounds = %#v, want %#v", rounds, want)
	}
}

func TestResponseStreamHandlerRunWithoutChannelConsumerDoesNotBlock(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			for i := 0; i < defaultStreamResponseBuffer*4; i++ {
				if err := out.Output("x"); err != nil {
					return err
				}
			}
			return nil
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	inv.Stdin = bytes.NewBuffer(nil)

	done := make(chan error, 1)
	go func() {
		done <- inv.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("run blocked without response stream consumer")
	}
}
