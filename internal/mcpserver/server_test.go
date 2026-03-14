package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pubgo/redant"
)

func TestCollectToolsAndSchema(t *testing.T) {
	var msg string
	var upper bool

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "echo",
		Short: "echo message",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&msg)},
		},
		Options: redant.OptionSet{
			{Flag: "upper", Value: redant.BoolOf(&upper), Description: "uppercase"},
			{Flag: "secret", Value: redant.StringOf(new(string)), Hidden: true},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	s := New(root)
	if len(s.tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(s.tools))
	}

	tool := s.tools[0]
	if tool.Name != "echo" {
		t.Fatalf("tool name = %q, want %q", tool.Name, "echo")
	}

	flags, ok := tool.InputSchema["properties"].(map[string]any)["flags"].(map[string]any)
	if !ok {
		t.Fatalf("flags schema missing")
	}
	flagProps, ok := flags["properties"].(map[string]any)
	if !ok {
		t.Fatalf("flags properties missing")
	}
	if _, exists := flagProps["upper"]; !exists {
		t.Fatalf("expected upper flag in schema")
	}
	if _, exists := flagProps["secret"]; exists {
		t.Fatalf("hidden flag should not be exposed")
	}
}

func TestCallToolSuccess(t *testing.T) {
	var msg string
	var upper bool

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "echo",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&msg)},
		},
		Options: redant.OptionSet{
			{Flag: "upper", Value: redant.BoolOf(&upper)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			if upper {
				_, _ = fmt.Fprint(inv.Stdout, strings.ToUpper(msg))
				return nil
			}
			_, _ = fmt.Fprint(inv.Stdout, msg)
			return nil
		},
	})

	s := New(root)
	result, err := s.callTool(context.Background(), toolsCallParams{
		Name: "echo",
		Arguments: map[string]any{
			"args":  map[string]any{"message": "hello"},
			"flags": map[string]any{"upper": true},
		},
	})
	if err != nil {
		t.Fatalf("callTool error: %v", err)
	}

	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("invalid content payload: %#v", result["content"])
	}
	text, _ := content[0]["text"].(string)
	if !strings.Contains(text, "HELLO") {
		t.Fatalf("content text = %q, want contains HELLO", text)
	}

	isError, _ := result["isError"].(bool)
	if isError {
		t.Fatalf("expected success result, got error")
	}
}

func TestServeSDKClientListAndCallTool(t *testing.T) {
	var msg string
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "echo",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&msg)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = fmt.Fprint(inv.Stdout, strings.ToUpper(msg))
			return nil
		},
	})

	srv := New(root)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()
	if initRes := session.InitializeResult(); initRes == nil || initRes.ServerInfo == nil {
		t.Fatalf("initialize result or server info is nil")
	} else if initRes.ServerInfo.Name != "app" {
		t.Fatalf("server info name = %q, want %q", initRes.ServerInfo.Name, "app")
	}

	listRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listRes.Tools) == 0 {
		t.Fatalf("expected at least one tool")
	}
	if listRes.Tools[0].Name != "echo" {
		t.Fatalf("tool name = %q, want %q", listRes.Tools[0].Name, "echo")
	}

	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "echo",
		Arguments: map[string]any{
			"args": map[string]any{"message": "hello"},
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if callRes.IsError {
		t.Fatalf("call tool returned error result")
	}
	if len(callRes.Content) == 0 {
		t.Fatalf("call tool content is empty")
	}
	text, ok := callRes.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content is not text")
	}
	if !strings.Contains(text.Text, "HELLO") {
		t.Fatalf("content text = %q, want contains HELLO", text.Text)
	}

	structured, ok := callRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is not object: %#v", callRes.StructuredContent)
	}
	if okVal, _ := structured["ok"].(bool); !okVal {
		t.Fatalf("structured ok = %#v, want true", structured["ok"])
	}
	if stdout, _ := structured["stdout"].(string); !strings.Contains(stdout, "HELLO") {
		t.Fatalf("structured stdout = %q, want contains HELLO", stdout)
	}

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func TestServeSDKClientValidatesToolDescriptionAndParameters(t *testing.T) {
	var (
		service string
		stage   string
		dryRun  bool
	)

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "deploy",
		Short: "deploy service",
		Long:  "deploy service to target environment",
		Args: redant.ArgSet{
			{Name: "service", Required: true, Value: redant.StringOf(&service), Description: "service name"},
		},
		Options: redant.OptionSet{
			{Flag: "stage", Value: redant.EnumOf(&stage, "dev", "prod"), Required: true, Description: "target environment"},
			{Flag: "dry-run", Value: redant.BoolOf(&dryRun), Description: "only print action"},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			if dryRun {
				_, _ = fmt.Fprintf(inv.Stdout, "dry-run deploy %s to %s", service, stage)
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stdout, "deploy %s to %s", service, stage)
			return nil
		},
	})

	srv := New(root)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	listRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	var deployTool *mcp.Tool
	for i := range listRes.Tools {
		if listRes.Tools[i] != nil && listRes.Tools[i].Name == "deploy" {
			deployTool = listRes.Tools[i]
			break
		}
	}
	if deployTool == nil {
		t.Fatalf("tool deploy not found in %#v", listRes.Tools)
	}

	if deployTool.Description != "deploy service\n\ndeploy service to target environment" {
		t.Fatalf("description = %q", deployTool.Description)
	}

	assertJSONSubset(t, deployTool.InputSchema, `{
	  "type": "object",
	  "additionalProperties": false,
	  "properties": {
	    "args": {
	      "type": "object",
	      "required": ["service"],
	      "properties": {
	        "service": {"type": "string", "description": "service name"}
	      }
	    },
	    "flags": {
	      "type": "object",
	      "required": ["stage"],
	      "properties": {
	        "stage": {"type": "string", "enum": ["dev", "prod"], "description": "target environment"},
	        "dry-run": {"type": "boolean", "description": "only print action"}
	      }
	    }
	  }
	}`)

	assertJSONSubset(t, deployTool.OutputSchema, `{
	  "type": "object",
	  "required": ["ok", "stdout", "stderr", "error", "combined"],
	  "properties": {
	    "ok": {"type": "boolean"},
	    "stdout": {"type": "string"},
	    "stderr": {"type": "string"},
	    "error": {"type": "string"},
	    "combined": {"type": "string"}
	  }
	}`)

	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "deploy",
		Arguments: map[string]any{
			"args": map[string]any{"service": "api"},
			"flags": map[string]any{
				"stage":   "dev",
				"dry-run": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("call tool valid params: %v", err)
	}
	if callRes.IsError {
		t.Fatalf("valid call should not be error, got: %q", firstText(callRes.Content))
	}
	if len(callRes.Content) == 0 {
		t.Fatalf("call tool content is empty")
	}
	text, ok := callRes.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content is not text")
	}
	if !strings.Contains(text.Text, "dry-run deploy api to dev") {
		t.Fatalf("content text = %q", text.Text)
	}

	assertJSONSubset(t, callRes.StructuredContent, `{
	  "ok": true,
	  "stdout": "dry-run deploy api to dev",
	  "stderr": "",
	  "error": "",
	  "combined": "dry-run deploy api to dev"
	}`)

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func TestServeSDKClientStructFlagAndArg(t *testing.T) {
	type payload struct {
		Name string `json:"name" yaml:"name"`
		Port int    `json:"port" yaml:"port"`
	}

	argPayload := &redant.Struct[payload]{}
	flagPayload := &redant.Struct[payload]{}

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "apply",
		Short: "apply config",
		Long:  "apply config with structured arg and structured flag",
		Args: redant.ArgSet{
			{Name: "config", Required: true, Value: argPayload, Description: "config payload"},
		},
		Options: redant.OptionSet{
			{Flag: "meta", Value: flagPayload, Required: true, Description: "meta payload"},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = fmt.Fprintf(inv.Stdout,
				"arg=%s:%d flag=%s:%d",
				argPayload.Value.Name,
				argPayload.Value.Port,
				flagPayload.Value.Name,
				flagPayload.Value.Port,
			)
			return nil
		},
	})

	srv := New(root)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	listRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	var applyTool *mcp.Tool
	for i := range listRes.Tools {
		if listRes.Tools[i] != nil && listRes.Tools[i].Name == "apply" {
			applyTool = listRes.Tools[i]
			break
		}
	}
	if applyTool == nil {
		t.Fatalf("tool apply not found in %#v", listRes.Tools)
	}

	assertJSONSubset(t, applyTool.InputSchema, `{
	  "type": "object",
	  "additionalProperties": false,
	  "properties": {
	    "args": {
	      "type": "object",
	      "required": ["config"],
	      "properties": {
	        "config": {"type": "object", "description": "config payload"}
	      }
	    },
	    "flags": {
	      "type": "object",
	      "required": ["meta"],
	      "properties": {
	        "meta": {"type": "object", "description": "meta payload"}
	      }
	    }
	  }
	}`)

	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "apply",
		Arguments: map[string]any{
			"args": map[string]any{
				"config": map[string]any{
					"name": "api",
					"port": 8080,
				},
			},
			"flags": map[string]any{
				"meta": map[string]any{
					"name": "prod",
					"port": 9000,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if callRes.IsError {
		t.Fatalf("call with struct payloads should succeed, got: %q", firstText(callRes.Content))
	}

	if len(callRes.Content) == 0 {
		t.Fatalf("call tool content is empty")
	}
	text, ok := callRes.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content is not text")
	}
	if !strings.Contains(text.Text, "arg=api:8080 flag=prod:9000") {
		t.Fatalf("content text = %q", text.Text)
	}

	assertJSONSubset(t, callRes.StructuredContent, `{
	  "ok": true,
	  "stdout": "arg=api:8080 flag=prod:9000",
	  "stderr": "",
	  "error": "",
	  "combined": "arg=api:8080 flag=prod:9000"
	}`)

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func firstText(content []mcp.Content) string {
	if len(content) == 0 {
		return ""
	}
	t, ok := content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return t.Text
}

func assertJSONSubset(t *testing.T, got any, wantJSON string) {
	t.Helper()

	gotNormalized := normalizeJSONLike(t, got)

	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("invalid expected json: %v\n%s", err, wantJSON)
	}

	if err := checkJSONSubset(gotNormalized, want, "$", true); err != nil {
		t.Fatalf("json contract mismatch: %v\nwant subset:\n%s\ngot:\n%s", err, prettyJSON(want), prettyJSON(gotNormalized))
	}
}

func normalizeJSONLike(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json-like value failed: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal json-like value failed: %v", err)
	}
	return out
}

func checkJSONSubset(got, want any, path string, exactArray bool) error {
	switch wantTyped := want.(type) {
	case map[string]any:
		gotMap, ok := got.(map[string]any)
		if !ok {
			return fmt.Errorf("%s expected object, got %T", path, got)
		}
		for k, wantV := range wantTyped {
			gotV, exists := gotMap[k]
			if !exists {
				return fmt.Errorf("%s.%s missing", path, k)
			}
			if err := checkJSONSubset(gotV, wantV, path+"."+k, exactArray); err != nil {
				return err
			}
		}
		return nil

	case []any:
		gotArr, ok := got.([]any)
		if !ok {
			return fmt.Errorf("%s expected array, got %T", path, got)
		}
		if exactArray && len(gotArr) != len(wantTyped) {
			return fmt.Errorf("%s expected array len %d, got %d", path, len(wantTyped), len(gotArr))
		}
		for i := range wantTyped {
			if i >= len(gotArr) {
				return fmt.Errorf("%s[%d] missing", path, i)
			}
			if err := checkJSONSubset(gotArr[i], wantTyped[i], fmt.Sprintf("%s[%d]", path, i), exactArray); err != nil {
				return err
			}
		}
		return nil

	default:
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("%s expected %#v, got %#v", path, want, got)
		}
		return nil
	}
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(b)
}
