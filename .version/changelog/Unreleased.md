# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 新增隐藏全局标志 `--args`：支持以重复参数或 CSV 形式覆盖命令位置参数，用于在需要时通过 flag 直接注入/替代 `args`。
- 新增 MCP 能力：提供 `cmds/mcpcmd` 与 `internal/mcpserver`，支持将命令树映射为 MCP Tools，并通过 `mcp serve --transport stdio` 对外服务。
- 新增 Web 控制台能力：提供 `cmds/webcmd` 与 `internal/webui`，支持可视化选择命令、填写 flags/args、执行并查看调用过程与结果。

## 修复

- 修复根命令重复初始化场景下的全局标志重复注入问题，避免在 web 执行路径触发 `completion flag redefined: env` panic。
- 修复 Web 控制台 `enum-array` 交互异常（选择一个值时误全选）。
- 修复 Web 控制台中 args 映射丢失问题：当命名 `args` 缺失时回退使用 `rawArgs`，保证调用链路参数完整。
- 修复 `internal/webui/server_test.go` 中 `resp.Body.Close()` 返回值未检查导致的 `task lint` 失败。

## 变更

- `internal/mcpserver` 的 MCP 协议处理切换为基于 `github.com/modelcontextprotocol/go-sdk`，移除自实现报文编解码，复用官方 Server/Transport 能力简化维护。
- MCP `serverInfo.name` 改为从根命令名动态推导（为空时回退 `redant-mcp`），使对外标识与 CLI 应用名一致。
- MCP `tools/call` 在保留文本 `Content` 的同时，新增 `StructuredContent`（`ok/stdout/stderr/error/combined`）并声明 `OutputSchema`，便于上层程序化消费。
- Web 运行接口返回扩展为 `program + argv + invocation`，前端据此渲染反斜杠续行的多行 CLI 调用过程，提升长命令可读性。

## 文档

- 补充 `README.md`、`docs/USAGE_AT_A_GLANCE.md` 与 `docs/DESIGN.md`：新增隐藏内部标志 `--args` 的用途、示例与 `RawArgs` 交互说明。
- 补充 `README.md`、`docs/DESIGN.md` 与 `docs/INDEX.md`：新增 MCP 集成入口、模块职责与阅读路径。
- 补充 `README.md`、`docs/USAGE_AT_A_GLANCE.md`、`docs/DESIGN.md` 与 `docs/INDEX.md`：新增 Web 控制台说明、调用过程展示约定与参数顺序语义。
- 新增 `docs/MCP.md`：补全 MCP 子命令、工具映射规则、`tools/call` 输入输出协议、类型映射与常见问题排查。
- 重排 `README.md` 为总览型入口（精简章节与篇幅），将 Busybox/MCP/Web 等细节流程下沉到 `docs/*` 专项文档。

