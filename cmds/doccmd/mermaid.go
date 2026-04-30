package doccmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/pubgo/redant"
)

// writeTree writes a Mermaid graph TD of the command tree.
func writeTree(w io.Writer, root *redant.Command, maxDepth int) error {
	p := &mermaidWriter{w: w}
	p.line("graph TD")

	rootID := nodeID(root.Name())
	p.line("    %s[\"%s\"]", rootID, escMermaid(root.Name()))
	p.line("    style %s fill:#1e40af,stroke:#3b82f6,color:#fff", rootID)

	writeTreeNodes(p, root, rootID, 0, maxDepth)

	return p.err
}

func writeTreeNodes(p *mermaidWriter, cmd *redant.Command, parentID string, depth, maxDepth int) {
	if maxDepth > 0 && depth >= maxDepth {
		return
	}
	for _, child := range cmd.Children {
		if child.Hidden {
			continue
		}
		childID := parentID + "_" + nodeID(child.Name())
		label := child.Name()
		if child.Short != "" {
			label += "\\n" + truncate(child.Short, 40)
		}

		switch {
		case child.ResponseStreamHandler != nil:
			p.line("    %s[[\"%s\"]]", childID, escMermaid(label))
			p.line("    style %s fill:#7c3aed,stroke:#a78bfa,color:#fff", childID)
		case child.ResponseHandler != nil:
			p.line("    %s[\\\"%s\\\"/]", childID, escMermaid(label))
			p.line("    style %s fill:#059669,stroke:#34d399,color:#fff", childID)
		case child.Handler != nil:
			p.line("    %s(\"%s\")", childID, escMermaid(label))
		default:
			p.line("    %s{{\"%s\"}}", childID, escMermaid(label))
			p.line("    style %s fill:#374151,stroke:#6b7280,color:#d1d5db", childID)
		}

		if child.Deprecated != "" {
			p.line("    style %s stroke-dasharray: 5 5", childID)
		}

		p.line("    %s --> %s", parentID, childID)

		writeTreeNodes(p, child, childID, depth+1, maxDepth)
	}
}

// writeDispatch writes a Mermaid flowchart of the command dispatch pipeline.
func writeDispatch(w io.Writer, root *redant.Command) error {
	p := &mermaidWriter{w: w}

	p.line("flowchart TD")
	p.line("    START([\"Invocation.Run()\"])")
	p.line("    INIT[\"init: register global flags\\nsetParentCommand\"]")
	p.line("    START --> INIT")

	p.line("")
	p.line("    subgraph RESOLVE[\"命令解析\"]")
	p.line("        GET_EXEC[\"getExecCommand\\n空格路径 / 冒号路径\"]")
	p.line("        MATCHED{\"matched?\"}")
	p.line("        ARGV0_RESOLVE[\"resolveArgv0Command\\nbusybox 分发\"]")
	p.line("        ARGV0_CHECK{\"argv0 matched?\"}")
	p.line("        USE_ROOT[\"使用根命令\"]")
	p.line("        CMD_FOUND[\"目标命令确定\"]")
	p.line("")
	p.line("        GET_EXEC --> MATCHED")
	p.line("        MATCHED -->|Yes| CMD_FOUND")
	p.line("        MATCHED -->|No| ARGV0_RESOLVE")
	p.line("        ARGV0_RESOLVE --> ARGV0_CHECK")
	p.line("        ARGV0_CHECK -->|Yes| CMD_FOUND")
	p.line("        ARGV0_CHECK -->|No| USE_ROOT")
	p.line("        USE_ROOT --> CMD_FOUND")
	p.line("    end")
	p.line("    INIT --> GET_EXEC")

	p.line("")
	p.line("    subgraph FLAGS[\"标志解析\"]")
	p.line("        INHERIT[\"继承父命令标志\\ncopyFlagSetWithout\"]")
	p.line("        PARSE[\"pflag.Parse(args)\"]")
	p.line("        INHERIT --> PARSE")
	p.line("    end")
	p.line("    CMD_FOUND --> INHERIT")

	p.line("")
	p.line("    subgraph SHORT[\"短路检测\"]")
	p.line("        CHECK_LIST{\"--list-commands\\n--list-flags?\"}")
	p.line("        LIST_OUT[\"输出并返回\"]")
	p.line("        CHECK_CHILD{\"还有子命令?\"}")
	p.line("        RECURSE[\"递归 inv.run()\"]")
	p.line("")
	p.line("        CHECK_LIST -->|Yes| LIST_OUT")
	p.line("        CHECK_LIST -->|No| CHECK_CHILD")
	p.line("        CHECK_CHILD -->|Yes| RECURSE")
	p.line("    end")
	p.line("    PARSE --> CHECK_LIST")

	p.line("")
	p.line("    subgraph EXEC[\"执行\"]")
	p.line("        CONTINUE[\"参数解析\"]")
	p.line("        MIDDLEWARE[\"中间件链\\nroot → parent → child\"]")
	p.line("        HELP_CHECK{\"需要帮助?\"}")
	p.line("        HELP[\"DefaultHelpFn()\"]")
	p.line("        HANDLER[\"Handler / ResponseHandler\\n/ ResponseStreamHandler\"]")
	p.line("")
	p.line("        CONTINUE --> MIDDLEWARE")
	p.line("        MIDDLEWARE --> HELP_CHECK")
	p.line("        HELP_CHECK -->|Yes| HELP")
	p.line("        HELP_CHECK -->|No| HANDLER")
	p.line("    end")
	p.line("    CHECK_CHILD -->|No| CONTINUE")

	p.line("")
	var cmdCount, mwCount int
	countCommands(root, &cmdCount, &mwCount)
	p.line("    INFO[\"%s: %d commands, %d middlewares\"]", escMermaid(root.Name()), cmdCount, mwCount)
	p.line("    style INFO fill:#1e293b,stroke:#475569,color:#94a3b8")

	return p.err
}

// writeMCPSequence writes a Mermaid sequence diagram of MCP tool invocation.
func writeMCPSequence(w io.Writer, root *redant.Command) error {
	p := &mermaidWriter{w: w}

	appName := root.Name()

	p.line("sequenceDiagram")
	p.line("    participant Agent")
	p.line("    participant MCP as MCP Server")
	p.line("    participant Router as Command Router")
	p.line("    participant Handler as Handler")
	p.line("")

	p.line("    Note over Agent,MCP: 初始化阶段 - %s", appName)
	p.line("    Agent->>MCP: initialize")
	p.line("    MCP-->>Agent: capabilities (tools, resources, prompts)")
	p.line("")

	p.line("    Note over Agent,MCP: 工具发现")
	p.line("    Agent->>MCP: tools/list")

	var toolCount int
	countToolCommands(root, &toolCount)
	p.line("    MCP-->>Agent: %d tools (inputSchema + outputSchema)", toolCount)
	p.line("")

	p.line("    Note over Agent,Handler: 工具调用")
	p.line("    Agent->>MCP: tools/call \"<tool-name>\"")
	p.line("    activate MCP")
	p.line("")

	p.line("    MCP->>MCP: buildArgv(inputSchema -> CLI flags)")
	p.line("    MCP->>Router: Invoke(argv...)")
	p.line("    activate Router")
	p.line("")

	p.line("    Router->>Router: getExecCommand()")
	p.line("    Router->>Router: flag inheritance + parse")
	p.line("    Router->>Router: middleware chain")
	p.line("    Router->>Handler: handler(ctx, inv)")
	p.line("    activate Handler")
	p.line("")

	p.line("    alt Unary (ResponseHandler)")
	p.line("        Handler-->>Router: (T, error)")
	p.line("        Router-->>MCP: structuredContent.response = T")
	p.line("    else Stream (ResponseStreamHandler)")
	p.line("        loop for each item")
	p.line("            Handler->>Router: stream.Send(T)")
	p.line("        end")
	p.line("        Router-->>MCP: structuredContent.response = []T")
	p.line("    else Plain (Handler)")
	p.line("        Handler-->>Router: stdout/stderr text")
	p.line("        Router-->>MCP: content[].text")
	p.line("    end")
	p.line("")

	p.line("    deactivate Handler")
	p.line("    deactivate Router")
	p.line("    MCP-->>Agent: CallToolResult")
	p.line("    deactivate MCP")

	p.line("")
	p.line("    Note over Agent,MCP: 辅助能力")
	p.line("    Agent->>MCP: resources/read \"llms.txt\"")
	p.line("    MCP-->>Agent: 命令树文档")
	p.line("    Agent->>MCP: prompts/get \"%s-overview\"", appName)
	p.line("    MCP-->>Agent: 全局使用指南")

	return p.err
}

// ---------- helpers ----------

type mermaidWriter struct {
	w   io.Writer
	err error
}

func (m *mermaidWriter) line(format string, args ...any) {
	if m.err != nil {
		return
	}
	_, m.err = fmt.Fprintf(m.w, format+"\n", args...)
}

func nodeID(name string) string {
	r := strings.NewReplacer("-", "_", ".", "_", ":", "_", " ", "_")
	return r.Replace(name)
}

func escMermaid(s string) string {
	r := strings.NewReplacer(
		"\"", "#quot;",
		"<", "&lt;",
		">", "&gt;",
		"[", "#lsqb;",
		"]", "#rsqb;",
		"(", "#lpar;",
		")", "#rpar;",
		"{", "#lbrace;",
		"}", "#rbrace;",
	)
	return r.Replace(s)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
}

func countCommands(cmd *redant.Command, cmdCount, mwCount *int) {
	for _, child := range cmd.Children {
		if child.Hidden {
			continue
		}
		*cmdCount++
		if child.Middleware != nil {
			*mwCount++
		}
		countCommands(child, cmdCount, mwCount)
	}
}

func countToolCommands(cmd *redant.Command, count *int) {
	for _, child := range cmd.Children {
		if child.Hidden {
			continue
		}
		if child.Handler != nil || child.ResponseHandler != nil || child.ResponseStreamHandler != nil {
			*count++
		}
		countToolCommands(child, count)
	}
}
