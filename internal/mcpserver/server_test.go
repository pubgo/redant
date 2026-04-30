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
	initRes := session.InitializeResult()
	serverInfoName := ""
	if initRes != nil && initRes.ServerInfo != nil {
		serverInfoName = initRes.ServerInfo.Name
	}
	if serverInfoName == "" {
		t.Fatalf("initialize result or server info is nil")
	}
	if serverInfoName != "app" {
		t.Fatalf("server info name = %q, want %q", serverInfoName, "app")
	}

	listRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listRes.Tools) == 0 {
		t.Fatalf("expected at least one tool")
	}
	if listRes.Tools[0] == nil {
		t.Fatalf("first tool is nil")
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
	if callRes.Content[0] == nil {
		t.Fatalf("first content is nil")
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

	var (
		deployTool  mcp.Tool
		deployFound bool
	)
	for i := range listRes.Tools {
		if listRes.Tools[i] != nil && listRes.Tools[i].Name == "deploy" {
			deployTool = *listRes.Tools[i]
			deployFound = true
			break
		}
	}
	if !deployFound {
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

	var (
		applyTool  mcp.Tool
		applyFound bool
	)
	for i := range listRes.Tools {
		if listRes.Tools[i] != nil && listRes.Tools[i].Name == "apply" {
			applyTool = *listRes.Tools[i]
			applyFound = true
			break
		}
	}
	if !applyFound {
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

func TestCollectToolsIncludesStreamHandler(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "chat",
		Short: "stream chat",
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			return out.Send("hello")
		}),
	})

	s := New(root)
	if len(s.tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(s.tools))
	}

	tool := s.tools[0]
	if tool.Name != "chat" {
		t.Fatalf("tool name = %q, want %q", tool.Name, "chat")
	}
	if !tool.SupportsStream {
		t.Fatalf("expected SupportsStream=true")
	}
	if tool.ResponseType == nil {
		t.Fatalf("expected ResponseType to be set")
	}
	if tool.ResponseType.TypeName != "string" {
		t.Fatalf("ResponseType.TypeName = %q, want string", tool.ResponseType.TypeName)
	}
	// Output schema should include response field for stream tools.
	props, _ := tool.OutputSchema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("output schema properties missing")
	}
	respProp, ok := props["response"]
	if !ok {
		t.Fatalf("output schema should have response property for stream tool")
	}
	respMap, _ := respProp.(map[string]any)
	if respMap["type"] != "array" {
		t.Fatalf("response schema type = %v, want array for stream", respMap["type"])
	}
}

func TestCollectToolsIncludesResponseHandler(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "greet",
		Short: "unary greet",
		ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (string, error) {
			return "hi", nil
		}),
	})

	s := New(root)
	if len(s.tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(s.tools))
	}

	tool := s.tools[0]
	if tool.Name != "greet" {
		t.Fatalf("tool name = %q, want %q", tool.Name, "greet")
	}
	if tool.SupportsStream {
		t.Fatalf("expected SupportsStream=false for ResponseHandler")
	}
	if tool.ResponseType == nil {
		t.Fatalf("expected ResponseType to be set for ResponseHandler")
	}
	if tool.ResponseType.TypeName != "string" {
		t.Fatalf("ResponseType.TypeName = %q, want string", tool.ResponseType.TypeName)
	}
	// Output schema should include response field with type info for unary.
	props, _ := tool.OutputSchema["properties"].(map[string]any)
	respProp, ok := props["response"]
	if !ok {
		t.Fatalf("output schema should have response property for unary tool")
	}
	respMap, _ := respProp.(map[string]any)
	if respMap["type"] == "array" {
		t.Fatalf("unary response schema should not be array type")
	}
	if _, hasXType := respMap["x-redant-type"]; !hasXType {
		t.Fatalf("unary response schema should have x-redant-type")
	}
}

func TestCallToolWithStreamHandler(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "chat",
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			if err := out.Send("chunk-1"); err != nil {
				return err
			}
			return out.Send("chunk-2")
		}),
	})

	s := New(root)
	result, err := s.callTool(context.Background(), toolsCallParams{
		Name:      "chat",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("callTool error: %v", err)
	}

	isError, _ := result["isError"].(bool)
	if isError {
		t.Fatalf("expected success result, got error")
	}

	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent missing")
	}
	stdout, _ := structured["stdout"].(string)
	if !strings.Contains(stdout, "chunk-1") || !strings.Contains(stdout, "chunk-2") {
		t.Fatalf("stdout = %q, want contains chunk-1 and chunk-2", stdout)
	}
	// Verify typed response array is collected.
	responses, ok := structured["response"].([]any)
	if !ok {
		t.Fatalf("structured response should be an array, got %T", structured["response"])
	}
	if len(responses) != 2 {
		t.Fatalf("expected 2 response chunks, got %d", len(responses))
	}
	if responses[0] != "chunk-1" || responses[1] != "chunk-2" {
		t.Fatalf("response = %v, want [chunk-1, chunk-2]", responses)
	}
}

func TestCallToolWithResponseHandler(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "greet",
		ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (string, error) {
			return "hello-unary", nil
		}),
	})

	s := New(root)
	result, err := s.callTool(context.Background(), toolsCallParams{
		Name:      "greet",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("callTool error: %v", err)
	}

	isError, _ := result["isError"].(bool)
	if isError {
		t.Fatalf("expected success result, got error")
	}

	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent missing")
	}
	stdout, _ := structured["stdout"].(string)
	if !strings.Contains(stdout, "hello-unary") {
		t.Fatalf("stdout = %q, want contains hello-unary", stdout)
	}
	// Verify typed unary response is collected (single value, not array).
	respVal, ok := structured["response"]
	if !ok {
		t.Fatalf("structured response should exist for unary handler")
	}
	if respVal != "hello-unary" {
		t.Fatalf("response = %v, want hello-unary", respVal)
	}
}

func TestServeSDKClientCallStreamTool(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "chat",
		Short: "streaming chat tool",
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			if err := out.Send("hello"); err != nil {
				return err
			}
			return out.Send("world")
		}),
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
	if len(listRes.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(listRes.Tools))
	}
	if listRes.Tools[0].Name != "chat" {
		t.Fatalf("tool name = %q, want chat", listRes.Tools[0].Name)
	}

	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "chat",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if callRes.IsError {
		t.Fatalf("expected success, got error: %q", firstText(callRes.Content))
	}

	text := firstText(callRes.Content)
	if !strings.Contains(text, "hello") || !strings.Contains(text, "world") {
		t.Fatalf("content text = %q, want contains hello and world", text)
	}

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func TestResourcesRegistered(t *testing.T) {
	var msg string
	root := &redant.Command{
		Use:   "myapp",
		Short: "Test app.",
	}
	root.Children = append(root.Children, &redant.Command{
		Use:   "echo",
		Short: "echo message",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&msg)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = fmt.Fprint(inv.Stdout, msg)
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

	// List resources should include llms.txt + per-command help
	listRes, err := session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}

	if len(listRes.Resources) < 2 {
		t.Fatalf("expected at least 2 resources (llms.txt + echo help), got %d", len(listRes.Resources))
	}

	var llmsTxtURI string
	var echoHelpURI string
	for _, r := range listRes.Resources {
		if r.Name == "llms.txt" {
			llmsTxtURI = r.URI
		}
		if r.Name == "echo help" {
			echoHelpURI = r.URI
		}
	}

	if llmsTxtURI == "" {
		t.Fatalf("llms.txt resource not found")
	}
	if echoHelpURI == "" {
		t.Fatalf("echo help resource not found")
	}

	// Read llms.txt
	readRes, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: llmsTxtURI})
	if err != nil {
		t.Fatalf("read llms.txt: %v", err)
	}
	if len(readRes.Contents) == 0 {
		t.Fatalf("llms.txt contents empty")
	}
	text := readRes.Contents[0].Text
	for _, want := range []string{"# myapp", "## Commands", "myapp echo", "echo message"} {
		if !strings.Contains(text, want) {
			t.Errorf("llms.txt missing %q\ncontent:\n%s", want, text)
		}
	}

	// Read echo help
	readRes2, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: echoHelpURI})
	if err != nil {
		t.Fatalf("read echo help: %v", err)
	}
	if len(readRes2.Contents) == 0 {
		t.Fatalf("echo help contents empty")
	}
	echoText := readRes2.Contents[0].Text
	for _, want := range []string{"# echo", "`message`"} {
		if !strings.Contains(echoText, want) {
			t.Errorf("echo help missing %q\ncontent:\n%s", want, echoText)
		}
	}

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func TestPromptsRegistered(t *testing.T) {
	var msg string
	root := &redant.Command{
		Use:   "myapp",
		Short: "Test app.",
	}
	root.Children = append(root.Children, &redant.Command{
		Use:   "echo",
		Short: "echo message",
		Args: redant.ArgSet{
			{Name: "message", Required: true, Value: redant.StringOf(&msg)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = fmt.Fprint(inv.Stdout, msg)
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

	// List prompts
	listRes, err := session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}

	// Should have overview + per-command prompts
	if len(listRes.Prompts) < 2 {
		t.Fatalf("expected at least 2 prompts (overview + use-echo), got %d", len(listRes.Prompts))
	}

	var foundOverview, foundUseEcho bool
	for _, p := range listRes.Prompts {
		if p.Name == "myapp-overview" {
			foundOverview = true
		}
		if p.Name == "use-echo" {
			foundUseEcho = true
		}
	}
	if !foundOverview {
		t.Fatalf("myapp-overview prompt not found")
	}
	if !foundUseEcho {
		t.Fatalf("use-echo prompt not found")
	}

	// Get overview prompt
	getRes, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "myapp-overview"})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(getRes.Messages) == 0 {
		t.Fatalf("overview prompt messages empty")
	}

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func TestSchemaResourcesRegistered(t *testing.T) {
	root := &redant.Command{Use: "myapp"}
	root.Children = append(root.Children, &redant.Command{
		Use:     "deploy",
		Short:   "Deploy app.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
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

	// List resources — should include schema resource
	listRes, err := session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}

	var schemaURI string
	for _, r := range listRes.Resources {
		if r.Name == "deploy schema" {
			schemaURI = r.URI
		}
	}
	if schemaURI == "" {
		t.Fatalf("deploy schema resource not found in %d resources", len(listRes.Resources))
	}

	// Read schema resource
	readRes, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: schemaURI})
	if err != nil {
		t.Fatalf("read schema resource: %v", err)
	}
	if len(readRes.Contents) == 0 {
		t.Fatalf("schema contents empty")
	}

	// Parse and validate JSON
	var schema map[string]any
	if err := json.Unmarshal([]byte(readRes.Contents[0].Text), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if name, _ := schema["name"].(string); name != "deploy" {
		t.Fatalf("schema name = %q, want deploy", name)
	}
	if _, ok := schema["inputSchema"]; !ok {
		t.Fatalf("schema missing inputSchema")
	}
	if _, ok := schema["outputSchema"]; !ok {
		t.Fatalf("schema missing outputSchema")
	}

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}

func TestToolAnnotationsWired(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "status",
		Short: "Get status.",
		Metadata: map[string]string{
			"agent.readonly":   "true",
			"agent.idempotent": "true",
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	root.Children = append(root.Children, &redant.Command{
		Use:   "delete",
		Short: "Delete resource.",
		Metadata: map[string]string{
			"agent.destructive": "true",
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	root.Children = append(root.Children, &redant.Command{
		Use:     "plain",
		Short:   "No annotations.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	srv := New(root)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrCh := make(chan error, 1)
	go func() { serverErrCh <- srv.server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	toolMap := make(map[string]*mcp.Tool)
	for _, tool := range res.Tools {
		toolMap[tool.Name] = tool
	}

	// status: readonly + idempotent
	if st, ok := toolMap["status"]; !ok {
		t.Fatalf("status tool not found")
	} else if st.Annotations == nil {
		t.Fatalf("status tool should have annotations")
	} else {
		if !st.Annotations.ReadOnlyHint {
			t.Errorf("status readOnlyHint should be true")
		}
		if !st.Annotations.IdempotentHint {
			t.Errorf("status idempotentHint should be true")
		}
	}

	// delete: destructive
	if del, ok := toolMap["delete"]; !ok {
		t.Fatalf("delete tool not found")
	} else if del.Annotations == nil {
		t.Fatalf("delete tool should have annotations")
	} else {
		if del.Annotations.DestructiveHint == nil || !*del.Annotations.DestructiveHint {
			t.Errorf("delete destructiveHint should be true")
		}
	}

	// plain: no annotations
	if pl, ok := toolMap["plain"]; !ok {
		t.Fatalf("plain tool not found")
	} else if pl.Annotations != nil {
		t.Fatalf("plain tool should NOT have annotations, got %+v", pl.Annotations)
	}

	cancel()
	if err := <-serverErrCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("server run error: %v", err)
	}
}
