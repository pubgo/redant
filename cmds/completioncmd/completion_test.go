package completioncmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pubgo/redant"
)

// TestCompletionCommandBasic tests the basic functionality of CompletionCommand
func TestCompletionCommandBasic(t *testing.T) {
	// Create a simple command structure with completion command
	rootCmd := &redant.Command{
		Use:   "testapp",
		Short: "Test application",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return nil
		},
	}

	// Add completion command to root command
	AddCompletionCommand(rootCmd)

	// Test that completion command is added correctly
	if len(rootCmd.Children) != 1 {
		t.Fatalf("Expected 1 child command, got %d", len(rootCmd.Children))
	}

	if rootCmd.Children[0].Use != "completion [shell]" {
		t.Fatalf("Expected completion command, got %s", rootCmd.Children[0].Use)
	}
}

// TestCompletionCommandMissingShell tests error handling for missing shell argument
func TestCompletionCommandMissingShell(t *testing.T) {
	// Create a simple command structure
	rootCmd := &redant.Command{
		Use:   "testapp",
		Short: "Test application",
	}

	AddCompletionCommand(rootCmd)

	// Capture stdout and stderr
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Create invocation
	inv := &redant.Invocation{
		Command: New(),
		Args:    []string{}, // No shell argument
		Stdout:  stdout,
		Stderr:  stderr,
	}

	// Get the completion command from rootCmd.Children
	completionCmd := rootCmd.Children[len(rootCmd.Children)-1]
	// Update the invocation to use this command instance
	inv.Command = completionCmd

	// Execute the command handler
	err := completionCmd.Handler(context.Background(), inv)

	// Verify error
	if err == nil {
		t.Fatal("Expected error for missing shell argument, got nil")
	}

	// Verify error message
	if !bytes.Contains(stderr.Bytes(), []byte("error: shell argument is required")) {
		t.Fatalf("Expected error message about missing shell argument, got: %s", stderr.String())
	}

	// Verify available shells are listed
	if !bytes.Contains(stderr.Bytes(), []byte("Available shells: bash, zsh, fish")) {
		t.Fatalf("Expected available shells list, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Fatalf("Expected empty stdout on missing shell, got: %s", stdout.String())
	}
}

// TestCompletionCommandUnsupportedShell tests error handling for unsupported shell
func TestCompletionCommandUnsupportedShell(t *testing.T) {
	// Create a simple command structure
	rootCmd := &redant.Command{
		Use:   "testapp",
		Short: "Test application",
	}

	AddCompletionCommand(rootCmd)

	// Capture stdout and stderr
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Create invocation
	inv := &redant.Invocation{
		Command: New(),
		Args:    []string{"invalid_shell"}, // Unsupported shell
		Stdout:  stdout,
		Stderr:  stderr,
	}

	// Use the actual completion command from rootCmd.Children
	completionCmd := rootCmd.Children[len(rootCmd.Children)-1]
	// Update the invocation to use this command
	inv.Command = completionCmd

	// Execute the command handler directly
	err := completionCmd.Handler(context.Background(), inv)

	// Verify error
	if err == nil {
		t.Fatal("Expected error for unsupported shell, got nil")
	}

	// Verify error message
	if !bytes.Contains(stderr.Bytes(), []byte("error: unsupported shell: invalid_shell")) {
		t.Fatalf("Expected error message about unsupported shell, got: %s", stderr.String())
	}

	// Verify available shells are listed
	if !bytes.Contains(stderr.Bytes(), []byte("Available shells: bash, zsh, fish")) {
		t.Fatalf("Expected available shells list, got: %s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Fatalf("Expected empty stdout on unsupported shell, got: %s", stdout.String())
	}
}

func TestCompletionCommandGeneratesScriptsForSupportedShells(t *testing.T) {
	oldArg0 := os.Args[0]
	os.Args[0] = "testapp"
	defer func() { os.Args[0] = oldArg0 }()

	tests := []struct {
		name   string
		shell  string
		golden string
	}{
		{name: "bash", shell: "bash", golden: "testapp.bash.golden"},
		{name: "zsh", shell: "zsh", golden: "testapp.zsh.golden"},
		{name: "fish", shell: "fish", golden: "testapp.fish.golden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := newCompletionTestRoot()
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			inv := rootCmd.Invoke("completion", tt.shell)
			inv.Stdout = stdout
			inv.Stderr = stderr

			if err := inv.Run(); err != nil {
				t.Fatalf("run completion %s: %v", tt.shell, err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("expected empty stderr, got: %s", stderr.String())
			}

			wantPath := filepath.Join("testdata", tt.golden)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(wantPath, []byte(stdout.String()), 0o644); err != nil {
					t.Fatalf("update golden %s: %v", wantPath, err)
				}
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read golden %s: %v", wantPath, err)
			}

			got := stdout.String()
			if got != string(want) {
				t.Fatalf("generated %s script mismatch\n--- got ---\n%s\n--- want ---\n%s", tt.shell, got, string(want))
			}
		})
	}
}

func newCompletionTestRoot() *redant.Command {
	var (
		verbose      bool
		outputFormat string
		configFile   string

		projectNS  string
		projectAll bool

		repoRegion string
		repoForce  bool

		createPrivate bool
		templateFile  string
		tags          []string
	)

	rootCmd := &redant.Command{
		Use:   "testapp",
		Short: "Test application",
		Options: redant.OptionSet{
			{Flag: "verbose", Shorthand: "v", Description: "verbose mode", Value: redant.BoolOf(&verbose)},
			{Flag: "output", Description: "output format", Value: redant.EnumOf(&outputFormat, "text", "json", "yaml"), Default: "text"},
			{Flag: "config", Description: "config file", Value: redant.StringOf(&configFile)},
		},
	}

	projectCmd := &redant.Command{
		Use:   "project",
		Short: "manage projects",
		Args: redant.ArgSet{
			{Name: "project_name", Required: false, Value: redant.StringOf(new(string)), Description: "project name"},
		},
		Options: redant.OptionSet{
			{Flag: "namespace", Description: "project namespace", Value: redant.StringOf(&projectNS)},
			{Flag: "all", Description: "apply to all projects", Value: redant.BoolOf(&projectAll)},
		},
	}

	repoCmd := &redant.Command{
		Use:   "repo",
		Short: "manage repositories",
		Args: redant.ArgSet{
			{Name: "repo_name", Required: false, Value: redant.StringOf(new(string)), Description: "repository name"},
		},
		Options: redant.OptionSet{
			{Flag: "region", Description: "target region", Value: redant.EnumOf(&repoRegion, "cn", "us", "eu"), Default: "cn"},
			{Flag: "force", Description: "force operation", Value: redant.BoolOf(&repoForce)},
		},
	}

	createCmd := &redant.Command{
		Use:   "create",
		Short: "create repository",
		Args: redant.ArgSet{
			{Name: "repo", Required: true, Value: redant.StringOf(new(string)), Description: "repository id"},
		},
		Options: redant.OptionSet{
			{Flag: "private", Description: "create private repository", Value: redant.BoolOf(&createPrivate)},
			{Flag: "template", Description: "template file", Value: redant.StringOf(&templateFile)},
			{Flag: "tags", Description: "repository tags", Value: redant.StringArrayOf(&tags)},
		},
	}

	repoCmd.Children = append(repoCmd.Children, createCmd)
	projectCmd.Children = append(projectCmd.Children, repoCmd)

	rootCmd.Children = append(rootCmd.Children,
		&redant.Command{Use: "hello", Short: "say hello", Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil }},
		&redant.Command{Use: "secret", Short: "hidden command", Hidden: true, Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil }},
		projectCmd,
	)
	AddCompletionCommand(rootCmd)
	return rootCmd
}
