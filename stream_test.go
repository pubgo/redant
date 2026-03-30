package redant

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"
)

func TestStreamHandlerFallsBackToStdIO(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		StreamHandler: func(ctx context.Context, stream *InvocationStream) error {
			if err := stream.Control("phase:init\n"); err != nil {
				return err
			}
			return stream.Output("hello, redant")
		},
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

func TestStreamHandlerWithChannels(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		StreamHandler: func(ctx context.Context, stream *InvocationStream) error {
			if err := stream.Output("echo:ping"); err != nil {
				return err
			}
			return stream.EndRound("done")
		},
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	inv.Stdin = bytes.NewBuffer(nil)

	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got StreamMessage
	for msg := range out {
		if msg.Type == StreamEventOutput {
			got = msg
			break
		}
	}

	if got.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want %q", got.JSONRPC, "2.0")
	}
	if got.ID == "" {
		t.Fatalf("id should not be empty")
	}
	if got.Type != StreamEventOutput {
		t.Fatalf("type = %q, want %q", got.Type, StreamEventOutput)
	}
	if got.Method != StreamMethodOutput {
		t.Fatalf("method = %q, want %q", got.Method, StreamMethodOutput)
	}
	if got.Text() != "echo:ping" {
		t.Fatalf("text = %q, want %q", got.Text(), "echo:ping")
	}
}

func TestStreamMessageNormalize(t *testing.T) {
	msg := (StreamMessage{Type: StreamEventOutput, Data: "hello"}).Normalize()

	if msg.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want %q", msg.JSONRPC, "2.0")
	}
	if msg.Type != StreamEventOutput {
		t.Fatalf("type = %q, want %q", msg.Type, StreamEventOutput)
	}
	if msg.Method != StreamMethodOutput {
		t.Fatalf("method = %q, want %q", msg.Method, StreamMethodOutput)
	}
	if msg.Text() != "hello" {
		t.Fatalf("text = %q, want %q", msg.Text(), "hello")
	}
}

func TestStreamControlUsesControlMethod(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		StreamHandler: func(ctx context.Context, stream *InvocationStream) error {
			return stream.Control("name?")
		},
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	inv.Stdin = bytes.NewBuffer(nil)

	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var msg StreamMessage
	for m := range out {
		if m.Type == StreamEventControl {
			msg = m
			break
		}
	}
	if msg.Method != StreamMethodControl {
		t.Fatalf("method = %q, want %q", msg.Method, StreamMethodControl)
	}
	if msg.ID == "" {
		t.Fatalf("id should not be empty")
	}
}

func TestInvocationRunClosesResponseStream(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		StreamHandler: func(ctx context.Context, stream *InvocationStream) error {
			return stream.Output("done")
		},
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
		StreamHandler: func(ctx context.Context, stream *InvocationStream) error {
			for i := 0; i < 2; i++ {
				if err := stream.EndRound("ok"); err != nil {
					return err
				}
			}
			return nil
		},
	}

	inv := cmd.Invoke()
	inv.Stdin = bytes.NewBuffer(nil)
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard
	out := inv.ResponseStream()

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for msg := range out {
		if msg.Method == StreamMethodRoundEnd {
			rounds = append(rounds, msg.Round)
		}
	}
	want := []int{1, 2}
	if !slices.Equal(rounds, want) {
		t.Fatalf("rounds = %#v, want %#v", rounds, want)
	}
}
