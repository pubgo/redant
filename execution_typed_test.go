package redant

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
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
			if err := out.Output("hello"); err != nil {
				return err
			}
			return out.OutputChunk("world")
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
		t.Fatalf("missing output event: %v", got)
	}
	if !slices.Contains(got, "world") {
		t.Fatalf("missing chunk event: %v", got)
	}
}

func TestRunCallback_StreamTypeMismatch(t *testing.T) {
	cmd := &Command{
		Use: "chat",
		ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[int]) error {
			return out.Output(1)
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
			return out.Output("hello")
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
