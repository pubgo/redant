package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pubgo/redant"
)

const (
	defaultMCPServerName   = "redant-mcp"
	mcpServerVersionPrefix = "v"
)

type Server struct {
	root   *redant.Command
	tools  []toolDef
	server *mcp.Server
}

type ToolInfo struct {
	Name           string                   `json:"name"`
	Description    string                   `json:"description,omitempty"`
	Path           []string                 `json:"path"`
	InputSchema    map[string]any           `json:"inputSchema"`
	OutputSchema   map[string]any           `json:"outputSchema"`
	SupportsStream bool                     `json:"supportsStream,omitempty"`
	ResponseType   *redant.ResponseTypeInfo `json:"responseType,omitempty"`
}

func ListToolInfos(root *redant.Command) []ToolInfo {
	defs := collectTools(root)
	out := make([]ToolInfo, 0, len(defs))
	for _, td := range defs {
		out = append(out, ToolInfo{
			Name:           td.Name,
			Description:    td.Description,
			Path:           append([]string(nil), td.PathTokens...),
			InputSchema:    td.InputSchema,
			OutputSchema:   td.OutputSchema,
			SupportsStream: td.SupportsStream,
			ResponseType:   td.ResponseType,
		})
	}
	return out
}

func New(root *redant.Command) *Server {
	s := &Server{
		root:  root,
		tools: collectTools(root),
		server: mcp.NewServer(&mcp.Implementation{
			Name:    serverNameFromRoot(root),
			Version: mcpServerVersionPrefix + strings.TrimSpace(redant.Version()),
		}, &mcp.ServerOptions{}),
	}
	s.registerTools()
	s.registerResources()
	s.registerPrompts()
	return s
}

func ServeStdio(ctx context.Context, root *redant.Command, r io.Reader, w io.Writer) error {
	return New(root).ServeStdio(ctx, r, w)
}

func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	if s == nil || s.root == nil || s.server == nil {
		return errors.New("mcp server root command is nil")
	}
	if r == nil {
		r = strings.NewReader("")
	}
	if w == nil {
		w = io.Discard
	}

	transport := &mcp.IOTransport{
		Reader: nopReadCloser{Reader: r},
		Writer: nopWriteCloser{Writer: w},
	}
	return s.server.Run(ctx, transport)
}

func (s *Server) registerTools() {
	if s == nil || s.server == nil {
		return
	}

	for _, td := range s.tools {
		tool := td
		mcpTool := &mcp.Tool{
			Name:         tool.Name,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		}
		if tool.Annotations != nil {
			mcpTool.Annotations = &mcp.ToolAnnotations{
				ReadOnlyHint:    tool.Annotations.ReadOnly,
				IdempotentHint:  tool.Annotations.Idempotent,
				DestructiveHint: tool.Annotations.Destructive,
				OpenWorldHint:   tool.Annotations.OpenWorld,
			}
		}
		s.server.AddTool(mcpTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{}
			if raw := req.Params.Arguments; len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid tool arguments: %v", err)}},
						IsError: true,
					}, nil
				}
			}

			result, err := s.callTool(ctx, toolsCallParams{
				Name:      tool.Name,
				Arguments: args,
			})
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
					IsError: true,
				}, nil
			}

			return mapToolResultToSDK(result), nil
		})
	}
}

func mapToolResultToSDK(result map[string]any) *mcp.CallToolResult {
	text := "ok"
	var structured any
	if content, ok := result["content"]; ok {
		switch vv := content.(type) {
		case []map[string]any:
			if len(vv) > 0 {
				if t, ok := vv[0]["text"].(string); ok && t != "" {
					text = t
				}
			}
		case []any:
			if len(vv) > 0 {
				if m, ok := vv[0].(map[string]any); ok {
					if t, ok := m["text"].(string); ok && t != "" {
						text = t
					}
				}
			}
		}
	}
	if sc, ok := result["structuredContent"]; ok {
		structured = sc
	}

	isErr, _ := result["isError"].(bool)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: text}},
		StructuredContent: structured,
		IsError:           isErr,
	}
}

type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error {
	return nil
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}

type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func serverNameFromRoot(root *redant.Command) string {
	if root == nil {
		return defaultMCPServerName
	}

	name := strings.TrimSpace(root.Name())
	if name == "" {
		return defaultMCPServerName
	}
	return name
}
