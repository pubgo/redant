package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pubgo/redant"
)

func TestCollectToolsCommandToToolDefComprehensive(t *testing.T) {
	var (
		verbose   bool
		parentVal string
		runVal    string
		target    string
	)

	root := &redant.Command{
		Use: "app",
		Options: redant.OptionSet{
			{Flag: "verbose", Value: redant.BoolOf(&verbose), Description: "enable verbose output"},
			{Flag: "internal", Value: redant.StringOf(new(string)), Hidden: true},
		},
	}

	group := &redant.Command{
		Use:   "group",
		Short: "group command",
		Options: redant.OptionSet{
			{Flag: "parent-flag", Value: redant.StringOf(&parentVal), Description: "inherited from parent"},
		},
	}

	run := &redant.Command{
		Use:   "run",
		Short: "run short",
		Long:  "run long description",
		Args: redant.ArgSet{
			{Name: "target", Required: true, Value: redant.StringOf(&target), Description: "target name"},
		},
		Options: redant.OptionSet{
			{Flag: "run-flag", Value: redant.StringOf(&runVal), Description: "child flag"},
			{Flag: "hidden-child", Value: redant.StringOf(new(string)), Hidden: true},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	hidden := &redant.Command{
		Use:    "hidden",
		Hidden: true,
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return nil
		},
	}

	echo := &redant.Command{
		Use:     "echo",
		Short:   "echo short",
		Aliases: []string{"e"},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return nil
		},
	}

	group.Children = append(group.Children, run, hidden)
	root.Children = append(root.Children, group, echo)

	tools := collectTools(root)
	if len(tools) != 2 {
		t.Fatalf("tools len = %d, want 2", len(tools))
	}

	runTool := mustFindToolByName(t, tools, "group.run")
	if runTool.Description != "run short\n\nrun long description" {
		t.Fatalf("run tool description = %q", runTool.Description)
	}

	if got := runTool.PathTokens; !reflect.DeepEqual(got, []string{"group", "run"}) {
		t.Fatalf("run tool path tokens = %#v", got)
	}

	flagsSchema, ok := runTool.InputSchema["properties"].(map[string]any)["flags"].(map[string]any)
	if !ok {
		t.Fatalf("run tool flags schema missing")
	}
	flagProps, ok := flagsSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("run tool flags properties missing")
	}

	for _, want := range []string{"verbose", "parent-flag", "run-flag"} {
		if _, exists := flagProps[want]; !exists {
			t.Fatalf("missing expected flag %q in schema", want)
		}
	}
	for _, notWant := range []string{"internal", "hidden-child", "help", "list-commands", "list-flags", "args"} {
		if _, exists := flagProps[notWant]; exists {
			t.Fatalf("unexpected flag %q in schema", notWant)
		}
	}

	argsSchema, ok := runTool.InputSchema["properties"].(map[string]any)["args"].(map[string]any)
	if !ok {
		t.Fatalf("run tool args schema missing")
	}
	required, ok := argsSchema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "target" {
		t.Fatalf("args required = %#v, want [target]", argsSchema["required"])
	}

	echoTool := mustFindToolByName(t, tools, "echo")
	if got := echoTool.PathTokens; !reflect.DeepEqual(got, []string{"echo"}) {
		t.Fatalf("echo tool path tokens = %#v", got)
	}
	if _, exists := echoTool.InputSchema["properties"].(map[string]any)["args"]; !exists {
		t.Fatalf("echo tool args schema missing")
	}
}

func TestBuildArgvDeterministicAndInheritedFlags(t *testing.T) {
	var (
		verbose   bool
		parentVal string
		runVal    string
		target    string
	)

	root := &redant.Command{
		Use: "app",
		Options: redant.OptionSet{
			{Flag: "verbose", Value: redant.BoolOf(&verbose)},
		},
	}
	group := &redant.Command{
		Use: "group",
		Options: redant.OptionSet{
			{Flag: "parent-flag", Value: redant.StringOf(&parentVal)},
		},
	}
	run := &redant.Command{
		Use: "run",
		Args: redant.ArgSet{
			{Name: "target", Required: true, Value: redant.StringOf(&target)},
		},
		Options: redant.OptionSet{
			{Flag: "run-flag", Value: redant.StringOf(&runVal)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
	group.Children = append(group.Children, run)
	root.Children = append(root.Children, group)

	runTool := mustFindToolByName(t, collectTools(root), "group.run")
	argv, err := buildArgv(runTool, map[string]any{
		"flags": map[string]any{
			"run-flag":    "rv",
			"parent-flag": "pv",
			"verbose":     true,
		},
		"args": map[string]any{"target": "svc"},
	})
	if err != nil {
		t.Fatalf("buildArgv error: %v", err)
	}

	want := []string{"group", "run", "--parent-flag", "pv", "--run-flag", "rv", "--verbose", "svc"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}
}

func TestBuildArgvRejectsUnknownFlag(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "echo",
		Args: redant.ArgSet{
			{Name: "message", Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	echoTool := mustFindToolByName(t, collectTools(root), "echo")
	_, err := buildArgv(echoTool, map[string]any{
		"flags": map[string]any{"not-exists": "x"},
	})
	if err == nil {
		t.Fatalf("expected unknown flag error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCallToolWithInheritedFlags(t *testing.T) {
	var (
		parentVal string
		runVal    string
		target    string
	)

	root := &redant.Command{Use: "app"}
	group := &redant.Command{
		Use: "group",
		Options: redant.OptionSet{
			{Flag: "parent-flag", Value: redant.StringOf(&parentVal)},
		},
	}
	run := &redant.Command{
		Use: "run",
		Args: redant.ArgSet{
			{Name: "target", Required: true, Value: redant.StringOf(&target)},
		},
		Options: redant.OptionSet{
			{Flag: "run-flag", Value: redant.StringOf(&runVal)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = fmt.Fprintf(inv.Stdout, "parent=%s run=%s target=%s", parentVal, runVal, target)
			return nil
		},
	}
	group.Children = append(group.Children, run)
	root.Children = append(root.Children, group)

	s := New(root)
	result, err := s.callTool(context.Background(), toolsCallParams{
		Name: "group.run",
		Arguments: map[string]any{
			"flags": map[string]any{
				"parent-flag": "pv",
				"run-flag":    "rv",
			},
			"args": map[string]any{"target": "svc"},
		},
	})
	if err != nil {
		t.Fatalf("callTool error: %v", err)
	}

	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent missing: %#v", result)
	}
	stdout, _ := structured["stdout"].(string)
	if !strings.Contains(stdout, "parent=pv run=rv target=svc") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestBuildFlagsSchemaComplexTypesAndRequiredRules(t *testing.T) {
	var (
		count  int64
		ratio  float64
		enable bool
		items  []string
		mode   string
		tags   []string
		token  string
		port   string
	)

	schema := buildFlagsSchema(redant.OptionSet{
		{Flag: "count", Value: redant.Int64Of(&count), Required: true},
		{Flag: "ratio", Value: redant.Float64Of(&ratio)},
		{Flag: "enable", Value: redant.BoolOf(&enable)},
		{Flag: "items", Value: redant.StringArrayOf(&items)},
		{Flag: "mode", Value: redant.EnumOf(&mode, "fast", "slow")},
		{Flag: "tags", Value: redant.EnumArrayOf(&tags, "a", "b")},
		{Flag: "token", Value: redant.StringOf(&token), Required: true, Envs: []string{"TOKEN"}},
		{Flag: "port", Value: redant.StringOf(&port), Required: true, Default: "8080"},
	})

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("flags properties missing")
	}

	assertSchemaType(t, props["count"], "integer")
	assertSchemaType(t, props["ratio"], "number")
	assertSchemaType(t, props["enable"], "boolean")
	assertSchemaType(t, props["items"], "array")
	assertSchemaType(t, props["mode"], "string")
	assertSchemaType(t, props["tags"], "array")

	modeSchema := props["mode"].(map[string]any)
	modeEnum, ok := modeSchema["enum"].([]string)
	if !ok || !reflect.DeepEqual(modeEnum, []string{"fast", "slow"}) {
		t.Fatalf("mode enum = %#v", modeSchema["enum"])
	}

	tagsItems := props["tags"].(map[string]any)["items"].(map[string]any)
	tagsEnum, ok := tagsItems["enum"].([]string)
	if !ok || !reflect.DeepEqual(tagsEnum, []string{"a", "b"}) {
		t.Fatalf("tags enum = %#v", tagsItems["enum"])
	}

	tokenSchema := props["token"].(map[string]any)
	xenv, ok := tokenSchema["x-env"].([]string)
	if !ok || !reflect.DeepEqual(xenv, []string{"TOKEN"}) {
		t.Fatalf("token x-env = %#v", tokenSchema["x-env"])
	}

	portSchema := props["port"].(map[string]any)
	if got, _ := portSchema["default"].(string); got != "8080" {
		t.Fatalf("port default = %q", got)
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required missing")
	}
	if !reflect.DeepEqual(required, []string{"count"}) {
		t.Fatalf("required = %#v, want [count]", required)
	}
}

func TestBuildArgsSchemaUnnamedAndDefault(t *testing.T) {
	var (
		first string
		mode  string
	)

	schema := buildArgsSchema(redant.ArgSet{
		{Name: "", Required: true, Value: redant.StringOf(&first), Description: "first positional"},
		{Name: "mode", Required: true, Default: "auto", Value: redant.StringOf(&mode)},
	})

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("args properties missing")
	}

	arg1, ok := props["arg1"].(map[string]any)
	if !ok {
		t.Fatalf("arg1 schema missing")
	}
	if got, _ := arg1["description"].(string); got != "first positional" {
		t.Fatalf("arg1 description = %q", got)
	}

	modeSchema, ok := props["mode"].(map[string]any)
	if !ok {
		t.Fatalf("mode schema missing")
	}
	if got, _ := modeSchema["default"].(string); got != "auto" {
		t.Fatalf("mode default = %q", got)
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required missing")
	}
	if !reflect.DeepEqual(required, []string{"arg1"}) {
		t.Fatalf("required = %#v, want [arg1]", required)
	}
}

func TestBuildArgvArrayFlagAndNoArgSetArgs(t *testing.T) {
	var tags []string

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "scan",
		Options: redant.OptionSet{
			{Flag: "tags", Value: redant.StringArrayOf(&tags)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	tool := mustFindToolByName(t, collectTools(root), "scan")
	argv, err := buildArgv(tool, map[string]any{
		"flags": map[string]any{"tags": []any{"x", "y"}},
		"args":  []any{"p1", "p2"},
	})
	if err != nil {
		t.Fatalf("buildArgv error: %v", err)
	}

	want := []string{"scan", "--tags", "x", "--tags", "y", "p1", "p2"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}
}

func TestBuildArgvMissingRequiredArg(t *testing.T) {
	var target string

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "run",
		Args: redant.ArgSet{
			{Name: "target", Required: true, Value: redant.StringOf(&target)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	tool := mustFindToolByName(t, collectTools(root), "run")
	_, err := buildArgv(tool, map[string]any{
		"args": map[string]any{},
	})
	if err == nil {
		t.Fatalf("expected missing required arg error, got nil")
	}
	if !strings.Contains(err.Error(), "missing required arg") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildFlagsSchemaStructGeneric(t *testing.T) {
	type payload struct {
		Name string `json:"name" yaml:"name"`
		Port int    `json:"port" yaml:"port"`
	}

	var cfg payload
	schema := buildFlagsSchema(redant.OptionSet{
		{Flag: "config", Value: &redant.Struct[payload]{Value: cfg}, Description: "service config"},
	})

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("flags properties missing")
	}

	configSchema, ok := props["config"].(map[string]any)
	if !ok {
		t.Fatalf("config schema missing")
	}
	if got, _ := configSchema["type"].(string); got != "object" {
		t.Fatalf("config type = %q, want object", got)
	}
	if ap, _ := configSchema["additionalProperties"].(bool); !ap {
		t.Fatalf("config additionalProperties = %#v, want true", configSchema["additionalProperties"])
	}
	if xvt, _ := configSchema["x-redant-value-type"].(string); !strings.HasPrefix(xvt, "struct[") {
		t.Fatalf("x-redant-value-type = %q, want prefix struct[", xvt)
	}
}

func TestBuildArgvSupportsStructFlagAndArg(t *testing.T) {
	type payload struct {
		Name string `json:"name" yaml:"name"`
		Port int    `json:"port" yaml:"port"`
	}

	var (
		argCfg  payload
		flagCfg payload
	)

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "apply",
		Args: redant.ArgSet{
			{Name: "config", Required: true, Value: &redant.Struct[payload]{Value: argCfg}},
		},
		Options: redant.OptionSet{
			{Flag: "meta", Value: &redant.Struct[payload]{Value: flagCfg}},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	tool := mustFindToolByName(t, collectTools(root), "apply")
	argv, err := buildArgv(tool, map[string]any{
		"flags": map[string]any{
			"meta": map[string]any{"name": "svc", "port": 9000},
		},
		"args": map[string]any{
			"config": map[string]any{"name": "api", "port": 8080},
		},
	})
	if err != nil {
		t.Fatalf("buildArgv error: %v", err)
	}

	if len(argv) != 4 {
		t.Fatalf("argv len = %d, want 4, argv=%#v", len(argv), argv)
	}
	if argv[0] != "apply" || argv[1] != "--meta" {
		t.Fatalf("argv prefix = %#v, want [apply --meta ...]", argv[:2])
	}

	assertJSONStringObjectContains(t, argv[2], map[string]any{"name": "svc", "port": float64(9000)})
	assertJSONStringObjectContains(t, argv[3], map[string]any{"name": "api", "port": float64(8080)})
}

func assertJSONStringObjectContains(t *testing.T, raw string, want map[string]any) {
	t.Helper()

	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("expected JSON object string, got %q (err: %v)", raw, err)
	}

	for k, v := range want {
		gv, ok := got[k]
		if !ok {
			t.Fatalf("json key %q missing in %q", k, raw)
		}
		if !reflect.DeepEqual(gv, v) {
			t.Fatalf("json key %q value = %#v, want %#v", k, gv, v)
		}
	}
}

func assertSchemaType(t *testing.T, raw any, want string) {
	t.Helper()
	schema, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("schema is not object: %#v", raw)
	}
	if got, _ := schema["type"].(string); got != want {
		t.Fatalf("schema type = %q, want %q", got, want)
	}
}

func mustFindToolByName(t *testing.T, tools []toolDef, name string) toolDef {
	t.Helper()
	for _, td := range tools {
		if td.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found in %#v", name, tools)
	return toolDef{}
}

func TestValueTypeToSchemaExtendedTypes(t *testing.T) {
	tests := []struct {
		typ    string
		field  string
		expect string
	}{
		{"duration", "format", "duration"},
		{"url", "format", "uri"},
		{"regexp", "format", "regex"},
		{"host:port", "pattern", "^[^:]+:\\d+$"},
		{"json", "contentMediaType", "application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			s := valueTypeToSchema(tt.typ)
			got, _ := s[tt.field].(string)
			if got != tt.expect {
				t.Fatalf("valueTypeToSchema(%q)[%q] = %q, want %q", tt.typ, tt.field, got, tt.expect)
			}
		})
	}
}

func TestCommandAgentHints(t *testing.T) {
	cmd := &redant.Command{
		Use:   "rm",
		Short: "Remove file.",
		Metadata: map[string]string{
			"agent.destructive":           "true",
			"agent.requires-confirmation": "true",
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	hints := commandAgentHints(cmd)
	if !strings.Contains(hints, "Destructive") {
		t.Fatalf("expected Destructive hint, got: %s", hints)
	}
	if !strings.Contains(hints, "Requires confirmation") {
		t.Fatalf("expected Requires confirmation hint, got: %s", hints)
	}
}

func TestCommandAgentHintsEmpty(t *testing.T) {
	cmd := &redant.Command{
		Use:     "ls",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
	if got := commandAgentHints(cmd); got != "" {
		t.Fatalf("expected empty hints, got: %q", got)
	}
}

func TestAgentHintsInToolDescription(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "deploy",
		Short: "Deploy the app.",
		Metadata: map[string]string{
			"agent.idempotent": "true",
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	tools := collectTools(root)
	tool := mustFindToolByName(t, tools, "deploy")
	if !strings.Contains(tool.Description, "Idempotent") {
		t.Fatalf("tool description should contain agent hint, got: %q", tool.Description)
	}
}

func TestOutputSchemaWithTypedResponse(t *testing.T) {
	type Result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "status",
		ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (Result, error) {
			return Result{}, nil
		}),
	})

	tools := collectTools(root)
	tool := mustFindToolByName(t, tools, "status")

	props, _ := tool.OutputSchema["properties"].(map[string]any)
	respProp, ok := props["response"]
	if !ok {
		t.Fatalf("output schema should have response property")
	}

	respMap, _ := respProp.(map[string]any)

	// Should have x-redant-type
	xType, _ := respMap["x-redant-type"].(string)
	if !strings.Contains(xType, "Result") {
		t.Fatalf("x-redant-type = %q, want to contain Result", xType)
	}

	// Should have properties from struct reflection
	respProps, _ := respMap["properties"].(map[string]any)
	if _, ok := respProps["ok"]; !ok {
		t.Fatalf("response schema should have 'ok' property")
	}
	if _, ok := respProps["message"]; !ok {
		t.Fatalf("response schema should have 'message' property")
	}
}

func TestOutputSchemaStreamTyped(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "logs",
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			return out.Send("line")
		}),
	})

	tools := collectTools(root)
	tool := mustFindToolByName(t, tools, "logs")

	props, _ := tool.OutputSchema["properties"].(map[string]any)
	respProp, ok := props["response"]
	if !ok {
		t.Fatalf("output schema should have response property")
	}

	respMap, _ := respProp.(map[string]any)
	if got, _ := respMap["type"].(string); got != "array" {
		t.Fatalf("stream response type = %q, want array", got)
	}
}

func TestBuildToolAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		meta     map[string]string
		wantNil  bool
		readonly bool
		idemp    bool
		destr    *bool
		open     *bool
	}{
		{"nil_metadata", nil, true, false, false, nil, nil},
		{"readonly+idempotent", map[string]string{
			"agent.readonly":   "true",
			"agent.idempotent": "true",
		}, false, true, true, nil, nil},
		{"destructive_true", map[string]string{
			"agent.destructive": "true",
		}, false, false, false, ptrBool(true), nil},
		{"destructive_false", map[string]string{
			"agent.destructive": "false",
		}, false, false, false, ptrBool(false), nil},
		{"open_world", map[string]string{
			"agent.open-world": "true",
		}, false, false, false, nil, ptrBool(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &redant.Command{Use: "x", Metadata: tt.meta}
			got := buildToolAnnotations(cmd)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil annotations, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil annotations")
			}
			if got.ReadOnly != tt.readonly {
				t.Fatalf("ReadOnly = %v, want %v", got.ReadOnly, tt.readonly)
			}
			if got.Idempotent != tt.idemp {
				t.Fatalf("Idempotent = %v, want %v", got.Idempotent, tt.idemp)
			}
			if !equalBoolPtr(got.Destructive, tt.destr) {
				t.Fatalf("Destructive = %v, want %v", got.Destructive, tt.destr)
			}
			if !equalBoolPtr(got.OpenWorld, tt.open) {
				t.Fatalf("OpenWorld = %v, want %v", got.OpenWorld, tt.open)
			}
		})
	}
}

func ptrBool(v bool) *bool { return &v }
func equalBoolPtr(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func TestArgsSchemaIncludesArgModes(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use: "greet",
		Args: redant.ArgSet{
			{Name: "name", Required: true, Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})

	tools := collectTools(root)
	tool := mustFindToolByName(t, tools, "greet")

	argsSection, ok := tool.InputSchema["properties"].(map[string]any)["args"].(map[string]any)
	if !ok {
		t.Fatalf("args section missing from input schema")
	}

	modes, ok := argsSection["x-redant-arg-modes"]
	if !ok {
		t.Fatalf("x-redant-arg-modes not set in args schema")
	}

	modeSlice, ok := modes.([]string)
	if !ok {
		t.Fatalf("x-redant-arg-modes is not []string: %T", modes)
	}

	expected := []string{"positional", "query", "form", "json"}
	if !reflect.DeepEqual(modeSlice, expected) {
		t.Fatalf("x-redant-arg-modes = %v, want %v", modeSlice, expected)
	}
}

func TestParseToolTimeout(t *testing.T) {
	tests := []struct {
		name   string
		meta   map[string]string
		wantMs int64 // 0 = no timeout
	}{
		{"no_metadata", nil, 0},
		{"no_timeout_key", map[string]string{"agent.readonly": "true"}, 0},
		{"valid_30s", map[string]string{"agent.timeout": "30s"}, 30000},
		{"valid_2m", map[string]string{"agent.timeout": "2m"}, 120000},
		{"invalid", map[string]string{"agent.timeout": "not-a-duration"}, 0},
		{"negative", map[string]string{"agent.timeout": "-5s"}, 0},
		{"zero", map[string]string{"agent.timeout": "0s"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &redant.Command{Use: "x", Metadata: tt.meta}
			got := parseToolTimeout(cmd)
			gotMs := got.Milliseconds()
			if gotMs != tt.wantMs {
				t.Fatalf("parseToolTimeout = %dms, want %dms", gotMs, tt.wantMs)
			}
		})
	}
}

func TestToolTimeoutInCallTool(t *testing.T) {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children, &redant.Command{
		Use:   "slow",
		Short: "A slow command.",
		Metadata: map[string]string{
			"agent.timeout": "100ms",
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			// Verify ctx has a deadline
			deadline, ok := ctx.Deadline()
			if !ok {
				return fmt.Errorf("expected deadline on context")
			}
			_ = deadline
			return nil
		},
	})

	srv := New(root)
	result, err := srv.callTool(context.Background(), toolsCallParams{
		Name:      "slow",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("callTool error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}
}
