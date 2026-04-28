package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pubgo/redant"
)

func (s *Server) registerResources() {
	if s == nil || s.server == nil || s.root == nil {
		return
	}

	appName := serverNameFromRoot(s.root)

	// Resource: llms.txt — full command tree documentation
	s.server.AddResource(&mcp.Resource{
		URI:         fmt.Sprintf("redant://%s/llms.txt", appName),
		Name:        "llms.txt",
		Description: "Complete command tree documentation (commands, flags, args, response types) in structured plain text.",
		MIMEType:    "text/markdown",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		var buf bytes.Buffer
		writeLLMSTxt(&buf, s.root, 0)
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "text/markdown",
				Text:     buf.String(),
			}},
		}, nil
	})

	// Resource: help for each command with a handler
	for _, td := range s.tools {
		tool := td
		uri := fmt.Sprintf("redant://%s/help/%s", appName, tool.Name)
		s.server.AddResource(&mcp.Resource{
			URI:         uri,
			Name:        tool.Name + " help",
			Description: fmt.Sprintf("Usage help for %q command.", strings.Join(tool.PathTokens, " ")),
			MIMEType:    "text/markdown",
		}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			var buf bytes.Buffer
			writeCommandHelp(&buf, tool)
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      req.Params.URI,
					MIMEType: "text/markdown",
					Text:     buf.String(),
				}},
			}, nil
		})
	}
}

// writeCommandHelp writes a single command's help in structured plain text.
func writeCommandHelp(w io.Writer, tool toolDef) {
	p := &resPrinter{w: w}

	p.line("# %s", strings.Join(tool.PathTokens, " "))
	p.line("")
	if tool.Description != "" {
		p.line("%s", tool.Description)
		p.line("")
	}

	// Args
	if len(tool.Command.Args) > 0 {
		p.line("## Arguments")
		p.line("")
		for _, arg := range tool.Command.Args {
			desc := arg.Description
			if arg.Required {
				desc += " (required)"
			}
			if arg.Default != "" {
				desc += fmt.Sprintf(" (default: %s)", arg.Default)
			}
			if desc == "" {
				desc = "(positional)"
			}
			p.line("- `%s` — %s", arg.Name, desc)
		}
		p.line("")
	}

	// Options
	var visibleOpts redant.OptionSet
	for _, opt := range tool.Options {
		if opt.Flag != "" && !opt.Hidden && !isSystemFlag(opt.Flag) {
			visibleOpts = append(visibleOpts, opt)
		}
	}
	if len(visibleOpts) > 0 {
		p.line("## Options")
		p.line("")
		for _, opt := range visibleOpts {
			flag := "--" + opt.Flag
			if opt.Shorthand != "" {
				flag = "-" + opt.Shorthand + ", " + flag
			}
			typeStr := opt.Type()
			if typeStr != "" && typeStr != "bool" {
				flag += " " + typeStr
			}

			parts := []string{"`" + flag + "`"}
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
			if len(extras) > 0 {
				parts = append(parts, "("+strings.Join(extras, "; ")+")")
			}
			p.line("- %s", strings.Join(parts, " "))
		}
		p.line("")
	}

	// Response type
	if tool.ResponseType != nil {
		kind := "Unary"
		if tool.SupportsStream {
			kind = "Stream"
		}
		p.line("## Response")
		p.line("")
		p.line("Type: %s `%s`", kind, tool.ResponseType.TypeName)
		p.line("")
	}
}

// writeLLMSTxt generates the full command tree documentation.
func writeLLMSTxt(w io.Writer, root *redant.Command, maxDepth int) {
	p := &resPrinter{w: w}

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

	// Global options
	var globals []redant.Option
	for _, opt := range root.Options {
		if opt.Flag != "" && !opt.Hidden {
			globals = append(globals, opt)
		}
	}
	if len(globals) > 0 {
		p.line("## Global Options")
		p.line("")
		writeResOptions(p, globals)
		p.line("")
	}

	// Commands
	if len(root.Children) > 0 {
		p.line("## Commands")
		p.line("")
		for _, child := range root.Children {
			writeResCommandTree(p, child, root.Name(), 1, maxDepth)
		}
	}
}

func writeResCommandTree(p *resPrinter, cmd *redant.Command, parentPath string, depth, maxDepth int) {
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

	// Args
	if len(cmd.Args) > 0 {
		p.line("**Arguments:**")
		p.line("")
		for _, arg := range cmd.Args {
			desc := arg.Description
			if arg.Required {
				desc += " (required)"
			}
			if desc == "" {
				desc = "(positional)"
			}
			p.line("- `%s` — %s", arg.Name, desc)
		}
		p.line("")
	}

	// Command-specific options
	var cmdOpts []redant.Option
	for _, opt := range cmd.Options {
		if opt.Flag != "" && !opt.Hidden {
			cmdOpts = append(cmdOpts, opt)
		}
	}
	if len(cmdOpts) > 0 {
		p.line("**Options:**")
		p.line("")
		writeResOptions(p, cmdOpts)
		p.line("")
	}

	// Response type
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

	if maxDepth > 0 && depth >= maxDepth {
		return
	}
	for _, child := range cmd.Children {
		writeResCommandTree(p, child, fullPath, depth+1, maxDepth)
	}
}

func writeResOptions(p *resPrinter, opts []redant.Option) {
	for _, opt := range opts {
		flag := "--" + opt.Flag
		if opt.Shorthand != "" {
			flag = "-" + opt.Shorthand + ", " + flag
		}
		typeStr := opt.Type()
		if typeStr != "" && typeStr != "bool" {
			flag += " " + typeStr
		}

		parts := []string{"`" + flag + "`"}
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
		if len(extras) > 0 {
			parts = append(parts, "("+strings.Join(extras, "; ")+")")
		}
		p.line("- %s", strings.Join(parts, " "))
	}
}

type resPrinter struct {
	w   io.Writer
	err error
}

func (p *resPrinter) line(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format+"\n", args...)
}
