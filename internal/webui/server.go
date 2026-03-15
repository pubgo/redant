package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/pflag"

	"github.com/pubgo/redant"
)

type ArgMeta struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	EnumValues  []string `json:"enumValues,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
}

type FlagMeta struct {
	Name        string   `json:"name"`
	Shorthand   string   `json:"shorthand,omitempty"`
	Envs        []string `json:"envs,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	EnumValues  []string `json:"enumValues,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
}

type CommandMeta struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Use         string     `json:"use"`
	Aliases     []string   `json:"aliases,omitempty"`
	Short       string     `json:"short,omitempty"`
	Long        string     `json:"long,omitempty"`
	Deprecated  string     `json:"deprecated,omitempty"`
	RawArgs     bool       `json:"rawArgs"`
	Path        []string   `json:"path"`
	Description string     `json:"description,omitempty"`
	Flags       []FlagMeta `json:"flags"`
	Args        []ArgMeta  `json:"args"`
}

type RunRequest struct {
	Command string         `json:"command"`
	Flags   map[string]any `json:"flags,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
	RawArgs []string       `json:"rawArgs,omitempty"`
}

type RunResponse struct {
	OK         bool     `json:"ok"`
	Command    string   `json:"command"`
	Program    string   `json:"program,omitempty"`
	Argv       []string `json:"argv,omitempty"`
	Invocation string   `json:"invocation"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	Error      string   `json:"error"`
	Combined   string   `json:"combined"`
}

type commandListResponse struct {
	Commands []CommandMeta `json:"commands"`
}

type App struct {
	root     *redant.Command
	commands []CommandMeta
	byID     map[string]CommandMeta
	mu       sync.Mutex
}

func New(root *redant.Command) *App {
	cmds := collectCommands(root)
	byID := make(map[string]CommandMeta, len(cmds))
	for _, c := range cmds {
		byID[c.ID] = c
	}
	return &App{root: root, commands: cmds, byID: byID}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/commands", a.handleCommands)
	mux.HandleFunc("/api/run", a.handleRun)
	return mux
}

func (a *App) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (a *App) handleCommands(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(commandListResponse{Commands: a.commands})
}

func (a *App) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	meta, ok := a.byID[strings.TrimSpace(req.Command)]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown command: %q", req.Command), http.StatusBadRequest)
		return
	}

	argv, program, invocation, err := buildInvocation(meta, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	a.mu.Lock()
	runErr := func() error {
		root := cloneCommandTree(a.root)
		inv := root.Invoke(argv...)
		inv.Stdout = &stdout
		inv.Stderr = &stderr
		inv.Stdin = bytes.NewReader(nil)
		return inv.WithContext(r.Context()).Run()
	}()
	a.mu.Unlock()

	resp := RunResponse{
		OK:         runErr == nil,
		Command:    meta.ID,
		Program:    program,
		Argv:       append([]string(nil), argv...),
		Invocation: invocation,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Combined:   combineOutput(stdout.String(), stderr.String(), runErr),
	}
	if runErr != nil {
		resp.Error = runErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func buildInvocation(meta CommandMeta, req RunRequest) ([]string, string, string, error) {
	argv := append([]string(nil), meta.Path...)

	for _, flag := range meta.Flags {
		v, ok := req.Flags[flag.Name]
		if !ok {
			continue
		}
		tokens, err := serializeFlag(flag, v)
		if err != nil {
			return nil, "", "", fmt.Errorf("flag %q: %w", flag.Name, err)
		}
		argv = append(argv, tokens...)
	}

	if len(meta.Args) > 0 {
		for i, arg := range meta.Args {
			v, ok := lookupArgValue(req.Args, arg.Name)
			if !ok && i < len(req.RawArgs) {
				v = req.RawArgs[i]
				ok = true
			}
			if !ok {
				if arg.Required && arg.Default == "" {
					return nil, "", "", fmt.Errorf("missing required arg %q", arg.Name)
				}
				continue
			}
			val, err := serializeValueByType(arg.Type, v)
			if err != nil {
				return nil, "", "", fmt.Errorf("arg %q: %w", arg.Name, err)
			}
			argv = append(argv, val)
		}
	} else if len(req.RawArgs) > 0 {
		argv = append(argv, req.RawArgs...)
	}

	prog := filepath.Base(os.Args[0])
	invocation := prog
	for _, token := range argv {
		invocation += " " + shellQuote(token)
	}

	return argv, prog, invocation, nil
}

func lookupArgValue(args map[string]any, name string) (any, bool) {
	if len(args) == 0 {
		return nil, false
	}

	if v, ok := args[name]; ok {
		return v, true
	}

	trimmed := strings.TrimSpace(name)
	if trimmed != name {
		if v, ok := args[trimmed]; ok {
			return v, true
		}
	}

	for k, v := range args {
		if strings.TrimSpace(k) == trimmed {
			return v, true
		}
	}

	return nil, false
}

func serializeFlag(flag FlagMeta, raw any) ([]string, error) {
	name := "--" + flag.Name
	if flag.Type == "bool" {
		b, ok := parseBool(raw)
		if !ok {
			return nil, fmt.Errorf("expected boolean")
		}
		if b {
			return []string{name}, nil
		}
		return []string{name + "=false"}, nil
	}

	if isArrayType(flag.Type) {
		vals, err := toStringSlice(raw)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(vals)*2)
		for _, v := range vals {
			out = append(out, name, v)
		}
		return out, nil
	}

	value, err := serializeValueByType(flag.Type, raw)
	if err != nil {
		return nil, err
	}
	return []string{name, value}, nil
}

func serializeValueByType(typ string, raw any) (string, error) {
	if strings.HasPrefix(typ, "struct[") {
		if s, ok := raw.(string); ok {
			return s, nil
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return "", fmt.Errorf("expected object-compatible value: %w", err)
		}
		return string(b), nil
	}
	return toString(raw), nil
}

func toString(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func parseBool(raw any) (bool, bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		return b, err == nil
	default:
		return false, false
	}
}

func toStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, toString(item))
		}
		return out, nil
	case string:
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, p)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array")
	}
}

func isArrayType(typ string) bool {
	return typ == "string-array" || strings.HasPrefix(typ, "enum-array[")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	needQuote := false
	for _, ch := range s {
		if !(ch == '_' || ch == '-' || ch == '.' || ch == '/' || ch == ':' || ch == '=' || (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
			needQuote = true
			break
		}
	}
	if !needQuote {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func combineOutput(stdout, stderr string, runErr error) string {
	var out bytes.Buffer
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
		if out.Len() > 0 {
			_, _ = out.WriteString("\n")
		}
		_, _ = out.WriteString("error:\n")
		_, _ = out.WriteString(runErr.Error())
	}
	if out.Len() == 0 {
		return "ok"
	}
	return out.String()
}

func cloneCommandTree(cmd *redant.Command) *redant.Command {
	if cmd == nil {
		return nil
	}
	cpy := *cmd
	cpy.Options = append(redant.OptionSet(nil), cmd.Options...)
	cpy.Args = append(redant.ArgSet(nil), cmd.Args...)
	cpy.Children = make([]*redant.Command, 0, len(cmd.Children))
	for _, child := range cmd.Children {
		cpy.Children = append(cpy.Children, cloneCommandTree(child))
	}
	return &cpy
}

func collectCommands(root *redant.Command) []CommandMeta {
	if root == nil {
		return nil
	}

	var out []CommandMeta
	var walk func(cmd *redant.Command, path []string, inherited redant.OptionSet)
	walk = func(cmd *redant.Command, path []string, inherited redant.OptionSet) {
		if cmd == nil || cmd.Hidden {
			return
		}

		effective := make(redant.OptionSet, 0, len(inherited)+len(cmd.Options))
		effective = append(effective, inherited...)
		effective = append(effective, cmd.Options...)

		if cmd.Handler != nil && len(path) > 0 && path[0] != "web" {
			out = append(out, toCommandMeta(cmd, path, effective))
		}

		for _, child := range cmd.Children {
			walk(child, append(path, child.Name()), effective)
		}
	}

	for _, child := range root.Children {
		walk(child, []string{child.Name()}, root.Options)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func toCommandMeta(cmd *redant.Command, path []string, opts redant.OptionSet) CommandMeta {
	return CommandMeta{
		ID:          strings.Join(path, " "),
		Name:        cmd.Name(),
		Use:         cmd.Use,
		Aliases:     append([]string(nil), cmd.Aliases...),
		Short:       strings.TrimSpace(cmd.Short),
		Long:        strings.TrimSpace(cmd.Long),
		Deprecated:  strings.TrimSpace(cmd.Deprecated),
		RawArgs:     cmd.RawArgs,
		Path:        append([]string(nil), path...),
		Description: commandDescription(cmd),
		Flags:       toFlagMeta(opts),
		Args:        toArgMeta(cmd.Args),
	}
}

func toFlagMeta(opts redant.OptionSet) []FlagMeta {
	byName := map[string]redant.Option{}
	for _, opt := range opts {
		if opt.Hidden || opt.Flag == "" || isSystemFlag(opt.Flag) {
			continue
		}
		byName[opt.Flag] = opt
	}

	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]FlagMeta, 0, len(names))
	for _, n := range names {
		opt := byName[n]
		out = append(out, FlagMeta{
			Name:        opt.Flag,
			Shorthand:   opt.Shorthand,
			Envs:        append([]string(nil), opt.Envs...),
			Description: strings.TrimSpace(opt.Description),
			Type:        opt.Type(),
			EnumValues:  extractEnumValues(opt.Value, opt.Type()),
			Required:    opt.Required,
			Default:     opt.Default,
		})
	}
	return out
}

func toArgMeta(args redant.ArgSet) []ArgMeta {
	out := make([]ArgMeta, 0, len(args))
	for i, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			name = fmt.Sprintf("arg%d", i+1)
		}
		typ := "string"
		if arg.Value != nil {
			if v, ok := arg.Value.(interface{ Type() string }); ok {
				typ = v.Type()
			}
		}
		out = append(out, ArgMeta{
			Name:        name,
			Description: strings.TrimSpace(arg.Description),
			Type:        typ,
			EnumValues:  extractEnumValues(arg.Value, typ),
			Required:    arg.Required,
			Default:     arg.Default,
		})
	}
	return out
}

func extractEnumValues(value pflag.Value, typ string) []string {
	vals := extractEnumValuesFromValue(value)
	if len(vals) == 0 {
		vals = parseEnumValuesFromType(typ)
	}
	return normalizeEnumValues(vals)
}

func extractEnumValuesFromValue(value pflag.Value) []string {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case *redant.Enum:
		return append([]string(nil), v.Choices...)
	case *redant.EnumArray:
		return append([]string(nil), v.Choices...)
	case interface{ Underlying() pflag.Value }:
		return extractEnumValuesFromValue(v.Underlying())
	default:
		return nil
	}
}

func parseEnumValuesFromType(typ string) []string {
	if !(strings.HasPrefix(typ, "enum[") || strings.HasPrefix(typ, "enum-array[")) || !strings.HasSuffix(typ, "]") {
		return nil
	}

	start := strings.IndexByte(typ, '[')
	if start < 0 || start+1 >= len(typ)-1 {
		return nil
	}
	inner := typ[start+1 : len(typ)-1]

	var parts []string
	if strings.Contains(inner, `\|`) {
		parts = strings.Split(inner, `\|`)
	} else {
		parts = strings.Split(inner, "|")
	}
	return parts
}

func normalizeEnumValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		v := normalizeEnumValue(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func normalizeEnumValue(value string) string {
	v := strings.TrimSpace(value)
	v = strings.ReplaceAll(v, `\|`, "|")
	v = strings.Trim(v, " \\|,;[](){}\"'`")
	return strings.TrimSpace(v)
}

func commandDescription(cmd *redant.Command) string {
	short := strings.TrimSpace(cmd.Short)
	long := strings.TrimSpace(cmd.Long)
	switch {
	case short != "" && long != "":
		return short + "\n\n" + long
	case short != "":
		return short
	case long != "":
		return long
	default:
		return ""
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
