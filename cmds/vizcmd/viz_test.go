package vizcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pubgo/redant"
)

func testRoot() *redant.Command {
	root := &redant.Command{
		Use:   "myapp",
		Short: "a test app",
	}
	root.Children = append(root.Children,
		&redant.Command{
			Use:   "greet",
			Short: "say hello",
			Handler: func(ctx context.Context, inv *redant.Invocation) error {
				return nil
			},
		},
		&redant.Command{
			Use:   "serve",
			Short: "start server (stream)",
			ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
				return nil
			}),
		},
		&redant.Command{
			Use:   "info",
			Short: "show info (unary)",
			ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (string, error) {
				return "ok", nil
			}),
		},
		&redant.Command{
			Use:        "old",
			Short:      "deprecated cmd",
			Hidden:     false,
			Deprecated: "use greet instead",
			Handler: func(ctx context.Context, inv *redant.Invocation) error {
				return nil
			},
		},
		&redant.Command{
			Use:    "secret",
			Short:  "hidden cmd",
			Hidden: true,
			Handler: func(ctx context.Context, inv *redant.Invocation) error {
				return nil
			},
		},
		&redant.Command{
			Use:   "group",
			Short: "a group",
			Children: []*redant.Command{
				{
					Use:   "sub",
					Short: "nested sub",
					Handler: func(ctx context.Context, inv *redant.Invocation) error {
						return nil
					},
				},
			},
		},
	)
	return root
}

func TestWriteTree(t *testing.T) {
	root := testRoot()
	var buf bytes.Buffer
	if err := WriteTree(&buf, root, 0); err != nil {
		t.Fatalf("WriteTree: %v", err)
	}
	out := buf.String()

	t.Run("starts_with_graph_TD", func(t *testing.T) {
		if !strings.HasPrefix(out, "graph TD\n") {
			t.Fatalf("expected graph TD header, got:\n%s", firstLines(out, 3))
		}
	})
	t.Run("contains_root_node", func(t *testing.T) {
		if !strings.Contains(out, "myapp") {
			t.Fatal("missing root node")
		}
	})
	t.Run("contains_child_nodes", func(t *testing.T) {
		for _, name := range []string{"greet", "serve", "info", "old"} {
			if !strings.Contains(out, name) {
				t.Errorf("missing child node %q", name)
			}
		}
	})
	t.Run("excludes_hidden_nodes", func(t *testing.T) {
		if strings.Contains(out, "secret") {
			t.Fatal("hidden command should not appear")
		}
	})
	t.Run("contains_edge_arrows", func(t *testing.T) {
		if !strings.Contains(out, "-->") {
			t.Fatal("missing edge arrows")
		}
	})
	t.Run("deprecated_has_dash_style", func(t *testing.T) {
		if !strings.Contains(out, "stroke-dasharray") {
			t.Fatal("deprecated command should have dashed stroke")
		}
	})
	t.Run("nested_subcommand_present", func(t *testing.T) {
		if !strings.Contains(out, "sub") {
			t.Fatal("nested subcommand missing")
		}
	})
	t.Run("stream_handler_styled", func(t *testing.T) {
		if !strings.Contains(out, "[[") {
			t.Fatal("stream handler should use stadium shape")
		}
	})
	t.Run("unary_handler_styled", func(t *testing.T) {
		if !strings.Contains(out, "#059669") {
			t.Fatal("unary handler should have green fill style")
		}
	})
}

func TestWriteTree_DepthLimit(t *testing.T) {
	root := testRoot()
	var buf bytes.Buffer
	if err := WriteTree(&buf, root, 1); err != nil {
		t.Fatalf("WriteTree depth=1: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "sub") {
		t.Fatal("depth=1 should not include nested subcommand")
	}
	if !strings.Contains(out, "greet") {
		t.Fatal("depth=1 should include immediate children")
	}
}

func TestWriteDispatch(t *testing.T) {
	root := testRoot()
	var buf bytes.Buffer
	if err := WriteDispatch(&buf, root); err != nil {
		t.Fatalf("WriteDispatch: %v", err)
	}
	out := buf.String()

	t.Run("starts_with_flowchart_TD", func(t *testing.T) {
		if !strings.HasPrefix(out, "flowchart TD\n") {
			t.Fatalf("expected flowchart TD, got:\n%s", firstLines(out, 3))
		}
	})
	t.Run("contains_subgraphs", func(t *testing.T) {
		for _, sg := range []string{"RESOLVE", "FLAGS", "SHORT", "EXEC"} {
			if !strings.Contains(out, "subgraph "+sg) {
				t.Errorf("missing subgraph %q", sg)
			}
		}
	})
	t.Run("contains_dispatch_nodes", func(t *testing.T) {
		for _, node := range []string{"getExecCommand", "resolveArgv0Command", "pflag.Parse", "Handler"} {
			if !strings.Contains(out, node) {
				t.Errorf("missing dispatch node %q", node)
			}
		}
	})
	t.Run("contains_info_stats", func(t *testing.T) {
		if !strings.Contains(out, "commands") {
			t.Fatal("missing command count in INFO node")
		}
	})
}

func TestWriteMCPSequence(t *testing.T) {
	root := testRoot()

	t.Run("generic", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteMCPSequence(&buf, root, ""); err != nil {
			t.Fatalf("WriteMCPSequence: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, "sequenceDiagram\n") {
			t.Fatalf("expected sequenceDiagram header, got:\n%s", firstLines(out, 3))
		}
		for _, participant := range []string{"Agent", "MCP", "Router", "Handler"} {
			if !strings.Contains(out, "participant "+participant) {
				t.Errorf("missing participant %q", participant)
			}
		}
		if !strings.Contains(out, "tools/list") {
			t.Fatal("missing tools/list step")
		}
		if !strings.Contains(out, "tools/call") {
			t.Fatal("missing tools/call step")
		}
		if !strings.Contains(out, "alt Unary") {
			t.Fatal("missing Unary alt branch")
		}
		if !strings.Contains(out, "CallToolResult") {
			t.Fatal("missing CallToolResult")
		}
	})

	t.Run("specific_tool", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteMCPSequence(&buf, root, "greet"); err != nil {
			t.Fatalf("WriteMCPSequence with tool: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "greet") {
			t.Fatal("specific tool name should appear in diagram")
		}
	})
}

func TestEscMermaid(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"a<b>c", "a&lt;b&gt;c"},
	}
	for _, tc := range tests {
		got := escMermaid(tc.in)
		if got != tc.want {
			t.Errorf("escMermaid(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"a long description here", 10, "a long ..."},
		{"exactly10!", 10, "exactly10!"},
	}
	for _, tc := range tests {
		got := truncate(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestNodeID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"greet", "greet"},
		{"my-cmd", "my_cmd"},
	}
	for _, tc := range tests {
		got := nodeID(tc.in)
		if got != tc.want {
			t.Errorf("nodeID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
