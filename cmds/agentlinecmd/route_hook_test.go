package agentlinecmd

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/pubgo/redant"
	agentlinemodule "github.com/pubgo/redant/pkg/agentline"
)

func TestAgentCommandRedirectsToAgentlineByHook(t *testing.T) {
	ensureRouteHookRegistered()

	var (
		executed   string
		initialArg []string
	)

	agentline := New()
	agentline.Handler = func(ctx context.Context, inv *redant.Invocation) error {
		executed = "agentline"
		return nil
	}
	agentline.Options = redant.OptionSet{
		{Flag: "initial-arg", Value: redant.StringArrayOf(&initialArg)},
	}

	commit := &redant.Command{
		Use:      "commit",
		Metadata: map[string]string{agentlinemodule.CommandMetaAgentEntry: "true"},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			executed = "commit"
			return nil
		},
	}

	root := &redant.Command{Use: "app", Children: []*redant.Command{commit, agentline}}

	inv := root.Invoke("commit", "hello world", "--dry-run")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executed != "agentline" {
		t.Fatalf("expected agentline executed, got %q", executed)
	}

	want := []string{"commit", "hello world", "--dry-run"}
	if !reflect.DeepEqual(initialArg, want) {
		t.Fatalf("initial-arg = %#v, want %#v", initialArg, want)
	}
}

func TestAgentCommandWithoutAgentlineFallbackByHook(t *testing.T) {
	ensureRouteHookRegistered()

	var executed string

	commit := &redant.Command{
		Use:      "commit",
		Metadata: map[string]string{agentlinemodule.CommandMetaAgentEntry: "true"},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			executed = "commit"
			return nil
		},
	}

	root := &redant.Command{Use: "app", Children: []*redant.Command{commit}}

	inv := root.Invoke("commit")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executed != "commit" {
		t.Fatalf("expected fallback to original command, got %q", executed)
	}
}

func TestAgentCommandDoesNotAutoRedirectByHook(t *testing.T) {
	ensureRouteHookRegistered()

	var executed string

	agentline := New()
	agentline.Handler = func(ctx context.Context, inv *redant.Invocation) error {
		executed = "agentline"
		return nil
	}

	commit := &redant.Command{
		Use:      "commit",
		Metadata: agentlinemodule.AgentCommandMetadata(),
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			executed = "commit"
			return nil
		},
	}

	root := &redant.Command{Use: "app", Children: []*redant.Command{commit, agentline}}

	inv := root.Invoke("commit")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executed != "commit" {
		t.Fatalf("expected normal command execution, got %q", executed)
	}
}
