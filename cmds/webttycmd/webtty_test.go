package webttycmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pubgo/redant"
)

func TestAddWebTTYCommand(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddWebTTYCommand(root)

	if len(root.Children) != 1 {
		t.Fatalf("expected one child command, got %d", len(root.Children))
	}
	if root.Children[0].Name() != "webtty" {
		t.Fatalf("expected child command webtty, got %s", root.Children[0].Name())
	}
}

func TestWebTTYCommandRunAndShutdown(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddWebTTYCommand(root)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inv := root.Invoke("webtty", "--addr", "127.0.0.1:0", "--open=false")
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
			t.Fatalf("webtty command run failed: %v (stderr=%s)", err, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("webtty command did not shutdown in time")
	}

	if !strings.Contains(stdout.String(), "webtty listening on") {
		t.Fatalf("expected startup output, got %q", stdout.String())
	}
}
