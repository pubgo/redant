package richlinecmd

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/pubgo/redant"
)

func TestCollectCompletionItems_WithDescription(t *testing.T) {
	root := buildTestRoot()

	items := collectCompletionItems(root, "com")
	item, ok := findCompletion(items, "commit")
	if !ok {
		t.Fatalf("expected commit in completion items: %+v", items)
	}
	if !strings.Contains(item.Description, "提交") {
		t.Fatalf("expected commit description, got: %q", item.Description)
	}
	if item.Kind != completionKindCommand {
		t.Fatalf("expected command kind, got: %q", item.Kind)
	}

	items = collectCompletionItems(root, "commit --fo")
	item, ok = findCompletion(items, "--format")
	if !ok {
		t.Fatalf("expected --format in completion items: %+v", items)
	}
	if !strings.Contains(item.Description, "输出格式") {
		t.Fatalf("expected flag description, got: %q", item.Description)
	}
	if item.Kind != completionKindFlag {
		t.Fatalf("expected flag kind, got: %q", item.Kind)
	}

	items = collectCompletionItems(root, "commit --format ")
	item, ok = findCompletion(items, "json")
	if !ok {
		t.Fatalf("expected enum value json in completion items: %+v", items)
	}
	if item.Kind != completionKindEnum {
		t.Fatalf("expected enum kind, got: %q", item.Kind)
	}
}

func TestApplySelectedCompletion(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		selected string
		want     string
	}{
		{name: "replace token", line: "com", selected: "commit", want: "commit "},
		{name: "append after space", line: "commit ", selected: "--format", want: "commit --format "},
		{name: "replace last token", line: "commit --fo", selected: "--format", want: "commit --format "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applySelectedCompletion(tt.line, tt.selected)
			if got != tt.want {
				t.Fatalf("completion mismatch, want=%q got=%q", tt.want, got)
			}
		})
	}
}

func TestFormatCommandLine(t *testing.T) {
	got := formatCommandLine("fastcommit", []string{"commit", "-m", "hello world", "--expr", "a|b"})
	want := `fastcommit commit -m "hello world" --expr "a|b"`
	if got != want {
		t.Fatalf("format mismatch\nwant=%s\ngot=%s", want, got)
	}
}

func TestVisibleSuggestionRange(t *testing.T) {
	tests := []struct {
		name     string
		total    int
		selected int
		maxRows  int
		wantS    int
		wantE    int
	}{
		{name: "empty", total: 0, selected: 0, maxRows: 10, wantS: 0, wantE: 0},
		{name: "small list", total: 6, selected: 3, maxRows: 10, wantS: 0, wantE: 6},
		{name: "center", total: 100, selected: 50, maxRows: 10, wantS: 45, wantE: 55},
		{name: "near top", total: 100, selected: 1, maxRows: 10, wantS: 0, wantE: 10},
		{name: "near bottom", total: 100, selected: 98, maxRows: 10, wantS: 90, wantE: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e := visibleSuggestionRange(tt.total, tt.selected, tt.maxRows)
			if s != tt.wantS || e != tt.wantE {
				t.Fatalf("range mismatch, want=(%d,%d) got=(%d,%d)", tt.wantS, tt.wantE, s, e)
			}
		})
	}
}

func TestSuggestionRows(t *testing.T) {
	tests := []struct {
		name  string
		total int
		h     int
		want  int
	}{
		{name: "default when window unknown", total: 30, h: 0, want: 10},
		{name: "bounded by default rows", total: 18, h: 40, want: 10},
		{name: "reserve output area", total: 30, h: 14, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &richlineModel{height: tt.h}
			got := m.suggestionRows(tt.total)
			if got != tt.want {
				t.Fatalf("suggestion rows mismatch, want=%d got=%d", tt.want, got)
			}
		})
	}
}

func TestRenderOutputLines_WithBlocks(t *testing.T) {
	m := &richlineModel{
		blocks: []outputBlock{
			{Title: "$ app commit -m hi", Lines: []string{"ok line 1", "ok line 2"}},
			{Title: "$ app release", Lines: []string{"release done"}},
		},
	}

	lines := m.renderOutputLines(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "#1") || !strings.Contains(joined, "#2") {
		t.Fatalf("expected block numbering in output, got: %s", joined)
	}
	if !strings.Contains(joined, "app commit") || !strings.Contains(joined, "release done") {
		t.Fatalf("expected block content in output, got: %s", joined)
	}
}

func TestAppendBlock_TrimOutputHistory(t *testing.T) {
	m := &richlineModel{}
	for i := 0; i < maxOutputBlocks+20; i++ {
		m.appendBlock(outputBlock{Title: "cmd", Lines: []string{"line"}})
	}
	if len(m.blocks) > maxOutputBlocks {
		t.Fatalf("expected blocks trimmed to <= %d, got %d", maxOutputBlocks, len(m.blocks))
	}
}

func TestVisibleOutputRange(t *testing.T) {
	tests := []struct {
		name   string
		total  int
		rows   int
		offset int
		wantS  int
		wantE  int
	}{
		{name: "empty", total: 0, rows: 10, offset: 0, wantS: 0, wantE: 0},
		{name: "fit all", total: 8, rows: 10, offset: 0, wantS: 0, wantE: 8},
		{name: "from bottom", total: 20, rows: 6, offset: 0, wantS: 14, wantE: 20},
		{name: "scroll up", total: 20, rows: 6, offset: 5, wantS: 9, wantE: 15},
		{name: "clamp overflow offset", total: 20, rows: 6, offset: 999, wantS: 0, wantE: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e := visibleOutputRange(tt.total, tt.rows, tt.offset)
			if s != tt.wantS || e != tt.wantE {
				t.Fatalf("range mismatch, want=(%d,%d) got=(%d,%d)", tt.wantS, tt.wantE, s, e)
			}
		})
	}
}

func TestRecomputeSuggestions_HideWhenInputEmpty(t *testing.T) {
	root := buildTestRoot()
	m := newRichlineModel(context.Background(), root, "richline> ", nil, "", false)
	m.input.SetValue("")
	m.recomputeSuggestions()
	if len(m.suggestions) != 0 {
		t.Fatalf("expected no suggestions on empty input, got=%d", len(m.suggestions))
	}

	m.input.SetValue("com")
	m.recomputeSuggestions()
	if len(m.suggestions) == 0 {
		t.Fatalf("expected suggestions for non-empty input")
	}
}

func TestCompletionItemsSortedByKindPriority(t *testing.T) {
	items := uniqueCompletionItems([]completionItem{
		{Insert: "json", Kind: completionKindEnum},
		{Insert: "<target>", Kind: completionKindArg},
		{Insert: "--format", Kind: completionKindFlag},
		{Insert: "commit", Kind: completionKindCommand},
		{Insert: "-m", Kind: completionKindFlag},
		{Insert: "deploy", Kind: completionKindCommand},
	})

	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, string(item.Kind)+":"+item.Insert)
	}

	want := []string{
		"command:commit",
		"command:deploy",
		"flag:--format",
		"flag:-m",
		"arg:<target>",
		"enum:json",
	}

	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected sorted order\nwant=%v\ngot=%v", want, got)
	}
}

func TestRenderSuggestionLine_RespectsWidth(t *testing.T) {
	item := completionItem{
		Insert:      "very-long-command-name-that-should-be-clamped",
		Description: strings.Repeat("description ", 20),
		Kind:        completionKindCommand,
	}

	line := renderSuggestionLine(item, false, 40)
	if got := lipgloss.Width(line); got > 40 {
		t.Fatalf("line width overflow, want <=40 got=%d", got)
	}
}

func TestTruncateDisplayWidth(t *testing.T) {
	if got := truncateDisplayWidth("abcdef", 4); got != "abc…" {
		t.Fatalf("unexpected truncate result: %q", got)
	}
	if got := truncateDisplayWidth("你好世界", 5); got != "你好…" {
		t.Fatalf("unexpected wide-char truncate result: %q", got)
	}
}

func TestNormalizeOutputLines(t *testing.T) {
	lines := normalizeOutputLines("progress 10%\rprogress 100%\nline2\rline2-final")
	if len(lines) != 2 {
		t.Fatalf("unexpected line count: %d", len(lines))
	}
	if lines[0] != "progress 100%" {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
	if lines[1] != "line2-final" {
		t.Fatalf("unexpected second line: %q", lines[1])
	}
}

func TestWrapDisplayWidth(t *testing.T) {
	wrapped := wrapDisplayWidth("abcdefghijklmnopqrstuvwxyz", 8)
	if len(wrapped) < 3 {
		t.Fatalf("expected wrapped lines, got: %v", wrapped)
	}
	for _, line := range wrapped {
		if w := lipgloss.Width(line); w > 8 {
			t.Fatalf("wrapped line too wide, line=%q width=%d", line, w)
		}
	}
}

func TestHandleSlashCommand_ModeSwitch(t *testing.T) {
	root := buildTestRoot()
	m := newRichlineModel(context.Background(), root, "richline> ", nil, "", false)

	handled, cmd := m.handleSlashCommand("/output")
	if !handled || cmd != nil {
		t.Fatalf("expected /output handled without cmd, handled=%v cmd=%v", handled, cmd)
	}
	if !m.outputFocus {
		t.Fatalf("expected outputFocus=true after /output")
	}

	handled, cmd = m.handleSlashCommand("/input")
	if !handled || cmd != nil {
		t.Fatalf("expected /input handled without cmd, handled=%v cmd=%v", handled, cmd)
	}
	if m.outputFocus {
		t.Fatalf("expected outputFocus=false after /input")
	}
}

func TestHandleSlashCommand_HelpAndUnknown(t *testing.T) {
	root := buildTestRoot()
	m := newRichlineModel(context.Background(), root, "richline> ", nil, "", false)
	baseBlocks := len(m.blocks)

	handled, _ := m.handleSlashCommand("/")
	if !handled {
		t.Fatalf("expected slash help handled")
	}
	if len(m.blocks) <= baseBlocks {
		t.Fatalf("expected help block appended")
	}
	if m.blocks[len(m.blocks)-1].Title != "/help" {
		t.Fatalf("expected last block title /help, got %q", m.blocks[len(m.blocks)-1].Title)
	}

	handled, _ = m.handleSlashCommand("/not-exists")
	if !handled {
		t.Fatalf("expected unknown slash handled")
	}
	last := m.blocks[len(m.blocks)-1]
	if !strings.Contains(strings.Join(last.Lines, "\n"), "未知 slash 命令") {
		t.Fatalf("expected unknown slash feedback, got: %v", last.Lines)
	}
}

func TestCollectSlashCompletionItems(t *testing.T) {
	items := collectSlashCompletionItems("/")
	if len(items) == 0 {
		t.Fatalf("expected slash suggestions for '/'")
	}
	if _, ok := findCompletion(items, "/output"); !ok {
		t.Fatalf("expected /output in slash suggestions")
	}

	items = collectSlashCompletionItems("/o")
	if _, ok := findCompletion(items, "/output"); !ok {
		t.Fatalf("expected /output for prefix /o")
	}

	items = collectSlashCompletionItems("/output now")
	if len(items) != 0 {
		t.Fatalf("expected no slash suggestions after second token, got=%d", len(items))
	}
}

func TestRecomputeSuggestions_Slash(t *testing.T) {
	root := buildTestRoot()
	m := newRichlineModel(context.Background(), root, "richline> ", nil, "", false)
	m.input.SetValue("/o")
	m.recomputeSuggestions()
	if len(m.suggestions) == 0 {
		t.Fatalf("expected slash suggestions")
	}
	if _, ok := findCompletion(m.suggestions, "/output"); !ok {
		t.Fatalf("expected /output in recomputed suggestions")
	}
}

func buildTestRoot() *redant.Command {
	var (
		format string
		msg    string
		target string
	)
	commit := &redant.Command{
		Use:   "commit",
		Short: "提交代码",
		Options: redant.OptionSet{
			{Flag: "format", Description: "输出格式", Value: redant.EnumOf(&format, "text", "json", "yaml")},
			{Flag: "message", Shorthand: "m", Description: "提交信息", Value: redant.StringOf(&msg)},
		},
		Args: redant.ArgSet{{Name: "target", Description: "目标环境", Value: redant.EnumOf(&target, "dev", "test", "prod")}},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			return nil
		},
	}
	return &redant.Command{
		Use:      "app",
		Children: []*redant.Command{commit},
		Handler:  func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
}

func findCompletion(items []completionItem, insert string) (completionItem, bool) {
	for _, item := range items {
		if item.Insert == insert {
			return item, true
		}
	}
	return completionItem{}, false
}
