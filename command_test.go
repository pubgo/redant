package redant

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandBasic(t *testing.T) {
	var executed bool
	cmd := &Command{
		Use:   "test",
		Short: "A test command",
		Handler: func(ctx context.Context, inv *Invocation) error {
			executed = true
			return nil
		},
	}

	inv := cmd.Invoke()
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executed {
		t.Error("handler was not executed")
	}
}

func TestCommandMeta(t *testing.T) {
	cmd := &Command{
		Use: "test",
		Metadata: map[string]string{
			"Mode":          " agent ",
			"agent.command": " true ",
		},
	}

	if got := cmd.Meta("mode"); got != "agent" {
		t.Fatalf("Meta(mode) = %q, want %q", got, "agent")
	}

	if got := cmd.Meta(" AGENT.COMMAND "); got != "true" {
		t.Fatalf("Meta(AGENT.COMMAND) = %q, want %q", got, "true")
	}

	if got := cmd.Meta("missing"); got != "" {
		t.Fatalf("Meta(missing) = %q, want empty", got)
	}

	if got := cmd.Meta("   "); got != "" {
		t.Fatalf("Meta(blank) = %q, want empty", got)
	}

	var nilCmd *Command
	if got := nilCmd.Meta("mode"); got != "" {
		t.Fatalf("nil command Meta(mode) = %q, want empty", got)
	}
}

func TestCommandWithFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedPort int64
		expectedName string
	}{
		{
			name:         "default values",
			args:         []string{},
			expectedPort: 8080,
			expectedName: "",
		},
		{
			name:         "long flag",
			args:         []string{"--port", "9090", "--name", "myserver"},
			expectedPort: 9090,
			expectedName: "myserver",
		},
		{
			name:         "short flag",
			args:         []string{"-p", "3000", "-n", "test"},
			expectedPort: 3000,
			expectedName: "test",
		},
		{
			name:         "equals syntax",
			args:         []string{"--port=4000", "--name=prod"},
			expectedPort: 4000,
			expectedName: "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var port int64
			var name string

			cmd := &Command{
				Use:   "server",
				Short: "Server command",
				Options: OptionSet{
					{
						Flag:        "port",
						Shorthand:   "p",
						Description: "Port to listen on",
						Value:       Int64Of(&port),
						Default:     "8080",
					},
					{
						Flag:        "name",
						Shorthand:   "n",
						Description: "Server name",
						Value:       StringOf(&name),
					},
				},
				Handler: func(ctx context.Context, inv *Invocation) error {
					return nil
				},
			}

			inv := cmd.Invoke(tt.args...)
			inv.Stdout = &bytes.Buffer{}
			inv.Stderr = &bytes.Buffer{}

			err := inv.Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if port != tt.expectedPort {
				t.Errorf("port = %d, want %d", port, tt.expectedPort)
			}
			if name != tt.expectedName {
				t.Errorf("name = %q, want %q", name, tt.expectedName)
			}
		})
	}
}

func TestCommandWithSubcommands(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedCmd string
	}{
		{
			name:        "subcommand",
			args:        []string{"server"},
			expectedCmd: "server",
		},
		{
			name:        "nested subcommand",
			args:        []string{"server", "start"},
			expectedCmd: "start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var executedCmd string

			rootCmd := &Command{
				Use:   "app",
				Short: "Root command",
			}

			serverCmd := &Command{
				Use:   "server",
				Short: "Server command",
				Handler: func(ctx context.Context, inv *Invocation) error {
					executedCmd = "server"
					return nil
				},
			}

			startCmd := &Command{
				Use:   "start",
				Short: "Start command",
				Handler: func(ctx context.Context, inv *Invocation) error {
					executedCmd = "start"
					return nil
				},
			}

			serverCmd.Children = append(serverCmd.Children, startCmd)
			rootCmd.Children = append(rootCmd.Children, serverCmd)

			inv := rootCmd.Invoke(tt.args...)
			inv.Stdout = &bytes.Buffer{}
			inv.Stderr = &bytes.Buffer{}

			err := inv.Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if executedCmd != tt.expectedCmd {
				t.Errorf("executed command = %q, want %q", executedCmd, tt.expectedCmd)
			}
		})
	}
}

func TestFlagInheritance(t *testing.T) {
	var parentFlag string
	var childFlag string

	rootCmd := &Command{
		Use:   "app",
		Short: "Root command",
	}

	parentCmd := &Command{
		Use:   "parent",
		Short: "Parent command",
		Options: OptionSet{
			{
				Flag:        "parent-flag",
				Description: "Parent flag",
				Value:       StringOf(&parentFlag),
			},
		},
	}

	childCmd := &Command{
		Use:   "child",
		Short: "Child command",
		Options: OptionSet{
			{
				Flag:        "child-flag",
				Description: "Child flag",
				Value:       StringOf(&childFlag),
			},
		},
		Handler: func(ctx context.Context, inv *Invocation) error {
			return nil
		},
	}

	parentCmd.Children = append(parentCmd.Children, childCmd)
	rootCmd.Children = append(rootCmd.Children, parentCmd)

	// Reset
	parentFlag = ""
	childFlag = ""

	inv := rootCmd.Invoke("parent", "child", "--parent-flag", "pvalue", "--child-flag", "cvalue")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parentFlag != "pvalue" {
		t.Errorf("parentFlag = %q, want %q", parentFlag, "pvalue")
	}
	if childFlag != "cvalue" {
		t.Errorf("childFlag = %q, want %q", childFlag, "cvalue")
	}
}

func TestMiddleware(t *testing.T) {
	var order []string

	middleware1 := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, inv *Invocation) error {
			order = append(order, "mw1-before")
			err := next(ctx, inv)
			order = append(order, "mw1-after")
			return err
		}
	}

	middleware2 := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, inv *Invocation) error {
			order = append(order, "mw2-before")
			err := next(ctx, inv)
			order = append(order, "mw2-after")
			return err
		}
	}

	cmd := &Command{
		Use:        "test",
		Short:      "Test command",
		Middleware: Chain(middleware1, middleware2),
		Handler: func(ctx context.Context, inv *Invocation) error {
			order = append(order, "handler")
			return nil
		},
	}

	inv := cmd.Invoke()
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestHelpFlag(t *testing.T) {
	cmd := &Command{
		Use:   "test",
		Short: "A test command",
		Long:  "A longer description of the test command",
		Handler: func(ctx context.Context, inv *Invocation) error {
			t.Error("handler should not be called when --help is passed")
			return nil
		},
	}

	var stdout bytes.Buffer
	inv := cmd.Invoke("--help")
	inv.Stdout = &stdout
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "test") {
		t.Error("help output should contain command name")
	}
	if !strings.Contains(output, "A test command") {
		t.Error("help output should contain short description")
	}
}

func TestRequiredFlag(t *testing.T) {
	var required string

	cmd := &Command{
		Use:   "test",
		Short: "Test command",
		Options: OptionSet{
			{
				Flag:        "required",
				Description: "A required flag",
				Value:       StringOf(&required),
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, inv *Invocation) error {
			return nil
		},
	}

	inv := cmd.Invoke() // No --required flag
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err == nil {
		t.Error("expected error for missing required flag")
	}

	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error should mention 'required', got: %v", err)
	}
}

func TestEnvVarFlag(t *testing.T) {
	var value string

	cmd := &Command{
		Use:   "test",
		Short: "Test command",
		Options: OptionSet{
			{
				Flag:        "value",
				Description: "A value from env",
				Value:       StringOf(&value),
				Envs:        []string{"TEST_VALUE"},
			},
		},
		Handler: func(ctx context.Context, inv *Invocation) error {
			return nil
		},
	}

	// Set environment variable
	t.Setenv("TEST_VALUE", "from-env")

	inv := cmd.Invoke()
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "from-env" {
		t.Errorf("value = %q, want %q", value, "from-env")
	}
}

func TestDeprecatedFlag(t *testing.T) {
	var deprecated string

	cmd := &Command{
		Use:   "test",
		Short: "Test command",
		Options: OptionSet{
			{
				Flag:        "old",
				Description: "An old flag",
				Value:       StringOf(&deprecated),
				Deprecated:  "use --new instead",
			},
		},
		Handler: func(ctx context.Context, inv *Invocation) error {
			return nil
		},
	}

	inv := cmd.Invoke("--old", "value")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The deprecated flag should still work and set the value
	if deprecated != "value" {
		t.Errorf("deprecated flag value = %q, want %q", deprecated, "value")
	}

	// Note: pflag prints deprecation warnings to os.Stderr directly,
	// not to the provided inv.Stderr. This is expected pflag behavior.
}

func TestBusyboxArgv0Dispatch(t *testing.T) {
	var gotArgs []string
	root := &Command{Use: "app"}
	child := &Command{
		Use: "echo",
		Handler: func(ctx context.Context, inv *Invocation) error {
			gotArgs = append([]string(nil), inv.Args...)
			return nil
		},
	}
	root.Children = append(root.Children, child)

	inv := root.Invoke("hello", "world")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}
	inv = inv.WithArgv0("echo")

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gotArgs) != 2 || gotArgs[0] != "hello" || gotArgs[1] != "world" {
		t.Fatalf("unexpected args: %#v", gotArgs)
	}
}

func TestBusyboxArgv0Alias(t *testing.T) {
	var executed bool
	root := &Command{Use: "app"}
	child := &Command{
		Use:     "serve",
		Aliases: []string{"svc"},
		Handler: func(ctx context.Context, inv *Invocation) error {
			executed = true
			return nil
		},
	}
	root.Children = append(root.Children, child)

	inv := root.Invoke()
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}
	inv = inv.WithArgv0("svc")

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executed {
		t.Fatalf("handler for alias was not executed")
	}
}

func TestBusyboxArgv0DoesNotOverrideExplicitArgs(t *testing.T) {
	var executed string
	root := &Command{Use: "app"}
	foo := &Command{
		Use: "foo",
		Handler: func(ctx context.Context, inv *Invocation) error {
			executed = "foo"
			return nil
		},
	}
	bar := &Command{
		Use: "bar",
		Handler: func(ctx context.Context, inv *Invocation) error {
			executed = "bar"
			return nil
		},
	}
	root.Children = append(root.Children, foo, bar)

	inv := root.Invoke("bar")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}
	inv = inv.WithArgv0("foo")

	if err := inv.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executed != "bar" {
		t.Fatalf("expected explicit args to win (bar), got %q", executed)
	}
}

func TestInternalArgsFlagOverridesParsedArgs(t *testing.T) {
	var gotFirst string
	var gotSecond string

	cmd := &Command{
		Use:   "app",
		Short: "test internal args flag",
		Args: ArgSet{
			{Name: "first", Value: StringOf(&gotFirst)},
			{Name: "second", Value: StringOf(&gotSecond)},
		},
		Handler: func(ctx context.Context, inv *Invocation) error {
			if len(inv.Args) != 2 {
				t.Fatalf("inv.Args length = %d, want 2", len(inv.Args))
			}
			if inv.Args[0] != "from-flag-1" || inv.Args[1] != "from-flag-2" {
				t.Fatalf("inv.Args = %#v, want [from-flag-1 from-flag-2]", inv.Args)
			}
			return nil
		},
	}

	inv := cmd.Invoke("from-cli-1", "from-cli-2", "--args", "from-flag-1", "--args", "from-flag-2")
	inv.Stdout = &bytes.Buffer{}
	inv.Stderr = &bytes.Buffer{}

	err := inv.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotFirst != "from-flag-1" {
		t.Fatalf("first arg value = %q, want %q", gotFirst, "from-flag-1")
	}
	if gotSecond != "from-flag-2" {
		t.Fatalf("second arg value = %q, want %q", gotSecond, "from-flag-2")
	}
}

func TestCommandInitHandlerValidation(t *testing.T) {
	tests := []struct {
		name    string
		command *Command
		wantErr bool
	}{
		{
			name: "invalid multiple handler models configured",
			command: &Command{
				Use: "echo",
				Handler: func(ctx context.Context, inv *Invocation) error {
					return nil
				},
				ResponseHandler: Unary(func(ctx context.Context, inv *Invocation) (string, error) {
					return "ok", nil
				}),
			},
			wantErr: true,
		},
		{
			name: "valid response stream handler",
			command: &Command{
				Use: "chat",
				ResponseStreamHandler: Stream(func(ctx context.Context, inv *Invocation, out *TypedWriter[string]) error {
					return out.Send("hello")
				}),
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.command.init()
			if tc.wantErr && err == nil {
				t.Fatalf("expected init error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected init error: %v", err)
			}
		})
	}
}

func TestPrintCommandsJSON(t *testing.T) {
	root := &Command{Use: "app"}
	root.Children = append(root.Children,
		&Command{
			Use:     "deploy",
			Short:   "Deploy app.",
			Aliases: []string{"d"},
			Handler: func(ctx context.Context, inv *Invocation) error { return nil },
		},
		&Command{
			Use:     "secret",
			Short:   "Hidden cmd.",
			Hidden:  true,
			Handler: func(ctx context.Context, inv *Invocation) error { return nil },
		},
	)

	var buf bytes.Buffer
	if err := PrintCommandsJSON(&buf, root); err != nil {
		t.Fatalf("PrintCommandsJSON error: %v", err)
	}

	var cmds []listCommandJSON
	if err := json.Unmarshal(buf.Bytes(), &cmds); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(cmds) != 1 {
		t.Fatalf("expected 1 command (hidden excluded), got %d", len(cmds))
	}
	if cmds[0].Path != "app:deploy" {
		t.Fatalf("path = %q, want %q", cmds[0].Path, "app:deploy")
	}
	if cmds[0].Short != "Deploy app." {
		t.Fatalf("short = %q, want %q", cmds[0].Short, "Deploy app.")
	}
	if len(cmds[0].Aliases) != 1 || cmds[0].Aliases[0] != "d" {
		t.Fatalf("aliases = %v, want [d]", cmds[0].Aliases)
	}
	if !cmds[0].HasHandler {
		t.Fatalf("hasHandler should be true")
	}
}

func TestPrintFlagsJSON(t *testing.T) {
	root := &Command{
		Use: "app",
		Options: OptionSet{
			{Flag: "verbose", Shorthand: "v", Description: "Enable verbose.", Value: BoolOf(new(bool))},
		},
	}
	root.Children = append(root.Children, &Command{
		Use:   "run",
		Short: "Run it.",
		Options: OptionSet{
			{Flag: "count", Description: "Count.", Value: Int64Of(new(int64)), Default: "1", Required: true},
			{Flag: "internal", Hidden: true, Value: StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *Invocation) error { return nil },
	})

	var buf bytes.Buffer
	if err := PrintFlagsJSON(&buf, root); err != nil {
		t.Fatalf("PrintFlagsJSON error: %v", err)
	}

	var flags []listFlagJSON
	if err := json.Unmarshal(buf.Bytes(), &flags); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have 1 global (verbose) + 1 command (count); hidden excluded
	if len(flags) != 2 {
		t.Fatalf("expected 2 flags, got %d: %+v", len(flags), flags)
	}

	// First should be global verbose
	if flags[0].Flag != "verbose" || !flags[0].IsGlobal {
		t.Fatalf("first flag = %+v, want global verbose", flags[0])
	}

	// Second should be count
	if flags[1].Flag != "count" || flags[1].IsGlobal {
		t.Fatalf("second flag = %+v, want non-global count", flags[1])
	}
	if !flags[1].Required {
		t.Fatalf("count should be required")
	}
}

func TestListCommandsFormatJSON(t *testing.T) {
	root := &Command{Use: "app"}
	root.Children = append(root.Children, &Command{
		Use:     "hello",
		Short:   "Say hi.",
		Handler: func(ctx context.Context, inv *Invocation) error { return nil },
	})

	var buf bytes.Buffer
	inv := root.Invoke("--list-commands", "--list-format", "json")
	inv.Stdout = &buf
	inv.Stderr = &bytes.Buffer{}
	inv.Stdin = bytes.NewBuffer(nil)

	if err := inv.Run(); err != nil {
		t.Fatalf("run error: %v", err)
	}

	var cmds []listCommandJSON
	if err := json.Unmarshal(buf.Bytes(), &cmds); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(cmds) == 0 {
		t.Fatalf("expected at least 1 command in JSON output")
	}
}
