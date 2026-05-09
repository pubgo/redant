package mcpcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/internal/mcpclient"
)

func newClientCommand() *redant.Command {
	var (
		command string
		format  string
	)

	connectOpts := redant.OptionSet{
		{
			Flag:        "command",
			Shorthand:   "c",
			Description: "External MCP server command (e.g. 'myapp mcp serve --transport stdio'). If empty, connects to self in-process.",
			Value:       redant.StringOf(&command),
		},
		{
			Flag:        "format",
			Shorthand:   "f",
			Description: "Output format.",
			Value:       redant.EnumOf(&format, "json", "text"),
			Default:     "json",
		},
	}

	connect := func(ctx context.Context, inv *redant.Invocation) (*mcpclient.Session, error) {
		cmd := strings.TrimSpace(command)
		if cmd != "" {
			parts := strings.Fields(cmd)
			return mcpclient.ConnectStdio(ctx, parts[0], parts[1:]...)
		}
		root := inv.Command
		for root.Parent() != nil {
			root = root.Parent()
		}
		return mcpclient.ConnectInProcess(ctx, root)
	}

	toolsCmd := &redant.Command{
		Use:     "tools",
		Short:   "List all MCP tools from the connected server.",
		Options: connectOpts,
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			sess, err := connect(ctx, inv)
			if err != nil {
				return err
			}
			defer sess.Close()

			tools, err := sess.ListTools(ctx)
			if err != nil {
				return fmt.Errorf("list tools: %w", err)
			}

			if format == "text" {
				for i, t := range tools {
					_, _ = fmt.Fprintf(inv.Stdout, "%d. %s\n", i+1, t.Name)
					if t.Description != "" {
						_, _ = fmt.Fprintf(inv.Stdout, "   %s\n", t.Description)
					}
				}
				if len(tools) == 0 {
					_, _ = fmt.Fprintln(inv.Stdout, "No tools found.")
				}
				return nil
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(tools)
		},
	}

	resourcesCmd := &redant.Command{
		Use:     "resources",
		Short:   "List all MCP resources from the connected server.",
		Options: connectOpts,
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			sess, err := connect(ctx, inv)
			if err != nil {
				return err
			}
			defer sess.Close()

			resources, err := sess.ListResources(ctx)
			if err != nil {
				return fmt.Errorf("list resources: %w", err)
			}

			if format == "text" {
				for i, r := range resources {
					_, _ = fmt.Fprintf(inv.Stdout, "%d. %s\n", i+1, r.Name)
					if r.URI != "" {
						_, _ = fmt.Fprintf(inv.Stdout, "   URI: %s\n", r.URI)
					}
					if r.Description != "" {
						_, _ = fmt.Fprintf(inv.Stdout, "   %s\n", r.Description)
					}
				}
				if len(resources) == 0 {
					_, _ = fmt.Fprintln(inv.Stdout, "No resources found.")
				}
				return nil
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(resources)
		},
	}

	promptsCmd := &redant.Command{
		Use:     "prompts",
		Short:   "List all MCP prompts from the connected server.",
		Options: connectOpts,
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			sess, err := connect(ctx, inv)
			if err != nil {
				return err
			}
			defer sess.Close()

			prompts, err := sess.ListPrompts(ctx)
			if err != nil {
				return fmt.Errorf("list prompts: %w", err)
			}

			if format == "text" {
				for i, p := range prompts {
					_, _ = fmt.Fprintf(inv.Stdout, "%d. %s\n", i+1, p.Name)
					if p.Description != "" {
						_, _ = fmt.Fprintf(inv.Stdout, "   %s\n", p.Description)
					}
				}
				if len(prompts) == 0 {
					_, _ = fmt.Fprintln(inv.Stdout, "No prompts found.")
				}
				return nil
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(prompts)
		},
	}

	var (
		toolName string
		toolArgs string
	)
	callCmd := &redant.Command{
		Use:   "call",
		Short: "Call an MCP tool by name and return the result.",
		Options: append(redant.OptionSet{
			{
				Flag:        "tool",
				Shorthand:   "t",
				Description: "Tool name to call.",
				Value:       redant.StringOf(&toolName),
				Required:    true,
			},
			{
				Flag:        "args",
				Shorthand:   "a",
				Description: "Tool arguments as JSON string.",
				Value:       redant.StringOf(&toolArgs),
				Default:     "{}",
			},
		}, connectOpts...),
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			sess, err := connect(ctx, inv)
			if err != nil {
				return err
			}
			defer sess.Close()

			var args map[string]any
			if err := json.Unmarshal([]byte(toolArgs), &args); err != nil {
				return fmt.Errorf("parse --args JSON: %w", err)
			}

			result, err := sess.CallTool(ctx, toolName, args)
			if err != nil {
				return fmt.Errorf("call tool %q: %w", toolName, err)
			}

			if result.IsError {
				_, _ = fmt.Fprintf(inv.Stderr, "tool returned error\n")
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")

			// Prefer structured content if available.
			if result.StructuredContent != nil {
				return enc.Encode(result.StructuredContent)
			}
			// Fall back to text content.
			text := mcpclient.ToolResultText(result)
			_, err = fmt.Fprintln(inv.Stdout, text)
			return err
		},
	}

	var serverInfoCmd = &redant.Command{
		Use:     "info",
		Short:   "Show MCP server info (name, version, capabilities).",
		Options: connectOpts,
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			sess, err := connect(ctx, inv)
			if err != nil {
				return err
			}
			defer sess.Close()

			info := sess.ServerInfo()
			if info == nil {
				_, _ = fmt.Fprintln(inv.Stdout, "No server info available.")
				return nil
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(info)
		},
	}

	return &redant.Command{
		Use:   "client",
		Short: "MCP client: connect to an MCP server and inspect tools/resources/prompts.",
		Long:  "Connect to self (in-process) or an external MCP server via stdio, then list or call tools, resources, and prompts.",
		Children: []*redant.Command{
			serverInfoCmd,
			toolsCmd,
			resourcesCmd,
			promptsCmd,
			callCmd,
		},
	}
}

// parseExternalCommand splits a command string into executable + args safely.
// Only used when --command is provided explicitly by the user.
func parseExternalCommand(cmdStr string) *exec.Cmd {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil
	}
	return exec.Command(parts[0], parts[1:]...)
}
