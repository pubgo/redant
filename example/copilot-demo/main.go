package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/agentlinecmd"
	"github.com/pubgo/redant/cmds/webcmd"
	agentlinemodule "github.com/pubgo/redant/pkg/agentline"
)

var demoRT = newDemoRuntime()

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

		hydrateSessions      bool
		hydrateTimeout       string
		hydrateMaxScanEvents int64

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
		Metadata: agentlinemodule.AgentCommandMetadata(),
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
				demoRT.StoreSession(session)
				_, _ = fmt.Fprintf(inv.Stdout, "session created and cached: %s\n", strings.TrimSpace(session.SessionID))

				return sendPromptAndRender(ctx, inv, session, strings.TrimSpace(prompt), streaming)
			})
		},
	}

	resumeCmd := &redant.Command{
		Use:      "resume",
		Short:    "恢复已有会话并继续发送 Prompt。",
		Metadata: agentlinemodule.AgentCommandMetadata(),
		Options: redant.OptionSet{
			{Flag: "session-id", Description: "待恢复的会话 ID", Value: redant.StringOf(&sessionID), Required: true},
			{Flag: "prompt", Shorthand: "p", Description: "继续发送的提示词", Value: redant.StringOf(&prompt), Default: "继续"},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return withClient(ctx, inv, clientOptions{cliPath: cliPath, logLevel: logLevel, cwd: workingDir, token: githubToken, useLoggedInUser: useLoggedInUser}, func(ctx context.Context, client *copilot.Client) error {
				sid := strings.TrimSpace(sessionID)
				var (
					session *copilot.Session
					ok      bool
					err     error
				)

				if session, ok = demoRT.GetSession(sid); ok {
					_, _ = fmt.Fprintf(inv.Stdout, "reuse cached session: %s\n", sid)
				} else {
					session, err = client.ResumeSession(ctx, sid, &copilot.ResumeSessionConfig{
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
					demoRT.StoreSession(session)
					_, _ = fmt.Fprintf(inv.Stdout, "session resumed and cached: %s\n", sid)
				}

				promptText := withDefault(strings.TrimSpace(prompt), "请继续")
				err = sendPromptAndRender(ctx, inv, session, promptText, streaming)
				if err == nil {
					return nil
				}

				if !ok {
					return err
				}

				// 缓存会话失效时，自动回退为一次真实 Resume。
				demoRT.DeleteSession(sid, inv.Stderr)
				session, err = client.ResumeSession(ctx, sid, &copilot.ResumeSessionConfig{
					Model:               strings.TrimSpace(model),
					ReasoningEffort:     strings.TrimSpace(reasoningEffort),
					Streaming:           streaming,
					OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
					OnUserInputRequest:  buildUserInputHandler(inv, autoUserAnswer),
					Hooks:               buildHooks(inv),
				})
				if err != nil {
					return fmt.Errorf("resume session after cache fallback: %w", err)
				}
				demoRT.StoreSession(session)
				_, _ = fmt.Fprintf(inv.Stdout, "cached session invalid, resumed again: %s\n", sid)
				return sendPromptAndRender(ctx, inv, session, promptText, streaming)
			})
		},
	}

	listSessionsCmd := &redant.Command{
		Use:      "sessions",
		Short:    "列出会话。",
		Metadata: agentlinemodule.AgentCommandMetadata(),
		Options: redant.OptionSet{
			{Flag: "hydrate", Description: "尝试恢复会话并提取最近消息摘要", Value: redant.BoolOf(&hydrateSessions), Default: "false"},
			{Flag: "hydrate-timeout", Description: "单个会话补全超时时间", Value: redant.StringOf(&hydrateTimeout), Default: "4s"},
			{Flag: "hydrate-max-events", Description: "每个会话最多扫描的最近事件数", Value: redant.Int64Of(&hydrateMaxScanEvents), Default: "50"},
		},
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

				hydrateCfg := hydrateConfig{
					enabled:   hydrateSessions,
					timeout:   parseDurationOrDefault(hydrateTimeout, 4*time.Second),
					maxEvents: int(hydrateMaxScanEvents),
				}

				onlyIDCount := 0
				hydratedCount := 0
				for _, s := range sessions {
					info := hydrateSessionInfo{maxEvents: hydrateCfg.maxEvents}
					if hydrateCfg.enabled {
						hydratedCount++
						info = hydrateSession(ctx, client, strings.TrimSpace(s.SessionID), hydrateCfg)
					}

					line, onlyID := renderSessionLine(s, info)
					if onlyID {
						onlyIDCount++
					}
					_, _ = fmt.Fprintln(inv.Stdout, line)
				}

				if onlyIDCount > 0 {
					_, _ = fmt.Fprintf(inv.Stdout, "\n提示: %d/%d 条会话仅返回 session id；这是上游 CLI 当前返回的数据范围，非本命令解析异常。\n", onlyIDCount, len(sessions))
				}
				if hydrateCfg.enabled {
					_, _ = fmt.Fprintf(inv.Stdout, "提示: hydrate 已尝试补全 %d 条会话（timeout=%s, maxEvents=%d）。\n", hydratedCount, hydrateCfg.timeout.String(), hydrateCfg.maxEvents)
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
				sid := strings.TrimSpace(deleteSessionID)
				if err := client.DeleteSession(ctx, sid); err != nil {
					return fmt.Errorf("delete session: %w", err)
				}
				demoRT.DeleteSession(sid, inv.Stderr)
				_, _ = fmt.Fprintf(inv.Stdout, "已删除会话: %s\n", sid)
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

	err := rootCmd.Invoke().WithOS().Run()
	demoRT.Close(os.Stderr)
	if err != nil {
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

func (o clientOptions) key() string {
	return strings.Join([]string{
		strings.TrimSpace(o.cliPath),
		withDefault(strings.TrimSpace(o.logLevel), "error"),
		strings.TrimSpace(o.cwd),
		strings.TrimSpace(o.token),
		fmt.Sprintf("%t", o.useLoggedInUser),
	}, "|")
}

type demoRuntime struct {
	mu        sync.Mutex
	client    *copilot.Client
	clientKey string
	sessions  map[string]*copilot.Session
}

func newDemoRuntime() *demoRuntime {
	return &demoRuntime{sessions: make(map[string]*copilot.Session)}
}

func (r *demoRuntime) ensureClient(ctx context.Context, inv *redant.Invocation, opts clientOptions) (*copilot.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := opts.key()
	if r.client != nil && r.clientKey == key {
		return r.client, nil
	}

	if err := r.closeLocked(inv.Stderr); err != nil {
		_, _ = fmt.Fprintf(inv.Stderr, "warn: close previous copilot runtime failed: %v\n", err)
	}

	client := copilot.NewClient(&copilot.ClientOptions{
		CLIPath:         strings.TrimSpace(opts.cliPath),
		LogLevel:        withDefault(strings.TrimSpace(opts.logLevel), "error"),
		Cwd:             strings.TrimSpace(opts.cwd),
		GitHubToken:     strings.TrimSpace(opts.token),
		UseLoggedInUser: copilot.Bool(opts.useLoggedInUser),
		AutoStart:       copilot.Bool(false),
	})

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("start client: %w", err)
	}

	r.client = client
	r.clientKey = key
	r.sessions = make(map[string]*copilot.Session)
	_, _ = fmt.Fprintln(inv.Stdout, "Copilot client started")
	return r.client, nil
}

func (r *demoRuntime) GetSession(sessionID string) (*copilot.Session, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionID]
	return s, ok
}

func (r *demoRuntime) StoreSession(session *copilot.Session) {
	if session == nil {
		return
	}
	sid := strings.TrimSpace(session.SessionID)
	if sid == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sid] = session
}

func (r *demoRuntime) DeleteSession(sessionID string, stderr io.Writer) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	r.mu.Lock()
	session, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()

	if ok {
		if err := session.Disconnect(); err != nil && stderr != nil {
			_, _ = fmt.Fprintf(stderr, "warn: disconnect cached session(%s) failed: %v\n", sessionID, err)
		}
	}
}

func (r *demoRuntime) Close(stderr io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.closeLocked(stderr); err != nil && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "warn: close copilot runtime failed: %v\n", err)
	}
}

func (r *demoRuntime) closeLocked(stderr io.Writer) error {
	var closeErr error
	for sid, session := range r.sessions {
		if session == nil {
			delete(r.sessions, sid)
			continue
		}
		if err := session.Disconnect(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("disconnect session(%s): %w", sid, err))
		}
		delete(r.sessions, sid)
	}

	if r.client != nil {
		if err := r.client.Stop(); err != nil {
			if stderr != nil {
				_, _ = fmt.Fprintf(stderr, "warn: stop client failed: %v\n", err)
			}
			closeErr = errors.Join(closeErr, fmt.Errorf("stop client: %w", err))
		}
	}

	r.client = nil
	r.clientKey = ""
	r.sessions = make(map[string]*copilot.Session)
	return closeErr
}

func withClient(ctx context.Context, inv *redant.Invocation, opts clientOptions, fn func(ctx context.Context, client *copilot.Client) error) error {
	client, err := demoRT.ensureClient(ctx, inv, opts)
	if err != nil {
		return err
	}
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

func withDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func renderSessionLine(s copilot.SessionMetadata, hydrate hydrateSessionInfo) (line string, onlyID bool) {
	parts := []string{fmt.Sprintf("- id=%s", withDefault(strings.TrimSpace(s.SessionID), "(empty)"))}

	if t := strings.TrimSpace(s.StartTime); t != "" {
		parts = append(parts, "start="+t)
	}
	if t := strings.TrimSpace(s.ModifiedTime); t != "" {
		parts = append(parts, "modified="+t)
	}

	if s.Summary != nil {
		summary := strings.TrimSpace(*s.Summary)
		if summary != "" {
			parts = append(parts, "summary="+summary)
		}
	}

	if s.Context != nil {
		if repo := strings.TrimSpace(s.Context.Repository); repo != "" {
			parts = append(parts, "repo="+repo)
		}
		if branch := strings.TrimSpace(s.Context.Branch); branch != "" {
			parts = append(parts, "branch="+branch)
		}
		if cwd := strings.TrimSpace(s.Context.Cwd); cwd != "" {
			parts = append(parts, "cwd="+cwd)
		}
	}

	if hydrate.errorText != "" {
		parts = append(parts, "hydrate.error="+hydrate.errorText)
	}
	if hydrate.messageCount > 0 {
		parts = append(parts, fmt.Sprintf("hydrate.messages=%d", hydrate.messageCount))
	}
	if hydrate.lastAssistant != "" {
		parts = append(parts, "hydrate.assistant="+hydrate.lastAssistant)
	}
	if hydrate.maxEvents > 0 {
		parts = append(parts, fmt.Sprintf("hydrate.scan=%d", hydrate.maxEvents))
	}

	if len(parts) == 1 {
		parts = append(parts, "meta=empty")
		return strings.Join(parts, "  "), true
	}

	return strings.Join(parts, "  "), false
}

type hydrateConfig struct {
	enabled   bool
	timeout   time.Duration
	maxEvents int
}

type hydrateSessionInfo struct {
	messageCount  int
	lastAssistant string
	errorText     string
	maxEvents     int
}

func hydrateSession(ctx context.Context, client *copilot.Client, sessionID string, cfg hydrateConfig) hydrateSessionInfo {
	info := hydrateSessionInfo{maxEvents: cfg.maxEvents}
	if !cfg.enabled || sessionID == "" {
		return info
	}

	if cfg.maxEvents <= 0 {
		cfg.maxEvents = 50
		info.maxEvents = cfg.maxEvents
	}

	rctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	session, err := client.ResumeSession(rctx, sessionID, &copilot.ResumeSessionConfig{
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
		DisableResume:       true,
	})
	if err != nil {
		info.errorText = compactText(err.Error(), 120)
		return info
	}
	defer func() {
		if err := session.Disconnect(); err != nil {
			if strings.TrimSpace(info.errorText) == "" {
				info.errorText = compactText(fmt.Sprintf("disconnect session: %v", err), 120)
			} else {
				info.errorText = compactText(info.errorText+"; disconnect session: "+err.Error(), 120)
			}
		}
	}()

	events, err := session.GetMessages(rctx)
	if err != nil {
		info.errorText = compactText(err.Error(), 120)
		return info
	}

	info.messageCount = len(events)
	start := 0
	if len(events) > cfg.maxEvents {
		start = len(events) - cfg.maxEvents
	}

	for i := len(events) - 1; i >= start; i-- {
		e := events[i]
		if e.Type == "assistant.message" && e.Data.Content != nil {
			text := strings.TrimSpace(*e.Data.Content)
			if text != "" {
				info.lastAssistant = compactText(text, 120)
				break
			}
		}
	}

	return info
}

func compactText(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return s[:max-1] + "…"
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
