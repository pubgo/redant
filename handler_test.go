package redant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRun_NoResponseHandler(t *testing.T) {
	called := false
	cmd := &Command{
		Use: "echo",
		Handler: func(ctx context.Context, inv *Invocation) error {
			called = true
			_, _ = inv.Stdout.Write([]byte("ok"))
			return nil
		},
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard

	if err := inv.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !called {
		t.Fatalf("expected handler called")
	}
}

func TestRunCallback_UnaryTyped(t *testing.T) {
	type reply struct{ Message string }

	cmd := &Command{
		Use: "echo",
		ResponseHandler: Unary(func(ctx context.Context, inv *Invocation) (reply, error) {
			return reply{Message: "ok"}, nil
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard

	var got string
	err := RunCallback[reply](inv, func(v reply) error {
		got = v.Message
		return nil
	})
	if err != nil {
		t.Fatalf("run callback failed: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got=%q, want=%q", got, "ok")
	}
}

func TestRunCallback_StreamTyped(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			if err := out.Send("hello"); err != nil {
				return err
			}
			return out.Send("world")
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard

	var got []string
	err := RunCallback[string](inv, func(v string) error {
		got = append(got, v)
		return nil
	})
	if err != nil {
		t.Fatalf("run callback failed: %v", err)
	}

	if !slices.Contains(got, "hello") {
		t.Fatalf("missing hello: %v", got)
	}
	if !slices.Contains(got, "world") {
		t.Fatalf("missing world: %v", got)
	}
}

func TestRunCallback_StreamTypeMismatch(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[int]) error {
			return out.Send(1)
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard

	err := RunCallback[string](inv, func(v string) error {
		return nil
	})
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
}

func TestRunCallback_UnaryTypeMismatch(t *testing.T) {
	cmd := &Command{
		Use: "echo",
		ResponseHandler: Unary(func(ctx context.Context, inv *Invocation) (int, error) {
			return 1, nil
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard

	err := RunCallback[string](inv, func(v string) error {
		return nil
	})
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
}

func TestRunCallback_CallbackError(t *testing.T) {
	wantErr := errors.New("stop")
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			return out.Send("hello")
		}),
	}

	inv := cmd.Invoke()
	inv.Stdout = io.Discard
	inv.Stderr = io.Discard

	err := RunCallback[string](inv, func(v string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestResponseStreamHandlerFallsBackToStdIO(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
			if err := out.Send("phase:init"); err != nil {
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

	// stdout should contain NDJSON envelopes
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d: %q", len(lines), stdout.String())
	}

	for i, line := range lines {
		var env StreamEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			t.Fatalf("line %d: invalid NDJSON: %v", i, err)
		}
		if env.Kind != "resp" {
			t.Fatalf("line %d: $.kind=%q, want \"resp\"", i, env.Kind)
		}
	}

	// verify first envelope data
	var env0 StreamEnvelope
	_ = json.Unmarshal([]byte(lines[0]), &env0)
	if data, ok := env0.Data.(string); !ok || data != "phase:init" {
		t.Fatalf("line 0: data=%v, want \"phase:init\"", env0.Data)
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
