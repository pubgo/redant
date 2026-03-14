# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 新增隐藏全局标志 `--args`：支持以重复参数或 CSV 形式覆盖命令位置参数，用于在需要时通过 flag 直接注入/替代 `args`。
- 新增 MCP 能力：提供 `cmds/mcpcmd` 与 `internal/mcpserver`，支持将命令树映射为 MCP Tools，并通过 `mcp serve --transport stdio` 对外服务。

## 修复

暂无

## 变更

- `internal/mcpserver` 的 MCP 协议处理切换为基于 `github.com/modelcontextprotocol/go-sdk`，移除自实现报文编解码，复用官方 Server/Transport 能力简化维护。
- MCP `serverInfo.name` 改为从根命令名动态推导（为空时回退 `redant-mcp`），使对外标识与 CLI 应用名一致。
- MCP `tools/call` 在保留文本 `Content` 的同时，新增 `StructuredContent`（`ok/stdout/stderr/error/combined`）并声明 `OutputSchema`，便于上层程序化消费。

## 文档

- 补充 `README.md`、`docs/USAGE_AT_A_GLANCE.md` 与 `docs/DESIGN.md`：新增隐藏内部标志 `--args` 的用途、示例与 `RawArgs` 交互说明。
- 补充 `README.md`、`docs/DESIGN.md` 与 `docs/INDEX.md`：新增 MCP 集成入口、模块职责与阅读路径。

