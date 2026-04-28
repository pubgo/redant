package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pubgo/redant"
)

type toolDef struct {
	Name           string
	Description    string
	PathTokens     []string
	Command        *redant.Command
	Options        redant.OptionSet
	InputSchema    map[string]any
	OutputSchema   map[string]any
	SupportsStream bool
	ResponseType   *redant.ResponseTypeInfo
	Annotations    *toolAnnotations
}

// toolAnnotations holds MCP-standard tool behavior hints derived from Command.Metadata.
type toolAnnotations struct {
	ReadOnly    bool
	Destructive *bool // nil = unset (MCP default: true)
	Idempotent  bool
	OpenWorld   *bool // nil = unset (MCP default: true)
}

func collectTools(root *redant.Command) []toolDef {
	if root == nil {
		return nil
	}

	var tools []toolDef
	var walk func(cmd *redant.Command, path []string, inheritedOptions redant.OptionSet)
	walk = func(cmd *redant.Command, path []string, inheritedOptions redant.OptionSet) {
		if cmd == nil || cmd.Hidden {
			return
		}

		effectiveOptions := make(redant.OptionSet, 0, len(inheritedOptions)+len(cmd.Options))
		effectiveOptions = append(effectiveOptions, inheritedOptions...)
		effectiveOptions = append(effectiveOptions, cmd.Options...)

		if cmd.Handler != nil || cmd.ResponseHandler != nil || cmd.ResponseStreamHandler != nil {
			var respType *redant.ResponseTypeInfo
			if cmd.ResponseHandler != nil {
				ti := cmd.ResponseHandler.TypeInfo()
				respType = &ti
			} else if cmd.ResponseStreamHandler != nil {
				ti := cmd.ResponseStreamHandler.TypeInfo()
				respType = &ti
			}
			tools = append(tools, toolDef{
				Name:           strings.Join(path, "."),
				Description:    commandDescription(cmd),
				PathTokens:     append([]string(nil), path...),
				Command:        cmd,
				Options:        append(redant.OptionSet(nil), effectiveOptions...),
				InputSchema:    buildInputSchema(cmd.Args, effectiveOptions),
				OutputSchema:   buildOutputSchema(respType, cmd.ResponseStreamHandler != nil),
				SupportsStream: cmd.ResponseStreamHandler != nil,
				ResponseType:   respType,
				Annotations:    buildToolAnnotations(cmd),
			})
		}

		for _, child := range cmd.Children {
			walk(child, append(path, child.Name()), effectiveOptions)
		}
	}

	for _, child := range root.Children {
		walk(child, []string{child.Name()}, root.Options)
	}

	return tools
}

// buildToolAnnotations extracts MCP-standard ToolAnnotations from Command.Metadata.
func buildToolAnnotations(cmd *redant.Command) *toolAnnotations {
	if cmd == nil || len(cmd.Metadata) == 0 {
		return nil
	}

	a := &toolAnnotations{}
	any := false

	if v := cmd.Meta("agent.readonly"); v == "true" {
		a.ReadOnly = true
		any = true
	}
	if v := cmd.Meta("agent.idempotent"); v == "true" {
		a.Idempotent = true
		any = true
	}
	if v := cmd.Meta("agent.destructive"); v != "" {
		b := v == "true"
		a.Destructive = &b
		any = true
	}
	if v := cmd.Meta("agent.open-world"); v != "" {
		b := v == "true"
		a.OpenWorld = &b
		any = true
	}

	if !any {
		return nil
	}
	return a
}

func commandDescription(cmd *redant.Command) string {
	short := strings.TrimSpace(cmd.Short)
	long := strings.TrimSpace(cmd.Long)
	var base string
	switch {
	case short != "" && long != "":
		base = short + "\n\n" + long
	case short != "":
		base = short
	case long != "":
		base = long
	}
	if hints := commandAgentHints(cmd); hints != "" {
		base += "\n" + hints
	}
	return base
}

func buildInputSchema(args redant.ArgSet, options redant.OptionSet) map[string]any {
	argsSchema := buildArgsSchema(args)
	flagsSchema := buildFlagsSchema(options)

	properties := map[string]any{"flags": flagsSchema}

	if len(args) > 0 {
		argsSchema["x-redant-arg-modes"] = []string{"positional", "query", "form", "json"}
		properties["args"] = argsSchema
	} else {
		properties["args"] = map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
			},
			"description": "Positional args array for commands without ArgSet definition.",
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	return schema
}

func buildOutputSchema(respType *redant.ResponseTypeInfo, isStream bool) map[string]any {
	props := map[string]any{
		"ok": map[string]any{
			"type": "boolean",
		},
		"stdout": map[string]any{
			"type": "string",
		},
		"stderr": map[string]any{
			"type": "string",
		},
		"error": map[string]any{
			"type": "string",
		},
		"combined": map[string]any{
			"type": "string",
		},
	}
	required := []string{"ok", "stdout", "stderr", "error", "combined"}

	if respType != nil && respType.TypeName != "" {
		respSchema := map[string]any{
			"description":   "typed response payload (" + respType.TypeName + ")",
			"x-redant-type": respType.TypeName,
		}
		if len(respType.Schema) > 0 {
			// Merge generated schema into respSchema.
			for k, v := range respType.Schema {
				respSchema[k] = v
			}
		}
		if isStream {
			respSchema = map[string]any{
				"type":          "array",
				"items":         respType.Schema,
				"description":   "typed response payload (" + respType.TypeName + ")",
				"x-redant-type": respType.TypeName,
			}
		}
		props["response"] = respSchema
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
		"required":             required,
	}
}

func buildArgsSchema(args redant.ArgSet) map[string]any {
	props := map[string]any{}
	var required []string

	for i, arg := range args {
		name := arg.Name
		if name == "" {
			name = fmt.Sprintf("arg%d", i+1)
		}

		argSchema := valueTypeToSchema(typeOfValue(arg.Value))
		if arg.Description != "" {
			argSchema["description"] = arg.Description
		}
		if arg.Default != "" {
			argSchema["default"] = arg.Default
		}

		props[name] = argSchema
		if arg.Required && arg.Default == "" {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func buildFlagsSchema(opts redant.OptionSet) map[string]any {
	props := map[string]any{}
	var required []string

	for _, opt := range opts {
		if opt.Flag == "" || opt.Hidden || isSystemFlag(opt.Flag) {
			continue
		}

		flagSchema := valueTypeToSchema(opt.Type())
		if opt.Description != "" {
			flagSchema["description"] = opt.Description
		}
		if opt.Default != "" {
			flagSchema["default"] = opt.Default
		}
		if len(opt.Envs) > 0 {
			flagSchema["x-env"] = opt.Envs
		}

		props[opt.Flag] = flagSchema
		if opt.Required && opt.Default == "" && len(opt.Envs) == 0 {
			required = append(required, opt.Flag)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func typeOfValue(v any) string {
	if v == nil {
		return "string"
	}
	t, ok := v.(interface{ Type() string })
	if !ok {
		return "string"
	}
	return t.Type()
}

func valueTypeToSchema(typ string) map[string]any {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return map[string]any{"type": "string"}
	}

	if strings.HasPrefix(typ, "enum[") && strings.HasSuffix(typ, "]") {
		choices := strings.TrimSuffix(strings.TrimPrefix(typ, "enum["), "]")
		return map[string]any{
			"type": "string",
			"enum": splitEnumChoices(choices),
		}
	}

	if strings.HasPrefix(typ, "enum-array[") && strings.HasSuffix(typ, "]") {
		choices := strings.TrimSuffix(strings.TrimPrefix(typ, "enum-array["), "]")
		return map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
				"enum": splitEnumChoices(choices),
			},
		}
	}

	if strings.HasPrefix(typ, "struct[") && strings.HasSuffix(typ, "]") {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"x-redant-value-type":  typ,
		}
	}

	switch typ {
	case "int", "int64":
		return map[string]any{"type": "integer"}
	case "float", "float64":
		return map[string]any{"type": "number"}
	case "bool":
		return map[string]any{"type": "boolean"}
	case "string-array":
		return map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
			},
		}
	case "duration":
		return map[string]any{
			"type":        "string",
			"format":      "duration",
			"description": "Go duration string (e.g. 30s, 5m, 1h30m)",
		}
	case "url":
		return map[string]any{
			"type":   "string",
			"format": "uri",
		}
	case "regexp":
		return map[string]any{
			"type":   "string",
			"format": "regex",
		}
	case "host:port":
		return map[string]any{
			"type":    "string",
			"pattern": "^[^:]+:\\d+$",
			"examples": []string{
				"localhost:8080",
				"0.0.0.0:443",
			},
		}
	case "json":
		return map[string]any{
			"type":             "string",
			"contentMediaType": "application/json",
		}
	default:
		return map[string]any{"type": "string"}
	}
}

func splitEnumChoices(raw string) []string {
	parts := strings.Split(raw, "\\|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) callTool(ctx context.Context, params toolsCallParams) (map[string]any, error) {
	if params.Name == "" {
		return nil, errorsNew("missing tool name")
	}

	tool, err := s.findTool(params.Name)
	if err != nil {
		return nil, err
	}

	argv, err := buildArgv(tool, params.Arguments)
	if err != nil {
		return nil, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	inv := s.root.Invoke(argv...)
	inv.Stdout = &stdout
	inv.Stderr = &stderr
	inv.Stdin = bytes.NewReader(nil)

	// For commands with typed response, use RunCallback to collect structured data.
	if tool.ResponseType != nil {
		var responses []any
		runErr := redant.RunCallback[any](inv.WithContext(ctx), func(v any) error {
			responses = append(responses, v)
			return nil
		})
		result := buildToolResult(stdout.String(), stderr.String(), runErr)
		if structured, ok := result["structuredContent"].(map[string]any); ok && len(responses) > 0 {
			if tool.SupportsStream {
				structured["response"] = responses
			} else {
				structured["response"] = responses[0]
			}
		}
		return result, nil
	}

	runErr := inv.WithContext(ctx).Run()
	return buildToolResult(stdout.String(), stderr.String(), runErr), nil
}

func (s *Server) findTool(name string) (toolDef, error) {
	for _, t := range s.tools {
		if t.Name == name {
			return t, nil
		}
	}
	return toolDef{}, fmt.Errorf("tool %q not found", name)
}

func buildArgv(tool toolDef, input map[string]any) ([]string, error) {
	argv := append([]string(nil), tool.PathTokens...)

	flagsInput := map[string]any{}
	if raw, ok := input["flags"]; ok {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, errorsNew("arguments.flags must be an object")
		}
		flagsInput = m
	}

	flagByName := map[string]redant.Option{}
	for _, opt := range tool.Options {
		if opt.Flag == "" || opt.Hidden || isSystemFlag(opt.Flag) {
			continue
		}
		flagByName[opt.Flag] = opt
	}

	keys := make([]string, 0, len(flagsInput))
	for k := range flagsInput {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := flagsInput[k]
		opt, ok := flagByName[k]
		if !ok {
			return nil, fmt.Errorf("unknown flag %q for tool %q", k, tool.Name)
		}

		flagTokens, err := serializeFlag(opt, v)
		if err != nil {
			return nil, fmt.Errorf("flag %q: %w", k, err)
		}
		argv = append(argv, flagTokens...)
	}

	argsTokens, err := serializeArgs(tool.Command.Args, input["args"])
	if err != nil {
		return nil, err
	}
	argv = append(argv, argsTokens...)

	return argv, nil
}

func serializeArgs(def redant.ArgSet, raw any) ([]string, error) {
	if len(def) == 0 {
		if raw == nil {
			return nil, nil
		}
		vals, ok := raw.([]any)
		if !ok {
			return nil, errorsNew("arguments.args must be an array for commands without ArgSet")
		}
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			out = append(out, toString(v))
		}
		return out, nil
	}

	if raw == nil {
		return nil, nil
	}
	argMap, ok := raw.(map[string]any)
	if !ok {
		return nil, errorsNew("arguments.args must be an object")
	}

	out := make([]string, 0, len(def))
	for i, arg := range def {
		name := arg.Name
		if name == "" {
			name = fmt.Sprintf("arg%d", i+1)
		}

		v, ok := argMap[name]
		if !ok {
			if arg.Required && arg.Default == "" {
				return nil, fmt.Errorf("missing required arg %q", name)
			}
			continue
		}

		encoded, err := serializeValueByType(typeOfValue(arg.Value), v)
		if err != nil {
			return nil, fmt.Errorf("arg %q: %w", name, err)
		}
		out = append(out, encoded)
	}

	return out, nil
}

func serializeFlag(opt redant.Option, v any) ([]string, error) {
	flag := "--" + opt.Flag
	schema := valueTypeToSchema(opt.Type())
	typ, _ := schema["type"].(string)

	switch typ {
	case "boolean":
		bv, ok := v.(bool)
		if !ok {
			return nil, errorsNew("expected boolean")
		}
		if bv {
			return []string{flag}, nil
		}
		return []string{flag + "=false"}, nil

	case "array":
		arr, ok := v.([]any)
		if !ok {
			return nil, errorsNew("expected array")
		}
		out := make([]string, 0, len(arr)*2)
		for _, item := range arr {
			out = append(out, flag, toString(item))
		}
		return out, nil

	case "object":
		encoded, err := serializeObjectLike(v)
		if err != nil {
			return nil, err
		}
		return []string{flag, encoded}, nil

	default:
		encoded, err := serializeValueByType(opt.Type(), v)
		if err != nil {
			return nil, err
		}
		return []string{flag, encoded}, nil
	}
}

func serializeValueByType(valueType string, v any) (string, error) {
	typeSchema := valueTypeToSchema(valueType)
	typeName, _ := typeSchema["type"].(string)

	switch typeName {
	case "object":
		return serializeObjectLike(v)
	default:
		return toString(v), nil
	}
}

func serializeObjectLike(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}

	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("expected object-compatible value: %w", err)
	}

	return string(b), nil
}

func toString(v any) string {
	switch vv := v.(type) {
	case nil:
		return ""
	case string:
		return vv
	case bool:
		return strconv.FormatBool(vv)
	case float64:
		if vv == float64(int64(vv)) {
			return strconv.FormatInt(int64(vv), 10)
		}
		return strconv.FormatFloat(vv, 'f', -1, 64)
	case json.Number:
		return vv.String()
	default:
		return fmt.Sprintf("%v", vv)
	}
}

func errorsNew(msg string) error {
	return errors.New(msg)
}

func buildToolResult(stdout, stderr string, runErr error) map[string]any {
	var out bytes.Buffer
	errText := ""
	if stdout != "" {
		_, _ = out.WriteString(stdout)
	}
	if stderr != "" {
		if out.Len() > 0 {
			_, _ = out.WriteString("\n")
		}
		_, _ = out.WriteString("stderr:\n")
		_, _ = out.WriteString(stderr)
	}
	if runErr != nil {
		errText = runErr.Error()
		if out.Len() > 0 {
			_, _ = out.WriteString("\n")
		}
		_, _ = out.WriteString("error:\n")
		_, _ = out.WriteString(errText)
	}
	if out.Len() == 0 {
		_, _ = out.WriteString("ok")
	}

	combined := out.String()
	structured := map[string]any{
		"ok":       runErr == nil,
		"stdout":   stdout,
		"stderr":   stderr,
		"error":    errText,
		"combined": combined,
	}

	return map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": combined,
		}},
		"isError":           runErr != nil,
		"structuredContent": structured,
	}
}

func isSystemFlag(flag string) bool {
	switch flag {
	case "help", "list-commands", "list-flags", "args":
		return true
	default:
		return false
	}
}
