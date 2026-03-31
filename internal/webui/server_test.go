package webui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/completioncmd"
)

func TestCommandsEndpoint(t *testing.T) {
	var global string
	var local string
	var format string

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
			{Flag: "format", Description: "output format", Value: redant.EnumOf(&format, "json", "text")},
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
	defer closeResponseBody(t, resp)

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
	if !slices.Equal(flagByName["format"].EnumValues, []string{"json", "text"}) {
		t.Fatalf("unexpected format enum values: %+v", flagByName["format"].EnumValues)
	}
	if strings.TrimSpace(flagByName["local"].Description) == "" {
		t.Fatalf("expected local flag description, got empty")
	}
	if !slices.Equal(flagByName["global"].Envs, []string{"GLOBAL_ENV"}) {
		t.Fatalf("unexpected global envs: %+v", flagByName["global"].Envs)
	}
}

func TestCommandsEndpointIncludesStreamMetadata(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use: "chat",
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			stream := out.Raw()
			if err := stream.Send(map[string]any{"event": "output", "data": "hello"}); err != nil {
				return err
			}
			return stream.Send(map[string]any{"event": "round_end", "data": map[string]any{"round": 1, "reason": "done"}})
		}),
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/commands")
	if err != nil {
		t.Fatalf("request commands: %v", err)
	}
	defer closeResponseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload commandListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode commands response: %v", err)
	}

	if len(payload.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(payload.Commands))
	}

	cmd := payload.Commands[0]
	if cmd.ID != "chat" {
		t.Fatalf("expected chat command, got %s", cmd.ID)
	}
	if !cmd.SupportsStream {
		t.Fatalf("expected supportsStream=true")
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
	defer closeResponseBody(t, resp)

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
	defer closeResponseBody(t, resp)

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
	defer closeResponseBody(t, resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing arg, got %d", resp.StatusCode)
	}
}

func TestRunEndpointUsesRawArgsFallback(t *testing.T) {
	var text string
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use:  "echo",
		Args: redant.ArgSet{{Name: "text", Required: true, Value: redant.StringOf(&text)}},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = inv.Stdout.Write([]byte(text))
			return nil
		},
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewBufferString(`{"command":"echo","rawArgs":["fallback-text"]}`))
	if err != nil {
		t.Fatalf("run command request: %v", err)
	}
	defer closeResponseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(payload))
	}

	var runResp RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if !runResp.OK {
		t.Fatalf("expected ok, got error=%s", runResp.Error)
	}
	if runResp.Stdout != "fallback-text" {
		t.Fatalf("expected fallback-text, got %q", runResp.Stdout)
	}
	if !strings.Contains(runResp.Invocation, "fallback-text") {
		t.Fatalf("expected invocation contains arg, got %q", runResp.Invocation)
	}
}

func TestRunEndpointWithPreInitializedRootNoDuplicateEnvPanic(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	completioncmd.AddCompletionCommand(root)

	// 先执行一次命令，模拟 web 子命令启动前根命令已初始化的真实场景。
	pre := root.Invoke("completion", "bash")
	pre.Stdout = &bytes.Buffer{}
	pre.Stderr = &bytes.Buffer{}
	if err := pre.Run(); err != nil {
		t.Fatalf("pre-initialize root failed: %v", err)
	}

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	body := `{"command":"completion","args":{"shell":"bash"}}`
	for i := 0; i < 2; i++ {
		resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("run request %d failed: %v", i+1, err)
		}

		if resp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			t.Fatalf("unexpected status on run %d: %d body=%s", i+1, resp.StatusCode, string(payload))
		}

		var out RunResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			_ = resp.Body.Close()
			t.Fatalf("decode run response %d: %v", i+1, err)
		}
		_ = resp.Body.Close()

		if !out.OK {
			t.Fatalf("run %d failed, error=%s stderr=%s", i+1, out.Error, out.Stderr)
		}
	}
}

func TestRunEndpointPassesStdin(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use: "cat",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			in, err := io.ReadAll(inv.Stdin)
			if err != nil {
				return err
			}
			_, _ = inv.Stdout.Write(in)
			return nil
		},
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewBufferString(`{"command":"cat","stdin":"line1\nline2\n"}`))
	if err != nil {
		t.Fatalf("run command request: %v", err)
	}
	defer closeResponseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(payload))
	}

	var runResp RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}

	if !runResp.OK {
		t.Fatalf("expected ok, got error=%s", runResp.Error)
	}

	if runResp.Stdout != "line1\nline2\n" {
		t.Fatalf("unexpected stdout: %q", runResp.Stdout)
	}
}

func TestRunEndpointTimeoutIncludesInteractiveHint(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use: "wait",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewBufferString(`{"command":"wait","timeoutSeconds":1}`))
	if err != nil {
		t.Fatalf("run command request: %v", err)
	}
	defer closeResponseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(payload))
	}

	var runResp RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}

	if runResp.OK {
		t.Fatalf("expected timeout error, got ok=true")
	}

	if !runResp.TimedOut {
		t.Fatalf("expected timedOut=true, got false")
	}

	if !strings.Contains(runResp.Error, "webcmd 不提供 TTY 交互") {
		t.Fatalf("expected timeout hint in error, got %q", runResp.Error)
	}
}

func TestRunWSEndpointInteractive(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use: "repl",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			reader := bufio.NewReader(inv.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			_, _ = inv.Stdout.Write([]byte("echo:" + line))
			return nil
		},
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/run/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()

	if err := wsjson.Write(ctx, conn, wsRunMessage{
		Type: "start",
		Request: &RunRequest{
			Command: "repl",
		},
		Rows: 24,
		Cols: 80,
	}); err != nil {
		t.Fatalf("write start message: %v", err)
	}

	var started wsRunMessage
	if err := wsjson.Read(ctx, conn, &started); err != nil {
		t.Fatalf("read started message: %v", err)
	}
	if started.Type != "started" {
		t.Fatalf("expected started message, got %q", started.Type)
	}

	if err := wsjson.Write(ctx, conn, wsRunMessage{Type: "stdin", Data: "hello\n"}); err != nil {
		t.Fatalf("write stdin message: %v", err)
	}

	var output strings.Builder
	for {
		var msg wsRunMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatalf("read ws message: %v", err)
		}

		switch msg.Type {
		case "output":
			output.WriteString(msg.Data)
		case "exit":
			if !msg.OK {
				t.Fatalf("expected ok exit, got error=%s", msg.Error)
			}
			if !strings.Contains(output.String(), "hello") {
				t.Fatalf("expected interactive output contains hello, got %q", output.String())
			}
			return
		}
	}
}

func TestRunStreamWSEndpoint(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use: "chat",
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
			stream := out.Raw()
			if err := stream.Send(map[string]any{"event": "output", "data": "hello"}); err != nil {
				return err
			}
			if err := stream.Send(map[string]any{"event": "round_end", "data": map[string]any{"round": 1, "reason": "done"}}); err != nil {
				return err
			}
			return stream.Send(map[string]any{"event": "exit", "data": map[string]any{"code": 0, "reason": "ok", "timedOut": false}})
		}),
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/run/stream/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()

	if err := wsjson.Write(ctx, conn, wsRunMessage{Type: "start", Request: &RunRequest{Command: "chat"}}); err != nil {
		t.Fatalf("write start message: %v", err)
	}

	sawStarted := false
	sawStream := false
	sawResult := false

	for !sawResult {
		var msg wsRunMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatalf("read ws message: %v", err)
		}

		switch msg.Type {
		case "started":
			sawStarted = true
		case "stream":
			sawStream = true
			b, err := json.Marshal(msg.Event)
			if err != nil {
				t.Fatalf("marshal stream event: %v", err)
			}
			if !strings.Contains(string(b), "\"event\":\"output\"") && !strings.Contains(string(b), "\"event\":\"round_end\"") && !strings.Contains(string(b), "\"event\":\"exit\"") {
				t.Fatalf("unexpected stream event payload: %s", string(b))
			}
		case "result":
			sawResult = true
			if !msg.OK {
				t.Fatalf("expected ok result, got error=%s", msg.Error)
			}
		}
	}

	if !sawStarted {
		t.Fatalf("expected started message")
	}
	if !sawStream {
		t.Fatalf("expected at least one stream message")
	}
}

func TestTerminalWSEndpointStartAndClose(t *testing.T) {
	root := &redant.Command{Use: "testapp"}
	root.Children = append(root.Children, &redant.Command{
		Use: "echo",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			_, _ = inv.Stdout.Write([]byte("ok\n"))
			return nil
		},
	})

	ts := httptest.NewServer(New(root).Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/terminal/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()

	if err := wsjson.Write(ctx, conn, wsRunMessage{Type: "start", Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("write start message: %v", err)
	}

	started := false
	var startedMsg wsRunMessage
	deadline := time.After(3 * time.Second)
	for !started {
		select {
		case <-deadline:
			t.Fatal("did not receive started message in time")
		default:
			var msg wsRunMessage
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				t.Fatalf("read ws message: %v", err)
			}
			if msg.Type == "started" {
				startedMsg = msg
				started = true
			}
		}
	}

	if strings.TrimSpace(startedMsg.Program) == "" {
		t.Fatalf("expected started message has program, got empty")
	}
	if strings.TrimSpace(startedMsg.WorkingDir) == "" {
		t.Fatalf("expected started message has workingDir, got empty")
	}

	if err := wsjson.Write(ctx, conn, wsRunMessage{Type: "close"}); err != nil {
		t.Fatalf("write close message: %v", err)
	}
}

func TestIsExpectedPTYReadClose(t *testing.T) {
	t.Parallel()

	if !isExpectedPTYReadClose(io.EOF) {
		t.Fatalf("expected io.EOF to be treated as expected close")
	}

	if !isExpectedPTYReadClose(os.ErrClosed) {
		t.Fatalf("expected os.ErrClosed to be treated as expected close")
	}

	if !isExpectedPTYReadClose(syscall.EIO) {
		t.Fatalf("expected syscall.EIO to be treated as expected close")
	}

	if isExpectedPTYReadClose(io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected EOF should not be treated as expected close")
	}
}

func closeResponseBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}
