# [Unreleased]

> 推荐维护方式：`/changelog draft|release`

## 新增

- `llms-txt --format skill`：支持生成符合 [Agent Skills 规范](https://agentskills.io/specification) 的 SKILL.md 文件，含 YAML frontmatter 校验（`name`、`description`、`compatibility` 字段约束）。
- `Command.Metadata` 的 `skill.*` 前缀约定：可通过 Metadata 配置 SKILL.md 的标准字段（`name`、`description`、`license`、`compatibility`、`allowed-tools`）及自定义 `metadata:` 嵌套属性。
- MCP 工具排除机制：通过 `Metadata["agent.exclude"] = "true"` 将命令及其子命令从 MCP tools/resources/prompts 中排除；内置基础设施命令（`mcp`、`completion`、`doc`、`llms-txt`、`readline`、`richline`、`web`、`webtty`）已默认标记排除。
- `llms-txt` 输出同步排除 `agent.exclude` 命令：Markdown、JSON、Skill 三种格式均过滤标记了 `agent.exclude` 的命令，与 MCP 行为一致。
- `mcp client` 子命令：内置 MCP 客户端，支持 `tools`、`resources`、`prompts`、`call`、`info` 操作，可连接自身（in-process）或外部 MCP 服务（`--command` stdio）。
- Web UI MCP 管理页面：在 `web` 命令的 Web 控制台中新增 MCP 标签页，提供 Tools/Resources/Prompts 浏览与 Tool 调用测试功能。
- `internal/mcpclient` 包：封装 MCP go-sdk 客户端，提供 `ConnectInProcess` 和 `ConnectStdio` 两种连接方式。

## 修复

- MCP 工具调用重复响应：修复 `ResponseHandler` / `ResponseStreamHandler` 的返回值同时出现在 Content text 和 StructuredContent 的问题。响应统一为 `{data, message}` 信封，仅通过 Content text 输出。
- 全局 flag 继承未生效：修复 `GlobalFlags()` 中设置了 `Inherit: false` 的 flag（如 `--list-format`）仍然被无条件注册到子命令的问题。

## 变更

- 简化 `Option` 继承配置：移除 `NoInherit`，统一使用 `Inherit bool` 控制是否向子命令继承（默认不继承）。
- 将 `--list-commands`、`--list-flags`、`--list-format` 调整为 `Inherit=false`，仅保留根命令可见/可用，避免扩散到子命令。
- 移除冗余 `DefaultOptionInherit` 常量；继承控制统一由 `Inherit` 字段显式声明。
- 移除未使用的 `Option.Category` 字段；flag 继承行为统一由 `Inherit` 控制。
- MCP 工具调用响应统一为 `{data, message}` JSON 信封：`data` 承载类型化结果或 stdout 文本，`message` 承载错误详情；移除原有 `structuredContent` 与 `ok/stdout/stderr/error/combined` 冗余字段。
- MCP `outputSchema` 字段从 `output`/`error`/`result` 收敛为 `data`/`message`，与响应信封保持一致。

## 文档

- 同步 MCP.md 输出结构（section 3/5/11）：反映 `{data, message}` 统一信封与 `outputSchema` 字段变更。
- 同步 DESIGN.md 执行上下文矩阵：更新 MCP callTool 节点描述。
