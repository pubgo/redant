package llmstxtcmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pubgo/redant"
)

// New returns a "llms-txt" command that prints a structured plain-text
// overview of the entire command tree, optimised for LLM consumption.
//
// The output follows the llms.txt convention (https://llmstxt.org/):
// human-readable Markdown that is also easy for language models to parse
// and grep.
func New() *redant.Command {
	var depth int64

	return &redant.Command{
		Use:   "llms-txt",
		Short: "Print command tree documentation in llms.txt format for LLM consumption.",
		Long:  "Generate a structured plain-text overview of all commands, flags, and arguments. The output is Markdown-based and optimised for LLMs to read, grep, and reference.",
		Options: redant.OptionSet{
			{
				Flag:        "depth",
				Shorthand:   "d",
				Description: "Maximum command tree depth (0 = unlimited).",
				Default:     "0",
				Value:       redant.Int64Of(&depth),
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}
			return WriteLLMSTxt(inv.Stdout, root, int(depth))
		},
	}
}

// WriteLLMSTxt writes the full command tree documentation to w.
func WriteLLMSTxt(w io.Writer, root *redant.Command, maxDepth int) error {
	p := &printer{w: w}

	p.line("# %s", root.Name())
	p.line("")
	if root.Short != "" {
		p.line("> %s", root.Short)
		p.line("")
	}
	if root.Long != "" {
		p.line("%s", root.Long)
		p.line("")
	}

	// Global options from root
	if len(root.Options) > 0 {
		var globals []redant.Option
		for _, opt := range root.Options {
			if opt.Flag != "" && !opt.Hidden {
				globals = append(globals, opt)
			}
		}
		if len(globals) > 0 {
			p.line("## Global Options")
			p.line("")
			writeOptions(p, globals)
			p.line("")
		}
	}

	// Commands
	if len(root.Children) > 0 {
		p.line("## Commands")
		p.line("")
		for _, child := range root.Children {
			writeCommandTree(p, child, root.Name(), 1, maxDepth)
		}
	}

	return p.err
}

func writeCommandTree(p *printer, cmd *redant.Command, parentPath string, depth, maxDepth int) {
	if cmd.Hidden {
		return
	}

	fullPath := parentPath + " " + cmd.Name()
	heading := strings.Repeat("#", min(depth+2, 6))

	p.line("%s %s", heading, fullPath)
	p.line("")

	if cmd.Short != "" {
		p.line("%s", cmd.Short)
		p.line("")
	}
	if cmd.Long != "" && cmd.Long != cmd.Short {
		p.line("%s", cmd.Long)
		p.line("")
	}

	if len(cmd.Aliases) > 0 {
		p.line("Aliases: %s", strings.Join(cmd.Aliases, ", "))
		p.line("")
	}

	if cmd.Deprecated != "" {
		p.line("**DEPRECATED**: %s", cmd.Deprecated)
		p.line("")
	}

	// Arguments
	if len(cmd.Args) > 0 {
		p.line("**Arguments:**")
		p.line("")
		for _, arg := range cmd.Args {
			desc := arg.Description
			extras := formatArgExtras(arg)
			if extras != "" {
				desc += " " + extras
			}
			if desc == "" {
				desc = "(positional)"
			}
			p.line("- `%s` — %s", arg.Name, desc)
		}
		p.line("")
	}

	// Options (command-specific only)
	var cmdOpts []redant.Option
	for _, opt := range cmd.Options {
		if opt.Flag != "" && !opt.Hidden {
			cmdOpts = append(cmdOpts, opt)
		}
	}
	if len(cmdOpts) > 0 {
		p.line("**Options:**")
		p.line("")
		writeOptions(p, cmdOpts)
		p.line("")
	}

	// Response type info
	if cmd.ResponseHandler != nil {
		ti := cmd.ResponseHandler.TypeInfo()
		p.line("**Response:** Unary `%s`", ti.TypeName)
		p.line("")
	} else if cmd.ResponseStreamHandler != nil {
		ti := cmd.ResponseStreamHandler.TypeInfo()
		p.line("**Response:** Stream `%s`", ti.TypeName)
		p.line("")
	}

	p.line("---")
	p.line("")

	// Recurse children
	if maxDepth > 0 && depth >= maxDepth {
		return
	}
	for _, child := range cmd.Children {
		writeCommandTree(p, child, fullPath, depth+1, maxDepth)
	}
}

func writeOptions(p *printer, opts []redant.Option) {
	for _, opt := range opts {
		var parts []string

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

		parts = append(parts, "`"+flag+"`")

		if opt.Description != "" {
			parts = append(parts, "— "+opt.Description)
		}

		var extras []string
		if opt.Default != "" {
			extras = append(extras, "default: "+opt.Default)
		}
		if opt.Required {
			extras = append(extras, "required")
		}
		if len(opt.Envs) > 0 {
			extras = append(extras, "env: "+strings.Join(opt.Envs, ", "))
		}
		if opt.Deprecated != "" {
			extras = append(extras, "DEPRECATED: "+opt.Deprecated)
		}
		if len(extras) > 0 {
			parts = append(parts, "("+strings.Join(extras, "; ")+")")
		}

		p.line("- %s", strings.Join(parts, " "))
	}
}

func formatArgExtras(arg redant.Arg) string {
	var extras []string
	if arg.Required {
		extras = append(extras, "required")
	}
	if arg.Default != "" {
		extras = append(extras, "default: "+arg.Default)
	}
	if len(extras) == 0 {
		return ""
	}
	return "(" + strings.Join(extras, "; ") + ")"
}

type printer struct {
	w   io.Writer
	err error
}

func (p *printer) line(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format+"\n", args...)
}
