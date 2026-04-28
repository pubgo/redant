package llmstxtcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pubgo/redant"
)

func TestWriteLLMSTxt_Golden(t *testing.T) {
	tests := []struct {
		name     string
		golden   string
		maxDepth int
		root     func() *redant.Command
	}{
		{
			name:   "basic_command_tree",
			golden: "basic_command_tree.golden",
			root:   newBasicRoot,
		},
		{
			name:     "depth_limit",
			golden:   "depth_limit.golden",
			maxDepth: 1,
			root:     newNestedRoot,
		},
		{
			name:   "hidden_excluded",
			golden: "hidden_excluded.golden",
			root:   newHiddenRoot,
		},
		{
			name:   "global_options",
			golden: "global_options.golden",
			root:   newGlobalOptsRoot,
		},
		{
			name:   "response_types",
			golden: "response_types.golden",
			root:   newResponseTypesRoot,
		},
		{
			name:   "comprehensive",
			golden: "comprehensive.golden",
			root:   newComprehensiveRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()

			var buf bytes.Buffer
			if err := WriteLLMSTxt(&buf, root, tt.maxDepth); err != nil {
				t.Fatalf("WriteLLMSTxt error: %v", err)
			}

			got := buf.String()
			wantPath := filepath.Join("testdata", tt.golden)

			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("mkdir testdata: %v", err)
				}
				if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
					t.Fatalf("update golden %s: %v", wantPath, err)
				}
			}

			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", wantPath, err)
			}

			if got != string(want) {
				t.Fatalf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					tt.golden, got, string(want))
			}
		})
	}
}

func TestNewCommand_Integration(t *testing.T) {
	root := &redant.Command{Use: "app", Short: "Test app."}
	root.Children = append(root.Children, &redant.Command{
		Use:     "hello",
		Short:   "Say hello.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	root.Children = append(root.Children, New())

	var buf bytes.Buffer
	inv := root.Invoke("llms-txt")
	inv.Stdout = &buf
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("run llms-txt: %v", err)
	}

	got := buf.String()
	wantPath := filepath.Join("testdata", "integration.golden")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
			t.Fatalf("update golden %s: %v", wantPath, err)
		}
	}

	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", wantPath, err)
	}

	if got != string(want) {
		t.Fatalf("output mismatch for integration\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}

// --- test fixtures ---

func newBasicRoot() *redant.Command {
	var msg string
	var upper bool

	root := &redant.Command{
		Use:   "myapp",
		Short: "My sample application.",
	}
	root.Children = append(root.Children, &redant.Command{
		Use:     "echo [message]",
		Short:   "Prints a message.",
		Aliases: []string{"ec"},
		Options: redant.OptionSet{
			{Flag: "upper", Shorthand: "u", Description: "Uppercase output.", Value: redant.BoolOf(&upper)},
		},
		Args: redant.ArgSet{
			{Name: "message", Description: "Text to echo.", Required: true, Value: redant.StringOf(&msg)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	return root
}

func newNestedRoot() *redant.Command {
	root := &redant.Command{Use: "app"}
	level1 := &redant.Command{
		Use:     "l1",
		Short:   "Level 1.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
	level2 := &redant.Command{
		Use:     "l2",
		Short:   "Level 2.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
	level1.Children = append(level1.Children, level2)
	root.Children = append(root.Children, level1)
	return root
}

func newHiddenRoot() *redant.Command {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children,
		&redant.Command{
			Use:     "visible",
			Short:   "Visible cmd.",
			Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
		},
		&redant.Command{
			Use:     "secret",
			Short:   "Secret cmd.",
			Hidden:  true,
			Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
		},
	)
	return root
}

func newGlobalOptsRoot() *redant.Command {
	root := &redant.Command{
		Use:   "app",
		Short: "App with globals.",
		Options: redant.OptionSet{
			{Flag: "verbose", Shorthand: "v", Description: "Enable verbose.", Value: redant.BoolOf(new(bool))},
			{Flag: "config", Description: "Config file path.", Value: redant.StringOf(new(string)), Envs: []string{"APP_CONFIG"}},
		},
	}
	root.Children = append(root.Children, &redant.Command{
		Use:     "run",
		Short:   "Run something.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	return root
}

func newResponseTypesRoot() *redant.Command {
	type StatusResult struct {
		OK bool `json:"ok"`
	}

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children,
		&redant.Command{
			Use:   "status",
			Short: "Get status.",
			ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (StatusResult, error) {
				return StatusResult{OK: true}, nil
			}),
		},
		&redant.Command{
			Use:   "logs",
			Short: "Stream logs.",
			ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
				return out.Send("line")
			}),
		},
	)
	return root
}

func newComprehensiveRoot() *redant.Command {
	var (
		output   string
		force    bool
		count    int64
		endpoint string
	)

	root := &redant.Command{
		Use:   "myctl",
		Short: "My control tool.",
		Long:  "A comprehensive CLI tool for managing resources.",
		Options: redant.OptionSet{
			{Flag: "output", Shorthand: "o", Description: "Output format.", Value: redant.EnumOf(&output, "text", "json", "yaml"), Default: "text"},
		},
	}

	deployCmd := &redant.Command{
		Use:   "deploy [target]",
		Short: "Deploy to environment.",
		Long:  "Deploy the application to the specified target environment.",
		Options: redant.OptionSet{
			{Flag: "force", Shorthand: "f", Description: "Force deploy.", Value: redant.BoolOf(&force)},
			{Flag: "count", Description: "Instance count.", Value: redant.Int64Of(&count), Default: "1", Required: true},
			{Flag: "endpoint", Description: "Deploy endpoint.", Value: redant.StringOf(&endpoint), Envs: []string{"DEPLOY_ENDPOINT"}, Default: "https://deploy.example.com"},
		},
		Args: redant.ArgSet{
			{Name: "target", Description: "Target environment.", Required: true, Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	rollbackCmd := &redant.Command{
		Use:        "rollback",
		Short:      "Rollback deployment.",
		Deprecated: "Use 'deploy --rollback' instead.",
		Handler:    func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	deployCmd.Children = append(deployCmd.Children, rollbackCmd)
	root.Children = append(root.Children, deployCmd)
	return root
}
