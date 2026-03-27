package readlinecmd

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/chzyer/readline"

	"github.com/pubgo/redant"
)

func TestAddReadlineCommand(t *testing.T) {
	root := &redant.Command{Use: "app"}
	AddReadlineCommand(root)
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	if got := root.Children[0].Name(); got != "readline" {
		t.Fatalf("expected child name readline, got %q", got)
	}
}

func TestSplitCommandLine(t *testing.T) {
	got, err := splitCommandLine(`commit -m "hello world" --format json`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"commit", "-m", "hello world", "--format", "json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("split mismatch want=%v got=%v", want, got)
	}

	if _, err := splitCommandLine(`commit -m "hello`); err == nil {
		t.Fatal("expected unclosed quote error, got nil")
	}
}

func TestDynamicCompleter(t *testing.T) {
	root := buildTestRoot()
	c := &dynamicCompleter{root: root}

	contains := func(vals []string, want string) bool {
		for _, v := range vals {
			if v == want {
				return true
			}
		}
		return false
	}

	if vals := suggestions(c, "com"); !contains(vals, "commit") {
		t.Fatalf("expected commit in %v", vals)
	}
	if vals := suggestions(c, "commit --fo"); !contains(vals, "--format") {
		t.Fatalf("expected --format in %v", vals)
	}
	if vals := suggestions(c, "commit --format "); !contains(vals, "json") {
		t.Fatalf("expected json in %v", vals)
	}
	if vals := suggestions(c, "commit --format=j"); !contains(vals, "--format=json") {
		t.Fatalf("expected --format=json in %v", vals)
	}
	if vals := suggestions(c, "commit "); !contains(vals, "<target>") {
		t.Fatalf("expected <target> in %v", vals)
	}
}

func suggestions(c *dynamicCompleter, input string) []string {
	_, current := splitCompletionInput(input)
	items, _ := c.Do([]rune(input), len([]rune(input)))
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := string(item)
		switch {
		case current == "":
			out = append(out, s)
		case strings.HasPrefix(s, current):
			out = append(out, s)
		default:
			out = append(out, current+s)
		}
	}
	return out
}

type closeTrackingReader struct {
	r      io.Reader
	closed bool
}

func (c *closeTrackingReader) Read(p []byte) (int, error) { return c.r.Read(p) }

func (c *closeTrackingReader) Close() error {
	c.closed = true
	return nil
}

func TestReaderOnlyNotClosedByInvocation(t *testing.T) {
	tracked := &closeTrackingReader{r: strings.NewReader("hello")}

	cmd := &redant.Command{
		Use: "noop",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			buf := make([]byte, 5)
			_, _ = inv.Stdin.Read(buf)
			return nil
		},
	}

	inv := cmd.Invoke()
	inv.Stdin = readerOnly{r: tracked}
	if _, ok := inv.Stdin.(io.ReadCloser); ok {
		t.Fatal("readerOnly should not implement io.ReadCloser")
	}

	if err := inv.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if tracked.closed {
		t.Fatal("underlying reader should not be closed")
	}
}

func TestFormatCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		program string
		args    []string
		want    string
	}{
		{
			name:    "simple",
			program: "fastcommit",
			args:    []string{"commit", "--format", "json"},
			want:    "fastcommit commit --format json",
		},
		{
			name:    "with spaces and symbols",
			program: "fastcommit",
			args:    []string{"commit", "-m", "hello world", "--expr", "a|b"},
			want:    `fastcommit commit -m "hello world" --expr "a|b"`,
		},
		{
			name:    "empty arg",
			program: "fastcommit",
			args:    []string{"commit", "--note", ""},
			want:    `fastcommit commit --note ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCommandLine(tt.program, tt.args)
			if got != tt.want {
				t.Fatalf("format mismatch\nwant=%s\ngot=%s", tt.want, got)
			}
		})
	}
}

func TestHandleReadlineReadError(t *testing.T) {
	t.Run("ctrl+c empty line exits by default", func(t *testing.T) {
		done, err, pending := handleReadlineReadError(context.Background(), readline.ErrInterrupt, "", false, false)
		if !done {
			t.Fatal("expected done=true for empty interrupt")
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if pending {
			t.Fatal("expected pending=false")
		}
	})

	t.Run("ctrl+c with input continues", func(t *testing.T) {
		done, err, pending := handleReadlineReadError(context.Background(), readline.ErrInterrupt, "commit", false, false)
		if done {
			t.Fatal("expected done=false for interrupt with input")
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if pending {
			t.Fatal("expected pending=false")
		}
	})

	t.Run("double ctrl+c mode requires two interrupts", func(t *testing.T) {
		done, err, pending := handleReadlineReadError(context.Background(), readline.ErrInterrupt, "", true, false)
		if done {
			t.Fatal("expected done=false on first interrupt in double mode")
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !pending {
			t.Fatal("expected pending=true after first interrupt")
		}

		done, err, pending = handleReadlineReadError(context.Background(), readline.ErrInterrupt, "", true, pending)
		if !done {
			t.Fatal("expected done=true on second interrupt in double mode")
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if pending {
			t.Fatal("expected pending=false after exit")
		}
	})

	t.Run("eof exits", func(t *testing.T) {
		done, err, pending := handleReadlineReadError(context.Background(), io.EOF, "", false, false)
		if !done {
			t.Fatal("expected done=true for EOF")
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if pending {
			t.Fatal("expected pending=false")
		}
	})

	t.Run("context canceled exits", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		done, err, pending := handleReadlineReadError(ctx, errors.New("read failed"), "", false, false)
		if !done {
			t.Fatal("expected done=true for canceled context")
		}
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if pending {
			t.Fatal("expected pending=false")
		}
	})

	t.Run("other errors return error", func(t *testing.T) {
		targetErr := errors.New("boom")
		done, err, pending := handleReadlineReadError(context.Background(), targetErr, "", false, false)
		if !done {
			t.Fatal("expected done=true for regular errors")
		}
		if !errors.Is(err, targetErr) {
			t.Fatalf("expected target error, got %v", err)
		}
		if pending {
			t.Fatal("expected pending=false")
		}
	})
}

func buildTestRoot() *redant.Command {
	var (
		format  string
		message string
		amend   bool
		target  string
	)

	commit := &redant.Command{
		Use: "commit",
		Options: redant.OptionSet{
			{Flag: "format", Value: redant.EnumOf(&format, "text", "json", "yaml")},
			{Flag: "message", Shorthand: "m", Value: redant.StringOf(&message)},
			{Flag: "amend", Value: redant.BoolOf(&amend)},
		},
		Args: redant.ArgSet{
			{Name: "target", Value: redant.EnumOf(&target, "alpha", "beta", "release")},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	return &redant.Command{
		Use:      "app",
		Children: []*redant.Command{commit},
		Handler:  func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
}
