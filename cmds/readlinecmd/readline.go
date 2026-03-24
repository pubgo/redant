package readlinecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
	"github.com/spf13/pflag"

	"github.com/pubgo/redant"
)

func New() *redant.Command {
	var (
		prompt            string
		history           string
		noHistory         bool
		doubleCtrlCToExit bool
	)

	return &redant.Command{
		Use:   "readline",
		Short: "启动交互式 readline 命令行",
		Long:  "进入多轮交互 REPL，支持命令补全、flag 补全、参数提示与循环执行。输入 exit/quit 退出。",
		Options: redant.OptionSet{
			{Flag: "prompt", Description: "交互提示符", Value: redant.StringOf(&prompt), Default: "redant> "},
			{Flag: "history-file", Description: "历史记录文件路径（为空自动使用 ~/.redant_readline_history）", Value: redant.StringOf(&history)},
			{Flag: "no-history", Description: "禁用历史记录持久化", Value: redant.BoolOf(&noHistory)},
			{Flag: "double-ctrl-c-exit", Description: "启用后需要连续按两次 Ctrl+C 才退出 readline", Value: redant.BoolOf(&doubleCtrlCToExit)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			historyFile := strings.TrimSpace(history)
			if historyFile == "" && !noHistory {
				if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
					historyFile = filepath.Join(home, ".redant_readline_history")
				}
			}

			cfg := &readline.Config{
				Prompt:          prompt,
				AutoComplete:    &dynamicCompleter{root: root},
				InterruptPrompt: "^C",
				EOFPrompt:       "exit",
				HistoryFile:     historyFile,
				Stdout:          inv.Stdout,
				Stderr:          inv.Stderr,
			}
			cfg.Stdin = io.NopCloser(inv.Stdin)
			if noHistory {
				cfg.HistoryFile = ""
				cfg.DisableAutoSaveHistory = true
			}

			rl, err := readline.NewEx(cfg)
			if err != nil {
				return err
			}
			defer func() { _ = rl.Close() }()

			_, _ = fmt.Fprintln(inv.Stdout, "readline mode started, type 'help' for tips, 'exit' to quit.")

			go func() {
				<-ctx.Done()
				_ = rl.Close()
			}()

			pendingInterrupt := false
			for {
				line, err := rl.Readline()
				done, readErr, nextPending := handleReadlineReadError(ctx, err, line, doubleCtrlCToExit, pendingInterrupt)
				pendingInterrupt = nextPending
				if done {
					return readErr
				}
				if err != nil {
					continue
				}
				pendingInterrupt = false

				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				switch line {
				case "exit", "quit", ":q", `\q`:
					return nil
				case "help", ":help", "?":
					printReadlineHelp(inv.Stdout, root)
					continue
				}

				args, parseErr := splitCommandLine(line)
				if parseErr != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "parse input failed: %v\n", parseErr)
					continue
				}
				if len(args) == 0 {
					continue
				}
				if args[0] == root.Name() {
					args = args[1:]
				}
				if len(args) == 0 {
					continue
				}

				runInv := root.Invoke(args...)
				runInv.Stdout = inv.Stdout
				runInv.Stderr = inv.Stderr
				runInv.Stdin = readerOnly{r: inv.Stdin}

				_, _ = fmt.Fprintf(inv.Stdout, "$ %s\n", formatCommandLine(root.Name(), args))

				if runErr := runInv.WithContext(ctx).Run(); runErr != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "error: %v\n", runErr)
				}
			}
		},
	}
}

func handleReadlineReadError(ctx context.Context, err error, line string, doubleCtrlCToExit bool, pendingInterrupt bool) (done bool, readErr error, nextPending bool) {
	if err == nil {
		return false, nil, false
	}

	if errors.Is(err, readline.ErrInterrupt) {
		if strings.TrimSpace(line) == "" {
			if !doubleCtrlCToExit {
				return true, nil, false
			}
			if pendingInterrupt {
				return true, nil, false
			}
			return false, nil, true
		}
		return false, nil, false
	}

	if errors.Is(err, io.EOF) {
		return true, nil, false
	}

	if ctx != nil && ctx.Err() != nil {
		return true, nil, false
	}

	return true, err, false
}

type readerOnly struct {
	r io.Reader
}

func (r readerOnly) Read(p []byte) (int, error) {
	if r.r == nil {
		return 0, io.EOF
	}
	return r.r.Read(p)
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

func AddReadlineCommand(rootCmd *redant.Command) {
	rootCmd.Children = append(rootCmd.Children, New())
}

func printReadlineHelp(w io.Writer, root *redant.Command) {
	_, _ = fmt.Fprintln(w, "available shortcuts:")
	_, _ = fmt.Fprintln(w, "  - TAB: completion")
	_, _ = fmt.Fprintln(w, "  - exit / quit: exit readline")
	_, _ = fmt.Fprintln(w, "  - help: show this message")
	_, _ = fmt.Fprintln(w, "examples:")
	_, _ = fmt.Fprintf(w, "  %s commit -m \"message\"\n", root.Name())
	_, _ = fmt.Fprintln(w, "  commit --help")
}

type dynamicCompleter struct {
	root *redant.Command
}

func (c *dynamicCompleter) Do(line []rune, pos int) ([][]rune, int) {
	if c == nil || c.root == nil || pos < 0 || pos > len(line) {
		return nil, 0
	}

	input := string(line[:pos])
	tokens, current := splitCompletionInput(input)

	cmd, consumed := resolveCommandContext(c.root, tokens)
	if cmd == nil {
		cmd = c.root
	}

	items := c.suggestions(cmd, consumed, tokens, current)
	if len(items) == 0 {
		return nil, len([]rune(current))
	}

	prefix := []rune(current)
	out := make([][]rune, 0, len(items))
	for _, item := range items {
		r := []rune(item)
		if len(prefix) > 0 && len(r) >= len(prefix) && runesHasPrefix(r, prefix) {
			out = append(out, r[len(prefix):])
			continue
		}
		out = append(out, r)
	}
	return out, len([]rune(current))
}

func runesHasPrefix(s, prefix []rune) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i := range prefix {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

func (c *dynamicCompleter) suggestions(cmd *redant.Command, consumed int, tokens []string, current string) []string {
	idx := buildOptionIndex(cmd)

	if v, ok := suggestFlagValues(idx, tokens, current); ok {
		return uniqueSorted(v)
	}

	var out []string
	if strings.HasPrefix(current, "-") {
		out = append(out, suggestFlags(idx, current)...)
		return uniqueSorted(out)
	}

	if consumed == len(tokens) {
		out = append(out, suggestChildren(cmd, current)...)
	}

	out = append(out, suggestArgs(cmd, idx, tokens[consumed:], current)...)
	out = append(out, suggestFlags(idx, current)...)

	return uniqueSorted(out)
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

	current := parts[len(parts)-1]
	return parts[:len(parts)-1], current
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
		for _, part := range parts {
			child := childByNameOrAlias(cur, part)
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
	idx := optionIndex{
		byLong:  map[string]redant.Option{},
		byShort: map[string]redant.Option{},
	}
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

func suggestChildren(cmd *redant.Command, current string) []string {
	if cmd == nil {
		return nil
	}
	out := make([]string, 0)
	for _, child := range cmd.Children {
		if child.Hidden {
			continue
		}
		if current == "" || strings.HasPrefix(child.Name(), current) {
			out = append(out, child.Name())
		}
		for _, alias := range child.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if current == "" || strings.HasPrefix(alias, current) {
				out = append(out, alias)
			}
		}
	}
	return out
}

func suggestFlags(idx optionIndex, current string) []string {
	out := make([]string, 0)
	for name := range idx.byLong {
		cand := "--" + name
		if current == "" || strings.HasPrefix(cand, current) {
			out = append(out, cand)
		}
	}
	for short := range idx.byShort {
		cand := "-" + short
		if current == "" || strings.HasPrefix(cand, current) {
			out = append(out, cand)
		}
	}
	return out
}

func suggestFlagValues(idx optionIndex, tokens []string, current string) ([]string, bool) {
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
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			if strings.HasPrefix(v, valuePrefix) {
				out = append(out, nameWithPrefix+"="+v)
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
		return filterPrefix(vals, current), true
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
		return filterPrefix(vals, current), true
	}

	return nil, false
}

func suggestArgs(cmd *redant.Command, idx optionIndex, restTokens []string, current string) []string {
	if cmd == nil || len(cmd.Args) == 0 {
		return nil
	}

	argPos := countProvidedPositionals(restTokens, idx)
	if argPos >= len(cmd.Args) {
		return nil
	}

	target := cmd.Args[argPos]
	vals := filterPrefix(enumValuesFromArg(target), current)
	if target.Name != "" && (current == "" || strings.HasPrefix(target.Name, current)) {
		vals = append(vals, "<"+target.Name+">")
	}
	return vals
}

func countProvidedPositionals(tokens []string, idx optionIndex) int {
	count := 0
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.HasPrefix(tok, "--") {
			name, _, hasEq := strings.Cut(strings.TrimPrefix(tok, "--"), "=")
			opt, ok := idx.byLong[name]
			if ok && optionNeedsValue(opt) && !hasEq {
				if i+1 < len(tokens) {
					i++
				}
			}
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) == 2 {
			short := strings.TrimPrefix(tok, "-")
			opt, ok := idx.byShort[short]
			if ok && optionNeedsValue(opt) {
				if i+1 < len(tokens) {
					i++
				}
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

func filterPrefix(values []string, prefix string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if prefix == "" || strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	return out
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
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
		return nil, fmt.Errorf("unfinished escape sequence")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote")
	}
	flush()
	return out, nil
}
