package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pubgo/redant"
	"github.com/pubgo/redant/internal/mcpserver"
)

// Session wraps an MCP client session with connection metadata.
type Session struct {
	session    *mcp.ClientSession
	serverInfo *mcp.Implementation
	cancel     context.CancelFunc
	serverErr  <-chan error // only set for in-process mode
}

// Close shuts down the session and underlying transport.
func (s *Session) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.session != nil {
		return s.session.Close()
	}
	return nil
}

// ServerInfo returns the server implementation info from initialization.
func (s *Session) ServerInfo() *mcp.Implementation {
	return s.serverInfo
}

// ListTools returns all tools exposed by the server.
func (s *Session) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	res, err := s.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// ListResources returns all resources exposed by the server.
func (s *Session) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	res, err := s.session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		return nil, err
	}
	return res.Resources, nil
}

// ListPrompts returns all prompts exposed by the server.
func (s *Session) ListPrompts(ctx context.Context) ([]*mcp.Prompt, error) {
	res, err := s.session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		return nil, err
	}
	return res.Prompts, nil
}

// CallTool invokes a tool by name with the given arguments.
func (s *Session) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	return s.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
}

// ReadResource reads a resource by URI.
func (s *Session) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	return s.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: uri,
	})
}

// GetPrompt gets a prompt by name with arguments.
func (s *Session) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	return s.session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
}

// ConnectInProcess creates an in-process MCP client session connected to the
// given redant command tree. The server runs in a goroutine.
func ConnectInProcess(ctx context.Context, root *redant.Command) (*Session, error) {
	srv := mcpserver.New(root)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(ctx)

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.ServeTransport(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "redant-mcp-client",
		Version: redant.Version(),
	}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect to in-process MCP server: %w", err)
	}

	var serverInfo *mcp.Implementation
	if initRes := session.InitializeResult(); initRes != nil {
		serverInfo = initRes.ServerInfo
	}

	return &Session{
		session:    session,
		serverInfo: serverInfo,
		cancel:     cancel,
		serverErr:  serverErrCh,
	}, nil
}

// ConnectStdio creates an MCP client session connected to an external command
// via stdio transport.
func ConnectStdio(ctx context.Context, command string, args ...string) (*Session, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	transport := &mcp.CommandTransport{Command: cmd}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "redant-mcp-client",
		Version: redant.Version(),
	}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", command, err)
	}

	var serverInfo *mcp.Implementation
	if initRes := session.InitializeResult(); initRes != nil {
		serverInfo = initRes.ServerInfo
	}

	return &Session{
		session:    session,
		serverInfo: serverInfo,
	}, nil
}

// ToolResultText extracts the text content from a CallToolResult.
func ToolResultText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// ToolResultJSON extracts structured content as JSON bytes from a CallToolResult.
func ToolResultJSON(res *mcp.CallToolResult) ([]byte, error) {
	if res.StructuredContent != nil {
		return json.Marshal(res.StructuredContent)
	}
	text := ToolResultText(res)
	if text != "" {
		return []byte(text), nil
	}
	return nil, nil
}
