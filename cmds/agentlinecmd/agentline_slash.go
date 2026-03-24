package agentlinecmd

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/pubgo/redant"
)

type slashCommand struct {
	Name        string
	Aliases     []string
	Description string
}

type slashHandler func(m *agentlineModel, raw, cmdText, argText string) tea.Cmd

type slashBuiltin struct {
	Name        string
	Aliases     []string
	Description string
	Handler     slashHandler
}

var slashBuiltins = []slashBuiltin{
	{
		Name:        "chat",
		Description: "绑定聊天粘性模式（/chat <command ...>）",
		Handler: func(m *agentlineModel, _, _, argText string) tea.Cmd {
			sticky, err := buildStickyInvocation(m.root, argText, m.agentOnlyMode)
			if err != nil {
				m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/chat", Lines: []string{err.Error()}})
				return nil
			}
			m.bindStickyInvocation(sticky)
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/chat", Lines: []string{
				"已进入聊天粘性模式。",
				fmt.Sprintf("后续普通输入将自动补全为: %s <文本>", strings.Join(append(append([]string(nil), sticky.BaseArgs...), sticky.PromptFlag), " ")),
				"输入 /unbind 退出聊天粘性模式。",
			}})
			return nil
		},
	},
	{
		Name:        "run",
		Aliases:     []string{"r"},
		Description: "执行 redant 命令（tool -> command -> result）",
		Handler: func(m *agentlineModel, _, _, argText string) tea.Cmd {
			if argText == "" {
				m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/run", Lines: []string{"用法：/run <command ...>"}})
				return nil
			}
			return m.startCommandRun(argText)
		},
	},
	{
		Name:        "output",
		Aliases:     []string{"o", "out"},
		Description: "进入输出滚动模式",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			m.outputFocus = true
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/output", Lines: []string{
				"已进入输出滚动模式。",
				"使用 ↑/↓ 单行滚动，PgUp/PgDn 翻页，Home/End 顶/底。",
				"输入 /input 返回普通输入模式。",
			}})
			return nil
		},
	},
	{
		Name:        "input",
		Aliases:     []string{"i"},
		Description: "返回输入模式",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			m.outputFocus = false
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/input", Lines: []string{"已返回输入模式。"}})
			return nil
		},
	},
	{Name: "top", Description: "跳到历史顶部", Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd { m.scrollOutputTop(); return nil }},
	{Name: "bottom", Aliases: []string{"end"}, Description: "跳到历史底部", Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd { m.scrollOutputBottom(); return nil }},
	{Name: "up", Description: "历史按行向上滚动", Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd { m.scrollOutputLines(1); return nil }},
	{Name: "down", Description: "历史按行向下滚动", Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd { m.scrollOutputLines(-1); return nil }},
	{Name: "pgup", Description: "历史按页向上滚动", Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd { m.scrollOutputPage(1); return nil }},
	{Name: "pgdown", Description: "历史按页向下滚动", Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd { m.scrollOutputPage(-1); return nil }},
	{
		Name:        "history",
		Aliases:     []string{"his"},
		Description: "查看输入历史（默认最近 20 条，可 /history 50）",
		Handler: func(m *agentlineModel, _, _, argText string) tea.Cmd {
			limit := 20
			if argText != "" {
				n, err := strconv.Atoi(argText)
				if err != nil || n <= 0 {
					m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/history", Lines: []string{"用法：/history [正整数]，例如 /history 50"}})
					return nil
				}
				limit = n
			}

			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/history", Lines: m.historyPreviewLines(limit)})
			m.outputOffset = 0
			return nil
		},
	},
	{
		Name:        "cancel",
		Aliases:     []string{"stop"},
		Description: "中断当前运行中的任务",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			if m.cancelActiveRun("收到 /cancel，正在尝试中断当前任务...") {
				return nil
			}
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/cancel", Lines: []string{"当前没有可中断的运行任务。"}})
			return nil
		},
	},
	{
		Name:        "fold",
		Description: "折叠 assistant/tool 详情块",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			if m.foldDetails {
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/fold", Lines: []string{"当前已是折叠状态。"}})
			} else {
				m.foldDetails = true
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/fold", Lines: []string{"已折叠 assistant/tool 详情。输入 /unfold 可恢复。"}})
			}
			return nil
		},
	},
	{
		Name:        "unfold",
		Description: "展开 assistant/tool 详情块",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			if !m.foldDetails {
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/unfold", Lines: []string{"当前已是展开状态。"}})
			} else {
				m.foldDetails = false
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/unfold", Lines: []string{"已展开 assistant/tool 详情。"}})
			}
			return nil
		},
	},
	{
		Name:        "clear",
		Aliases:     []string{"cls"},
		Description: "清空历史块（保留欢迎信息）",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			m.blocks = []sessionBlock{{
				Kind:  blockKindSystem,
				Title: "system",
				Lines: []string{"输出历史已清空。", "输入 /help 查看可用命令。"},
			}}
			m.outputOffset = 0
			return nil
		},
	},
	{
		Name:        "unbind",
		Aliases:     []string{"chat-off"},
		Description: "退出聊天粘性模式",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			if !m.isChatMode() {
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/unbind", Lines: []string{"当前未启用聊天粘性模式。"}})
			} else {
				m.unbindStickyInvocation()
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/unbind", Lines: []string{"已退出聊天粘性模式。"}})
			}
			return nil
		},
	},
	{
		Name:        "help",
		Aliases:     []string{"?"},
		Description: "显示 slash 命令帮助",
		Handler: func(m *agentlineModel, _, _, _ string) tea.Cmd {
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/help", Lines: slashHelpLines(m.root, m.agentOnlyMode)})
			return nil
		},
	},
	{Name: "quit", Aliases: []string{"exit", "q"}, Description: "退出 agentline", Handler: func(_ *agentlineModel, _, _, _ string) tea.Cmd { return tea.Quit }},
}

var (
	slashCommands      = buildSlashCommands(slashBuiltins)
	slashHandlerByName = buildSlashHandlerByName(slashBuiltins)
)

func buildSlashCommands(builtins []slashBuiltin) []slashCommand {
	out := make([]slashCommand, 0, len(builtins))
	for _, item := range builtins {
		out = append(out, slashCommand{Name: item.Name, Aliases: append([]string(nil), item.Aliases...), Description: item.Description})
	}
	return out
}

func buildSlashHandlerByName(builtins []slashBuiltin) map[string]slashHandler {
	index := make(map[string]slashHandler, len(builtins)*2)
	for _, item := range builtins {
		if item.Handler == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if name != "" {
			index[name] = item.Handler
		}
		for _, alias := range item.Aliases {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key == "" {
				continue
			}
			index[key] = item.Handler
		}
	}
	return index
}

func (m *agentlineModel) handleSlashInput(line string) (bool, tea.Cmd) {
	raw := strings.TrimSpace(line)
	if !strings.HasPrefix(raw, "/") {
		return false, nil
	}

	cmdText := strings.TrimSpace(strings.TrimPrefix(raw, "/"))
	if cmdText == "" {
		cmdText = "help"
	}
	parts := strings.Fields(cmdText)
	cmd := strings.ToLower(parts[0])
	argText := strings.TrimSpace(strings.TrimPrefix(cmdText, parts[0]))

	if handler, ok := slashHandlerByName[cmd]; ok {
		cmdOut := handler(m, raw, cmdText, argText)
		m.normalizeOutputOffset()
		return true, cmdOut
	}

	if isCommandLikeInputWithAlias(m.root, cmdText, m.agentOnlyMode, false) {
		return true, m.startCommandRun(cmdText)
	}

	m.appendBlock(sessionBlock{Kind: blockKindError, Title: raw, Lines: []string{
		fmt.Sprintf("未知 slash 命令: %s", cmd),
		"可尝试 /run <command...>，或直接使用 /<command ...> 形式。",
		"输入 /help 查看可用命令。",
	}})
	m.normalizeOutputOffset()
	return true, nil
}

func slashHelpLines(root *redant.Command, agentOnly bool) []string {
	lines := []string{
		"slash commands:",
		"  /chat <command ...>: 绑定聊天粘性模式（后续普通输入自动补全命令前缀）",
		"  /<command ...>: 直接执行命令（例如 /commit --message hi）",
		"  /run <command...>: 执行命令并输出 tool/command/result",
		"  /history [N]: 查看最近输入历史（默认 20 条）",
		"  /cancel: 中断当前运行中的任务",
		"  /fold: 折叠 assistant/tool 详情块",
		"  /unfold: 展开 assistant/tool 详情块",
		"  /unbind: 退出聊天粘性模式",
		"  /output (/o): 进入输出滚动模式",
		"  /input (/i): 返回输入模式",
		"  /top /bottom /up /down /pgup /pgdown: 浏览历史",
		"  /clear: 清空历史块",
		"  /quit: 退出 agentline",
	}

	commands := collectCommandSlashItems(root, agentOnly, "")
	if len(commands) > 0 {
		lines = append(lines, "")
		lines = append(lines, "可直接执行的命令：")
		limit := len(commands)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			lines = append(lines, "  "+strings.TrimSpace(commands[i].Insert))
		}
		if len(commands) > limit {
			lines = append(lines, fmt.Sprintf("  ...(共 %d 个，可输入 / 查看候选)", len(commands)))
		}
	}

	return lines
}
