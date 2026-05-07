package llmstxtcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pubgo/redant"
)

// WriteSkill writes the command tree as SKILL.md documents following the
// standard format: YAML frontmatter (---) + # title + ## 使用场景 + usage example.
// Each leaf command becomes a separate skill entry.
func WriteSkill(w io.Writer, root *redant.Command, maxDepth int) error {
	p := &printer{w: w}

	// Collect all leaf/handler commands
	var commands []*skillEntry
	collectSkillEntries(root, root.Name(), 0, maxDepth, &commands)

	if len(commands) == 0 {
		return nil
	}

	for i, entry := range commands {
		if i > 0 {
			p.line("")
		}
		writeSkillEntry(p, entry)
	}

	return p.err
}

type skillEntry struct {
	name         string   // underscore-joined unique name (e.g. "平台_project_create")
	pathSegments []string // command path segments from root (e.g. ["平台", "project", "create"])
	fullPath     string   // space-joined command path (e.g. "平台 project create")
	description  string
	long         string
	aliases      []string
	args         []redant.Arg
	options      []redant.Option
	response     string // e.g. "Unary StatusResult" or "Stream string"
}

func collectSkillEntries(cmd *redant.Command, path string, depth, maxDepth int, out *[]*skillEntry) {
	if cmd.Hidden {
		return
	}

	// Build path segments from the space-separated path
	segments := strings.Fields(path)

	// A command is a skill entry if it has a handler (leaf) or explicitly has
	// ResponseHandler/ResponseStreamHandler
	isLeaf := cmd.Handler != nil || cmd.ResponseHandler != nil || cmd.ResponseStreamHandler != nil

	if isLeaf {
		entry := &skillEntry{
			name:         strings.Join(segments, "_"),
			pathSegments: segments,
			fullPath:     path,
			description:  cmd.Short,
			long:         cmd.Long,
			aliases:      cmd.Aliases,
		}

		// Collect non-hidden options (skip global flags)
		for _, opt := range cmd.Options {
			if opt.Flag != "" && !opt.Hidden {
				entry.options = append(entry.options, opt)
			}
		}

		// Collect args
		for _, arg := range cmd.Args {
			entry.args = append(entry.args, arg)
		}

		// Response type
		if cmd.ResponseHandler != nil {
			ti := cmd.ResponseHandler.TypeInfo()
			entry.response = fmt.Sprintf("Unary `%s`", ti.TypeName)
		} else if cmd.ResponseStreamHandler != nil {
			ti := cmd.ResponseStreamHandler.TypeInfo()
			entry.response = fmt.Sprintf("Stream `%s`", ti.TypeName)
		}

		*out = append(*out, entry)
	}

	// Recurse into children
	if maxDepth > 0 && depth >= maxDepth {
		return
	}
	for _, child := range cmd.Children {
		childPath := path + " " + child.Name()
		collectSkillEntries(child, childPath, depth+1, maxDepth, out)
	}
}

func writeSkillEntry(p *printer, e *skillEntry) {
	// Build description with "Use when:" hints
	desc := e.description
	if desc == "" {
		desc = fmt.Sprintf("Run %s command.", e.name)
	}

	// Build argument-hint from args
	argHint := ""
	if len(e.args) > 0 {
		hints := make([]string, 0, len(e.args))
		for _, arg := range e.args {
			hints = append(hints, arg.Name)
		}
		argHint = strings.Join(hints, ", ")
	}

	// --- YAML frontmatter ---
	p.line("---")
	p.line("name: %s", e.name)
	p.line("description: \"%s\"", escapeYAMLString(desc))
	if argHint != "" {
		p.line("argument-hint: \"%s\"", argHint)
	}
	p.line("---")
	p.line("")

	// # Title
	p.line("# %s", e.fullPath)
	p.line("")

	// ## 使用场景
	p.line("## 使用场景")
	p.line("")
	if e.long != "" && e.long != e.description {
		p.line("- %s", e.long)
	} else {
		p.line("- %s", desc)
	}
	if len(e.aliases) > 0 {
		p.line("- 别名: %s", strings.Join(e.aliases, ", "))
	}
	if e.response != "" {
		p.line("- 响应类型: %s", e.response)
	}
	p.line("")

	// ## 用法
	p.line("## 用法")
	p.line("")

	// Usage line
	usage := e.fullPath
	for _, arg := range e.args {
		if arg.Required {
			usage += " <" + arg.Name + ">"
		} else {
			usage += " [" + arg.Name + "]"
		}
	}
	p.line("```sh")
	p.line("%s", usage)
	p.line("```")
	p.line("")

	// Arguments
	if len(e.args) > 0 {
		p.line("### 参数")
		p.line("")
		for _, arg := range e.args {
			req := ""
			if arg.Required {
				req = "（必填）"
			}
			desc := arg.Description
			if arg.Default != "" {
				desc += fmt.Sprintf("（默认: %s）", arg.Default)
			}
			if desc == "" {
				desc = "位置参数"
			}
			p.line("- `%s`%s — %s", arg.Name, req, desc)
		}
		p.line("")
	}

	// Options
	if len(e.options) > 0 {
		p.line("### 选项")
		p.line("")
		for _, opt := range e.options {
			flag := "--" + opt.Flag
			if opt.Shorthand != "" {
				flag = "-" + opt.Shorthand + ", " + flag
			}
			typeStr := ""
			if opt.Value != nil {
				typeStr = opt.Value.Type()
			}
			if typeStr != "" && typeStr != "bool" {
				flag += " " + typeStr
			}

			desc := opt.Description
			var extras []string
			if opt.Default != "" {
				extras = append(extras, "默认: "+opt.Default)
			}
			if opt.Required {
				extras = append(extras, "必填")
			}
			if len(opt.Envs) > 0 {
				extras = append(extras, "环境变量: "+strings.Join(opt.Envs, ", "))
			}
			if len(extras) > 0 {
				desc += "（" + strings.Join(extras, "; ") + "）"
			}

			p.line("- `%s` — %s", flag, desc)
		}
		p.line("")
	}
}

func escapeYAMLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// WriteSkillDir writes each leaf command as a separate SKILL.md file under dir.
// Directory structure follows the command tree:
//
//	<dir>/<root>/<sub1>/<sub2>/SKILL.md
//
// The SKILL.md name field uses underscore-joined path to ensure uniqueness.
func WriteSkillDir(dir string, root *redant.Command, maxDepth int) error {
	var commands []*skillEntry
	collectSkillEntries(root, root.Name(), 0, maxDepth, &commands)

	for _, entry := range commands {
		// Build nested directory path from path segments
		skillDir := filepath.Join(dir, filepath.Join(entry.pathSegments...))
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return fmt.Errorf("creating skill dir %s: %w", skillDir, err)
		}

		var buf bytes.Buffer
		p := &printer{w: &buf}
		writeSkillEntry(p, entry)
		if p.err != nil {
			return fmt.Errorf("writing skill %s: %w", entry.name, p.err)
		}

		path := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	return nil
}
