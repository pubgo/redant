package richlinecmd

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
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/pflag"

	"github.com/pubgo/redant"
)

const (
	defaultSuggestionRows = 10
	defaultOutputRows     = 20
	minOutputRows         = 6
	maxOutputBlocks       = 300
	maxLogLines           = 2000
)

var (
	stylePrompt      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleInputText   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleHeader      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleHint        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleDescription = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleRunning     = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	styleSelectedRow = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true)
	styleBlockHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)

	styleKindCommand = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	styleKindFlag    = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	styleKindArg     = lipgloss.NewStyle().Foreground(lipgloss.Color("150")).Bold(true)
	styleKindEnum    = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Bold(true)
	styleKindDefault = lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Bold(true)
)

type completionItem struct {
	Insert      string
	Description string
	Kind        completionKind
}

type completionKind string

const (
	completionKindCommand completionKind = "command"
	completionKindFlag    completionKind = "flag"
	completionKindArg     completionKind = "arg"
	completionKindEnum    completionKind = "enum"
)

type outputBlock struct {
	Title string
	Lines []string
}

type slashCommand struct {
	Name        string
	Aliases     []string
	Description string
}

var slashCommands = []slashCommand{
	{Name: "help", Aliases: []string{"?"}, Description: "显示 slash 命令帮助"},
	{Name: "output", Aliases: []string{"o", "out"}, Description: "进入输出滚动模式"},
	{Name: "input", Aliases: []string{"i"}, Description: "返回输入模式"},
	{Name: "top", Description: "跳到输出历史顶部"},
	{Name: "bottom", Aliases: []string{"end"}, Description: "跳到输出历史底部"},
	{Name: "up", Description: "输出按行向上滚动"},
	{Name: "down", Description: "输出按行向下滚动"},
	{Name: "pgup", Description: "输出按页向上滚动"},
	{Name: "pgdown", Description: "输出按页向下滚动"},
	{Name: "quit", Aliases: []string{"exit", "q"}, Description: "退出 richline"},
}

func New() *redant.Command {
	var (
		prompt    string
		history   string
		noHistory bool
	)

	return &redant.Command{
		Use:   "richline",
		Short: "基于 Bubble Tea 的交互命令行（竖向补全列表）",
		Long:  "启动交互式 richline，使用 Bubble Tea 提供竖向补全候选和描述信息展示。",
		Options: redant.OptionSet{
			{Flag: "prompt", Description: "交互提示符", Value: redant.StringOf(&prompt), Default: "richline> "},
			{Flag: "history-file", Description: "历史记录文件路径（为空自动使用 ~/.redant_richline_history）", Value: redant.StringOf(&history)},
			{Flag: "no-history", Description: "禁用历史记录持久化", Value: redant.BoolOf(&noHistory)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			historyFile := strings.TrimSpace(history)
			if historyFile == "" && !noHistory {
				if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
					historyFile = filepath.Join(home, ".redant_richline_history")
				}
			}

			historyLines := []string{}
			if !noHistory && historyFile != "" {
				historyLines = loadHistory(historyFile)
			}

			model := newRichlineModel(ctx, root, prompt, historyLines, historyFile, !noHistory)
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
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func AddRichlineCommand(rootCmd *redant.Command) {
	rootCmd.Children = append(rootCmd.Children, New())
}

type richlineModel struct {
	ctx            context.Context
	root           *redant.Command
	input          textinput.Model
	prompt         string
	history        []string
	historyPos     int
	historyFile    string
	persistHistory bool
	blocks         []outputBlock
	suggestions    []completionItem
	selected       int
	running        bool
	width          int
	height         int
	outputOffset   int
	outputFocus    bool
	starterPinned  bool
}

type runLineResultMsg struct {
	block outputBlock
	quit  bool
}

func newRichlineModel(ctx context.Context, root *redant.Command, prompt string, history []string, historyFile string, persist bool) *richlineModel {
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
		prompt = "richline> "
		ti.Prompt = prompt
	}

	m := &richlineModel{
		ctx:            ctx,
		root:           root,
		input:          ti,
		prompt:         prompt,
		history:        append([]string(nil), history...),
		historyPos:     len(history),
		historyFile:    historyFile,
		persistHistory: persist,
		blocks: []outputBlock{{
			Title: "system",
			Lines: []string{"richline mode started, TAB 查看补全，↑/↓ 选择候选，Enter 执行，Ctrl+C 退出。"},
		}},
	}
	m.recomputeSuggestions()
	return m
}

func (m *richlineModel) Init() tea.Cmd { return nil }

func (m *richlineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case runLineResultMsg:
		m.running = false
		if strings.TrimSpace(msg.block.Title) != "" || len(msg.block.Lines) > 0 {
			m.appendBlock(msg.block)
			m.outputOffset = 0
		}
		if msg.quit {
			return m, tea.Quit
		}
		m.recomputeSuggestions()
		m.normalizeOutputOffset()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > len([]rune(m.prompt))+4 {
			m.input.SetWidth(m.width - len([]rune(m.prompt)) - 4)
		}
		m.normalizeOutputOffset()
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
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
			m.starterPinned = false
			return m, nil
		}

		if m.outputFocus {
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
				m.suggestions = collectStarterCompletionItems(m.root)
				m.selected = 0
				m.starterPinned = true
				m.normalizeOutputOffset()
				return m, nil
			}
			m.starterPinned = false
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
			return m, nil
		case "enter":
			if m.running {
				return m, nil
			}
			line := strings.TrimSpace(m.input.Value())
			if line == "" {
				return m, nil
			}
			m.starterPinned = false
			m.appendHistory(line)
			m.input.SetValue("")
			m.historyPos = len(m.history)
			m.suggestions = nil
			m.selected = 0
			if handled, cmd := m.handleSlashCommand(line); handled {
				return m, cmd
			}
			m.running = true
			m.outputFocus = false
			return m, runLineCmd(m.ctx, m.root, line)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.recomputeSuggestions()
	m.normalizeOutputOffset()
	return m, cmd
}

func (m *richlineModel) View() tea.View {
	var b strings.Builder
	contentWidth := m.contentWidth()
	renderedLines := m.renderOutputLines(contentWidth)
	outputRows := m.outputRows()
	offset := clampOutputOffset(m.outputOffset, len(renderedLines), outputRows)
	start, end := visibleOutputRange(len(renderedLines), outputRows, offset)

	mode := "输入模式"
	if m.outputFocus {
		mode = "输出滚动"
	}
	header := fmt.Sprintf("输出历史（%d 行，%d 块，显示 %d-%d，%s）", len(renderedLines), len(m.blocks), displayStart(start, end), end, mode)
	b.WriteString(styleHeader.Render(truncateDisplayWidth(header, contentWidth)))
	b.WriteByte('\n')

	if len(renderedLines) == 0 {
		b.WriteString(styleHint.Render("暂无输出"))
		b.WriteByte('\n')
	} else {
		for i := start; i < end; i++ {
			line := renderedLines[i]
			b.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				b.WriteByte('\n')
			}
		}
	}

	if len(m.suggestions) > 0 {
		rows := m.suggestionRows(len(m.suggestions))
		start, end := visibleSuggestionRange(len(m.suggestions), m.selected, rows)
		b.WriteString("\n")
		header := fmt.Sprintf("补全候选（%d，显示 %d-%d）：", len(m.suggestions), start+1, end)
		b.WriteString(styleHeader.Render(truncateDisplayWidth(header, contentWidth)))
		b.WriteByte('\n')
		suggestionWidth := contentWidth
		if suggestionWidth > 0 {
			suggestionWidth -= 2 // 前缀 "  " / "> "
		}
		for i := start; i < end; i++ {
			item := m.suggestions[i]
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			b.WriteString(prefix)
			b.WriteString(renderSuggestionLine(item, i == m.selected, suggestionWidth))
			b.WriteByte('\n')
		}
		b.WriteString("  ")
		hint := "提示：↑/↓ 选择，PgUp/PgDn 翻页；Tab 空输入可列出命令；Esc 隐藏候选"
		b.WriteString(styleHint.Render(truncateDisplayWidth(hint, suggestionWidth)))
		b.WriteByte('\n')
	}

	if m.running {
		b.WriteString("\n")
		b.WriteString(styleRunning.Render(truncateDisplayWidth("执行中...", contentWidth)))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(m.input.View())
	b.WriteByte('\n')
	b.WriteString(styleHint.Render(truncateDisplayWidth("输出历史：PgUp/PgDn 翻页，Home/End 顶/底；Ctrl+O 或 /output 进入精细滚动；/help 查看 slash 命令", contentWidth)))
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m *richlineModel) handleSlashCommand(line string) (bool, tea.Cmd) {
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

	switch cmd {
	case "help", "?":
		m.appendBlock(outputBlock{Title: "/help", Lines: slashHelpLines()})
	case "o", "out", "output":
		m.outputFocus = true
		m.appendBlock(outputBlock{Title: "/output", Lines: []string{
			"已进入输出滚动模式。",
			"使用 ↑/↓ 单行滚动，PgUp/PgDn 翻页，Home/End 顶/底。",
			"输入 /input 返回普通输入模式。",
		}})
	case "i", "input":
		m.outputFocus = false
		m.appendBlock(outputBlock{Title: "/input", Lines: []string{"已返回输入模式。"}})
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
	case "quit", "exit", "q":
		return true, tea.Quit
	default:
		m.appendBlock(outputBlock{Title: raw, Lines: []string{
			fmt.Sprintf("未知 slash 命令: %s", cmd),
			"输入 /help 查看可用命令。",
		}})
	}

	m.normalizeOutputOffset()
	return true, nil
}

func displayStart(start, end int) int {
	if end == 0 {
		return 0
	}
	return start + 1
}

func (m *richlineModel) contentWidth() int {
	if m.width <= 0 {
		return 0
	}
	w := m.width - 1
	if w < 1 {
		return 1
	}
	return w
}

func (m *richlineModel) suggestionRows(total int) int {
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

func (m *richlineModel) outputRows() int {
	if m.height <= 0 {
		return defaultOutputRows
	}

	occupied := m.baseOccupiedRows(false)
	if len(m.suggestions) > 0 {
		occupied += 3 + m.suggestionRows(len(m.suggestions)) // 候选块：空行 + 标题 + 提示 + 数据行
	}

	rows := m.height - occupied
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *richlineModel) baseOccupiedRows(withSuggestionFrame bool) int {
	rows := 3 // 输出头部 + 输入框前空行 + 输入框
	if m.running {
		rows += 2 // 运行态：空行 + 提示
	}
	if withSuggestionFrame {
		rows += 3 // 候选块：空行 + 标题 + 提示
	}
	return rows
}

func (m *richlineModel) recomputeSuggestions() {
	line := m.input.Value()
	if strings.TrimSpace(line) == "" {
		if m.starterPinned && len(m.suggestions) > 0 {
			if m.selected >= len(m.suggestions) {
				m.selected = len(m.suggestions) - 1
			}
			if m.selected < 0 {
				m.selected = 0
			}
			return
		}
		m.suggestions = nil
		m.selected = 0
		m.starterPinned = false
		return
	}
	m.starterPinned = false

	if strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "/") {
		m.suggestions = collectSlashCompletionItems(line)
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
		return
	}

	items := collectCompletionItems(m.root, line)
	m.suggestions = items
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

func collectSlashCompletionItems(input string) []completionItem {
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
		return nil
	}

	prefix := strings.TrimPrefix(first, "/")
	out := make([]completionItem, 0, len(slashCommands)*2)
	addCandidate := func(name, desc string) {
		cand := "/" + strings.TrimSpace(name)
		if cand == "/" {
			return
		}
		if prefix == "" || strings.HasPrefix(strings.TrimPrefix(cand, "/"), prefix) {
			out = append(out, completionItem{Insert: cand, Description: "slash · " + desc, Kind: completionKindCommand})
		}
	}

	for _, cmd := range slashCommands {
		addCandidate(cmd.Name, cmd.Description)
		for _, alias := range cmd.Aliases {
			addCandidate(alias, fmt.Sprintf("%s（%s 的别名）", cmd.Description, cmd.Name))
		}
	}

	return uniqueCompletionItems(out)
}

func collectStarterCompletionItems(root *redant.Command) []completionItem {
	if root == nil {
		return nil
	}
	items := uniqueCompletionItems(suggestChildrenItems(root, ""))
	if len(items) > 0 {
		return items
	}
	return collectCompletionItems(root, "")
}

func (m *richlineModel) applySuggestion() {
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

func (m *richlineModel) scrollOutputLines(delta int) {
	total := len(m.renderOutputLines(m.contentWidth()))
	rows := m.outputRows()
	m.outputOffset = clampOutputOffset(m.outputOffset+delta, total, rows)
}

func (m *richlineModel) scrollOutputPage(deltaPage int) {
	if deltaPage == 0 {
		return
	}
	rows := m.outputRows()
	m.scrollOutputLines(deltaPage * rows)
}

func (m *richlineModel) scrollOutputTop() {
	total := len(m.renderOutputLines(m.contentWidth()))
	rows := m.outputRows()
	maxOffset := total - rows
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.outputOffset = maxOffset
}

func (m *richlineModel) scrollOutputBottom() {
	m.outputOffset = 0
}

func (m *richlineModel) normalizeOutputOffset() {
	total := len(m.renderOutputLines(m.contentWidth()))
	m.outputOffset = clampOutputOffset(m.outputOffset, total, m.outputRows())
}

func (m *richlineModel) renderOutputLines(width int) []string {
	if len(m.blocks) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.blocks)*3)
	for i, block := range m.blocks {
		title := strings.TrimSpace(block.Title)
		if title == "" {
			title = "output"
		}
		head := fmt.Sprintf("■ #%d %s", i+1, title)
		out = append(out, styleBlockHeader.Render(truncateDisplayWidth(head, width)))

		if len(block.Lines) == 0 {
			out = append(out, "  (no output)")
		} else {
			for _, line := range block.Lines {
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
			sep := ""
			if width > 0 {
				sep = strings.Repeat("─", width)
			} else {
				sep = "────────────────"
			}
			out = append(out, sep)
		}
	}
	return out
}

func (m *richlineModel) appendBlock(block outputBlock) {
	title := strings.TrimSpace(block.Title)
	if title == "" {
		title = "output"
	}

	normalized := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		for _, s := range normalizeOutputLines(line) {
			normalized = append(normalized, s)
		}
	}

	m.blocks = append(m.blocks, outputBlock{Title: title, Lines: normalized})
	m.trimOutputHistory()
}

func (m *richlineModel) trimOutputHistory() {
	if len(m.blocks) == 0 {
		return
	}
	total := 0
	for _, b := range m.blocks {
		total += len(b.Lines)
	}

	for len(m.blocks) > 1 && (len(m.blocks) > maxOutputBlocks || total > maxLogLines) {
		total -= len(m.blocks[0].Lines)
		m.blocks = m.blocks[1:]
	}
}

func (m *richlineModel) appendHistory(line string) {
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

func (m *richlineModel) historyUp() {
	if len(m.history) == 0 {
		return
	}
	if m.historyPos <= 0 {
		m.historyPos = 0
		m.input.SetValue(m.history[m.historyPos])
		m.input.CursorEnd()
		return
	}
	m.historyPos--
	m.input.SetValue(m.history[m.historyPos])
	m.input.CursorEnd()
}

func (m *richlineModel) historyDown() {
	if len(m.history) == 0 {
		return
	}
	if m.historyPos >= len(m.history)-1 {
		m.historyPos = len(m.history)
		m.input.SetValue("")
		m.input.CursorEnd()
		return
	}
	m.historyPos++
	m.input.SetValue(m.history[m.historyPos])
	m.input.CursorEnd()
}

func runLineCmd(ctx context.Context, root *redant.Command, line string) tea.Cmd {
	return func() tea.Msg {
		switch strings.TrimSpace(line) {
		case "exit", "quit", ":q", `\q`:
			return runLineResultMsg{quit: true}
		case "help", ":help", "?":
			return runLineResultMsg{block: outputBlock{Title: "$ help", Lines: richlineHelpLines(root)}}
		}

		args, parseErr := splitCommandLine(line)
		if parseErr != nil {
			return runLineResultMsg{block: outputBlock{Title: "$ " + line, Lines: []string{fmt.Sprintf("parse input failed: %v", parseErr)}}}
		}
		if len(args) == 0 {
			return runLineResultMsg{}
		}
		if args[0] == root.Name() {
			args = args[1:]
		}
		if len(args) == 0 {
			return runLineResultMsg{}
		}

		title := "$ " + formatCommandLine(root.Name(), args)
		lines := make([]string, 0, 8)
		stdout := bytes.NewBuffer(nil)
		stderr := bytes.NewBuffer(nil)

		runInv := root.Invoke(args...)
		runInv.Stdout = stdout
		runInv.Stderr = stderr
		runInv.Stdin = strings.NewReader("")

		runErr := runInv.WithContext(ctx).Run()

		if out := strings.TrimSpace(stdout.String()); out != "" {
			lines = append(lines, strings.Split(out, "\n")...)
		}
		if out := strings.TrimSpace(stderr.String()); out != "" {
			lines = append(lines, strings.Split(out, "\n")...)
		}
		if runErr != nil {
			lines = append(lines, fmt.Sprintf("error: %v", runErr))
		}
		if len(lines) == 0 {
			lines = append(lines, "(no output)")
		}

		return runLineResultMsg{block: outputBlock{Title: title, Lines: lines}}
	}
}

func richlineHelpLines(root *redant.Command) []string {
	return []string{
		"available shortcuts:",
		"  - TAB: apply selected completion (空输入首次 TAB 显示起始候选)",
		"  - ↑/↓: switch completion candidate (or history when no candidate)",
		"  - PgUp/PgDn/Home/End: scroll output history (when no suggestion list)",
		"  - Ctrl+O: toggle output scroll mode",
		"  - /output: enter output scroll mode (Ctrl+O 替代)",
		"  - /input: back to input mode",
		"  - /help: show slash commands",
		"  - output mode: ↑/↓ line, PgUp/PgDn page, Home/End top/bottom",
		"  - enter: execute command",
		"  - exit / quit: exit richline",
		"examples:",
		fmt.Sprintf("  %s commit -m \"message\"", root.Name()),
		"  commit --help",
	}
}

func slashHelpLines() []string {
	return []string{
		"slash commands:",
		"  /output (/o): 进入输出滚动模式",
		"  /input  (/i): 返回输入模式",
		"  /top /bottom: 跳到输出历史顶/底",
		"  /up /down: 输出按行滚动",
		"  /pgup /pgdown: 输出按页滚动",
		"  /help: 显示本帮助",
		"  /quit: 退出 richline",
	}
}

func tailLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
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

func collectCompletionItems(root *redant.Command, input string) []completionItem {
	if root == nil {
		return nil
	}
	tokens, current := splitCompletionInput(input)
	cmd, consumed := resolveCommandContext(root, tokens)
	if cmd == nil {
		cmd = root
	}

	idx := buildOptionIndex(cmd)
	if vals, ok := suggestFlagValueItems(idx, tokens, current); ok {
		return uniqueCompletionItems(vals)
	}

	var out []completionItem
	if strings.HasPrefix(current, "-") {
		out = append(out, suggestFlagItems(idx, current)...)
		return uniqueCompletionItems(out)
	}

	if consumed == len(tokens) {
		out = append(out, suggestChildrenItems(cmd, current)...)
	}
	out = append(out, suggestArgItems(cmd, idx, tokens[consumed:], current)...)
	out = append(out, suggestFlagItems(idx, current)...)

	return uniqueCompletionItems(out)
}

func suggestionKindTag(kind completionKind) string {
	switch kind {
	case completionKindCommand:
		return "[CMD ]"
	case completionKindFlag:
		return "[FLAG]"
	case completionKindArg:
		return "[ARG ]"
	case completionKindEnum:
		return "[ENUM]"
	default:
		return "[ITEM]"
	}
}

func renderKindTag(kind completionKind) string {
	tag := suggestionKindTag(kind)
	switch kind {
	case completionKindCommand:
		return styleKindCommand.Render(tag)
	case completionKindFlag:
		return styleKindFlag.Render(tag)
	case completionKindArg:
		return styleKindArg.Render(tag)
	case completionKindEnum:
		return styleKindEnum.Render(tag)
	default:
		return styleKindDefault.Render(tag)
	}
}

func renderSuggestionLine(item completionItem, selected bool, maxWidth int) string {
	kind := suggestionKindTag(item.Kind)
	kindWidth := lipgloss.Width(kind)

	insertWidth := 24
	if maxWidth > 0 {
		maxInsert := maxWidth - kindWidth - 1
		if maxInsert < 1 {
			maxInsert = 1
		}
		if insertWidth > maxInsert {
			insertWidth = maxInsert
		}
	}

	insert := padRightDisplay(item.Insert, insertWidth)
	line := fmt.Sprintf("%s %s", renderKindTag(item.Kind), insert)

	baseWidth := kindWidth + 1 + lipgloss.Width(insert)
	if item.Description != "" {
		desc := item.Description
		if maxWidth > 0 {
			descWidth := maxWidth - baseWidth - 1
			if descWidth < 1 {
				desc = ""
			} else {
				desc = truncateDisplayWidth(desc, descWidth)
			}
		}
		if desc != "" {
			line += " " + styleDescription.Render(desc)
		}
	}

	if selected {
		return styleSelectedRow.Render(line)
	}
	return line
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

func applySelectedCompletion(input, selected string) string {
	trimmedRight := strings.TrimRightFunc(input, unicode.IsSpace)
	if trimmedRight == "" {
		return selected + " "
	}
	if len(trimmedRight) < len(input) {
		return trimmedRight + " " + selected + " "
	}
	idx := strings.LastIndexFunc(trimmedRight, unicode.IsSpace)
	if idx < 0 {
		return selected + " "
	}
	return trimmedRight[:idx+1] + selected + " "
}

func splitCompletionInput(input string) ([]string, string) {
	trimmedRight := strings.TrimRightFunc(input, unicode.IsSpace)
	if trimmedRight == "" {
		return nil, ""
	}
	if len(trimmedRight) < len(input) {
		return strings.Fields(trimmedRight), ""
	}
	parts := strings.Fields(trimmedRight)
	if len(parts) == 0 {
		return nil, ""
	}
	return parts[:len(parts)-1], parts[len(parts)-1]
}

func resolveCommandContext(root *redant.Command, tokens []string) (*redant.Command, int) {
	if root == nil {
		return nil, 0
	}
	cur := root
	consumed := 0
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if i == 0 && tok == root.Name() {
			consumed++
			continue
		}
		if strings.HasPrefix(tok, "-") {
			break
		}
		next, ok := resolveCommandToken(cur, tok)
		if !ok {
			break
		}
		cur = next
		consumed++
	}
	return cur, consumed
}

func resolveCommandToken(parent *redant.Command, token string) (*redant.Command, bool) {
	if parent == nil || token == "" {
		return nil, false
	}
	if strings.Contains(token, ":") {
		parts := strings.Split(token, ":")
		cur := parent
		for _, p := range parts {
			child := childByNameOrAlias(cur, p)
			if child == nil {
				return nil, false
			}
			cur = child
		}
		return cur, true
	}
	child := childByNameOrAlias(parent, token)
	if child == nil {
		return nil, false
	}
	return child, true
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

type optionIndex struct {
	byLong  map[string]redant.Option
	byShort map[string]redant.Option
}

func buildOptionIndex(cmd *redant.Command) optionIndex {
	idx := optionIndex{byLong: map[string]redant.Option{}, byShort: map[string]redant.Option{}}
	if cmd == nil {
		return idx
	}
	for _, opt := range cmd.FullOptions() {
		if opt.Hidden || opt.Flag == "" {
			continue
		}
		idx.byLong[opt.Flag] = opt
		if opt.Shorthand != "" {
			idx.byShort[opt.Shorthand] = opt
		}
	}
	return idx
}

func suggestChildrenItems(cmd *redant.Command, current string) []completionItem {
	if cmd == nil {
		return nil
	}
	out := make([]completionItem, 0)
	for _, child := range cmd.Children {
		if child.Hidden {
			continue
		}
		desc := commandDescription(child)
		if current == "" || strings.HasPrefix(child.Name(), current) {
			out = append(out, completionItem{Insert: child.Name(), Description: desc, Kind: completionKindCommand})
		}
		for _, alias := range child.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if current == "" || strings.HasPrefix(alias, current) {
				ad := desc
				if ad == "" {
					ad = fmt.Sprintf("%s 的别名", child.Name())
				} else {
					ad = fmt.Sprintf("%s（%s 的别名）", ad, child.Name())
				}
				out = append(out, completionItem{Insert: alias, Description: ad, Kind: completionKindCommand})
			}
		}
	}
	return out
}

func suggestFlagItems(idx optionIndex, current string) []completionItem {
	out := make([]completionItem, 0)
	for name, opt := range idx.byLong {
		cand := "--" + name
		if current == "" || strings.HasPrefix(cand, current) {
			out = append(out, completionItem{Insert: cand, Description: flagDescription(opt), Kind: completionKindFlag})
		}
	}
	for short, opt := range idx.byShort {
		cand := "-" + short
		if current == "" || strings.HasPrefix(cand, current) {
			out = append(out, completionItem{Insert: cand, Description: flagDescription(opt), Kind: completionKindFlag})
		}
	}
	return out
}

func suggestFlagValueItems(idx optionIndex, tokens []string, current string) ([]completionItem, bool) {
	if strings.HasPrefix(current, "--") && strings.Contains(current, "=") {
		nameWithPrefix, valuePrefix, _ := strings.Cut(current, "=")
		name := strings.TrimPrefix(nameWithPrefix, "--")
		opt, ok := idx.byLong[name]
		if !ok || !optionNeedsValue(opt) {
			return nil, false
		}
		vals := enumValuesFromOption(opt)
		if len(vals) == 0 {
			return nil, false
		}
		out := make([]completionItem, 0, len(vals))
		for _, v := range vals {
			if strings.HasPrefix(v, valuePrefix) {
				out = append(out, completionItem{Insert: nameWithPrefix + "=" + v, Description: "枚举值", Kind: completionKindEnum})
			}
		}
		return out, true
	}

	if len(tokens) == 0 {
		return nil, false
	}
	prev := tokens[len(tokens)-1]
	if strings.HasPrefix(prev, "--") {
		name := strings.TrimPrefix(prev, "--")
		opt, ok := idx.byLong[name]
		if !ok || !optionNeedsValue(opt) {
			return nil, false
		}
		vals := enumValuesFromOption(opt)
		if len(vals) == 0 {
			return nil, false
		}
		out := make([]completionItem, 0, len(vals))
		for _, v := range vals {
			if current == "" || strings.HasPrefix(v, current) {
				out = append(out, completionItem{Insert: v, Description: "枚举值", Kind: completionKindEnum})
			}
		}
		return out, true
	}
	if strings.HasPrefix(prev, "-") && len(prev) == 2 {
		short := strings.TrimPrefix(prev, "-")
		opt, ok := idx.byShort[short]
		if !ok || !optionNeedsValue(opt) {
			return nil, false
		}
		vals := enumValuesFromOption(opt)
		if len(vals) == 0 {
			return nil, false
		}
		out := make([]completionItem, 0, len(vals))
		for _, v := range vals {
			if current == "" || strings.HasPrefix(v, current) {
				out = append(out, completionItem{Insert: v, Description: "枚举值", Kind: completionKindEnum})
			}
		}
		return out, true
	}
	return nil, false
}

func suggestArgItems(cmd *redant.Command, idx optionIndex, restTokens []string, current string) []completionItem {
	if cmd == nil || len(cmd.Args) == 0 {
		return nil
	}
	argPos := countProvidedPositionals(restTokens, idx)
	if argPos >= len(cmd.Args) {
		return nil
	}
	target := cmd.Args[argPos]
	desc := strings.TrimSpace(target.Description)
	if desc == "" {
		desc = "参数"
	}

	out := make([]completionItem, 0)
	for _, v := range enumValuesFromArg(target) {
		if current == "" || strings.HasPrefix(v, current) {
			out = append(out, completionItem{Insert: v, Description: "枚举值 · " + desc, Kind: completionKindEnum})
		}
	}
	if target.Name != "" && (current == "" || strings.HasPrefix(target.Name, current)) {
		out = append(out, completionItem{Insert: "<" + target.Name + ">", Description: desc, Kind: completionKindArg})
	}
	return out
}

func countProvidedPositionals(tokens []string, idx optionIndex) int {
	count := 0
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.HasPrefix(tok, "--") {
			name, _, hasEq := strings.Cut(strings.TrimPrefix(tok, "--"), "=")
			opt, ok := idx.byLong[name]
			if ok && optionNeedsValue(opt) && !hasEq && i+1 < len(tokens) {
				i++
			}
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) == 2 {
			short := strings.TrimPrefix(tok, "-")
			opt, ok := idx.byShort[short]
			if ok && optionNeedsValue(opt) && i+1 < len(tokens) {
				i++
			}
			continue
		}
		count++
	}
	return count
}

func optionNeedsValue(opt redant.Option) bool {
	return strings.TrimSpace(opt.Type()) != "bool"
}

func flagDescription(opt redant.Option) string {
	desc := strings.TrimSpace(opt.Description)
	if desc == "" {
		desc = "flag"
	}
	typ := strings.TrimSpace(opt.Type())
	if typ != "" {
		desc = fmt.Sprintf("%s [%s]", desc, typ)
	}
	return desc
}

func commandDescription(cmd *redant.Command) string {
	short := strings.TrimSpace(cmd.Short)
	if short != "" {
		return short
	}
	return strings.TrimSpace(cmd.Long)
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
		pi := completionKindPriority(out[i].Kind)
		pj := completionKindPriority(out[j].Kind)
		if pi != pj {
			return pi < pj
		}
		if out[i].Insert != out[j].Insert {
			return out[i].Insert < out[j].Insert
		}
		return out[i].Description < out[j].Description
	})
	return out
}

func completionKindPriority(kind completionKind) int {
	switch kind {
	case completionKindCommand:
		return 1
	case completionKindFlag:
		return 2
	case completionKindArg:
		return 3
	case completionKindEnum:
		return 4
	default:
		return 99
	}
}

func enumValuesFromArg(arg redant.Arg) []string {
	if arg.Value == nil {
		return nil
	}
	return parseEnumValues(arg.Value.Type())
}

func enumValuesFromOption(opt redant.Option) []string {
	if opt.Value == nil {
		return nil
	}
	vals := enumValuesFromValue(opt.Value)
	if len(vals) == 0 {
		vals = parseEnumValues(opt.Value.Type())
	}
	return uniqueSorted(vals)
}

func enumValuesFromValue(value pflag.Value) []string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case *redant.Enum:
		return append([]string(nil), v.Choices...)
	case *redant.EnumArray:
		return append([]string(nil), v.Choices...)
	case interface{ Underlying() pflag.Value }:
		return enumValuesFromValue(v.Underlying())
	default:
		return nil
	}
}

func parseEnumValues(typ string) []string {
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
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(strings.ReplaceAll(p, `\|`, "|"))
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return uniqueSorted(out)
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
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
