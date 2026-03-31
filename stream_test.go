package redant

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestResponseStreamHandlerFallsBackToStdIO(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			if err := out.Send("phase:init\n"); err != nil {
				return err
			}
			return out.Send("hello, redant")
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
			return out.Send("echo:ping")
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

	var got string
	for evt := range out {
		if s, ok := evt.(string); ok {
			got = s
			break
		}
	}

	if got != "echo:ping" {
		t.Fatalf("got = %q, want %q", got, "echo:ping")
	}
}

func TestInvocationRunClosesResponseStream(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			return out.Send("done")
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

func TestResponseStreamHandlerRunWithoutChannelConsumerDoesNotBlock(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			for i := 0; i < defaultStreamResponseBuffer*4; i++ {
				if err := out.Send("x"); err != nil {
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
