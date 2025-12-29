package completioncmd

import (
	"bytes"
	"context"
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
}
