package agentlinecmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pubgo/redant"
	agentlinemodule "github.com/pubgo/redant/pkg/agentline"
)

const (
	defaultSuggestionRows = 8
	defaultOutputRows     = 20
	defaultInputRows      = 4
	minOutputRows         = 6
	maxOutputBlocks       = 500
	maxOutputLines        = 4000
)

type mouseRegion string

const (
	mouseRegionOutput mouseRegion = "output"
	mouseRegionInput  mouseRegion = "input"
)

type mouseScrollMsg struct {
	Region mouseRegion
	Delta  int
}

type mouseSelectHistoryMsg struct {
	HistoryIndex int
}

type blockKind string

const (
	blockKindSystem    blockKind = "system"
	blockKindUser      blockKind = "user"
	blockKindAssistant blockKind = "assistant"
	blockKindTool      blockKind = "tool"
	blockKindCommand   blockKind = "command"
	blockKindResult    blockKind = "result"
	blockKindError     blockKind = "error"
)

type sessionBlock struct {
	Kind  blockKind
	Title string
	Lines []string
}

type completionItem struct {
	Insert      string
	Description string
}

type slashCommand struct {
	Name        string
	Aliases     []string
	Description string
}

var slashCommands = []slashCommand{
	{Name: "ask", Aliases: []string{"a"}, Description: "发起一次用户提问（user -> assistant）"},
	{Name: "plan", Aliases: []string{"p"}, Description: "让 assistant 给出分步骤计划"},
	{Name: "run", Aliases: []string{"r"}, Description: "执行 redant 命令（tool -> command -> result）"},
	{Name: "output", Aliases: []string{"o", "out"}, Description: "进入输出滚动模式"},
	{Name: "input", Aliases: []string{"i"}, Description: "返回输入模式"},
	{Name: "top", Description: "跳到历史顶部"},
	{Name: "bottom", Aliases: []string{"end"}, Description: "跳到历史底部"},
	{Name: "up", Description: "历史按行向上滚动"},
	{Name: "down", Description: "历史按行向下滚动"},
	{Name: "pgup", Description: "历史按页向上滚动"},
	{Name: "pgdown", Description: "历史按页向下滚动"},
	{Name: "history", Aliases: []string{"his"}, Description: "查看输入历史（默认最近 20 条，可 /history 50）"},
	{Name: "cancel", Aliases: []string{"stop"}, Description: "中断当前运行中的任务"},
	{Name: "fold", Description: "折叠 assistant/tool 详情块"},
	{Name: "unfold", Description: "展开 assistant/tool 详情块"},
	{Name: "clear", Aliases: []string{"cls"}, Description: "清空历史块（保留欢迎信息）"},
	{Name: "help", Aliases: []string{"?"}, Description: "显示 slash 命令帮助"},
	{Name: "quit", Aliases: []string{"exit", "q"}, Description: "退出 agentline"},
}

var (
	stylePrompt          = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleInputText       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleHeader          = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleHint            = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleSelected        = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true)
	styleHistorySelected = lipgloss.NewStyle().Background(lipgloss.Color("60")).Foreground(lipgloss.Color("230"))
	styleRunning         = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	styleDesc            = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleStatusIdle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	styleStatusBusy      = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)

	styleKindSystem    = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	styleKindUser      = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	styleKindAssistant = lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
	styleKindTool      = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	styleKindCommand   = lipgloss.NewStyle().Foreground(lipgloss.Color("150")).Bold(true)
	styleKindResult    = lipgloss.NewStyle().Foreground(lipgloss.Color("121")).Bold(true)
	styleKindError     = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

func New() *redant.Command {
	ensureRouteHookRegistered()

	var (
		prompt     string
		history    string
		noHistory  bool
		initialArg []string
	)

	return &redant.Command{
		Use:   "agentline",
		Short: "Agent CLI 风格交互终端（会话块 + slash）",
		Long:  "启动交互式 agentline，支持 user/assistant/tool/command/result 会话块与 /ask /plan /run 等 slash 命令。",
		Options: redant.OptionSet{
			{Flag: "prompt", Description: "交互提示符", Value: redant.StringOf(&prompt), Default: "agent> "},
			{Flag: "history-file", Description: "历史记录文件路径（为空自动使用 ~/.redant_agentline_history）", Value: redant.StringOf(&history)},
			{Flag: "no-history", Description: "禁用历史记录持久化", Value: redant.BoolOf(&noHistory)},
			{Flag: "initial-arg", Description: "内部参数：自动进入 agent 模式时的原始 argv", Value: redant.StringArrayOf(&initialArg), Hidden: true},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			historyFile := strings.TrimSpace(history)
			if historyFile == "" && !noHistory {
				if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
					historyFile = filepath.Join(home, ".redant_agentline_history")
				}
			}

			historyLines := []string{}
			if !noHistory && historyFile != "" {
				historyLines = loadHistory(historyFile)
			}

			model := newAgentlineModel(ctx, root, prompt, historyLines, historyFile, !noHistory, initialArg)
			p := tea.NewProgram(model, tea.WithInput(inv.Stdin), tea.WithOutput(inv.Stdout))

			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					p.Quit()
				case <-done:
				}
			}()

			_, err := p.Run()
			close(done)
			return err
		},
	}
}

func AddAgentlineCommand(rootCmd *redant.Command) {
	rootCmd.Children = append(rootCmd.Children, New())
}

type agentlineModel struct {
	ctx             context.Context
	root            *redant.Command
	input           textinput.Model
	prompt          string
	history         []string
	historyPos      int
	historyFile     string
	persistHistory  bool
	blocks          []sessionBlock
	suggestions     []completionItem
	selected        int
	running         bool
	width           int
	height          int
	outputOffset    int
	outputFocus     bool
	inputOffset     int
	selectedHistory int
	foldDetails     bool
	currentCancel   context.CancelFunc
	initialArgv     []string
	agentOnlyMode   bool
}

type runResultMsg struct {
	blocks []sessionBlock
	quit   bool
}

func newAgentlineModel(ctx context.Context, root *redant.Command, prompt string, history []string, historyFile string, persist bool, initialArgv []string) *agentlineModel {
	ti := textinput.New()
	ti.Prompt = prompt
	styles := textinput.DefaultStyles(true)
	styles.Focused.Prompt = stylePrompt
	styles.Focused.Text = styleInputText
	styles.Blurred.Prompt = stylePrompt
	styles.Blurred.Text = styleInputText
	ti.SetStyles(styles)
	ti.Focus()
	ti.CharLimit = 0
	ti.SetValue("")

	if strings.TrimSpace(prompt) == "" {
		prompt = "agent> "
		ti.Prompt = prompt
	}

	agentOnlyMode := hasAnyAgentCommand(root)

	m := &agentlineModel{
		ctx:             ctx,
		root:            root,
		input:           ti,
		prompt:          prompt,
		history:         append([]string(nil), history...),
		historyPos:      len(history),
		historyFile:     historyFile,
		persistHistory:  persist,
		selectedHistory: -1,
		initialArgv:     append([]string(nil), initialArgv...),
		agentOnlyMode:   agentOnlyMode,
		blocks: []sessionBlock{{
			Kind:  blockKindSystem,
			Title: "system",
			Lines: []string{
				"agentline started. 默认输入可自动识别为 /ask 或 /run。",
				"试试：/ask 如何发布版本、/plan 实现 mcp 工具、/run commit --help",
				"快捷键：Tab 补全，↑/↓ 选择候选，Ctrl+O 切换输出滚动，Ctrl+C 退出。",
			},
		}},
	}
	if agentOnlyMode {
		m.blocks[0].Lines = append(m.blocks[0].Lines, "已检测到 agent command 元数据，仅自动识别这些命令为可执行命令输入。")
	}
	m.recomputeSuggestions()
	return m
}

func (m *agentlineModel) Init() tea.Cmd {
	if len(m.initialArgv) == 0 {
		return nil
	}

	request := formatCommandLine(m.root.Name(), m.initialArgv)
	return m.startCommandRun(request)
}

func (m *agentlineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case runResultMsg:
		m.running = false
		m.currentCancel = nil
		if len(msg.blocks) > 0 {
			m.appendBlocks(msg.blocks)
			m.outputOffset = 0
		}
		if msg.quit {
			return m, tea.Quit
		}
		m.normalizeOutputOffset()
		m.normalizeInputOffset()
		m.recomputeSuggestions()
		return m, nil

	case mouseScrollMsg:
		switch msg.Region {
		case mouseRegionInput:
			m.outputFocus = false
			m.scrollInputLines(msg.Delta)
		default:
			m.outputFocus = true
			m.scrollOutputLines(msg.Delta)
		}
		return m, nil

	case mouseSelectHistoryMsg:
		if msg.HistoryIndex < 0 || msg.HistoryIndex >= len(m.history) {
			return m, nil
		}
		m.outputFocus = false
		m.historyPos = msg.HistoryIndex
		m.selectedHistory = msg.HistoryIndex
		m.input.SetValue(m.history[msg.HistoryIndex])
		m.input.CursorEnd()
		m.recomputeSuggestions()
		m.normalizeInputOffset()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > len([]rune(m.prompt))+4 {
			m.input.SetWidth(m.width - len([]rune(m.prompt)) - 4)
		}
		m.normalizeOutputOffset()
		m.normalizeInputOffset()
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			if m.running {
				if m.cancelActiveRun("收到 Ctrl+C，正在尝试中断当前任务...") {
					return m, nil
				}
				m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "interrupt", Lines: []string{"当前任务不支持中断，等待完成中..."}})
				m.normalizeOutputOffset()
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+o":
			m.outputFocus = !m.outputFocus
			return m, nil
		case "esc":
			if m.outputFocus {
				m.outputFocus = false
				return m, nil
			}
			m.suggestions = nil
			m.selected = 0
			return m, nil
		}

		if m.outputFocus && len(m.suggestions) == 0 {
			switch key {
			case "up":
				m.scrollOutputLines(1)
				return m, nil
			case "down":
				m.scrollOutputLines(-1)
				return m, nil
			case "pgup":
				m.scrollOutputPage(1)
				return m, nil
			case "pgdown":
				m.scrollOutputPage(-1)
				return m, nil
			case "home":
				m.scrollOutputTop()
				return m, nil
			case "end":
				m.scrollOutputBottom()
				return m, nil
			}
		}

		switch key {
		case "tab":
			if strings.TrimSpace(m.input.Value()) == "" && len(m.suggestions) == 0 {
				m.suggestions = collectStarterSlashItems()
				m.suggestions = append(m.suggestions, collectCommandSlashItems(m.root, m.agentOnlyMode, "")...)
				m.suggestions = uniqueCompletionItems(m.suggestions)
				m.selected = 0
				return m, nil
			}
			m.applySuggestion()
			m.recomputeSuggestions()
			m.normalizeOutputOffset()
			return m, nil

		case "home":
			if len(m.suggestions) > 0 {
				m.selected = 0
				return m, nil
			}
			m.scrollOutputTop()
			return m, nil

		case "end":
			if len(m.suggestions) > 0 {
				m.selected = len(m.suggestions) - 1
				return m, nil
			}
			m.scrollOutputBottom()
			return m, nil

		case "pgup":
			if len(m.suggestions) > 0 {
				m.selected -= m.suggestionRows(len(m.suggestions))
				if m.selected < 0 {
					m.selected = 0
				}
				return m, nil
			}
			m.scrollOutputPage(1)
			return m, nil

		case "pgdown":
			if len(m.suggestions) > 0 {
				m.selected += m.suggestionRows(len(m.suggestions))
				if m.selected >= len(m.suggestions) {
					m.selected = len(m.suggestions) - 1
				}
				return m, nil
			}
			m.scrollOutputPage(-1)
			return m, nil

		case "up":
			if len(m.suggestions) > 0 {
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			}
			m.historyUp()
			m.recomputeSuggestions()
			m.normalizeInputOffset()
			return m, nil

		case "down":
			if len(m.suggestions) > 0 {
				if m.selected < len(m.suggestions)-1 {
					m.selected++
				}
				return m, nil
			}
			m.historyDown()
			m.recomputeSuggestions()
			m.normalizeInputOffset()
			return m, nil

		case "enter":
			if m.running {
				return m, nil
			}
			line := strings.TrimSpace(m.input.Value())
			if line == "" {
				return m, nil
			}

			m.appendHistory(line)
			m.input.SetValue("")
			m.historyPos = len(m.history)
			m.suggestions = nil
			m.selected = 0
			m.inputOffset = 0
			m.selectedHistory = -1

			if handled, cmd := m.handleSlashInput(line); handled {
				return m, cmd
			}

			if isCommandLikeInput(m.root, line, m.agentOnlyMode) {
				return m, m.startCommandRun(line)
			}
			m.running = true
			m.currentCancel = nil
			return m, runAskCmd(line)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.recomputeSuggestions()
	m.normalizeOutputOffset()
	m.normalizeInputOffset()
	return m, cmd
}

func (m *agentlineModel) View() tea.View {
	contentWidth := m.contentWidth()

	renderedOutput := m.renderOutputLines(contentWidth)
	outputRows := m.outputRows()
	outputOffset := clampOutputOffset(m.outputOffset, len(renderedOutput), outputRows)
	outputStart, outputEnd := visibleOutputRange(len(renderedOutput), outputRows, outputOffset)

	status := styleStatusIdle.Render("IDLE")
	if m.running {
		status = styleStatusBusy.Render("RUNNING")
	}
	focus := "INPUT"
	if m.outputFocus {
		focus = "OUTPUT"
	}

	lines := make([]string, 0, m.height+8)
	header := fmt.Sprintf("agentline · status=%s · focus=%s · blocks=%d · lines=%d", status, focus, len(m.blocks), len(renderedOutput))
	lines = append(lines, styleHeader.Render(truncateDisplayWidth(header, contentWidth)))

	outputTitle := fmt.Sprintf("输出区域（%d-%d/%d）", displayStart(outputStart, outputEnd), outputEnd, len(renderedOutput))
	lines = append(lines, styleHeader.Render(truncateDisplayWidth(outputTitle, contentWidth)))
	outputRegionStart := len(lines) - 1
	if len(renderedOutput) == 0 {
		lines = append(lines, styleHint.Render("暂无输出"))
	} else {
		for i := outputStart; i < outputEnd; i++ {
			lines = append(lines, renderedOutput[i])
		}
	}

	if len(m.suggestions) > 0 {
		rows := m.suggestionRows(len(m.suggestions))
		s, e := visibleSuggestionRange(len(m.suggestions), m.selected, rows)
		suggestionHeader := fmt.Sprintf("slash 候选（%d，显示 %d-%d）", len(m.suggestions), s+1, e)
		lines = append(lines, styleHeader.Render(truncateDisplayWidth(suggestionHeader, contentWidth)))

		suggestionWidth := contentWidth
		if suggestionWidth > 0 {
			suggestionWidth -= 2
		}

		for i := s; i < e; i++ {
			item := m.suggestions[i]
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			line := padRightDisplay(item.Insert, 18)
			raw := line
			if item.Description != "" {
				raw += " " + styleDesc.Render(item.Description)
			}
			raw = truncateDisplayWidth(raw, suggestionWidth)
			if i == m.selected {
				raw = styleSelected.Render(raw)
			}
			lines = append(lines, prefix+raw)
		}
		lines = append(lines, "  "+styleHint.Render("提示：↑/↓ 选择，Tab 应用，Esc 关闭候选"))
	}

	if m.running {
		lines = append(lines, styleRunning.Render(truncateDisplayWidth("执行中...", contentWidth)))
	}
	outputRegionEnd := len(lines) - 1

	inputTitle := "输入区域（历史请使用 /history 查看）"
	inputRegionStart := len(lines)
	lines = append(lines, styleHeader.Render(truncateDisplayWidth(inputTitle, contentWidth)))
	lines = append(lines, m.input.View())
	lines = append(lines, styleHint.Render(truncateDisplayWidth("命令：/ask /plan /run /history /cancel /fold /unfold；Ctrl+O 或 /output 开启滚轮滚动；/help 查看帮助", contentWidth)))
	inputRegionEnd := len(lines) - 1

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	if m.outputFocus {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.OnMouse = func(msg tea.MouseMsg) tea.Cmd {
		switch event := msg.(type) {
		case tea.MouseWheelMsg:
			mouse := event.Mouse()
			delta := 0
			switch mouse.Button {
			case tea.MouseWheelUp:
				delta = 1
			case tea.MouseWheelDown:
				delta = -1
			default:
				return nil
			}

			if mouse.Y < outputRegionStart || mouse.Y > inputRegionEnd {
				return nil
			}

			region := mouseRegionOutput
			if mouse.Y >= inputRegionStart && mouse.Y <= inputRegionEnd {
				region = mouseRegionInput
			} else if mouse.Y > outputRegionEnd {
				return nil
			}

			return func() tea.Msg {
				return mouseScrollMsg{Region: region, Delta: delta}
			}

		case tea.MouseClickMsg:
			return nil
		}

		return nil
	}

	return v
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

	switch cmd {
	case "help", "?":
		m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/help", Lines: slashHelpLines(m.root, m.agentOnlyMode)})

	case "a", "ask":
		if argText == "" {
			m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/ask", Lines: []string{"用法：/ask <问题>"}})
			return true, nil
		}
		m.running = true
		return true, runAskCmd(argText)

	case "p", "plan":
		if argText == "" {
			m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/plan", Lines: []string{"用法：/plan <目标>"}})
			return true, nil
		}
		m.running = true
		return true, runPlanCmd(argText)

	case "r", "run":
		if argText == "" {
			m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/run", Lines: []string{"用法：/run <command ...>"}})
			return true, nil
		}
		return true, m.startCommandRun(argText)

	case "cancel", "stop":
		if m.cancelActiveRun("收到 /cancel，正在尝试中断当前任务...") {
			return true, nil
		}
		m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/cancel", Lines: []string{"当前没有可中断的运行任务。"}})
		m.normalizeOutputOffset()
		return true, nil

	case "fold":
		if m.foldDetails {
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/fold", Lines: []string{"当前已是折叠状态。"}})
		} else {
			m.foldDetails = true
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/fold", Lines: []string{"已折叠 assistant/tool 详情。输入 /unfold 可恢复。"}})
		}
		m.normalizeOutputOffset()
		return true, nil

	case "unfold":
		if !m.foldDetails {
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/unfold", Lines: []string{"当前已是展开状态。"}})
		} else {
			m.foldDetails = false
			m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/unfold", Lines: []string{"已展开 assistant/tool 详情。"}})
		}
		m.normalizeOutputOffset()
		return true, nil

	case "o", "out", "output":
		m.outputFocus = true
		m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/output", Lines: []string{
			"已进入输出滚动模式。",
			"使用 ↑/↓ 单行滚动，PgUp/PgDn 翻页，Home/End 顶/底。",
			"输入 /input 返回普通输入模式。",
		}})

	case "i", "input":
		m.outputFocus = false
		m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/input", Lines: []string{"已返回输入模式。"}})

	case "top":
		m.scrollOutputTop()
	case "bottom", "end":
		m.scrollOutputBottom()
	case "up":
		m.scrollOutputLines(1)
	case "down":
		m.scrollOutputLines(-1)
	case "pgup":
		m.scrollOutputPage(1)
	case "pgdown":
		m.scrollOutputPage(-1)

	case "history", "his":
		limit := 20
		if argText != "" {
			n, err := strconv.Atoi(argText)
			if err != nil || n <= 0 {
				m.appendBlock(sessionBlock{Kind: blockKindError, Title: "/history", Lines: []string{"用法：/history [正整数]，例如 /history 50"}})
				m.normalizeOutputOffset()
				return true, nil
			}
			limit = n
		}

		m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "/history", Lines: m.historyPreviewLines(limit)})
		m.outputOffset = 0
		m.normalizeOutputOffset()
		return true, nil

	case "clear", "cls":
		m.blocks = []sessionBlock{{
			Kind:  blockKindSystem,
			Title: "system",
			Lines: []string{"输出历史已清空。", "输入 /help 查看可用命令。"},
		}}
		m.outputOffset = 0

	case "quit", "exit", "q":
		return true, tea.Quit

	default:
		if isCommandLikeInputWithAlias(m.root, cmdText, m.agentOnlyMode, false) {
			return true, m.startCommandRun(cmdText)
		}

		m.appendBlock(sessionBlock{Kind: blockKindError, Title: raw, Lines: []string{
			fmt.Sprintf("未知 slash 命令: %s", cmd),
			"可尝试 /run <command...>，或直接使用 /<command ...> 形式。",
			"输入 /help 查看可用命令。",
		}})
	}

	m.normalizeOutputOffset()
	return true, nil
}

func runAskCmd(text string) tea.Cmd {
	return func() tea.Msg {
		startedAt := time.Now()
		question := strings.TrimSpace(text)
		if question == "" {
			return runResultMsg{blocks: []sessionBlock{{Kind: blockKindError, Title: "/ask", Lines: []string{"问题不能为空"}}}}
		}

		reply := buildAssistantReply(question)
		duration := formatDuration(time.Since(startedAt))
		return runResultMsg{blocks: []sessionBlock{
			{Kind: blockKindUser, Title: "user", Lines: []string{question}},
			{Kind: blockKindAssistant, Title: "assistant.think", Lines: buildAskThinkingLines(question)},
			{Kind: blockKindTool, Title: "tool.placeholder", Lines: buildAskToolHintLines(question)},
			{Kind: blockKindAssistant, Title: "assistant", Lines: append([]string{fmt.Sprintf("duration: %s", duration)}, reply...)},
		}}
	}
}

func runPlanCmd(goal string) tea.Cmd {
	return func() tea.Msg {
		goal = strings.TrimSpace(goal)
		if goal == "" {
			return runResultMsg{blocks: []sessionBlock{{Kind: blockKindError, Title: "/plan", Lines: []string{"目标不能为空"}}}}
		}

		return runResultMsg{blocks: []sessionBlock{
			{Kind: blockKindUser, Title: "user(plan)", Lines: []string{goal}},
			{Kind: blockKindAssistant, Title: "assistant(plan)", Lines: buildPlanLines(goal)},
		}}
	}
}

func runSlashRunCmd(ctx context.Context, root *redant.Command, commandLine string) tea.Cmd {
	return func() tea.Msg {
		startedAt := time.Now()
		request := strings.TrimSpace(commandLine)
		blocks := []sessionBlock{{
			Kind:  blockKindTool,
			Title: "tool.run",
			Lines: []string{fmt.Sprintf("request: %s", request)},
		}}

		appendResult := func(status string, output []string, runErr error) runResultMsg {
			lines := make([]string, 0, len(output)+2)
			lines = append(lines, fmt.Sprintf("status: %s", status))
			lines = append(lines, fmt.Sprintf("duration: %s", formatDuration(time.Since(startedAt))))
			if len(output) == 0 {
				lines = append(lines, "(no output)")
			} else {
				lines = append(lines, output...)
			}
			resultBlocks := append(blocks, sessionBlock{Kind: blockKindResult, Title: "result", Lines: lines})
			if runErr != nil {
				resultBlocks = append(resultBlocks, sessionBlock{Kind: blockKindError, Title: "error", Lines: []string{fmt.Sprintf("%v", runErr)}})
			}
			return runResultMsg{blocks: resultBlocks}
		}

		args, parseErr := splitCommandLine(request)
		if parseErr != nil {
			blocks = append(blocks, sessionBlock{Kind: blockKindError, Title: "parse", Lines: []string{fmt.Sprintf("parse input failed: %v", parseErr)}})
			return appendResult("failed", nil, parseErr)
		}
		if len(args) == 0 {
			err := errors.New("empty command")
			blocks = append(blocks, sessionBlock{Kind: blockKindError, Title: "parse", Lines: []string{err.Error()}})
			return appendResult("failed", nil, err)
		}
		if args[0] == root.Name() {
			args = args[1:]
		}
		if len(args) == 0 {
			err := errors.New("missing target command after root name")
			blocks = append(blocks, sessionBlock{Kind: blockKindError, Title: "parse", Lines: []string{err.Error()}})
			return appendResult("failed", nil, err)
		}

		blocks = append(blocks, sessionBlock{Kind: blockKindTool, Title: "tool.parse", Lines: []string{
			fmt.Sprintf("argv: %v", args),
			fmt.Sprintf("argc: %d", len(args)),
		}})

		title := "$ " + formatCommandLine(root.Name(), args)
		blocks = append(blocks, sessionBlock{Kind: blockKindCommand, Title: title, Lines: []string{"dispatching..."}})

		stdout := bytes.NewBuffer(nil)
		stderr := bytes.NewBuffer(nil)

		runInv := root.Invoke(args...)
		runInv.Stdout = stdout
		runInv.Stderr = stderr
		runInv.Stdin = strings.NewReader("")

		runErr := runInv.WithContext(ctx).Run()
		status := runStatus(runErr)

		resultLines := make([]string, 0, 8)
		if out := strings.TrimSpace(stdout.String()); out != "" {
			resultLines = append(resultLines, strings.Split(out, "\n")...)
		}
		if out := strings.TrimSpace(stderr.String()); out != "" {
			resultLines = append(resultLines, strings.Split(out, "\n")...)
		}
		return appendResult(status, resultLines, runErr)
	}
}

func runStatus(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "failed"
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return d.String()
	}
	return d.Round(time.Millisecond).String()
}

func buildAssistantReply(question string) []string {
	q := strings.TrimSpace(question)
	if q == "" {
		return []string{"请提供一个更具体的问题，我就能更快给你可执行建议。"}
	}

	return []string{
		fmt.Sprintf("收到问题：%s", q),
		"建议下一步：",
		"1) 用 /plan 把目标拆成可验证步骤；",
		"2) 用 /run 执行关键命令验证现状；",
		"3) 迭代直到测试通过。",
	}
}

func buildAskThinkingLines(question string) []string {
	q := strings.TrimSpace(question)
	if q == "" {
		q = "（空问题）"
	}
	return []string{
		fmt.Sprintf("分析问题：%s", q),
		"拆解目标、约束与验收条件...",
		"匹配可执行路径：/plan 生成步骤，/run 验证命令链路。",
	}
}

func buildAskToolHintLines(question string) []string {
	q := strings.TrimSpace(question)
	if q == "" {
		q = "(empty)"
	}
	return []string{
		fmt.Sprintf("planned-tool: summarize-input(%q)", truncateDisplayWidth(q, 60)),
		"planned-tool: propose-plan",
		"planned-tool: suggest-command-checks",
	}
}

func buildPlanLines(goal string) []string {
	goal = strings.TrimSpace(goal)
	return []string{
		fmt.Sprintf("目标：%s", goal),
		"计划：",
		"1) 明确期望行为与验收条件；",
		"2) 定位相关代码与依赖边界；",
		"3) 设计最小改动并分步实施；",
		"4) 每步运行测试并修复回归；",
		"5) 更新文档/changelog 并复盘风险。",
	}
}

func isCommandLikeInput(root *redant.Command, line string, agentOnly bool) bool {
	return isCommandLikeInputWithAlias(root, line, agentOnly, true)
}

func isCommandLikeInputWithAlias(root *redant.Command, line string, agentOnly bool, allowAlias bool) bool {
	cmd, ok := resolveCommandLikeInput(root, line, allowAlias)
	if !ok {
		return false
	}

	if !agentOnly {
		return true
	}

	return agentlinemodule.IsAgentCommand(cmd.Metadata)
}

func hasAnyAgentCommand(root *redant.Command) bool {
	if root == nil {
		return false
	}

	for _, child := range root.Children {
		if agentlinemodule.IsAgentCommand(child.Metadata) {
			return true
		}
		if hasAnyAgentCommand(child) {
			return true
		}
	}

	return false
}

func resolveCommandLikeInput(root *redant.Command, line string, allowAlias bool) (*redant.Command, bool) {
	args, err := splitCommandLine(line)
	if err != nil || len(args) == 0 || root == nil {
		return nil, false
	}

	if args[0] == root.Name() {
		args = args[1:]
		if len(args) == 0 {
			return nil, false
		}
	}

	current := root
	consumed := 0
	for _, token := range args {
		if strings.HasPrefix(token, "-") || strings.HasPrefix(token, "/") || strings.Contains(token, "=") {
			break
		}

		if strings.Contains(token, ":") {
			parts := strings.Split(token, ":")
			for _, part := range parts {
				next := childByToken(current, part, allowAlias)
				if next == nil {
					if consumed == 0 {
						return nil, false
					}
					return current, true
				}
				current = next
				consumed++
			}
			continue
		}

		next := childByToken(current, token, allowAlias)
		if next == nil {
			break
		}
		current = next
		consumed++
	}

	if consumed == 0 {
		return nil, false
	}

	return current, true
}

func childByToken(parent *redant.Command, token string, allowAlias bool) *redant.Command {
	if allowAlias {
		return childByNameOrAlias(parent, token)
	}
	return childByName(parent, token)
}

func childByName(parent *redant.Command, token string) *redant.Command {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Children {
		if child.Hidden {
			continue
		}
		if child.Name() == token {
			return child
		}
	}
	return nil
}

func childByNameOrAlias(parent *redant.Command, token string) *redant.Command {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Children {
		if child.Hidden {
			continue
		}
		if child.Name() == token {
			return child
		}
		for _, alias := range child.Aliases {
			if strings.TrimSpace(alias) == token {
				return child
			}
		}
	}
	return nil
}

func (m *agentlineModel) recomputeSuggestions() {
	line := m.input.Value()
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if strings.TrimSpace(line) == "" {
		m.suggestions = nil
		m.selected = 0
		return
	}

	if strings.HasPrefix(trimmed, "/") {
		m.suggestions = collectSlashCompletionItems(m.root, line, m.agentOnlyMode)
	} else {
		m.suggestions = nil
	}

	if len(m.suggestions) == 0 {
		m.selected = 0
		return
	}
	if m.selected >= len(m.suggestions) {
		m.selected = len(m.suggestions) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func collectStarterSlashItems() []completionItem {
	return uniqueCompletionItems([]completionItem{
		{Insert: "/ask ", Description: "提问（user -> assistant）"},
		{Insert: "/plan ", Description: "生成步骤计划"},
		{Insert: "/run ", Description: "执行命令"},
		{Insert: "/help", Description: "查看 slash 帮助"},
		{Insert: "/output", Description: "进入输出滚动"},
	})
}

func collectSlashCompletionItems(root *redant.Command, input string, agentOnly bool) []completionItem {
	trimmedRight := strings.TrimRightFunc(input, unicode.IsSpace)
	if trimmedRight == "" {
		return nil
	}

	fields := strings.Fields(trimmedRight)
	if len(fields) == 0 {
		return nil
	}
	first := fields[0]
	if !strings.HasPrefix(first, "/") {
		return nil
	}

	if len(fields) > 1 || len(trimmedRight) < len(input) {
		probeTokens := append([]string{strings.TrimPrefix(first, "/")}, fields[1:]...)
		probeLine := strings.TrimSpace(strings.Join(probeTokens, " "))
		cmd, ok := resolveCommandLikeInput(root, probeLine, false)

		// 命令还未解析成功时，继续给出命令名补全。
		if !ok {
			return collectSlashNameSuggestions(root, agentOnly, strings.TrimPrefix(first, "/"))
		}

		// 场景：/commit <TAB>
		if len(fields) == 1 && len(trimmedRight) < len(input) {
			return collectCommandFlagItems(cmd, "")
		}

		// 场景：/commit --m<TAB>
		last := fields[len(fields)-1]
		if strings.HasPrefix(last, "-") {
			return collectCommandFlagItems(cmd, last)
		}

		// 场景：/commit --message hi <TAB>
		if len(trimmedRight) < len(input) {
			return collectCommandFlagItems(cmd, "")
		}

		return nil
	}

	return collectSlashNameSuggestions(root, agentOnly, strings.TrimPrefix(first, "/"))
}

func collectSlashNameSuggestions(root *redant.Command, agentOnly bool, prefix string) []completionItem {
	prefix = strings.TrimSpace(prefix)
	out := make([]completionItem, 0, len(slashCommands)+8)
	addCandidate := func(name, desc string) {
		candidate := "/" + strings.TrimSpace(name)
		if candidate == "/" {
			return
		}
		if prefix == "" || strings.HasPrefix(strings.TrimPrefix(candidate, "/"), prefix) {
			out = append(out, completionItem{Insert: candidate, Description: desc})
		}
	}

	for _, sc := range slashCommands {
		addCandidate(sc.Name, sc.Description)
	}

	out = append(out, collectCommandSlashItems(root, agentOnly, prefix)...)

	return uniqueCompletionItems(out)
}

func collectCommandFlagItems(cmd *redant.Command, prefix string) []completionItem {
	if cmd == nil {
		return nil
	}

	prefix = strings.TrimSpace(prefix)
	var out []completionItem
	for _, opt := range cmd.FullOptions() {
		if opt.Hidden || strings.TrimSpace(opt.Flag) == "" {
			continue
		}
		flagName := "--" + strings.TrimSpace(opt.Flag)
		if prefix != "" && !strings.HasPrefix(flagName, prefix) {
			continue
		}
		desc := strings.TrimSpace(opt.Description)
		if desc == "" {
			desc = "命令参数"
		}
		out = append(out, completionItem{Insert: flagName + " ", Description: desc})
	}

	return uniqueCompletionItems(out)
}

func collectCommandSlashItems(root *redant.Command, agentOnly bool, prefix string) []completionItem {
	if root == nil {
		return nil
	}

	prefix = strings.TrimSpace(prefix)
	var out []completionItem

	var walk func(parent *redant.Command, path []string)
	walk = func(parent *redant.Command, path []string) {
		if parent == nil {
			return
		}

		for _, child := range parent.Children {
			if child == nil || child.Hidden {
				continue
			}
			if child.Name() == agentlinemodule.CommandName {
				continue
			}

			cmdPath := append(path, child.Name())

			if !agentOnly || agentlinemodule.IsAgentCommand(child.Metadata) {
				pathText := strings.Join(cmdPath, " ")
				if prefix == "" || strings.HasPrefix(pathText, prefix) {
					desc := strings.TrimSpace(child.Short)
					if desc == "" {
						desc = "执行命令"
					}
					out = append(out, completionItem{
						Insert:      "/" + pathText + " ",
						Description: desc,
					})
				}
			}

			walk(child, cmdPath)
		}
	}

	walk(root, nil)
	return out
}

func (m *agentlineModel) applySuggestion() {
	if len(m.suggestions) == 0 {
		m.recomputeSuggestions()
		if len(m.suggestions) == 0 {
			return
		}
	}

	idx := m.selected
	if idx < 0 || idx >= len(m.suggestions) {
		idx = 0
	}

	newLine := applySelectedCompletion(m.input.Value(), m.suggestions[idx].Insert)
	m.input.SetValue(newLine)
	m.input.CursorEnd()
}

func applySelectedCompletion(input, selected string) string {
	trimmedRight := strings.TrimRightFunc(input, unicode.IsSpace)
	if trimmedRight == "" {
		if strings.HasSuffix(selected, " ") {
			return selected
		}
		return selected + " "
	}
	if len(trimmedRight) < len(input) {
		if strings.HasSuffix(selected, " ") {
			return trimmedRight + " " + selected
		}
		return trimmedRight + " " + selected + " "
	}
	idx := strings.LastIndexFunc(trimmedRight, unicode.IsSpace)
	if idx < 0 {
		if strings.HasSuffix(selected, " ") {
			return selected
		}
		return selected + " "
	}
	if strings.HasSuffix(selected, " ") {
		return trimmedRight[:idx+1] + selected
	}
	return trimmedRight[:idx+1] + selected + " "
}

func (m *agentlineModel) scrollOutputLines(delta int) {
	total := len(m.renderOutputLines(m.contentWidth()))
	rows := m.outputRows()
	m.outputOffset = clampOutputOffset(m.outputOffset+delta, total, rows)
}

func (m *agentlineModel) scrollInputLines(delta int) {
	total := len(m.renderInputHistoryLines(m.contentWidth()))
	rows := m.inputRows()
	m.inputOffset = clampInputOffset(m.inputOffset+delta, total, rows)
}

func (m *agentlineModel) scrollOutputPage(deltaPage int) {
	if deltaPage == 0 {
		return
	}
	rows := m.outputRows()
	m.scrollOutputLines(deltaPage * rows)
}

func (m *agentlineModel) scrollOutputTop() {
	total := len(m.renderOutputLines(m.contentWidth()))
	rows := m.outputRows()
	maxOffset := total - rows
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.outputOffset = maxOffset
}

func (m *agentlineModel) scrollOutputBottom() {
	m.outputOffset = 0
}

func (m *agentlineModel) normalizeOutputOffset() {
	total := len(m.renderOutputLines(m.contentWidth()))
	m.outputOffset = clampOutputOffset(m.outputOffset, total, m.outputRows())
}

func (m *agentlineModel) normalizeInputOffset() {
	total := len(m.renderInputHistoryLines(m.contentWidth()))
	m.inputOffset = clampInputOffset(m.inputOffset, total, m.inputRows())
}

func (m *agentlineModel) renderOutputLines(width int) []string {
	if len(m.blocks) == 0 {
		return nil
	}

	out := make([]string, 0, len(m.blocks)*3)
	for i, block := range m.blocks {
		title := strings.TrimSpace(block.Title)
		if title == "" {
			title = string(block.Kind)
		}

		head := fmt.Sprintf("■ #%d [%s] %s", i+1, strings.ToUpper(string(block.Kind)), title)
		out = append(out, renderBlockHeader(block.Kind, truncateDisplayWidth(head, width)))

		linesToRender := block.Lines
		if m.foldDetails && (block.Kind == blockKindAssistant || block.Kind == blockKindTool) && len(block.Lines) > 1 {
			linesToRender = []string{
				block.Lines[0],
				fmt.Sprintf("... (%d more lines folded)", len(block.Lines)-1),
			}
		}

		if len(linesToRender) == 0 {
			out = append(out, "  (no output)")
		} else {
			for _, line := range linesToRender {
				wrapped := wrapDisplayWidth(line, width-2)
				if len(wrapped) == 0 {
					continue
				}
				for _, w := range wrapped {
					out = append(out, "  "+w)
				}
			}
		}

		if i < len(m.blocks)-1 {
			sep := "────────────────"
			if width > 0 {
				sep = strings.Repeat("─", width)
			}
			out = append(out, sep)
		}
	}

	return out
}

func (m *agentlineModel) renderInputHistoryLines(width int) []string {
	lines, _ := m.renderInputHistoryLinesWithIndices(width)
	return lines
}

func (m *agentlineModel) renderInputHistoryLinesWithIndices(width int) ([]string, []int) {
	if len(m.history) == 0 {
		return nil, nil
	}

	historyWidth := width
	if historyWidth > 0 {
		historyWidth -= 2
	}
	out := make([]string, 0, len(m.history))
	indices := make([]int, 0, len(m.history))
	for i, line := range m.history {
		entry := fmt.Sprintf("%03d %s", i+1, strings.TrimSpace(line))
		wrapped := wrapDisplayWidth(entry, historyWidth)
		if len(wrapped) == 0 {
			continue
		}
		for _, w := range wrapped {
			out = append(out, w)
			indices = append(indices, i)
		}
	}
	return out, indices
}

func renderBlockHeader(kind blockKind, text string) string {
	switch kind {
	case blockKindSystem:
		return styleKindSystem.Render(text)
	case blockKindUser:
		return styleKindUser.Render(text)
	case blockKindAssistant:
		return styleKindAssistant.Render(text)
	case blockKindTool:
		return styleKindTool.Render(text)
	case blockKindCommand:
		return styleKindCommand.Render(text)
	case blockKindResult:
		return styleKindResult.Render(text)
	case blockKindError:
		return styleKindError.Render(text)
	default:
		return text
	}
}

func (m *agentlineModel) appendBlocks(blocks []sessionBlock) {
	for _, block := range blocks {
		m.appendBlock(block)
	}
}

func (m *agentlineModel) appendBlock(block sessionBlock) {
	title := strings.TrimSpace(block.Title)
	if title == "" {
		title = string(block.Kind)
	}

	kind := block.Kind
	if kind == "" {
		kind = blockKindSystem
	}

	normalized := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		for _, s := range normalizeOutputLines(line) {
			normalized = append(normalized, s)
		}
	}

	m.blocks = append(m.blocks, sessionBlock{Kind: kind, Title: title, Lines: normalized})
	m.trimOutputHistory()
}

func (m *agentlineModel) trimOutputHistory() {
	if len(m.blocks) == 0 {
		return
	}

	total := 0
	for _, b := range m.blocks {
		total += len(b.Lines)
	}

	for len(m.blocks) > 1 && (len(m.blocks) > maxOutputBlocks || total > maxOutputLines) {
		total -= len(m.blocks[0].Lines)
		m.blocks = m.blocks[1:]
	}
}

func (m *agentlineModel) appendHistory(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == line {
		return
	}
	m.history = append(m.history, line)
	if m.persistHistory && m.historyFile != "" {
		_ = appendHistoryLine(m.historyFile, line)
	}
}

func (m *agentlineModel) historyUp() {
	if len(m.history) == 0 {
		return
	}
	if m.historyPos <= 0 {
		m.historyPos = 0
		m.selectedHistory = m.historyPos
		m.input.SetValue(m.history[m.historyPos])
		m.input.CursorEnd()
		return
	}
	m.historyPos--
	m.selectedHistory = m.historyPos
	m.input.SetValue(m.history[m.historyPos])
	m.input.CursorEnd()
}

func (m *agentlineModel) historyDown() {
	if len(m.history) == 0 {
		return
	}
	if m.historyPos >= len(m.history)-1 {
		m.historyPos = len(m.history)
		m.selectedHistory = -1
		m.input.SetValue("")
		m.input.CursorEnd()
		return
	}
	m.historyPos++
	m.selectedHistory = m.historyPos
	m.input.SetValue(m.history[m.historyPos])
	m.input.CursorEnd()
}

func (m *agentlineModel) contentWidth() int {
	if m.width <= 0 {
		return 0
	}
	w := m.width - 1
	if w < 1 {
		return 1
	}
	return w
}

func (m *agentlineModel) suggestionRows(total int) int {
	if total <= 0 {
		return 1
	}

	rows := defaultSuggestionRows
	if m.height <= 0 {
		if total < rows {
			return total
		}
		return rows
	}

	available := m.height - m.baseOccupiedRows(true) - minOutputRows
	if available < 1 {
		available = 1
	}
	if available > rows {
		available = rows
	}
	if available > total {
		available = total
	}
	return available
}

func (m *agentlineModel) inputRows() int {
	if m.height <= 0 {
		return defaultInputRows
	}

	rows := defaultInputRows
	maxRows := m.height / 3
	if maxRows < 1 {
		maxRows = 1
	}
	if rows > maxRows {
		rows = maxRows
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *agentlineModel) outputRows() int {
	if m.height <= 0 {
		return defaultOutputRows
	}

	occupied := m.baseOccupiedRows(false)
	if len(m.suggestions) > 0 {
		occupied += 3 + m.suggestionRows(len(m.suggestions))
	}

	rows := m.height - occupied
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *agentlineModel) baseOccupiedRows(withSuggestionFrame bool) int {
	rows := 5
	if m.running {
		rows++
	}
	if withSuggestionFrame {
		rows += 2
	}
	return rows
}

func displayStart(start, end int) int {
	if end == 0 {
		return 0
	}
	return start + 1
}

func visibleSuggestionRange(total, selected, maxRows int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if maxRows <= 0 {
		maxRows = 1
	}
	if total <= maxRows {
		return 0, total
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}

	start = selected - maxRows/2
	if start < 0 {
		start = 0
	}
	if start+maxRows > total {
		start = total - maxRows
	}
	end = start + maxRows
	return start, end
}

func visibleOutputRange(total, rows, offset int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if rows <= 0 {
		rows = 1
	}
	if total <= rows {
		return 0, total
	}

	offset = clampOutputOffset(offset, total, rows)
	end = total - offset
	start = end - rows
	return start, end
}

func visibleInputRange(total, rows, offset int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if rows <= 0 {
		rows = 1
	}
	if total <= rows {
		return 0, total
	}

	offset = clampInputOffset(offset, total, rows)
	end = total - offset
	start = end - rows
	return start, end
}

func clampOutputOffset(offset, total, rows int) int {
	if offset < 0 {
		offset = 0
	}
	if total <= 0 {
		return 0
	}
	if rows <= 0 {
		rows = 1
	}
	maxOffset := total - rows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func clampInputOffset(offset, total, rows int) int {
	if offset < 0 {
		offset = 0
	}
	if total <= 0 {
		return 0
	}
	if rows <= 0 {
		rows = 1
	}
	maxOffset := total - rows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func uniqueCompletionItems(items []completionItem) []completionItem {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]completionItem, 0, len(items))
	for _, item := range items {
		ins := strings.TrimSpace(item.Insert)
		if ins == "" {
			continue
		}
		if _, ok := seen[ins]; ok {
			continue
		}
		seen[ins] = struct{}{}
		item.Insert = ins
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Insert != out[j].Insert {
			return out[i].Insert < out[j].Insert
		}
		return out[i].Description < out[j].Description
	})
	return out
}

func padRightDisplay(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncateDisplayWidth(s, width)
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateDisplayWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)
	if maxWidth <= ellipsisWidth {
		return ellipsis
	}

	target := maxWidth - ellipsisWidth
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > target {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + ellipsis
}

func wrapDisplayWidth(s string, maxWidth int) []string {
	s = strings.ReplaceAll(s, "\t", "    ")
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return []string{s}
	}

	var lines []string
	var cur strings.Builder
	curWidth := 0

	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curWidth = 0
	}

	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if rw <= 0 {
			rw = 1
		}
		if curWidth > 0 && curWidth+rw > maxWidth {
			flush()
		}
		cur.WriteRune(r)
		curWidth += rw
	}
	if cur.Len() > 0 {
		flush()
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func normalizeOutputLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.Contains(part, "\r") {
			seg := strings.Split(part, "\r")
			part = seg[len(seg)-1]
		}
		part = strings.TrimRight(part, "\r")
		out = append(out, part)
	}
	return out
}

func slashHelpLines(root *redant.Command, agentOnly bool) []string {
	lines := []string{
		"slash commands:",
		"  /<command ...>: 直接执行命令（例如 /commit --message hi）",
		"  /ask <question>: 追加 user/assistant 对话块",
		"  /plan <goal>: 生成分步骤计划块",
		"  /run <command...>: 执行命令并输出 tool/command/result",
		"  /history [N]: 查看最近输入历史（默认 20 条）",
		"  /cancel: 中断当前运行中的任务",
		"  /fold: 折叠 assistant/tool 详情块",
		"  /unfold: 展开 assistant/tool 详情块",
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

func (m *agentlineModel) historyPreviewLines(limit int) []string {
	if len(m.history) == 0 {
		return []string{"暂无输入历史。"}
	}

	if limit <= 0 {
		limit = 20
	}

	start := len(m.history) - limit
	if start < 0 {
		start = 0
	}

	lines := make([]string, 0, len(m.history)-start+1)
	lines = append(lines, fmt.Sprintf("total: %d, showing: %d-%d", len(m.history), start+1, len(m.history)))
	for i := start; i < len(m.history); i++ {
		lines = append(lines, fmt.Sprintf("%03d %s", i+1, strings.TrimSpace(m.history[i])))
	}
	return lines
}

func (m *agentlineModel) startCommandRun(commandLine string) tea.Cmd {
	runCtx := m.ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	runCtx, cancel := context.WithCancel(runCtx)
	m.running = true
	m.currentCancel = cancel
	m.outputFocus = false
	return runSlashRunCmd(runCtx, m.root, commandLine)
}

func (m *agentlineModel) cancelActiveRun(message string) bool {
	if !m.running || m.currentCancel == nil {
		return false
	}
	cancel := m.currentCancel
	m.currentCancel = nil
	cancel()
	if strings.TrimSpace(message) != "" {
		m.appendBlock(sessionBlock{Kind: blockKindSystem, Title: "cancel", Lines: []string{message}})
	}
	return true
}

func loadHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	out := make([]string, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func appendHistoryLine(path, line string) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(line) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintln(f, line)
	return err
}

func splitCommandLine(input string) ([]string, error) {
	var (
		out     []string
		cur     strings.Builder
		quote   rune
		escaped bool
	)
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, cur.String())
		cur.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
		}
	}

	if escaped {
		return nil, errors.New("unfinished escape sequence")
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	flush()
	return out, nil
}

func formatCommandLine(program string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteShellArg(program))
	for _, arg := range args {
		parts = append(parts, quoteShellArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteShellArg(s string) string {
	if s == "" {
		return `""`
	}
	if !needsQuote(s) {
		return s
	}
	return strconv.Quote(s)
}

func needsQuote(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
		switch r {
		case '"', '\'', '\\', '$', '`', '|', '&', ';', '(', ')', '<', '>', '*', '?', '[', ']', '{', '}', '!':
			return true
		}
	}
	return false
}
