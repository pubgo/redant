package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/llmstxtcmd"
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
		_ = llmstxtcmd.WriteLLMSTxt(&buf, s.root, 0)
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

	// Resource: JSON schema for each tool (machine-consumable)
	for _, td := range s.tools {
		tool := td
		uri := fmt.Sprintf("redant://%s/schema/%s", appName, tool.Name)
		s.server.AddResource(&mcp.Resource{
			URI:         uri,
			Name:        tool.Name + " schema",
			Description: fmt.Sprintf("JSON Schema for %q tool input/output.", tool.Name),
			MIMEType:    "application/json",
		}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			schema := map[string]any{
				"name":         tool.Name,
				"description":  tool.Description,
				"inputSchema":  tool.InputSchema,
				"outputSchema": tool.OutputSchema,
			}
			b, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("marshal schema: %w", err)
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      req.Params.URI,
					MIMEType: "application/json",
					Text:     string(b),
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
