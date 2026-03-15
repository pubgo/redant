package mcpcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pubgo/redant"
)

func TestAddMCPCommand(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddMCPCommand(root)

	if len(root.Children) != 1 {
		t.Fatalf("children len = %d, want 1", len(root.Children))
	}

	mcp := root.Children[0]
	if mcp.Name() != "mcp" {
		t.Fatalf("child name = %q, want %q", mcp.Name(), "mcp")
	}

	if len(mcp.Children) != 2 {
		t.Fatalf("mcp children len = %d, want 2", len(mcp.Children))
	}

	hasList := false
	hasServe := false
	for _, child := range mcp.Children {
		switch child.Name() {
		case "list":
			hasList = true
		case "serve":
			hasServe = true
		}
	}

	if !hasList || !hasServe {
		t.Fatalf("expected mcp list and mcp serve subcommands")
	}
}

func TestMCPListCommandPrintsToolInfosJSONByDefault(t *testing.T) {
	root := &redant.Command{Use: "app"}

	var message string
	root.Children = append(root.Children, &redant.Command{
		Use:   "echo",
		Short: "echo one message",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&message), Description: "text to echo"},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	AddMCPCommand(root)

	var stdout bytes.Buffer
	inv := root.Invoke("mcp", "list")
	inv.Stdout = &stdout

	if err := inv.Run(); err != nil {
		t.Fatalf("run mcp list: %v", err)
	}

	var tools []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &tools); err != nil {
		t.Fatalf("parse mcp list output as json: %v\noutput:\n%s", err, stdout.String())
	}
	if len(tools) == 0 {
		t.Fatalf("mcp list output is empty")
	}

	var echoTool map[string]any
	for _, tool := range tools {
		if name, _ := tool["name"].(string); name == "echo" {
			echoTool = tool
			break
		}
	}
	if echoTool == nil {
		t.Fatalf("echo tool not found in mcp list output: %#v", tools)
	}

	if _, ok := echoTool["inputSchema"].(map[string]any); !ok {
		t.Fatalf("echo inputSchema missing: %#v", echoTool)
	}
	if _, ok := echoTool["outputSchema"].(map[string]any); !ok {
		t.Fatalf("echo outputSchema missing: %#v", echoTool)
	}
}

func TestMCPListCommandPrintsToolInfosText(t *testing.T) {
	root := &redant.Command{Use: "app"}

	var message string
	root.Children = append(root.Children, &redant.Command{
		Use:   "echo",
		Short: "echo one message",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&message), Description: "text to echo"},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	AddMCPCommand(root)

	var stdout bytes.Buffer
	inv := root.Invoke("mcp", "list", "--format", "text")
	inv.Stdout = &stdout

	if err := inv.Run(); err != nil {
		t.Fatalf("run mcp list --format text: %v", err)
	}

	out := stdout.String()
	for _, mustContain := range []string{
		"1. echo",
		"description: echo one message",
		"path: echo",
		"inputSchema: yes",
		"outputSchema: yes",
	} {
		if !strings.Contains(out, mustContain) {
			t.Fatalf("text output missing %q\noutput:\n%s", mustContain, out)
		}
	}
}
