package mcpclient

import (
	"context"
	"testing"

	"github.com/pubgo/redant"
)

func TestConnectInProcess(t *testing.T) {
	root := &redant.Command{Use: "testapp", Short: "Test application."}
	root.Children = append(root.Children, &redant.Command{
		Use:   "greet",
		Short: "Say hello.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = inv.Stdout.Write([]byte("hello"))
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, err := ConnectInProcess(ctx, root)
	if err != nil {
		t.Fatalf("ConnectInProcess: %v", err)
	}
	defer sess.Close() //nolint:errcheck // best-effort cleanup in test

	info := sess.ServerInfo()
	if info == nil {
		t.Fatalf("server info is nil")
	}
	if info.Name != "testapp" {
		t.Fatalf("server name = %q, want %q", info.Name, "testapp")
	}

	// List tools
	tools, err := sess.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "greet" {
		t.Fatalf("tool name = %q, want %q", tools[0].Name, "greet")
	}

	// Call tool
	result, err := sess.CallTool(ctx, "greet", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}
	text := ToolResultText(result)
	if text == "" {
		t.Fatalf("expected non-empty text result")
	}

	// List resources
	resources, err := sess.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) == 0 {
		t.Fatalf("expected at least 1 resource (llms.txt)")
	}

	// List prompts
	prompts, err := sess.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) == 0 {
		t.Fatalf("expected at least 1 prompt")
	}
}
