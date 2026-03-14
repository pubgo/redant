package mcpcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/internal/mcpserver"
)

func New() *redant.Command {
	var transport string

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
		Long:  "Print all mapped MCP tools (name, description, path, input/output schema) as JSON.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			root := inv.Command
			for root.Parent() != nil {
				root = root.Parent()
			}

			infos := mcpserver.ListToolInfos(root)
			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(infos)
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
