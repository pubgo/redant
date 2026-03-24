package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/agentlinecmd"
	"github.com/pubgo/redant/cmds/webcmd"
	agentlinemodule "github.com/pubgo/redant/pkg/agentline"
)

func main() {
	var (
		cliPath         string
		logLevel        string
		workingDir      string
		githubToken     string
		useLoggedInUser bool

		model           string
		reasoningEffort string
		systemMessage   string
		streaming       bool
		autoUserAnswer  string

		prompt    string
		sessionID string
		pingMsg   string

		deleteSessionID string
	)

	rootCmd := &redant.Command{
		Use:   "copilot-demo",
		Short: "Copilot SDK + redant + agentline 集成示例。",
		Long:  "演示如何在 redant CLI 中通过 Copilot Go SDK 复用 Copilot CLI 能力，并支持 agentline 的 slash 命令执行。",
		Options: redant.OptionSet{
			{Flag: "copilot-cli-path", Description: "Copilot CLI 可执行路径（可选）", Value: redant.StringOf(&cliPath)},
			{Flag: "copilot-log-level", Description: "Copilot CLI 日志级别", Value: redant.StringOf(&logLevel), Default: "error"},
			{Flag: "copilot-cwd", Description: "Copilot CLI 进程工作目录", Value: redant.StringOf(&workingDir)},
			{Flag: "copilot-token", Description: "GitHub Token（可选，优先于已登录用户）", Value: redant.StringOf(&githubToken), Envs: []string{"GITHUB_TOKEN"}},
			{Flag: "copilot-use-logged-in-user", Description: "是否使用已登录用户身份", Value: redant.BoolOf(&useLoggedInUser), Default: "true"},
			{Flag: "model", Description: "会话模型", Value: redant.StringOf(&model), Default: "gpt-5"},
			{Flag: "reasoning-effort", Description: "推理强度(low/medium/high/xhigh)", Value: redant.StringOf(&reasoningEffort)},
			{Flag: "system-message", Description: "追加系统提示词（append 模式）", Value: redant.StringOf(&systemMessage)},
			{Flag: "stream", Description: "启用流式输出", Value: redant.BoolOf(&streaming), Default: "false"},
			{Flag: "auto-user-answer", Description: "ask_user 工具触发时自动回答内容", Value: redant.StringOf(&autoUserAnswer), Default: "继续执行"},
		},
	}

	chatCmd := &redant.Command{
		Use:      "chat",
		Short:    "创建新会话并发送 Prompt。",
		Metadata: map[string]string{agentlinemodule.CommandMetaAgentCommand: "true"},
		Options: redant.OptionSet{
			{Flag: "prompt", Shorthand: "p", Description: "要发送给 Copilot 的提示词", Value: redant.StringOf(&prompt), Required: true},
			{Flag: "session-id", Description: "指定会话 ID（可选，不指定则自动生成）", Value: redant.StringOf(&sessionID)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				tool := newEchoTool(inv)
				session, err := client.CreateSession(ctx, &copilot.SessionConfig{
					SessionID:           strings.TrimSpace(sessionID),
					Model:               strings.TrimSpace(model),
					ReasoningEffort:     strings.TrimSpace(reasoningEffort),
					Streaming:           streaming,
					Tools:               []copilot.Tool{tool},
					SystemMessage:       buildSystemMessage(systemMessage),
					OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
					OnUserInputRequest:  buildUserInputHandler(inv, autoUserAnswer),
					Hooks:               buildHooks(inv),
				})
				if err != nil {
					return fmt.Errorf("create session: %w", err)
				}
				defer session.Disconnect()

				return sendPromptAndRender(ctx, inv, session, strings.TrimSpace(prompt), streaming)
			})
		},
	}

	resumeCmd := &redant.Command{
		Use:      "resume",
		Short:    "恢复已有会话并继续发送 Prompt。",
		Metadata: map[string]string{agentlinemodule.CommandMetaAgentCommand: "true"},
		Options: redant.OptionSet{
			{Flag: "session-id", Description: "待恢复的会话 ID", Value: redant.StringOf(&sessionID), Required: true},
			{Flag: "prompt", Shorthand: "p", Description: "继续发送的提示词", Value: redant.StringOf(&prompt), Required: true},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				session, err := client.ResumeSession(ctx, strings.TrimSpace(sessionID), &copilot.ResumeSessionConfig{
					Model:               strings.TrimSpace(model),
					ReasoningEffort:     strings.TrimSpace(reasoningEffort),
					Streaming:           streaming,
					OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
					OnUserInputRequest:  buildUserInputHandler(inv, autoUserAnswer),
					Hooks:               buildHooks(inv),
				})
				if err != nil {
					return fmt.Errorf("resume session: %w", err)
				}
				defer session.Disconnect()

				return sendPromptAndRender(ctx, inv, session, strings.TrimSpace(prompt), streaming)
			})
		},
	}

	listSessionsCmd := &redant.Command{
		Use:   "sessions",
		Short: "列出会话。",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				sessions, err := client.ListSessions(ctx, nil)
				if err != nil {
					return fmt.Errorf("list sessions: %w", err)
				}

				if len(sessions) == 0 {
					_, _ = fmt.Fprintln(inv.Stdout, "暂无会话")
					return nil
				}

				for _, s := range sessions {
					_, _ = fmt.Fprintf(inv.Stdout, "- %s  start=%s  modified=%s\n", s.SessionID, s.StartTime, s.ModifiedTime)
				}
				return nil
			})
		},
	}

	lastSessionCmd := &redant.Command{
		Use:   "last-session",
		Short: "获取最近活跃会话 ID。",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				id, err := client.GetLastSessionID(ctx)
				if err != nil {
					return fmt.Errorf("get last session: %w", err)
				}
				if id == nil || strings.TrimSpace(*id) == "" {
					_, _ = fmt.Fprintln(inv.Stdout, "暂无最近会话")
					return nil
				}
				_, _ = fmt.Fprintln(inv.Stdout, *id)
				return nil
			})
		},
	}

	deleteSessionCmd := &redant.Command{
		Use:   "delete-session",
		Short: "删除指定会话。",
		Options: redant.OptionSet{
			{Flag: "session-id", Description: "待删除会话 ID", Value: redant.StringOf(&deleteSessionID), Required: true},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				if err := client.DeleteSession(ctx, strings.TrimSpace(deleteSessionID)); err != nil {
					return fmt.Errorf("delete session: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stdout, "已删除会话: %s\n", strings.TrimSpace(deleteSessionID))
				return nil
			})
		},
	}

	modelsCmd := &redant.Command{
		Use:   "models",
		Short: "列出可用模型。",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				models, err := client.ListModels(ctx)
				if err != nil {
					return fmt.Errorf("list models: %w", err)
				}
				if len(models) == 0 {
					_, _ = fmt.Fprintln(inv.Stdout, "无可用模型")
					return nil
				}
				for _, m := range models {
					_, _ = fmt.Fprintf(inv.Stdout, "- %s (%s)\n", m.ID, m.Name)
				}
				return nil
			})
		},
	}

	statusCmd := &redant.Command{
		Use:   "status",
		Short: "查看连接、认证与状态信息。",
		Options: redant.OptionSet{
			{Flag: "ping-message", Description: "Ping 消息", Value: redant.StringOf(&pingMsg), Default: "copilot-demo ping"},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				resp, err := client.Ping(ctx, strings.TrimSpace(pingMsg))
				if err != nil {
					return fmt.Errorf("ping: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stdout, "ping: message=%q timestamp=%d\n", resp.Message, resp.Timestamp)

				status, err := client.GetStatus(ctx)
				if err == nil {
					_, _ = fmt.Fprintf(inv.Stdout, "status: version=%s protocol=%d\n", status.Version, status.ProtocolVersion)
				}

				auth, err := client.GetAuthStatus(ctx)
				if err == nil {
					_, _ = fmt.Fprintf(inv.Stdout, "auth: isAuthenticated=%v\n", auth.IsAuthenticated)
				}

				return nil
			})
		},
	}

	rootCmd.Children = []*redant.Command{
		chatCmd,
		resumeCmd,
		listSessionsCmd,
		lastSessionCmd,
		deleteSessionCmd,
		modelsCmd,
		statusCmd,
		webcmd.New(),
		agentlinecmd.New(),
	}

	if err := rootCmd.Invoke().WithOS().Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type clientOptions struct {
	cliPath         string
	logLevel        string
	cwd             string
	token           string
	useLoggedInUser bool
}

func withClient(ctx context.Context, inv *redant.Invocation, opts clientOptions, fn func(ctx context.Context, client *copilot.Client) error) error {
	client := copilot.NewClient(&copilot.ClientOptions{
		CLIPath:         strings.TrimSpace(opts.cliPath),
		LogLevel:        withDefault(strings.TrimSpace(opts.logLevel), "error"),
		Cwd:             strings.TrimSpace(opts.cwd),
		GitHubToken:     strings.TrimSpace(opts.token),
		UseLoggedInUser: copilot.Bool(opts.useLoggedInUser),
		AutoStart:       copilot.Bool(false),
	})

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("start client: %w", err)
	}
	defer client.Stop()

	_, _ = fmt.Fprintln(inv.Stdout, "Copilot client started")
	return fn(ctx, client)
}

func sendPromptAndRender(ctx context.Context, inv *redant.Invocation, session *copilot.Session, prompt string, stream bool) error {
	if prompt == "" {
		return fmt.Errorf("prompt 不能为空")
	}

	_, _ = fmt.Fprintf(inv.Stdout, "session=%s\n", session.SessionID)

	done := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch event.Type {
		case "assistant.message_delta", "assistant.reasoning_delta":
			if stream && event.Data.DeltaContent != nil {
				_, _ = fmt.Fprint(inv.Stdout, *event.Data.DeltaContent)
			}
		case "assistant.message":
			if event.Data.Content != nil {
				if stream {
					_, _ = fmt.Fprintln(inv.Stdout)
				}
				_, _ = fmt.Fprintf(inv.Stdout, "assistant: %s\n", *event.Data.Content)
			}
		case "session.error":
			if event.Data.Message != nil {
				select {
				case errCh <- fmt.Errorf("session error: %s", *event.Data.Message):
				default:
				}
			}
		case "session.idle":
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})
	defer unsubscribe()

	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: prompt}); err != nil {
		return fmt.Errorf("send prompt: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	select {
	case <-done:
		return nil
	case err := <-errCh:
		return err
	case <-waitCtx.Done():
		return fmt.Errorf("wait session idle: %w", waitCtx.Err())
	}
}

func buildHooks(inv *redant.Invocation) *copilot.SessionHooks {
	return &copilot.SessionHooks{
		OnSessionStart: func(input copilot.SessionStartHookInput, invocation copilot.HookInvocation) (*copilot.SessionStartHookOutput, error) {
			_, _ = fmt.Fprintf(inv.Stdout, "[hook] session start: source=%s session=%s\n", input.Source, invocation.SessionID)
			return nil, nil
		},
		OnSessionEnd: func(input copilot.SessionEndHookInput, invocation copilot.HookInvocation) (*copilot.SessionEndHookOutput, error) {
			_, _ = fmt.Fprintf(inv.Stdout, "[hook] session end: reason=%s session=%s\n", input.Reason, invocation.SessionID)
			return nil, nil
		},
	}
}

func buildSystemMessage(content string) *copilot.SystemMessageConfig {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	return &copilot.SystemMessageConfig{Content: content}
}

func buildUserInputHandler(inv *redant.Invocation, answer string) copilot.UserInputHandler {
	ans := withDefault(strings.TrimSpace(answer), "继续执行")
	return func(request copilot.UserInputRequest, invocation copilot.UserInputInvocation) (copilot.UserInputResponse, error) {
		_, _ = fmt.Fprintf(inv.Stdout, "[ask_user] session=%s question=%s\n", invocation.SessionID, request.Question)
		return copilot.UserInputResponse{Answer: ans, WasFreeform: true}, nil
	}
}

func newEchoTool(inv *redant.Invocation) copilot.Tool {
	return copilot.Tool{
		Name:           "demo_echo",
		Description:    "Echo input text for demo",
		SkipPermission: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "text to echo"},
			},
			"required": []string{"text"},
		},
		Handler: func(invocation copilot.ToolInvocation) (copilot.ToolResult, error) {
			text := ""
			if m, ok := invocation.Arguments.(map[string]any); ok {
				if v, ok := m["text"]; ok {
					text = strings.TrimSpace(fmt.Sprint(v))
				}
			}
			if text == "" {
				text = "(empty)"
			}

			_, _ = fmt.Fprintf(inv.Stdout, "[tool:demo_echo] session=%s text=%q\n", invocation.SessionID, text)
			return copilot.ToolResult{
				TextResultForLLM: text,
				ResultType:       "success",
				SessionLog:       "demo_echo executed",
				ToolTelemetry:    map[string]any{"tool": "demo_echo"},
			}, nil
		},
	}
}

func withDefault(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
