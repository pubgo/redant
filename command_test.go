package redant

import (
	"bytes"
	"context"
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
