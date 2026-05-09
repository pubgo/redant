package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/cmds/llmstxtcmd"
)

func (s *Server) registerPrompts() {
	if s == nil || s.server == nil || s.root == nil {
		return
	}

	appName := serverNameFromRoot(s.root)

	// Prompt: how to use the entire CLI
	s.server.AddPrompt(&mcp.Prompt{
		Name:        appName + "-overview",
		Description: fmt.Sprintf("Overview of %s CLI: available commands, global flags, and usage patterns.", appName),
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		var buf bytes.Buffer
		if err := llmstxtcmd.WriteLLMSTxt(&buf, s.root, 0); err != nil {
			return nil, fmt.Errorf("generating llms.txt for prompt: %w", err)
		}
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Overview of %s CLI", appName),
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: buf.String()},
			}},
		}, nil
	})

	// Prompt: per-command usage guide
	for _, td := range s.tools {
		tool := td
		promptName := "use-" + tool.Name

		args := buildPromptArgs(tool)

		promptDesc := buildPromptDescription(tool)

		s.server.AddPrompt(&mcp.Prompt{
			Name:        promptName,
			Description: promptDesc,
			Arguments:   args,
		}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			var buf bytes.Buffer
			writeCommandHelp(&buf, tool)

			// If user supplied argument values, append them as context.
			if len(req.Params.Arguments) > 0 {
				buf.WriteString("\n## User-specified values\n\n")
				for k, v := range req.Params.Arguments {
					fmt.Fprintf(&buf, "- %s = %s\n", k, v)
				}
			}

			return &mcp.GetPromptResult{
				Description: fmt.Sprintf("Usage guide for %s", strings.Join(tool.PathTokens, " ")),
				Messages: []*mcp.PromptMessage{{
					Role:    "user",
					Content: &mcp.TextContent{Text: buf.String()},
				}},
			}, nil
		})
	}
}

// buildPromptDescription builds a rich description for a per-command prompt,
// including the command description, positional args and visible flags.
func buildPromptDescription(tool toolDef) string {
	var b strings.Builder

	path := strings.Join(tool.PathTokens, " ")
	if tool.Description != "" {
		b.WriteString(tool.Description)
	} else {
		fmt.Fprintf(&b, "Usage guide for %s.", path)
	}

	// Append args summary.
	if len(tool.Command.Args) > 0 {
		b.WriteString("\n\nArgs:")
		for _, arg := range tool.Command.Args {
			b.WriteString(" ")
			if arg.Required {
				fmt.Fprintf(&b, "<%s>", arg.Name)
			} else {
				fmt.Fprintf(&b, "[%s]", arg.Name)
			}
		}
	}

	// Append flags summary.
	var flags []string
	for _, opt := range tool.Options {
		if opt.Flag == "" || opt.Hidden || isSystemFlag(opt.Flag) {
			continue
		}
		flags = append(flags, "--"+opt.Flag)
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "\nFlags: %s", strings.Join(flags, ", "))
	}

	return b.String()
}

// buildPromptArgs creates MCP prompt arguments from a tool's flags and args.
func buildPromptArgs(tool toolDef) []*mcp.PromptArgument {
	var args []*mcp.PromptArgument

	for _, arg := range tool.Command.Args {
		args = append(args, &mcp.PromptArgument{
			Name:        arg.Name,
			Description: arg.Description,
			Required:    arg.Required,
		})
	}

	for _, opt := range tool.Options {
		if opt.Flag == "" || opt.Hidden || isSystemFlag(opt.Flag) {
			continue
		}
		args = append(args, &mcp.PromptArgument{
			Name:        opt.Flag,
			Description: opt.Description,
		})
	}

	return args
}

// commandAgentHints generates agent-oriented hint text from Command.Metadata.
func commandAgentHints(cmd *redant.Command) string {
	if cmd == nil || len(cmd.Metadata) == 0 {
		return ""
	}

	var hints []string
	// Recognized agent hint keys.
	hintKeys := []struct {
		key   string
		label string
	}{
		{"agent.readonly", "Read-only"},
		{"agent.idempotent", "Idempotent"},
		{"agent.destructive", "Destructive"},
		{"agent.requires-confirmation", "Requires confirmation"},
		{"agent.side-effects", "Side effects"},
	}

	for _, hk := range hintKeys {
		if v := cmd.Meta(hk.key); v != "" {
			hints = append(hints, fmt.Sprintf("- **%s**: %s", hk.label, v))
		}
	}

	if len(hints) == 0 {
		return ""
	}
	return "\n## Agent Hints\n\n" + strings.Join(hints, "\n") + "\n"
}
