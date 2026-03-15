package webcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pubgo/redant"
)

func TestAddWebCommand(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddWebCommand(root)

	if len(root.Children) != 1 {
		t.Fatalf("expected one child command, got %d", len(root.Children))
	}
	if root.Children[0].Name() != "web" {
		t.Fatalf("expected child command web, got %s", root.Children[0].Name())
	}
}

func TestWebCommandRunAndShutdown(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "hello",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = inv.Stdout.Write([]byte("hello"))
			return nil
		},
	})
	AddWebCommand(root)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inv := root.Invoke("web", "--addr", "127.0.0.1:0", "--open=false")
	inv.Stdout = stdout
	inv.Stderr = stderr

	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("web command run failed: %v (stderr=%s)", err, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("web command did not shutdown in time")
	}

	if !strings.Contains(stdout.String(), "web ui listening on") {
		t.Fatalf("expected startup output, got %q", stdout.String())
	}
}
