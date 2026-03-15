package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/pubgo/redant"
)

func TestCommandsEndpoint(t *testing.T) {
	var global string
	var local string

	root := &redant.Command{
		Use: "testapp",
		Options: redant.OptionSet{
			{Flag: "global", Description: "global flag", Envs: []string{"GLOBAL_ENV"}, Value: redant.StringOf(&global)},
		},
	}

	echoCmd := &redant.Command{
		Use:     "echo [text]",
		Aliases: []string{"e"},
		Short:   "echo text",
		Long:    "echo text long description",
		Options: redant.OptionSet{
			{Flag: "local", Description: "local flag", Value: redant.StringOf(&local)},
		},
		Args: redant.ArgSet{
			{Name: "text", Description: "text to print", Required: true, Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = inv.Stdout.Write([]byte("ok"))
			return nil
		},
	}

	webCmd := &redant.Command{Use: "web", Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil }}
	root.Children = append(root.Children, echoCmd, webCmd)

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/commands")
	if err != nil {
		t.Fatalf("request commands: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload commandListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode commands response: %v", err)
	}

	if len(payload.Commands) != 1 {
		t.Fatalf("expected 1 command (web should be excluded), got %d", len(payload.Commands))
	}

	cmd := payload.Commands[0]
	if cmd.ID != "echo" {
		t.Fatalf("expected command id echo, got %s", cmd.ID)
	}
	if cmd.Use != "echo [text]" {
		t.Fatalf("expected use echo [text], got %s", cmd.Use)
	}
	if !slices.Equal(cmd.Aliases, []string{"e"}) {
		t.Fatalf("unexpected aliases: %+v", cmd.Aliases)
	}
	if cmd.Short != "echo text" {
		t.Fatalf("unexpected short: %s", cmd.Short)
	}
	if cmd.Long != "echo text long description" {
		t.Fatalf("unexpected long: %s", cmd.Long)
	}
	if len(cmd.Args) != 1 || cmd.Args[0].Name != "text" {
		t.Fatalf("unexpected args metadata: %+v", cmd.Args)
	}

	flagNames := make([]string, 0, len(cmd.Flags))
	flagByName := make(map[string]FlagMeta, len(cmd.Flags))
	for _, f := range cmd.Flags {
		flagNames = append(flagNames, f.Name)
		flagByName[f.Name] = f
	}
	if !slices.Contains(flagNames, "global") || !slices.Contains(flagNames, "local") {
		t.Fatalf("expected global+local flags, got: %v", flagNames)
	}
	if strings.TrimSpace(flagByName["local"].Description) == "" {
		t.Fatalf("expected local flag description, got empty")
	}
	if !slices.Equal(flagByName["global"].Envs, []string{"GLOBAL_ENV"}) {
		t.Fatalf("unexpected global envs: %+v", flagByName["global"].Envs)
	}
}

func TestIndexPageServedFromStatic(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{Use: "echo", Handler: func(ctx context.Context, inv *redant.Invocation) error {
		return nil
	}})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("request index: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read index body: %v", err)
	}

	page := string(body)
	if !strings.Contains(page, "cdn.tailwindcss.com") {
		t.Fatalf("expected tailwindcdn tag in page")
	}
	if !strings.Contains(page, "alpinejs") {
		t.Fatalf("expected alpinejs tag in page")
	}
}

func TestRunEndpoint(t *testing.T) {
	var text string
	var upper bool

	root := &redant.Command{Use: "testapp"}
	echoCmd := &redant.Command{
		Use: "echo",
		Options: redant.OptionSet{
			{Flag: "upper", Description: "uppercase", Value: redant.BoolOf(&upper)},
		},
		Args: redant.ArgSet{
			{Name: "text", Required: true, Value: redant.StringOf(&text)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			out := text
			if upper {
				out = strings.ToUpper(text)
			}
			_, _ = inv.Stdout.Write([]byte(out))
			return nil
		},
	}
	root.Children = append(root.Children, echoCmd)

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	requestBody := `{"command":"echo","flags":{"upper":true},"args":{"text":"hello"}}`
	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewBufferString(requestBody))
	if err != nil {
		t.Fatalf("run command request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var runResp RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}

	if !runResp.OK {
		t.Fatalf("expected ok response, got error=%s stderr=%s", runResp.Error, runResp.Stderr)
	}
	if runResp.Stdout != "HELLO" {
		t.Fatalf("expected HELLO, got %q", runResp.Stdout)
	}
	if !strings.Contains(runResp.Invocation, "echo") || !strings.Contains(runResp.Invocation, "--upper") {
		t.Fatalf("unexpected invocation: %s", runResp.Invocation)
	}
}

func TestRunEndpointMissingRequiredArg(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use:  "echo",
		Args: redant.ArgSet{{Name: "text", Required: true, Value: redant.StringOf(new(string))}},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return nil
		},
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewBufferString(`{"command":"echo","args":{}}`))
	if err != nil {
		t.Fatalf("run command request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing arg, got %d", resp.StatusCode)
	}
}
