package mcpcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/internal/mcpserver"
)

func New() *redant.Command {
	var transport string
	var listFormat string

	serveCmd := &redant.Command{
		Use:   "serve",
		Short: "Start MCP server for current command tree.",
		Long:  "Expose current redant command tree as MCP tools over selected transport.",
		Options: redant.OptionSet{
			{
				Flag:        "transport",
				Description: "MCP transport type.",
				Value:       redant.EnumOf(&transport, "stdio"),
				Default:     "stdio",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			transport = strings.TrimSpace(transport)
			if transport == "" {
				transport = "stdio"
			}

			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			switch transport {
			case "stdio":
				return mcpserver.ServeStdio(ctx, root, inv.Stdin, inv.Stdout)
			default:
				return fmt.Errorf("unsupported mcp transport: %s", transport)
			}
		},
	}

	listCmd := &redant.Command{
		Use:   "list",
		Short: "List all MCP tools metadata.",
		Long:  "List all mapped MCP tools (name, description, path, input/output schema).",
		Options: redant.OptionSet{
			{
				Flag:        "format",
				Description: "Output format.",
				Value:       redant.EnumOf(&listFormat, "json", "text"),
				Default:     "json",
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			infos := mcpserver.ListToolInfos(root)
			format := strings.TrimSpace(strings.ToLower(listFormat))
			if format == "" {
				format = "json"
			}

			switch format {
			case "json":
				enc := json.NewEncoder(inv.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(infos)
			case "text":
				return writeToolInfosText(inv.Stdout, infos)
			default:
				return fmt.Errorf("unsupported format: %s", format)
			}
		},
	}

	return &redant.Command{
		Use:   "mcp",
		Short: "Model Context Protocol integration commands.",
		Long:  "Expose redant CLI definitions (commands/flags/args) as MCP tools.",
		Children: []*redant.Command{
			listCmd,
			serveCmd,
		},
	}
}

func AddMCPCommand(rootCmd *redant.Command) {
	rootCmd.Children = append(rootCmd.Children, New())
}

func writeToolInfosText(w io.Writer, infos []mcpserver.ToolInfo) error {
	if len(infos) == 0 {
		_, err := fmt.Fprintln(w, "No MCP tools found.")
		return err
	}

	for i, info := range infos {
		if _, err := fmt.Fprintf(w, "%d. %s\n", i+1, info.Name); err != nil {
			return err
		}
		desc := strings.TrimSpace(info.Description)
		if desc == "" {
			desc = "(no description)"
		}
		if _, err := fmt.Fprintf(w, "   description: %s\n", desc); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "   path: %s\n", strings.Join(info.Path, " > ")); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "   inputSchema: yes"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "   outputSchema: yes"); err != nil {
			return err
		}
	}

	return nil
}
